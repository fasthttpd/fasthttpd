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
		cfg    tree.Map
		want   *Content
		errstr string
	}{
		{
			cfg: tree.Map{
				"headers": tree.Map{"Content-Type": tree.ToValue("text/plain")},
				"body":    tree.ToValue("Hello"),
			},
			want: &Content{
				headerKeys:   [][]byte{[]byte("Content-Type")},
				headerValues: [][]byte{[]byte("text/plain")},
				body:         []byte("Hello"),
			},
		}, {
			cfg: tree.Map{
				"headers": tree.Array{tree.ToValue("Content-Type: text/plain")},
				"body":    tree.ToValue("Hello"),
			},
			want: &Content{
				headerKeys:   [][]byte{[]byte("Content-Type")},
				headerValues: [][]byte{[]byte("text/plain")},
				body:         []byte("Hello"),
			},
		}, {
			cfg: tree.Map{
				"headers": tree.Array{tree.Map{"Content-Type": tree.ToValue("text/plain")}},
				"body":    tree.ToValue("Hello"),
			},
			want: &Content{
				headerKeys:   [][]byte{[]byte("Content-Type")},
				headerValues: [][]byte{[]byte("text/plain")},
				body:         []byte("Hello"),
			},
		}, {
			cfg: tree.Map{
				"headers": tree.Array{tree.ToValue("Invalid-Header")},
				"body":    tree.ToValue("Hello"),
			},
			errstr: `invalid header format "Invalid-Header"`,
		},
	}
	for i, test := range tests {
		got, err := NewContent(test.cfg)
		if test.errstr != "" {
			if err == nil {
				t.Fatalf("tests[%d] unexpected no error", i)
			}
			if err.Error() != test.errstr {
				t.Errorf("tests[%d] unexpected error: %q; want %q", i, err.Error(), test.errstr)
			}
			continue
		}
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
		cfg  tree.Map
		want func() *fasthttp.RequestCtx
	}{
		{
			cfg: tree.Map{
				"headers": tree.Map{"Content-Type": tree.ToValue("text/plain")},
				"body":    tree.ToValue("Hello"),
			},
			want: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.Response.Header.Set("Content-Type", "text/plain")
				ctx.Response.SetBodyString("Hello")
				return ctx
			},
		}, {
			cfg: tree.Map{
				"headers": tree.Array{
					tree.ToValue("Header: Value1"),
					tree.ToValue("Header: Value2"),
				},
				"body": tree.ToValue("Hello"),
			},
			want: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.Response.Header.Set("Header", "Value1")
				ctx.Response.Header.Set("Header", "Value2")
				ctx.Response.SetBodyString("Hello")
				return ctx
			},
		},
	}
	for i, test := range tests {
		fn, err := NewContentHandler(test.cfg, logger.NilLogger)
		if err != nil {
			t.Fatalf("tests[%d] unexpected error %v", i, err)
		}
		ctx := &fasthttp.RequestCtx{}
		fn(ctx)

		got := ctx.String()
		want := test.want().String()
		if got != want {
			t.Errorf("tests[%d] unexpected content %q; want %q", i, got, want)
		}
	}
}
