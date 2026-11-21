package accesslog

import (
	"io"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/fasthttpd/fasthttpd/pkg/config"
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

// unreadReader fails the test when read.
type unreadReader struct{ t *testing.T }

func (r unreadReader) Read([]byte) (int, error) {
	r.t.Error("body stream read")
	return 0, io.EOF
}

func TestAccessLog_ResponseSize(t *testing.T) {
	stream := func(size int) func(t *testing.T, ctx *fasthttp.RequestCtx) {
		return func(t *testing.T, ctx *fasthttp.RequestCtx) {
			ctx.Response.SetBodyStream(unreadReader{t}, size)
		}
	}
	body := func(t *testing.T, ctx *fasthttp.RequestCtx) {
		ctx.Response.SetBodyString("body")
	}
	head := func(t *testing.T, ctx *fasthttp.RequestCtx) {
		ctx.Request.Header.SetMethod(fasthttp.MethodHead)
		// As fasthttp.FS answers HEAD.
		ctx.Response.SkipBody = true
		ctx.Response.Header.SetContentLength(4)
	}
	headerSize := func(n int) func(ctx *fasthttp.RequestCtx) string {
		return func(ctx *fasthttp.RequestCtx) string {
			return strconv.Itoa(len(ctx.Response.Header.Header()) + n)
		}
	}
	str := func(s string) func(*fasthttp.RequestCtx) string {
		return func(*fasthttp.RequestCtx) string { return s }
	}
	testCases := []struct {
		caseName string
		setup    func(t *testing.T, ctx *fasthttp.RequestCtx)
		appendFn func(dst []byte, ctx *fasthttp.RequestCtx) []byte
		want     func(ctx *fasthttp.RequestCtx) string
		// contains is set for appendFns that write a whole line.
		contains bool
	}{
		{
			caseName: "ncsa B stream unknown",
			setup:    stream(-1),
			appendFn: appendB,
			want:     str("-"),
		}, {
			caseName: "ncsa B stream known",
			setup:    stream(4),
			appendFn: appendB,
			want:     str("4"),
		}, {
			caseName: "ncsa O stream unknown",
			setup:    stream(-1),
			appendFn: appendO,
			want:     str("-"),
		}, {
			caseName: "ncsa O stream known",
			setup:    stream(4),
			appendFn: appendO,
			want:     headerSize(4),
		}, {
			caseName: "ncsa S stream unknown",
			setup:    stream(-1),
			appendFn: appendS,
			want:     str("-"),
		}, {
			caseName: "ltsv stream unknown",
			setup:    stream(-1),
			appendFn: appendLTSVLog,
			want:     str("\tsize:\t"),
			contains: true,
		}, {
			caseName: "json stream unknown",
			setup:    stream(-1),
			appendFn: appendJSONLog,
			want:     str(`,"size":null,`),
			contains: true,
		}, {
			caseName: "ncsa B body",
			setup:    body,
			appendFn: appendB,
			want:     str("4"),
		}, {
			caseName: "ncsa b body",
			setup:    body,
			appendFn: appendLb,
			want:     str("4"),
		}, {
			caseName: "ltsv body",
			setup:    body,
			appendFn: appendLTSVLog,
			want:     str("\tsize:4\t"),
			contains: true,
		}, {
			caseName: "json body",
			setup:    body,
			appendFn: appendJSONLog,
			want:     str(`,"size":4,`),
			contains: true,
		}, {
			caseName: "ncsa B body after unknown stream",
			setup: func(t *testing.T, ctx *fasthttp.RequestCtx) {
				ctx.Response.SetBodyStream(unreadReader{t}, -1)
				ctx.Response.SetBodyString("body")
			},
			appendFn: appendB,
			want:     str("4"),
		}, {
			caseName: "ncsa B body replacing a proxied body",
			setup: func(t *testing.T, ctx *fasthttp.RequestCtx) {
				ctx.Response.Header.SetContentLength(1000)
				ctx.Response.SetBodyString("body")
			},
			appendFn: appendB,
			want:     str("4"),
		}, {
			caseName: "ncsa B head",
			setup:    head,
			appendFn: appendB,
			want:     str("0"),
		}, {
			caseName: "ncsa b head",
			setup:    head,
			appendFn: appendLb,
			want:     str("-"),
		}, {
			caseName: "ncsa O head",
			setup:    head,
			appendFn: appendO,
			want:     headerSize(0),
		}, {
			caseName: "json head",
			setup:    head,
			appendFn: appendJSONLog,
			want:     str(`,"size":0,`),
			contains: true,
		}, {
			caseName: "ncsa B head with body",
			setup: func(t *testing.T, ctx *fasthttp.RequestCtx) {
				ctx.Request.Header.SetMethod(fasthttp.MethodHead)
				ctx.Response.SetBodyString("body")
			},
			appendFn: appendB,
			want:     str("0"),
		}, {
			caseName: "ncsa B 304 with body",
			setup: func(t *testing.T, ctx *fasthttp.RequestCtx) {
				ctx.Response.SetStatusCode(fasthttp.StatusNotModified)
				ctx.Response.SetBodyString("body")
			},
			appendFn: appendB,
			want:     str("0"),
		}, {
			caseName: "ncsa B 204 stream",
			setup: func(t *testing.T, ctx *fasthttp.RequestCtx) {
				ctx.Response.SetStatusCode(fasthttp.StatusNoContent)
				ctx.Response.SetBodyStream(unreadReader{t}, -1)
			},
			appendFn: appendB,
			want:     str("0"),
		}, {
			caseName: "ncsa B 1xx with body",
			setup: func(t *testing.T, ctx *fasthttp.RequestCtx) {
				ctx.Response.SetStatusCode(fasthttp.StatusContinue)
				ctx.Response.SetBodyString("body")
			},
			appendFn: appendB,
			want:     str("0"),
		}, {
			caseName: "ncsa B status below 100 with body",
			setup: func(t *testing.T, ctx *fasthttp.RequestCtx) {
				ctx.Response.SetStatusCode(99)
				ctx.Response.SetBodyString("body")
			},
			appendFn: appendB,
			want:     str("4"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}
			tc.setup(t, ctx)

			got := string(tc.appendFn(nil, ctx))
			want := tc.want(ctx)
			if tc.contains && !strings.Contains(got, want) {
				t.Errorf("got %q; want it to contain %q", got, want)
			} else if !tc.contains && got != want {
				t.Errorf("got %q; want %q", got, want)
			}
		})
	}
}
