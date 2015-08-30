package pxlocal

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strconv"

	"github.com/gorilla/websocket"
	"github.com/qiniu/log"
)

func StartAgent(debug bool, pURL *url.URL, subdomain, serverAddr string, remoteListenPort int) {
	if !debug {
		log.SetOutputLevel(log.Linfo)
	} else {
		log.SetOutputLevel(log.Ldebug)
	}
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
	query.Add("protocal", pURL.Scheme)
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
			println("client exit")
			break
		}
		log.Debug("recv:", msg)

		// sURL: serverURL
		rnl := NewRevNetListener()
		go handleRevConn(pURL, rnl)
		handleWsMsg(msg, sURL, rnl)
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
			log.Println("dial local:", pURL)
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
		log.Println("Unknown protocal:", pURL.Scheme)
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
