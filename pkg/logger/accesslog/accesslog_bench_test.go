package accesslog

import (
	"io"
	"net"
	"testing"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/valyala/fasthttp"
)

func benchmarkAccessLog(b *testing.B, format string, ctx *fasthttp.RequestCtx) {
	cfg := config.Config{
		Host: "localhost",
		AccessLog: config.AccessLog{
			Format: format,
		},
	}

	l, err := newAccessLog(&logger.NopRotator{Writer: io.Discard}, cfg)
	if err != nil {
		b.Fatalf("unexpected error: %v", err)
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			l.Collect(ctx)
			l.Log(ctx)
		}
	})
}

func BenchmarkAccessLog_File(b *testing.B) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/path")

	benchmarkAccessLog(b, "%f", ctx)
}

func BenchmarkAccessLog_Request(b *testing.B) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/path")

	benchmarkAccessLog(b, "%r", ctx)
}

func BenchmarkAccessLog_Time(b *testing.B) {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/path")

	benchmarkAccessLog(b, "%t", ctx)
}

func BenchmarkAccessLog_Common(b *testing.B) {
	remoteIP := net.IPv4(10, 1, 2, 3)
	remoteAddr := &net.TCPAddr{IP: remoteIP, Port: 1234}
	ctx := &fasthttp.RequestCtx{}
	ctx.SetRemoteAddr(remoteAddr)
	ctx.Request.SetRequestURI("/path")
	ctx.URI().SetUsername("foo")
	ctx.Response.SetBody([]byte("body"))
	ctx.Response.Header.SetContentLength(4)

	benchmarkAccessLog(b, FormatCommon, ctx)
}

func BenchmarkAccessLog_Combined(b *testing.B) {
	remoteIP := net.IPv4(10, 1, 2, 3)
	remoteAddr := &net.TCPAddr{IP: remoteIP, Port: 1234}
	ctx := &fasthttp.RequestCtx{}
	ctx.SetRemoteAddr(remoteAddr)
	ctx.Request.SetRequestURI("/path")
	ctx.URI().SetUsername("foo")
	ctx.Request.Header.Set("User-Agent", "accesslog_test")
	ctx.Request.Header.Set("Referer", "/referer")
	ctx.Response.SetBody([]byte("body"))
	ctx.Response.Header.SetContentLength(4)

	benchmarkAccessLog(b, FormatCombined, ctx)
}
