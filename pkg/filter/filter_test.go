package filter

import (
	"strings"
	"testing"

	"github.com/jarxorg/tree"
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
