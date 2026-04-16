package handler

import (
	"testing"

	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/mojatter/tree"
)

// TestNewBalancerHandler checks that the deprecated 'balancer' handler type
// still constructs successfully by delegating to NewProxyHandler. The full
// algorithm / error matrix is covered in proxy_test.go.
func TestNewBalancerHandler(t *testing.T) {
	testCases := []struct {
		caseName string
		cfg      tree.Map
		errstr   string
	}{
		{
			caseName: "multiple urls",
			cfg: tree.Map{
				"urls": tree.ToArrayValues(
					"http://localhost:9000",
					"http://localhost:9001",
				),
			},
		}, {
			caseName: "single url",
			cfg: tree.Map{
				"url": tree.ToValue("http://localhost:9000"),
			},
		}, {
			caseName: "empty cfg returns error",
			cfg:      tree.Map{},
			errstr:   `failed to create proxy: require 'url' or 'urls' entry`,
		}, {
			caseName: "dropped algorithm least-load returns error",
			cfg: tree.Map{
				"url":       tree.ToValue("http://localhost:9000"),
				"algorithm": tree.ToValue("least-load"),
			},
			errstr: `failed to create proxy: algorithm not supported: least-load`,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			h, err := NewBalancerHandler(tc.cfg, logger.NilLogger)
			if tc.errstr != "" {
				if err == nil {
					t.Fatalf("unexpected no error")
				}
				if err.Error() != tc.errstr {
					t.Errorf("unexpected error: %q; want %q", err.Error(), tc.errstr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if h == nil {
				t.Fatalf("unexpected nil handler")
			}
		})
	}
}
