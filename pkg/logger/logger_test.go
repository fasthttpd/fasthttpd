package logger

import (
	"bytes"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/fasthttpd/fasthttpd/pkg/config"
)

func Test_NewLogger(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "*.logger_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		cfg    config.Log
		want   func(cfg config.Log) Logger
		errstr string
	}{
		{
			cfg: config.Log{},
			want: func(cfg config.Log) Logger {
				return NilLogger
			},
		}, {
			cfg: config.Log{Output: "stdout"},
			want: func(cfg config.Log) Logger {
				out, _ := SharedRotator("stdout", cfg.Rotation)
				return &logger{
					Logger:  log.New(out, "", 0),
					rotator: out,
				}
			},
		}, {
			cfg: config.Log{Output: "stderr"},
			want: func(cfg config.Log) Logger {
				out, _ := SharedRotator("stderr", cfg.Rotation)
				return &logger{
					Logger:  log.New(out, "", 0),
					rotator: out,
				}
			},
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
			want: func(cfg config.Log) Logger {
				out, _ := SharedRotator(filepath.Join(tmpDir, "test.log"), cfg.Rotation)
				return &logger{
					Logger:  log.New(out, "", 0),
					rotator: out,
				}
			},
		}, {
			cfg: config.Log{
				Output: filepath.Join(tmpDir, "test.log"),
				Flags:  []string{"testflag"},
			},
			errstr: "unknown flag: testflag",
		},
	}
	fn := func(i int) {
		test := tests[i]
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

		want := test.want(test.cfg)
		defer want.Close()

		if !reflect.DeepEqual(got, want) {
			t.Errorf("tests[%d] got %#v; want %#v", i, got, want)
		}
	}
	for i := range tests {
		fn(i)
	}
}

func Test_Logger_Printf(t *testing.T) {
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
