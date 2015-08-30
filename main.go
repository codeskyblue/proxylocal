package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/codeskyblue/proxylocal/pxlocal"
)

func main() {
	var serverMode bool
	var serverAddr string
	var proxyPort int
	var proxyAddr string
	var subDomain string
	var protocal string
	var domain string
	var debug bool
	flag.BoolVar(&serverMode, "server", false, "run in server mode")
	flag.StringVar(&serverAddr, "server-addr", "proxylocal.xyz:8080", "server address")
	flag.StringVar(&domain, "server-domain", "", "proxy server domain name, optional")
	flag.StringVar(&protocal, "protocal", "http", "tcp or http")
	flag.StringVar(&subDomain, "subdomain", "", "proxy subdomain, used for http")
	flag.BoolVar(&debug, "debug", false, "open debug mode")
	flag.IntVar(&proxyPort, "port", 0, "proxy server listen port, used for tcp")

	flag.Usage = func() {
		fmt.Printf("Usage: %s [OPTIONS] <port | host:port>\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if !serverMode && len(flag.Args()) != 1 {
		flag.Usage()
		return
	}

	proxyAddr = flag.Arg(0)
	if !strings.Contains(proxyAddr, ":") {
		if _, err := strconv.Atoi(proxyAddr); err == nil { // only contain port
			proxyAddr = "localhost:" + proxyAddr
		} else { // only contain host
			proxyAddr = proxyAddr + ":80"
		}
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

	pxlocal.StartAgent(debug, protocal, subDomain, proxyAddr, serverAddr, proxyPort)
}
