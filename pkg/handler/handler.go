package handler

import (
	"fmt"

	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
)

// NewHandlerFunc is a function that creates a new fasthttp.RequestHandler.
type NewHandlerFunc func(cfg tree.Map, l logger.Logger) (fasthttp.RequestHandler, error)

var typedNewHandlerFunc = map[string]NewHandlerFunc{}

// RegisterNewHandlerFunc registers a NewHandlerFunc with the specified type name.
func RegisterNewHandlerFunc(typeName string, fn NewHandlerFunc) {
	typedNewHandlerFunc[typeName] = fn
}

// NewHandler creates a new fasthttp.RequestHandler.
func NewHandler(cfg tree.Map, l logger.Logger) (fasthttp.RequestHandler, error) {
	typeName := cfg.Get("type").Value().String()
	if fn, ok := typedNewHandlerFunc[typeName]; ok {
		return fn(cfg, l)
	}
	return nil, fmt.Errorf("unknown handler type: %s", typeName)
}
