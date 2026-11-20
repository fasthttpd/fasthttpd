package handler

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/mojatter/tree"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
)

func TestNewProxyHandler(t *testing.T) {
	testCases := []struct {
		caseName string
		cfg      tree.Map
		errstr   string
	}{
		{
			caseName: "single url",
			cfg:      tree.Map{"url": tree.ToValue("http://localhost:9000/")},
		}, {
			caseName: "multiple urls",
			cfg: tree.Map{
				"urls": tree.ToArrayValues(
					"http://localhost:9000",
					"http://localhost:9001",
				),
				"healthCheckInterval": tree.ToValue(1),
			},
		}, {
			caseName: "empty cfg returns error",
			cfg:      tree.Map{},
			errstr:   `failed to create proxy: require 'url' or 'urls' entry`,
		}, {
			caseName: "invalid url returns parse error",
			cfg:      tree.Map{"url": tree.ToValue(":invalid url")},
			errstr:   `failed to create proxy: parse ":invalid url": missing protocol scheme`,
		}, {
			caseName: "algorithm round-robin",
			cfg: tree.Map{
				"url":       tree.ToValue("http://localhost:9000"),
				"algorithm": tree.ToValue("round-robin"),
			},
		}, {
			caseName: "algorithm random",
			cfg: tree.Map{
				"url":       tree.ToValue("http://localhost:9000"),
				"algorithm": tree.ToValue("random"),
			},
		}, {
			caseName: "algorithm ip-hash",
			cfg: tree.Map{
				"url":       tree.ToValue("http://localhost:9000"),
				"algorithm": tree.ToValue("ip-hash"),
			},
		}, {
			caseName: "unknown algorithm returns error",
			cfg: tree.Map{
				"url":       tree.ToValue("http://localhost:9000"),
				"algorithm": tree.ToValue("UNKNOWN"),
			},
			errstr: `failed to create proxy: algorithm not supported: UNKNOWN`,
		}, {
			caseName: "dropped algorithm p2c returns error",
			cfg: tree.Map{
				"url":       tree.ToValue("http://localhost:9000"),
				"algorithm": tree.ToValue("p2c"),
			},
			errstr: `failed to create proxy: algorithm not supported: p2c`,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			h, err := NewProxyHandler(tc.cfg, logger.NilLogger)
			if tc.errstr != "" {
				if err == nil {
					t.Fatalf("unexpected no error")
				}
				if err.Error() != tc.errstr {
					t.Errorf("unexpected error: %q; want %q", err.Error(), tc.errstr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if h == nil {
				t.Fatalf("unexpected nil handler")
			}
		})
	}
}

// TestProxyBalancer_RoundRobinRotation verifies that successive picks under
// round-robin cycle through every backend before repeating.
func TestProxyBalancer_RoundRobinRotation(t *testing.T) {
	b, err := newProxyBalancer(
		[]string{"http://a", "http://b", "http://c"},
		algoRoundRobin,
		logger.NilLogger,
	)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for range 6 {
		be := b.pick(req)
		if be == nil {
			t.Fatalf("unexpected nil backend")
		}
		seen[be.url.String()]++
	}
	for _, u := range []string{"http://a", "http://b", "http://c"} {
		if seen[u] != 2 {
			t.Errorf("backend %s picked %d times; want 2", u, seen[u])
		}
	}
}

// TestProxyBalancer_IPHashDeterministic verifies that the same RemoteAddr is
// always mapped to the same backend.
func TestProxyBalancer_IPHashDeterministic(t *testing.T) {
	b, err := newProxyBalancer(
		[]string{"http://a", "http://b", "http://c"},
		algoIPHash,
		logger.NilLogger,
	)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	first := b.pick(req)
	for range 5 {
		if be := b.pick(req); be != first {
			t.Fatalf("ip-hash picked different backend on repeat: got %s want %s",
				be.url, first.url)
		}
	}
}

// TestProxyBalancer_SkipDeadBackend verifies that pick skips backends that are
// marked down and falls through to the next alive one.
func TestProxyBalancer_SkipDeadBackend(t *testing.T) {
	b, err := newProxyBalancer(
		[]string{"http://a", "http://b", "http://c"},
		algoRoundRobin,
		logger.NilLogger,
	)
	if err != nil {
		t.Fatal(err)
	}
	b.backends[0].alive.Store(false)
	b.backends[2].alive.Store(false)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for range 3 {
		be := b.pick(req)
		if be == nil || be.url.String() != "http://b" {
			t.Fatalf("expected http://b; got %v", be)
		}
	}
}

// TestProxyBalancer_NoAliveBackendReturns503 verifies that ServeHTTP returns
// 503 when every backend is marked down.
func TestProxyBalancer_NoAliveBackendReturns503(t *testing.T) {
	b, err := newProxyBalancer(
		[]string{"http://a", "http://b"},
		algoRoundRobin,
		logger.NilLogger,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, be := range b.backends {
		be.alive.Store(false)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	b.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d; want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

// TestProxyBalancer_RunHealthCheck drives the tick loop via a synthetic channel
// and verifies that one iteration updates every backend's alive state based on
// its HEAD response. Covers three transitions in a single tick:
//   - 2xx backend that was alive stays alive
//   - 5xx backend that was alive is marked down
//   - 2xx backend that was previously down comes back online
func TestProxyBalancer_RunHealthCheck(t *testing.T) {
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer good.Close()

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()

	recovering := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer recovering.Close()

	b, err := newProxyBalancer(
		[]string{good.URL, bad.URL, recovering.URL},
		algoRoundRobin,
		logger.NilLogger,
	)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate "recovering" having been marked down by a previous iteration.
	b.backends[2].alive.Store(false)

	tick := make(chan time.Time, 1)
	done := make(chan struct{})
	client := &fasthttp.Client{
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	}
	go func() {
		defer close(done)

		b.runHealthCheck(tick, client)
	}()

	tick <- time.Now()
	close(tick)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("runHealthCheck did not exit within 3s of tick channel close")
	}

	if !b.backends[0].alive.Load() {
		t.Errorf("good backend should stay alive")
	}
	if b.backends[1].alive.Load() {
		t.Errorf("bad backend (5xx) should be marked down")
	}
	if !b.backends[2].alive.Load() {
		t.Errorf("recovering backend should be marked back online")
	}
}

func TestProxy_SchemaRegistered(t *testing.T) {
	testCases := []struct {
		caseName string
		handler  tree.Map
		wantErr  string
	}{
		{
			caseName: "valid single-url proxy",
			handler: tree.Map{
				"type": tree.V("proxy"),
				"url":  tree.V("http://localhost:8080"),
			},
		},
		{
			caseName: "valid balanced proxy",
			handler: tree.Map{
				"type":                tree.V("proxy"),
				"urls":                tree.A("http://a:8080", "http://b:8080"),
				"algorithm":           tree.V("ip-hash"),
				"healthCheckInterval": tree.V(5),
			},
		},
		{
			caseName: "unknown algorithm rejected",
			handler: tree.Map{
				"type":      tree.V("proxy"),
				"url":       tree.V("http://localhost"),
				"algorithm": tree.V("least-conn"),
			},
			wantErr: "algorithm",
		},
		{
			caseName: "unknown proxy field",
			handler: tree.Map{
				"type":  tree.V("proxy"),
				"bogus": tree.V(1),
			},
			wantErr: `.handlers["p"]: unknown key "bogus"`,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			docs := []tree.Map{{"handlers": tree.Map{"p": tc.handler}}}
			err := config.ValidateTreeMaps(docs)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateTreeMaps returned %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateTreeMaps returned nil, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestProxy_MultipartForwarding(t *testing.T) {
	const contentType = "multipart/form-data; boundary=XB"
	body := strings.Join([]string{
		"--XB",
		`Content-Disposition: form-data; name="b"`,
		"X-Extra: keep-me",
		"",
		"2",
		"--XB",
		`Content-Disposition: form-data; name="a"`,
		"",
		"1",
		"--XB",
		`Content-Disposition: form-data; name="f"; filename="f.bin"`,
		"Content-Type: application/octet-stream",
		"",
		"\x00\x01binary\r\ndata",
		"--XB--",
		"",
	}, "\r\n")

	testCases := []struct {
		caseName  string
		server    tree.Map
		wantBytes bool
	}{
		{
			caseName:  "default forwards byte-for-byte",
			wantBytes: true,
		}, {
			caseName: "pre-parse re-encodes",
			server:   tree.Map{"disablePreParseMultipartForm": tree.ToValue(false)},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			got := make(chan []byte, 1)
			gotType := make(chan string, 1)
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				gotType <- r.Header.Get("Content-Type")
				got <- b
			}))
			defer backend.Close()

			m := tree.Map{}
			if tc.server != nil {
				m["server"] = tc.server
			}
			cfg, err := config.FromTreeMap(m)
			if err != nil {
				t.Fatal(err)
			}
			h, err := NewProxyHandler(tree.Map{"url": tree.ToValue(backend.URL)}, logger.NilLogger)
			if err != nil {
				t.Fatal(err)
			}
			s := &fasthttp.Server{Handler: h}
			cfg.Server.ApplyTo(s)
			c := serveInmemory(t, s)

			req := fasthttp.AcquireRequest()
			defer fasthttp.ReleaseRequest(req)

			req.SetRequestURI("http://example.com/")
			req.Header.SetMethod(fasthttp.MethodPost)
			req.Header.SetContentType(contentType)
			req.SetBodyString(body)
			resp := &fasthttp.Response{}
			if err := c.Do(req, resp); err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode() != http.StatusOK {
				t.Fatalf("unexpected status %d", resp.StatusCode())
			}
			b := <-got
			if gotBytes := string(b) == body; gotBytes != tc.wantBytes {
				t.Errorf("byte-for-byte %v; want %v; got %q", gotBytes, tc.wantBytes, b)
			}
			if gotExtra := strings.Contains(string(b), "X-Extra: keep-me"); gotExtra != tc.wantBytes {
				t.Errorf("part header kept %v; want %v", gotExtra, tc.wantBytes)
			}
			// Both modes must forward the same form content.
			_, params, err := mime.ParseMediaType(<-gotType)
			if err != nil {
				t.Fatal(err)
			}
			form, err := multipart.NewReader(bytes.NewReader(b), params["boundary"]).ReadForm(1 << 20)
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprint(form.Value); got != "map[a:[1] b:[2]]" {
				t.Errorf("unexpected values %s", got)
			}
			if len(form.File["f"]) != 1 || form.File["f"][0].Filename != "f.bin" {
				t.Fatalf("unexpected files %v", form.File)
			}
			f, err := form.File["f"][0].Open()
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()

			if data, _ := io.ReadAll(f); string(data) != "\x00\x01binary\r\ndata" {
				t.Errorf("unexpected file data %q", data)
			}
		})
	}
}

// Run with -race: the transport must not read the body after fasthttp reuses its buffer (#111).
func TestProxy_EarlyResponseBody(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer backend.Close()

	h, err := NewProxyHandler(tree.Map{"url": tree.ToValue(backend.URL)}, logger.NilLogger)
	if err != nil {
		t.Fatal(err)
	}
	const size = 8 << 20
	c := serveInmemory(t, &fasthttp.Server{Handler: h, MaxRequestBodySize: size})
	body := strings.Repeat("x", size)
	for i := range 10 {
		if got := doPost(t, c, "example.com", body, false).StatusCode(); got != http.StatusUnauthorized {
			t.Fatalf("request %d: unexpected status %d", i, got)
		}
	}
}

// streamAndDisconnect sends req to h over TCP and resets the connection once "first" arrives.
func streamAndDisconnect(t *testing.T, h fasthttp.RequestHandler, req string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go (&fasthttp.Server{Handler: h, Logger: logger.NilLogger}).Serve(ln) //nolint:errcheck // closed by ln.Close

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if _, err := io.WriteString(conn, req); err != nil {
		t.Fatal(err)
	}
	conn.SetReadDeadline(time.Now().Add(10 * time.Second)) //nolint:errcheck // fail rather than hang
	br := bufio.NewReader(conn)
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(line, "first") {
			break
		}
	}
	conn.(*net.TCPConn).SetLinger(0) //nolint:errcheck // reset so the next proxy write fails
}

// Also run with -race: fasthttp resets the RequestCtx of a streamed response before ServeHTTP returns.
func TestProxy_StreamClientDisconnect(t *testing.T) {
	release := make(chan struct{})
	cancelled := make(chan bool, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("first\n")) //nolint:errcheck // proxy reads it
		w.(http.Flusher).Flush()
		<-release
		// This write makes fasthttp notice the disconnect; the proxy must then cancel without another one.
		w.Write([]byte("more\n")) //nolint:errcheck // proxy may be gone
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
			cancelled <- true
		case <-time.After(2 * time.Second):
			cancelled <- false
		}
	}))
	defer backend.Close()
	// Unblock the backend before Close waits for it, also when the test fails early.
	releaseOnce := sync.OnceFunc(func() { close(release) })
	defer releaseOnce()

	h, err := NewProxyHandler(tree.Map{"url": tree.ToValue(backend.URL)}, logger.NilLogger)
	if err != nil {
		t.Fatal(err)
	}
	streamAndDisconnect(t, h, "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
	releaseOnce()
	if !<-cancelled {
		t.Error("backend request not cancelled after the client disconnected")
	}
}

func TestProxy_DetachedOnRelease(t *testing.T) {
	testCases := []struct {
		caseName string
		req      string
		wantBody bool
	}{
		{
			caseName: "without body",
			req:      "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n",
		}, {
			caseName: "with body",
			req:      "POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 1\r\n\r\nx",
			wantBody: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			b, err := newProxyBalancer([]string{"http://127.0.0.1:1"}, algoRoundRobin, logger.NilLogger)
			if err != nil {
				t.Fatal(err)
			}
			type result struct{ cancelled, wrapped, closed bool }
			got := make(chan result, 1)
			b.backends[0].handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var res result
				defer func() { got <- res }()

				deadline := time.After(2 * time.Second)
				for !res.cancelled {
					// Writes fail once the client is gone, and fasthttp then releases the RequestCtx.
					w.Write([]byte("first\n")) //nolint:errcheck // client may be gone
					w.(http.Flusher).Flush()
					select {
					case <-r.Context().Done():
						res.cancelled = true
					case <-deadline:
						return
					case <-time.After(10 * time.Millisecond):
					}
				}
				if body, ok := r.Body.(*detachedBody); ok {
					body.mu.Lock()
					res.wrapped, res.closed = true, body.closed
					body.mu.Unlock()
				}
			})
			streamAndDisconnect(t, fasthttpadaptor.NewFastHTTPHandler(b), tc.req)
			res := <-got
			if !res.cancelled {
				t.Error("context not cancelled after the RequestCtx was released")
			}
			if res.wrapped != tc.wantBody || res.closed != tc.wantBody {
				t.Errorf("body wrapped %v, closed %v; want %v", res.wrapped, res.closed, tc.wantBody)
			}
		})
	}
}

// Run with -race: a dial for a request served by a pooled conn outlives its handler.
func TestProxy_SlowDial(t *testing.T) {
	wait := func(ch <-chan struct{}) {
		select {
		case <-ch:
		case <-time.After(10 * time.Second):
			t.Error("timed out")
		}
	}
	slowDial := make(chan struct{})
	finish := make(chan struct{})
	var served atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The second request holds the pooled conn until the other one dials.
		if served.Add(1) == 2 {
			wait(slowDial)
		}
	}))
	defer backend.Close()

	b, err := newProxyBalancer([]string{backend.URL}, algoRoundRobin, logger.NilLogger)
	if err != nil {
		t.Fatal(err)
	}
	var dials atomic.Int32
	dialed := make(chan struct{}, 4)
	tr := &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		if dials.Add(1) == 2 {
			close(slowDial)
			wait(finish)
		}
		conn, err := (&net.Dialer{}).DialContext(ctx, network, addr)
		dialed <- struct{}{}
		return conn, err
	}}
	defer tr.CloseIdleConnections()

	b.backends[0].handler.(*httputil.ReverseProxy).Transport = tr
	c := serveInmemory(t, &fasthttp.Server{Handler: fasthttpadaptor.NewFastHTTPHandler(b)})
	get := func() {
		req := fasthttp.AcquireRequest()
		defer fasthttp.ReleaseRequest(req)

		req.SetRequestURI("http://example.com/")
		if err := c.Do(req, &fasthttp.Response{}); err != nil {
			t.Error(err)
		}
	}
	get()
	wait(dialed)

	// One request takes the idle conn; the other dials and gets that conn back first.
	var wg sync.WaitGroup
	wg.Go(get)
	wg.Go(get)
	wg.Wait()
	get()
	get()
	close(finish)
	wait(dialed)
}
