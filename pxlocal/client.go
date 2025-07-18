package pxlocal

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/gobuild/log"
	"github.com/gorilla/websocket"
)

var (
	ErrWebsocketBroken  = errors.New("Error websocket connection")
	ErrDialTCP          = errors.New("Error dial tcp connection")
	ErrUnknownProtocol  = errors.New("Unknown protocol")
	ErrPrototolRequired = errors.New("Protocol required")
)

type ProxyProtocol string

const (
	TCP  = ProxyProtocol("tcp")
	HTTP = ProxyProtocol("http")
)

type ProxyOptions struct {
	LocalAddr  string
	Proto      ProxyProtocol
	Subdomain  string
	ListenPort int
	ExtraData  string
}

type Client struct {
	sURL *url.URL
}

// Proxy Client
func NewClient(serverAddr string) *Client {
	u := &url.URL{
		Scheme: "ws",
		Host:   serverAddr,
		Path:   "/ws",
	}
	return &Client{u}
}

type ProxyConnector struct {
	wsConn     *websocket.Conn
	err        error
	wg         sync.WaitGroup
	remoteAddr string
}

func (p *ProxyConnector) Close() error {
	return p.wsConn.Close()
}

func (p *ProxyConnector) Wait() error {
	p.wg.Wait()
	return p.err
}

func (p *ProxyConnector) RemoteAddr() string {
	return p.remoteAddr
}

// This is a immediately return function
func (c *Client) RunProxy(opts ProxyOptions) (pc *ProxyConnector, err error) {
	if opts.Proto == "" {
		return nil, ErrPrototolRequired
	}
	q := c.sURL.Query()
	q.Add("protocol", string(opts.Proto))
	q.Add("subdomain", opts.Subdomain)
	q.Add("data", opts.ExtraData)
	if opts.ListenPort != 0 {
		q.Add("port", strconv.Itoa(opts.ListenPort))
	}
	c.sURL.RawQuery = q.Encode()

	conn, err := net.Dial("tcp", c.sURL.Host)
	if err != nil {
		return nil, ErrDialTCP
	}
	wsclient, _, err := websocket.NewClient(conn, c.sURL, nil, 1024, 1024)
	if err != nil {
		return nil, err
	}
	pc = &ProxyConnector{wsConn: wsclient}
	var msg message
	if err := wsclient.ReadJSON(&msg); err != nil {
		return nil, err
	}
	pc.remoteAddr = msg.Body

	pc.wg.Add(1)
	go idleWsSend(wsclient) // keep websocket alive to prevent nginx timeout issue
	go func() {
		defer wsclient.Close()
		revListener := newRevNetListener()
		defer revListener.Close()
		defer pc.wg.Done()

		go serveRevConn(opts.Proto, opts.LocalAddr, revListener)
		for {
			if err := wsclient.ReadJSON(&msg); err != nil {
				pc.err = err
				return
			}
			go handleWsMsg(msg, c.sURL, revListener) // send new conn to rnl
		}
	}()
	return pc, nil
}

func idleWsSend(wsc *websocket.Conn) {
	var msg message
	msg.Type = TYPE_IDLE
	msg.Name = "idle"
	for {
		if err := wsc.WriteJSON(&msg); err != nil {
			break
		}
		time.Sleep(5 * time.Second)
	}
}

type reverseNetListener struct {
	connCh chan net.Conn
}

func newRevNetListener() *reverseNetListener {
	return &reverseNetListener{
		connCh: make(chan net.Conn, 100),
	}
}

func (r *reverseNetListener) Accept() (net.Conn, error) {
	conn, ok := <-r.connCh
	if !ok {
		return nil, errors.New("RevNet Closed")
	}
	return conn, nil
}

func (r *reverseNetListener) Addr() net.Addr {
	return nil
}

func (r *reverseNetListener) Close() error {
	close(r.connCh)
	return nil
}

// msg comes from px server by websocket
// 1: connect to px server, use msg.Name to identify self.
// 2: change conn to reverse conn
func handleWsMsg(msg message, sURL *url.URL, rnl *reverseNetListener) {
	u := sURL
	switch msg.Type {
	case TYPE_NEWCONN:
		log.Debug("dial remote:", u.Host)
		sconn, err := net.Dial("tcp", u.Host)
		if err != nil {
			log.Println(err)
			break
		}
		log.Infof("proxy for: %s", msg.Name)
		_, err = sconn.Write([]byte(fmt.Sprintf(
			"GET /proxyhijack HTTP/1.1\r\nHost: proxylocal\r\nX-Proxy-For: %s \r\n\r\n", msg.Name)))
		if err != nil {
			log.Println(err)
			break
		}
		rnl.connCh <- sconn
	case TYPE_MESSAGE:
		fmt.Printf("Recv Message: %v\n", msg.Body)
	default:
		log.Warnf("Type: %v not support", msg.Type)
	}
}

func serveRevConn(proto ProxyProtocol, pAddr string, lis net.Listener) error {
	switch proto {
	case TCP:
		for {
			rconn, err := lis.Accept()
			if err != nil {
				log.Errorf("accept error: %v", err)
				return err
			}
			log.Info("local dial tcp", pAddr)
			lconn, err := net.Dial("tcp", pAddr)
			if err != nil {
				log.Warn(err)
				rconn.Close()
				return err
			}
			// start forward local proxy
			pc := &proxyConn{
				lconn: lconn,
				rconn: rconn,
				stats: proxyStats,
			}
			go pc.start()
		}
	case HTTP:
		rp := &httputil.ReverseProxy{
			Director: func(req *http.Request) {
				req.Host = pAddr
				req.URL.Scheme = "http"
				req.URL.Host = pAddr
			},
		}
		return http.Serve(lis, rp)
	default:
		log.Println("Unknown protocol:", proto)
		return ErrUnknownProtocol
	}
}
