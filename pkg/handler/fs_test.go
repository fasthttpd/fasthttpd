package handler

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/mojatter/tree"
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
			cfg: tree.Map{
				"root":                     tree.ToValue("path/to/public"),
				"allowSymlinksOutsideRoot": tree.ToValue(true),
			},
			want: &fasthttp.FS{
				Root:                     "path/to/public",
				AllowSymlinksOutsideRoot: true,
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

func TestFS_HandlerSymlinkOutsideRoot(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "public")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "outside.txt"), []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "inside.txt"), []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "outside.txt"), filepath.Join(root, "rel.txt")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "inside.txt"), filepath.Join(root, "abs.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "outside.txt"), filepath.Join(root, "absout.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "idx.html"), []byte("idx"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "..", "idx.html"), filepath.Join(root, "sub", "index.html")); err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		caseName   string
		uri        string
		allow      bool
		wantStatus int
		wantBody   string
	}{
		{caseName: "relative link outside root rejected by default", uri: "/rel.txt", wantStatus: fasthttp.StatusNotFound},
		{caseName: "relative link outside root followed when allowed", uri: "/rel.txt", allow: true, wantStatus: fasthttp.StatusOK, wantBody: "outside"},
		{caseName: "absolute link inside root rejected by default", uri: "/abs.txt", wantStatus: fasthttp.StatusNotFound},
		{caseName: "absolute link inside root followed when allowed", uri: "/abs.txt", allow: true, wantStatus: fasthttp.StatusOK, wantBody: "inside"},
		{caseName: "absolute link outside root rejected by default", uri: "/absout.txt", wantStatus: fasthttp.StatusNotFound},
		{caseName: "absolute link outside root followed when allowed", uri: "/absout.txt", allow: true, wantStatus: fasthttp.StatusOK, wantBody: "outside"},
		{caseName: "index file linking outside root forbidden by default", uri: "/sub/", wantStatus: fasthttp.StatusForbidden},
		{caseName: "index file linking outside root served when allowed", uri: "/sub/", allow: true, wantStatus: fasthttp.StatusOK, wantBody: "idx"},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			handler, err := NewFSHandler(tree.Map{
				"root":                     tree.ToValue(root),
				"indexNames":               tree.ToArrayValues("index.html"),
				"allowSymlinksOutsideRoot": tree.ToValue(tc.allow),
			}, logger.NilLogger)
			if err != nil {
				t.Fatal(err)
			}

			ctx := &fasthttp.RequestCtx{}
			ctx.Init(&fasthttp.Request{}, nil, nil)
			ctx.Request.SetRequestURI(tc.uri)
			handler(ctx)
			if got := ctx.Response.StatusCode(); got != tc.wantStatus {
				t.Errorf("status = %d; want %d", got, tc.wantStatus)
			}
			if tc.wantBody != "" && string(ctx.Response.Body()) != tc.wantBody {
				t.Errorf("body = %q; want %q", ctx.Response.Body(), tc.wantBody)
			}
		})
	}
}

func TestFS_SchemaRegistered(t *testing.T) {
	testCases := []struct {
		caseName string
		handler  tree.Map
		wantErr  string // substring; empty means success
	}{
		{
			caseName: "valid fs handler",
			handler: tree.Map{
				"type":       tree.V("fs"),
				"root":       tree.V("./public"),
				"indexNames": tree.A("index.html"),
				"compress":   tree.V(true),
			},
		},
		{
			caseName: "allowSymlinksOutsideRoot is accepted",
			handler: tree.Map{
				"type":                     tree.V("fs"),
				"allowSymlinksOutsideRoot": tree.V(true),
			},
		},
		{
			caseName: "unknown fs field is rejected",
			handler: tree.Map{
				"type":  tree.V("fs"),
				"root":  tree.V("./public"),
				"bogus": tree.V(123),
			},
			wantErr: `.handlers["static"]: unknown key "bogus"`,
		},
		{
			caseName: "wrong type on fs field",
			handler: tree.Map{
				"type":     tree.V("fs"),
				"compress": tree.V("yes"), // want bool
			},
			wantErr: "compress",
		},
		{
			caseName: "compressedFileSuffixes accepts user keys",
			handler: tree.Map{
				"type": tree.V("fs"),
				"compressedFileSuffixes": tree.Map{
					"gzip": tree.V(".fasthttp.gz"),
					"br":   tree.V(".fasthttp.br"),
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			docs := []tree.Map{{
				"handlers": tree.Map{"static": tc.handler},
			}}
			err := config.ValidateTreeMaps(docs)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateTreeMaps returned %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateTreeMaps returned nil, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}
