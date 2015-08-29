package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/qiniu/log"
)

// :thinking
// start server HTTP service
// start agent
//  - agent connect server with websocket
//  - agent convert http request to conn
// need ref: revproxy

const (
	TYPE_NEWCONN = iota + 1
)

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	namedConnection = make(map[string]chan net.Conn, 10)
	proxyStats      = &ProxyStats{}
)

type Msg struct {
	Type int
	Name string
}

type Tunnel struct {
	wsconn *websocket.Conn
	sync.Mutex
	index int64
}

func (t *Tunnel) uniqName() string {
	t.Lock()
	defer t.Unlock()
	t.index += 1
	return fmt.Sprintf("%d", t.index)
}

func (t *Tunnel) RequestNewConn(remoteAddr string) (net.Conn, error) {
	connCh := make(chan net.Conn)
	namedConnection[remoteAddr] = connCh
	defer delete(namedConnection, remoteAddr)

	// request a reverse connection
	var msg = Msg{Type: TYPE_NEWCONN, Name: remoteAddr}
	t.wsconn.WriteJSON(msg)
	lconn := <-connCh
	if lconn == nil {
		return nil, errors.New("maybe hijack not supported, failed")
	}
	return lconn, nil
}

// used for httputil reverse proxy
func (t *Tunnel) generateTransportDial() func(network, addr string) (net.Conn, error) {
	return func(network, addr string) (net.Conn, error) {
		log.Println("transport", network, addr)
		return t.RequestNewConn(t.uniqName())
	}
}

// Listen and forward connections
func NewProxyListener(tunnel *Tunnel, listenAddress string) (lis net.Listener, err error) {
	laddr, err := net.ResolveTCPAddr("tcp", listenAddress)
	if err != nil {
		return nil, err
	}
	listener, err := net.ListenTCP("tcp", laddr)
	if err != nil {
		return nil, err
	}
	go func() {
		for {
			rconn, err := listener.AcceptTCP()
			if err != nil {
				log.Println(err)
				break
			}
			// find proxy to where
			log.Println(laddr, "Receive new connections from", rconn.RemoteAddr())
			lconn, err := tunnel.RequestNewConn(rconn.RemoteAddr().String())
			if err != nil {
				log.Println("request new conn err:", err)
				rconn.Close()
				continue
			}

			log.Println("request new conn:", lconn, err)
			pc := &ProxyConn{
				lconn: lconn,
				rconn: rconn,
				stats: proxyStats,
			}
			go pc.start()
		}
	}()
	return listener, nil
}

func FigureListenAddress(r *http.Request) (protocal, subdomain string, port int) {
	protocal = r.FormValue("protocal")
	if protocal == "" {
		protocal = "http"
	}
	reqPort := r.FormValue("port")
	if reqPort == "" {
		port = 12345
	} else {
		fmt.Sscanf(reqPort, "%d", &port)
	}
	subdomain = r.FormValue("subdomain")
	return
}

func controlHandler(w http.ResponseWriter, r *http.Request) {
	// read listen port from request
	protocal, subdomain, port := FigureListenAddress(r)
	log.Println("proxy listen addr:", protocal, subdomain, port)

	// create websocket connection
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	defer conn.Close()
	log.Println(conn.RemoteAddr())

	tunnel := &Tunnel{wsconn: conn}
	// TCP: create new port to listen
	switch protocal {
	case "tcp":
		proxyAddr := fmt.Sprintf("0.0.0.0:%d", port)
		listener, err := NewProxyListener(tunnel, proxyAddr)
		if err != nil {
			http.Error(w, err.Error(), 501)
			return
		}
		defer listener.Close()
	case "http":
		log.Println("Not implement")
	}
	// HTTP: use httputil.ReverseProxy

	for {
		var msg Msg
		if err := conn.ReadJSON(&msg); err != nil {
			log.Println(err)
			break
		}
		log.Println("recv json:", msg)
	}
}

type HijactRW struct {
	net.Conn
	bufrw *bufio.ReadWriter
}

func (this *HijactRW) Write(data []byte) (int, error) {
	nn, err := this.bufrw.Write(data)
	this.bufrw.Flush()
	return nn, err
}

func (this *HijactRW) Read(p []byte) (int, error) {
	return this.bufrw.Read(p)
}

func NewHijackReadWriteCloser(conn net.Conn, bufrw *bufio.ReadWriter) net.Conn {
	return &HijactRW{
		bufrw: bufrw,
		Conn:  conn,
	}
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "webserver don't support hijacking", http.StatusInternalServerError)
		return
	}

	var proxyFor = r.Header.Get("X-Proxy-For")
	log.Println("proxy name:", proxyFor)

	connCh, ok := namedConnection[proxyFor]
	if !ok {
		http.Error(w, "inside error: proxy not ready to receive conn", http.StatusInternalServerError)
		return
	}
	hjconn, bufrw, err := hj.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		connCh <- nil
		return
	}

	conn := NewHijackReadWriteCloser(hjconn, bufrw)
	connCh <- conn
}

func homepageHandler(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, fmt.Sprintf("recvBytes: %d, sendBytes: %d",
		proxyStats.receivedBytes, proxyStats.sentBytes))
}

type ProxyServer struct {
	*http.ServeMux
}

func (p *ProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Println(r.RequestURI)
	h, _ := p.Handler(r)
	h.ServeHTTP(w, r)
}

//func (p *ProxyServer) AddHttpReverseProxy(host string

func NewProxyServer() *ProxyServer {
	p := &ProxyServer{
		ServeMux: http.NewServeMux(),
	}
	p.HandleFunc("/", homepageHandler)
	p.HandleFunc("/ws", controlHandler)
	p.HandleFunc("/proxyhijack", proxyHandler)

	//rp := httputil.ReverseProxy{}
	//rp.Transport
	return p
}

func startAgent(proxyAddr string, serverAddr string, remoteListenPort int) {
	if !regexp.MustCompile("^http[s]://").MatchString(serverAddr) {
		serverAddr = "http://" + serverAddr
	}
	u, err := url.Parse(serverAddr)
	if err != nil {
		log.Fatal(err)
	}

	u.Path = "/ws"
	log.Println("host:", u.Host)
	conn, err := net.Dial("tcp", u.Host)
	if err != nil {
		log.Fatal(err)
	}
	// specify remote listen port
	query := u.Query()
	query.Add("protocal", "tcp")
	query.Add("subdomain", "")
	if remoteListenPort != 0 {
		query.Add("port", strconv.Itoa(remoteListenPort))
	}
	u.RawQuery = query.Encode()

	wsclient, _, err := websocket.NewClient(conn, u, nil, 1024, 1024)
	if err != nil {
		log.Fatal(err)
	}
	defer wsclient.Close()

	for {
		var msg Msg
		if err := wsclient.ReadJSON(&msg); err != nil {
			log.Println("recv err:", err)
			break
		}
		log.Println("recv:", msg)

		sconn, err := net.Dial("tcp", u.Host)
		if err != nil {
			log.Println(err)
			break
		}
		_, err = sconn.Write([]byte(fmt.Sprintf(
			"GET /proxyhijack HTTP/1.1\r\nX-Proxy-For: %s \r\n\r\n", msg.Name)))
		if err != nil {
			log.Println(err)
			break
		}

		// call local service
		lconn, err := net.Dial("tcp", proxyAddr)
		if err != nil {
			// wsclient
			log.Println(err)
			sconn.Close()
			break
		}
		// start forward local proxy
		pc := &ProxyConn{
			lconn: lconn,
			rconn: sconn,
			stats: proxyStats,
		}
		go pc.start()
	}
}

func main() {
	var addr = ":5000"
	var serverMode bool
	var serverAddr string
	var proxyPort int
	var proxyAddr string
	var subDomain string
	flag.BoolVar(&serverMode, "server", false, "run in server mode")
	flag.StringVar(&serverAddr, "addr", "localhost:5000", "server address")
	flag.IntVar(&proxyPort, "proxy-port", 0, "server proxy listen port")
	flag.StringVar(&subDomain, "subdomain", "", "proxy subdomain")

	flag.Usage = func() {
		fmt.Printf("Usage: %s [OPTIONS] <port | host:port>\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if !serverMode && len(flag.Args()) != 1 {
		flag.Usage()
		return
	}

	proxyAddr = flag.Arg(0)
	if !strings.Contains(proxyAddr, ":") { // only contains port
		proxyAddr = "localhost:" + proxyAddr
	}

	if serverMode {
		fmt.Println("Hello proxy local")
		ps := NewProxyServer()
		//http.HandleFunc("/", homepageHandler)
		//http.HandleFunc("/ws", controlHandler)
		//http.HandleFunc("/proxyhijack", ProxyHandler)
		log.Fatal(http.ListenAndServe(addr, ps))
	}

	startAgent(proxyAddr, serverAddr, proxyPort)
}
