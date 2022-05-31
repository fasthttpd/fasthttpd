package handler

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/filter"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/fasthttpd/fasthttpd/pkg/logger/accesslog"
	"github.com/fasthttpd/fasthttpd/pkg/route"
	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
)

// ServerHandler is an interface that defines the methods to handle a server.
type ServerHandler interface {
	// Config returns the config.Config.Server.
	Config() tree.Map
	// Logger returns a logger.
	Logger() logger.Logger
	// Handle handles the provided request.
	Handle(ctx *fasthttp.RequestCtx)
	// HandleError implements fasthttp.Server.ErrorHandler.
	HandleError(ctx *fasthttp.RequestCtx, err error)
	// Close closes the server.
	Close() error
}

// NewServerHandler creates a new ServerHandler by the provided cfgs.
func NewServerHandler(cfgs []config.Config) (ServerHandler, error) {
	if len(cfgs) == 1 {
		return newHostHandler(cfgs[0])
	}
	return newVirtualHandler(cfgs)
}

type virtualHandler struct {
	handlers []*hostHandler
	logger   logger.Logger
}

func newVirtualHandler(cfgs []config.Config) (*virtualHandler, error) {
	outputs := map[string]bool{}
	var handlers []*hostHandler
	var loggers []logger.Logger
	for _, cfg := range cfgs {
		h, err := newHostHandler(cfg)
		if err != nil {
			return nil, err
		}
		handlers = append(handlers, h)
		if _, ok := outputs[cfg.Log.Output]; !ok {
			outputs[cfg.Log.Output] = true
			loggers = append(loggers, h.Logger())
		}
	}
	return &virtualHandler{
		handlers: handlers,
		logger: &logger.LoggerDelegator{
			PrintfFunc: func(format string, args ...interface{}) {
				for _, l := range loggers {
					l.Printf(format, args...)
				}
			},
		},
	}, nil
}

func (v *virtualHandler) handler(hostBytes []byte) *hostHandler {
	host := hostBytes
	if i := bytes.Index(hostBytes, []byte{':'}); i != -1 {
		host = hostBytes[:i]
	}
	v.logger.Printf("## virtualHandler.handler called %s", host)
	if len(v.handlers) > 1 {
		for _, h := range v.handlers {
			if bytes.Equal(host, h.hostBytes) {
				return h
			}
		}
	}
	return v.handlers[0]
}

// Config returns the config.Config.Server.
func (v *virtualHandler) Config() tree.Map {
	return v.handlers[0].cfg.Server
}

// Logger returns a logger.
func (v *virtualHandler) Logger() logger.Logger {
	return v.logger
}

// Handle handles the provided request.
func (v *virtualHandler) Handle(ctx *fasthttp.RequestCtx) {
	v.handler(ctx.Host()).Handle(ctx)
}

// HandleError implements fasthttp.Server.ErrorHandler.
func (v *virtualHandler) HandleError(ctx *fasthttp.RequestCtx, err error) {
	v.handler(ctx.Host()).HandleError(ctx, err)
}

// Close calls Close each handlers.
func (v *virtualHandler) Close() error {
	var errs []string
	for _, h := range v.handlers {
		if err := h.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to multi close: %s", strings.Join(errs, "; "))
	}
	return nil
}

type hostHandler struct {
	cfg        config.Config
	hostBytes  []byte
	logger     logger.Logger
	accessLog  accesslog.AccessLog
	errorPages *ErrorPages
	filters    map[string]filter.Filter
	handlers   map[string]fasthttp.RequestHandler
	routes     *route.Routes
}

func newHostHandler(cfg config.Config) (*hostHandler, error) {
	h := &hostHandler{
		cfg:        cfg,
		hostBytes:  []byte(cfg.Host),
		errorPages: NewErrorPages(cfg.Root, cfg.ErrorPages),
	}
	if err := h.init(); err != nil {
		return nil, err
	}
	return h, nil
}

func (h *hostHandler) init() error {
	l, err := logger.NewLogger(h.cfg.Log)
	if err != nil {
		return err
	}
	al, err := accesslog.NewAccessLog(h.cfg)
	if err != nil {
		return err
	}
	h.logger = l
	h.accessLog = al

	h.filters = map[string]filter.Filter{}
	for name, filterCfg := range h.cfg.Filters {
		f, err := filter.NewFilter(filterCfg)
		if err != nil {
			return err
		}
		h.filters[name] = f
	}

	h.handlers = map[string]fasthttp.RequestHandler{}
	for name, hcfg := range h.cfg.Handlers {
		if hcfg.Get("root").Value().String() == "" {
			hcfg.Set("root", tree.ToValue(h.cfg.Root)) //nolint:errcheck
		}
		hh, err := NewHandler(hcfg, l)
		if err != nil {
			return err
		}
		h.handlers[name] = hh
	}

	routes, err := route.NewRoutes(h.cfg)
	if err != nil {
		return err
	}
	h.routes = routes

	return nil
}

// Config returns the config.Config.Server.
func (h *hostHandler) Config() tree.Map {
	return h.cfg.Server
}

// Logger returns a logger.
func (h *hostHandler) Logger() logger.Logger {
	return h.logger
}

// Handle handles the provided request.
func (h *hostHandler) Handle(ctx *fasthttp.RequestCtx) {
	h.accessLog.Collect(ctx)
	defer h.accessLog.Log(ctx)

	result := h.routes.CachedRouteCtx(ctx)
	if uri := result.RewriteURIWithQueryString(ctx); len(uri) > 0 {
		ctx.Request.SetRequestURIBytes(uri)
	}
	for _, f := range result.Filters {
		if !h.filters[f].Request(ctx) {
			h.errorPages.Handle(ctx)
			return
		}
	}
	if uri := result.RedirectURIWithQueryString(ctx); len(uri) > 0 {
		ctx.RedirectBytes(uri, result.StatusCode)
		return
	}
	if result.StatusCode > 0 {
		ctx.Response.Reset()
		ctx.Response.SetStatusCode(result.StatusCode)
		ctx.Response.Header.SetStatusMessage(result.StatusMessage)
	} else if result.Handler != "" {
		h.handlers[result.Handler](ctx)
	} else {
		ctx.Response.SetStatusCode(http.StatusNotFound)
	}
	h.errorPages.Handle(ctx)
	for _, f := range result.Filters {
		if !h.filters[f].Response(ctx) {
			break
		}
	}
}

// HandleError implements fasthttp.Server.ErrorHandler.
func (h *hostHandler) HandleError(ctx *fasthttp.RequestCtx, err error) {
	if _, ok := err.(*fasthttp.ErrSmallBuffer); ok {
		ctx.Response.SetStatusCode(http.StatusRequestHeaderFieldsTooLarge)
	} else if netErr, ok := err.(*net.OpError); ok && netErr.Timeout() {
		ctx.Response.SetStatusCode(http.StatusRequestTimeout)
	} else {
		ctx.Response.SetStatusCode(http.StatusBadRequest)
	}
	h.errorPages.Handle(ctx)
}

// Close closes the server.
func (h *hostHandler) Close() error {
	var errs []string
	if h.accessLog != nil {
		if err := h.accessLog.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to close main handler: %s", strings.Join(errs, "; "))
	}
	return nil
}
