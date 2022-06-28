package handler

import (
	"net/http"
	"strconv"

	"github.com/valyala/bytebufferpool"
	"github.com/valyala/fasthttp"
)

var (
	errorPagesStatusOffset = 400
	errorPagesStatusUntil  = 600
)

// ErrorPages represents a handler that provides custom error pages.
type ErrorPages struct {
	root         string
	statusToPath map[string]string
	fs           fasthttp.RequestHandler
	errorPaths   [][]byte
}

var _ fasthttp.RequestHandler = (*ErrorPages)(nil).Handle

// NewErrorPages creates a new ErrorPages.
// The statusToPath maps from a http status text to a path of error page.
// A http status text can be contain 'x' as wildcard. (eg. '404', '40x')
func NewErrorPages(root string, statusToPath map[string]string) *ErrorPages {
	if len(root) == 0 || len(statusToPath) == 0 {
		return &ErrorPages{}
	}
	fs := &fasthttp.FS{
		Root:     root,
		Compress: true,
	}
	return &ErrorPages{
		root:         root,
		statusToPath: statusToPath,
		fs:           fs.NewRequestHandler(),
		errorPaths:   make([][]byte, errorPagesStatusUntil-errorPagesStatusOffset),
	}
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
	if path := p.errorPaths[status-errorPagesStatusOffset]; path != nil {
		p.sendError(ctx, path)
		return
	}
	sb := []byte(strconv.Itoa(status))
	p.handleStatus(ctx, sb, len(sb))
}

func (p *ErrorPages) handleStatus(ctx *fasthttp.RequestCtx, sb []byte, l int) {
	status := ctx.Response.StatusCode()
	if l < 0 {
		// NOTE: store no erorr page.
		p.errorPaths[status-errorPagesStatusOffset] = []byte{}
		SendDefaultError(ctx)
		return
	}
	if l < len(sb) {
		sb[l] = 'x'
	}
	if page := p.statusToPath[string(sb)]; page != "" {
		path := []byte(page)
		p.errorPaths[status-errorPagesStatusOffset] = path
		p.sendError(ctx, path)
		return
	}
	p.handleStatus(ctx, sb, l-1)
}

func (p *ErrorPages) sendError(ctx *fasthttp.RequestCtx, path []byte) {
	if len(path) == 0 {
		SendDefaultError(ctx)
		return
	}

	status := ctx.Response.StatusCode()
	uri := string(ctx.Request.RequestURI())

	ctx.Request.SetRequestURIBytes(path)
	p.fs(ctx)
	statusFS := ctx.Response.StatusCode()

	ctx.Request.SetRequestURI(uri)
	ctx.SetStatusCode(status)

	if statusFS != http.StatusOK {
		// NOTE: store no erorr page.
		p.errorPaths[status-errorPagesStatusOffset] = []byte{}
		ctx.Logger().Printf("invalid ErrorPages.fs status %d on %q", statusFS, path)

		ctx.Response.SetBody([]byte{})
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
// If the provided ctx has response body, it does nothing.
func SendDefaultError(ctx *fasthttp.RequestCtx) {
	if len(ctx.Response.Body()) > 0 {
		return
	}

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
