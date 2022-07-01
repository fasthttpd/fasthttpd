package filter

import (
	"strings"
	"testing"

	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
)

func TestNewFilter(t *testing.T) {
	tests := []struct {
		cfg    tree.Map
		errstr string
	}{
		{
			cfg: tree.Map{
				"type": tree.ToValue("basicAuth"),
			},
		}, {
			cfg: tree.Map{
				"type":      tree.ToValue("basicAuth"),
				"usersFile": tree.ToValue("not-found.yaml"),
			},
			errstr: "open not-found.yaml: no such file or directory",
		}, {
			cfg: tree.Map{
				"type":      tree.ToValue("basicAuth"),
				"usersFile": tree.ToValue("filter_test.go"),
			},
			errstr: "yaml:",
		}, {
			cfg: tree.Map{
				"type": tree.ToValue("UNKNOWN"),
			},
			errstr: "unknown filter type: UNKNOWN",
		},
	}
	for i, test := range tests {
		_, err := NewFilter(test.cfg)
		if test.errstr != "" {
			if err == nil {
				t.Fatalf("tests[%d] no error", i)
			}
			if !strings.HasPrefix(err.Error(), test.errstr) {
				t.Errorf("tests[%d] got %q; want %q", i, err.Error(), test.errstr)
			}
			continue
		}
		if err != nil {
			t.Fatalf("tests[%d] error %v", i, err)
		}
	}
}

func TestFilterDelegator(t *testing.T) {
	tests := []struct {
		f        Filter
		wantReq  bool
		wantResp bool
	}{
		{
			f:        &FilterDelegator{},
			wantReq:  true,
			wantResp: true,
		}, {
			f: &FilterDelegator{
				RequestFunc: func(ctx *fasthttp.RequestCtx) bool {
					return false
				},
				ResponseFunc: func(ctx *fasthttp.RequestCtx) bool {
					return false
				},
			},
		},
	}
	for i, test := range tests {
		ctx := &fasthttp.RequestCtx{}
		gotReq := test.f.Request(ctx)
		if gotReq != test.wantReq {
			t.Errorf("tests[%d] got req %v; want %v", i, gotReq, test.wantReq)
		}
		gotResp := test.f.Response(ctx)
		if gotResp != test.wantResp {
			t.Errorf("tests[%d] got req %v; want %v", i, gotResp, test.wantResp)
		}
	}
}
