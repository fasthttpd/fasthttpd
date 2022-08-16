package handler

import (
	"testing"

	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/jarxorg/tree"
)

func TestNewBalancerHandler(t *testing.T) {
	tests := []struct {
		cfg    tree.Map
		errstr string
	}{
		{
			cfg: tree.Map{
				"urls": tree.ToArrayValues(
					"http://localhost:9000",
					"http://localhost:9001",
				),
				"healthCheckInterval": tree.ToValue(1),
			},
		}, {
			cfg: tree.Map{
				"url": tree.ToValue("http://localhost:9000"),
			},
		}, {
			cfg:    tree.Map{},
			errstr: `failed to create balancer: require 'url' or 'urls' entry`,
		}, {
			cfg: tree.Map{
				"url":       tree.ToValue("http://localhost:9000"),
				"algorithm": tree.ToValue("ip-hash"),
			},
		}, {
			cfg: tree.Map{
				"url":       tree.ToValue("http://localhost:9000"),
				"algorithm": tree.ToValue("consistent-hash"),
			},
		}, {
			cfg: tree.Map{
				"url":       tree.ToValue("http://localhost:9000"),
				"algorithm": tree.ToValue("p2c"),
			},
		}, {
			cfg: tree.Map{
				"url":       tree.ToValue("http://localhost:9000"),
				"algorithm": tree.ToValue("random"),
			},
		}, {
			cfg: tree.Map{
				"url":       tree.ToValue("http://localhost:9000"),
				"algorithm": tree.ToValue("round-robin"),
			},
		}, {
			cfg: tree.Map{
				"url":       tree.ToValue("http://localhost:9000"),
				"algorithm": tree.ToValue("least-load"),
			},
		}, {
			cfg: tree.Map{
				"url":       tree.ToValue("http://localhost:9000"),
				"algorithm": tree.ToValue("bounded"),
			},
		}, {
			cfg: tree.Map{
				"url":       tree.ToValue("http://localhost:9000"),
				"algorithm": tree.ToValue("UNKNOWN"),
			},
			errstr: `failed to create balancer: algorithm not supported`,
		},
	}
	for i, test := range tests {
		h, err := NewBalancerHandler(test.cfg, logger.NilLogger)
		if test.errstr != "" {
			if err == nil {
				t.Fatalf("tests[%d] unexpected no error", i)
			}
			if err.Error() != test.errstr {
				t.Errorf("tests[%d] unexpected error: %q; want %q", i, err.Error(), test.errstr)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if h == nil {
			t.Fatalf("tests[%d] unexpected nil handler", i)
		}
	}
}
