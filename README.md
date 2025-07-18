# proxylocal
[![GoDoc](https://godoc.org/github.com/codeskyblue/proxylocal/pxlocal?status.svg)](https://godoc.org/github.com/codeskyblue/proxylocal/pxlocal)

Proxy local service to public.

> I want to expose a local server behide a NAT or firewall to the internet.

There are some similar service.

* <http://localtunnel.me/> Write in nodejs. Very good one.
* <https://pagekite.net/> Write in python.
* <https://ngrok.com/> Blocked by GFW.
* <https://forwardhq.com/> Need pay price to get service.
* <http://www.tunnel.mobi/> Server seems down. Use ngrok, VPS in china.

Binary can be download from [gorelease](http://gorelease.herokuapp.com/codeskyblue/proxylocal)

Very suggest to compile use `go1.4`. I donot know why, but use `go1.5`, the proxylocal got a very very bad performance.

At the beginning this is just for study how to proxy local server to public network. Now it can be stable use.

这个东西目前看来确实是个很不错的东西，可以调试微信，可以把自家路由器的东西放到外网。还可以通过它的tcp转发功能，远程调试家中的树莓派。用途多多

不过服务器最好自己搭. 如果希望贡献出来你的server可以发起个Issue.

## Installation
```
go get -v github.com/codeskyblue/proxylocal
```

## Usage
Run server in a public network, listen in port 8080 (Assuming your ip is 122.2.2.1)

	proxylocal --listen 8080

Assume you are running your local tcp-server on port 5037. To make it publicly available run:

	proxylocal --server 122.2.2.1:8080 --proto tcp 5037

If this is a web server, only need to update `--proto`
	
	proxylocal --server 122.2.2.1:8080 --proto http 5037

	# expects output
	proxy URL: http://localhost:5037
	Recv Message: Local server is now publicly available via:
	http://wn8yn.t.localhost

## Hooks
The functions of hooks are limited.

The hook system is very familar with git hook. When a new proxy request comes to the server. Server will execute some script.

Now I put all the hook script in hooks dir. 

There are examples you found in [hooks](hooks)

## Use as a library
```go
package main

import "github.com/codeskyblue/proxylocal/pxlocal"

func main(){
	client := pxlocal.NewClient("10.0.1.1:4000")
	px, err := client.StartProxy(pxlocal.ProxyOptions{
		Proto:      pxlocal.TCP,
		LocalAddr:  "192.168.0.1:7000",
		ListenPort: 40000, // public port
	})
	if err != nil {
		log.Fatal(err)
	}
	// px.RemoteAddr()
	err = px.Wait()
	log.Fatal(err)
}
```
### Environment
Server address default from env-var `PXL_SERVER_ADDR`

## LICENSE
[MIT LICENSE](LICENSE)
