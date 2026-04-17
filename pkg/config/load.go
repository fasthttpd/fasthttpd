package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/mojatter/tree"
	"go.yaml.in/yaml/v3"
)

// LoadTreeMaps reads path as YAML (multi-document) or JSON (single
// object) and returns each document as a tree.Map. `include` directives
// in the loaded documents are expanded recursively; circular includes
// are detected and reported.
func LoadTreeMaps(path string) ([]tree.Map, error) {
	return loadTreeMapsPath(path, nil)
}

func loadTreeMapsPath(path string, includes []string) ([]tree.Map, error) {
	if slices.Contains(includes, path) {
		return nil, fmt.Errorf("circular dependency %v", includes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(strings.ToLower(path), ".json") {
		n, err := tree.UnmarshalJSON(data)
		if err != nil {
			return nil, err
		}
		m, ok := n.(tree.Map)
		if !ok {
			return nil, fmt.Errorf("%s: JSON root must be an object", path)
		}
		return expandIncludes([]tree.Map{m}, append(includes, path))
	}
	return unmarshalTreeMaps(data, append(includes, path))
}

func unmarshalTreeMaps(data []byte, includes []string) ([]tree.Map, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var ms []tree.Map
	for {
		var m tree.Map
		if err := dec.Decode(&m); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		ms = append(ms, m)
	}
	return expandIncludes(ms, includes)
}

func expandIncludes(ms []tree.Map, includes []string) ([]tree.Map, error) {
	var out []tree.Map
	for _, m := range ms {
		inc := m.Get("include").Value().String()
		if inc == "" {
			out = append(out, m)
			continue
		}
		paths, err := filepath.Glob(inc)
		if err != nil {
			return nil, err
		}
		for _, p := range paths {
			sub, err := loadTreeMapsPath(p, includes)
			if err != nil {
				return nil, err
			}
			out = append(out, sub...)
		}
	}
	return out, nil
}

// Edit applies tq-style edit expressions to ms as a batch. Expressions
// without a leading "." are automatically prefixed with ".[]." so they
// target every document; unquoted non-literal RHS values are quoted so
// bare strings (e.g. "root=./public") parse.
func Edit(ms []tree.Map, exprs []string) ([]tree.Map, error) {
	if len(exprs) == 0 {
		return ms, nil
	}
	arr := make(tree.Array, len(ms))
	for i, m := range ms {
		arr[i] = m
	}
	var n tree.Node = arr
	for _, expr := range exprs {
		lr := strings.SplitN(expr, "=", 2)
		if len(lr) == 2 {
			if !strings.HasPrefix(lr[0], ".") {
				lr[0] = ".[]." + lr[0]
			}
			if !strings.HasPrefix(lr[1], `"`) &&
				lr[1] != "true" && lr[1] != "false" && lr[1] != "null" {
				if _, err := strconv.Atoi(lr[1]); err != nil {
					lr[1] = strconv.Quote(lr[1])
				}
			}
			expr = lr[0] + "=" + lr[1]
		}
		if err := tree.Edit(&n, expr); err != nil {
			return nil, err
		}
	}
	outArr, ok := n.(tree.Array)
	if !ok {
		return nil, fmt.Errorf("unexpected edit result type %T", n)
	}
	out := make([]tree.Map, len(outArr))
	for i, elem := range outArr {
		m, ok := elem.(tree.Map)
		if !ok {
			return nil, fmt.Errorf("document[%d] is not a map after edit (got %T)", i, elem)
		}
		out[i] = m
	}
	return out, nil
}

// FromTreeMap converts a tree.Map document into a Config. The map is
// routed through a YAML decoder with KnownFields(true) so typos in
// typed portions of Config (Host / Listen / SSL / Log / AccessLog /
// Routes / RoutesCache / Server) fail loudly rather than being
// silently dropped. After decode, SetDefaults fills unset fields and
// Normalize applies the remaining fixups.
func FromTreeMap(m tree.Map) (Config, error) {
	cfg := Config{}.SetDefaults()
	data, err := tree.MarshalYAML(m)
	if err != nil {
		return cfg, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return cfg, err
	}
	return cfg.Normalize()
}

// FromTreeMaps applies FromTreeMap to each element of ms.
func FromTreeMaps(ms []tree.Map) ([]Config, error) {
	cfgs := make([]Config, len(ms))
	for i, m := range ms {
		cfg, err := FromTreeMap(m)
		if err != nil {
			return nil, err
		}
		cfgs[i] = cfg
	}
	return cfgs, nil
}
