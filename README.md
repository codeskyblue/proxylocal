# proxylocal
[![gorelease](https://dn-gorelease.qbox.me/gorelease-download-blue.svg)](http://gorelease.herokuapp.com/dn-gobuild5.qbox.me/proxylocal/master)

Just for study how to proxy local server to public network.

反向代理服务，使内网的服务能在在外网被访问到。

目前服务是放在了 <http://proxylocal.xyz:8080>, 当前只是试验阶段。

这个东西目前看来确实是个很不错的东西，可以调试微信，可以把自家路由器的东西放到外网。还可以通过它的tcp转发功能，远程调试家中的树莓派。用途多多

然而运行proxylocal必须要一台外网服务器。谁都知道维护外网服务器是需要成本的，尤其是访问人数多的时候。

虽然我也不用靠它来给我盈利，但是免费的服务总是很难长久的。

所以我有了这个想法, 将用户分为高级用户和普通用户。

高级用户可以享受自定义的二级域名。比如 `myname.proxylocal.xyz`,而普通用户则可以正常使用，只不过是二级域名是随机生成的。

成为高级用户有两种途径

1. 付费
2. 为该项目做贡献（比如提交个pr, 或者为该项目做了个文档)

真希望这个项目能够坚挺的活下去。现在并没有开始收费，有什么建议可以提出来，提issue也是可以的。


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
$ proxylocal tcp://localhost:5037
Recv Message: Local tcp conn is now publicly available via:
proxylocal.xyz:13000
```

### 环境变量
server address地址 `PXL_SERVER_ADDR`

## 没想通的事情

`proxylocal www.baidu.com` 为什么不可以，而 `proxylocal localhost:8000` 却没问题

## LICENSE
MIT(LICENSE)
