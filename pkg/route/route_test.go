package route

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/fasthttpd/fasthttpd/pkg/config"
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
				Path:  "^/view/(.+)",
				Match: config.MatchRegexp,
				Rewrite: config.Rewrite{
					URI: "/view?id=$1",
				},
			},
			method: http.MethodGet,
			path:   "/view/1",
			want:   true,
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

func Test_Route(t *testing.T) {
	c, err := config.UnmarshalYAMLPath("../config/testdata/full.yaml")
	if err != nil {
		t.Fatal(err)
	}

	rs, err := NewRoutes(c)
	if err != nil {
		t.Fatal(err)
	}

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
				RedirectURL:   []byte("http://example.com/"),
				StatusCode:    302,
				StatusMessage: []byte(http.StatusText(302)),
			},
		}, {
			method: http.MethodGet,
			path:   "/redirect-internal",
			want: &RoutesResult{
				RedirectURL:       []byte("/internal?foo=bar"),
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
		got := rs.Route([]byte(test.method), []byte(test.path))
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("tests[%d] got %#v; want %#v", i, *got, *test.want)
		}
	}
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
