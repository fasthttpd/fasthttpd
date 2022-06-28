package filter

import (
	"fmt"

	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
)

// Filter represents a filter that defines to filter request and response.
type Filter interface {
	// Request filters before handling request context. If it returns false,
	// further processing will stop.
	Request(ctx *fasthttp.RequestCtx) bool
	// Response filters after handling request context. If it returns false,
	// further processing will stop.
	Response(ctx *fasthttp.RequestCtx) bool
}

// NewFilterFunc is a function that returns a new filter.
type NewFilterFunc func(cfg tree.Map) (Filter, error)

var typedNewFilterFunc = map[string]NewFilterFunc{}

// RegisterNewFilterFunc registers a NewFilterFunc with filterType.
func RegisterNewFilterFunc(filterType string, fn NewFilterFunc) {
	typedNewFilterFunc[filterType] = fn
}

// NewFilter returns a new filter.
func NewFilter(cfg tree.Map) (Filter, error) {
	t := cfg.Get("type").Value().String()
	if fn, ok := typedNewFilterFunc[t]; ok {
		return fn(cfg)
	}
	return nil, fmt.Errorf("unknown filter type: %s", t)
}

// FilterDelegator can delegate the Filter functions.
type FilterDelegator struct {
	RequestFunc  func(ctx *fasthttp.RequestCtx) bool
	ResponseFunc func(ctx *fasthttp.RequestCtx) bool
}

var _ Filter = (*FilterDelegator)(nil)

func (d *FilterDelegator) Request(ctx *fasthttp.RequestCtx) bool {
	if d.RequestFunc != nil {
		return d.RequestFunc(ctx)
	}
	return true
}

func (d *FilterDelegator) Response(ctx *fasthttp.RequestCtx) bool {
	if d.ResponseFunc != nil {
		return d.ResponseFunc(ctx)
	}
	return true
}
