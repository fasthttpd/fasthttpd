package handler

import (
	"net/http"
	"strconv"
	"sync/atomic"

	"github.com/valyala/bytebufferpool"
	"github.com/valyala/fasthttp"
)

var (
	errorPagesStatusOffset = 400
	errorPagesStatusUntil  = 600
)

// ErrorPages represents a handler that provides custom error pages.
type ErrorPages struct {
	fs fasthttp.RequestHandler
	// errorPaths is indexed by status-errorPagesStatusOffset and read-only after NewErrorPages.
	errorPaths [][]byte
	// missing marks pages that fasthttp.FS did not find.
	missing []atomic.Bool
}

var _ fasthttp.RequestHandler = (*ErrorPages)(nil).Handle

// NewErrorPages creates a new ErrorPages.
// The statusToPath maps from a http status text to a path of error page.
// A http status text can be contain 'x' as wildcard. (eg. '404', '40x')
// If 'root' is specified in statusToPath, it overrides the default root.
func NewErrorPages(root string, statusToPath map[string]string) *ErrorPages {
	if overrideRoot, ok := statusToPath["root"]; ok {
		root = overrideRoot
	}
	if len(statusToPath) == 0 {
		return &ErrorPages{}
	}
	fs := &fasthttp.FS{
		Root:     root,
		Compress: true,
	}
	n := errorPagesStatusUntil - errorPagesStatusOffset
	errorPaths := make([][]byte, n)
	for i := range errorPaths {
		errorPaths[i] = lookupErrorPath(statusToPath, errorPagesStatusOffset+i)
	}
	return &ErrorPages{
		fs:         fs.NewRequestHandler(),
		errorPaths: errorPaths,
		missing:    make([]atomic.Bool, n),
	}
}

// lookupErrorPath returns the page for status, trying e.g. "404", "40x", "4xx" and "xxx" in order.
func lookupErrorPath(statusToPath map[string]string, status int) []byte {
	sb := []byte(strconv.Itoa(status))
	for l := len(sb); l >= 0; l-- {
		if l < len(sb) {
			sb[l] = 'x'
		}
		if page := statusToPath[string(sb)]; page != "" {
			return []byte(page)
		}
	}
	return nil
}

func (p *ErrorPages) Handle(ctx *fasthttp.RequestCtx) {
	status := ctx.Response.StatusCode()
	if status < errorPagesStatusOffset || status >= errorPagesStatusUntil {
		return
	}
	if p.fs == nil {
		SendDefaultError(ctx)
		return
	}
	i := status - errorPagesStatusOffset
	path := p.errorPaths[i]
	if len(path) == 0 {
		SendDefaultError(ctx)
		return
	}
	if p.missing[i].Load() {
		ctx.Response.SetBody(nil)
		SendDefaultError(ctx)
		return
	}
	p.sendError(ctx, i, path)
}

// fsRequestHeaders can make fasthttp.FS answer other than 200 with the page (#116).
var fsRequestHeaders = [...]string{
	fasthttp.HeaderIfModifiedSince,
	fasthttp.HeaderIfNoneMatch,
	fasthttp.HeaderRange,
	fasthttp.HeaderIfRange,
}

// entityHeaders describe the body of the response that an error page replaces (#120).
var entityHeaders = []string{
	fasthttp.HeaderContentType,
	fasthttp.HeaderContentEncoding,
	fasthttp.HeaderContentLanguage,
	fasthttp.HeaderContentLocation,
	fasthttp.HeaderContentDisposition,
	fasthttp.HeaderContentRange,
	fasthttp.HeaderETag,
	fasthttp.HeaderLastModified,
}

func delEntityHeaders(h *fasthttp.ResponseHeader) {
	for _, k := range entityHeaders {
		h.Del(k)
	}
}

func (p *ErrorPages) sendError(ctx *fasthttp.RequestCtx, i int, path []byte) {
	status := ctx.Response.StatusCode()
	uri := string(ctx.Request.RequestURI())
	var saved [len(fsRequestHeaders)][]byte
	for j, k := range fsRequestHeaders {
		if v := ctx.Request.Header.Peek(k); v != nil {
			saved[j] = append([]byte{}, v...)
			ctx.Request.Header.Del(k)
		}
	}

	delEntityHeaders(&ctx.Response.Header)
	ctx.Request.SetRequestURIBytes(path)
	p.fs(ctx)
	statusFS := ctx.Response.StatusCode()

	ctx.Request.SetRequestURI(uri)
	for j, k := range fsRequestHeaders {
		if saved[j] != nil {
			ctx.Request.Header.SetBytesV(k, saved[j])
		}
	}
	ctx.SetStatusCode(status)

	if statusFS != http.StatusOK {
		if statusFS == http.StatusNotFound {
			p.missing[i].Store(true)
		}
		ctx.Logger().Printf("invalid ErrorPages.fs status %d on %q", statusFS, path)

		ctx.Response.SetBody(nil)
		SendDefaultError(ctx)
	}
}

var (
	defaultErrorContentType = []byte("text/html; charset=utf-8")
	defaultErrorHtmls       = [][]byte{
		[]byte(`<!DOCTYPE html><html><head><title>`),
		nil,
		[]byte(`</title><style>h1,p { text-align: center; }</style></head><body><h1>`),
		nil,
		[]byte(`</h1></body></html>`),
	}
)

// SendDefaultError sends default error page using ctx.Response.StatusCode().
// It works as fasthttp.RequestHandler.
// If the provided ctx has response body or body stream, it does nothing.
func SendDefaultError(ctx *fasthttp.RequestCtx) {
	// Body() would read a body stream to the end, e.g. a streamed proxy response (#113).
	if ctx.Response.IsBodyStream() || len(ctx.Response.Body()) > 0 {
		return
	}

	delEntityHeaders(&ctx.Response.Header)
	status := ctx.Response.StatusCode()
	b := bytebufferpool.Get()
	for _, h := range defaultErrorHtmls {
		if len(h) == 0 {
			b.B = fasthttp.AppendUint(b.B, status)
			b.B = append(b.B, ' ')
			b.B = append(b.B, http.StatusText(status)...)
			continue
		}
		b.B = append(b.B, h...)
	}
	ctx.Response.Header.SetContentTypeBytes(defaultErrorContentType)
	ctx.Response.SetBody(b.B)
	bytebufferpool.Put(b)
}

var _ fasthttp.RequestHandler = SendDefaultError
