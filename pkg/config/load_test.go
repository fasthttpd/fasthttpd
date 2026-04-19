package config

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/mojatter/tree"
)

func TestLoadTreeMaps(t *testing.T) {
	got, err := LoadTreeMaps("testdata/full.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 documents, got %d", len(got))
	}
	if got[0].Get("host").Value().String() != "localhost" {
		t.Errorf("doc[0].host = %q, want %q", got[0].Get("host").Value().String(), "localhost")
	}
	if got[1].Get("listen").Value().String() != ":8443" {
		t.Errorf("doc[1].listen = %q, want %q", got[1].Get("listen").Value().String(), ":8443")
	}
}

func TestLoadTreeMaps_JSON(t *testing.T) {
	// JSON is a subset of YAML 1.2, so a .json file goes through the
	// same YAML decoder and yields an equivalent tree.Map.
	got, err := LoadTreeMaps("testdata/simple.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 document, got %d", len(got))
	}
	m := got[0]
	if h := m.Get("host").Value().String(); h != "localhost" {
		t.Errorf("host = %q, want %q", h, "localhost")
	}
	if c := m.Get("server").Get("concurrency").Value().Int(); c != 256 {
		t.Errorf("server.concurrency = %d, want 256", c)
	}
	routes := m.Get("routes").Array()
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	if p := routes[0].Get("path").Value().String(); p != "/" {
		t.Errorf("routes[0].path = %q, want %q", p, "/")
	}
}

func TestLoadTreeMaps_NotFound(t *testing.T) {
	_, err := LoadTreeMaps("testdata/not-found.yaml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no such file") {
		t.Errorf("error %q does not mention missing file", err.Error())
	}
}

func TestLoadTreeMaps_Include(t *testing.T) {
	// Glob in include expansion resolves relative to cwd.
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(currentDir) //nolint:errcheck

	if err := os.Chdir("testdata"); err != nil {
		t.Fatal(err)
	}

	got, err := LoadTreeMaps("include.yaml")
	if err != nil {
		t.Fatal(err)
	}
	wantHosts := []string{"include1.local", "include2.local"}
	gotHosts := make([]string, len(got))
	for i, m := range got {
		gotHosts[i] = m.Get("host").Value().String()
	}
	if !reflect.DeepEqual(gotHosts, wantHosts) {
		t.Errorf("hosts = %v, want %v", gotHosts, wantHosts)
	}
}

func TestLoadTreeMaps_CircularInclude(t *testing.T) {
	// Glob in include expansion resolves relative to cwd.
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(currentDir) //nolint:errcheck

	if err := os.Chdir("testdata"); err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		caseName string
		path     string
	}{
		{
			caseName: "self include with identical spelling",
			path:     "include-circular.yaml",
		},
		{
			caseName: "self include across ./ prefix mismatch",
			path:     "include-circular-dotslash.yaml",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			_, err := LoadTreeMaps(tc.path)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "circular dependency") {
				t.Errorf("error %q does not mention circular dependency", err.Error())
			}
		})
	}
}

func TestEdit(t *testing.T) {
	base := func() []tree.Map {
		return []tree.Map{{
			"host": tree.V("localhost"),
			"root": tree.V("./public"),
		}}
	}
	testCases := []struct {
		caseName string
		exprs    []string
		check    func(t *testing.T, ms []tree.Map)
		wantErr  string
	}{
		{
			caseName: "empty exprs returns input unchanged",
			exprs:    nil,
			check: func(t *testing.T, ms []tree.Map) {
				if got := ms[0].Get("root").Value().String(); got != "./public" {
					t.Errorf("root mutated unexpectedly: %q", got)
				}
			},
		},
		{
			caseName: "override with auto-quoted string",
			exprs:    []string{"root=."},
			check: func(t *testing.T, ms []tree.Map) {
				if got := ms[0].Get("root").Value().String(); got != "." {
					t.Errorf("root = %q, want %q", got, ".")
				}
			},
		},
		{
			caseName: "override with integer keeps numeric",
			exprs:    []string{"count=5"},
			check: func(t *testing.T, ms []tree.Map) {
				if got := ms[0].Get("count").Value().Int(); got != 5 {
					t.Errorf("count = %d, want 5", got)
				}
			},
		},
		{
			caseName: "override with explicit bool",
			exprs:    []string{"debug=true"},
			check: func(t *testing.T, ms []tree.Map) {
				if got := ms[0].Get("debug").Value().Bool(); !got {
					t.Errorf("debug = false, want true")
				}
			},
		},
		{
			caseName: "explicit .[]. prefix is respected",
			exprs:    []string{".[].host=\"example.com\""},
			check: func(t *testing.T, ms []tree.Map) {
				if got := ms[0].Get("host").Value().String(); got != "example.com" {
					t.Errorf("host = %q, want %q", got, "example.com")
				}
			},
		},
		{
			caseName: "array literal is handed through as YAML flow",
			exprs:    []string{"indexNames=[index.php, fallback.html]"},
			check: func(t *testing.T, ms []tree.Map) {
				arr := ms[0].Get("indexNames").Array()
				if len(arr) != 2 {
					t.Fatalf("indexNames length = %d, want 2", len(arr))
				}
				if got := arr[0].Value().String(); got != "index.php" {
					t.Errorf("indexNames[0] = %q, want %q", got, "index.php")
				}
				if got := arr[1].Value().String(); got != "fallback.html" {
					t.Errorf("indexNames[1] = %q, want %q", got, "fallback.html")
				}
			},
		},
		{
			caseName: "map literal is handed through as YAML flow",
			exprs:    []string{"headers={Content-Type: text/plain}"},
			check: func(t *testing.T, ms []tree.Map) {
				got := ms[0].Get("headers").Get("Content-Type").Value().String()
				if got != "text/plain" {
					t.Errorf("headers.Content-Type = %q, want %q", got, "text/plain")
				}
			},
		},
		{
			caseName: "invalid expression returns error",
			exprs:    []string{"not a valid expr"},
			wantErr:  "syntax error",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			got, err := Edit(base(), tc.exprs)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}

func TestFromTreeMaps(t *testing.T) {
	ms := []tree.Map{
		{
			"host":   tree.V("a.local"),
			"listen": tree.V(":8080"),
		},
		{
			"host":   tree.V("b.local"),
			"listen": tree.V(":8443"),
		},
	}
	got, err := FromTreeMaps(ms)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(got))
	}
	wantHosts := []string{"a.local", "b.local"}
	gotHosts := []string{got[0].Host, got[1].Host}
	if !reflect.DeepEqual(gotHosts, wantHosts) {
		t.Errorf("hosts = %v, want %v", gotHosts, wantHosts)
	}
}
