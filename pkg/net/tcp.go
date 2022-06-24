package net

import (
	"net"
	"strings"
	"time"
)

func GetNetwork(listen string) string {
	if strings.Count(listen, ":") >= 2 {
		return "tcp6"
	}
	return "tcp4"
}

// NOTE: Copy from fasthttp/server.go
//
// TcpKeepaliveListener sets TCP keep-alive timeouts on accepted
// connections. It's used by ListenAndServe, ListenAndServeTLS and
// ListenAndServeTLSEmbed so dead TCP connections (e.g. closing laptop mid-download)
// eventually go away.
type TcpKeepaliveListener struct {
	*net.TCPListener
	Keepalive       bool
	KeepalivePeriod time.Duration
}

func (ln *TcpKeepaliveListener) Accept() (net.Conn, error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return nil, err
	}
	if err := tc.SetKeepAlive(ln.Keepalive); err != nil {
		tc.Close() //nolint:errcheck
		return nil, err
	}
	if ln.KeepalivePeriod > 0 {
		if err := tc.SetKeepAlivePeriod(ln.KeepalivePeriod); err != nil {
			tc.Close() //nolint:errcheck
			return nil, err
		}
	}
	return tc, nil
}
