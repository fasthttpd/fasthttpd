package route

import (
	"net/http"
	"testing"
	"time"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/util"
	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
)

func TestRoute_Match(t *testing.T) {
	tests := []struct {
		cfg    config.Route
		method string
		path   string
		errstr string
		want   bool
	}{
		{
			cfg:    config.Route{},
			method: http.MethodGet,
			path:   "/",
			want:   true,
		}, {
			cfg: config.Route{
				Path:    "/",
				Methods: []string{http.MethodPut, http.MethodDelete},
			},
			method: http.MethodDelete,
			path:   "/",
			want:   true,
		}, {
			cfg: config.Route{
				Path:    `.*\.(js|css|jpg|png|gif)$`,
				Methods: []string{http.MethodGet, http.MethodHead},
				Match:   config.MatchRegexp,
			},
			method: http.MethodGet,
			path:   "/mg/test.png",
			want:   true,
		}, {
			cfg: config.Route{
				Path:    `.*\.(js|css|jpg|png|gif)$`,
				Methods: []string{http.MethodGet, http.MethodHead},
				Match:   config.MatchRegexp,
			},
			method: http.MethodOptions,
			path:   "/img/test.png",
		}, {
			cfg: config.Route{
				Path:    "^/view/(.+)",
				Match:   config.MatchRegexp,
				Rewrite: "/view?id=$1",
			},
			method: http.MethodGet,
			path:   "/view/1",
			want:   true,
		}, {
			cfg: config.Route{
				Path:  "/",
				Match: "invalid-match",
			},
			method: http.MethodGet,
			path:   "/",
			errstr: `unknown match: invalid-match`,
		}, {
			cfg: config.Route{
				Path:  "(invalid regexp",
				Match: config.MatchRegexp,
			},
			method: http.MethodGet,
			path:   "/",
			errstr: "error parsing regexp: missing closing ): `(invalid regexp`",
		},
	}

	for i, test := range tests {
		r, err := NewRoute(test.cfg)
		if test.errstr != "" {
			if err == nil {
				t.Fatalf("tests[%d] no error; want %q", i, test.errstr)
			}
			if err.Error() != test.errstr {
				t.Errorf("tests[%d] error is %q; want %q", i, err.Error(), test.errstr)
			}
			continue
		}
		if err != nil {
			t.Fatalf("tests[%d] error: %v", i, err)
		}
		got := r.Match([]byte(test.method), []byte(test.path))
		if got != test.want {
			t.Errorf("tests[%d] matchPath returns %v; want %v", i, got, test.want)
		}
	}
}

func TestNewRoutes(t *testing.T) {
	tests := []struct {
		c      config.Config
		errstr string
	}{
		{
			c: config.Config{
				Routes: []config.Route{
					{
						Filters: util.StringSet{"test"},
					},
				},
			},
			errstr: "unknown filter: test",
		}, {
			c: config.Config{
				Routes: []config.Route{
					{
						Handler: "test",
					},
				},
			},
			errstr: "unknown handler: test",
		}, {
			c: config.Config{
				Routes: []config.Route{
					{
						Path:  "(invalid regexp",
						Match: config.MatchRegexp,
					},
				},
			},
			errstr: "error parsing regexp: missing closing ): `(invalid regexp`",
		},
	}
	for i, test := range tests {
		_, err := NewRoutes(test.c)
		if test.errstr != "" {
			if err == nil {
				t.Fatalf("tests[%d] no error; want %q", i, test.errstr)
			}
			if err.Error() != test.errstr {
				t.Errorf("tests[%d] error is %q; want %q", i, err.Error(), test.errstr)
			}
			continue
		}
		if err != nil {
			t.Fatalf("tests[%d] error: %v", i, err)
		}
	}
}

func testRoute(t *testing.T, rs *Routes) {
	tests := []struct {
		method string
		path   string
		off    int
		want   *Result
	}{
		{
			method: http.MethodDelete,
			path:   "/",
			want: &Result{
				StatusCode:    405,
				StatusMessage: []byte("Method not allowed"),
			},
		}, {
			method: http.MethodGet,
			path:   "/",
			want: &Result{
				Handler: "static",
			},
		}, {
			method: http.MethodGet,
			path:   "/img/test.png",
			want: &Result{
				Filters: util.StringSet{"cache"},
				Handler: "static-overwrite",
			},
		}, {
			method: http.MethodGet,
			path:   "/view/1",
			want: &Result{
				Handler:    "hello",
				Filters:    util.StringSet{"auth"},
				RewriteURI: []byte("/view?id=1"),
			},
		}, {
			method: http.MethodGet,
			path:   "/redirect-external",
			want: &Result{
				RedirectURI:   []byte("http://example.com/"),
				StatusCode:    302,
				StatusMessage: []byte(http.StatusText(302)),
			},
		}, {
			method: http.MethodGet,
			path:   "/redirect-internal",
			want: &Result{
				RedirectURI:       []byte("/internal?foo=bar"),
				AppendQueryString: true,
				StatusCode:        302,
				StatusMessage:     []byte(http.StatusText(302)),
			},
		}, {
			method: http.MethodGet,
			path:   "/route/to/hello",
			want: &Result{
				Handler: "hello",
				Filters: util.StringSet{"auth"},
			},
		},
	}
	for i, test := range tests {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod(test.method)
		ctx.URI().SetPath(test.path)

		got := rs.CachedRouteCtx(ctx, test.off)
		if !got.Equal(test.want) {
			t.Errorf("tests[%d] unexpected result %#v; want %#v", i, *got, *test.want)
		}
		if rs.cache != nil {
			got2 := rs.CachedRouteCtx(ctx, test.off)
			if !got2.Equal(test.want) {
				t.Errorf("tests[%d] unexpected 2nd result %#v; want %#v", i, *got2, *test.want)
			}
			got2.Release()
		}
		got.Release()
	}
}

func TestRoute(t *testing.T) {
	c, err := config.UnmarshalYAMLPath("../config/testdata/full.yaml")
	if err != nil {
		t.Fatal(err)
	}
	rs, err := NewRoutes(c[0])
	if err != nil {
		t.Fatal(err)
	}
	testRoute(t, rs)

	rs.cache = nil
	testRoute(t, rs)
}

func TestRoute_NotFound(t *testing.T) {
	tests := []struct {
		cfg    config.Config
		method string
		path   string
		off    int
		want   *Result
	}{
		{
			cfg:    config.Config{},
			method: http.MethodGet,
			path:   "/",
			off:    0,
			want: &Result{
				StatusCode: http.StatusNotFound,
			},
		}, {
			cfg: config.Config{
				Handlers: map[string]tree.Map{
					"static": {},
				},
				Routes: []config.Route{
					{
						Path:    "/static",
						Handler: "static",
					},
				},
			},
			method: http.MethodGet,
			path:   "/",
			off:    0,
			want: &Result{
				StatusCode: http.StatusNotFound,
			},
		},
	}
	for i, test := range tests {
		rs, err := NewRoutes(test.cfg)
		if err != nil {
			t.Fatal(err)
		}
		got := rs.Route([]byte(test.method), []byte(test.path), test.off)
		if !got.Equal(test.want) {
			t.Errorf("tests[%d] got %#v; want %#v", i, got, test.want)
		}
		got.Release()
	}
}

func Test_onResultReleased(t *testing.T) {
	cfg := config.Config{
		Routes: []config.Route{
			{
				Path:   "/",
				Match:  config.MatchPrefix,
				Status: http.StatusOK,
			},
		},
		RoutesCache: config.RoutesCache{
			Enable:   true,
			Expire:   1,
			Interval: 1,
		},
	}
	rs, err := NewRoutes(cfg)
	if err != nil {
		t.Fatal(err)
	}

	_ = rs.CachedRoute([]byte(http.MethodGet), []byte("/1"), 0)
	_ = rs.CachedRoute([]byte(http.MethodGet), []byte("/2"), 0)
	_ = rs.CachedRoute([]byte(http.MethodGet), []byte("/3"), 0)
	if size := rs.cache.Len(); size != 3 {
		t.Errorf("unexpected cache size %d; want 3", size)
	}

	time.Sleep(2 * time.Millisecond)
	// NOTE: maybe call rs.cache.notify.
	_ = rs.CachedRoute([]byte(http.MethodGet), []byte("/4"), 0)

	// NOTE: wait rs.cache.expires.
	time.Sleep(2 * time.Millisecond)

	// NOTE: old cached items are deleted.
	if size := rs.cache.Len(); size != 1 {
		t.Errorf("unexpected cache size %d; want 1", size)
	}
}

func TestRoute_IsNextIfNotFound(t *testing.T) {
	tests := []struct {
		rs   *Routes
		ri   int
		want bool
	}{
		{
			rs: &Routes{
				routes: []*Route{
					{
						nextIfNotFound: true,
					}, {
						nextIfNotFound: false,
					},
				},
			},
			ri:   0,
			want: true,
		}, {
			rs: &Routes{
				routes: []*Route{
					{
						nextIfNotFound: true,
					}, {
						nextIfNotFound: false,
					},
				},
			},
			ri:   1,
			want: false,
		}, {
			rs:   &Routes{},
			ri:   0,
			want: false,
		},
	}
	for i, test := range tests {
		got := test.rs.IsNextIfNotFound(test.ri)
		if got != test.want {
			t.Errorf("tests[%d] got %#v; want %#v", i, got, test.want)
		}
	}
}
