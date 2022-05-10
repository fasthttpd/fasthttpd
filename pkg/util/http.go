package util

import (
	"bytes"
	"net/http"
)

var (
	HttpProtocol  = []byte("http://")
	HttpsProtocol = []byte("https://")
)

func IsHttpOrHttps(uri []byte) bool {
	return bytes.HasPrefix(uri, HttpProtocol) || bytes.HasPrefix(uri, HttpsProtocol)
}

func IsHttpStatusRedirect(status int) bool {
	switch status {
	case 0:
		return false
	case http.StatusMovedPermanently,
		http.StatusFound,
		http.StatusSeeOther,
		http.StatusTemporaryRedirect,
		http.StatusPermanentRedirect:
		return true
	}
	return false
}

// SplitRequestURI splits path and query string.
func SplitRequestURI(uri []byte) ([]byte, []byte) {
	q := bytes.Index(uri, []byte{'?'})
	if q == -1 {
		return uri, nil
	}
	return uri[:q], uri[q+1:]
}

// AppendQueryString appends qstr to dst.
func AppendQueryString(dst, qstr []byte) []byte {
	q := bytes.Index(dst, []byte{'?'})
	if q == -1 {
		dst = append(dst, '?')
	} else {
		dst = append(dst, '&')
	}
	return append(dst, qstr...)
}
