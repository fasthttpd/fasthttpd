package handler

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

	for status := errorPagesStatusOffset; status < errorPagesStatusUntil; status++ {
		i := status - errorPagesStatusOffset
		var wantPath string
		switch {
		case status == 400:
			wantPath = "/err/400.html"
		case status == 404:
			wantPath = "/err/404.html"
		case status >= 500:
			wantPath = "/err/5xx.html"
		}
		if got := string(errorPages.errorPaths[i]); got != wantPath {
			t.Errorf("errorPaths[%d] got %q; want %q", status, got, wantPath)
		}
		if got, want := errorPages.missing[i].Load(), status == 400; got != want {
			t.Errorf("missing[%d] got %v; want %v", status, got, want)
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

func TestErrorPages_Concurrent(t *testing.T) {
	log, err := logger.NewLoggerWriter(config.Log{}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	errorPages := NewErrorPages("", map[string]string{
		"root": "testdata/public",
		"400":  "/err/400.html",
		"404":  "/err/404.html",
	})

	testCases := []struct {
		caseName    string
		status      int
		wantBody    string
		wantMissing bool
	}{
		{caseName: "page", status: http.StatusNotFound, wantBody: "Custom 404 Not Found"},
		{caseName: "missing page", status: http.StatusBadRequest, wantBody: "<!DOCTYPE html><html><head><title>400 Bad Request", wantMissing: true},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			var wg sync.WaitGroup
			for range 8 {
				wg.Add(1)
				go func() {
					defer wg.Done()

					ctx := &fasthttp.RequestCtx{}
					ctx.Init(&fasthttp.Request{}, nil, log)
					ctx.Request.SetRequestURI("/missing")
					ctx.Response.SetStatusCode(tc.status)
					errorPages.Handle(ctx)
					if got := ctx.Response.StatusCode(); got != tc.status {
						t.Errorf("statusCode got %d; want %d", got, tc.status)
					}
					if got := string(ctx.Response.Body()); !strings.HasPrefix(got, tc.wantBody) {
						t.Errorf("body got %s; want prefix %s", got, tc.wantBody)
					}
				}()
			}
			wg.Wait()
			if got := errorPages.missing[tc.status-errorPagesStatusOffset].Load(); got != tc.wantMissing {
				t.Errorf("missing got %v; want %v", got, tc.wantMissing)
			}
		})
	}
}

func TestLookupErrorPath(t *testing.T) {
	all := map[string]string{"404": "/404", "40x": "/40x", "4xx": "/4xx", "xxx": "/xxx"}
	testCases := []struct {
		caseName     string
		statusToPath map[string]string
		status       int
		want         string
		wantNil      bool
	}{
		{caseName: "exact", statusToPath: all, status: 404, want: "/404"},
		{caseName: "40x", statusToPath: all, status: 401, want: "/40x"},
		{caseName: "4xx", statusToPath: all, status: 410, want: "/4xx"},
		{caseName: "xxx", statusToPath: all, status: 500, want: "/xxx"},
		{caseName: "empty value falls through", statusToPath: map[string]string{"404": "", "40x": "/40x"}, status: 404, want: "/40x"},
		{caseName: "no match", statusToPath: map[string]string{"root": "/public", "5xx": "/5xx"}, status: 404, wantNil: true},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			got := lookupErrorPath(tc.statusToPath, tc.status)
			if (got == nil) != tc.wantNil {
				t.Fatalf("got %q; want nil %v", got, tc.wantNil)
			}
			if string(got) != tc.want {
				t.Errorf("got %q; want %q", got, tc.want)
			}
		})
	}
}
