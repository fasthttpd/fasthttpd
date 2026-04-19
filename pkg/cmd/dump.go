package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/fasthttpd/fasthttpd/pkg/config"
	"github.com/mojatter/tree"
	"go.yaml.in/yaml/v3"
)

// dumpFormat backs the -T flag. Implementing IsBoolFlag lets users
// write bare "-T" (treated as -T=yaml). "-T=yaml" / "-T=json" pass the
// requested format; any other value is rejected at flag-parse time.
type dumpFormat struct {
	value string
}

func (d *dumpFormat) String() string { return d.value }

func (d *dumpFormat) Set(v string) error {
	switch v {
	case "true", "yaml":
		d.value = "yaml"
	case "json":
		d.value = "json"
	default:
		return fmt.Errorf("invalid -T format %q (must be yaml or json)", v)
	}
	return nil
}

// IsBoolFlag makes bare "-T" legal (flag package then calls Set("true")).
func (d *dumpFormat) IsBoolFlag() bool { return true }

// ANSI SGR sequences used to dim the stderr pre-edit section on a TTY.
const (
	ansiDim   = "\x1b[2m"
	ansiReset = "\x1b[0m"
)

// shouldColor reports whether ANSI color codes should be emitted to
// f. It returns false when NO_COLOR is set (per https://no-color.org/)
// or when f is not a character device (file / pipe / bytes.Buffer),
// so redirected streams stay free of escape codes.
func shouldColor(f *os.File) bool {
	if f == nil {
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// testOrDump runs the config pipeline (Load → Edit → Validate →
// FromTreeMaps) without starting any server, then either reports
// success (-t) or writes the final config to stdout (-T). When -e
// was applied, the pre-Edit tree.Map is included in the dump as a
// second section so users can diff "as written" vs "as served".
// dimStderr wraps the stderr pre-edit section in ANSI dim codes so a
// TTY reader's eye naturally falls on the primary (stdout) output.
func (d *FastHttpd) testOrDump(stdout, stderr io.Writer, dimStderr bool) error {
	ms, err := d.loadTreeMaps()
	if err != nil {
		return err
	}

	var preEdit []tree.Map
	if d.dumpFormat.value != "" && len(d.editExprs) > 0 {
		preEdit = cloneTreeMaps(ms)
	}

	ms, err = config.Edit(ms, d.editExprs)
	if err != nil {
		return err
	}
	if err := config.ValidateTreeMaps(ms); err != nil {
		return err
	}
	cfgs, err := config.FromTreeMaps(ms)
	if err != nil {
		return err
	}

	if d.dumpFormat.value == "" {
		_, err := fmt.Fprintln(stderr, "configuration test is successful")
		return err
	}
	return writeDump(stdout, stderr, preEdit, cfgs, d.dumpFormat.value, dimStderr)
}

// cloneTreeMaps deep-copies ms so a subsequent in-place Edit leaves
// the snapshot untouched.
func cloneTreeMaps(ms []tree.Map) []tree.Map {
	out := make([]tree.Map, len(ms))
	for i, m := range ms {
		if c, ok := tree.CloneDeep(m).(tree.Map); ok {
			out[i] = c
		}
	}
	return out
}

// writeDump emits the normalized []Config to stdout and (when -e was
// applied) the pre-Edit tree.Map to stderr, so stdout stays a single
// parseable document in the requested format. A trailing blank line
// is appended to the stderr block in both formats so subsequent
// terminal output (next prompt or interleaved stdout tail) stays
// visually separated. If dimStderr is set, the whole stderr block
// (content + trailing newline) is wrapped in ANSI dim codes.
func writeDump(stdout, stderr io.Writer, preEdit []tree.Map, cfgs []config.Config, format string, dimStderr bool) error {
	if len(preEdit) > 0 {
		if dimStderr {
			if _, err := fmt.Fprint(stderr, ansiDim); err != nil {
				return err
			}
		}
		if err := writeTreeMaps(stderr, preEdit, format); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(stderr); err != nil {
			return err
		}
		if dimStderr {
			if _, err := fmt.Fprint(stderr, ansiReset); err != nil {
				return err
			}
		}
	}
	return writeConfigs(stdout, cfgs, format)
}

// writeTreeMaps serializes ms in the requested format. YAML uses
// multi-document streams with `---` separators; JSON emits a single
// array.
func writeTreeMaps(w io.Writer, ms []tree.Map, format string) error {
	switch format {
	case "yaml":
		for i, m := range ms {
			if i > 0 {
				if _, err := fmt.Fprintln(w, "---"); err != nil {
					return err
				}
			}
			b, err := tree.MarshalYAML(m)
			if err != nil {
				return err
			}
			if _, err := w.Write(b); err != nil {
				return err
			}
		}
		return nil
	case "json":
		arr := make(tree.Array, len(ms))
		for i, m := range ms {
			arr[i] = m
		}
		b, err := tree.MarshalJSON(arr)
		if err != nil {
			return err
		}
		if _, err := w.Write(b); err != nil {
			return err
		}
		_, err = fmt.Fprintln(w)
		return err
	default:
		return fmt.Errorf("unknown dump format %q", format)
	}
}

// writeConfigs serializes cfgs in the requested format. YAML emits
// each config as its own `---`-separated document; JSON emits a
// single array.
func writeConfigs(w io.Writer, cfgs []config.Config, format string) error {
	switch format {
	case "yaml":
		b, err := yaml.Marshal(cfgs)
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(cfgs)
	default:
		return fmt.Errorf("unknown dump format %q", format)
	}
}
