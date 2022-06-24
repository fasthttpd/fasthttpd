package accesslog

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/valyala/fasthttp"
)

func TestNewAccessLog(t *testing.T) {
	tmp, err := os.CreateTemp("", "*.log")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.Remove(tmp.Name())

	tests := []struct {
		cfg config.Config
	}{
		{
			cfg: config.Config{},
		}, {
			cfg: config.Config{
				AccessLog: config.AccessLog{
					Output: "stdout",
				},
			},
		}, {
			cfg: config.Config{
				AccessLog: config.AccessLog{
					Output: "stderr",
				},
			},
		}, {
			cfg: config.Config{
				AccessLog: config.AccessLog{
					Output: tmp.Name(),
				},
			},
		},
	}
	for i, test := range tests {
		l, err := NewAccessLog(test.cfg)
		if err != nil {
			t.Fatalf("tests[%d] unexpected error: %v", i, err)
		}
		l.Close()
	}
}

func TestAccessLog_appendNCSADate(t *testing.T) {
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

func TestAccessLog_appendNCSARequest(t *testing.T) {
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

func TestAccessLog(t *testing.T) {
	remoteIP := net.IPv4(10, 1, 2, 3)
	remoteAddr := &net.TCPAddr{IP: remoteIP, Port: 1234}

	envKey := "ACCESSLOG_TEST"
	envValue := "env-value"
	os.Setenv(envKey, envValue)
	defer os.Unsetenv(envKey)

	osHost, err := os.Hostname()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	timeNowOrg := timeNow
	defer func() { timeNow = timeNowOrg }()
	timeNow = func() time.Time {
		var zeroTime time.Time
		return zeroTime.Add(9876543 * time.Microsecond)
	}

	ctx0 := &fasthttp.RequestCtx{}
	req0 := &fasthttp.Request{}
	ctx0.Init(req0, remoteAddr, logger.NilLogger)

	tests := []struct {
		cfg  config.Config
		ctx  func() *fasthttp.RequestCtx
		want string
	}{
		{
			cfg: config.Config{
				AccessLog: config.AccessLog{},
			},
			ctx: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.SetRemoteAddr(remoteAddr)
				ctx.Request.SetRequestURI("/path")
				ctx.URI().SetUsername("foo")
				ctx.Response.SetBody([]byte("body"))
				ctx.Response.Header.SetContentLength(4)
				return ctx
			},
			want: `10.1.2.3 - foo [01/Jan/0001:00:00:00 +0000] "GET /path HTTP/1.1" 200 4`,
		}, {
			cfg: config.Config{
				AccessLog: config.AccessLog{
					Format: FormatCombined,
				},
			},
			ctx: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.SetRemoteAddr(remoteAddr)
				ctx.Request.SetRequestURI("/path")
				ctx.URI().SetUsername("foo")
				ctx.Request.Header.Set("User-Agent", "accesslog_test")
				ctx.Request.Header.Set("Referer", "/referer")
				ctx.Response.SetBody([]byte("body"))
				ctx.Response.Header.SetContentLength(4)
				return ctx
			},
			want: `10.1.2.3 - foo [01/Jan/0001:00:00:00 +0000] "GET /path HTTP/1.1" 200 4 "/referer" "accesslog_test"`,
		}, {
			cfg: config.Config{
				AccessLog: config.AccessLog{
					Format: "%% %Y %{cookie}C %{ACCESSLOG_TEST}e %{Request-Header}i %{Response-Header}o",
				},
			},
			ctx: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.SetRemoteAddr(remoteAddr)
				ctx.Request.SetRequestURI("/path")
				ctx.URI().SetUsername("foo")
				ctx.Request.Header.Set("Request-Header", "request-header-value")
				ctx.Request.Header.Set("Cookie", "cookie=cookie-value")
				ctx.Response.SetBody([]byte("body"))
				ctx.Response.Header.SetContentLength(4)
				ctx.Response.Header.Set("Response-Header", "response-header-value")
				return ctx
			},
			want: "% %Y cookie-value " + envValue + " request-header-value response-header-value",
		}, {
			cfg: config.Config{
				Host: "example.com",
				AccessLog: config.AccessLog{
					Format: "%p %{remote}p %v %V",
				},
			},
			ctx: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.SetRemoteAddr(remoteAddr)
				ctx.Request.SetRequestURI("/path")
				ctx.URI().SetUsername("foo")
				ctx.Request.Header.Set("Request-Header", "request-header-value")
				ctx.Request.Header.Set("Cookie", "cookie=cookie-value")
				ctx.Response.SetBody([]byte("body"))
				ctx.Response.Header.SetContentLength(4)
				ctx.Response.Header.Set("Response-Header", "response-header-value")
				return ctx
			},
			want: "- 1234 example.com " + osHost,
		}, {
			cfg: config.Config{
				Host: "example.com",
				AccessLog: config.AccessLog{
					Format: "%a %A %B %D %T %f %H %m %q %U %X %I %O %S",
				},
			},
			ctx: func() *fasthttp.RequestCtx {
				ctx := &fasthttp.RequestCtx{}
				ctx.SetRemoteAddr(remoteAddr)
				rawRequest := "GET /path?foo=bar HTTP/1.1\r\n"
				rawRequest += "Host: example.com\r\n"
				rawRequest += "Request-Header: request-header-value\r\n"
				rawRequest += "Cookie: cookie=cookie-value\r\n"
				rawRequest += "Content-Length: 12\r\n\r\n"
				rawRequest += "request-body"
				if err := ctx.Request.Read(bufio.NewReader(strings.NewReader(rawRequest))); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				ctx.URI().SetUsername("foo")
				ctx.Response.SetBody([]byte("body"))
				ctx.Response.Header.SetContentLength(4)
				ctx.Response.Header.Set("Response-Header", "response-header-value")
				return ctx
			},
			want: "10.1.2.3:1234 0.0.0.0:0 4 9876543 9 /path HTTP/1.1 GET ?foo=bar /path + 120 160 280",
		}, {
			cfg: config.Config{
				Host: "example.com",
				AccessLog: config.AccessLog{
					Format: "%P %L %k %q %u %b %{Unknown}C %{Unknown}e %{Unknown}i %{Unknown}o",
				},
			},
			ctx: func() *fasthttp.RequestCtx {
				return ctx0
			},
			want: fmt.Sprintf("%d %d 0 - - - - - - -", os.Getegid(), ctx0.ID()),
		}, {
			cfg: config.Config{
				AccessLog: config.AccessLog{
					Format: "%{%Y-%m-%d %H:%I:%S %a %P}t",
				},
			},
			ctx: func() *fasthttp.RequestCtx {
				return &fasthttp.RequestCtx{}
			},
			want: `0001-01-01 00:00:00 Mon AM`,
		},
	}
	out := new(bytes.Buffer)
	for i, test := range tests {
		out.Reset()
		ctx := test.ctx()

		l, err := newAccessLog(&logger.NopRotator{Writer: out}, test.cfg)
		if err != nil {
			l.Close()
			t.Fatalf("tests[%d] unexpected error: %v", i, err)
		}
		l.Collect(ctx)
		l.Log(ctx)

		got := strings.TrimRight(out.String(), "\n")
		if got != test.want {
			t.Errorf("tests[%d] got %q; want %q", i, got, test.want)
		}
		l.Close()
	}
}
