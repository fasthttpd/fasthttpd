package handler

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/filter"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/fasthttpd/fasthttpd/pkg/logger/accesslog"
	"github.com/fasthttpd/fasthttpd/pkg/route"
	"github.com/mojatter/tree"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttputil"
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
outer:
	for key, value := range resp.Header.All() {
		strKey := string(key)
		if strKey == "Date" {
			continue
		}
		for j, kv := range headers {
			k, v := kv[0], kv[1]
			if k == strKey {
				if v != string(value) {
					err = fmt.Errorf("unexpected header %s: %s; want %s", k, value, v)
				}
				headers = append(headers[:j], headers[j+1:]...)
				continue outer
			}
		}
		err = fmt.Errorf("unnecessary header %s: %s", key, value)
	}
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

// serveInmemory starts s on an in-memory listener and returns a client dialing it.
func serveInmemory(t *testing.T, s *fasthttp.Server) *fasthttp.Client {
	t.Helper()
	ln := fasthttputil.NewInmemoryListener()
	go s.Serve(ln) //nolint:errcheck // closed by ln.Close
	t.Cleanup(func() { ln.Close() })
	return &fasthttp.Client{Dial: func(string) (net.Conn, error) { return ln.Dial() }}
}

// doPost posts body to host through c, chunked when requested.
func doPost(t *testing.T, c *fasthttp.Client, host, body string, chunked bool) *fasthttp.Response {
	t.Helper()
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)

	req.SetRequestURI("http://" + host + "/")
	req.Header.SetMethod(fasthttp.MethodPost)
	if chunked {
		req.SetBodyStream(strings.NewReader(body), -1)
	} else {
		req.SetBodyString(body)
	}
	resp := &fasthttp.Response{}
	if err := c.Do(req, resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestServerHandler_streamedBodyLimit(t *testing.T) {
	newCfg := func(host string, server config.Server) config.Config {
		return config.Config{
			Host:   host,
			Server: server,
			Routes: []config.Route{{Path: "/", Match: config.MatchPrefix, Status: http.StatusOK}},
		}
	}
	streamed := func(limit config.Size) config.Server {
		return config.Server{StreamRequestBody: true, MaxRequestBodySize: limit}
	}
	testCases := []struct {
		caseName   string
		cfgs       []config.Config
		host       string
		body       string
		chunked    bool
		wantStatus int
		wantClose  bool
		wantLog    bool
	}{
		{
			caseName:   "under limit",
			cfgs:       []config.Config{newCfg("a", streamed(16))},
			host:       "a",
			body:       "0123456789",
			wantStatus: http.StatusOK,
		},
		{
			caseName:   "over limit content-length",
			cfgs:       []config.Config{newCfg("a", streamed(16))},
			host:       "a",
			body:       "0123456789abcdefg",
			wantStatus: http.StatusBadRequest,
			wantClose:  true,
			wantLog:    true,
		},
		{
			caseName:   "over limit chunked",
			cfgs:       []config.Config{newCfg("a", streamed(16))},
			host:       "a",
			body:       "0123456789abcdefg",
			chunked:    true,
			wantStatus: http.StatusBadRequest,
			wantClose:  true,
			wantLog:    true,
		},
		{
			caseName:   "virtual host uses first document limit",
			cfgs:       []config.Config{newCfg("a", streamed(16)), newCfg("b", streamed(1024))},
			host:       "b",
			body:       "0123456789abcdefg",
			wantStatus: http.StatusBadRequest,
			wantClose:  true,
			wantLog:    true,
		},
		{
			caseName:   "virtual host under first document limit",
			cfgs:       []config.Config{newCfg("a", streamed(16)), newCfg("b", streamed(1024))},
			host:       "b",
			body:       "0123456789abcdef",
			wantStatus: http.StatusOK,
		},
		{
			caseName:   "streaming disabled",
			cfgs:       []config.Config{newCfg("a", config.Server{MaxRequestBodySize: 16})},
			host:       "a",
			body:       "0123456789abcdefg",
			wantStatus: http.StatusBadRequest,
			wantClose:  true,
			wantLog:    true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			h, err := NewServerHandler(tc.cfgs)
			if err != nil {
				t.Fatal(err)
			}
			defer h.Close()

			// fasthttp logs its own rejections after the response, so collect lines on a channel.
			logs := make(chan string, 8)
			l := &logger.LoggerDelegator{PrintfFunc: func(format string, args ...any) {
				logs <- fmt.Sprintf(format, args...)
			}}
			s := &fasthttp.Server{Handler: h.Handle, ErrorHandler: h.HandleError, Logger: l}
			h.Config().ApplyTo(s)

			resp := doPost(t, serveInmemory(t, s), tc.host, tc.body, tc.chunked)
			if got := resp.StatusCode(); got != tc.wantStatus {
				t.Errorf("status = %d, want %d", got, tc.wantStatus)
			}
			if got := resp.ConnectionClose(); got != tc.wantClose {
				t.Errorf("connection close = %v, want %v", got, tc.wantClose)
			}
			if tc.wantLog {
				select {
				case <-logs:
				case <-time.After(time.Second):
					t.Error("no error log")
				}
			} else if len(logs) > 0 {
				t.Errorf("unexpected error log %q", <-logs)
			}
		})
	}
}
