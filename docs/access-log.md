# Access log

FastHttpd writes access logs through a `bufio.Writer` + `sync.Pool` pipeline that keeps the formatting hot path allocation-free for the classic NCSA format, the JSON preset and the LTSV preset.

## NCSA format (default)

`format` accepts an Apache-style format string. The default is the NCSA Common Log Format:

```yaml
accessLog:
  output: logs/access.log
  format: '%h %l %u %t "%r" %>s %b'
```

Format directives follow the [Apache Custom Log Formats](https://httpd.apache.org/docs/2.4/en/mod/mod_log_config.html) convention.

## JSON preset

Set `format: json` to emit one JSON object per request with the following 15 fields. The schema is flat `snake_case` so it can be ingested by jq, Loki, Elasticsearch, CloudWatch Logs Insights, and similar tools without transformation.

```yaml
accessLog:
  output: logs/access.log
  format: json
  bufferSize: 4096
  flushInterval: 1000
```

Sample output (one line, pretty-printed here for readability):

```json
{
  "time": "2026-04-11T10:19:13+09:00",
  "remote_addr": "10.1.2.3:51002",
  "client_ip": "10.1.2.3",
  "remote_user": "alice",
  "method": "GET",
  "uri": "/path?foo=bar",
  "proto": "HTTP/1.1",
  "scheme": "http",
  "host": "example.com",
  "status": 200,
  "size": 1234,
  "bytes_received": 0,
  "duration_us": 412,
  "referer": "https://example.com/",
  "user_agent": "curl/8.7.1"
}
```

Notes:

- `time` is RFC 3339 with the local timezone offset.
- `client_ip` is the left-most entry of the `X-Forwarded-For` header when present, otherwise the IP portion of the connecting peer.
- `duration_us` is integer microseconds; the unit is encoded in the key name to avoid ambiguity.
- `size` is the response body length and `bytes_received` is the request header + body byte count.

## LTSV preset

Set `format: ltsv` to emit one [Labeled Tab-separated Values](http://ltsv.org/) record per request. The 13-field schema follows the nginx / fluentd LTSV convention, so existing parsers and log forwarders ingest it without configuration.

```yaml
accessLog:
  output: logs/access.log
  format: ltsv
  bufferSize: 4096
  flushInterval: 1000
```

Sample output (one line, wrapped here for readability):

```
time:2026-04-11T10:19:13+09:00	host:10.1.2.3	forwardedfor:	user:alice
	req:GET /path?foo=bar HTTP/1.1	scheme:http	vhost:example.com
	status:200	size:1234	reqsize:0	reqtime_microsec:412
	referer:https://example.com/	ua:curl/8.7.1
```

Notes:

- `host` is the direct peer IP (without port), matching nginx's `$remote_addr`.
- `forwardedfor` is the raw `X-Forwarded-For` header value; unlike the JSON preset, no left-most derivation is performed.
- `req` combines the method, request URI and protocol NCSA-style.
- Empty string fields are written as empty values, not `-`, following pure LTSV convention.
- Any `TAB`, `LF` or `CR` byte inside a string value is replaced with a single space so a crafted header cannot break the LTSV line structure (equivalent to nginx's `escape=default`).
