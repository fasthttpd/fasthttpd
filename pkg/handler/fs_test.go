package handler

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/jarxorg/tree"
	"github.com/valyala/fasthttp"
)

func TestNewFS(t *testing.T) {
	tests := []struct {
		cfg    tree.Map
		want   *fasthttp.FS
		errstr string
	}{
		{
			cfg: tree.Map{
				"root":       tree.ToValue("path/to/public"),
				"indexNames": tree.ToArrayValues("index.html"),
				"compress":   tree.ToValue(true),
				"compressedFileSuffixes": tree.Map{
					"gzip": tree.ToValue(".fasthttp.gz"),
					"br":   tree.ToValue(".fasthttp.br"),
				},
			},
			want: &fasthttp.FS{
				Root:       "path/to/public",
				IndexNames: []string{"index.html"},
				Compress:   true,
				CompressedFileSuffixes: map[string]string{
					"gzip": ".fasthttp.gz",
					"br":   ".fasthttp.br",
				},
			},
		}, {
			cfg:    tree.Map{},
			errstr: "failed to create FS: require 'root' entry",
		}, {
			cfg: tree.Map{
				"pathNotFound": tree.ToValue("test"),
			},
			errstr: "json: cannot unmarshal string into Go struct field",
		},
	}
	for i, test := range tests {
		got, err := NewFS(test.cfg)
		if test.errstr != "" {
			if err == nil {
				t.Fatalf("tests[%d] unexpected no error", i)
			}
			if !strings.Contains(err.Error(), test.errstr) {
				t.Errorf("tests[%d] unexpected error: %q; want %q", i, err.Error(), test.errstr)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		got.PathNotFound = nil
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("tests[%d] got %#v; want %#v", i, got, test.want)
		}
	}
}

func TestFS_Handler(t *testing.T) {
	cfg := tree.Map{
		"root":       tree.ToValue("testdata/public"),
		"indexNames": tree.ToArrayValues("index.html"),
	}
	handler, err := NewFSHandler(cfg, logger.NilLogger)
	if err != nil {
		t.Fatal(err)
	}

	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/")
	handler(ctx)

	expected, err := os.ReadFile("testdata/public/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ctx.Response.Body(), expected) {
		t.Errorf("unexpected body %q; want %q", ctx.Response.Body(), expected)
	}
}
