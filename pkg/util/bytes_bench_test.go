package util

import (
	"bytes"
	"testing"
)

func BenchmarkAppendPaddingUint(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		var got []byte
		want := []byte("000012345")
		for pb.Next() {
			got = AppendPaddingUint(got[:0], 12345, 9, '0')
			if !bytes.Equal(want, got) {
				b.Errorf("unexpected result %s; want %s", got, want)
			}
		}
	})
}
