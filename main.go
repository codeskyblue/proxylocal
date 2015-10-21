package main

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"

	"github.com/codeskyblue/proxylocal/pxlocal"
	"github.com/qiniu/log"
	"gopkg.in/alecthomas/kingpin.v2"
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
var localAddr string

func init() {
	var defaultServerAddr = os.Getenv("PXL_SERVER_ADDR")
	if defaultServerAddr == "" {
		defaultServerAddr = "proxylocal.xyz"
	}

	kingpin.Flag("debug", "Enable debug mode.").BoolVar(&cfg.Debug)

	// flag.BoolVar(&cfg.Debug, "debug", false, "open debug mode")
	// flag.BoolVar(&cfg.Server.Enable, "server", false, "run in server mode")
	// flag.StringVar(&cfg.Server.Addr, "server-addr", defaultServerAddr, "server address")
	// flag.StringVar(&cfg.Server.Domain, "server-domain", "", "proxy server domain name, optional")
	// flag.StringVar(&cfg.SubDomain, "subdomain", "", "proxy subdomain, used for http")
	// flag.StringVar(&cfg.Proto, "proto", "http", "default protocal, http or tcp")
	// flag.IntVar(&cfg.ProxyPort, "port", 0, "proxy server listen port, used for tcp")

	kingpin.Flag("proto", "Default protocol, http or tcp").Default("http").EnumVar(&cfg.Proto, "http", "tcp") // .StringVar(&cfg.Proto)
	kingpin.Flag("subdomain", "Proxy subdomain, used for http").StringVar(&cfg.SubDomain)
	kingpin.Flag("remote-port", "Proxy server listen port, only used in tcp").IntVar(&cfg.ProxyPort)
	kingpin.Flag("server", "Specify server address").OverrideDefaultFromEnvar("PXL_SERVER_ADDR").StringVar(&cfg.Server.Addr)

	kingpin.Flag("listen", "Run in server mode").Short('l').BoolVar(&cfg.Server.Enable)
	kingpin.Flag("domain", "Proxy server mode domain name, optional").StringVar(&cfg.Server.Domain)

	kingpin.Arg("local", "Local address").Required().StringVar(&localAddr)
}

func parseURL(addr string, defaultProto string) (u *url.URL, err error) {
	if !regexp.MustCompile("^(http|https|tcp)://").MatchString(addr) {
		if _, err := strconv.Atoi(addr); err == nil { // only contain port
			addr = "localhost:" + addr
		} else {
			_, _, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
		}
		addr = defaultProto + "://" + addr
	}
	return url.Parse(addr)
}

func main() {
	kingpin.Version(VERSION)
	kingpin.CommandLine.HelpFlag.Short('h')
	kingpin.Parse()

	if !cfg.Server.Enable && localAddr == "" {
		kingpin.Usage()
		return
	}
	if !cfg.Debug {
		log.SetOutputLevel(log.Linfo)
	} else {
		log.SetOutputLevel(log.Ldebug)
	}

	pURL, err := parseURL(localAddr, cfg.Proto)
	if err != nil {
		log.Fatal(err)
	}

	if cfg.Server.Enable {
		_, port, _ := net.SplitHostPort(pURL.Host)
		if port == "" {
			port = "80"
		}
		addr := net.JoinHostPort("0.0.0.0", port)
		if cfg.Server.Domain == "" {
			cfg.Server.Domain = "localhost" //cfg.Server.Addr
		}
		fmt.Printf("proxylocal: server listen on %v, domain is %v\n", addr, cfg.Server.Domain)
		ps := pxlocal.NewProxyServer(cfg.Server.Domain)
		log.Fatal(http.ListenAndServe(addr, ps))
	}

	// var localAddr = flag.Arg(0)
	// if !regexp.MustCompile("^(http|https|tcp)://").MatchString(localAddr) {
	// 	if _, err := strconv.Atoi(localAddr); err == nil { // only contain port
	// 		localAddr = "localhost:" + localAddr
	// 	} else {
	// 		//localAddr += ":80"
	// 	}
	// 	localAddr = cfg.Proto + "://" + localAddr
	// }

	// pURL, err := url.Parse(localAddr)
	fmt.Println("proxy URL:", pURL)
	pxlocal.StartAgent(pURL, cfg.SubDomain, cfg.Server.Addr, cfg.ProxyPort)
}
