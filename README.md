# proxylocal
<del>[![gorelease](https://dn-gorelease.qbox.me/gorelease-download-blue.svg)](http://gorelease.herokuapp.com/codeskyblue/proxylocal)</del>

Very suggest to compile gorelease use `go1.4`. I donot know why, but use `go1.5`, the proxylocal got a very very bad performance.

Proxy local service to public.

> I want to expose a local server behide a NAT or firewall to the internet.

There are some similar service.

* <http://localtunnel.me/> Write in nodejs. Very good one.
* <https://ngrok.com/> Blocked by GFW.
* <https://forwardhq.com/> Need pay price to get service.
* <http://www.tunnel.mobi/> Server seems down. Use ngrok, VPS in china.

At the beginning this is just for study how to proxy local server to public network. Now it can be stable use.

这个东西目前看来确实是个很不错的东西，可以调试微信，可以把自家路由器的东西放到外网。还可以通过它的tcp转发功能，远程调试家中的树莓派。用途多多

不过服务器最好自己搭. 如果希望贡献出来你的server可以发起个Issue.

## Installation
Build from source code, (use [godep](https://github.com/tools/godep) is your build got some error)

```
go get -v github.com/codeskyblue/proxylocal
```

<del>Or [download](https://github.com/codeskyblue/proxylocal/releases) according to your platform.</del>

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

### Environment
Server address default from env-var `PXL_SERVER_ADDR`

## LICENSE
MIT(LICENSE)
