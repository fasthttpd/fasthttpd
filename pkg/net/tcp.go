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

// TcpKeepaliveListener sets TCP keep-alive on accepted connections so dead
// TCP connections (e.g. laptop closed mid-download) eventually go away.
//
// fasthttp.Server.acceptConn also sets keepalive via its connKeepAliveer
// interface since v1.38, so this wrapper looks redundant for plain TCP
// listeners. It is kept because *tls.Conn does not implement SetKeepAlive,
// so fasthttp cannot apply Server.TCPKeepalive when the listener is
// tls.NewListener-wrapped. Setting keepalive on the inner *net.TCPConn here,
// before TLS wrapping, preserves the setting for TLS servers.
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
