package util

import (
	"bytes"
	"net/http"
)

var (
	HttpProtocol  = []byte("http://")
	HttpsProtocol = []byte("https://")
)

func IsHttpOrHttps(u []byte) bool {
	return bytes.HasPrefix(u, HttpProtocol) || bytes.HasPrefix(u, HttpsProtocol)
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
