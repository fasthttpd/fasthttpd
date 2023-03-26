package route

import (
	"bytes"
	"encoding/binary"
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
	nextIfNotFound           bool
}

// NewRoute creates a new Route by the provided rcfg.
func NewRoute(rcfg config.Route) (*Route, error) {
	r := &Route{
		filters:                  rcfg.Filters,
		rewriteUriBytes:          []byte(rcfg.Rewrite),
		rewriteAppendQueryString: rcfg.RewriteAppendQueryString,
		handler:                  rcfg.Handler,
		statusCode:               rcfg.Status,
		nextIfNotFound:           rcfg.NextIfNotFound,
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

func onResultReleased(_ util.CacheKey, value interface{}) {
	if r, ok := value.(*Result); ok {
		r.Release()
	}
}

// Routes represents a list of routes that can be used to match requested URLs.
type Routes struct {
	routes []*Route
	cache  util.Cache
	intBuf sync.Pool
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
		rs.cache.OnRelease(onResultReleased)
	}
	return rs, nil
}

func (rs *Routes) IsNextIfNotFound(i int) bool {
	if i >= len(rs.routes) {
		return false
	}
	return rs.routes[i].nextIfNotFound
}

// Route find routes by the provided method and path and returns a new Result.
func (rs *Routes) Route(method, path []byte, off int) *Result {
	result := AcquireResult()
	if off >= len(rs.routes) {
		result.StatusCode = fasthttp.StatusNotFound
		return result
	}
	for i, r := range rs.routes[off:] {
		if !r.Match(method, path) {
			continue
		}
		if len(r.filters) > 0 {
			result.Filters = result.Filters.Append(r.filters...)
		}
		result.RouteIndex = i
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

func (rs *Routes) acquireIntBuf() []byte {
	x := rs.intBuf.Get()
	if x != nil {
		return *(x.(*[]byte))
	}
	return make([]byte, 4)
}

func (rs *Routes) releaseIntBuf(b []byte) {
	for i := range b {
		b[i] = 0
	}
	rs.intBuf.Put(&b)
}

// CachedRoute provides Read-Through caching for rs.Route if the cache is enabled.
func (rs *Routes) CachedRoute(method, path []byte, off int) *Result {
	if rs.cache == nil {
		return rs.Route(method, path, off)
	}

	b := rs.acquireIntBuf()
	binary.LittleEndian.PutUint32(b, uint32(off))
	key := util.CacheKeyBytes(b, method, []byte{0}, path)
	rs.releaseIntBuf(b)

	if v := rs.cache.Get(key); v != nil {
		result := v.(*Result)
		return result.CopyTo(AcquireResult())
	}
	result := rs.Route(method, path, off)
	rs.cache.Set(key, result)
	return result.CopyTo(AcquireResult())
}

// CachedRouteCtx provides Read-Through caching for rs.Route if the cache is enabled.
func (rs *Routes) CachedRouteCtx(ctx *fasthttp.RequestCtx, off int) *Result {
	return rs.CachedRoute(ctx.Method(), ctx.Path(), off)
}
