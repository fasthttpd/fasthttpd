package route

import (
	"net/http"
	"testing"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/jarxorg/tree"
)

var (
	benchmarkRoutesConfig = config.Config{
		Handlers: map[string]tree.Map{
			"equal":  {},
			"prefix": {},
			"regexp": {},
		},
		Routes: []config.Route{
			{
				Path:    "/",
				Match:   config.MatchEqual,
				Handler: "equal",
			}, {
				Path:    "/img/",
				Match:   config.MatchPrefix,
				Handler: "prefix",
			}, {
				Path:    `.*\.(js|css|jpg|png|gif|ico)$`,
				Match:   config.MatchRegexp,
				Handler: "regexp",
			},
		},
		RoutesCache: config.RoutesCache{
			Enable: true,
		},
	}
	benchmarkRoutes, _ = NewRoutes(benchmarkRoutesConfig)
)

func benchmarkRoute(b *testing.B, method, path []byte, handler string) {
	got := benchmarkRoutes.Route(method, path)
	if got.Handler != handler {
		b.Errorf("unexpected route result %#v", got)
	}
	got.Release()
}

func benchmarkCachedRoute(b *testing.B, method, path []byte, handler string) {
	got := benchmarkRoutes.CachedRoute(method, path)
	if got.Handler != handler {
		b.Errorf("unexpected route result %#v", got)
	}
	got.Release()
}

func BenchmarkRoutes_Equal(b *testing.B) {
	method := []byte(http.MethodGet)
	path := []byte("/")
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			benchmarkRoute(b, method, path, "equal")
		}
	})
}

func BenchmarkCachedRoutes_Equal(b *testing.B) {
	method := []byte(http.MethodGet)
	path := []byte("/")
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			benchmarkCachedRoute(b, method, path, "equal")
		}
	})
}

func BenchmarkRoutes_Prefix(b *testing.B) {
	method := []byte(http.MethodGet)
	path := []byte("/img/test.png")
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			benchmarkRoute(b, method, path, "prefix")
		}
	})
}

func BenchmarkCachedRoutes_Prefix(b *testing.B) {
	method := []byte(http.MethodGet)
	path := []byte("/img/test.png")
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			benchmarkCachedRoute(b, method, path, "prefix")
		}
	})
}

func BenchmarkRoutes_Regexp(b *testing.B) {
	method := []byte(http.MethodGet)
	path := []byte("/favicon.ico")
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			benchmarkRoute(b, method, path, "regexp")
		}
	})
}

func BenchmarkCachedRoutes_Regexp(b *testing.B) {
	method := []byte(http.MethodGet)
	path := []byte("/favicon.ico")
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			benchmarkCachedRoute(b, method, path, "regexp")
		}
	})
}
