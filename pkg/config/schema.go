package config

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mojatter/tree"
)

// Schema validates a single tree.Node against a constraint. Concrete
// implementations may assert value types (e.g. StringSchema) or simply
// assert that the node is a container so validateTree may descend
// into its children (ArraySchema, MapSchema).
type Schema interface {
	// Validate returns nil when v satisfies the schema, otherwise an error
	// describing the violation without a path prefix.
	Validate(v tree.Node) error
}

// StringSchema matches a string-typed leaf. When Enum is non-empty the
// value must appear in the slice; when Regex is non-empty the value must
// match the regular expression.
type StringSchema struct {
	Enum  []string
	Regex string
}

var _ Schema = StringSchema{}

// Validate implements Schema.
func (s StringSchema) Validate(v tree.Node) error {
	if !v.Type().IsStringValue() {
		return fmt.Errorf("expected string, got %s", typeName(v.Type()))
	}
	str := v.Value().String()
	if len(s.Enum) > 0 && !slices.Contains(s.Enum, str) {
		return fmt.Errorf("value %q not in allowed set %v", str, s.Enum)
	}
	if s.Regex != "" {
		re, err := regexp.Compile(s.Regex)
		if err != nil {
			return fmt.Errorf("schema: invalid regex %q: %w", s.Regex, err)
		}
		if !re.MatchString(str) {
			return fmt.Errorf("value %q does not match %s", str, s.Regex)
		}
	}
	return nil
}

// BoolSchema matches a boolean-typed leaf.
type BoolSchema struct{}

var _ Schema = BoolSchema{}

// Validate implements Schema.
func (BoolSchema) Validate(v tree.Node) error {
	if !v.Type().IsBoolValue() {
		return fmt.Errorf("expected bool, got %s", typeName(v.Type()))
	}
	return nil
}

// IntSchema matches an integer-typed leaf with optional bounds. Min
// and Max are inclusive; nil means "no bound".
type IntSchema struct {
	Min *int64
	Max *int64
}

var _ Schema = IntSchema{}

// Validate implements Schema.
func (s IntSchema) Validate(v tree.Node) error {
	if !v.Type().IsNumberValue() {
		return fmt.Errorf("expected number, got %s", typeName(v.Type()))
	}
	n := v.Value().Int64()
	if s.Min != nil && n < *s.Min {
		return fmt.Errorf("value %d less than min %d", n, *s.Min)
	}
	if s.Max != nil && n > *s.Max {
		return fmt.Errorf("value %d greater than max %d", n, *s.Max)
	}
	return nil
}

// DurationSchema matches a value expressible as time.Duration: either
// a string parseable by time.ParseDuration ("60s") or a number
// (interpreted as the already-normalised duration representation that
// Config.Normalize produces).
type DurationSchema struct {
	Min time.Duration
}

var _ Schema = DurationSchema{}

// Validate implements Schema.
func (s DurationSchema) Validate(v tree.Node) error {
	var d time.Duration
	switch {
	case v.Type().IsStringValue():
		str := v.Value().String()
		dd, err := time.ParseDuration(str)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", str, err)
		}
		d = dd
	case v.Type().IsNumberValue():
		d = time.Duration(v.Value().Int64())
	default:
		return fmt.Errorf("expected duration (string or number), got %s", typeName(v.Type()))
	}
	if s.Min > 0 && d < s.Min {
		return fmt.Errorf("duration %s less than min %s", d, s.Min)
	}
	return nil
}

// ArraySchema asserts that a node is an array. It is a structural
// schema; after validating, validateTree continues into the element
// children so element-level rules (e.g. ".foo[]") can still run.
type ArraySchema struct{}

var _ Schema = ArraySchema{}

// Validate implements Schema.
func (ArraySchema) Validate(v tree.Node) error {
	if !v.Type().IsArray() {
		return fmt.Errorf("expected array, got %s", typeName(v.Type()))
	}
	return nil
}

// MapSchema asserts that a node is a map. Structural; descent continues.
type MapSchema struct{}

var _ Schema = MapSchema{}

// Validate implements Schema.
func (MapSchema) Validate(v tree.Node) error {
	if !v.Type().IsMap() {
		return fmt.Errorf("expected map, got %s", typeName(v.Type()))
	}
	return nil
}

// AnySchema accepts any node without further checks. It is *terminal*,
// so validateTree does not descend into its children. Useful for
// polymorphic fields (e.g. content handler's `headers` which may be a
// map or an array) where a tighter schema would reject valid forms.
type AnySchema struct{}

var _ Schema = AnySchema{}
var _ terminalSchema = AnySchema{}

// Validate implements Schema.
func (AnySchema) Validate(tree.Node) error { return nil }

func (AnySchema) terminal() {}

// MapOfSchema validates a map whose every value satisfies Value. It is
// a *terminal* schema: after validating, validateTree does not descend
// into children, so strict unknown-key checks are suppressed within
// the map. Useful for fields like fasthttp.FS.CompressedFileSuffixes
// whose keys are user-defined encoding names.
type MapOfSchema struct {
	Value Schema
}

var _ Schema = MapOfSchema{}
var _ terminalSchema = MapOfSchema{}

// Validate implements Schema.
func (s MapOfSchema) Validate(v tree.Node) error {
	if !v.Type().IsMap() {
		return fmt.Errorf("expected map, got %s", typeName(v.Type()))
	}
	for k, vv := range v.Map() {
		if err := s.Value.Validate(vv); err != nil {
			return fmt.Errorf("[%q]: %w", k, err)
		}
	}
	return nil
}

func (MapOfSchema) terminal() {}

// terminalSchema marks schemas that validate a container in full.
// After such a schema matches, validateTree does not descend into the
// node's children (so unknown-key checks inside the container are
// suppressed).
type terminalSchema interface {
	Schema
	terminal()
}

// validateTree walks root and validates each encountered node against
// pathSchemas. Paths are normalised for lookup: map keys contribute
// ".key", array indices contribute "[]". Display paths for error
// messages keep the concrete index (e.g. ".users[0].name").
//
// When strict is true, a leaf whose normalised path is absent from
// pathSchemas is reported as an unknown key. Container nodes (map /
// array) without a schema are still descended so nested leaf schemas
// can match; strict unknown-key reporting fires at the leaf level.
//
// prefix is prepended verbatim to every reported path, letting
// callers attribute errors to a named subtree (e.g. `handlers["static"]`).
func validateTree(root tree.Node, pathSchemas map[string]Schema, strict bool, prefix string) []error {
	var errs []error
	_ = tree.Walk(root, func(n tree.Node, keys []any) error {
		if len(keys) == 0 {
			// The root itself carries no path; caller validates its type.
			return nil
		}
		lookup := renderPathLookup(keys)
		display := prefix + renderPathDisplay(keys)
		s, ok := pathSchemas[lookup]
		if ok {
			if err := s.Validate(n); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", display, err))
			}
			if _, isTerminal := s.(terminalSchema); isTerminal {
				return tree.SkipWalk
			}
			return nil
		}
		// No schema for this path.
		if n.Type().IsMap() || n.Type().IsArray() {
			// Unknown container: descend to surface leaf-level schemas.
			return nil
		}
		if strict {
			errs = append(errs, fmt.Errorf("%s: unknown key", display))
		}
		return nil
	})
	return errs
}

// renderPathLookup builds the path used to find a schema. Array
// indices collapse to "[]" so a single rule can cover every element.
func renderPathLookup(keys []any) string {
	var b strings.Builder
	for _, k := range keys {
		switch kv := k.(type) {
		case string:
			b.WriteByte('.')
			b.WriteString(kv)
		case int:
			b.WriteString("[]")
		}
	}
	return b.String()
}

// renderPathDisplay builds a human-readable path for error messages
// that keeps concrete array indices.
func renderPathDisplay(keys []any) string {
	var b strings.Builder
	for _, k := range keys {
		switch kv := k.(type) {
		case string:
			b.WriteByte('.')
			b.WriteString(kv)
		case int:
			fmt.Fprintf(&b, "[%d]", kv)
		}
	}
	return b.String()
}

// typeName returns a short, lowercase label for use in error messages.
func typeName(t tree.Type) string {
	switch {
	case t.IsStringValue():
		return "string"
	case t.IsBoolValue():
		return "bool"
	case t.IsNumberValue():
		return "number"
	case t.IsArray():
		return "array"
	case t.IsMap():
		return "map"
	case t.IsNilValue():
		return "nil"
	}
	return "unknown"
}

// Int64Ptr is a tiny helper for building IntSchema bounds inline.
func Int64Ptr(v int64) *int64 { return &v }

// schemaMu guards the registries and rebuilds triggered by Register
// calls. Registrations typically happen from handler / filter init()
// functions (serial), but the lock keeps Register safe against a
// Validate call running on another goroutine.
var schemaMu sync.Mutex

// handlerSchemas maps a handler type (e.g. "fs", "proxy") to the path
// → Schema set applied to that handler's config subtree.
var handlerSchemas = map[string]map[string]Schema{}

// filterSchemas is the analogous registry for filters.
var filterSchemas = map[string]map[string]Schema{}

// RegisterHandlerSchema registers the schema set for a handler type.
// Typically called from the handler package's init(). Subsequent
// registrations for the same type replace the previous entry.
func RegisterHandlerSchema(typeName string, schemas map[string]Schema) {
	schemaMu.Lock()
	handlerSchemas[typeName] = schemas
	schemaMu.Unlock()
}

// RegisterFilterSchema registers the schema set for a filter type.
func RegisterFilterSchema(typeName string, schemas map[string]Schema) {
	schemaMu.Lock()
	filterSchemas[typeName] = schemas
	schemaMu.Unlock()
}

// Validate runs schema-driven validation over each document's
// free-form handlers / filters subtrees. Typed portions of Config
// (including the Server struct) are validated by FromTreeMap via the
// yaml decoder with KnownFields(true), so this function intentionally
// does not re-check them.
func Validate(ms []tree.Map) error {
	schemaMu.Lock()
	defer schemaMu.Unlock()

	var errs []error
	for i, m := range ms {
		prefix := ""
		if len(ms) > 1 {
			prefix = fmt.Sprintf("documents[%d].", i)
		}
		if handlers := m.Get("handlers"); handlers.Type().IsMap() {
			hmap := handlers.Map()
			for _, name := range sortedMapKeys(hmap) {
				errs = append(errs, validateHandler(name, hmap[name], prefix)...)
			}
		}
		if filters := m.Get("filters"); filters.Type().IsMap() {
			fmap := filters.Map()
			for _, name := range sortedMapKeys(fmap) {
				errs = append(errs, validateFilter(name, fmap[name], prefix)...)
			}
		}
	}
	return errors.Join(errs...)
}

func validateHandler(name string, hnode tree.Node, rootPrefix string) []error {
	display := fmt.Sprintf("%shandlers[%q]", rootPrefix, name)
	if !hnode.Type().IsMap() {
		return []error{fmt.Errorf("%s: expected map, got %s", display, typeName(hnode.Type()))}
	}
	hcfg := hnode.Map()
	t := hcfg.Get("type").Value().String()
	if t == "" {
		return []error{fmt.Errorf("%s: missing 'type'", display)}
	}
	// When a handler type has no registered schema, skip validation
	// for this subtree. handler.NewHandler still surfaces unknown
	// handler types at runtime, so a missing schema only means "type
	// not yet covered by validation", not "type is wrong".
	schemas, ok := handlerSchemas[t]
	if !ok {
		return nil
	}
	return validateTree(hcfg, schemas, true, display)
}

func validateFilter(name string, fnode tree.Node, rootPrefix string) []error {
	display := fmt.Sprintf("%sfilters[%q]", rootPrefix, name)
	if !fnode.Type().IsMap() {
		return []error{fmt.Errorf("%s: expected map, got %s", display, typeName(fnode.Type()))}
	}
	fcfg := fnode.Map()
	t := fcfg.Get("type").Value().String()
	if t == "" {
		return []error{fmt.Errorf("%s: missing 'type'", display)}
	}
	schemas, ok := filterSchemas[t]
	if !ok {
		return nil
	}
	return validateTree(fcfg, schemas, true, display)
}

// sortedMapKeys returns the keys of m in lexical order so error
// output is stable across runs.
func sortedMapKeys(m tree.Map) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
