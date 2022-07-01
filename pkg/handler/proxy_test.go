package handler

import (
	"testing"

	"github.com/fasthttpd/fasthttpd/pkg/logger"
	"github.com/jarxorg/tree"
)

func TestNewProxyHandler(t *testing.T) {
	tests := []struct {
		cfg    tree.Map
		errstr string
	}{
		{
			cfg: tree.Map{"url": tree.ToValue("http://localhost:9000/")},
		}, {
			cfg:    tree.Map{},
			errstr: `failed to create proxy: require 'url' entry`,
		}, {
			cfg:    tree.Map{"url": tree.ToValue(":invalid url")},
			errstr: `parse ":invalid url": missing protocol scheme`,
		},
	}
	for i, test := range tests {
		h, err := NewProxyHandler(test.cfg, logger.NilLogger)
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
