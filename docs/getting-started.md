# Getting started

This guide walks through serving static files with FastHttpd.

## Configuration

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

## Start fasthttpd

Start fasthttpd with a partial override of the built-in configuration, then open <http://localhost:8080/> in your browser.

```sh
fasthttpd -e root=.
```

Or specify your configuration file:

```sh
fasthttpd -f config.yaml
```

See [Configuration](configuration.md) for the full reference, and [CLI](cli.md) for command-line flags.
