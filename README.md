# FastHttpd

FastHttpd is a HTTP server using [valyala/fasthttp](https://github.com/valyala/fasthttp).

## Install

```
VERSION=0.0.2 GOOS=Darwin GOARCH=arm64; curl -fsSL "https://github.com/fasthttpd/fasthttpd/releases/download/v${VERSION}/fasthttpd_${VERSION}_${GOOS}_${GOARCH}.tar.gz" | tar xz fasthttpd && mv fasthttpd /usr/local/bin
```

## Quick start

```sh
% fasthttpd -f config.yaml
% fasthttpd -e root=./public -e listen=0.0.0.0:8800
```

## Configuration

The following is a configuration that is minimal.

```yaml
root: ./public

handlers:
  static:
    type: fs
    indexNames: ['index.html']

routes:
  - path: /
    handler: static
```

The following is a configuration that uses most of the current FastHttpd features.

```yaml
host: localhost
# NOTE: Define listen addr. It is supported ipv6 `[::1]:8800`
listen: ':8800'
root: ./public
log:
  output: stderr
  # NOTE: Flags supports date|time|microseconds
  flags: ['date', 'time']
accessLog:
  output: stdout
  format: '%h %l %u %t "%r" %>s %b'

# Define fasthttp.Server settings.
server:
  name: fasthttpd
  readBufferSize: 4096
  writeBufferSize: 4096
 
# Define custom error pages (x matches [0-9])
errorPages:
  '404': /err/404.html
  '5xx': /err/5xx.html

# Define named filters
filters:

  auth:
    type: basicAuth
    users:
      # WARNING: It is unsafe to define plain secrets. It is recommended for development use only.
      - name: fast
        secret: httpd
    usersFile: ./users.yaml

# Define named handlers
handlers:

  static:
    type: fs
    indexNames: ['index.html']
    generateIndexPages: false
    compress: true
  
  backend:
    type: proxy
    url: 'http://localhost:9000'

# Define routes
routes:

  # Allows GET, POST, HEAD only.
  - methods: [PUT, DELETE, CONNECT, OPTIONS, TRACE, PATCH]
    status:
      code: 405
      message: Method not allowed

  # Route to /index.html.
  - path: /
    match: equal
    handler: static

  # Redirect to external url with status code 302.
  - path: /redirect-external
    match: equal
    rewrite:
      uri: http://example.com/
    status:
      code: 302

  # Redirect to internal uri with status code 302 and appendQueryString.
  # If "GET /redirect-internal?name=value" is requested then it redirect to "/internal?foo=bar&name=value"
  - path: /redirect-internal
    match: equal
    rewrite:
      uri: /internal?foo=bar
      appendQueryString: true
    status:
      code: 302
  
  # Route to static resources using regexp.
  - path: .*\.(js|css|jpg|png|gif|ico)$
    match: regexp
    methods: [GET, HEAD]
    handler: static

  # Rewrite the path and route to next (no handler and no status).
  - path: ^/view/(.+)
    match: regexp
    rewrite:
      uri: /view?id=$1

  # Other requests are routed to backend with auth filter.
  - filters: ['auth']
    handler: backend

routesCache:
  enable: true
  expire: 60000
```


## TODO

- Daemonize
- Custom headers
- Rotate logs
- Support https
