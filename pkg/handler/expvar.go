package handler

import (
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/expvarhandler"
)

// NewExpvarHandler returns a ExpvarHandler that dumps json representation of
// expvars to http response.
// Refer to https://github.com/valyala/fasthttp/tree/master/expvarhandler
func NewExpvarHandler(cfg tree.Map, l logger.Logger) (fasthttp.RequestHandler, error) {
	return expvarhandler.ExpvarHandler, nil
}

func init() {
	RegisterNewHandlerFunc("expvar", NewExpvarHandler)
}
