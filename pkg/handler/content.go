package handler

import (
	"fmt"
	"strings"

	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
)

// Content represents a handler that provides a simple content.
type Content struct {
	headerKeys   [][]byte
	headerValues [][]byte
	body         []byte
}

var _ fasthttp.RequestHandler = (*Content)(nil).Handle

// NewContent creates a new Content that provides a simple content.
// The cfg can be specified 'body' string and 'headers' map.
func NewContent(cfg tree.Map) (*Content, error) {
	c := &Content{
		body: []byte(cfg.Get("body").Value().String()),
	}

	headers := cfg.Get("headers")
	switch headers.Type() {
	case tree.TypeMap:
		for k, v := range headers.Map() {
			c.headerKeys = append(c.headerKeys, []byte(k))
			c.headerValues = append(c.headerValues, []byte(v.Value().String()))
		}
	case tree.TypeArray:
		for _, v := range headers.Array() {
			switch v.Type() {
			case tree.TypeStringValue:
				vs := v.Value().String()
				vv := strings.SplitN(vs, ": ", 2)
				if len(vv) != 2 {
					return nil, fmt.Errorf("invalid header format %q", vs)
				}
				c.headerKeys = append(c.headerKeys, []byte(vv[0]))
				c.headerValues = append(c.headerValues, []byte(vv[1]))
			case tree.TypeMap:
				for kk, vv := range v.Map() {
					c.headerKeys = append(c.headerKeys, []byte(kk))
					c.headerValues = append(c.headerValues, []byte(vv.Value().String()))
				}
			}
		}
	}
	return c, nil
}

// Handle sets headers and body to the provided ctx.
func (h *Content) Handle(ctx *fasthttp.RequestCtx) {
	for i, k := range h.headerKeys {
		ctx.Response.Header.SetBytesKV(k, h.headerValues[i])
	}
	ctx.Response.SetBody(h.body)
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
