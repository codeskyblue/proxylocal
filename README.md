# proxylocal

Just for study how to proxy local server to public network.


## Installation

ProxyLocal is a tool that runs on the command line.

You can combine from source code

```
go get -v github.com/codeskyblue/proxylocal
```

Or [download](https://github.com/codeskyblue/proxylocal/releases) according to your platform.

<https://github.com/codeskyblue/proxylocal/releases>

## Usage

Assume you are running your local web-server on port 3000. To make it publicly available run:

```
$ proxylocal 3000
Local server on port 3000 is now publicly available via:
http://fp9k.t.proxylocal.com/
```

Now you can open this link in your favorite browser and request will be proxied to your local web-server.

Also you can specify preferred host you want to use, e.g.:

```
$ proxylocal -subdomain testhost 3000
Local server on port 3000 is now publicly available via:
http://testhost.proxylocal.com/
```

## LICENSE
MIT(LICENSE)
