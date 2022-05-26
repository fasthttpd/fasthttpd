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

type ErrorPages struct {
	root       string
	cfg        map[string]string
	fs         fasthttp.RequestHandler
	errorPaths [][]byte
}

func NewErrorPages(root string, cfg map[string]string) *ErrorPages {
	if len(root) == 0 || len(cfg) == 0 {
		return &ErrorPages{}
	}
	fs := &fasthttp.FS{
		Root:     root,
		Compress: true,
	}
	return &ErrorPages{
		root:       root,
		cfg:        cfg,
		fs:         fs.NewRequestHandler(),
		errorPaths: make([][]byte, errorPagesStatusUntil-errorPagesStatusOffset),
	}
}

func (p *ErrorPages) Handle(ctx *fasthttp.RequestCtx) {
	status := ctx.Response.StatusCode()
	if status < errorPagesStatusOffset || status >= errorPagesStatusUntil {
		return
	}
	if p.fs == nil {
		sendDefaultError(ctx)
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
		sendDefaultError(ctx)
		return
	}
	if l < len(sb) {
		sb[l] = 'x'
	}
	if page := p.cfg[string(sb)]; page != "" {
		path := []byte(page)
		p.errorPaths[status-errorPagesStatusOffset] = path
		p.sendError(ctx, path)
		return
	}
	p.handleStatus(ctx, sb, l-1)
}

func (p *ErrorPages) sendError(ctx *fasthttp.RequestCtx, path []byte) {
	if len(path) == 0 {
		sendDefaultError(ctx)
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
		sendDefaultError(ctx)
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

func sendDefaultError(ctx *fasthttp.RequestCtx) {
	if len(ctx.Response.Body()) > 0 {
		return
	}

	status := ctx.Response.StatusCode()
	b := bytebufferpool.Get()
	for _, h := range defaultErrorHtmls {
		if len(h) == 0 {
			b.B = fasthttp.AppendUint(b.B, status)
			b.B = append(b.B, ' ')
			b.B = append(b.B, []byte(http.StatusText(status))...)
			continue
		}
		b.B = append(b.B, h...)
	}
	ctx.Response.Header.SetContentTypeBytes(defaultErrorContentType)
	ctx.Response.SetBody(b.B)
	bytebufferpool.Put(b)
}
