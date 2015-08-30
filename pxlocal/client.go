package pxlocal

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"

	"github.com/gorilla/websocket"
	"github.com/qiniu/log"
)

func StartAgent(debug bool, protocal, subdomain, proxyAddr string, serverAddr string, remoteListenPort int) {
	if !debug {
		log.SetOutputLevel(log.Linfo)
	}
	log.Debug("start proxy", proxyAddr)
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
	query.Add("protocal", protocal)
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
		handleWsMsg(msg, sURL, proxyAddr)
	}
}

func handleWsMsg(msg Msg, sURL *url.URL, pAddr string) {
	u := sURL
	switch msg.Type {
	case TYPE_NEWCONN:
		log.Println("dial remote:", u.Host)
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
		log.Println("dial local:", pAddr)
		lconn, err := net.Dial("tcp", pAddr)
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
	case TYPE_MESSAGE:
		fmt.Printf("Recv Message: %v\n", msg.Body)
	default:
		log.Warnf("Type: %v not support", msg.Type)
	}
}
