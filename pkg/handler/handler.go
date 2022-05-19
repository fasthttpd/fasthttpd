package handler

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/signal"
	"strings"
	"syscall"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/filter"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/fasthttpd/fasthttpd/pkg/logger/accesslog"
	"github.com/fasthttpd/fasthttpd/pkg/route"
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

type MainHandler struct {
	cfg        config.Config
	logger     logger.Logger
	accessLog  accesslog.AccessLog
	stopHup    context.CancelFunc
	errorPages *ErrorPages
	filters    map[string]filter.Filter
	handlers   map[string]fasthttp.RequestHandler
	routes     *route.Routes
}

func NewMainHandler(cfg config.Config) (*MainHandler, error) {
	h := &MainHandler{
		cfg:        cfg,
		errorPages: NewErrorPages(cfg.Root, cfg.ErrorPages),
	}
	if err := h.init(); err != nil {
		return nil, err
	}
	return h, nil
}

func (h *MainHandler) Close() error {
	if h.stopHup != nil {
		stop := h.stopHup
		h.stopHup = nil
		stop()
	}
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

func (h *MainHandler) init() error {
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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGHUP)
	h.stopHup = stop
	go func() {
		for {
			<-ctx.Done()
			if h.stopHup == nil {
				break
			}
			l.Printf("signal hup: rotate logs\n")
			l.Rotate()  //nolint:errcheck
			al.Rotate() //nolint:errcheck
			stop()
			ctx, stop = signal.NotifyContext(context.Background(), syscall.SIGHUP)
			h.stopHup = stop
		}
	}()

	return nil
}

func (h *MainHandler) Logger() logger.Logger {
	return h.logger
}

// Handle handles requests.
func (h *MainHandler) Handle(ctx *fasthttp.RequestCtx) {
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
func (h *MainHandler) HandleError(ctx *fasthttp.RequestCtx, err error) {
	if _, ok := err.(*fasthttp.ErrSmallBuffer); ok {
		ctx.Response.SetStatusCode(http.StatusRequestHeaderFieldsTooLarge)
	} else if netErr, ok := err.(*net.OpError); ok && netErr.Timeout() {
		ctx.Response.SetStatusCode(http.StatusRequestTimeout)
	} else {
		ctx.Response.SetStatusCode(http.StatusBadRequest)
	}
	h.errorPages.Handle(ctx)
}
