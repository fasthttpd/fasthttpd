package route

import (
	"bytes"
	"net/http"
	"reflect"
	"testing"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/valyala/fasthttp"
)

func Test_Match(t *testing.T) {
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

func Test_NewRoutes(t *testing.T) {
	tests := []struct {
		c      config.Config
		errstr string
	}{
		{
			c: config.Config{
				Routes: []config.Route{
					{
						Filters: []string{"test"},
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

func testRoutes(t *testing.T, rs *Routes) {
	tests := []struct {
		method string
		path   string
		want   *RoutesResult
	}{
		{
			method: http.MethodDelete,
			path:   "/",
			want: &RoutesResult{
				StatusCode:    405,
				StatusMessage: []byte("Method not allowed"),
			},
		}, {
			method: http.MethodGet,
			path:   "/",
			want: &RoutesResult{
				Handler: "static",
			},
		}, {
			method: http.MethodGet,
			path:   "/img/test.png",
			want: &RoutesResult{
				Filters: []string{"cache"},
				Handler: "static",
			},
		}, {
			method: http.MethodGet,
			path:   "/view/1",
			want: &RoutesResult{
				Handler:    "backend",
				Filters:    []string{"auth"},
				RewriteURI: []byte("/view?id=1"),
			},
		}, {
			method: http.MethodGet,
			path:   "/redirect-external",
			want: &RoutesResult{
				RedirectURI:   []byte("http://example.com/"),
				StatusCode:    302,
				StatusMessage: []byte(http.StatusText(302)),
			},
		}, {
			method: http.MethodGet,
			path:   "/redirect-internal",
			want: &RoutesResult{
				RedirectURI:       []byte("/internal?foo=bar"),
				AppendQueryString: true,
				StatusCode:        302,
				StatusMessage:     []byte(http.StatusText(302)),
			},
		}, {
			method: http.MethodGet,
			path:   "/route/to/bachend",
			want: &RoutesResult{
				Handler: "backend",
				Filters: []string{"auth"},
			},
		},
	}
	for i, test := range tests {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.Header.SetMethod(test.method)
		ctx.URI().SetPath(test.path)
		got := rs.CachedRouteCtx(ctx)
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("tests[%d] unexpected result %#v; want %#v", i, *got, *test.want)
		}
		if rs.cache != nil {
			got2 := rs.CachedRoute([]byte(test.method), []byte(test.path))
			if !reflect.DeepEqual(got2, test.want) {
				t.Errorf("tests[%d] unexpected 2nd result %#v; want %#v", i, *got2, *test.want)
			}
		}
	}
}

func Test_Routes(t *testing.T) {
	c, err := config.UnmarshalYAMLPath("../config/testdata/full.yaml")
	if err != nil {
		t.Fatal(err)
	}
	rs, err := NewRoutes(c[0])
	if err != nil {
		t.Fatal(err)
	}
	testRoutes(t, rs)

	rs.cache = nil
	testRoutes(t, rs)
}

func Test_RouteNotFound(t *testing.T) {
	rs, err := NewRoutes(config.Config{})
	if err != nil {
		t.Fatal(err)
	}

	got := rs.Route([]byte(http.MethodGet), []byte("/"))
	want := &RoutesResult{StatusCode: http.StatusNotFound}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v; want %#v", got, want)
	}
}

func Test_RoutesResult_RewriteURIWithQueryString(t *testing.T) {
	tests := []struct {
		result     *RoutesResult
		requestUri []byte
		want       []byte
	}{
		{
			result: &RoutesResult{
				RewriteURI:        []byte("/rewrite-uri"),
				AppendQueryString: false,
			},
			requestUri: []byte("/path?a=1"),
			want:       []byte("/rewrite-uri"),
		}, {
			result: &RoutesResult{
				RewriteURI:        []byte("/rewrite-uri"),
				AppendQueryString: true,
			},
			requestUri: []byte("/path?a=1"),
			want:       []byte("/rewrite-uri?a=1"),
		}, {
			result: &RoutesResult{
				RewriteURI:        []byte("/rewrite-uri?b=2"),
				AppendQueryString: true,
			},
			requestUri: []byte("/path?a=1"),
			want:       []byte("/rewrite-uri?b=2&a=1"),
		},
	}
	for i, test := range tests {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI(string(test.requestUri))
		got := test.result.RewriteURIWithQueryString(ctx)
		if !bytes.Equal(got, test.want) {
			t.Errorf("tests[%d] unexpected uri %q; want %q", i, got, test.want)
		}
	}
}

func Test_RoutesResult_RedirectURIWithQueryString(t *testing.T) {
	tests := []struct {
		result     *RoutesResult
		requestUri []byte
		want       []byte
	}{
		{
			result: &RoutesResult{
				RedirectURI:       []byte("http://example.com/"),
				AppendQueryString: false,
			},
			requestUri: []byte("/path?a=1"),
			want:       []byte("http://example.com/"),
		}, {
			result: &RoutesResult{
				RedirectURI:       []byte("http://example.com/"),
				AppendQueryString: true,
			},
			requestUri: []byte("/path?a=1"),
			want:       []byte("http://example.com/?a=1"),
		}, {
			result: &RoutesResult{
				RedirectURI:       []byte("http://example.com/?a=1"),
				AppendQueryString: true,
			},
			requestUri: []byte("/path?b=2"),
			want:       []byte("http://example.com/?a=1&b=2"),
		},
	}
	for i, test := range tests {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI(string(test.requestUri))
		got := test.result.RedirectURIWithQueryString(ctx)
		if !bytes.Equal(got, test.want) {
			t.Errorf("tests[%d] unexpected uri %q; want %q", i, got, test.want)
		}
	}
}
