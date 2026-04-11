package accesslog

import (
	"os"
	"testing"

	"github.com/fasthttpd/fasthttpd/pkg/config"
)

func TestNewAccessLog(t *testing.T) {
	tmp, err := os.CreateTemp("", "*.log")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.Remove(tmp.Name())

	tests := []struct {
		cfg config.Config
	}{
		{
			cfg: config.Config{},
		}, {
			cfg: config.Config{
				AccessLog: config.AccessLog{
					Output: "stdout",
				},
			},
		}, {
			cfg: config.Config{
				AccessLog: config.AccessLog{
					Output: "stderr",
				},
			},
		}, {
			cfg: config.Config{
				AccessLog: config.AccessLog{
					Output: tmp.Name(),
				},
			},
		},
	}
	for i, test := range tests {
		l, err := NewAccessLog(test.cfg)
		if err != nil {
			t.Fatalf("tests[%d] unexpected error: %v", i, err)
		}
		l.Close()
	}
}
