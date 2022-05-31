package handler

import (
	"fmt"

	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
)

type NewHandlerFunc func(cfg tree.Map, l logger.Logger) (fasthttp.RequestHandler, error)

var typedNewHandlerFunc = map[string]NewHandlerFunc{}

func RegisterNewHandlerFunc(t string, fn NewHandlerFunc) {
	typedNewHandlerFunc[t] = fn
}

func NewHandler(cfg tree.Map, l logger.Logger) (fasthttp.RequestHandler, error) {
	t := cfg.Get("type").Value().String()
	if fn, ok := typedNewHandlerFunc[t]; ok {
		return fn(cfg, l)
	}
	return nil, fmt.Errorf("unknown handler type: %s", t)
}
