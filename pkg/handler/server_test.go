package handler

import (
	"bytes"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/filter"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/fasthttpd/fasthttpd/pkg/logger/accesslog"
	"github.com/fasthttpd/fasthttpd/pkg/route"
	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
)

func TestNewServerHandler(t *testing.T) {
	tests := []struct {
		cfgs []config.Config
		want string
	}{
		{
			cfgs: []config.Config{
				{},
			},
			want: "*handler.hostHandler",
		}, {
			cfgs: []config.Config{
				{}, {},
			},
			want: "*handler.virtualHandler",
		},
	}
	for i, test := range tests {
		h, err := NewServerHandler(test.cfgs)
		if err != nil {
			t.Fatal(err)
		}
		got := reflect.TypeOf(h).String()
		if got != test.want {
			t.Errorf("tests[%d] unexpected type %v; want %v", i, got, test.want)
		}
	}
}

func newHostHandlerTest(t *testing.T) *hostHandler {
	rs, err := route.NewRoutes(config.Config{
		Filters: map[string]tree.Map{
			"auth":  {},
			"cache": {},
		},
		Handlers: map[string]tree.Map{
			"static":  {},
			"backend": {},
		},
		Routes: []config.Route{
			{
				Path:           `\.(js|css|jpg|png)$`,
				Match:          config.MatchRegexp,
				Handler:        "static",
				Filters:        []string{"cache"},
				NextIfNotFound: true,
			}, {
				Path:    `/view/([0-9]+)$`,
				Match:   config.MatchRegexp,
				Rewrite: "/view?id=$1",
			}, {
				Path:   `/admin`,
				Match:  config.MatchPrefix,
				Status: http.StatusForbidden,
			}, {
				Path:    "/",
				Match:   config.MatchPrefix,
				Handler: "backend",
				Filters: []string{"auth"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &hostHandler{
		logger:    logger.NilLogger,
		accessLog: accesslog.NilAccessLog,
		filters: map[string]filter.Filter{
			"auth": &filter.FilterDelegator{
				RequestFunc: func(ctx *fasthttp.RequestCtx) bool {
					if bytes.Equal(ctx.Request.Header.Peek("X-Auth-Fail"), []byte("true")) {
						ctx.Response.SetStatusCode(http.StatusUnauthorized)
						return false
					}
					return true
				},
			},
			"cache": &filter.FilterDelegator{
				ResponseFunc: func(ctx *fasthttp.RequestCtx) bool {
					ctx.Response.Header.Set("Cache-Control", "private, max-age=3600")
					return false
				},
			},
		},
		handlers: map[string]fasthttp.RequestHandler{
			"static": func(ctx *fasthttp.RequestCtx) {
				if bytes.Equal(ctx.Request.Header.Peek("X-NotFound-Static"), []byte("true")) {
					ctx.Response.SetStatusCode(http.StatusNotFound)
					return
				}
				ctx.Response.Header.Set("Content-Type", "image/png")
				ctx.Response.SetBodyString("Body static")
			},
			"backend": func(ctx *fasthttp.RequestCtx) {
				ctx.Response.Header.Set("Content-Type", "text/html; charset=utf-8")
				ctx.Response.SetBodyString("Body")
			},
		},
		routes:     rs,
		errorPages: &ErrorPages{},
	}
}

func assertResponse(resp *fasthttp.Response, status int, body string, headers [][]string) (err error) {
	gotStatus := resp.StatusCode()
	if gotStatus != status {
		return fmt.Errorf("unexpected status %d; want %d", gotStatus, status)
	}
	gotBody := resp.Body()
	if !bytes.Contains(gotBody, []byte(body)) {
		return fmt.Errorf("unexpected body %q; want contains %s", gotBody, body)
	}
	resp.Header.VisitAll(func(key, value []byte) {
		strKey := string(key)
		if strKey == "Date" {
			return
		}
		for j, kv := range headers {
			k, v := kv[0], kv[1]
			if k == strKey {
				if v != string(value) {
					err = fmt.Errorf("unexpected header %s: %s; want %s", k, value, v)
				}
				headers = append(headers[:j], headers[j+1:]...)
				return
			}
		}
		err = fmt.Errorf("unnecessary header %s: %s", key, value)
	})
	if err != nil {
		return
	}
	if len(headers) != 0 {
		return fmt.Errorf("require headers %v", headers)
	}
	return nil
}

func Test_hostHandler_Handle(t *testing.T) {
	h := newHostHandlerTest(t)
	defer h.Close()

	tests := []struct {
		note        string
		ctx         func() *fasthttp.RequestCtx
		wantReqUri  string
		wantStatus  int
		wantBody    string
		wantHeaders [][]string
	}{
		{
			note: "typical backend",
			ctx: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.Request.SetRequestURI("/")
				return ctx
			},
			wantStatus: http.StatusOK,
			wantBody:   "Body",
			wantHeaders: [][]string{
				{"Content-Type", "text/html; charset=utf-8"},
			},
		}, {
			note: "typical rewrite",
			ctx: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.Request.SetRequestURI("/view/1")
				return ctx
			},
			wantReqUri: "/view?id=1",
			wantStatus: http.StatusOK,
			wantBody:   "Body",
			wantHeaders: [][]string{
				{"Content-Type", "text/html; charset=utf-8"},
			},
		}, {
			note: "typical static",
			ctx: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.Request.SetRequestURI("/img/test.png")
				return ctx
			},
			wantStatus: http.StatusOK,
			wantBody:   "Body static",
			wantHeaders: [][]string{
				{"Content-Type", "image/png"},
				{"Cache-Control", "private, max-age=3600"},
			},
		}, {
			note: "next if not found",
			ctx: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.Request.SetRequestURI("/img/test.png")
				ctx.Request.Header.Set("X-NotFound-Static", "true")
				return ctx
			},
			wantStatus: http.StatusOK,
			wantBody:   "Body",
			wantHeaders: [][]string{
				{"Content-Type", "text/html; charset=utf-8"},
			},
		}, {
			note: "typical status",
			ctx: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.Request.SetRequestURI("/admin")
				return ctx
			},
			wantStatus: http.StatusForbidden,
			wantBody:   "403 Forbidden",
			wantHeaders: [][]string{
				{"Content-Type", "text/html; charset=utf-8"},
			},
		}, {
			note: "failed to auth",
			ctx: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.Request.SetRequestURI("/")
				ctx.Request.Header.Set("X-Auth-Fail", "true")
				return ctx
			},
			wantStatus: http.StatusUnauthorized,
			wantBody:   "401 Unauthorized",
			wantHeaders: [][]string{
				{"Content-Type", "text/html; charset=utf-8"},
			},
		},
	}
	for i, test := range tests {
		ctx := test.ctx()
		h.Handle(ctx)

		if test.wantReqUri != "" {
			if reqUri := string(ctx.RequestURI()); reqUri != test.wantReqUri {
				t.Errorf("tests[%d] unexpected request uri %s; want %s", i, reqUri, test.wantReqUri)
			}
		}
		if err := assertResponse(&ctx.Response, test.wantStatus, test.wantBody, test.wantHeaders); err != nil {
			t.Errorf("tests[%d] %v", i, err)
		}
	}
}

func Test_virtualHandler(t *testing.T) {
	h, err := newVirtualHandler([]config.Config{
		{
			Host: "host1",
			Routes: []config.Route{
				{
					Path:   "/",
					Match:  config.MatchEqual,
					Status: http.StatusOK,
				},
			},
		}, {
			Host: "host2",
			Routes: []config.Route{
				{
					Path:    "/",
					Match:   config.MatchPrefix,
					Status:  http.StatusFound,
					Rewrite: "http://example.com/",
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	tests := []struct {
		host       string
		path       string
		wantStatus int
	}{
		{
			host:       "host1",
			path:       "/",
			wantStatus: http.StatusOK,
		}, {
			host:       "host1",
			path:       "/not-found",
			wantStatus: http.StatusNotFound,
		}, {
			host:       "host2",
			path:       "/",
			wantStatus: http.StatusFound,
		},
	}
	for i, test := range tests {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.URI().SetHost(test.host)
		ctx.Request.URI().SetPath(test.path)
		h.Handle(ctx)

		gotStatus := ctx.Response.StatusCode()
		if gotStatus != test.wantStatus {
			t.Errorf("tests[%d] unexpected status %d; want %d", i, gotStatus, test.wantStatus)
		}
	}
}
