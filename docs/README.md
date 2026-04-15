# FastHttpd Documentation

A lightweight HTTP server built on [valyala/fasthttp](https://github.com/valyala/fasthttp).

## Contents

- [Getting started](getting-started.md) — minimal configuration and first run
- [Installation](installation.md) — Go install, binary download, Homebrew, apt/yum, Docker
- [Configuration](configuration.md) — full YAML reference
- [Access log](access-log.md) — NCSA format, JSON preset, LTSV preset
- [CLI](cli.md) — command-line usage and flags

## Features

- Serve static files
- Flexible routing (exact, prefix, and regular-expression match)
- Access logging (NCSA, JSON, and LTSV presets; allocation-free hot path)
- Reverse proxy
- Customize request and response headers
- TLS (HTTPS/SSL), including automatic certificates via Let's Encrypt (autocert / ACME)
- Virtual hosts
- YAML configuration with CLI override

See the top-level [README](../README.md) for a quick feature tour, benchmarks and release-install snippets.
