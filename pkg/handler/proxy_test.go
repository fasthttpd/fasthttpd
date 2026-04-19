package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/mojatter/tree"
	"github.com/valyala/fasthttp"
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
			wantErr: ".bogus: unknown key",
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
