# Installation

## Go install

```sh
go install github.com/fasthttpd/fasthttpd/cmd/fasthttpd@latest
```

## Download binary

Download a binary from the [releases page](https://github.com/fasthttpd/fasthttpd/releases).

```sh
VERSION=0.9.0 GOOS=linux GOARCH=amd64; \
  curl -fsSL "https://github.com/fasthttpd/fasthttpd/releases/download/v${VERSION}/fasthttpd_${VERSION}_${GOOS}_${GOARCH}.tar.gz" | \
  tar xz fasthttpd && \
  sudo mv fasthttpd /usr/sbin
```

- `GOOS` supports `linux`, `darwin`, `windows`
- `GOARCH` supports `amd64`, `arm64`, `386`

## Homebrew

```sh
brew tap fasthttpd/fasthttpd
brew install fasthttpd
```

## Using yum or apt

Download a `.deb` or `.rpm` package from the [releases page](https://github.com/fasthttpd/fasthttpd/releases), then run `apt install` or `yum install`.

```sh
VERSION=0.9.0 ARCH=amd64; \
  curl -fsSL -O "https://github.com/fasthttpd/fasthttpd/releases/download/v${VERSION}/fasthttpd_${VERSION}_${ARCH}.deb"
sudo apt install "./fasthttpd_${VERSION}_${ARCH}.deb"
```

- Default configuration path is `/etc/fasthttpd/config.yaml`
- Default log directory is `/var/log/fasthttpd`
- FastHttpd is started automatically by systemd

## Docker

See <https://hub.docker.com/r/fasthttpd/fasthttpd>.

### Exposing an external port

```sh
docker run --rm -p 8080:80 fasthttpd/fasthttpd
```

### Serve static content

```sh
docker run --rm -p 8080:80 -v $(pwd)/public:/usr/share/fasthttpd/html fasthttpd/fasthttpd
```

### Specify a config file

```sh
docker run --rm -p 8080:80 -v your.config.yaml:/etc/fasthttpd/config.yaml fasthttpd/fasthttpd
```
