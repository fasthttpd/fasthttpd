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
	var got []byte
	for i, test := range tests {
		got = AppendZeroPaddingUint(got[:0], test.n, test.p)
		if !bytes.Equal(got, test.want) {
			t.Errorf("tests[%d] got %q; want %q", i, got, test.want)
		}
	}
}

func Test_AppendZeroPaddingUint_panics(t *testing.T) {
	tests := []struct {
		n      int
		p      int
		errstr string
	}{
		{
			n:      -1,
			errstr: "number must be positive",
		}, {
			p:      21,
			errstr: "padding size must be at most 20",
		},
	}
	fn := func(i, n, p int, errstr string) {
		defer func() {
			err := recover()
			if err == nil {
				t.Fatalf("tests[%d] unexpected no error", i)
			}
			if err != errstr {
				t.Errorf("tests[%d] unexpected error %q; want %q", i, err, errstr)
			}
		}()
		AppendZeroPaddingUint(nil, n, p)
	}
	for i, test := range tests {
		fn(i, test.n, test.p, test.errstr)
	}
}
