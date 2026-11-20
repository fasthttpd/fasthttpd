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

func TestAccessLog_StreamedResponseSize(t *testing.T) {
	testCases := []struct {
		caseName string
		size     int
		appendFn func(dst []byte, ctx *fasthttp.RequestCtx) []byte
		want     func(ctx *fasthttp.RequestCtx) string
	}{
		{
			caseName: "ncsa B unknown",
			size:     -1,
			appendFn: appendB,
			want:     func(*fasthttp.RequestCtx) string { return "-" },
		}, {
			caseName: "ncsa B known",
			size:     4,
			appendFn: appendB,
			want:     func(*fasthttp.RequestCtx) string { return "4" },
		}, {
			caseName: "ncsa O unknown",
			size:     -1,
			appendFn: appendO,
			want:     func(*fasthttp.RequestCtx) string { return "-" },
		}, {
			caseName: "ncsa O known",
			size:     4,
			appendFn: appendO,
			want: func(ctx *fasthttp.RequestCtx) string {
				return strconv.Itoa(len(ctx.Response.Header.Header()) + 4)
			},
		}, {
			caseName: "ncsa S unknown",
			size:     -1,
			appendFn: appendS,
			want:     func(*fasthttp.RequestCtx) string { return "-" },
		}, {
			caseName: "ltsv unknown",
			size:     -1,
			appendFn: appendLTSVLog,
			want:     func(*fasthttp.RequestCtx) string { return "\tsize:\t" },
		}, {
			caseName: "json unknown",
			size:     -1,
			appendFn: appendJSONLog,
			want:     func(*fasthttp.RequestCtx) string { return `,"size":null,` },
		},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			ctx := &fasthttp.RequestCtx{}
			ctx.Response.SetBodyStream(unreadReader{t}, tc.size)

			got := string(tc.appendFn(nil, ctx))
			if want := tc.want(ctx); !strings.Contains(got, want) {
				t.Errorf("got %q; want it to contain %q", got, want)
			}
		})
	}
}
