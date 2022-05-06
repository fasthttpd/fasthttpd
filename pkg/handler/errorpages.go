package handler

import (
	"net/http"
	"strconv"

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
	if p.fs == nil {
		return
	}
	status := ctx.Response.StatusCode()
	if status < errorPagesStatusOffset || status >= errorPagesStatusUntil {
		return
	}
	if path := p.errorPaths[status-errorPagesStatusOffset]; path != nil {
		p.sendError(ctx, status, path)
		return
	}
	sb := []byte(strconv.Itoa(status))
	p.handleStatus(ctx, status, sb, len(sb))
}

func (p *ErrorPages) handleStatus(ctx *fasthttp.RequestCtx, status int, sb []byte, l int) {
	if l < 0 {
		// NOTE: store no erorr page.
		p.errorPaths[status-errorPagesStatusOffset] = []byte{}
		return
	}
	if l < len(sb) {
		sb[l] = 'x'
	}
	if page := p.cfg[string(sb)]; page != "" {
		path := []byte(page)
		p.errorPaths[status-errorPagesStatusOffset] = path
		p.sendError(ctx, status, path)
		return
	}
	p.handleStatus(ctx, status, sb, l-1)
}

func (p *ErrorPages) sendError(ctx *fasthttp.RequestCtx, status int, path []byte) {
	if len(path) == 0 {
		ctx.Error(http.StatusText(status), status)
		return
	}

	uri := string(ctx.Request.RequestURI())
	ctx.Request.SetRequestURIBytes(path)
	p.fs(ctx)
	ctx.Request.SetRequestURI(uri)

	if ctx.Response.StatusCode() == http.StatusOK {
		ctx.SetStatusCode(status)
	} else {
		ctx.Logger().Printf("error page %q status %d", path, ctx.Response.StatusCode())
		// NOTE: store the path has error.
		p.errorPaths[status-errorPagesStatusOffset] = []byte{}
		ctx.Error(http.StatusText(status), status)
	}
}
