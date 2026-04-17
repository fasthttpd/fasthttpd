package config

import (
	"strings"
	"testing"

	"github.com/mojatter/tree"
)

func TestValidate(t *testing.T) {
	testCases := []struct {
		caseName string
		docs     []tree.Map
		wantErr  string // substring; empty means success
	}{
		{
			caseName: "valid minimal",
			docs: []tree.Map{{
				"host":   tree.V("localhost"),
				"listen": tree.V(":8080"),
				"root":   tree.V("./public"),
			}},
		},
		{
			caseName: "handler missing type",
			docs: []tree.Map{{
				"handlers": tree.Map{"x": tree.Map{"root": tree.V("/")}},
			}},
			wantErr: `handlers["x"]: missing 'type'`,
		},
		{
			caseName: "unregistered handler type passes through",
			docs: []tree.Map{{
				"handlers": tree.Map{"x": tree.Map{
					"type":     tree.V("not-registered"),
					"whatever": tree.V(1),
				}},
			}},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			err := Validate(tc.docs)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate returned %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate returned nil, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// TestFromTreeMap exercises the typed Config decoding path. Unknown
// top-level keys fail KnownFields strict decoding; unknown Server
// fields fail the same way one level deeper; invalid duration values
// fail Duration.UnmarshalYAML.
func TestFromTreeMap(t *testing.T) {
	testCases := []struct {
		caseName string
		doc      tree.Map
		wantErr  string // substring; empty means success
	}{
		{
			caseName: "valid server",
			doc: tree.Map{
				"server": tree.Map{
					"name":        tree.V("fasthttpd"),
					"concurrency": tree.V(100),
					"readTimeout": tree.V("60s"),
				},
			},
		},
		{
			caseName: "unknown top-level key",
			doc: tree.Map{
				"listn": tree.V(":8080"), // typo of "listen"
			},
			wantErr: "field listn not found",
		},
		{
			caseName: "wrong type on known top-level field",
			doc: tree.Map{
				"routes": tree.V("single"), // want array of Route
			},
			wantErr: "cannot unmarshal",
		},
		{
			caseName: "unknown server field",
			doc: tree.Map{
				"server": tree.Map{"futureField": tree.V("x")},
			},
			wantErr: "field futureField not found",
		},
		{
			caseName: "wrong type on known server field",
			doc: tree.Map{
				"server": tree.Map{"concurrency": tree.V("many")},
			},
			wantErr: "cannot unmarshal !!str `many` into int",
		},
		{
			caseName: "invalid server duration",
			doc: tree.Map{
				"server": tree.Map{"readTimeout": tree.V("not-a-duration")},
			},
			wantErr: "invalid duration",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			_, err := FromTreeMap(tc.doc)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("FromTreeMap returned %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("FromTreeMap returned nil, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}
