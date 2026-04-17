# Configuration

## Built-in configuration

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

## Custom configuration

The following configuration exercises most of the current FastHttpd features.

<details>
<summary>Expand full configuration</summary>

```yaml
host: localhost
# NOTE: Define listen addr. IPv6 such as `[::1]:8080` is supported.
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
      # WARNING: Defining plain secrets is unsafe. For development use only.
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

  'expvar':
    type: expvar

  'hello':
    type: content
    headers:
      'Content-Type': 'text/plain; charset=utf-8'
    body: |
      Hello FastHttpd

# Routes are processed in sequence and interrupted when `status` or `handler` is specified.
routes:

  # Allow GET, POST, HEAD only.
  - methods: [PUT, DELETE, CONNECT, OPTIONS, TRACE, PATCH]
    status: 405
    statusMessage: 'Method not allowed'

  # Route to /index.html.
  - path: /
    match: equal
    handler: static

  # Route to expvar handler.
  - path: /expvar
    match: equal
    handler: expvar

  # Redirect to external URL with status code 302.
  - path: /redirect-external
    match: equal
    rewrite: http://example.com/
    status: 302

  # Redirect to internal URI with status code 302 and appendQueryString.
  # If "GET /redirect-internal?name=value" is requested, it redirects to "/internal?foo=bar&name=value"
  - path: /redirect-internal
    match: equal
    rewrite: /internal?foo=bar
    rewriteAppendQueryString: true
    status: 302

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

---

include: conf.d/*.yaml
```

</details>

## Host

```yaml
host: localhost
```

## Listen

Listen represents the IP address and port.

```yaml
listen: ':8080'
```

## Root

Path to the root directory to serve files from. `./` indicates the directory where `config.yaml` is located.

```yaml
root: ./public
```

## Server

Server represents settings for `fasthttp.Server`.

```yaml
server:
  name: fasthttpd
  readBufferSize: 4096
  writeBufferSize: 4096
  readTimeout: 60s
  writeTimeout: 60s
```

| Key | Description |
| --- | ----------- |
| `name` | Server name sent in response headers. |
| `readBufferSize` | Per-connection buffer size for reading requests. This also limits the maximum header size. Increase this buffer if clients send multi-KB request URIs or headers (for example, BIG cookies). Uses fasthttp's default if not set. |
| `writeBufferSize` | Per-connection buffer size for writing responses. Uses fasthttp's default if not set. |
| `readTimeout` | Maximum time allowed to read the full request including the body. The read deadline is reset when the connection opens, or for keep-alive connections after the first byte has been read. Unlimited by default. Valid units: `ns`, `us` (or `µs`), `ms`, `s`, `m`, `h`. |
| `writeTimeout` | Maximum time before timing out a response write. Reset after the request handler returns. Unlimited by default. Same unit rules as `readTimeout`. |

Other string, bool and numeric fields can also be set. See [fasthttp/server.go](https://github.com/valyala/fasthttp/blob/master/server.go) for details.

## Log

Log represents settings for logging.

```yaml
log:
  output: logs/error.log
  # NOTE: Flags supports date|time|microseconds
  flags: [date, time]
  rotation:
    maxSize: 100
    maxBackups: 14
    maxAge: 28
    compress: true
    localTime: true
```

| Key | Description |
| --- | ----------- |
| `output` | Output file path. `stdout` and `stderr` are special strings that indicate standard output and standard error. |
| `flags` | For example, `flags: [date, time]` produces `2009/01/23 01:23:23 message`. |
| `rotation.maxSize` | Maximum size in megabytes before the log file is rotated. Defaults to 100 MB. |
| `rotation.maxBackups` | Maximum number of rotated files to retain. The default is to keep all rotated files. |
| `rotation.maxAge` | Maximum number of days to retain rotated files based on the timestamp in their filename. A "day" is defined as 24 hours. The default is not to remove files based on age. |
| `rotation.compress` | Compress rotated log files with gzip. The default is no compression. |
| `rotation.localTime` | Use the machine's local time for timestamps in backup filenames. The default is UTC. |

The rotation is based on [natefinch/lumberjack](https://github.com/natefinch/lumberjack).

## AccessLog

AccessLog represents settings for request-level access logging.

```yaml
accessLog:
  output: logs/access.log
  format: '%h %l %u %t "%r" %>s %b'
  rotation:
    maxSize: 100
    maxBackups: 14
    maxAge: 28
    compress: true
    localTime: true
```

| Key | Description |
| --- | ----------- |
| `output` | Output file path. `stdout` and `stderr` are special strings for standard output and standard error. |
| `format` | Apache-style format string, or `json` / `ltsv` to select a structured preset. See [Apache Custom Log Formats](https://httpd.apache.org/docs/2.4/en/mod/mod_log_config.html). |
| `bufferSize` | Write-buffer size used by the background writer (bytes). |
| `flushInterval` | Maximum time the buffer may sit unflushed (milliseconds). |
| `rotation.*` | Same fields as `log.rotation`. |

The JSON and LTSV presets emit a fixed schema per line; see [docs/access-log.md](access-log.md) for the full field list and samples.

## ErrorPages

```yaml
# Define custom error pages (x matches [0-9])
errorPages:
  '404': /err/404.html
  '5xx': /err/5xx.html
```

| Key | Description |
| --- | ----------- |
| `root` | (Optional) Override the top-level `root` for error-page lookup. |
| `(http status)` | Path to the custom error page. The status key may contain `x` as a wildcard digit (e.g. `5xx`, `40x`). |

## Filters

Named filters can be declared under `filters`.

```yaml
filters:

  'auth':
    type: basicAuth
    users:
      # WARNING: Defining plain secrets is unsafe. For development use only.
      - name: fast
        secret: httpd
    usersFile: ./users.yaml

  'cache':
    type: header
    response:
      set:
        'Cache-Control': 'private, max-age=3600'
```

The following filter types are supported.

- `basicAuth` — HTTP Basic access authentication.
- `header` — customize request and response headers.

### BasicAuth

BasicAuth represents settings for HTTP Basic access authentication.

```yaml
filters:
  'auth':
    type: basicAuth
    users:
      # WARNING: Defining plain secrets is unsafe. For development use only.
      - name: fast
        secret: httpd
    usersFile: ./users.yaml
```

| Key | Description |
| --- | ----------- |
| `users[].name` | User name. |
| `users[].secret` | Plain secret. |
| `usersFile` | Path to a users file. See [testdata/users.yaml](https://github.com/fasthttpd/fasthttpd/blob/main/pkg/config/testdata/users.yaml). |

### Header

Header represents settings for customizing request and response headers.

```yaml
filters:
  'cache':
    type: header
    response:
      set:
        'Cache-Control': 'private, max-age=3600'
```

| Key | Description |
| --- | ----------- |
| `request.set` | Header-value mapping. Existing headers are overwritten. |
| `request.add` | Header-value mapping. Appended to existing values. |
| `request.del` | List of header names to delete. |
| `response.set` | Header-value mapping. Existing headers are overwritten. |
| `response.add` | Header-value mapping. Appended to existing values. |
| `response.del` | List of header names to delete. |

## Handlers

Named handlers can be declared under `handlers`.

```yaml
handlers:

  'static':
    type: fs
    indexNames: [index.html]
    generateIndexPages: false
    compress: true

  'hello':
    type: content
    headers:
      'Content-Type': 'text/plain; charset=utf-8'
    body: |
      Hello FastHttpd

  'backend':
    type: proxy
    url: http://localhost:9000/
```

The following handler types are supported.

- `fs` — serve static files from the local filesystem.
- `content` — serve in-memory content.
- `proxy` — reverse-proxy to one or more backends with a configurable algorithm.
- `balancer` — deprecated alias of `proxy`.

### FS

FS serves static files from the local filesystem.

```yaml
handlers:
  'static':
    type: fs
    indexNames: [index.html]
    generateIndexPages: false
    compress: true
```

| Key | Description |
| --- | ----------- |
| `root` | Path to the root directory. If omitted, the top-level `root` is used. |
| `indexNames` | List of index file names to try when a directory is requested. |
| `generateIndexPages` | Auto-generate index pages for directories that do not match `indexNames`. |
| `compress` | Transparently compress responses. |

Other string, bool and numeric fields can also be set. See [fasthttp/fs.go](https://github.com/valyala/fasthttp/blob/master/fs.go) for details.

### Content

Content serves in-memory content.

```yaml
handlers:
  'hello':
    type: content
    headers:
      'Content-Type': 'text/plain; charset=utf-8'
    body: |
      Hello FastHttpd
```

| Key | Description |
| --- | ----------- |
| `headers` | Key-value mapping or `Key: Value` list. |
| `body` | Content body. |

### Proxy

Proxy reverse-proxies to one or more backends.

```yaml
handlers:
  'single':
    type: proxy
    url: 'http://localhost:8080'

  'pool':
    type: proxy
    urls:
      - http://localhost:9000/
      - http://localhost:9001/
      - http://localhost:9002/
    algorithm: round-robin
    healthCheckInterval: 5
```

| Key | Description |
| --- | ----------- |
| `url` | Single backend URL (used when `urls` is not set). |
| `urls` | Backend URL list. |
| `algorithm` | One of `round-robin` (default), `random`, `ip-hash`. |
| `healthCheckInterval` | Health-check interval in seconds. Omit or set to `0` (default) to disable the checker. |

### Balancer

`balancer` is a deprecated alias of [Proxy](#proxy) that accepts the same config keys. It is kept for backward compatibility; prefer `type: proxy` in new configs.

## Routes

Routes are processed in sequence and interrupted when `status` or `handler` is specified.

```yaml
routes:
  - path: / # The request path
    match: prefix # The match type: prefix | equal | regexp
    methods: [] # Allowed HTTP methods
    filters: [] # Filter names
    rewrite: '' # The rewrite path
    rewriteAppendQueryString: false # Like Apache's QSA (Query String Append) flag
    handler: '' # The handler name
    status: 0 # HTTP status
    statusMessage: '' # Custom HTTP status message
```

### Route examples

Rewrite the path and route to a backend.

```yaml
handlers:
  'backend':
    type: proxy
    url: 'http://localhost:8080'
routes:
  - path: ^/view/(.+)
    match: regexp
    rewrite: /view?id=$1
  - path: /
    handler: backend
```

Redirect to an external URL with status code 302.

```yaml
routes:
  - path: /redirect-external
    match: equal
    rewrite: http://example.com/
    status: 302
```

## Routes Cache

Route calculations are cached, yielding significant gains when routing relies heavily on regular expressions.

```yaml
routesCache:
  enable: true
  expire: 60000    # TTL in milliseconds
  interval: 60000  # Background eviction interval in milliseconds
  maxEntries: 0    # 0 (default) means unbounded; set a positive value to cap the cache
```

| Key | Description |
| --- | ----------- |
| `enable` | Enable the routes cache. |
| `expire` | Entry TTL in milliseconds. Defaults to 5 minutes (300000) when omitted or non-positive. |
| `interval` | Minimum gap between background eviction passes, in milliseconds. Defaults to 1 minute (60000) when omitted or non-positive. |
| `maxEntries` | Maximum number of stored entries. When the cap is reached, `Set` on a new key is dropped (existing entries are preserved); this prioritizes already-cached hot paths over adversarial unique-key floods. Zero or negative means unbounded. |

## SSL

```yaml
ssl:
  certFile: ./ssl/localhost.crt
  keyFile: ./ssl/localhost.key
```

## SSL auto cert

```yaml
ssl:
  autoCert: true
  autoCertCacheDir: /etc/fasthttpd/cache
```

See [examples/config.autocert.yaml](https://github.com/fasthttpd/fasthttpd/blob/main/examples/config.autocert.yaml) for a runnable example.

## Virtual hosts

Virtual hosts can be defined in a multi-document YAML file. Each document becomes a virtual host, and the first document is the default host that handles requests whose `Host` header does not match any other document.

```yaml
host: default.example.com
listen: ':80'
---
host: other.example.com
listen: ':80'
```

### Shared listener merge rules

When multiple documents share the same `listen` address, fasthttpd opens a single listener and merges the documents as follows:

- **Routes, handlers, filters** — dispatched per request based on the `Host` header. Each virtual host has its own route table, handler set, and filter set.
- **Server settings (`server:` block)** — the first document's values are used for the shared listener. Values from other documents are ignored.
- **Top-level fields that take effect per listener** (such as `shutdownTimeout`) — the first document's value is used.
- **Log output (`log.output`)** — writes are fanned out to every unique output across the documents on the shared listener. Set the same `log.output` in every document to avoid duplicate lines.

Fields that are inherently per-host (`host`, `root`, `errorPages`, route/handler/filter definitions) apply only to their own document.

## Include

```yaml
include: /etc/fasthttpd/conf.d/*.yaml
```
