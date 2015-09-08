package pxlocal

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
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
	TCP_MIN_PORT = 13000
	TCP_MAX_PORT = 14000

	TYPE_NEWCONN = iota + 1
	TYPE_MESSAGE
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
	Body string
}

type Tunnel struct {
	wsconn *websocket.Conn
	sync.Mutex
	index    int64
	freeport *FreePort
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

/*
func listenTcpInRangePort(port, minPort, maxPort int) (finnalPort int, lis *net.TCPListener, err error) {
	if port != 0 {
		laddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			return 0, nil, err
		}
		lis, err = net.ListenTCP("tcp", laddr)
		return port, lis, err
	}
	for port = minPort; port < maxPort; port++ {
		laddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			return 0, nil, err
		}
		lis, err := net.ListenTCP("tcp", laddr)
		if err == nil {
			return port, lis, nil
		}
	}
	return 0, nil, errors.New("No port avaliable")
}
*/

// Listen and forward connections
func NewTcpProxyListener(tunnel *Tunnel, port int) (listener *net.TCPListener, err error) {
	var laddr *net.TCPAddr
	if port != 0 {
		laddr, _ = net.ResolveTCPAddr("tcp", ":"+strconv.Itoa(port))
		listener, err = net.ListenTCP("tcp", laddr)
	} else {
		laddr, listener, err = tunnel.freeport.ListenTCP()
	}
	if err != nil {
		return nil, err
	}
	port = laddr.Port
	//port, listener, err = listenTcpInRangePort(port, TCP_MIN_PORT, TCP_MAX_PORT)
	//if err != nil {
	//return nil, err
	//}
	// hook here
	err = hook(HOOK_TCP_POST_CONNECT, []string{
		"PORT=" + strconv.Itoa(port),
		"CLIENT_ADDRESS=" + tunnel.wsconn.RemoteAddr().String(),
	})
	if err != nil {
		listener.Close()
		return
	}

	go func() {
		for {
			rconn, err := listener.AcceptTCP()
			if err != nil {
				log.Println(err)
				break
			}
			// find proxy to where
			log.Println("Receive new connections from", rconn.RemoteAddr())
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

func parseConnectRequest(r *http.Request) (protocal, subdomain string, port int) {
	protocal = r.FormValue("protocal")
	if protocal == "" {
		protocal = "http"
	}
	reqPort := r.FormValue("port")
	if reqPort == "" {
		port = 0
	} else {
		fmt.Sscanf(reqPort, "%d", &port)
	}
	subdomain = r.FormValue("subdomain")
	return
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

type ProxyServer struct {
	domain string
	*http.ServeMux
	revProxies map[string]*httputil.ReverseProxy
	sync.RWMutex
}

func wsSendMessage(conn *websocket.Conn, message string) error {
	return conn.WriteJSON(&Msg{Type: TYPE_MESSAGE, Body: message})
}

func (ps *ProxyServer) newHomepageHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "text/html")
		io.WriteString(w, fmt.Sprintf("<b>TCP:</b> recvBytes: %d, sendBytes: %d <br>",
			proxyStats.receivedBytes, proxyStats.sentBytes))
		io.WriteString(w, fmt.Sprintf("<b>HTTP:</b> ...<br>"))
		io.WriteString(w, "<hr>")
		for pname, _ := range ps.revProxies {
			io.WriteString(w, fmt.Sprintf("http proxy: %s <br>", pname))
		}
	}
}

func (ps *ProxyServer) newControlHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		// read listen port from request
		protocal, subdomain, port := parseConnectRequest(r)
		log.Println("proxy listen addr:", protocal, subdomain, port)

		// create websocket connection
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		defer conn.Close()
		log.Println(conn.RemoteAddr())

		tunnel := &Tunnel{
			wsconn:   conn,
			freeport: NewFreePort(TCP_MIN_PORT, TCP_MAX_PORT),
		}
		// TCP: create new port to listen
		switch protocal {
		case "tcp":
			// proxyAddr := fmt.Sprintf("0.0.0.0:%d", port)
			listener, err := NewTcpProxyListener(tunnel, port)
			if err != nil {
				log.Warnf("new tcp proxy err: %v", err)
				http.Error(w, err.Error(), 501)
				return
			}
			defer listener.Close()
			host, _, _ := net.SplitHostPort(ps.domain)
			_, port, _ := net.SplitHostPort(listener.Addr().String())
			wsSendMessage(conn, fmt.Sprintf(
				"Local tcp conn is now publicly available via:\n%v:%v\n", host, port))
		case "http", "https":
			log.Println("start http proxy")
			tr := &http.Transport{
				Dial: tunnel.generateTransportDial(),
			}
			revProxy := &httputil.ReverseProxy{
				Director: func(req *http.Request) {
					log.Println("director:", req.RequestURI)
				},
				Transport: tr,
			}
			// should hook here
			// hook(HOOK_CREATE_HTTP_SUBDOMAIN, subdomain)
			// generate a uniq domain
			if subdomain == "" {
				subdomain = uniqName(5) + ".t"
			}
			pxDomain := subdomain + "." + ps.domain
			log.Println("http px use domain:", pxDomain)
			if _, exists := ps.revProxies[pxDomain]; exists {
				wsSendMessage(conn, fmt.Sprintf("subdomain [%s] has already been taken", pxDomain))
				return
			}
			ps.Lock()
			ps.revProxies[pxDomain] = revProxy
			ps.Unlock()
			wsSendMessage(conn, fmt.Sprintf(
				"Local server is now publicly available via:\nhttp://%s\n", pxDomain))

			defer func() {
				ps.Lock()
				delete(ps.revProxies, pxDomain)
				ps.Unlock()
			}()
		default:
			log.Println("unknown protocal:", protocal)
			return
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
}

func (p *ProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Println("request info:", r.Method, r.Host, r.RequestURI)
	//host, _, _ := net.SplitHostPort(r.Host)
	// http://stackoverflow.com/questions/6899069/why-are-request-url-host-and-scheme-blank-in-the-development-server
	r.URL.Scheme = "http" // ??
	r.URL.Host = r.Host   // ??
	log.Println("URL path:", r.URL.Path)
	log.Printf("pxies: %v", p.revProxies)
	if rpx, ok := p.revProxies[r.Host]; ok {
		log.Println("server http rev proxy")
		rpx.ServeHTTP(w, r)
		return
	}

	//if p.domain != host {
	//	http.Error(w, fmt.Sprintf("%s not ready", host), 504)
	//	return
	//}

	h, _ := p.Handler(r)
	h.ServeHTTP(w, r)
}

// domain, ex shengxiang.me
// dns should set *.shengxiang.me
func NewProxyServer(domain string) *ProxyServer {
	if domain == "" {
		domain = "localhost"
	}
	p := &ProxyServer{
		domain:     domain,
		ServeMux:   http.NewServeMux(),
		revProxies: make(map[string]*httputil.ReverseProxy),
	}
	p.HandleFunc("/", p.newHomepageHandler())
	p.HandleFunc("/ws", p.newControlHandler())
	p.HandleFunc("/proxyhijack", proxyHandler)

	return p
}
