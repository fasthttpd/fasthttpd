package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestShouldColor(t *testing.T) {
	t.Run("nil file returns false", func(t *testing.T) {
		if shouldColor(nil) {
			t.Error("shouldColor(nil) = true, want false")
		}
	})
	t.Run("regular file returns false", func(t *testing.T) {
		tmp, err := os.CreateTemp(t.TempDir(), "color-*")
		if err != nil {
			t.Fatal(err)
		}
		defer tmp.Close()
		if shouldColor(tmp) {
			t.Error("shouldColor(regular file) = true, want false")
		}
	})
	t.Run("NO_COLOR forces false", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")
		// os.Stdout may or may not be a TTY under `go test`, but
		// NO_COLOR must short-circuit regardless.
		if shouldColor(os.Stdout) {
			t.Error("shouldColor with NO_COLOR=1 returned true")
		}
	})
}

func TestDumpFormatSet(t *testing.T) {
	testCases := []struct {
		caseName string
		input    string
		want     string
		wantErr  string
	}{
		{caseName: "bare -T (IsBoolFlag path)", input: "true", want: "yaml"},
		{caseName: "explicit yaml", input: "yaml", want: "yaml"},
		{caseName: "explicit json", input: "json", want: "json"},
		{caseName: "rejects junk", input: "xml", wantErr: "invalid -T format"},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			var d dumpFormat
			err := d.Set(tc.input)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want substring %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if d.value != tc.want {
				t.Errorf("value = %q, want %q", d.value, tc.want)
			}
		})
	}
}

func TestFastHttpd_TestOrDump(t *testing.T) {
	// loadTreeMaps() os.Chdir's into the configFile's directory, so
	// save and restore cwd to keep subtests isolated.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	testCases := []struct {
		caseName   string
		configFile string
		editExprs  []string
		dumpFormat string
		isTest     bool
		dimStderr  bool
		wantStdout []string
		missStdout []string
		wantStderr []string
		missStderr []string
		wantErr    string
	}{
		{
			caseName:   "-t on minimal fallback succeeds",
			isTest:     true,
			wantStderr: []string{"configuration test is successful"},
			missStdout: []string{"host"},
		},
		{
			caseName:   "-t reports load error",
			isTest:     true,
			configFile: "/nonexistent-dir-for-fasthttpd-test/no.yaml",
			wantErr:    "no such file",
		},
		{
			caseName:   "-T=yaml without -e dumps only normalized to stdout",
			dumpFormat: "yaml",
			wantStdout: []string{"- host: localhost", "listen: :8080"},
			missStderr: []string{"host: localhost"},
		},
		{
			caseName:   "-T=yaml with -e sends pre-edit to stderr, normalized to stdout",
			dumpFormat: "yaml",
			editExprs:  []string{"listen=:9000"},
			// stdout = normalized []Config (array form with leading
			// "- ", Config.SetDefaults-supplied fields like
			// shutdownTimeout, and the -e override).
			// stderr = pre-edit tree.Map (bare top-level keys, no
			// array wrap, no Config defaults, no header line).
			wantStdout: []string{"- host: localhost", "listen: :9000", "shutdownTimeout:"},
			wantStderr: []string{"host: localhost"},
			missStderr: []string{"- host:", "listen: :9000", "shutdownTimeout:", "# pre-edit"},
		},
		{
			caseName:   "-T=json without -e dumps bare array",
			dumpFormat: "json",
			wantStdout: []string{`[`, `"Host": "localhost"`},
			missStdout: []string{`"preEdit"`, `"normalized"`},
		},
		{
			caseName:   "-T=json with -e splits streams",
			dumpFormat: "json",
			editExprs:  []string{"listen=:9000"},
			wantStdout: []string{`"Listen": ":9000"`},
			missStdout: []string{`"preEdit"`, `"normalized"`},
			wantStderr: []string{`"host"`, `"localhost"`},
			missStderr: []string{`":9000"`},
		},
		{
			caseName:   "dimStderr wraps pre-edit section in ANSI dim codes",
			dumpFormat: "yaml",
			editExprs:  []string{"listen=:9000"},
			dimStderr:  true,
			wantStdout: []string{"listen: :9000"},
			// dim codes must only touch stderr (stdout stays clean
			// for pipelines).
			missStdout: []string{"\x1b["},
			wantStderr: []string{"\x1b[2m", "host: localhost", "\x1b[0m"},
		},
		{
			caseName:   "dimStderr=false leaves stderr escape-free",
			dumpFormat: "yaml",
			editExprs:  []string{"listen=:9000"},
			dimStderr:  false,
			wantStderr: []string{"host: localhost"},
			missStderr: []string{"\x1b["},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			if err := os.Chdir(cwd); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			d := &FastHttpd{
				configFile: tc.configFile,
				editExprs:  tc.editExprs,
				isTest:     tc.isTest,
				dumpFormat: dumpFormat{value: tc.dumpFormat},
			}
			err := d.testOrDump(&stdout, &stderr, tc.dimStderr)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want substring %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
			}
			for _, sub := range tc.wantStdout {
				if !strings.Contains(stdout.String(), sub) {
					t.Errorf("stdout missing %q; got:\n%s", sub, stdout.String())
				}
			}
			for _, sub := range tc.missStdout {
				if strings.Contains(stdout.String(), sub) {
					t.Errorf("stdout unexpectedly contains %q; got:\n%s", sub, stdout.String())
				}
			}
			for _, sub := range tc.wantStderr {
				if !strings.Contains(stderr.String(), sub) {
					t.Errorf("stderr missing %q; got:\n%s", sub, stderr.String())
				}
			}
			for _, sub := range tc.missStderr {
				if strings.Contains(stderr.String(), sub) {
					t.Errorf("stderr unexpectedly contains %q; got:\n%s", sub, stderr.String())
				}
			}
		})
	}
}
