package handler

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/mojatter/tree"
	"github.com/valyala/fasthttp"
)

func TestErrorPages(t *testing.T) {
	logBuf := new(bytes.Buffer)
	logger, err := logger.NewLoggerWriter(config.Log{}, logBuf)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &fasthttp.RequestCtx{}

	errorPages := NewErrorPages("", map[string]string{
		"root": "testdata/public",
		"400":  "/err/400.html",
		"404":  "/err/404.html",
		"5xx":  "/err/5xx.html",
	})

	tests := []struct {
		handle          func()
		wantStatusCode  int
		wantContentType string
		wantBody        string
		wantLogOutput   string
	}{
		{
			handle: func() {
				ctx.Request.Header.SetMethod(http.MethodGet)
				ctx.Request.SetRequestURI("/forbidden.html")
				ctx.Response.SetStatusCode(403)
				ctx.Response.Header.SetContentType("text/html; charset=utf-8")
				ctx.Response.SetBody([]byte("forbidden"))
			},
			wantStatusCode:  403,
			wantContentType: "text/html; charset=utf-8",
			wantBody:        "forbidden",
		}, {
			handle: func() {
				ctx.Request.Header.SetMethod(http.MethodGet)
				ctx.Request.SetRequestURI("/not-found.html")
				ctx.Response.SetStatusCode(404)
				ctx.Response.Header.SetContentType("text/html; charset=utf-8")
				ctx.Response.SetBody([]byte("not found"))
			},
			wantStatusCode:  404,
			wantContentType: "text/html; charset=utf-8",
			wantBody:        "Custom 404 Not Found",
		}, {
			handle: func() {
				ctx.Request.Header.SetMethod(http.MethodGet)
				ctx.Request.SetRequestURI("/internal-server-error.html")
				ctx.Response.SetStatusCode(500)
				ctx.Response.Header.SetContentType("text/html; charset=utf-8")
				ctx.Response.SetBody([]byte("internal server error"))
			},
			wantStatusCode:  500,
			wantContentType: "text/html; charset=utf-8",
			wantBody:        "Custom 5xx Error",
		}, {
			handle: func() {
				ctx.Request.Header.SetMethod(http.MethodGet)
				ctx.Request.SetRequestURI("/bad-gateway.html")
				ctx.Response.SetStatusCode(502)
				ctx.Response.Header.SetContentType("text/html; charset=utf-8")
				ctx.Response.SetBody([]byte("bad gateway"))
			},
			wantStatusCode:  502,
			wantContentType: "text/html; charset=utf-8",
			wantBody:        "Custom 5xx Error",
		}, {
			handle: func() {
				ctx.Request.Header.SetMethod(http.MethodGet)
				ctx.Request.SetRequestURI("/bad-request.html")
				ctx.Response.SetStatusCode(400)
			},
			wantStatusCode:  400,
			wantContentType: "text/html; charset=utf-8",
			wantBody:        `<!DOCTYPE html><html><head><title>400 Bad Request</title><style>h1,p { text-align: center; }</style></head><body><h1>400 Bad Request</h1></body></html>`,
			wantLogOutput:   `invalid ErrorPages.fs status 404 on "/err/400.html"`,
		},
	}

	for i, test := range tests {
		logBuf.Reset()
		ctx.Request.Reset()
		ctx.Response.Reset()
		ctx.Init(&ctx.Request, nil, logger)

		test.handle()
		errorPages.Handle(ctx)

		if got := ctx.Response.StatusCode(); got != test.wantStatusCode {
			t.Errorf("tests[%d] statusCode got %d; want %d", i, got, test.wantStatusCode)
		}
		if got := string(ctx.Response.Header.ContentType()); got != test.wantContentType {
			t.Errorf("tests[%d] contentType got %s; want %s", i, got, test.wantContentType)
		}
		if got := string(ctx.Response.Body()); got != test.wantBody {
			t.Errorf("tests[%d] body got %s; want %s", i, got, test.wantBody)
		}
		if got := strings.TrimSpace(logBuf.String()); !strings.HasSuffix(got, test.wantLogOutput) {
			t.Errorf("tests[%d] logOutput got %s; want %s", i, got, test.wantLogOutput)
		}
	}

	wantErrorPaths := make([][]byte, 200)
	wantErrorPaths[400-errorPagesStatusOffset] = []byte{'-'}
	wantErrorPaths[403-errorPagesStatusOffset] = []byte{}
	wantErrorPaths[404-errorPagesStatusOffset] = []byte("/err/404.html")
	wantErrorPaths[500-errorPagesStatusOffset] = []byte("/err/5xx.html")
	wantErrorPaths[502-errorPagesStatusOffset] = []byte("/err/5xx.html")

	if !reflect.DeepEqual(errorPages.errorPaths, wantErrorPaths) {
		t.Errorf("errorPaths got %v; want %v", errorPages.errorPaths, wantErrorPaths)
		for status, errPath := range errorPages.errorPaths {
			if errPath != nil {
				t.Errorf("errorPaths[%d] %s", status, errPath)
			}
		}
	}
}

// unreadReader fails the test when read.
type unreadReader struct{ t *testing.T }

func (r unreadReader) Read([]byte) (int, error) {
	r.t.Error("body stream read")
	return 0, io.EOF
}

func TestSendDefaultError(t *testing.T) {
	testCases := []struct {
		caseName   string
		setBody    func(t *testing.T, ctx *fasthttp.RequestCtx)
		wantStream bool
		wantBody   string
	}{
		{
			caseName: "no body",
			setBody:  func(*testing.T, *fasthttp.RequestCtx) {},
			wantBody: `<!DOCTYPE html><html><head><title>404 Not Found</title><style>h1,p { text-align: center; }</style></head><body><h1>404 Not Found</h1></body></html>`,
		}, {
			caseName: "body",
			setBody:  func(_ *testing.T, ctx *fasthttp.RequestCtx) { ctx.Response.SetBodyString("gone") },
			wantBody: "gone",
		}, {
			caseName:   "body stream",
			setBody:    func(t *testing.T, ctx *fasthttp.RequestCtx) { ctx.Response.SetBodyStream(unreadReader{t}, -1) },
			wantStream: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}
			ctx.Response.SetStatusCode(http.StatusNotFound)
			tc.setBody(t, ctx)
			SendDefaultError(ctx)

			if got := ctx.Response.IsBodyStream(); got != tc.wantStream {
				t.Fatalf("IsBodyStream got %v; want %v", got, tc.wantStream)
			}
			if tc.wantStream {
				return
			}
			if got := string(ctx.Response.Body()); got != tc.wantBody {
				t.Errorf("body got %s; want %s", got, tc.wantBody)
			}
		})
	}
}

func TestErrorPages_StreamedProxyError(t *testing.T) {
	release := make(chan struct{})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("first\n")) //nolint:errcheck // proxy reads it
		w.(http.Flusher).Flush()
		// Idle until the test ends; the client must not wait for this.
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer backend.Close()

	defer close(release)

	proxy, err := NewProxyHandler(tree.Map{"url": tree.ToValue(backend.URL)}, logger.NilLogger)
	if err != nil {
		t.Fatal(err)
	}
	errorPages := &ErrorPages{}
	streamAndDisconnect(t, func(ctx *fasthttp.RequestCtx) {
		proxy(ctx)
		errorPages.Handle(ctx)
	}, "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
}
