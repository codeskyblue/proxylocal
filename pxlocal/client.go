package pxlocal

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/qiniu/log"
)

var ErrWebsocketBroken = errors.New("Error websocket connection")
var ErrDialTCP = errors.New("Error dial tcp connection")
var ErrUnknownProtocol = errors.New("Unknown protocol")

type AgentOptions struct {
	Subdomain        string
	ServerAddr       string
	RemoteListenPort int
	Data             string
}

// pURL: proxy address
// sURL: server address
func StartAgent(pURL, sURL *url.URL, opt AgentOptions) error {
	log.Debug("start proxy", pURL)
	log.Debug("server host:", sURL.Host)

	sURL.Path = "/ws"
	conn, err := net.Dial("tcp", sURL.Host)
	if err != nil {
		return ErrDialTCP
	}
	// specify remote listen port
	sURL.Scheme = "ws"
	query := sURL.Query()
	query.Add("protocol", pURL.Scheme)
	query.Add("subdomain", opt.Subdomain)
	query.Add("data", opt.Data)
	if opt.RemoteListenPort != 0 {
		query.Add("port", strconv.Itoa(opt.RemoteListenPort))
	}
	sURL.RawQuery = query.Encode()

	log.Debug(sURL)
	wsclient, _, err := websocket.NewClient(conn, sURL, nil, 1024, 1024)
	if err != nil {
		return err
	}
	defer wsclient.Close()
	go idleWsSend(wsclient)

	rnl := NewRevNetListener()
	defer rnl.Close()

	go serveRevConn(pURL, rnl) // handle conn from rnl
	for {
		var msg Msg
		if err := wsclient.ReadJSON(&msg); err != nil {
			fmt.Println("client exit: " + err.Error())
			return ErrWebsocketBroken
		}
		log.Debug("recv:", msg)

		// sURL: serverURL
		go handleWsMsg(msg, sURL, rnl) // send new conn to rnl
	}
}

func idleWsSend(wsc *websocket.Conn) {
	var msg Msg
	msg.Type = TYPE_IDLE
	msg.Name = "idle"
	for {
		if err := wsc.WriteJSON(&msg); err != nil {
			log.Warnf("write idle msg error: %v", err)
			break
		}
		time.Sleep(5 * time.Second)
	}
}

type RevNetListener struct {
	connCh chan net.Conn
}

func NewRevNetListener() *RevNetListener {
	return &RevNetListener{
		connCh: make(chan net.Conn, 100),
	}
}

func (r *RevNetListener) Accept() (net.Conn, error) {
	conn, ok := <-r.connCh
	if !ok {
		return nil, errors.New("RevNet Closed")
	}
	return conn, nil
}

func (r *RevNetListener) Addr() net.Addr {
	return nil
}

func (r *RevNetListener) Close() error {
	close(r.connCh)
	return nil
}

// msg comes from px server by websocket
// 1: connect to px server, use msg.Name to identify self.
// 2: change conn to reverse conn
func handleWsMsg(msg Msg, sURL *url.URL, rnl *RevNetListener) {
	u := sURL
	switch msg.Type {
	case TYPE_NEWCONN:
		log.Debug("dial remote:", u.Host)
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
		rnl.connCh <- sconn
	case TYPE_MESSAGE:
		fmt.Printf("Recv Message: %v\n", msg.Body)
	default:
		log.Warnf("Type: %v not support", msg.Type)
	}
}

func serveRevConn(pURL *url.URL, lis net.Listener) error {
	switch pURL.Scheme {
	case "tcp":
		for {
			rconn, err := lis.Accept()
			if err != nil {
				log.Errorf("accept error: %v", err)
				return err
			}
			log.Info("dial local:", pURL)
			lconn, err := net.Dial("tcp", pURL.Host)
			if err != nil {
				// wsclient
				log.Warn(err)
				rconn.Close()
				return err
			}
			// start forward local proxy
			pc := &ProxyConn{
				lconn: lconn,
				rconn: rconn,
				stats: proxyStats,
			}
			go pc.start()
		}
	case "http", "https":
		remote := pURL
		rp := &httputil.ReverseProxy{
			Director: func(req *http.Request) {
				req.Host = remote.Host
				req.URL.Scheme = remote.Scheme
				req.URL.Host = remote.Host
			},
		}
		return http.Serve(lis, rp)
	default:
		log.Println("Unknown protocol:", pURL.Scheme)
		return ErrUnknownProtocol
	}
}
