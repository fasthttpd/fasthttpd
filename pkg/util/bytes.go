package util

// AppendZeroPaddingUint appends n with zero padding that size of p to dst
// and returns the extended dst.
func AppendZeroPaddingUint(dst []byte, n, p int) []byte {
	return AppendPaddingUint(dst, n, p, '0')
}

// AppendSpacePaddingUint appends n with space padding that size of p to dst
// and returns the extended dst.
func AppendSpacePaddingUint(dst []byte, n, p int) []byte {
	return AppendPaddingUint(dst, n, p, ' ')
}

// AppendPaddingUint appends n with c padding that size of p to dst and returns
// the extended dst.
func AppendPaddingUint(dst []byte, n, p int, c byte) []byte {
	if n < 0 {
		panic("number must be positive")
	}
	if p > 20 {
		panic("padding size must be at most 20")
	}

	var b [20]byte
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

	for o := len(b) - p; i > o; {
		i--
		buf[i] = c
	}

	return append(dst, buf[i:]...)
}
