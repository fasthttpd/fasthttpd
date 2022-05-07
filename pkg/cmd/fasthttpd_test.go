package cmd

import (
	"bufio"
	"net"
	"net/http"
	"os"
	"testing"

	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttputil"
)

func Test_FastHttpd_Typical(t *testing.T) {
	t.Parallel()

	ln := fasthttputil.NewInmemoryListener()
	defer ln.Close()

	netListenOrg := netListen
	defer func() { netListen = netListenOrg }()

	netListen = func(listen string) (net.Listener, error) {
		return ln, nil
	}

	d := NewFastHttpd()
	defer d.Shutdown() //nolint:errcheck
	args := []string{"fasthttpd", "-e", "root=."}

	go func() {
		if err := d.Run(args); err != nil {
			t.Error(err)
		}
	}()

	c, err := ln.Dial()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err = c.Write([]byte("GET /fasthttpd.go HTTP/1.1\r\nHost: localhost\r\n\r\n")); err != nil {
		t.Fatal(err)
	}

	br := bufio.NewReader(c)
	var resp fasthttp.Response
	if err := resp.Read(br); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("got status code: %d. want %d", resp.StatusCode(), http.StatusOK)
	}

	info, err := os.Stat("fasthttpd.go")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Header.ContentLength() != int(info.Size()) {
		t.Errorf("got content length %d; want %d", resp.Header.ContentLength(), info.Size())
	}
}
