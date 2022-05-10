package logger

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/fasthttpd/fasthttpd/pkg/config"
)

func Test_Logger_Printf(t *testing.T) {
	tests := []struct {
		cfg  config.Log
		want string
	}{
		{
			want: `test [0-9]+$`,
		}, {
			cfg: config.Log{
				Prefix: "PREFIX ",
				Flags:  []string{"time", "date"},
			},
			want: `^PREFIX [0-9]+/[0-9]+/[0-9]+ [0-9]+:[0-9]+:[0-9]+ test [0-9]+$`,
		}, {
			cfg: config.Log{
				Prefix: "MSGPREFIX ",
				Flags:  []string{"time", "date", "msgprefix"},
			},
			want: `^[0-9]+/[0-9]+/[0-9]+ [0-9]+:[0-9]+:[0-9]+ MSGPREFIX test [0-9]+$`,
		}, {
			cfg: config.Log{
				Flags: []string{"time", "date", "microsecond", "utc"},
			},
			want: `^[0-9]+/[0-9]+/[0-9]+ [0-9]+:[0-9]+:[0-9]+\.[0-9]+ test [0-9]+$`,
		},
	}
	b := new(bytes.Buffer)
	for i, test := range tests {
		b.Reset()
		l, err := NewLoggerWriter(test.cfg, b)
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
