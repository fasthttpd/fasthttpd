//go:build debug

package handler

import (
	"bytes"
	"fmt"
	"net/http/pprof"

	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
)

var pprofPrefix = []byte("/debug/pprof/")

// Pprof represents a handler that provides runtime profiling data via "net/http/pprof".
// The following is a configuration that is minimal. routes.path is must "/debug/pprof/".
//
//   listen: ':8080'
//   handlers:
//     'pprof':
//       type: pprof
//   routes:
//     - path: /debug/pprof/
//       handler: pprof
//
// Then you can see http://localhost:8080/debug/pprof/ in your browser.
type Pprof struct {
	index   fasthttp.RequestHandler
	cmdline fasthttp.RequestHandler
	profile fasthttp.RequestHandler
	symbol  fasthttp.RequestHandler
	trace   fasthttp.RequestHandler
}

var _ fasthttp.RequestHandler = (*Pprof)(nil).Handle

// NewPprof creates a new Pprof that provides runtime profiling data.
func NewPprof(cfg tree.Map) (*Pprof, error) {
	return &Pprof{
		index:   fasthttpadaptor.NewFastHTTPHandlerFunc(pprof.Index),
		cmdline: fasthttpadaptor.NewFastHTTPHandlerFunc(pprof.Cmdline),
		profile: fasthttpadaptor.NewFastHTTPHandlerFunc(pprof.Profile),
		symbol:  fasthttpadaptor.NewFastHTTPHandlerFunc(pprof.Symbol),
		trace:   fasthttpadaptor.NewFastHTTPHandlerFunc(pprof.Trace),
	}, nil
}

// Handle serves runtime profiling via "net/http/pprof"
func (h *Pprof) Handle(ctx *fasthttp.RequestCtx) {
	path := ctx.Path()
	if !bytes.HasPrefix(path, pprofPrefix) {
		msg := fmt.Sprintf("Warning: pprof works only with the prefix %q", pprofPrefix)
		ctx.Response.SetBodyString(msg)
		return
	}

	switch string(path[len(pprofPrefix):]) {
	case "cmdline":
		h.cmdline(ctx)
	case "profile":
		h.profile(ctx)
	case "symbol":
		h.symbol(ctx)
	case "trace":
		h.trace(ctx)
	default:
		h.index(ctx)
	}
}

// NewPprofHandler creates a new fasthttp.RequestHandler via Pprof.Handle.
func NewPprofHandler(cfg tree.Map, _ logger.Logger) (fasthttp.RequestHandler, error) {
	h, err := NewPprof(cfg)
	if err != nil {
		return nil, err
	}
	return h.Handle, nil
}

func init() {
	RegisterNewHandlerFunc("pprof", NewPprofHandler)
}
