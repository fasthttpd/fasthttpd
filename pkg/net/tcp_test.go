package net

import (
	"net"
	"sync"
	"testing"
	"time"
)

func TestGetNetwork(t *testing.T) {
	tests := []struct {
		listen string
		want   string
	}{
		{
			listen: ":8080",
			want:   "tcp4",
		}, {
			listen: "[::1]:8080",
			want:   "tcp6",
		},
	}
	for i, test := range tests {
		got := GetNetwork(test.listen)
		if got != test.want {
			t.Errorf("tests[%d] got %q; want %q", i, got, test.want)
		}
	}
}

func TestTcpKeepaliveListener(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	tcpLn := &TcpKeepaliveListener{
		TCPListener:     ln.(*net.TCPListener),
		Keepalive:       true,
		KeepalivePeriod: 30 * time.Second,
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		serverConn, err := tcpLn.Accept()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		serverConn.Close()
	}()

	dialer := net.Dialer{Timeout: 10 * time.Millisecond}
	conn, err := dialer.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()

	wg.Wait()
	ln.Close()
}
