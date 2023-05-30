package handler

import (
	"bytes"
	"math/rand"
	"strings"

	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
)

// Content represents a handler that provides a simple content.
type Content struct {
	handlerCfg tree.Map
}

var _ fasthttp.RequestHandler = (*Content)(nil).Handle

// NewContent creates a new Content that provides a simple content.
// The handlerCfg can be specified 'body' string and 'headers' map.
func NewContent(handlerCfg tree.Map) (*Content, error) {
	return &Content{handlerCfg: handlerCfg}, nil
}

func (h *Content) outputHeaders(ctx *fasthttp.RequestCtx, cfg tree.Map) {
	headers := cfg.Get("headers")
	if headers.IsNil() {
		headers = h.handlerCfg.Get("headers")
	}
	switch headers.Type() {
	case tree.TypeMap:
		for k, v := range headers.Map() {
			ctx.Response.Header.Set(k, v.Value().String())
		}
	case tree.TypeArray:
		for _, v := range headers.Array() {
			switch v.Type() {
			case tree.TypeStringValue:
				kv := strings.SplitN(v.Value().String(), ": ", 2)
				if len(kv) == 2 {
					ctx.Response.Header.Add(kv[0], kv[1])
				}
			case tree.TypeMap:
				for kk, vv := range v.Map() {
					ctx.Response.Header.Add(kk, vv.Value().String())
				}
			}
		}
	}
}

func (h *Content) output(ctx *fasthttp.RequestCtx, cfg tree.Map) {
	h.outputHeaders(ctx, cfg)
	ctx.Response.SetBody([]byte(cfg.Get("body").Value().String()))
	if cfg.Has("status") {
		ctx.Response.SetStatusCode(cfg.Get("status").Value().Int())
	}
}

var contentRandomPercentage = func() int {
	return rand.Intn(100)
}

// Handle sets headers and body to the provided ctx.
func (h *Content) Handle(ctx *fasthttp.RequestCtx) {
	for _, condCfg := range h.handlerCfg.Get("conditions").Array() {
		if condCfg.Has("path") {
			if string(ctx.Path()) == condCfg.Get("path").Value().String() {
				h.output(ctx, condCfg.Map())
				return
			}
		} else if condCfg.Has("queryStringContains") {
			matches := true
			args := ctx.Request.URI().QueryArgs()
			condArgs := fasthttp.AcquireArgs()
			condArgs.Parse(condCfg.Get("queryStringContains").Value().String())
			condArgs.VisitAll(func(key, value []byte) {
				if matches && !bytes.Equal(args.PeekBytes(key), value) {
					matches = false
				}
			})
			fasthttp.ReleaseArgs(condArgs)
			if matches {
				h.output(ctx, condCfg.Map())
				return
			}
		} else if percentage := condCfg.Get("percentage").Value().Int(); percentage > 0 {
			if contentRandomPercentage() <= percentage {
				h.output(ctx, condCfg.Map())
				return
			}
		}
	}
	h.output(ctx, h.handlerCfg)
}

// NewContentHandler creates a new fasthttp.RequestHandler via Content.Handle.
func NewContentHandler(cfg tree.Map, _ logger.Logger) (fasthttp.RequestHandler, error) {
	h, err := NewContent(cfg)
	if err != nil {
		return nil, err
	}
	return h.Handle, nil
}

func init() {
	RegisterNewHandlerFunc("content", NewContentHandler)
}
