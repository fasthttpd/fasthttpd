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

func TestLoadTreeMaps_NotFound(t *testing.T) {
	_, err := LoadTreeMaps("testdata/not-found.yaml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no such file") {
		t.Errorf("error %q does not mention missing file", err.Error())
	}
}

func TestLoadTreeMaps_CircularInclude(t *testing.T) {
	// Glob in include expansion resolves relative to cwd, so match
	// the cwd assumption used by TestUnmarshalYAMLPath_IncludeCircular.
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
