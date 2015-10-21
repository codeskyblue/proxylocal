package pxlocal

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/qiniu/log"
)

func StartAgent(pURL *url.URL, subdomain, serverAddr string, remoteListenPort int) {
	log.Debug("start proxy", pURL)
	if !regexp.MustCompile("^http[s]://").MatchString(serverAddr) {
		serverAddr = "http://" + serverAddr
	}
	sURL, err := url.Parse(serverAddr)
	if err != nil {
		log.Fatal(err)
	}

	sURL.Path = "/ws"
	log.Debug("server host:", sURL.Host)
	conn, err := net.Dial("tcp", sURL.Host)
	if err != nil {
		log.Fatal(err)
	}
	// specify remote listen port
	query := sURL.Query()
	query.Add("protocol", pURL.Scheme)
	query.Add("subdomain", subdomain)
	if remoteListenPort != 0 {
		query.Add("port", strconv.Itoa(remoteListenPort))
	}
	sURL.RawQuery = query.Encode()

	wsclient, _, err := websocket.NewClient(conn, sURL, nil, 1024, 1024)
	if err != nil {
		log.Fatal(err)
	}
	defer wsclient.Close()

	for {
		var msg Msg
		if err := wsclient.ReadJSON(&msg); err != nil {
			fmt.Println("client exit: " + err.Error())
			break
		}
		log.Debug("recv:", msg)

		// sURL: serverURL
		rnl := NewRevNetListener()
		go handleRevConn(pURL, rnl)
		go idleWsSend(wsclient)
		handleWsMsg(msg, sURL, rnl)
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
	return <-r.connCh, nil
}

func (r *RevNetListener) Addr() net.Addr {
	return nil
}

func (r *RevNetListener) Close() error {
	return nil
}

func handleRevConn(pURL *url.URL, lis net.Listener) {
	switch pURL.Scheme {
	case "tcp":
		for {
			rconn, err := lis.Accept()
			if err != nil {
				log.Errorf("accept error: %v", err)
				return
			}
			log.Info("dial local:", pURL)
			lconn, err := net.Dial("tcp", pURL.Host)
			if err != nil {
				// wsclient
				log.Println(err)
				rconn.Close()
				break
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
		http.Serve(lis, rp)
	default:
		log.Println("Unknown protocol:", pURL.Scheme)
	}
}

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
