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
	"strconv"

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

func FigureListenAddress(r *http.Request) string {
	var listenPort int
	reqPort := r.FormValue("port")
	if reqPort == "" {
		listenPort = 12345
	} else {
		fmt.Sscanf(reqPort, "%d", &listenPort)
		// ignore error
	}
	return fmt.Sprintf("0.0.0.0:%d", listenPort)
}

func ControlHandler(w http.ResponseWriter, r *http.Request) {
	// read listen port from request
	proxyAddr := FigureListenAddress(r)
	log.Println("proxy listen addr:", proxyAddr)

	// create websocket connection
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	defer conn.Close()
	log.Println(conn.RemoteAddr())

	// create new port to listen
	tunnel := &Tunnel{wsconn: conn}
	listener, err := NewProxyListener(tunnel, proxyAddr)
	if err != nil {
		http.Error(w, err.Error(), 501)
		return
	}
	defer listener.Close()

	for {
		var msg Msg
		if err := conn.ReadJSON(&msg); err != nil {
			log.Println(err)
			break
		}
		log.Println("recv json:", msg)
	}
	// agentName := r.RemoteAddr
	// pipech := NewPipeChan() // for each connection generate a pipe chan, use forward port as name
	// namedChan[agentName] = pipech

	// // go func() {
	// for msg := range pipech.recvChan { // recv request from browser or some tcp client
	// 	// request agent create new connection
	// 	if err := conn.WriteJSON(msg); err != nil {
	// 		break
	// 	}
	// 	// get reverse connection
	// 	if err := conn.ReadJSON(&msg); err != nil {
	// 		log.Println(err)
	// 		break
	// 	}
	// }
	// }()
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

func ProxyHandler(w http.ResponseWriter, r *http.Request) {
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

	// defer conn.Close()
	// _ = bufrw
	// // TODO
}

func HomepageHandler(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, fmt.Sprintf("recvBytes: %d, sendBytes: %d",
		proxyStats.receivedBytes, proxyStats.sentBytes))
}

func startAgent(proxyAddr string, serverAddr string, remoteListenPort int) {
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
	if remoteListenPort != 0 {
		query := u.Query()
		query.Add("port", strconv.Itoa(remoteListenPort))
		u.RawQuery = query.Encode()
	}

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
		// var sendMsg Msg
		// if err := wsclient.WriteJSON(sendMsg); err != nil {
		// 	log.Println("write err:", err)
		// 	break
		// }
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
	flag.BoolVar(&serverMode, "s", false, "run in server mode")
	flag.StringVar(&serverAddr, "addr", "localhost:5000", "server address")
	flag.IntVar(&proxyPort, "proxy-port", 0, "server proxy listen port")
	flag.StringVar(&proxyAddr, "proxy", "www.163.com:80", "proxyed service address")

	flag.Parse()

	if serverMode {
		fmt.Println("Hello proxy local")
		http.HandleFunc("/", HomepageHandler)
		http.HandleFunc("/ws", ControlHandler)
		http.HandleFunc("/proxyhijack", ProxyHandler)
		log.Fatal(http.ListenAndServe(addr, nil))
	}

	startAgent(proxyAddr, "http://"+serverAddr, proxyPort)
}
