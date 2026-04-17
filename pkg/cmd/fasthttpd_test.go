package cmd

import (
	"bufio"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttputil"
)

func TestFastHttpd(t *testing.T) {
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
		if err := d.Main(args); err != nil {
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
		t.Fatalf("unexpected status code %d; want %d", resp.StatusCode(), http.StatusOK)
	}

	info, err := os.Stat("fasthttpd.go")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Header.ContentLength() != int(info.Size()) {
		t.Errorf("unexpected content length %d; want %d", resp.Header.ContentLength(), info.Size())
	}
}

func TestFastHttpd_HandleHUP(t *testing.T) {
	// Note: no t.Parallel — this test sends SIGHUP to the whole test process.

	tmpDir := t.TempDir()
	output := filepath.Join(tmpDir, "hup.log")

	rotator, err := logger.SharedRotator(output, config.Rotation{
		MaxSize:    1,
		MaxBackups: 2,
		MaxAge:     3,
		LocalTime:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rotator.Close()

	if _, err := rotator.Write([]byte("before rotate\n")); err != nil {
		t.Fatal(err)
	}

	d := NewFastHttpd()
	d.handleHUP()
	defer d.Shutdown() //nolint:errcheck

	if err := syscall.Kill(os.Getpid(), syscall.SIGHUP); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		entries, err := os.ReadDir(tmpDir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) >= 2 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("rotation did not happen within deadline")
}
