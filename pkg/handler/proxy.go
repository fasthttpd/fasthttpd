package handler

import (
	"net/http/httputil"
	"net/url"

	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
)

type ProxyHandler struct {
	URL string
}

func NewProxyHandler(cfg tree.Map) (fasthttp.RequestHandler, error) {
	h := &ProxyHandler{}
	if err := tree.UnmarshalViaYAML(cfg, h); err != nil {
		return nil, err
	}
	u, err := url.Parse(h.URL)
	if err != nil {
		return nil, err
	}
	// TODO: Use fasthttp.Client, or ProxyHandler listed on TODO of valyala/fasthttp.
	proxy := httputil.NewSingleHostReverseProxy(u)
	return fasthttpadaptor.NewFastHTTPHandler(proxy), nil
}

func init() {
	RegisterNewHandlerFunc("proxy", NewProxyHandler)
}
