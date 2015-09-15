# proxylocal
[![gorelease](https://dn-gorelease.qbox.me/gorelease-download-blue.svg)](http://gorelease.herokuapp.com/codeskyblue/proxylocal)

Just for study how to proxy local server to public network.

反向代理服务，使内网的服务能在在外网被访问到。

目前服务是放在了 <http://proxylocal.xyz:8080>, 当前只是试验阶段。

这个东西目前看来确实是个很不错的东西，可以调试微信，可以把自家路由器的东西放到外网。还可以通过它的tcp转发功能，远程调试家中的树莓派。用途多多

## Installation

proxylocal is a tool that runs on the command line.

Build from source code

```
go get -v github.com/codeskyblue/proxylocal
```

<del>Or [download](https://github.com/codeskyblue/proxylocal/releases) according to your platform.</del>

## Usage

Assume you are running your local web-server on port 3000. To make it publicly available run:

```
$ proxylocal 3000
Recv Message: Local server is now publicly available via:
http://v61ny.t.proxylocal.xyz:8080
```

Now you can open this link in your favorite browser and request will be proxied to your local web-server.

Also you can specify preferred host you want to use, e.g.:

```
$ proxylocal -subdomain testhost 3000
Recv Message: Local server is now publicly available via:
http://testhost.proxylocal.com/
```

### TCP的代理方法

```
$ proxylocal -proto tcp 5037
Recv Message: Local tcp conn is now publicly available via:
proxylocal.xyz:13000
```

### 环境变量
server address地址 `PXL_SERVER_ADDR`

### Alternative
There are some good website which offer proxy service.

* <http://www.tunnel.mobi/> Use ngrok, VPS in china.
* <https://forwardhq.com/> Need pay price to get service.

## LICENSE
MIT(LICENSE)
