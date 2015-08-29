# proxylocal

Just for study how to proxy local server to public network.

## Install
```
go get -v github.com/codeskyblue/proxylocal
```

## Usage
Now the usage is complex, I'll modify it later.

**Server**

```
proxylocal -s -addr :5000
```

**Agent** which in private addr.

ex: your server is in proxy.local.com
```
proxylocal -addr proxy.local.com:5000 -proxy-port 7777 -proxy localhost:8080
```

Maybe you need to run a simple file server to test, if 8080 is not listen.

```
python -mSimpleHTTPServer 8080
```

Check `proxy.local.com:7777`

## LICENSE
MIT(LICENSE)
