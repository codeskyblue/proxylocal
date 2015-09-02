package pxlocal

import (
	"net"
	"sync"

	"github.com/qiniu/log"
)

//A proxy represents a pair of connections and their state
type ProxyStats struct {
	sentBytes     uint64
	receivedBytes uint64
	// laddr, raddr  *net.TCPAddr
}

type ProxyConn struct {
	sentBytes     uint64
	receivedBytes uint64
	lconn, rconn  net.Conn
	stats         *ProxyStats
}

func closeRead(c net.Conn) error {
	if x, ok := c.(interface {
		CloseRead() error
	}); ok {
		return x.CloseRead()
	} else {
		log.Println("force close", c)
		return c.Close()
	}
	// if tcpconn, ok := c.(*net.TCPConn); ok {
	// 	return tcpconn.CloseRead()
	// } else {
	// 	// return c.Close()
	// }
	// return nil
}

func closeWrite(c net.Conn) error {
	if x, ok := c.(interface {
		CloseWrite() error
	}); ok {
		return x.CloseWrite()
	} else {
		log.Println("force close", c)
		return c.Close()
	}
	// if tcpconn, ok := c.(*net.TCPConn); ok {
	// 	return tcpconn.CloseWrite()
	// } else {
	// 	log.Println("force close", c)
	// 	// return c.Close()
	// 	return nil
	// }
	// return nil
}

func (p *ProxyConn) start() {
	defer p.lconn.Close()
	defer p.rconn.Close()
	//connect to remote
	// rconn, err := net.DialTCP("tcp", nil, p.raddr)
	// if err != nil {
	// 	log.Printf("Remote connection failed: %s", err)
	// 	return
	// }
	// p.rconn = rconn
	// defer p.rconn.Close()

	// FIXME: may need to set a flag
	if tcpconn, ok := p.lconn.(*net.TCPConn); ok {
		tcpconn.SetNoDelay(true)
	}
	if tcpconn, ok := p.rconn.(*net.TCPConn); ok {
		tcpconn.SetNoDelay(true)
	}
	// p.lconn.SetNoDelay(true)
	// p.rconn.SetNoDelay(true)

	//display both ends
	// log.Printf("Opened %s >>> %s", p.lconn.RemoteAddr().String(), p.rconn.RemoteAddr().String())
	//bidirectional copy
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		ch1 := p.pipe(p.lconn, p.rconn)
		<-ch1
		closeRead(p.lconn)
		closeWrite(p.rconn)
		log.Println("close local -> remote")
		wg.Done()
	}()
	go func() {
		ch2 := p.pipe(p.rconn, p.lconn)
		<-ch2
		closeRead(p.rconn)
		closeWrite(p.lconn)
		log.Println("close remote -> local")
		wg.Done()
	}()
	wg.Wait()
	//wait for close...
	// log.Printf("Closed (%d bytes sent, %d bytes recieved)", p.sentBytes, p.receivedBytes)
}

func (p *ProxyConn) pipe(src, dst net.Conn) chan error {
	//data direction
	errch := make(chan error, 1)
	islocal := src == p.lconn

	//directional copy (64k buffer)
	buff := make([]byte, 0xffff)
	go func() {
		for {
			n, err := src.Read(buff)
			if err != nil {
				errch <- err
				return
			}
			b := buff[:n]

			//write out result
			n, err = dst.Write(b)
			if err != nil {
				errch <- err
				log.Printf("Write failed '%s'\n", err)
				return
			}
			log.Debug("pipe --> local:", islocal, "write:", n) //, string(b[:n]))
			if islocal {
				p.sentBytes += uint64(n)
				p.stats.sentBytes += uint64(n)
			} else {
				p.receivedBytes += uint64(n)
				p.stats.receivedBytes += uint64(n)
			}
		}
	}()
	return errch
}
