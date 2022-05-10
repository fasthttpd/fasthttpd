package route

import (
	"bytes"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/util"
	"github.com/valyala/bytebufferpool"
	"github.com/valyala/fasthttp"
)

type Route struct {
	cfg                config.Route
	pathBytes          []byte
	rewriteUriBytes    []byte
	statusMessageBytes []byte
	methodsBytes       [][]byte
	re                 *regexp.Regexp
}

func NewRoute(cfg config.Route) (*Route, error) {
	if cfg.Path == "" {
		cfg.Path = "/"
	}
	if cfg.Match == "" {
		cfg.Match = config.MatchPrefix
	}
	r := &Route{
		cfg:             cfg,
		pathBytes:       []byte(cfg.Path),
		rewriteUriBytes: []byte(cfg.Rewrite.URI),
	}
	if cfg.Status.Message != "" {
		r.statusMessageBytes = []byte(cfg.Status.Message)
	} else if cfg.Status.Code > 0 {
		r.statusMessageBytes = []byte(http.StatusText(cfg.Status.Code))
	}
	if len(cfg.Methods) > 0 {
		r.methodsBytes = make([][]byte, len(cfg.Methods))
		for i, m := range cfg.Methods {
			r.methodsBytes[i] = []byte(strings.ToUpper(m))
		}
	}
	if cfg.Match == config.MatchRegexp {
		re, err := regexp.Compile(cfg.Path)
		if err != nil {
			return nil, err
		}
		r.re = re
	}
	return r, nil
}

func (r *Route) Match(method, path []byte) bool {
	return r.matchMethods(method) && r.matchPath(path)
}

func (r *Route) matchMethods(method []byte) bool {
	if len(r.cfg.Methods) == 0 {
		return true
	}
	for _, m := range r.methodsBytes {
		if bytes.Equal(method, m) {
			return true
		}
	}
	return false
}

func (r *Route) matchPath(path []byte) bool {
	switch r.cfg.Match {
	case config.MatchEqual:
		if bytes.Equal(path, r.pathBytes) {
			return true
		}
	case config.MatchPrefix:
		if bytes.HasPrefix(path, r.pathBytes) {
			return true
		}
	case config.MatchRegexp:
		if r.re.Match(path) {
			return true
		}
	}
	return false
}

func (r *Route) rewrite(path []byte) []byte {
	if len(r.rewriteUriBytes) > 0 && r.re != nil {
		return r.re.ReplaceAll(path, r.rewriteUriBytes)
	}
	return r.rewriteUriBytes
}

type RoutesResult struct {
	StatusCode        int
	StatusMessage     []byte
	RewriteURI        []byte
	RedirectURI       []byte
	AppendQueryString bool
	Handler           string
	Filters           []string
}

func (r RoutesResult) RewriteURIWithQueryString(ctx *fasthttp.RequestCtx) []byte {
	if r.AppendQueryString && len(r.RewriteURI) > 0 {
		return util.AppendQueryString(r.RewriteURI, ctx.URI().QueryString())
	}
	return r.RewriteURI
}

func (r RoutesResult) RedirectURIWithQueryString(ctx *fasthttp.RequestCtx) []byte {
	if r.AppendQueryString && len(r.RedirectURI) > 0 {
		return util.AppendQueryString(r.RedirectURI, ctx.URI().QueryString())
	}
	return r.RedirectURI
}

var DefaultRoutesCacheSize = 100

type Routes struct {
	rs    []*Route
	cache util.Cache
}

func NewRoutes(cfg config.Config) (*Routes, error) {
	rs := make([]*Route, len(cfg.Routes))
	for i, rcfg := range cfg.Routes {
		for _, f := range rcfg.Filters {
			if _, ok := cfg.Filters[f]; !ok {
				return nil, fmt.Errorf("unknown filter: %s", f)
			}
		}
		if rcfg.Handler != "" {
			if _, ok := cfg.Handlers[rcfg.Handler]; !ok {
				return nil, fmt.Errorf("unknown handler: %s", rcfg.Handler)
			}
		}
		r, err := NewRoute(rcfg)
		if err != nil {
			return nil, err
		}
		rs[i] = r
	}
	routes := &Routes{rs: rs}
	if cfg.RoutesCache.Enable {
		routes.cache = util.NewExpireCache(int64(cfg.RoutesCache.Expire))
	}
	return routes, nil
}

func (rs Routes) Route(method, path []byte) *RoutesResult {
	result := &RoutesResult{}
	var uniqFilters map[string]bool
	for _, r := range rs.rs {
		if !r.Match(method, path) {
			continue
		}
		for _, f := range r.cfg.Filters {
			if _, ok := uniqFilters[f]; !ok {
				result.Filters = append(result.Filters, f)
				if uniqFilters == nil {
					uniqFilters = map[string]bool{}
				}
				uniqFilters[f] = true
			}
		}
		result.StatusCode = r.cfg.Status.Code
		result.StatusMessage = r.statusMessageBytes
		result.Handler = r.cfg.Handler

		if rewriteUri := r.rewrite(path); len(rewriteUri) > 0 {
			result.AppendQueryString = r.cfg.Rewrite.AppendQueryString
			if util.IsHttpOrHttps(rewriteUri) {
				result.RedirectURI = rewriteUri
				return result
			}
			if util.IsHttpStatusRedirect(result.StatusCode) {
				result.RedirectURI = rewriteUri
				return result
			}
			result.RewriteURI = rewriteUri
			path, _ = util.SplitRequestURI(rewriteUri)
		}
		if result.StatusCode > 0 || result.Handler != "" {
			return result
		}
	}
	result.StatusCode = fasthttp.StatusNotFound
	return result
}

func (rs Routes) CachedRoute(method, path []byte) *RoutesResult {
	if rs.cache == nil {
		return rs.Route(method, path)
	}

	b := bytebufferpool.Get()
	b.B = append(b.B, method...)
	b.B = append(b.B, ' ')
	b.B = append(b.B, path...)
	key := b.String()
	bytebufferpool.Put(b)

	if v := rs.cache.Get(key); v != nil {
		return v.(*RoutesResult)
	}
	result := rs.Route(method, path)
	rs.cache.Set(key, result)
	return result
}

func (rs Routes) CachedRouteCtx(ctx *fasthttp.RequestCtx) *RoutesResult {
	return rs.CachedRoute(ctx.Method(), ctx.Path())
}
