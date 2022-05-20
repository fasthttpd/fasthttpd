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

// Route represents a route settings that can be used to match requested URLs.
type Route struct {
	methodsBytes             [][]byte
	filters                  []string
	rewriteUriBytes          []byte
	rewriteAppendQueryString bool
	handler                  string
	statusCode               int
	statusMessageBytes       []byte
	matchPath                func(path []byte) bool
	matchPattern             *regexp.Regexp
}

// NewRoute creates a new Route by the provided rcfg.
func NewRoute(rcfg config.Route) (*Route, error) {
	r := &Route{
		filters:                  rcfg.Filters,
		rewriteUriBytes:          []byte(rcfg.Rewrite.URI),
		rewriteAppendQueryString: rcfg.Rewrite.AppendQueryString,
		handler:                  rcfg.Handler,
		statusCode:               rcfg.Status.Code,
	}
	if rcfg.Status.Message != "" {
		r.statusMessageBytes = []byte(rcfg.Status.Message)
	} else if rcfg.Status.Code > 0 {
		r.statusMessageBytes = []byte(http.StatusText(rcfg.Status.Code))
	}
	if len(rcfg.Methods) > 0 {
		r.methodsBytes = make([][]byte, len(rcfg.Methods))
		for i, m := range rcfg.Methods {
			r.methodsBytes[i] = []byte(strings.ToUpper(m))
		}
	}
	if err := r.initMatchPath(rcfg.Match, rcfg.Path); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Route) initMatchPath(cfgMatch, cfgPath string) error {
	if cfgMatch == "" {
		cfgMatch = config.MatchPrefix
	}
	if cfgPath == "" {
		cfgPath = "/"
	}
	switch cfgMatch {
	case config.MatchEqual:
		eq := []byte(cfgPath)
		r.matchPath = func(path []byte) bool {
			return bytes.Equal(path, eq)
		}
		return nil
	case config.MatchPrefix:
		prefix := []byte(cfgPath)
		r.matchPath = func(path []byte) bool {
			return bytes.HasPrefix(path, prefix)
		}
		return nil
	case config.MatchRegexp:
		pattern, err := regexp.Compile(cfgPath)
		if err != nil {
			return err
		}
		r.matchPath = func(path []byte) bool {
			return pattern.Match(path)
		}
		r.matchPattern = pattern
		return nil
	}
	return fmt.Errorf("unknown match: %s", cfgMatch)
}

// Match matches the provided method and path.
func (r *Route) Match(method, path []byte) bool {
	return r.matchMethods(method) && r.matchPath(path)
}

func (r *Route) matchMethods(method []byte) bool {
	if len(r.methodsBytes) == 0 {
		return true
	}
	for _, m := range r.methodsBytes {
		if bytes.Equal(method, m) {
			return true
		}
	}
	return false
}

func (r *Route) rewrite(path []byte) []byte {
	if len(r.rewriteUriBytes) > 0 && r.matchPattern != nil {
		return r.matchPattern.ReplaceAll(path, r.rewriteUriBytes)
	}
	return r.rewriteUriBytes
}

// RouteResult represents a result of routing.
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

// Routes represents a list of routes that can be used to match requested URLs.
type Routes struct {
	routes []*Route
	cache  util.Cache
}

// NewRoutes creates a new Routes the provided cfg.Routes.
func NewRoutes(cfg config.Config) (*Routes, error) {
	routes := make([]*Route, len(cfg.Routes))
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
		routes[i] = r
	}
	rs := &Routes{routes: routes}
	if cfg.RoutesCache.Enable {
		rs.cache = util.NewExpireCache(int64(cfg.RoutesCache.Expire))
	}
	return rs, nil
}

// Route find routes by the provided method and path and returns a new RoutesResult.
func (rs Routes) Route(method, path []byte) *RoutesResult {
	result := &RoutesResult{}
	var uniqFilters map[string]bool
	for _, r := range rs.routes {
		if !r.Match(method, path) {
			continue
		}
		for _, f := range r.filters {
			if _, ok := uniqFilters[f]; !ok {
				result.Filters = append(result.Filters, f)
				if uniqFilters == nil {
					uniqFilters = map[string]bool{}
				}
				uniqFilters[f] = true
			}
		}
		result.StatusCode = r.statusCode
		result.StatusMessage = r.statusMessageBytes
		result.Handler = r.handler

		if rewriteUri := r.rewrite(path); len(rewriteUri) > 0 {
			result.AppendQueryString = r.rewriteAppendQueryString
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

// CachedRoute provides Read-Through caching for rs.Route if the cache is enabled.
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

// CachedRouteCtx provides Read-Through caching for rs.Route if the cache is enabled.
func (rs Routes) CachedRouteCtx(ctx *fasthttp.RequestCtx) *RoutesResult {
	return rs.CachedRoute(ctx.Method(), ctx.Path())
}
