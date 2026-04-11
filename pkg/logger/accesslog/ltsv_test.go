package accesslog

import (
	"bytes"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/mojatter/io2"
	"github.com/valyala/fasthttp"
)

func TestAppendLTSVValue(t *testing.T) {
	testCases := []struct {
		caseName string
		in       string
		want     string
	}{
		{caseName: "empty", in: "", want: ""},
		{caseName: "ascii", in: "hello", want: "hello"},
		{caseName: "tab", in: "a\tb", want: "a b"},
		{caseName: "newline", in: "a\nb", want: "a b"},
		{caseName: "cr", in: "a\rb", want: "a b"},
		{caseName: "crlf", in: "a\r\nb", want: "a  b"},
		{caseName: "mixed", in: "a\tb\nc\rd", want: "a b c d"},
		{caseName: "colon_passthrough", in: "a:b", want: "a:b"},
		{caseName: "space_passthrough", in: "a b c", want: "a b c"},
		{caseName: "non_ascii_utf8", in: "日本語", want: "日本語"},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			got := string(appendLTSVValue(nil, []byte(tc.in)))
			if got != tc.want {
				t.Errorf("appendLTSVValue(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestAppendHostIP(t *testing.T) {
	testCases := []struct {
		caseName string
		addr     net.Addr
		want     string
	}{
		{
			caseName: "ipv4",
			addr:     &net.TCPAddr{IP: net.IPv4(10, 1, 2, 3), Port: 51002},
			want:     "10.1.2.3",
		},
		{
			caseName: "ipv6",
			addr:     &net.TCPAddr{IP: net.ParseIP("2001:db8::1"), Port: 51002},
			want:     "2001:db8::1",
		},
		{
			caseName: "non_tcp",
			addr:     &net.UnixAddr{Name: "/tmp/sock", Net: "unix"},
			want:     "",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			got := string(appendHostIP(nil, tc.addr))
			if got != tc.want {
				t.Errorf("appendHostIP = %q; want %q", got, tc.want)
			}
		})
	}
}

// parseLTSVLine parses a single LTSV line into a map. It mirrors the
// semantics of TestAccessLog_LTSV and is not meant to be a full LTSV
// implementation.
func parseLTSVLine(line string) map[string]string {
	out := map[string]string{}
	for field := range strings.SplitSeq(line, "\t") {
		k, v, ok := strings.Cut(field, ":")
		if !ok {
			continue
		}
		out[k] = v
	}
	return out
}

func TestAccessLog_LTSV(t *testing.T) {
	timeNowOrg := timeNow
	defer func() { timeNow = timeNowOrg }()

	// fasthttp.RequestCtx exposes Time() but no setter, so the request
	// time is the zero value here. timeNow is mocked to make
	// reqtime_microsec deterministic against that zero value.
	timeNow = func() time.Time {
		var zeroTime time.Time
		return zeroTime.Add(1234 * time.Microsecond)
	}
	wantTime := "0001-01-01T00:00:00Z"

	remoteAddr := &net.TCPAddr{IP: net.IPv4(10, 1, 2, 3), Port: 51002}

	testCases := []struct {
		caseName string
		setup    func(ctx *fasthttp.RequestCtx)
		want     map[string]string
	}{
		{
			caseName: "minimal_get",
			setup: func(ctx *fasthttp.RequestCtx) {
				ctx.Request.SetRequestURI("/path?foo=bar")
				ctx.Request.SetHost("localhost")
				ctx.Response.SetBody([]byte("body"))
				ctx.Response.Header.SetContentLength(4)
			},
			want: map[string]string{
				"time":             wantTime,
				"host":             "10.1.2.3",
				"forwardedfor":     "",
				"user":             "",
				"req":              "GET /path?foo=bar HTTP/1.1",
				"scheme":           "http",
				"vhost":            "localhost",
				"status":           "200",
				"size":             "4",
				"reqsize":          "0",
				"reqtime_microsec": "1234",
				"referer":          "",
				"ua":               "",
			},
		},
		{
			caseName: "with_user_forwardedfor_ua",
			setup: func(ctx *fasthttp.RequestCtx) {
				ctx.Request.SetRequestURI("/")
				ctx.Request.SetHost("example.com")
				ctx.URI().SetUsername("alice")
				ctx.Request.Header.Set("X-Forwarded-For", "203.0.113.7, 198.51.100.1")
				ctx.Request.Header.Set("Referer", "http://example.com/prev")
				ctx.Request.Header.Set("User-Agent", "Mozilla/5.0")
			},
			want: map[string]string{
				"time":             wantTime,
				"host":             "10.1.2.3",
				"forwardedfor":     "203.0.113.7, 198.51.100.1",
				"user":             "alice",
				"req":              "GET / HTTP/1.1",
				"scheme":           "http",
				"vhost":            "example.com",
				"status":           "200",
				"size":             "0",
				"reqsize":          "0",
				"reqtime_microsec": "1234",
				"referer":          "http://example.com/prev",
				"ua":               "Mozilla/5.0",
			},
		},
		{
			caseName: "tab_in_header_replaced_with_space",
			setup: func(ctx *fasthttp.RequestCtx) {
				ctx.Request.SetRequestURI("/")
				ctx.Request.SetHost("localhost")
				// A literal TAB inside a header value must be sanitized
				// so it does not break the LTSV line structure.
				ctx.Request.Header.Set("User-Agent", "bad\tagent")
			},
			want: map[string]string{
				"time":             wantTime,
				"host":             "10.1.2.3",
				"forwardedfor":     "",
				"user":             "",
				"req":              "GET / HTTP/1.1",
				"scheme":           "http",
				"vhost":            "localhost",
				"status":           "200",
				"size":             "0",
				"reqsize":          "0",
				"reqtime_microsec": "1234",
				"referer":          "",
				"ua":               "bad agent",
			},
		},
	}

	wroteCh := make(chan bool, 1)
	buf := new(bytes.Buffer)
	out := &io2.Delegator{
		WriteFunc: func(p []byte) (n int, err error) {
			n, err = buf.Write(p)
			wroteCh <- true
			return
		},
	}

	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			buf.Reset()
			ctx := &fasthttp.RequestCtx{}
			ctx.SetRemoteAddr(remoteAddr)
			tc.setup(ctx)

			cfg := config.Config{AccessLog: config.AccessLog{
				Format: FormatLTSV,
				// Flush every Log() so wroteCh fires immediately.
				BufferSize: 1,
			}}
			l, err := newAccessLog(&logger.NopRotator{Writer: out}, cfg)
			if err != nil {
				t.Fatalf("newAccessLog: %v", err)
			}
			defer l.Close()

			l.Collect(ctx)
			l.Log(ctx)
			<-wroteCh

			line := strings.TrimRight(buf.String(), "\n")
			if strings.ContainsAny(line, "\n\r") {
				t.Errorf("LTSV line contains unsanitized newline: %q", line)
			}
			got := parseLTSVLine(line)
			// Every expected field must match exactly, and no
			// unexpected fields should appear.
			if len(got) != len(tc.want) {
				t.Errorf("field count mismatch: got %d (%v), want %d", len(got), got, len(tc.want))
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("field %q = %q; want %q\nraw: %s", k, got[k], v, line)
				}
			}
		})
	}
}
