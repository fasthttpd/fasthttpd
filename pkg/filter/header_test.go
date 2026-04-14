package filter

import (
	"bytes"
	"testing"

	"github.com/mojatter/tree"
	"github.com/valyala/fasthttp"
)

func TestHeaderFilter(t *testing.T) {
	tests := []struct {
		cfg  tree.Map
		got  func() *fasthttp.RequestCtx
		want func() *fasthttp.RequestCtx
	}{
		{
			cfg: tree.Map{
				"response": tree.Map{
					"set": tree.Map{
						"Cache-Control": tree.ToValue("private, max-age=3600"),
					},
				},
			},
			got: func() *fasthttp.RequestCtx {
				return &fasthttp.RequestCtx{}
			},
			want: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.Response.Header.Set("Cache-Control", "private, max-age=3600")
				return ctx
			},
		}, {
			cfg: tree.Map{
				"response": tree.Map{
					"add": tree.Map{
						"Cache-Control": tree.ToValue("private, max-age=3600"),
					},
				},
			},
			got: func() *fasthttp.RequestCtx {
				return &fasthttp.RequestCtx{}
			},
			want: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.Response.Header.Set("Cache-Control", "private, max-age=3600")
				return ctx
			},
		}, {
			cfg: tree.Map{
				"response": tree.Map{
					"add": tree.Map{
						"Cache-Control": tree.ToValue("max-age=3600"),
					},
				},
			},
			got: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.Response.Header.Set("Cache-Control", "private")
				return ctx
			},
			want: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.Response.Header.Set("Cache-Control", "private")
				ctx.Response.Header.Add("Cache-Control", "max-age=3600")
				return ctx
			},
		}, {
			cfg: tree.Map{
				"response": tree.Map{
					"del": tree.ToArrayValues("Cache-Control"),
				},
			},
			got: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.Response.Header.Set("Cache-Control", "private")
				return ctx
			},
			want: func() *fasthttp.RequestCtx {
				return &fasthttp.RequestCtx{}
			},
		},
	}
	for i, test := range tests {
		f, err := NewHeaderFilter(test.cfg)
		if err != nil {
			t.Fatalf("tests[%d] unexpected error: %v", i, err)
		}
		got := test.got()
		if !f.Request(got) {
			t.Errorf("tests[%d] request returns false", i)
		}
		if !f.Response(got) {
			t.Errorf("tests[%d] response returns false", i)
		}
		want := test.want()
		gotHeader := bytes.TrimSpace(got.Request.Header.Header())
		wantHeader := bytes.TrimSpace(want.Request.Header.Header())
		if !bytes.Equal(gotHeader, wantHeader) {
			t.Errorf("tests[%d] unexpected request:\n%s", i, gotHeader)
			t.Errorf("want:\n%s", wantHeader)
		}
		gotHeader = bytes.TrimSpace(got.Response.Header.Header())
		wantHeader = bytes.TrimSpace(want.Response.Header.Header())
		if !bytes.Equal(gotHeader, wantHeader) {
			t.Errorf("tests[%d] unexpected response:\n%s", i, gotHeader)
			t.Errorf("want:\n%s", wantHeader)
		}
	}
}

// newBenchHeaderFilter returns a HeaderFilter covering the mutation shapes
// we want to measure for allocations: set, add and del on both the request
// and response sides.
func newBenchHeaderFilter(b *testing.B) Filter {
	b.Helper()
	f, err := NewHeaderFilter(tree.Map{
		"request": tree.Map{
			"set": tree.Map{
				"X-Request-ID": tree.ToValue("bench"),
			},
			"del": tree.ToArrayValues("X-Forwarded-For"),
		},
		"response": tree.Map{
			"set": tree.Map{
				"Cache-Control": tree.ToValue("private, max-age=3600"),
			},
			"add": tree.Map{
				"X-Frame-Options": tree.ToValue("DENY"),
			},
			"del": tree.ToArrayValues("Server"),
		},
	})
	if err != nil {
		b.Fatalf("NewHeaderFilter: %v", err)
	}
	return f
}

// Each iteration below resets the per-ctx header before invoking the
// filter so that repeated Add mutations do not accumulate across
// iterations. This mirrors production, where every request owns a fresh
// RequestCtx.

func BenchmarkHeaderFilter_Request(b *testing.B) {
	f := newBenchHeaderFilter(b)
	ctx := &fasthttp.RequestCtx{}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		ctx.Request.Header.Reset()
		ctx.Request.Header.Set("X-Forwarded-For", "10.0.0.1")
		f.Request(ctx)
	}
}

func BenchmarkHeaderFilter_Response(b *testing.B) {
	f := newBenchHeaderFilter(b)
	ctx := &fasthttp.RequestCtx{}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		ctx.Response.Header.Reset()
		ctx.Response.Header.Set("Server", "fasthttpd")
		f.Response(ctx)
	}
}
