package config

import (
	"errors"
	"fmt"
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
		return fmt.Errorf("%s: missing 'type'", q)
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
		return fmt.Errorf("%s: missing 'type'", q)
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
// validation. Only `handlers` / `filters` are checked here — other
// top-level fields (host, listen, server, routes, ...) are validated
// by FromTreeMap via the yaml decoder with KnownFields(true).
var topLevelRules = schema.QueryRules{
	".handlers": schema.Every{Rules: schema.QueryRules{
		".": HandlerDispatch{},
	}},
	".filters": schema.Every{Rules: schema.QueryRules{
		".": FilterDispatch{},
	}},
}

// ValidateTreeMaps runs schema-driven validation over each document's
// free-form handlers / filters subtrees. Typed portions of Config
// (including the Server struct) are validated by FromTreeMap, so
// this function intentionally does not re-check them.
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
