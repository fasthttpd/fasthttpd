package util

import (
	"bytes"
	"testing"
)

func Test_AppendZeroPaddingUint(t *testing.T) {
	tests := []struct {
		n    int
		p    int
		want []byte
	}{
		{
			n:    1,
			p:    2,
			want: []byte("01"),
		}, {
			n:    12,
			p:    3,
			want: []byte("012"),
		}, {
			n:    123,
			p:    1,
			want: []byte("123"),
		},
	}
	for i, test := range tests {
		var got []byte
		got = AppendZeroPaddingUint(got, test.n, test.p)
		if !bytes.Equal(got, test.want) {
			t.Errorf("tests[%d] got %q; want %q", i, got, test.want)
		}
	}
}
