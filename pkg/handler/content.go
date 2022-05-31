package handler

import (
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
)

type Content struct {
	headerKeys   [][]byte
	headerValues [][]byte
	body         []byte
}

func NewContent(cfg tree.Map) *Content {
	c := &Content{
		body: []byte(cfg.Get("body").Value().String()),
	}
	for k, v := range cfg.Get("headers").Map() {
		c.headerKeys = append(c.headerKeys, []byte(k))
		c.headerValues = append(c.headerValues, []byte(v.Value().String()))
	}
	return c
}

func (h *Content) Handle(ctx *fasthttp.RequestCtx) {
	for i, k := range h.headerKeys {
		ctx.Response.Header.SetBytesKV(k, h.headerValues[i])
	}
	ctx.Response.SetBody(h.body)
}

func NewContentHandler(cfg tree.Map, _ logger.Logger) (fasthttp.RequestHandler, error) {
	return NewContent(cfg).Handle, nil
}

func init() {
	RegisterNewHandlerFunc("content", NewContentHandler)
}
