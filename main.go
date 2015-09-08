package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"

	"github.com/codeskyblue/proxylocal/pxlocal"
	"github.com/qiniu/log"
)

const (
	VERSION = "0.1"
)

func main() {
	var serverMode bool
	var serverAddr string
	var proxyPort int
	var proxyAddr string
	var subDomain string
	var domain string
	var debug bool

	var defaultServerAddr = os.Getenv("PXL_SERVER_ADDR")
	if defaultServerAddr == "" {
		defaultServerAddr = "proxylocal.xyz"
	}
	flag.BoolVar(&serverMode, "server", false, "run in server mode")
	flag.StringVar(&serverAddr, "server-addr", defaultServerAddr, "server address")
	flag.StringVar(&domain, "server-domain", "", "proxy server domain name, optional")
	flag.StringVar(&subDomain, "subdomain", "", "proxy subdomain, used for http")
	flag.BoolVar(&debug, "debug", false, "open debug mode")
	flag.IntVar(&proxyPort, "port", 0, "proxy server listen port, used for tcp")

	flag.Usage = func() {
		fmt.Printf("proxylocal version: %v\nUsage: %s [OPTIONS] <port | host:port>\n", VERSION, os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if !serverMode && len(flag.Args()) != 1 {
		flag.Usage()
		return
	}
	if !debug {
		log.SetOutputLevel(log.Linfo)
	} else {
		log.SetOutputLevel(log.Ldebug)
	}

	if serverMode {
		_, port, _ := net.SplitHostPort(serverAddr)
		if port == "" {
			port = "80"
		}
		addr := net.JoinHostPort("0.0.0.0", port)
		if domain == "" {
			domain = serverAddr
		}
		fmt.Println("proxylocal: server listen on", addr)
		ps := pxlocal.NewProxyServer(domain)
		log.Fatal(http.ListenAndServe(addr, ps))
	}

	proxyAddr = flag.Arg(0)
	if !regexp.MustCompile("^(http|https|tcp)://").MatchString(proxyAddr) {
		if _, err := strconv.Atoi(proxyAddr); err == nil { // only contain port
			proxyAddr = "localhost:" + proxyAddr
		} else {
			//proxyAddr += ":80"
		}
		proxyAddr = "http://" + proxyAddr
	}
	pURL, err := url.Parse(proxyAddr)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("proxy URL:", pURL)
	pxlocal.StartAgent(pURL, subDomain, serverAddr, proxyPort)
}
