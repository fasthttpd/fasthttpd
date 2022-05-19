# syntax=docker/dockerfile:1.0-experimental
FROM golang:1.18.2-alpine3.15

ENV GO111MODULE=on
ENV GOPATH=""

RUN --mount=target=. GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
    go build -o /fasthttpd ./cmd/fasthttpd/main.go

FROM alpine:3.15

COPY --from=0 /fasthttpd /etc/fasthttpd/bin/fasthttpd
COPY examples/config.minimal.yaml /etc/fasthttpd/config.yaml
COPY examples/public /etc/fasthttpd/public

ENV FASTHTTPD_CONFIG=/etc/fasthttpd/config.yaml
EXPOSE 8080

ENTRYPOINT [ "/etc/fasthttpd/bin/fasthttpd" ]