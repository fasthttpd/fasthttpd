package handler

import (
	"errors"
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/mojatter/tree"
	"github.com/mojatter/tree/schema"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
)

// Supported algorithms for NewProxyHandler.
const (
	algoRoundRobin = "round-robin"
	algoRandom     = "random"
	algoIPHash     = "ip-hash"
)

var supportedAlgorithms = map[string]struct{}{
	algoRoundRobin: {},
	algoRandom:     {},
	algoIPHash:     {},
}

type proxyBackend struct {
	url     *url.URL
	handler http.Handler
	alive   atomic.Bool
}

type proxyBalancer struct {
	backends  []*proxyBackend
	algorithm string
	counter   atomic.Uint64
	l         logger.Logger
}

func newProxyBalancer(urls []string, algorithm string, l logger.Logger) (*proxyBalancer, error) {
	if len(urls) == 0 {
		return nil, errors.New("require 'url' or 'urls' entry")
	}
	if _, ok := supportedAlgorithms[algorithm]; !ok {
		return nil, fmt.Errorf("algorithm not supported: %s", algorithm)
	}
	backends := make([]*proxyBackend, 0, len(urls))
	for _, s := range urls {
		u, err := url.Parse(s)
		if err != nil {
			return nil, err
		}
		rp := httputil.NewSingleHostReverseProxy(u)
		rp.ErrorLog = l.LogLogger()
		be := &proxyBackend{url: u, handler: rp}
		be.alive.Store(true)
		backends = append(backends, be)
	}
	return &proxyBalancer{
		backends:  backends,
		algorithm: algorithm,
		l:         l,
	}, nil
}

// pick returns the next alive backend according to the configured algorithm,
// skipping backends that are currently marked down by the health checker.
func (b *proxyBalancer) pick(r *http.Request) *proxyBackend {
	n := uint64(len(b.backends))
	if n == 0 {
		return nil
	}
	var start uint64
	switch b.algorithm {
	case algoRandom:
		start = rand.Uint64N(n)
	case algoIPHash:
		start = hashClientIP(r) % n
	default: // round-robin
		start = b.counter.Add(1) - 1
	}
	for i := range n {
		be := b.backends[(start+i)%n]
		if be.alive.Load() {
			return be
		}
	}
	return nil
}

func (b *proxyBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	be := b.pick(r)
	if be == nil {
		http.Error(w, "no healthy backend", http.StatusServiceUnavailable)
		return
	}
	be.handler.ServeHTTP(w, r)
}

func hashClientIP(r *http.Request) uint64 {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	hs := fnv.New64a()
	hs.Write([]byte(host))
	return hs.Sum64()
}

// startHealthCheck launches a background goroutine that HEADs each backend.
// intervalSec <= 0 disables the checker entirely.
//
// The HEAD timeout is derived as min(interval, 5s): capping at the interval
// guarantees in-flight checks cannot pile up across ticks, and 5s is a safe
// upper bound for realistic backends.
func (b *proxyBalancer) startHealthCheck(intervalSec int) {
	if intervalSec <= 0 {
		return
	}
	interval := time.Duration(intervalSec) * time.Second
	timeout := min(interval, 5*time.Second)
	ticker := time.NewTicker(interval)
	client := &fasthttp.Client{
		ReadTimeout:  timeout,
		WriteTimeout: timeout,
	}
	go func() {
		defer ticker.Stop()

		b.runHealthCheck(ticker.C, client)
	}()
}

// runHealthCheck iterates over tick events, HEADs every backend through client
// and updates their alive state. Exits when tick is closed. Extracted from
// startHealthCheck so tests can drive the loop with a synthetic channel.
func (b *proxyBalancer) runHealthCheck(tick <-chan time.Time, client *fasthttp.Client) {
	for range tick {
		for _, be := range b.backends {
			err := headBackend(client, be.url.String())
			alive := err == nil
			if was := be.alive.Swap(alive); was != alive {
				if alive {
					b.l.Printf("backend %s back online", be.url.String())
				} else {
					b.l.Printf("backend %s marked down: err=%v", be.url.String(), err)
				}
			}
		}
	}
}

// headBackend issues a HEAD request via fasthttp.Client and returns nil iff the
// backend is reachable and responded with a non-5xx status. Request/response
// objects are acquired from fasthttp pools to keep the checker allocation-free.
func headBackend(client *fasthttp.Client, url string) error {
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)

	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(url)
	req.Header.SetMethod(fasthttp.MethodHead)
	if err := client.Do(req, resp); err != nil {
		return err
	}
	if code := resp.StatusCode(); code >= 500 {
		return fmt.Errorf("status %d", code)
	}
	return nil
}

func proxyURLs(cfg tree.Map) ([]string, error) {
	urls := cfg.Get("urls").Array()
	if len(urls) == 0 {
		single := cfg.Get("url").Value().String()
		if single == "" {
			return nil, errors.New("require 'url' or 'urls' entry")
		}
		return []string{single}, nil
	}
	result := make([]string, len(urls))
	for i, u := range urls {
		result[i] = u.Value().String()
	}
	return result, nil
}

// NewProxyHandler creates a new proxy handler that proxies to one or more
// backend URLs.
//
// The specified cfg supports the following keys:
//   - url                 - single backend URL (used when 'urls' is empty)
//   - urls                - backend URL list
//   - algorithm           - one of round-robin (default), random, ip-hash
//   - healthCheckInterval - health-check interval in seconds (0 disables)
func NewProxyHandler(cfg tree.Map, l logger.Logger) (fasthttp.RequestHandler, error) {
	urls, err := proxyURLs(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create proxy: %w", err)
	}
	algorithm := cfg.Get("algorithm").Value().String()
	if algorithm == "" {
		algorithm = algoRoundRobin
	}
	b, err := newProxyBalancer(urls, algorithm, l)
	if err != nil {
		return nil, fmt.Errorf("failed to create proxy: %w", err)
	}
	b.startHealthCheck(cfg.Get("healthCheckInterval").Value().Int())
	return fasthttpadaptor.NewFastHTTPHandler(b), nil
}

func init() {
	RegisterNewHandlerFunc("proxy", NewProxyHandler)
	config.RegisterHandlerSchema("proxy", proxySchemas("proxy"))
}

// proxySchemas builds the rule set for a proxy-like handler with the
// given type discriminator. Shared with balancer, which is an alias
// that differs only in the accepted `type` value.
func proxySchemas(typeName string) schema.QueryRules {
	return schema.QueryRules{
		".": schema.Map{KeyedRules: map[string]schema.Rule{
			"type":                schema.String{Enum: []string{typeName}},
			"url":                 schema.String{},
			"urls":                schema.Array{},
			"algorithm":           schema.String{Enum: []string{algoRoundRobin, algoRandom, algoIPHash}},
			"healthCheckInterval": schema.Int{Min: tree.Int64Ptr(0)},
		}},
		".urls[]": schema.String{},
	}
}
