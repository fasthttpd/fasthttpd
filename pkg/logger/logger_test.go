package logger

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/fasthttpd/fasthttpd/pkg/config"
)

func TestNewLogger(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "*.logger_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		cfg    config.Log
		errstr string
	}{
		{
			cfg: config.Log{},
		}, {
			cfg: config.Log{Output: "stdout"},
		}, {
			cfg: config.Log{Output: "stderr"},
		}, {
			cfg: config.Log{
				Output: filepath.Join(tmpDir, "test.log"),
				Rotation: config.Rotation{
					MaxSize:    1,
					MaxBackups: 2,
					MaxAge:     3,
					Compress:   true,
					LocalTime:  true,
				},
			},
		}, {
			cfg: config.Log{
				Output: filepath.Join(tmpDir, "test.log"),
				Flags:  []string{"testflag"},
			},
			errstr: "unknown flag: testflag",
		},
	}
	for i, test := range tests {
		func() {
			got, err := NewLogger(test.cfg)
			if err == nil {
				defer got.Close()
			}
			if test.errstr != "" {
				if err == nil {
					t.Fatalf("tests[%d] no error", i)
				}
				if err.Error() != test.errstr {
					t.Errorf("tests[%d] got error %q; want %q", i, err.Error(), test.errstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("tests[%d] unexpected error: %v", i, err)
			}
		}()
	}
}

func TestLogger_Printf(t *testing.T) {
	tests := []struct {
		cfg  config.Log
		want string
	}{
		{
			want: `test [0-9]+$`,
		}, {
			cfg: config.Log{
				Output: "stdout",
				Prefix: "PREFIX ",
				Flags:  []string{"time", "date"},
			},
			want: `^PREFIX [0-9]+/[0-9]+/[0-9]+ [0-9]+:[0-9]+:[0-9]+ test [0-9]+$`,
		}, {
			cfg: config.Log{
				Output: "stdout",
				Prefix: "MSGPREFIX ",
				Flags:  []string{"time", "date", "msgprefix"},
			},
			want: `^[0-9]+/[0-9]+/[0-9]+ [0-9]+:[0-9]+:[0-9]+ MSGPREFIX test [0-9]+$`,
		}, {
			cfg: config.Log{
				Output: "stdout",
				Flags:  []string{"time", "date", "microsecond", "utc"},
			},
			want: `^[0-9]+/[0-9]+/[0-9]+ [0-9]+:[0-9]+:[0-9]+\.[0-9]+ test [0-9]+$`,
		},
	}

	b := new(bytes.Buffer)
	out := &NopRotator{Writer: b}

	for i, test := range tests {
		b.Reset()
		l, err := newLogger(out, test.cfg)
		if err != nil {
			t.Fatal(err)
		}
		l.Printf("test %d", i)
		got := strings.TrimSpace(b.String())
		if !regexp.MustCompile(test.want).MatchString(got) {
			t.Errorf("tests[%d] no match %q; want pattern %q", i, got, test.want)
		}
		l.Close()
	}
}
