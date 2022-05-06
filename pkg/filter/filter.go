package filter

import (
	"fmt"

	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
)

type Filter func(ctx *fasthttp.RequestCtx) bool

type NewFilterFunc func(cfg tree.Map) (Filter, error)

var typedNewFilterFunc = map[string]NewFilterFunc{}

func RegisterNewFilterFunc(t string, fn NewFilterFunc) {
	typedNewFilterFunc[t] = fn
}

func NewFilter(cfg tree.Map) (Filter, error) {
	t := cfg.Get("type").Value().String()
	if fn, ok := typedNewFilterFunc[t]; ok {
		return fn(cfg)
	}
	return nil, fmt.Errorf("unknown filter type: %s", t)
}
