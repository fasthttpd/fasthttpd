package filter

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
)

func TestNewBasicAuthFilter(t *testing.T) {
	tests := []struct {
		cfg    tree.Map
		want   *BasicAuthFilter
		errstr string
	}{
		{
			cfg: tree.Map{},
			want: &BasicAuthFilter{
				Realm: DefaultRealm,
			},
		}, {
			cfg: tree.Map{
				"realm": tree.ToValue("staff only"),
			},
			want: &BasicAuthFilter{
				Realm: "staff only",
			},
		}, {
			cfg: tree.Map{
				"users": tree.Array{
					tree.Map{
						"name":   tree.ToValue("fast"),
						"secret": tree.ToValue("httpd"),
					},
				},
			},
			want: &BasicAuthFilter{
				Realm: DefaultRealm,
				Users: []*BasicAuthUser{
					{
						Name: "fast",
						auth: []byte{0x5a, 0x6d, 0x46, 0x7a, 0x64, 0x44, 0x70, 0x6f, 0x64, 0x48, 0x52, 0x77, 0x5a, 0x41, 0x3d, 0x3d},
					},
				},
			},
		}, {
			cfg: tree.Map{
				"users": tree.Array{
					tree.Map{
						"name":   tree.ToValue("fast"),
						"secret": tree.ToValue("httpd"),
					},
				},
				"usersFile": tree.ToValue("../config/testdata/users.yaml"),
			},
			want: &BasicAuthFilter{
				Realm: DefaultRealm,
				Users: []*BasicAuthUser{
					{
						Name: "fast",
						auth: []byte{0x5a, 0x6d, 0x46, 0x7a, 0x64, 0x44, 0x70, 0x6f, 0x64, 0x48, 0x52, 0x77, 0x5a, 0x41, 0x3d, 0x3d},
					}, {
						Name: "user01",
						auth: []byte{0x64, 0x58, 0x4e, 0x6c, 0x63, 0x6a, 0x41, 0x78, 0x4f, 0x6e, 0x4e, 0x6c, 0x59, 0x33, 0x4a, 0x6c, 0x64, 0x44, 0x41, 0x78},
					}, {
						Name: "user02",
						auth: []byte{0x64, 0x58, 0x4e, 0x6c, 0x63, 0x6a, 0x41, 0x79, 0x4f, 0x6e, 0x4e, 0x6c, 0x59, 0x33, 0x4a, 0x6c, 0x64, 0x44, 0x41, 0x79},
					},
				},
				UsersFile: "../config/testdata/users.yaml",
			},
		}, {
			cfg: tree.Map{
				"usersFile": tree.ToValue("not-found.yaml"),
			},
			errstr: "open not-found.yaml: no such file or directory",
		},
	}
	for i, test := range tests {
		got, err := NewBasicAuthFilter(test.cfg)
		if test.errstr != "" {
			if err == nil {
				t.Fatalf("tests[%d] is no error; want %q", i, test.errstr)
			}
			if err.Error() != test.errstr {
				t.Errorf("tests[%d] error %q; want %q", i, err.Error(), test.errstr)
			}
			continue
		}
		if err != nil {
			t.Fatalf("tests[%d] error %v", i, err)
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("tests[%d] got %#v; want %#v", i, got, *test.want)
		}
	}
}

func TestBasicAuthFilter(t *testing.T) {
	tests := []struct {
		auth       *BasicAuthFilter
		ctx        func() *fasthttp.RequestCtx
		want       bool
		statusCode int
	}{
		{
			auth: &BasicAuthFilter{
				Users: []*BasicAuthUser{
					{
						Name:   "foo",
						Secret: "bar",
					},
				},
			},
			ctx: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				return ctx
			},
			statusCode: http.StatusUnauthorized,
		}, {
			auth: &BasicAuthFilter{
				Users: []*BasicAuthUser{
					{
						Name:   "foo",
						Secret: "bar",
					},
				},
			},
			ctx: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.Request.Header.Add("Authorization", "Basic Zm9vOmJhcg==")
				return ctx
			},
			want:       true,
			statusCode: http.StatusOK,
		}, {
			auth: &BasicAuthFilter{
				Users: []*BasicAuthUser{
					{
						Name:   "foo",
						Secret: "bar",
					},
				},
			},
			ctx: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.Request.Header.Add("Authorization", "Unknown")
				return ctx
			},
			want:       false,
			statusCode: http.StatusBadRequest,
		}, {
			auth: &BasicAuthFilter{
				Users: []*BasicAuthUser{
					{
						Name:   "foo",
						Secret: "foo",
					},
				},
			},
			ctx: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.Request.Header.Add("Authorization", "Basic Zm9vOmJhcg==")
				return ctx
			},
			want:       false,
			statusCode: http.StatusUnauthorized,
		},
	}
	for i, test := range tests {
		if err := test.auth.init(); err != nil {
			t.Fatal(err)
		}
		ctx := test.ctx()
		got := test.auth.Request(ctx)
		if got != test.want {
			t.Fatalf("tests[%d] got %v; want %v", i, got, test.want)
		}
		statusCode := ctx.Response.StatusCode()
		if statusCode != test.statusCode {
			t.Errorf("tests[%d] statusCode is %d; want %d", i, statusCode, test.statusCode)
		}
		if !test.auth.Response(ctx) {
			t.Errorf("tests[%d] response returns false", i)
		}
	}
}
