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
	off := CopyPaddingRightUint(b[:], n, p, c)

	return append(dst, b[off:]...)
}

// CopyPaddingRightUint copies n with c padding that size of p to the
// right side of dst, and returns coppied offset of dst.
func CopyPaddingRightUint(dst []byte, n, p int, c byte) (offset int) {
	i := len(dst)
	var q int
	for i > 0 && n >= 10 {
		i--
		q = n / 10
		dst[i] = '0' + byte(n-q*10)
		n = q
	}
	if i > 0 {
		i--
		dst[i] = '0' + byte(n)

		for o := len(dst) - p; i > o; {
			i--
			dst[i] = c
		}
	}
	return i
}

// CopyRightUint copies n to the right side of dst.
func CopyRightUint(dst []byte, n int) (offset int) {
	i := len(dst)
	var q int
	for i > 0 && n >= 10 {
		i--
		q = n / 10
		dst[i] = '0' + byte(n-q*10)
		n = q
	}
	if i > 0 {
		i--
		dst[i] = '0' + byte(n)
	}
	return i
}
