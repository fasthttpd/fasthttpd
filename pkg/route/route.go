package route

import (
	"bytes"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/util"
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
		rewriteUriBytes:          []byte(rcfg.Rewrite),
		rewriteAppendQueryString: rcfg.RewriteAppendQueryString,
		handler:                  rcfg.Handler,
		statusCode:               rcfg.Status,
	}
	if rcfg.StatusMessage != "" {
		r.statusMessageBytes = []byte(rcfg.StatusMessage)
	} else if rcfg.Status > 0 {
		r.statusMessageBytes = []byte(http.StatusText(rcfg.Status))
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

// acquireRoutesResult returns an empty RoutesResult object from the pool.
//
// The returned RoutesResult may be returned to the pool with Release when no
// longer needed. This allows reducing GC load.
func acquireRoutesResult() *RoutesResult {
	return routesResultPool.Get().(*RoutesResult)
}

var routesResultPool = &sync.Pool{
	New: func() interface{} {
		r := &RoutesResult{}
		r.reset()
		return r
	},
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

func (r *RoutesResult) RewriteURIWithQueryString(ctx *fasthttp.RequestCtx) []byte {
	if r.AppendQueryString && len(r.RewriteURI) > 0 {
		return util.AppendQueryString(r.RewriteURI, ctx.URI().QueryString())
	}
	return r.RewriteURI
}

func (r *RoutesResult) RedirectURIWithQueryString(ctx *fasthttp.RequestCtx) []byte {
	if r.AppendQueryString && len(r.RedirectURI) > 0 {
		return util.AppendQueryString(r.RedirectURI, ctx.URI().QueryString())
	}
	return r.RedirectURI
}

func (r *RoutesResult) reset() {
	r.StatusCode = 0
	r.StatusMessage = r.StatusMessage[:0]
	r.RewriteURI = r.RewriteURI[:0]
	r.RedirectURI = r.RedirectURI[:0]
	r.AppendQueryString = false
	r.Handler = ""
	r.Filters = r.Filters[:0]
}

func (r *RoutesResult) copyTo(dst *RoutesResult) *RoutesResult {
	dst.reset()
	dst.StatusCode = r.StatusCode
	dst.StatusMessage = append(dst.StatusMessage[:0], r.StatusMessage...)
	dst.RewriteURI = append(dst.RewriteURI[:0], r.RewriteURI...)
	dst.RedirectURI = append(dst.RedirectURI, r.RedirectURI...)
	dst.AppendQueryString = r.AppendQueryString
	dst.Handler = r.Handler
	dst.Filters = append(dst.Filters[:0], r.Filters...)
	return dst
}

func (a *RoutesResult) equal(b *RoutesResult) bool {
	if len(a.Filters) != len(b.Filters) {
		return false
	}
	for i, f := range a.Filters {
		if f != b.Filters[i] {
			return false
		}
	}
	return a.StatusCode == b.StatusCode &&
		bytes.Equal(a.StatusMessage, b.StatusMessage) &&
		bytes.Equal(a.RewriteURI, b.RewriteURI) &&
		bytes.Equal(a.RedirectURI, b.RedirectURI) &&
		a.AppendQueryString == b.AppendQueryString &&
		a.Handler == b.Handler
}

// Release returns the object acquired via AcquireRoutesResult to the pool.
//
// Do not access the released RoutesResult object, otherwise data races may occur.
func (r *RoutesResult) Release() {
	r.reset()
	routesResultPool.Put(r)
}

func onRoutesResultExpired(_ util.CacheKey, value interface{}) {
	if r, ok := value.(*RoutesResult); ok {
		r.Release()
	}
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
		rs.cache = util.NewExpireCacheInterval(
			int64(cfg.RoutesCache.Expire),
			int64(cfg.RoutesCache.Interval),
		)
		rs.cache.OnExpired(onRoutesResultExpired)
	}
	return rs, nil
}

// Route find routes by the provided method and path and returns a new RoutesResult.
func (rs *Routes) Route(method, path []byte) *RoutesResult {
	result := acquireRoutesResult()
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
		result.StatusMessage = append(result.StatusMessage[:0], r.statusMessageBytes...)
		result.Handler = r.handler

		if rewriteUri := r.rewrite(path); len(rewriteUri) > 0 {
			result.AppendQueryString = r.rewriteAppendQueryString
			if util.IsHttpOrHttps(rewriteUri) || util.IsHttpStatusRedirect(result.StatusCode) {
				result.RedirectURI = append(result.RedirectURI, rewriteUri...)
				return result
			}
			result.RewriteURI = append(result.RewriteURI[:0], rewriteUri...)
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
func (rs *Routes) CachedRoute(method, path []byte) *RoutesResult {
	if rs.cache == nil {
		return rs.Route(method, path)
	}

	key := util.CacheKeyBytes(method, []byte{' '}, path)
	if v := rs.cache.Get(key); v != nil {
		result := v.(*RoutesResult)
		return result.copyTo(acquireRoutesResult())
	}

	result := rs.Route(method, path)
	rs.cache.Set(key, result)
	return result.copyTo(acquireRoutesResult())
}

// CachedRouteCtx provides Read-Through caching for rs.Route if the cache is enabled.
func (rs *Routes) CachedRouteCtx(ctx *fasthttp.RequestCtx) *RoutesResult {
	return rs.CachedRoute(ctx.Method(), ctx.Path())
}
