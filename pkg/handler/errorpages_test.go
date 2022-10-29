package handler

import (
	"bytes"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
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
