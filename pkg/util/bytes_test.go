package util

import (
	"bytes"
	"testing"
)

func Test_AppendZeroPaddingUint(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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

func Test_CopyPaddingRightUint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		dst        []byte
		n          int
		p          int
		c          byte
		want       []byte
		wantOffset int
	}{
		{
			dst:        make([]byte, 3),
			n:          1,
			p:          2,
			c:          '0',
			want:       []byte{0, '0', '1'},
			wantOffset: 1,
		}, {
			dst:        []byte{'a', 'b', 'c'},
			n:          1,
			p:          -1,
			c:          0,
			want:       []byte{'a', 'b', '1'},
			wantOffset: 2,
		}, {
			dst:        make([]byte, 3),
			n:          1234,
			p:          -1,
			c:          0,
			want:       []byte{'2', '3', '4'},
			wantOffset: 0,
		},
	}
	for i, test := range tests {
		got := test.dst
		gotOffset := CopyPaddingRightUint(got, test.n, test.p, test.c)
		if gotOffset != test.wantOffset {
			t.Errorf("tests[%d] got offset %d; want offset %d", i, gotOffset, test.wantOffset)
		}
		if !bytes.Equal(got, test.want) {
			t.Errorf("tests[%d] got %q; want %q", i, got, test.want)
		}
	}
}

func Test_CopyRightUint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		dst        []byte
		n          int
		want       []byte
		wantOffset int
	}{
		{
			dst:        make([]byte, 3),
			n:          1,
			want:       []byte{0, 0, '1'},
			wantOffset: 2,
		}, {
			dst:        []byte{'a', 'b', 'c'},
			n:          1,
			want:       []byte{'a', 'b', '1'},
			wantOffset: 2,
		}, {
			dst:        make([]byte, 3),
			n:          1234,
			want:       []byte{'2', '3', '4'},
			wantOffset: 0,
		},
	}
	for i, test := range tests {
		got := test.dst
		gotOffset := CopyRightUint(got, test.n)
		if gotOffset != test.wantOffset {
			t.Errorf("tests[%d] got offset %d; want offset %d", i, gotOffset, test.wantOffset)
		}
		if !bytes.Equal(got, test.want) {
			t.Errorf("tests[%d] got %q; want %q", i, got, test.want)
		}
	}
}

func Test_Bytes2DEqual(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a    [][]byte
		b    [][]byte
		want bool
	}{
		{
			a:    nil,
			b:    nil,
			want: true,
		}, {
			a:    nil,
			b:    [][]byte{},
			want: true,
		}, {
			a:    [][]byte{{'a', 'b'}},
			b:    [][]byte{{'a', 'b'}},
			want: true,
		}, {
			a:    [][]byte{{'a', 'b'}},
			b:    [][]byte{{'a'}},
			want: false,
		}, {
			a:    [][]byte{{'a', 'b'}},
			b:    [][]byte{{'a', 'B'}},
			want: false,
		},
	}
	for i, test := range tests {
		got := Bytes2DEqual(test.a, test.b)
		if got != test.want {
			t.Errorf("tests[%d] got %v; want %v", i, got, test.want)
		}
	}
}
