package cmd

import (
	"crypto/tls"
	"net"
	"strings"
	"time"

	"github.com/fasthttpd/fasthttpd/pkg/config"
)

type stringList []string

func (l *stringList) String() string {
	return strings.Join(*l, ",")
}

func (l *stringList) Set(value string) error {
	*l = append(*l, value)
	return nil
}

func getNetwork(listen string) string {
	if strings.Count(listen, ":") >= 2 {
		return "tcp6"
	}
	return "tcp4"
}

var netListen = func(listen string) (net.Listener, error) {
	return net.Listen(getNetwork(listen), listen)
}

// tlsConfig returns a *tls.Config via tls.LoadX509KeyPair.
func tlsConfig(cfgs []config.Config) (*tls.Config, error) {
	var certs []tls.Certificate
	for _, cfg := range cfgs {
		if cfg.SSL.CertFile == "" || cfg.SSL.KeyFile == "" {
			return nil, nil
		}
		cert, err := tls.LoadX509KeyPair(cfg.SSL.CertFile, cfg.SSL.KeyFile)
		if err != nil {
			return nil, err
		}
		certs = append(certs, cert)
	}
	tlsCfg := &tls.Config{
		NextProtos:   []string{"http/1.1"},
		Certificates: certs,
	}
	if len(certs) > 1 {
		tlsCfg.BuildNameToCertificate()
	}
	return tlsCfg, nil
}

// NOTE: Copy from fasthttp/server.go
//
// tcpKeepAliveListener sets TCP keep-alive timeouts on accepted
// connections. It's used by ListenAndServe, ListenAndServeTLS and
// ListenAndServeTLSEmbed so dead TCP connections (e.g. closing laptop mid-download)
// eventually go away.
type tcpKeepaliveListener struct {
	*net.TCPListener
	keepalive       bool
	keepalivePeriod time.Duration
}

func (ln *tcpKeepaliveListener) Accept() (net.Conn, error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return nil, err
	}
	if err := tc.SetKeepAlive(ln.keepalive); err != nil {
		tc.Close() //nolint:errcheck
		return nil, err
	}
	if ln.keepalivePeriod > 0 {
		if err := tc.SetKeepAlivePeriod(ln.keepalivePeriod); err != nil {
			tc.Close() //nolint:errcheck
			return nil, err
		}
	}
	return tc, nil
}
