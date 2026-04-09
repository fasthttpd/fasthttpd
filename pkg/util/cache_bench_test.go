package util

import (
	"encoding/binary"
	"testing"
)

// BenchmarkCacheKeyBuilder_RouterKey mirrors the three-field shape that
// pkg/route/route.go.CachedRoute builds for every cached lookup: a
// little-endian uint32 offset, the HTTP method, and the URL path. The
// expectation is 0 B/op, 0 allocs/op — any regression here means a
// field has started escaping again.
func BenchmarkCacheKeyBuilder_RouterKey(b *testing.B) {
	var offBuf [4]byte
	binary.LittleEndian.PutUint32(offBuf[:], 42)
	method := []byte("GET")
	path := []byte("/api/users/123")

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			kb := AcquireCacheKeyBuilder()
			kb.Write(offBuf[:])
			kb.Write(method)
			kb.Write(path)
			_ = kb.Sum()
			ReleaseCacheKeyBuilder(kb)
		}
	})
}

// BenchmarkCacheKeyOfString covers the single-field shortcut used by
// pkg/logger/accesslog. Expected: 0 B/op, 0 allocs/op.
func BenchmarkCacheKeyOfString(b *testing.B) {
	const addr = "192.168.1.1:54321"

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = CacheKeyOfString(addr)
		}
	})
}
