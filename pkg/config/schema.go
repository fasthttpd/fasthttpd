package config

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/mojatter/tree"
	"github.com/mojatter/tree/schema"
)

// schemaMu guards the handler / filter registries. Registrations
// typically happen from init() functions (serial) but the lock keeps
// Register safe against a concurrent Validate.
var schemaMu sync.Mutex

// handlerSchemas maps a handler type (e.g. "fs", "proxy") to the
// QueryRules applied to that handler's config subtree.
var handlerSchemas = map[string]schema.QueryRules{}

// filterSchemas is the analogous registry for filters.
var filterSchemas = map[string]schema.QueryRules{}

// RegisterHandlerSchema registers the QueryRules for a handler type.
// Typically called from the handler package's init(). Subsequent
// registrations for the same type replace the previous entry.
func RegisterHandlerSchema(typeName string, rules schema.QueryRules) {
	schemaMu.Lock()
	defer schemaMu.Unlock()

	handlerSchemas[typeName] = rules
}

// RegisterFilterSchema registers the QueryRules for a filter type.
func RegisterFilterSchema(typeName string, rules schema.QueryRules) {
	schemaMu.Lock()
	defer schemaMu.Unlock()

	filterSchemas[typeName] = rules
}

// SchemaRuler is implemented by types whose schema rule cannot be
// derived from their Go type alone — typically types with a custom
// YAML unmarshaler that accepts multiple input shapes (e.g. [Duration]
// accepts both "60s" and an integer nanoseconds value).
//
// [SchemaFromStruct] honors this method when building the schema for
// a struct field of type T (or *T) where T implements SchemaRuler.
type SchemaRuler interface {
	SchemaRule() schema.Rule
}

// SchemaFromStruct walks v's type via reflect and returns a
// [schema.Map] whose KeyedRules mirror the struct's yaml-tagged
// fields. Nested structs recurse; types implementing [SchemaRuler]
// override the default kind-based mapping.
//
// The key for each field follows the yaml/v3 decoder convention:
// the `yaml:"..."` tag's name, or — when the tag is absent — the
// lowercased Go field name. Fields tagged `yaml:"-"` and unexported
// fields are skipped.
//
// Intended for the typed [Config] struct so that schema-driven
// validation can report unknown keys and type mismatches in the same
// jq-path format used for handlers / filters.
func SchemaFromStruct(v any) schema.Map {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return schemaMapFromType(t)
}

var (
	schemaRulerIface = reflect.TypeFor[SchemaRuler]()
	treeMapType      = reflect.TypeFor[tree.Map]()
)

// ruleForType maps a Go type to the [schema.Rule] that validates a
// node of that type. Custom types implementing [SchemaRuler] short-
// circuit the kind-based default. [tree.Map] is special-cased as an
// "open" map (no allow-list) because handler / filter contents are
// validated by their own registered schemas.
func ruleForType(t reflect.Type) schema.Rule {
	if t.Implements(schemaRulerIface) {
		return reflect.Zero(t).Interface().(SchemaRuler).SchemaRule()
	}
	if reflect.PointerTo(t).Implements(schemaRulerIface) {
		return reflect.New(t).Interface().(SchemaRuler).SchemaRule()
	}
	if t == treeMapType {
		return schema.Map{}
	}
	switch t.Kind() {
	case reflect.String:
		return schema.String{}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return schema.Int{}
	case reflect.Float32, reflect.Float64:
		return schema.Float{}
	case reflect.Bool:
		return schema.Bool{}
	case reflect.Slice, reflect.Array:
		return schema.Every{Rules: schema.QueryRules{".": ruleForType(t.Elem())}}
	case reflect.Map:
		return schema.Every{Rules: schema.QueryRules{".": ruleForType(t.Elem())}}
	case reflect.Struct:
		return schemaMapFromType(t)
	case reflect.Pointer:
		return ruleForType(t.Elem())
	default:
		return schema.Map{}
	}
}

// schemaMapFromType builds a [schema.Map] with KeyedRules from a
// struct type. Each exported field is mapped using the same key the
// yaml/v3 decoder would use: the `yaml:"..."` tag's name, or — when
// the tag is absent — the lowercased Go field name.
func schemaMapFromType(t reflect.Type) schema.Map {
	rules := map[string]schema.Rule{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("yaml")
		if tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name == "" {
			name = strings.ToLower(f.Name)
		}
		rules[name] = ruleForType(f.Type)
	}
	return schema.Map{KeyedRules: rules}
}

// DurationRule validates a value expressible as time.Duration: either
// a string parseable by time.ParseDuration ("60s") or an integer
// (interpreted as nanoseconds, matching Config.Normalize's output).
// The name is deliberately not "Duration" to avoid colliding with the
// YAML-unmarshal type [Duration] defined in config.go.
type DurationRule struct {
	Min time.Duration
}

// Validate implements schema.Rule.
func (r DurationRule) Validate(n tree.Node, q string) error {
	if n.IsNil() {
		return nil
	}
	var d time.Duration
	switch {
	case n.Type().IsStringValue():
		s := n.Value().String()
		dd, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("%s: invalid duration %q: %w", q, s, err)
		}
		d = dd
	case n.Type().IsNumberValue():
		d = time.Duration(n.Value().Int64())
	default:
		return fmt.Errorf("%s: expected duration (string or number), got %s", q, n.Type().String())
	}
	if r.Min > 0 && d < r.Min {
		return fmt.Errorf("%s: duration %s less than min %s", q, d, r.Min)
	}
	return nil
}

// SchemaRule lets [SchemaFromStruct] map a [Duration]-typed field to
// [DurationRule] instead of the default schema.Int{}.
func (Duration) SchemaRule() schema.Rule { return DurationRule{} }

// SizeRule validates a value expressible as a [Size]: either a string
// parseable by parseSize ("4k", "8 KiB") or a plain integer literal
// (interpreted as a byte count). Mirrors [DurationRule] in shape so
// schema errors for Size and Duration follow the same template.
type SizeRule struct{}

// Validate implements schema.Rule.
func (SizeRule) Validate(n tree.Node, q string) error {
	if n.IsNil() {
		return nil
	}
	switch {
	case n.Type().IsStringValue():
		if _, err := parseSize(n.Value().String()); err != nil {
			return fmt.Errorf("%s: %w", q, err)
		}
	case n.Type().IsNumberValue():
		// Any integer is a valid byte count.
	default:
		return fmt.Errorf("%s: expected size (string or number), got %s", q, n.Type().String())
	}
	return nil
}

// SchemaRule lets [SchemaFromStruct] map a [Size]-typed field to
// [SizeRule] instead of the default schema.Int{}.
func (Size) SchemaRule() schema.Rule { return SizeRule{} }

// HandlerDispatch validates a single handler entry: it reads the
// ".type" field and applies the QueryRules registered under that
// type. An unregistered type is passed through (handler.NewHandler
// surfaces the "unknown type" error at runtime), matching the pre-
// schema behaviour.
type HandlerDispatch struct{}

// Validate implements schema.Rule.
func (HandlerDispatch) Validate(n tree.Node, q string) error {
	if n.IsNil() {
		return nil
	}
	if !n.Type().IsMap() {
		return fmt.Errorf("%s: expected map, got %s", q, n.Type().String())
	}
	t := n.Map().Get("type").Value().String()
	if t == "" {
		return fmt.Errorf(`%s: "type" is required`, q)
	}
	rules, ok := lookupHandlerRules(t)
	if !ok {
		return nil
	}
	return schema.ValidateWithPrefix(n, rules, q)
}

func lookupHandlerRules(t string) (schema.QueryRules, bool) {
	schemaMu.Lock()
	defer schemaMu.Unlock()

	rules, ok := handlerSchemas[t]
	return rules, ok
}

// FilterDispatch is the analogous rule for a single filter entry.
type FilterDispatch struct{}

// Validate implements schema.Rule.
func (FilterDispatch) Validate(n tree.Node, q string) error {
	if n.IsNil() {
		return nil
	}
	if !n.Type().IsMap() {
		return fmt.Errorf("%s: expected map, got %s", q, n.Type().String())
	}
	t := n.Map().Get("type").Value().String()
	if t == "" {
		return fmt.Errorf(`%s: "type" is required`, q)
	}
	rules, ok := lookupFilterRules(t)
	if !ok {
		return nil
	}
	return schema.ValidateWithPrefix(n, rules, q)
}

func lookupFilterRules(t string) (schema.QueryRules, bool) {
	schemaMu.Lock()
	defer schemaMu.Unlock()

	rules, ok := filterSchemas[t]
	return rules, ok
}

// topLevelRules describes the document-root shape for schema-driven
// validation. The base shape is generated from [Config] via reflect
// (see [SchemaFromStruct]), then the `handlers` / `filters` entries
// are overridden so per-type registered rules are dispatched against
// each entry.
var topLevelRules = func() schema.QueryRules {
	root := SchemaFromStruct(Config{})
	root.KeyedRules["handlers"] = schema.Every{Rules: schema.QueryRules{
		".": HandlerDispatch{},
	}}
	root.KeyedRules["filters"] = schema.Every{Rules: schema.QueryRules{
		".": FilterDispatch{},
	}}
	return schema.QueryRules{".": root}
}()

// ValidateTreeMaps runs schema-driven validation over each document.
// The schema covers the entire typed [Config] shape (generated from
// the struct via reflect) plus dispatched rules for the free-form
// handlers / filters subtrees. After ValidateTreeMaps succeeds,
// FromTreeMap can decode the document into Config without
// KnownFields-style strict checking.
//
// When a registered schema rule itself is malformed (its query string
// fails to parse via tree.Find), the underlying *schema.ErrQuery is
// wrapped with an "internal" message so the operator can tell it
// apart from a config error. The original *schema.ErrQuery remains
// reachable via errors.As.
func ValidateTreeMaps(ms []tree.Map) error {
	var errs []error
	for i, m := range ms {
		prefix := ""
		if len(ms) > 1 {
			prefix = fmt.Sprintf("documents[%d]", i)
		}
		if err := schema.ValidateWithPrefix(m, topLevelRules, prefix); err != nil {
			errs = append(errs, err)
		}
	}
	joined := errors.Join(errs...)
	if joined == nil {
		return nil
	}
	var qe *schema.ErrQuery
	if errors.As(joined, &qe) {
		return fmt.Errorf("internal: malformed schema rule: %w", joined)
	}
	return joined
}
