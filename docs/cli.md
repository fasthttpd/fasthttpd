# CLI

## Usage

```
FastHttpd is a HTTP server using valyala/fasthttp.

Usage:
  fasthttpd [flags] [query] ([file...])

Flags:
  -e value
        edit expression (eg. -e KEY=VALUE)
  -f string
        configuration file
  -h    help for fasthttpd
  -v    print version
```

## Examples

```sh
fasthttpd -f examples/config.minimal.yaml
fasthttpd -f examples/config.minimal.yaml -e accessLog.output=stdout
fasthttpd -e root=./examples/public -e listen=0.0.0.0:8080
```

The `-e` flag overrides values in `config.yaml` using the expression syntax from [mojatter/tree](https://github.com/mojatter/tree).
