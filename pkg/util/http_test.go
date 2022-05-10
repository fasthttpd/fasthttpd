package util

import (
	"bytes"
	"net/http"
	"testing"
)

func Test_IsHttpOrHttps(t *testing.T) {
	tests := []struct {
		uri  []byte
		want bool
	}{
		{
			uri:  []byte("http://example.com/"),
			want: true,
		}, {
			uri:  []byte("https://example.com/"),
			want: true,
		}, {
			uri:  []byte("file://path/to/file"),
			want: false,
		},
	}
	for i, test := range tests {
		got := IsHttpOrHttps(test.uri)
		if got != test.want {
			t.Errorf("tests[%d] got %v; want %v", i, got, test.want)
		}
	}
}

func Test_IsHttpStatusRedirect(t *testing.T) {
	tests := []struct {
		status int
		want   bool
	}{
		{
			status: 0,
			want:   false,
		}, {
			status: http.StatusMovedPermanently,
			want:   true,
		}, {
			status: http.StatusFound,
			want:   true,
		}, {
			status: http.StatusSeeOther,
			want:   true,
		}, {
			status: http.StatusTemporaryRedirect,
			want:   true,
		}, {
			status: http.StatusPermanentRedirect,
			want:   true,
		}, {
			status: http.StatusOK,
			want:   false,
		},
	}
	for i, test := range tests {
		got := IsHttpStatusRedirect(test.status)
		if got != test.want {
			t.Errorf("tests[%d] got %v; want %v", i, got, test.want)
		}
	}
}

func Test_SplitRequestURI(t *testing.T) {
	tests := []struct {
		uri  []byte
		path []byte
		qstr []byte
	}{
		{
			uri:  []byte("/path?query=string"),
			path: []byte("/path"),
			qstr: []byte("query=string"),
		}, {
			uri:  []byte("/path"),
			path: []byte("/path"),
		},
	}
	for i, test := range tests {
		path, qstr := SplitRequestURI(test.uri)
		if !bytes.Equal(path, test.path) {
			t.Errorf("tests[%d] unexpected path %s; want %s", i, path, test.path)
		}
		if !bytes.Equal(qstr, test.qstr) {
			t.Errorf("tests[%d] unexpected query string %s; want %s", i, qstr, test.qstr)
		}
	}
}

func Test_AppendQueryString(t *testing.T) {
	tests := []struct {
		dst  []byte
		qstr []byte
		want []byte
	}{
		{
			qstr: []byte("a=1"),
			want: []byte("?a=1"),
		}, {
			dst:  []byte("/path"),
			qstr: []byte("a=1"),
			want: []byte("/path?a=1"),
		}, {
			dst:  []byte("/path?a=1"),
			qstr: []byte("b=2"),
			want: []byte("/path?a=1&b=2"),
		},
	}
	for i, test := range tests {
		got := AppendQueryString(test.dst, test.qstr)
		if !bytes.Equal(got, test.want) {
			t.Errorf("tests[%d] unexpected dst %s; want %s", i, got, test.want)
		}
	}
}
