package route

import (
	"bytes"
	"sync"

	"github.com/fasthttpd/fasthttpd/pkg/util"
	"github.com/valyala/fasthttp"
)

var resultPool sync.Pool

// AcquireResult returns an empty Result object from the pool.
//
// The returned Result may be returned to the pool with Release when no
// longer needed. This allows reducing GC load.
func AcquireResult() *Result {
	x := resultPool.Get()
	if x != nil {
		return x.(*Result)
	}
	return &Result{}
}

// Result represents a result of routing.
type Result struct {
	StatusCode        int
	StatusMessage     []byte
	RewriteURI        []byte
	RedirectURI       []byte
	AppendQueryString bool
	Handler           string
	Filters           util.StringSet
	RouteIndex        int
}

// RewriteURIWithQueryString returns r.RewriteURI with queryString.
func (r *Result) RewriteURIWithQueryString(ctx *fasthttp.RequestCtx) []byte {
	if r.AppendQueryString && len(r.RewriteURI) > 0 {
		return util.AppendQueryString(r.RewriteURI, ctx.URI().QueryString())
	}
	return r.RewriteURI
}

// RedirectURIWithQueryString returns r.RedirectURI with queryString.
func (r *Result) RedirectURIWithQueryString(ctx *fasthttp.RequestCtx) []byte {
	if r.AppendQueryString && len(r.RedirectURI) > 0 {
		return util.AppendQueryString(r.RedirectURI, ctx.URI().QueryString())
	}
	return r.RedirectURI
}

// Reset clears result.
func (r *Result) Reset() {
	r.StatusCode = 0
	r.StatusMessage = r.StatusMessage[:0]
	r.RewriteURI = r.RewriteURI[:0]
	r.RedirectURI = r.RedirectURI[:0]
	r.AppendQueryString = false
	r.Handler = ""
	r.Filters = r.Filters[:0]
	r.RouteIndex = 0
}

// CopyTo copies all the result to dst.
func (r *Result) CopyTo(dst *Result) *Result {
	dst.Reset()
	dst.StatusCode = r.StatusCode
	dst.StatusMessage = append(dst.StatusMessage[:0], r.StatusMessage...)
	dst.RewriteURI = append(dst.RewriteURI[:0], r.RewriteURI...)
	dst.RedirectURI = append(dst.RedirectURI, r.RedirectURI...)
	dst.AppendQueryString = r.AppendQueryString
	dst.Handler = r.Handler
	dst.Filters = append(dst.Filters[:0], r.Filters...)
	dst.RouteIndex = r.RouteIndex
	return dst
}

// Equal reports whether a and b.
func (a *Result) Equal(b *Result) bool {
	if len(a.Filters) != len(b.Filters) {
		return false
	}
	for i, f := range a.Filters {
		if f != b.Filters[i] {
			return false
		}
	}
	return a.StatusCode == b.StatusCode &&
		bytes.Equal(a.StatusMessage, b.StatusMessage) &&
		bytes.Equal(a.RewriteURI, b.RewriteURI) &&
		bytes.Equal(a.RedirectURI, b.RedirectURI) &&
		a.AppendQueryString == b.AppendQueryString &&
		a.Handler == b.Handler
}

// Release returns the object acquired via AcquireResult to the pool.
//
// Do not access the released Result object, otherwise data races may occur.
func (r *Result) Release() {
	r.Reset()
	resultPool.Put(r)
}
