package handler

import (
	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/mojatter/tree"
	"github.com/valyala/fasthttp"
)

// NewBalancerHandler is kept as a backward-compatible alias of
// NewProxyHandler. The 'balancer' handler type accepts the same config keys
// as 'proxy' (url / urls / algorithm / healthCheckInterval).
//
// Deprecated: prefer handler type 'proxy' in new configs.
func NewBalancerHandler(cfg tree.Map, l logger.Logger) (fasthttp.RequestHandler, error) {
	return NewProxyHandler(cfg, l)
}

func init() {
	RegisterNewHandlerFunc("balancer", NewBalancerHandler)
	config.RegisterHandlerSchema("balancer", proxySchemas("balancer"))
}
