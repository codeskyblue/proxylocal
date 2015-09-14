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

type GlobalConfig struct {
	Server struct {
		Enable bool
		Addr   string
		Domain string
	}

	Proto     string
	ProxyPort int
	SubDomain string
	Debug     bool
}

var cfg GlobalConfig

func init() {
	var defaultServerAddr = os.Getenv("PXL_SERVER_ADDR")
	if defaultServerAddr == "" {
		defaultServerAddr = "proxylocal.xyz"
	}
	flag.BoolVar(&cfg.Debug, "debug", false, "open debug mode")
	flag.BoolVar(&cfg.Server.Enable, "server", false, "run in server mode")
	flag.StringVar(&cfg.Server.Addr, "server-addr", defaultServerAddr, "server address")
	flag.StringVar(&cfg.Server.Domain, "server-domain", "", "proxy server domain name, optional")
	flag.StringVar(&cfg.SubDomain, "subdomain", "", "proxy subdomain, used for http")
	flag.StringVar(&cfg.Proto, "proto", "http", "default protocal, http or tcp")
	flag.IntVar(&cfg.ProxyPort, "port", 0, "proxy server listen port, used for tcp")

	flag.Usage = func() {
		fmt.Printf("proxylocal version: %v\nUsage: %s [OPTIONS] <port | host:port>\n", VERSION, os.Args[0])
		flag.PrintDefaults()
	}
}

func main() {
	flag.Parse()
	if !cfg.Server.Enable && len(flag.Args()) != 1 {
		flag.Usage()
		return
	}
	if !cfg.Debug {
		log.SetOutputLevel(log.Linfo)
	} else {
		log.SetOutputLevel(log.Ldebug)
	}

	if cfg.Server.Enable {
		_, port, _ := net.SplitHostPort(cfg.Server.Addr)
		if port == "" {
			port = "80"
		}
		addr := net.JoinHostPort("0.0.0.0", port)
		if cfg.Server.Domain == "" {
			cfg.Server.Domain = cfg.Server.Addr
		}
		fmt.Println("proxylocal: server listen on", addr)
		ps := pxlocal.NewProxyServer(cfg.Server.Domain)
		log.Fatal(http.ListenAndServe(addr, ps))
	}

	var proxyAddr = flag.Arg(0)
	if !regexp.MustCompile("^(http|https|tcp)://").MatchString(proxyAddr) {
		if _, err := strconv.Atoi(proxyAddr); err == nil { // only contain port
			proxyAddr = "localhost:" + proxyAddr
		} else {
			//proxyAddr += ":80"
		}
		proxyAddr = cfg.Proto + "://" + proxyAddr
	}
	pURL, err := url.Parse(proxyAddr)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("proxy URL:", pURL)
	pxlocal.StartAgent(pURL, cfg.SubDomain, cfg.Server.Addr, cfg.ProxyPort)
}
