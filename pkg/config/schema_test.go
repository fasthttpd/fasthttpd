package config

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mojatter/tree"
	"github.com/mojatter/tree/schema"
)

func TestValidateTreeMaps(t *testing.T) {
	testCases := []struct {
		caseName string
		docs     []tree.Map
		wantErr  string // substring; empty means success
	}{
		{
			caseName: "valid minimal",
			docs: []tree.Map{{
				"host":   tree.V("localhost"),
				"listen": tree.V(":8080"),
				"root":   tree.V("./public"),
			}},
		},
		{
			caseName: "handler missing type",
			docs: []tree.Map{{
				"handlers": tree.Map{"x": tree.Map{"root": tree.V("/")}},
			}},
			wantErr: `.handlers["x"]: "type" is required`,
		},
		{
			caseName: "unregistered handler type passes through",
			docs: []tree.Map{{
				"handlers": tree.Map{"x": tree.Map{
					"type":     tree.V("not-registered"),
					"whatever": tree.V(1),
				}},
			}},
		},
		{
			caseName: "handlers is not a map",
			docs: []tree.Map{{
				"handlers": tree.V("not map"),
			}},
			wantErr: ".handlers: expected array or map",
		},
		{
			caseName: "unknown top-level key",
			docs: []tree.Map{{
				"listn": tree.V(":8080"), // typo of "listen"
			}},
			wantErr: `.: unknown key "listn"`,
		},
		{
			caseName: "wrong type on top-level field",
			docs: []tree.Map{{
				"routes": tree.V("single"), // want array of Route
			}},
			wantErr: ".routes: expected array or map",
		},
		{
			caseName: "unknown server field",
			docs: []tree.Map{{
				"server": tree.Map{"futureField": tree.V("x")},
			}},
			wantErr: `.server: unknown key "futureField"`,
		},
		{
			caseName: "wrong type on server field",
			docs: []tree.Map{{
				"server": tree.Map{"concurrency": tree.V("many")},
			}},
			wantErr: ".server.concurrency: expected number, got string",
		},
		{
			caseName: "invalid server duration",
			docs: []tree.Map{{
				"server": tree.Map{"readTimeout": tree.V("not-a-duration")},
			}},
			wantErr: `.server.readTimeout: invalid duration "not-a-duration"`,
		},
		{
			caseName: "invalid server size",
			docs: []tree.Map{{
				"server": tree.Map{"readBufferSize": tree.V("not-a-size")},
			}},
			wantErr: `.server.readBufferSize: invalid size "not-a-size"`,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			err := ValidateTreeMaps(tc.docs)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateTreeMaps returned %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateTreeMaps returned nil, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// TestFromTreeMap exercises the typed Config decoding path on a
// schema-valid input. Error cases (unknown keys, wrong types, invalid
// durations) are exercised against ValidateTreeMaps in
// TestValidateTreeMaps — once the schema layer accepts a document,
// FromTreeMap can decode it without re-checking those constraints.
func TestFromTreeMap(t *testing.T) {
	doc := tree.Map{
		"server": tree.Map{
			"name":        tree.V("fasthttpd"),
			"concurrency": tree.V(100),
			"readTimeout": tree.V("60s"),
		},
	}
	if _, err := FromTreeMap(doc); err != nil {
		t.Fatalf("FromTreeMap returned %v, want nil", err)
	}
}

func TestDurationRule_Validate(t *testing.T) {
	testCases := []struct {
		caseName string
		rule     DurationRule
		node     tree.Node
		wantErr  string
	}{
		{caseName: "string ok", rule: DurationRule{}, node: tree.V("30s")},
		{caseName: "number ok", rule: DurationRule{}, node: tree.V(int64(time.Second))},
		{caseName: "nil skipped", rule: DurationRule{}, node: tree.Nil},
		{caseName: "invalid string", rule: DurationRule{}, node: tree.V("not"), wantErr: "invalid duration"},
		{caseName: "bool rejected", rule: DurationRule{}, node: tree.V(true), wantErr: "expected duration"},
		{caseName: "min ok", rule: DurationRule{Min: time.Second}, node: tree.V("10s")},
		{caseName: "min violated", rule: DurationRule{Min: time.Second}, node: tree.V("500ms"), wantErr: "less than min"},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			err := tc.rule.Validate(tc.node, ".x")
			checkErr(t, err, tc.wantErr)
		})
	}
}

func TestSizeRule_Validate(t *testing.T) {
	testCases := []struct {
		caseName string
		node     tree.Node
		wantErr  string
	}{
		{caseName: "string ok", node: tree.V("4k")},
		{caseName: "string with spaces ok", node: tree.V("8 KiB")},
		{caseName: "number ok", node: tree.V(int64(4096))},
		{caseName: "nil skipped", node: tree.Nil},
		{caseName: "invalid string", node: tree.V("not"), wantErr: "invalid size"},
		{caseName: "bool rejected", node: tree.V(true), wantErr: "expected size"},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			err := SizeRule{}.Validate(tc.node, ".x")
			checkErr(t, err, tc.wantErr)
		})
	}
}

// customRulerType implements [SchemaRuler] to verify the override
// path in [ruleForType].
type customRulerType struct{}

func (customRulerType) SchemaRule() schema.Rule { return schema.Bool{} }

// untaggedFieldStruct verifies the lowercased-field-name fallback for
// fields without a `yaml:"..."` tag — matching yaml/v3 decoder
// behavior.
type untaggedFieldStruct struct {
	Tagged   string `yaml:"explicit"`
	Rotation string // no tag → key "rotation"
	skipMe   string //nolint:unused // verifies unexported fields are ignored
}

// kitchenSink covers the kinds [SchemaFromStruct] must handle: the
// SchemaRuler override, primitives, slices, maps of strings, maps of
// tree.Map (special-cased), nested structs, pointers, and tagged-skip.
type kitchenSink struct {
	Custom   customRulerType     `yaml:"custom"`
	Str      string              `yaml:"str"`
	Int      int                 `yaml:"int"`
	Bool     bool                `yaml:"bool"`
	Float    float64             `yaml:"float"`
	Slice    []string            `yaml:"slice"`
	Strs     map[string]string   `yaml:"strs"`
	RawMap   tree.Map            `yaml:"rawMap"`
	Maps     map[string]tree.Map `yaml:"maps"`
	Nested   untaggedFieldStruct `yaml:"nested"`
	PtrToInt *int                `yaml:"ptrToInt"`
	Skipped  string              `yaml:"-"`
	Untagged bool                // key "untagged"
}

func TestSchemaFromStruct(t *testing.T) {
	got := SchemaFromStruct(kitchenSink{})

	wantKeys := []string{
		"custom", "str", "int", "bool", "float",
		"slice", "strs", "rawMap", "maps",
		"nested", "ptrToInt", "untagged",
	}
	for _, k := range wantKeys {
		if _, ok := got.KeyedRules[k]; !ok {
			t.Errorf("KeyedRules missing key %q", k)
		}
	}
	if _, ok := got.KeyedRules["Skipped"]; ok {
		t.Errorf(`KeyedRules contains "Skipped"; yaml:"-" should be skipped`)
	}
	if _, ok := got.KeyedRules["skipMe"]; ok {
		t.Errorf(`KeyedRules contains "skipMe"; unexported fields should be skipped`)
	}

	// SchemaRuler override: customRulerType returns schema.Bool{}.
	if _, ok := got.KeyedRules["custom"].(schema.Bool); !ok {
		t.Errorf("custom rule = %T; want schema.Bool", got.KeyedRules["custom"])
	}
	// Primitives.
	if _, ok := got.KeyedRules["str"].(schema.String); !ok {
		t.Errorf("str rule = %T; want schema.String", got.KeyedRules["str"])
	}
	if _, ok := got.KeyedRules["int"].(schema.Int); !ok {
		t.Errorf("int rule = %T; want schema.Int", got.KeyedRules["int"])
	}
	if _, ok := got.KeyedRules["bool"].(schema.Bool); !ok {
		t.Errorf("bool rule = %T; want schema.Bool", got.KeyedRules["bool"])
	}
	if _, ok := got.KeyedRules["float"].(schema.Float); !ok {
		t.Errorf("float rule = %T; want schema.Float", got.KeyedRules["float"])
	}
	// tree.Map → schema.Map{} (no KeyedRules — accept any shape).
	rawMap, ok := got.KeyedRules["rawMap"].(schema.Map)
	if !ok {
		t.Errorf("rawMap rule = %T; want schema.Map", got.KeyedRules["rawMap"])
	} else if len(rawMap.KeyedRules) != 0 {
		t.Errorf("rawMap rule has KeyedRules; tree.Map should be open")
	}
	// Nested struct → schema.Map with KeyedRules.
	nested, ok := got.KeyedRules["nested"].(schema.Map)
	if !ok {
		t.Errorf("nested rule = %T; want schema.Map", got.KeyedRules["nested"])
	} else {
		if _, ok := nested.KeyedRules["explicit"]; !ok {
			t.Errorf(`nested missing "explicit" key (from yaml:"explicit")`)
		}
		if _, ok := nested.KeyedRules["rotation"]; !ok {
			t.Errorf(`nested missing "rotation" key (lowercased Rotation)`)
		}
	}
}

// TestSchemaFromStruct_AppliedToConfig is a smoke test that the
// generator succeeds on the real Config struct and produces a schema
// that accepts every key the Config struct actually defines.
func TestSchemaFromStruct_AppliedToConfig(t *testing.T) {
	root := SchemaFromStruct(Config{})
	for _, k := range []string{
		"host", "listen", "ssl", "root", "server",
		"log", "accessLog", "errorPages",
		"filters", "handlers", "routes", "routesCache",
		"shutdownTimeout",
	} {
		if _, ok := root.KeyedRules[k]; !ok {
			t.Errorf("Config schema missing top-level key %q", k)
		}
	}
}

func TestRegisterHandlerSchema_Dispatch(t *testing.T) {
	RegisterHandlerSchema("test-fs-handler", schema.QueryRules{
		".": schema.Map{KeyedRules: map[string]schema.Rule{
			"type": schema.String{Enum: []string{"test-fs-handler"}},
			"root": schema.String{},
		}},
	})
	t.Cleanup(func() {
		schemaMu.Lock()
		delete(handlerSchemas, "test-fs-handler")
		schemaMu.Unlock()
	})

	testCases := []struct {
		caseName string
		handler  tree.Node
		wantErr  string
	}{
		{
			caseName: "valid",
			handler:  tree.Map{"type": tree.V("test-fs-handler"), "root": tree.V("/srv")},
		},
		{
			caseName: "unknown key",
			handler:  tree.Map{"type": tree.V("test-fs-handler"), "bogus": tree.V(1)},
			wantErr:  `.handlers["h"]: unknown key "bogus"`,
		},
		{
			caseName: "not a map",
			handler:  tree.V("scalar"),
			wantErr:  "expected map",
		},
		{
			caseName: "missing type",
			handler:  tree.Map{"root": tree.V("/srv")},
			wantErr:  `.handlers["h"]: "type" is required`,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			docs := []tree.Map{{"handlers": tree.Map{"h": tc.handler}}}
			err := ValidateTreeMaps(docs)
			checkErr(t, err, tc.wantErr)
		})
	}
}

func TestValidateTreeMaps_MultiDocPrefix(t *testing.T) {
	// With more than one document, errors are prefixed with
	// `documents[N]` so the user can locate the offending doc.
	docs := []tree.Map{
		{},
		{"handlers": tree.Map{"h": tree.Map{"root": tree.V("/srv")}}},
	}
	err := ValidateTreeMaps(docs)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "documents[1].handlers") {
		t.Errorf("error %q missing documents[1].handlers prefix", err.Error())
	}
}

func TestRegisterFilterSchema_Dispatch(t *testing.T) {
	RegisterFilterSchema("test-fs-filter", schema.QueryRules{
		".": schema.Map{KeyedRules: map[string]schema.Rule{
			"type": schema.String{Enum: []string{"test-fs-filter"}},
			"name": schema.String{},
		}},
	})
	t.Cleanup(func() {
		schemaMu.Lock()
		delete(filterSchemas, "test-fs-filter")
		schemaMu.Unlock()
	})

	testCases := []struct {
		caseName string
		filter   tree.Map
		wantErr  string
	}{
		{
			caseName: "valid",
			filter:   tree.Map{"type": tree.V("test-fs-filter"), "name": tree.V("x")},
		},
		{
			caseName: "missing type",
			filter:   tree.Map{"name": tree.V("x")},
			wantErr:  `.filters["f"]: "type" is required`,
		},
		{
			caseName: "unknown key",
			filter:   tree.Map{"type": tree.V("test-fs-filter"), "bogus": tree.V(1)},
			wantErr:  `.filters["f"]: unknown key "bogus"`,
		},
		{
			caseName: "filter is not a map",
			filter:   nil,
			wantErr:  "expected map",
		},
		{
			caseName: "unregistered filter type passes through",
			filter:   tree.Map{"type": tree.V("not-registered"), "whatever": tree.V(1)},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			var v tree.Node = tc.filter
			if tc.filter == nil {
				v = tree.V("not-a-map")
			}
			docs := []tree.Map{{"filters": tree.Map{"f": v}}}
			err := ValidateTreeMaps(docs)
			checkErr(t, err, tc.wantErr)
		})
	}
}

// TestValidateTreeMaps_MalformedSchemaQuery covers the case where a
// registered handler's QueryRules carries a query string that
// tree.Find cannot parse. ValidateTreeMaps should wrap the underlying
// *schema.ErrQuery with an "internal" prefix so the operator can tell
// it apart from a config error, while keeping the original ErrQuery
// reachable via errors.As.
func TestValidateTreeMaps_MalformedSchemaQuery(t *testing.T) {
	RegisterHandlerSchema("test-bad-query", schema.QueryRules{
		".[broken": schema.String{},
	})
	t.Cleanup(func() {
		schemaMu.Lock()
		delete(handlerSchemas, "test-bad-query")
		schemaMu.Unlock()
	})

	docs := []tree.Map{{
		"handlers": tree.Map{"x": tree.Map{"type": tree.V("test-bad-query")}},
	}}
	err := ValidateTreeMaps(docs)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "internal:") {
		t.Errorf("error %q missing 'internal:' prefix", err.Error())
	}
	var qe *schema.ErrQuery
	if !errors.As(err, &qe) {
		t.Errorf("error %v does not wrap *schema.ErrQuery", err)
	}
}

func checkErr(t *testing.T, err error, wantErr string) {
	t.Helper()
	if wantErr == "" {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", wantErr)
	}
	if !strings.Contains(err.Error(), wantErr) {
		t.Errorf("error %q does not contain %q", err.Error(), wantErr)
	}
}
