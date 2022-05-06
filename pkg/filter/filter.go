package filter

import (
	"fmt"

	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
)

// Filter is a function that filters ctx and returns bool (false means break filters).
type Filter func(ctx *fasthttp.RequestCtx) bool

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
