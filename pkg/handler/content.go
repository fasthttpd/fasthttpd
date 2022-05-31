package handler

import (
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
)

type Content struct {
	resp fasthttp.Response
}

func NewContent(cfg tree.Map) *Content {
	var resp fasthttp.Response
	for k, v := range cfg.Get("headers").Map() {
		resp.Header.AddBytesKV([]byte(k), []byte(v.Value().String()))
	}
	resp.SetBodyString(cfg.Get("body").Value().String())

	return &Content{resp: resp}
}

func (h *Content) Handle(ctx *fasthttp.RequestCtx) {
	h.resp.CopyTo(&ctx.Response)
}

func NewContentHandler(cfg tree.Map, _ logger.Logger) (fasthttp.RequestHandler, error) {
	return NewContent(cfg).Handle, nil
}

func init() {
	RegisterNewHandlerFunc("content", NewContentHandler)
}
