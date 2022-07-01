# syntax=docker/dockerfile:1.0-experimental
FROM golang:1.18.2-alpine3.15

ENV GO111MODULE=on
ENV GOPATH=""

RUN --mount=target=. GOOS=linux CGO_ENABLED=0 \
    go build -o /fasthttpd ./cmd/fasthttpd/main.go

FROM scratch

COPY --from=0 /fasthttpd /usr/sbin/fasthttpd
COPY examples/config.docker.yaml /etc/fasthttpd/config.yaml
COPY examples/public /usr/share/fasthttpd/html

ENV FASTHTTPD_CONFIG=/etc/fasthttpd/config.yaml

EXPOSE 80

ENTRYPOINT [ "/usr/sbin/fasthttpd" ]