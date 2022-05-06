package accesslog

import (
	"bytes"
	"testing"
	"time"
)

func Test_appendNCSADate(t *testing.T) {
	tokyo, _ := time.LoadLocation("Asia/Tokyo")
	chicago, _ := time.LoadLocation("America/Chicago")

	tests := []struct {
		date time.Time
		want []byte
	}{
		{
			date: time.Date(2006, 1, 2, 3, 4, 5, 0, tokyo),
			want: []byte("[02/Jan/2006:03:04:05 +0900]"),
		}, {
			date: time.Date(2006, 1, 2, 3, 4, 5, 0, chicago),
			want: []byte("[02/Jan/2006:03:04:05 -0600]"),
		}, {
			date: time.Date(2006, 1, 2, 3, 4, 5, 0, time.UTC),
			want: []byte("[02/Jan/2006:03:04:05 +0000]"),
		}, {
			date: time.Date(2999, 12, 31, 23, 59, 59, 0, time.UTC),
			want: []byte("[31/Dec/2999:23:59:59 +0000]"),
		},
	}
	for i, test := range tests {
		var got []byte
		got = appendNCSADate(got, test.date)
		if !bytes.Equal(got, test.want) {
			t.Errorf("tests[%d] got %q; want %q", i, got, test.want)
		}
	}
}

func Test_appendNCSARequest(t *testing.T) {
	tests := []struct {
		method   []byte
		uri      []byte
		protocol []byte
		want     []byte
	}{
		{
			method:   []byte("GET"),
			uri:      []byte("/"),
			protocol: []byte("HTTP/1.1"),
			want:     []byte(`GET / HTTP/1.1`),
		}, {
			method:   []byte("GET"),
			uri:      []byte(`/"quote"`),
			protocol: []byte("HTTP/1.1"),
			want:     []byte(`GET /\"quote\" HTTP/1.1`),
		},
	}
	for i, test := range tests {
		var got []byte
		got = appendNCSARequest(got, test.method, test.uri, test.protocol)
		if !bytes.Equal(got, test.want) {
			t.Errorf("tests[%d] got %q; want %q", i, got, test.want)
		}
	}
}
