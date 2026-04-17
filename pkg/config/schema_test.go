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
