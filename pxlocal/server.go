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
	"time"

	"github.com/gobuild/log"
	"github.com/gorilla/websocket"
)

// :thinking
// start server HTTP service
// start agent
//  - agent connect server with websocket
//  - agent convert http request to conn
// need ref: revproxy

const (
	TCP_MIN_PORT = 40000
	TCP_MAX_PORT = 50000

	TYPE_NEWCONN = iota + 1
	TYPE_MESSAGE
	TYPE_IDLE
)

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	namedConnection = make(map[string]chan net.Conn, 10)
	proxyStats      = &ProxyStats{}
)

type message struct {
	Type int
	Name string
	Body string
}

type webSocketTunnel struct {
	wsconn *websocket.Conn
	data   string
	sync.Mutex
	index int64
}

var freeport = newFreePort(TCP_MIN_PORT, TCP_MAX_PORT)

func (t *webSocketTunnel) uniqName() string {
	t.Lock()
	defer t.Unlock()
	t.index += 1
	return fmt.Sprintf("%d", t.index)
}

func (t *webSocketTunnel) RequestNewConn(remoteAddr string) (net.Conn, error) {
	connC := make(chan net.Conn)
	namedConnection[remoteAddr] = connC
	defer delete(namedConnection, remoteAddr)

	// request a reverse connection
	var msg = message{Type: TYPE_NEWCONN, Name: remoteAddr}
	t.wsconn.WriteJSON(msg)
	select {
	case lconn := <-connC:
		if lconn == nil {
			return nil, errors.New("maybe hijack not supported, failed")
		}
		return lconn, nil
	case <-time.After(10 * time.Second):
		return nil, errors.New("wait reverse connection timeout(10s)")
	}
}

// used for httputil reverse proxy
func (t *webSocketTunnel) generateTransportDial() func(network, addr string) (net.Conn, error) {
	return func(network, addr string) (net.Conn, error) {
		log.Println("transport", network, addr)
		return t.RequestNewConn(t.uniqName())
	}
}

// Listen and forward connections
func newTcpProxyListener(tunnel *webSocketTunnel, port int) (listener *net.TCPListener, err error) {
	var laddr *net.TCPAddr
	if port != 0 {
		laddr, _ = net.ResolveTCPAddr("tcp", ":"+strconv.Itoa(port))
		listener, err = net.ListenTCP("tcp", laddr)
	} else {
		laddr, listener, err = freeport.ListenTCP()
	}
	if err != nil {
		return nil, err
	}
	port = laddr.Port
	// hook here
	err = hook(HOOK_TCP_POST_CONNECT, []string{
		"PORT=" + strconv.Itoa(port),
		"REMOTE_ADDR=" + tunnel.wsconn.RemoteAddr().String(),
		"CLIENT_ADDRESS=" + tunnel.wsconn.RemoteAddr().String(),
		"REMOTE_DATA=" + tunnel.data,
	})
	if err != nil {
		listener.Close()
		return
	}

	go func() {
		for {
			rconn, err := listener.AcceptTCP()
			if err != nil {
				log.Warn(err)
				break
			}
			// find proxy to where
			log.Debug("Receive new connections from", rconn.RemoteAddr())
			lconn, err := tunnel.RequestNewConn(rconn.RemoteAddr().String())
			if err != nil {
				log.Debug("request new conn err:", err)
				rconn.Close()
				continue
			}

			log.Debug("request new conn:", lconn, err)
			pc := &proxyConn{
				lconn: lconn,
				rconn: rconn,
				stats: proxyStats,
			}
			go pc.start()
		}
	}()
	return listener, nil
}

type RequestInfo struct {
	Protocol  string
	Subdomain string
	Port      int
	Data      string
}

func parseConnectRequest(r *http.Request) RequestInfo {
	protocol := r.FormValue("protocol")
	if protocol == "" {
		protocol = r.FormValue("protocal") // The last version has type error
	}
	if protocol == "" {
		protocol = "http"
	}

	var port int
	reqPort := r.FormValue("port")
	if reqPort == "" {
		port = 0
	} else {
		fmt.Sscanf(reqPort, "%d", &port)
	}
	subdomain := r.FormValue("subdomain")
	return RequestInfo{
		Protocol:  protocol,
		Subdomain: subdomain,
		Port:      port,
		Data:      r.FormValue("data"),
	}
}

type hijactRW struct {
	*net.TCPConn
	bufrw *bufio.ReadWriter
}

func (this *hijactRW) Write(data []byte) (int, error) {
	nn, err := this.bufrw.Write(data)
	this.bufrw.Flush()
	return nn, err
}

func (this *hijactRW) Read(p []byte) (int, error) {
	return this.bufrw.Read(p)
}

func newHijackReadWriteCloser(conn *net.TCPConn, bufrw *bufio.ReadWriter) net.Conn {
	return &hijactRW{
		bufrw:   bufrw,
		TCPConn: conn,
	}
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	var proxyFor = r.Header.Get("X-Proxy-For")
	log.Infof("[remote %s] X-Proxy-For [%s]", r.RemoteAddr, proxyFor)

	connC, ok := namedConnection[proxyFor]
	if !ok {
		http.Error(w, "inside error: proxy not ready to receive conn", http.StatusInternalServerError)
		return
	}
	conn, err := hijackHTTPRequest(w)
	if err != nil {
		log.Warnf("hijeck failed, %v", err)
		connC <- nil
		return
	}
	connC <- conn
}

func hijackHTTPRequest(w http.ResponseWriter) (conn net.Conn, err error) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		err = errors.New("webserver don't support hijacking")
		return
	}

	hjconn, bufrw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}
	conn = newHijackReadWriteCloser(hjconn.(*net.TCPConn), bufrw)
	return
}

type ProxyServer struct {
	domain string
	*http.ServeMux
	revProxies map[string]*httputil.ReverseProxy
	sync.RWMutex
}

func wsSendMessage(conn *websocket.Conn, text string) error {
	return conn.WriteJSON(&message{Type: TYPE_MESSAGE, Body: text})
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
		//protocol, subdomain, port
		reqInfo := parseConnectRequest(r)
		log.Debugf("proxy listen proto: %v, subdomain: %v port: %v",
			reqInfo.Protocol, reqInfo.Subdomain, reqInfo.Port)

		// create websocket connection
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		defer conn.Close()
		log.Debug("remote client addr:", conn.RemoteAddr())

		tunnel := &webSocketTunnel{
			wsconn: conn,
			data:   reqInfo.Data,
		}
		// TCP: create new port to listen
		log.Infof("New %s proxy for %v", reqInfo.Protocol, conn.RemoteAddr())
		switch reqInfo.Protocol {
		case "tcp":
			listener, err := newTcpProxyListener(tunnel, reqInfo.Port)
			if err != nil {
				log.Warnf("new tcp proxy err: %v", err)
				http.Error(w, err.Error(), 501)
				return
			}
			defer listener.Close()
			_, port, _ := net.SplitHostPort(listener.Addr().String())
			wsSendMessage(conn, fmt.Sprintf("%s:%v", ps.domain, port))
		case "http", "https":
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
			if reqInfo.Subdomain == "" {
				reqInfo.Subdomain = uniqName(5) + ".t"
			}
			pxDomain := reqInfo.Subdomain + "." + ps.domain
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
			log.Warn("unknown protocol:", reqInfo.Protocol)
			return
		}
		// HTTP: use httputil.ReverseProxy
		for {
			var msg message
			if err := conn.ReadJSON(&msg); err != nil {
				log.Warn(err)
				break
			}
			log.Debug("recv json:", msg)
		}
	}
}

func (p *ProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// http://stackoverflow.com/questions/6899069/why-are-request-url-host-and-scheme-blank-in-the-development-server
	r.URL.Scheme = "http" // ??
	r.URL.Host = r.Host   // ??
	log.Debug("URL path:", r.URL.Path)
	log.Debugf("proxy lists: %v", p.revProxies)
	if rpx, ok := p.revProxies[r.Host]; ok {
		log.Debug("server http rev proxy")
		rpx.ServeHTTP(w, r)
		return
	}

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
