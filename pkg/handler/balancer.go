package handler

import (
	"errors"
	"fmt"

	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
	"github.com/zehuamama/balancer/proxy"
)

func getBalancerUrls(cfg tree.Map) ([]string, error) {
	urls := cfg.Get("urls").Array()
	if len(urls) == 0 {
		if cfg.Get("url").Value().String() == "" {
			return nil, errors.New("failed to create balancer: require 'url' or 'urls' entry")
		}
		urls = tree.Array{cfg.Get("url").Value()}
	}
	urlStrs := make([]string, len(urls))
	for i, u := range urls {
		urlStrs[i] = u.Value().String()
	}
	return urlStrs, nil
}

// NewBalancerHandler creates a new a proxy handler that proxies to backend url.
// This handler using https://github.com/zehuamama/balancer.
// The specified cfg supports following keys.
//   - urls - backend url list
//   - algorithm - ip-hash, consistent-hash, p2c, random, round-robin, least-load, bounded
//   - healthCheckInterval - health-check interval (seconds)
//
// NOTE: If NewProxyHandler supports multiple URLs, this implementation may be deprecated.
func NewBalancerHandler(cfg tree.Map, l logger.Logger) (fasthttp.RequestHandler, error) {
	urls, err := getBalancerUrls(cfg)
	if err != nil {
		return nil, err
	}
	algorithm := cfg.Get("algorithm").Value().String()
	if algorithm == "" {
		algorithm = "round-robin"
	}
	p, err := proxy.NewHTTPProxy(urls, algorithm)
	if err != nil {
		return nil, fmt.Errorf("failed to create balancer: %w", err)
	}
	if interval := cfg.Get("healthCheckInterval").Value().Int(); interval > 0 {
		p.HealthCheck(uint(interval))
	}
	return fasthttpadaptor.NewFastHTTPHandler(p), nil
}

func init() {
	RegisterNewHandlerFunc("balancer", NewBalancerHandler)
}
