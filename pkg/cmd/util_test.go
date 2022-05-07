package cmd

import (
	"net"
	"sync"
	"testing"
	"time"
)

func Test_getNetwork(t *testing.T) {
	tests := []struct {
		listen string
		want   string
	}{
		{
			listen: ":8800",
			want:   "tcp4",
		}, {
			listen: "[::1]:8800",
			want:   "tcp6",
		},
	}
	for i, test := range tests {
		got := getNetwork(test.listen)
		if got != test.want {
			t.Errorf("tests[%d] got %q; want %q", i, got, test.want)
		}
	}
}

func Test_netListen(t *testing.T) {
	ln, err := netListen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
}

func Test_tcpKeepaliveListener(t *testing.T) {
	ln, err := netListen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	tcpLn := &tcpKeepaliveListener{
		TCPListener:     ln.(*net.TCPListener),
		keepalive:       true,
		keepalivePeriod: 30 * time.Second,
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		c, err := tcpLn.Accept()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		c.Close()
	}()

	d := net.Dialer{Timeout: 10 * time.Millisecond}
	c, err := d.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	c.Close()

	ln.Close()
	wg.Wait()
}
