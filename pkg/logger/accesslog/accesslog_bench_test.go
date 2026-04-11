package accesslog

import (
	"io"
	"net"
	"os"
	"testing"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/valyala/fasthttp"
)

func benchmarkAccessLog(b *testing.B, format string, newCtx func() *fasthttp.RequestCtx) {
	benchmarkAccessLogWithSink(b, format, newCtx, io.Discard)
}

// benchmarkAccessLogWithSink drives the Common/Combined workload against an
// arbitrary sink. The default io.Discard variant measures in-process overhead
// only; real-file and /dev/null variants expose the cost of the write-through
// path once the channel/goroutine pipeline is replaced in later sub-PRs.
func benchmarkAccessLogWithSink(b *testing.B, format string, newCtx func() *fasthttp.RequestCtx, w io.Writer) {
	cfg := config.Config{
		Host: "localhost",
		AccessLog: config.AccessLog{
			Format: format,
		},
	}

	l, err := newAccessLog(&logger.NopRotator{Writer: w}, cfg)
	if err != nil {
		b.Fatalf("unexpected error: %v", err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		ctx := newCtx()
		for pb.Next() {
			l.Collect(ctx)
			l.Log(ctx)
		}
	})
}

// openBenchTempFile creates a temporary file scoped to the benchmark. The file
// is closed and removed when the benchmark ends.
func openBenchTempFile(b *testing.B) *os.File {
	b.Helper()
	f, err := os.CreateTemp(b.TempDir(), "accesslog-bench-*.log")
	if err != nil {
		b.Fatalf("CreateTemp: %v", err)
	}
	b.Cleanup(func() {
		f.Close() //nolint:errcheck // bench cleanup
	})
	return f
}

// openBenchDevNull opens /dev/null (or the platform equivalent) as a *os.File
// so the write path exercises a real file descriptor without the cost of
// persistent filesystem writes. Useful for isolating syscall overhead.
func openBenchDevNull(b *testing.B) *os.File {
	b.Helper()
	f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		b.Fatalf("open %s: %v", os.DevNull, err)
	}
	b.Cleanup(func() {
		f.Close() //nolint:errcheck // bench cleanup
	})
	return f
}

func commonCtx() *fasthttp.RequestCtx {
	remoteIP := net.IPv4(10, 1, 2, 3)
	remoteAddr := &net.TCPAddr{IP: remoteIP, Port: 1234}
	ctx := &fasthttp.RequestCtx{}
	ctx.SetRemoteAddr(remoteAddr)
	ctx.Request.SetRequestURI("/path")
	ctx.URI().SetUsername("foo")
	ctx.Response.SetBody([]byte("body"))
	ctx.Response.Header.SetContentLength(4)
	return ctx
}

func combinedCtx() *fasthttp.RequestCtx {
	ctx := commonCtx()
	ctx.Request.Header.Set("User-Agent", "accesslog_test")
	ctx.Request.Header.Set("Referer", "/referer")
	return ctx
}

func simpleCtx() *fasthttp.RequestCtx {
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/path")
	return ctx
}

func BenchmarkAccessLog_File(b *testing.B) {
	benchmarkAccessLog(b, "%f", simpleCtx)
}

func BenchmarkAccessLog_Request(b *testing.B) {
	benchmarkAccessLog(b, "%r", simpleCtx)
}

func BenchmarkAccessLog_Time(b *testing.B) {
	benchmarkAccessLog(b, "%t", simpleCtx)
}

func BenchmarkAccessLog_Common(b *testing.B) {
	benchmarkAccessLog(b, FormatCommon, commonCtx)
}

func BenchmarkAccessLog_Common_DevNull(b *testing.B) {
	benchmarkAccessLogWithSink(b, FormatCommon, commonCtx, openBenchDevNull(b))
}

func BenchmarkAccessLog_Common_TempFile(b *testing.B) {
	benchmarkAccessLogWithSink(b, FormatCommon, commonCtx, openBenchTempFile(b))
}

func BenchmarkAccessLog_Combined(b *testing.B) {
	benchmarkAccessLog(b, FormatCombined, combinedCtx)
}

func BenchmarkAccessLog_Combined_DevNull(b *testing.B) {
	benchmarkAccessLogWithSink(b, FormatCombined, combinedCtx, openBenchDevNull(b))
}

func BenchmarkAccessLog_Combined_TempFile(b *testing.B) {
	benchmarkAccessLogWithSink(b, FormatCombined, combinedCtx, openBenchTempFile(b))
}

func BenchmarkAccessLog_RemoteAddr(b *testing.B) {
	benchmarkAccessLog(b, "%a", commonCtx)
}

func BenchmarkAccessLog_LocalAddr(b *testing.B) {
	benchmarkAccessLog(b, "%A", commonCtx)
}

func BenchmarkAccessLog_Port(b *testing.B) {
	benchmarkAccessLog(b, "%p", commonCtx)
}

func BenchmarkAccessLog_RemotePort(b *testing.B) {
	benchmarkAccessLog(b, "%{remote}p", commonCtx)
}

func BenchmarkAccessLog_JSON(b *testing.B) {
	benchmarkAccessLog(b, FormatJSON, combinedCtx)
}

func BenchmarkAccessLog_JSON_DevNull(b *testing.B) {
	benchmarkAccessLogWithSink(b, FormatJSON, combinedCtx, openBenchDevNull(b))
}

func BenchmarkAccessLog_JSON_TempFile(b *testing.B) {
	benchmarkAccessLogWithSink(b, FormatJSON, combinedCtx, openBenchTempFile(b))
}

func BenchmarkAccessLog_LTSV(b *testing.B) {
	benchmarkAccessLog(b, FormatLTSV, combinedCtx)
}

func BenchmarkAccessLog_LTSV_DevNull(b *testing.B) {
	benchmarkAccessLogWithSink(b, FormatLTSV, combinedCtx, openBenchDevNull(b))
}

func BenchmarkAccessLog_LTSV_TempFile(b *testing.B) {
	benchmarkAccessLogWithSink(b, FormatLTSV, combinedCtx, openBenchTempFile(b))
}
