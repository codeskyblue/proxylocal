package pxlocal

import (
	"errors"
	"fmt"
	"net"
)

type freePort struct {
	minPort int // >=
	maxPort int // <
	next    int
	count   int
}

func newFreePort(min, max int) *freePort {
	if max > 65535 {
		max = 65535
	}
	return &freePort{
		next:    min,
		minPort: min,
		maxPort: max,
		count:   max - min,
	}
}

func (this *freePort) ListenTCP() (taddr *net.TCPAddr, lis *net.TCPListener, err error) {
	next := this.next
	for i := 0; i < this.count; i++ {
		next = (this.next+i-this.minPort)%this.count + this.minPort
		taddr, _ := net.ResolveTCPAddr("tcp", fmt.Sprintf(":%d", next))
		lis, err := net.ListenTCP("tcp", taddr)
		if err == nil {
			this.next = next + 1
			return taddr, lis, nil
		}
	}
	return nil, nil, errors.New("Not free port")
}
