package util

import (
	"testing"

	"github.com/valyala/bytebufferpool"
)

func benchmarkBytesToKey(b *testing.B, fn func(bs ...[]byte) interface{}) {
	b.RunParallel(func(pb *testing.PB) {
		bs := [][]byte{
			[]byte("GET"),
			[]byte(" "),
			{0},
		}
		i := 0
		for pb.Next() {
			i++
			bs[2][0] = byte(i % 256)
			fn(bs...)
		}
	})
}

func BenchmarkBytesToKey_CacheKeyBytes(b *testing.B) {
	fn := func(bs ...[]byte) interface{} {
		return CacheKeyBytes(bs...)
	}
	benchmarkBytesToKey(b, fn)
}

func BenchmarkBytesToKey_BytebufferpoolString(b *testing.B) {
	fn := func(bs ...[]byte) interface{} {
		p := bytebufferpool.Get()
		for _, bb := range bs {
			p.B = append(p.B, bb...)
		}
		key := string(p.B)
		bytebufferpool.Put(p)
		return key
	}
	benchmarkBytesToKey(b, fn)
}
