# FastHttpd

[![PkgGoDev](https://pkg.go.dev/badge/github.com/fasthttpd/fasthttpd)](https://pkg.go.dev/github.com/fasthttpd/fasthttpd)
[![Report Card](https://goreportcard.com/badge/github.com/fasthttpd/fasthttpd)](https://goreportcard.com/report/github.com/fasthttpd/fasthttpd)

FastHttpd is a lightweight http server using [valyala/fasthttp](https://github.com/valyala/fasthttp).

## Features

- Serve static files
- Simple routing
- Access logging
- Reverse proxy
- Customize headers
- Support TLS
- Virtual hosts
- YAML configuration

## Installation

### Go install

```sh
go install github.com/fasthttpd/fasthttpd/cmd/fasthttpd@latest
```

### Download binary

Download binary from [release](https://github.com/fasthttpd/fasthttpd/releases).

```sh
VERSION=0.5.1 GOOS=linux GOARCH=amd64; \
  curl -fsSL "https://github.com/fasthttpd/fasthttpd/releases/download/v${VERSION}/fasthttpd_${VERSION}_${GOOS}_${GOARCH}.tar.gz" | \
  tar xz fasthttpd && \
  sudo mv fasthttpd /usr/sbin
```

- GOOS supports `linux` `darwin` `windows`
- GOARCH supports `amd64` `arm64` `386`

### Homebrew

```sh
brew tap fasthttpd/fasthttpd
brew install fasthttpd
```

### Using yum or apt

Download deb or rpm from [release](https://github.com/fasthttpd/fasthttpd/releases), and then execute `apt install` or `yum install`. 

```sh
VERSION=0.5.1 ARCH=amd64; \
  curl -fsSL -O "https://github.com/fasthttpd/fasthttpd/releases/download/v${VERSION}/fasthttpd_${VERSION}_${ARCH}.deb"
sudo apt install "./fasthttpd_${VERSION}_${ARCH}.deb"
```

- Default configuration path is /etc/fasthttpd/config.yaml
- Default log directory is /var/log/fasthttpd
- FastHttpd is automatically started by systemd

### Docker

See [https://hub.docker.com/r/fasthttpd/fasthttpd](https://hub.docker.com/r/fasthttpd/fasthttpd)

```
docker run --rm -p 8080:80 fasthttpd/fasthttpd
```

Then you can hit http://localhost:8080 in your browser.

## Quick start

Usage

```sh
FastHttpd is a HTTP server using valyala/fasthttp.

Usage:
  fasthttpd [flags] [query] ([file...])

Flags:
  -e value
    	edit expression (eg. -e KEY=VALUE)
  -f string
    	configuration file
  -h	help for fasthttpd
  -v	print version
```

Examples

```sh
% fasthttpd -f examples/config.minimal.yaml
% fasthttpd -f examples/config.minimal.yaml -e accessLog.output=stdout
% fasthttpd -e root=./examples/public -e listen=0.0.0.0:8080
```

## Configuration

For more information, refer to [fasthttpd.org/configuration](https://fasthttpd.org/configuration).

The following is a minimal configuration built into fasthttpd.

```yaml
host: localhost
listen: ':8080'
root: ./public
log:
  output: stderr

handlers:
  'static':
    type: fs
    indexNames: [index.html]

routes:
  - path: /
    handler: static
```

The following is a configuration that uses most of the current FastHttpd features.

```yaml
host: localhost
# NOTE: Define listen addr. It is supported ipv6 `[::1]:8080`
listen: ':8080'
root: ./public

# Define fasthttp.Server settings.
server:
  name: fasthttpd
  readBufferSize: 4096
  writeBufferSize: 4096

log:
  output: logs/error.log
  # NOTE: Flags supports date|time|microseconds
  flags: [date, time]
  rotation:
    maxSize: 100

accessLog:
  output: logs/access.log
  format: '%h %l %u %t "%r" %>s %b'
  rotation:
    maxSize: 100
    maxBackups: 14
    maxAge: 28
    compress: true
    localTime: true
 
# Define custom error pages (x matches [0-9])
errorPages:
  '404': /err/404.html
  '5xx': /err/5xx.html

# Define named filters
filters:

  'auth':
    type: basicAuth
    users:
      # WARNING: It is unsafe to define plain secrets. It is recommended for development use only.
      - name: fast
        secret: httpd
    usersFile: ./users.yaml

  'cache':
    type: header
    response:
      set:
        'Cache-Control': 'private, max-age=3600'

# Define named handlers
handlers:

  'static':
    type: fs
    indexNames: [index.html]
    generateIndexPages: false
    compress: true
    compressRoot: ./compressed

  'static-overwrite':
    type: fs
    indexNames: [index.html]
    root: ./public-overwrite
  
  'hello':
    type: content
    headers:
      'Content-Type': 'text/plain; charset=utf-8'
    body: Hello FastHttpd
    conditions:
      - path: '/hello/world'
        body: Hello world
      - queryStringContains: 'time=morning'
        body: Good morning FastHttpd
      - percentage: 10
        body: 10% hit FastHttpd

# The routes are processed in sequence and interrupted when the status or the handler is specified.
routes:

  # Allows GET, POST, HEAD only.
  - methods: [PUT, DELETE, CONNECT, OPTIONS, TRACE, PATCH]
    status: 405
    statusMessage: 'Method not allowed'

  # Route to /index.html.
  - path: /
    match: equal
    handler: static

  # Redirect to external url with status code 302.
  - path: /redirect-external
    match: equal
    rewrite: http://example.com/
    status: 302

  # Redirect to internal uri with status code 302 and appendQueryString.
  # If "GET /redirect-internal?name=value" is requested then it redirect to "/internal?foo=bar&name=value"
  - path: /redirect-internal
    match: equal
    rewrite: /internal?foo=bar
    rewriteAppendQueryString: true
    status: 302
  
  # Route to static-overwrite resources using regexp.
  - path: .*\.(js|css|jpg|png|gif|ico)$
    match: regexp
    filters: [cache]
    methods: [GET, HEAD]
    handler: static-overwrite
    nextIfNotFound: true
  
  # Route to static resources using regexp.
  - path: .*\.(js|css|jpg|png|gif|ico)$
    match: regexp
    filters: [cache]
    methods: [GET, HEAD]
    handler: static

  # Rewrite the path and route to next (no handler and no status).
  - path: ^/view/(.+)
    match: regexp
    rewrite: /view?id=$1

  # Other requests are routed to hello with auth filter.
  - filters: [auth]
    handler: hello

routesCache:
  enable: true
  expire: 60000

---

host: localhost
listen: ':8443'

ssl:
  certFile: ./ssl/localhost.crt
  keyFile: ./ssl/localhost.key

handlers:
  'backend':
    type: proxy
    url: 'http://localhost:8080'

routes:
  - path: /
    handler: backend
```

## Override configuration using edit option

FastHttpd can override some of the values in config.yaml with the -e option via [jarxorg/tree](https://github.com/jarxorg/tree).

Customize content root

```sh
fasthttpd -f config.yaml -e root=/custom-root
```

Show access log and disable other log

```sh
fasthttpd -f config.yaml -e log.output="" -e accessLog.output=stdout
```

## RoutesCache

The following is a benchmark report of route. 
This report shows that caching is effective when routing makes heavy use of regular expressions.

```
% GOMAXPROCS=1 go test -bench=. -benchmem -memprofile=mem.prof -cpuprofile=cpu.prof ./pkg/route/... -benchtime=10s
goos: darwin
goarch: arm64
pkg: github.com/fasthttpd/fasthttpd/pkg/route
BenchmarkRoutes_Equal        	543322784	        22.05 ns/op	       0 B/op	       0 allocs/op
BenchmarkCachedRoutes_Equal  	141902754	        84.47 ns/op	       1 B/op	       1 allocs/op
BenchmarkRoutes_Prefix       	428678508	        27.95 ns/op	       0 B/op	       0 allocs/op
BenchmarkCachedRoutes_Prefix 	120594448	        99.57 ns/op	       1 B/op	       1 allocs/op
BenchmarkRoutes_Regexp       	34690477	       341.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkCachedRoutes_Regexp 	121977412	        98.47 ns/op	       1 B/op	       1 allocs/op
```

## TODO

- Support HTTP/3
- Benchmark reports

## Third-party library licenses

- [valyala/fasthttp](https://github.com/valyala/fasthttp)
- [natefinch/lumberjack](https://github.com/natefinch/lumberjack)
- [zehuamama/balancer](https://github.com/zehuamama/balancer)
- [jarxorg/tree](https://github.com/jarxorg/tree)
