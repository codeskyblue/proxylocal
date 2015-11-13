package pxlocal

import (
	"net"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestParseURL(t *testing.T) {
	opt := URLOpts{
		DefaultScheme: "test",
		DefaultHost:   "local-test",
		DefaultPort:   443,
	}
	validAddrs := []string{
		"80",
		"local-test",
		"test://local-test",
	}
	for _, addr := range validAddrs {
		Convey("Should parse "+addr, t, func() {
			u, err := ParseURL(addr, opt)
			So(err, ShouldBeNil)
			So(u.Scheme, ShouldEqual, "test")
			host, _, err := net.SplitHostPort(u.Host)
			So(err, ShouldBeNil)
			So(host, ShouldEqual, "local-test")
		})
	}
}
