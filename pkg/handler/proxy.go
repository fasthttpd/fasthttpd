package handler

import (
	"errors"
	"net/http/httputil"
	"net/url"

	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
)

// NewProxyHandler creates a new a proxy handler that proxies to backend url.
// The specified cfg must have 'url' entry.
func NewProxyHandler(cfg tree.Map, l logger.Logger) (fasthttp.RequestHandler, error) {
	urlStr := cfg.Get("url").Value().String()
	if urlStr == "" {
		return nil, errors.New("failed to create proxy: require 'url' entry")
	}
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	// TODO: Use fasthttp.Client, or ProxyHandler listed on TODO of valyala/fasthttp.
	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.ErrorLog = l.LogLogger()
	return fasthttpadaptor.NewFastHTTPHandler(proxy), nil
}

func init() {
	RegisterNewHandlerFunc("proxy", NewProxyHandler)
}
