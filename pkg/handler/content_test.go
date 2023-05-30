package handler

import (
	"reflect"
	"testing"

	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
)

func TestNewContent(t *testing.T) {
	tests := []struct {
		cfg  tree.Map
		want *Content
	}{
		{
			cfg: tree.Map{
				"headers": tree.Map{"Content-Type": tree.ToValue("text/plain")},
				"body":    tree.ToValue("Hello"),
			},
			want: &Content{
				handlerCfg: tree.Map{
					"headers": tree.Map{"Content-Type": tree.ToValue("text/plain")},
					"body":    tree.ToValue("Hello"),
				},
			},
		},
	}
	for i, test := range tests {
		got, err := NewContent(test.cfg)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("tests[%d] unexpected content %v; want %v", i, got, test.want)
		}
	}
}

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
