package util

import (
	"bytes"
	"math"
)

// AppendZeroPaddingUint appends n with zero padding that size of p to dst
// and returns the extended dst.
func AppendZeroPaddingUint(dst []byte, n, p int) []byte {
	if n < 0 {
		panic("int must be positive")
	}
	if p < 1 {
		panic("padding size must be at least 1")
	}

	if q := int(math.Log10(float64(n))) + 1; p < q {
		p = q
	}

	b := make([]byte, p)
	buf := b[:]
	i := len(buf)
	var q int
	for n >= 10 {
		i--
		q = n / 10
		buf[i] = '0' + byte(n-q*10)
		n = q
	}
	i--
	buf[i] = '0' + byte(n)

	for j := 0; j < i; j++ {
		buf[j] = '0'
	}
	return append(dst, buf...)
}

// AppendQueryString appends queryString to dst.
func AppendQueryString(dst, queryString []byte) []byte {
	q := bytes.Index(dst, []byte{'?'})
	if q == -1 {
		dst = append(dst, '?')
	} else {
		dst = append(dst, '&')
	}
	return append(dst, queryString...)
}
