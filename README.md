# FastHttpd

[![PkgGoDev](https://pkg.go.dev/badge/github.com/fasthttpd/fasthttpd)](https://pkg.go.dev/github.com/fasthttpd/fasthttpd)
[![Report Card](https://goreportcard.com/badge/github.com/fasthttpd/fasthttpd)](https://goreportcard.com/report/github.com/fasthttpd/fasthttpd)

FastHttpd is a lightweight http server using [valyala/fasthttp](https://github.com/valyala/fasthttp).

> FastHttpd and fasthttp are versioned independently. FastHttpd **v0.7.0** is built against fasthttp **v1.70.0**.

## Features

- Serve static files
- Simple routing
- Access logging (NCSA-style, JSON or LTSV, allocation-free hot path)
- Reverse proxy
- Customize headers
- Support TLS (HTTPS/SSL)
- Automatic TLS certificates via Let's Encrypt (autocert / ACME)
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
VERSION=0.7.0 GOOS=linux GOARCH=amd64; \
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
VERSION=0.7.0 ARCH=amd64; \
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

For the full reference, see [docs/configuration.md](docs/configuration.md).
For enabling Let's Encrypt (autocert), see [examples/config.autocert.yaml](examples/config.autocert.yaml).

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

For a fuller configuration that exercises most of FastHttpd's features, see [docs/configuration.md](docs/configuration.md).

## Override configuration using edit option

FastHttpd can override some of the values in config.yaml with the -e option via [mojatter/tree](https://github.com/mojatter/tree).

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

Since v0.6.0 the cached route path is fully allocation-free, thanks to the
`maphash`-based `CacheKeyBuilder` used for cache key construction.

```
% GOMAXPROCS=1 go test -bench=. -benchmem ./pkg/route/... -benchtime=10s
goos: darwin
goarch: arm64
pkg: github.com/fasthttpd/fasthttpd/pkg/route
cpu: Apple M4
BenchmarkRoutes_Equal        	920876146	        13.20 ns/op	       0 B/op	       0 allocs/op
BenchmarkCachedRoutes_Equal  	135314598	        89.03 ns/op	       0 B/op	       0 allocs/op
BenchmarkRoutes_Prefix       	746245645	        16.05 ns/op	       0 B/op	       0 allocs/op
BenchmarkCachedRoutes_Prefix 	134016280	        89.55 ns/op	       0 B/op	       0 allocs/op
BenchmarkRoutes_Regexp       	75967437	       158.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkCachedRoutes_Regexp 	133490258	        89.56 ns/op	       0 B/op	       0 allocs/op
```

## Access log

FastHttpd writes access logs through a `bufio.Writer` + `sync.Pool` pipeline that keeps the formatting hot path allocation-free for the classic NCSA format, the JSON preset and the LTSV preset.

See [docs/access-log.md](docs/access-log.md) for format options, JSON / LTSV schema and field-level notes.

### Benchmark

```
% GOMAXPROCS=10 go test -bench='BenchmarkAccessLog_(Common|Combined|JSON|LTSV)$' -benchmem -benchtime=3s ./pkg/logger/accesslog/
goos: darwin
goarch: arm64
pkg: github.com/fasthttpd/fasthttpd/pkg/logger/accesslog
cpu: Apple M4
BenchmarkAccessLog_Common-10      17620472          187.6 ns/op       0 B/op       0 allocs/op
BenchmarkAccessLog_Combined-10    17296656          209.6 ns/op       0 B/op       0 allocs/op
BenchmarkAccessLog_JSON-10        14670214          243.3 ns/op       0 B/op       0 allocs/op
BenchmarkAccessLog_LTSV-10        17862510          201.7 ns/op       0 B/op       0 allocs/op
```

## Third-party library licenses

- [valyala/fasthttp](https://github.com/valyala/fasthttp)
- [natefinch/lumberjack](https://github.com/natefinch/lumberjack)
- [zehuamama/balancer](https://github.com/zehuamama/balancer)
- [mojatter/tree](https://github.com/mojatter/tree)
