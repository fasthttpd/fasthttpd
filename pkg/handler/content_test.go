package handler

import (
	"strings"
	"testing"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/mojatter/tree"
	"github.com/valyala/fasthttp"
)

func TestContent_Handle(t *testing.T) {
	tests := []struct {
		setup      func() func()
		cfg        tree.Map
		requestURI string
		want       func() *fasthttp.RequestCtx
	}{
		{
			setup: func() func() {
				return func() {}
			},
			cfg: tree.Map{
				"headers": tree.Map{"Content-Type": tree.ToValue("text/plain")},
				"body":    tree.ToValue("hello"),
			},
			want: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.Response.Header.Set("Content-Type", "text/plain")
				ctx.Response.SetBodyString("hello")
				return ctx
			},
		}, {
			setup: func() func() {
				return func() {}
			},
			cfg: tree.Map{
				"headers": tree.Map{"Location": tree.ToValue("http://example.com/")},
				"status":  tree.ToValue(fasthttp.StatusMovedPermanently),
			},
			want: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.Response.Header.Set("Location", "http://example.com/")
				ctx.Response.SetStatusCode(fasthttp.StatusMovedPermanently)
				return ctx
			},
		}, {
			setup: func() func() {
				return func() {}
			},
			cfg: tree.Map{
				"headers": tree.Array{
					tree.ToValue("Header: value1"),
					tree.Map{"Header": tree.ToValue("value2")},
				},
				"body": tree.ToValue("hello"),
			},
			want: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.Response.Header.Add("Header", "value1")
				ctx.Response.Header.Add("Header", "value2")
				ctx.Response.SetBodyString("hello")
				return ctx
			},
		}, {
			setup: func() func() {
				return func() {}
			},
			cfg: tree.Map{
				"body":    tree.ToValue("hello"),
				"headers": tree.Map{"Content-Type": tree.ToValue("text/plain")},
				"conditions": tree.Array{
					tree.Map{
						"path": tree.ToValue("/good-morning"),
						"body": tree.ToValue("good morning"),
					},
				},
			},
			requestURI: "/good-morning",
			want: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.Response.Header.Set("Content-Type", "text/plain")
				ctx.Response.SetBodyString("good morning")
				return ctx
			},
		}, {
			setup: func() func() {
				return func() {}
			},
			cfg: tree.Map{
				"body": tree.ToValue("queryString no match"),
				"conditions": tree.Array{
					tree.Map{
						"queryStringContains": tree.ToValue("a=1&c=3"),
						"body":                tree.ToValue("queryString match"),
					},
				},
			},
			requestURI: "/hello?a=1&b=2&c=3",
			want: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.Response.SetBodyString("queryString match")
				return ctx
			},
		}, {
			setup: func() func() {
				return func() {}
			},
			cfg: tree.Map{
				"body": tree.ToValue("queryString no match"),
				"conditions": tree.Array{
					tree.Map{
						"queryStringContains": tree.ToValue("a=1&c=3"),
						"body":                tree.ToValue("queryString match"),
					},
				},
			},
			requestURI: "/hello?a=0&b=2&c=3",
			want: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.Response.SetBodyString("queryString no match")
				return ctx
			},
		}, {
			setup: func() func() {
				contentRandomPercentageOrg := contentRandomPercentage
				contentRandomPercentage = func() int {
					return 10
				}
				return func() { contentRandomPercentage = contentRandomPercentageOrg }
			},
			cfg: tree.Map{
				"body": tree.ToValue("no hit"),
				"conditions": tree.Array{
					tree.Map{
						"percentage": tree.ToValue(10),
						"body":       tree.ToValue("hit"),
					},
				},
			},
			requestURI: "/hello",
			want: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.Response.SetBodyString("hit")
				return ctx
			},
		}, {
			setup: func() func() {
				contentRandomPercentageOrg := contentRandomPercentage
				contentRandomPercentage = func() int {
					return 20
				}
				return func() { contentRandomPercentage = contentRandomPercentageOrg }
			},
			cfg: tree.Map{
				"body": tree.ToValue("no hit"),
				"conditions": tree.Array{
					tree.Map{
						"percentage": tree.ToValue(10),
						"body":       tree.ToValue("hit"),
					},
				},
			},
			requestURI: "/hello",
			want: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.Response.SetBodyString("no hit")
				return ctx
			},
		},
	}
	for i, test := range tests {
		func() {
			done := test.setup()
			defer done()

			fn, err := NewContentHandler(test.cfg, logger.NilLogger)
			if err != nil {
				t.Fatalf("tests[%d] unexpected error %v", i, err)
			}
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI(test.requestURI)
			fn(ctx)

			got := ctx.Response.String()
			want := test.want().Response.String()
			if got != want {
				t.Errorf("tests[%d] unexpected content %q; want %q", i, got, want)
			}
		}()
	}
}

// newBenchContentHandler builds a Content handler backed by the supplied
// config and asserts it constructs cleanly.
func newBenchContentHandler(b *testing.B, cfg tree.Map) fasthttp.RequestHandler {
	b.Helper()
	fn, err := NewContentHandler(cfg, logger.NilLogger)
	if err != nil {
		b.Fatalf("NewContentHandler: %v", err)
	}
	return fn
}

// Each iteration resets the response and re-seeds the request URI before
// invoking the handler, so repeated Set/Add calls do not accumulate state
// across iterations.

func BenchmarkContent_Handle_Unconditional(b *testing.B) {
	fn := newBenchContentHandler(b, tree.Map{
		"headers": tree.Map{
			"Content-Type": tree.ToValue("text/plain"),
		},
		"body": tree.ToValue("hello"),
	})
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/hello")

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		ctx.Response.Reset()
		fn(ctx)
	}
}

func BenchmarkContent_Handle_PathCondition(b *testing.B) {
	fn := newBenchContentHandler(b, tree.Map{
		"body": tree.ToValue("default"),
		"conditions": tree.Array{
			tree.Map{
				"path": tree.ToValue("/good-morning"),
				"body": tree.ToValue("good morning"),
			},
		},
	})
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/good-morning")

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		ctx.Response.Reset()
		fn(ctx)
	}
}

func BenchmarkContent_Handle_QueryCondition(b *testing.B) {
	fn := newBenchContentHandler(b, tree.Map{
		"body": tree.ToValue("default"),
		"conditions": tree.Array{
			tree.Map{
				"queryStringContains": tree.ToValue("a=1&c=3"),
				"body":                tree.ToValue("matched"),
			},
		},
	})
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/hello?a=1&b=2&c=3")

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		ctx.Response.Reset()
		fn(ctx)
	}
}

func TestContent_SchemaRegistered(t *testing.T) {
	testCases := []struct {
		caseName string
		handler  tree.Map
		wantErr  string
	}{
		{
			caseName: "valid content",
			handler: tree.Map{
				"type":   tree.V("content"),
				"body":   tree.V("hi"),
				"status": tree.V(200),
				"headers": tree.Map{
					"Content-Type": tree.V("text/plain"),
				},
			},
		},
		{
			caseName: "conditions with path",
			handler: tree.Map{
				"type": tree.V("content"),
				"conditions": tree.Array{
					tree.Map{"path": tree.V("/x"), "body": tree.V("x")},
				},
			},
		},
		{
			caseName: "unknown content field",
			handler: tree.Map{
				"type":  tree.V("content"),
				"bogus": tree.V(1),
			},
			wantErr: ".bogus: unknown key",
		},
		{
			caseName: "status out of range",
			handler: tree.Map{
				"type":   tree.V("content"),
				"status": tree.V(999),
			},
			wantErr: "status",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			docs := []tree.Map{{"handlers": tree.Map{"c": tc.handler}}}
			err := config.Validate(docs)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate returned %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate returned nil, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}
