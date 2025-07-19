package main

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/codeskyblue/proxylocal/pxlocal"
	"github.com/gobuild/log"
)

type GlobalConfig struct {
	Server struct {
		Enable bool
		Addr   string
		Domain string
	}

	Proto     string
	Data      string
	ProxyPort int
	SubDomain string
	Debug     bool
}

var cfg GlobalConfig
var localAddr string

func init() {
	kingpin.Flag("debug", "Enable debug mode.").BoolVar(&cfg.Debug)

	kingpin.Flag("proto", "Default protocol, http or tcp").Default("http").Short('p').EnumVar(&cfg.Proto, "http", "tcp") // .StringVar(&cfg.Proto)
	kingpin.Flag("subdomain", "Proxy subdomain, used for http").StringVar(&cfg.SubDomain)
	kingpin.Flag("remote-port", "Proxy server listen port, only used in tcp").IntVar(&cfg.ProxyPort)
	kingpin.Flag("data", "Data send to server, can be anything").StringVar(&cfg.Data)
	kingpin.Flag("server", "Specify server address").Short('s').OverrideDefaultFromEnvar("PXL_SERVER_ADDR").Default("https://your-proxylocal-domain.com").StringVar(&cfg.Server.Addr)

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

func extractHostname(addr string) string {
	if strings.Contains(addr, "://") {
		addr = strings.SplitN(addr, "://", 2)[1]
	}
	return addr
}

func main() {
	kingpin.Version(VERSION)
	kingpin.CommandLine.VersionFlag.Short('v')
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

	pURL, err := pxlocal.ParseURL(localAddr, pxlocal.URLOpts{DefaultScheme: cfg.Proto})
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
			cfg.Server.Domain = extractHostname(cfg.Server.Addr)
		}
		fmt.Printf("proxylocal: server listen on %v, domain is %v\n", addr, cfg.Server.Domain)
		ps := pxlocal.NewProxyServer(cfg.Server.Domain)
		log.Fatal(http.ListenAndServe(addr, ps))
	}

	client := pxlocal.NewClient(cfg.Server.Addr)
	fmt.Println("proxy server:", client.URL())
	fmt.Println("local server:", pURL)
	for {
		px, err := client.RunProxy(pxlocal.ProxyOptions{
			Proto:      pxlocal.ProxyProtocol(cfg.Proto),
			Subdomain:  cfg.SubDomain,
			LocalAddr:  localAddr,
			ListenPort: cfg.ProxyPort,
		})
		if err == nil {
			err = px.Wait()
		} else {
			log.Warnf("RunProxy error: %v", err)
			fmt.Println("Reconnect after 5 seconds ...")
			time.Sleep(time.Duration(5) * time.Second)
		}
	}
}
