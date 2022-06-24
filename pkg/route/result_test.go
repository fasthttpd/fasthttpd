package route

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/fasthttpd/fasthttpd/pkg/util"
	"github.com/valyala/fasthttp"
)

func TestResult_RewriteURIWithQueryString(t *testing.T) {
	tests := []struct {
		result     *Result
		requestUri []byte
		want       []byte
	}{
		{
			result: &Result{
				RewriteURI:        []byte("/rewrite-uri"),
				AppendQueryString: false,
			},
			requestUri: []byte("/path?a=1"),
			want:       []byte("/rewrite-uri"),
		}, {
			result: &Result{
				RewriteURI:        []byte("/rewrite-uri"),
				AppendQueryString: true,
			},
			requestUri: []byte("/path?a=1"),
			want:       []byte("/rewrite-uri?a=1"),
		}, {
			result: &Result{
				RewriteURI:        []byte("/rewrite-uri?b=2"),
				AppendQueryString: true,
			},
			requestUri: []byte("/path?a=1"),
			want:       []byte("/rewrite-uri?b=2&a=1"),
		},
	}
	for i, test := range tests {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI(string(test.requestUri))
		got := test.result.RewriteURIWithQueryString(ctx)
		if !bytes.Equal(got, test.want) {
			t.Errorf("tests[%d] unexpected uri %q; want %q", i, got, test.want)
		}
	}
}

func TestResult_RedirectURIWithQueryString(t *testing.T) {
	tests := []struct {
		result     *Result
		requestUri []byte
		want       []byte
	}{
		{
			result: &Result{
				RedirectURI:       []byte("http://example.com/"),
				AppendQueryString: false,
			},
			requestUri: []byte("/path?a=1"),
			want:       []byte("http://example.com/"),
		}, {
			result: &Result{
				RedirectURI:       []byte("http://example.com/"),
				AppendQueryString: true,
			},
			requestUri: []byte("/path?a=1"),
			want:       []byte("http://example.com/?a=1"),
		}, {
			result: &Result{
				RedirectURI:       []byte("http://example.com/?a=1"),
				AppendQueryString: true,
			},
			requestUri: []byte("/path?b=2"),
			want:       []byte("http://example.com/?a=1&b=2"),
		},
	}
	for i, test := range tests {
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI(string(test.requestUri))
		got := test.result.RedirectURIWithQueryString(ctx)
		if !bytes.Equal(got, test.want) {
			t.Errorf("tests[%d] unexpected uri %q; want %q", i, got, test.want)
		}
	}
}

func TestResult_Equal(t *testing.T) {
	fullResult := &Result{
		StatusCode:        http.StatusOK,
		StatusMessage:     []byte(http.StatusText(http.StatusOK)),
		RewriteURI:        []byte("Rewrite-URI"),
		RedirectURI:       []byte("Redirect-URI"),
		AppendQueryString: true,
		Handler:           "default",
		Filters:           util.StringSet{"auth"},
	}
	diffFilterResult := fullResult.CopyTo(&Result{})
	diffFilterResult.Filters = util.StringSet{"no-cache"}
	tests := []struct {
		a    *Result
		b    *Result
		want bool
	}{
		{
			a:    fullResult,
			b:    fullResult.CopyTo(&Result{}),
			want: true,
		}, {
			a: fullResult,
			b: &Result{},
		}, {
			a: fullResult,
			b: diffFilterResult,
		},
	}
	for i, test := range tests {
		got := test.a.Equal(test.b)
		if got != test.want {
			t.Errorf("tests[%d] unexpected equal %v; want %v", i, got, test.want)
		}
	}
}
