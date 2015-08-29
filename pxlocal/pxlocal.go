package pxlocal

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
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
	*net.TCPConn
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

func NewHijackReadWriteCloser(conn *net.TCPConn, bufrw *bufio.ReadWriter) net.Conn {
	return &HijactRW{
		bufrw:   bufrw,
		TCPConn: conn,
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
	if _, ok := hjconn.(*net.TCPConn); ok {
		log.Println("Hijack is tcp conn")
	}

	conn := NewHijackReadWriteCloser(hjconn.(*net.TCPConn), bufrw)
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
