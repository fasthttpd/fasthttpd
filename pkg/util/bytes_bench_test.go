package util

import (
	"testing"
)

func BenchmarkAppendPaddingUint(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		var buf []byte
		i := 0
		for pb.Next() {
			i++
			buf = AppendPaddingUint(buf[:0], i, 9, '0')
			if len(buf) != 9 {
				b.Errorf("unexpected length %d", len(buf))
			}
		}
	})
}
