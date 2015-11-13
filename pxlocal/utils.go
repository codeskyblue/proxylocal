package pxlocal

import (
	"math/rand"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

var uniqMap = make(map[string]bool)
var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz1234567890") //ABCDEFGHIJKLMNOPQRSTUVWXYZ")

func uniqName(n int) string {
	for {
		b := make([]rune, n)
		for i := range b {
			b[i] = letterRunes[rand.Intn(len(letterRunes))]
		}
		s := string(b)

		if uniqMap[s] {
			continue
		}
		uniqMap[s] = true
		return s
	}
}

type URLOpts struct {
	DefaultScheme string
	DefaultHost   string
	DefaultPort   int
}

func ParseURL(s string, opts ...URLOpts) (u *url.URL, err error) {
	opt := URLOpts{
		DefaultScheme: "http",
		DefaultHost:   "localhost",
		DefaultPort:   80,
	}
	if len(opts) > 0 {
		opt = opts[0]
		if opt.DefaultHost == "" {
			opt.DefaultHost = "localhost"
		}
		if opt.DefaultPort == 0 {
			opt.DefaultPort = 80
		}
		if opt.DefaultScheme == "" {
			opt.DefaultScheme = "http"
		}
	}

	if !regexp.MustCompile(`^(\w+)://`).MatchString(s) {
		if _, er := strconv.Atoi(s); er == nil { // only contain port
			s = opt.DefaultHost + ":" + s
		}
		if _, _, er := net.SplitHostPort(s); er != nil {
			s = s + ":" + strconv.Itoa(opt.DefaultPort)
		}
		s = opt.DefaultScheme + "://" + s
	}
	u, err = url.Parse(s)
	if err != nil {
		return
	}
	// must contains port
	if _, _, er := net.SplitHostPort(u.Host); er != nil {
		u.Host += ":" + strconv.Itoa(opt.DefaultPort)
	}
	return
}
