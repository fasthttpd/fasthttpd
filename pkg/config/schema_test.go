package config

import (
	"strings"
	"testing"
	"time"

	"github.com/mojatter/tree"
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
			wantErr: `handlers["x"]: missing 'type'`,
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

// TestFromTreeMap exercises the typed Config decoding path. Unknown
// top-level keys fail KnownFields strict decoding; unknown Server
// fields fail the same way one level deeper; invalid duration values
// fail Duration.UnmarshalYAML.
func TestFromTreeMap(t *testing.T) {
	testCases := []struct {
		caseName string
		doc      tree.Map
		wantErr  string // substring; empty means success
	}{
		{
			caseName: "valid server",
			doc: tree.Map{
				"server": tree.Map{
					"name":        tree.V("fasthttpd"),
					"concurrency": tree.V(100),
					"readTimeout": tree.V("60s"),
				},
			},
		},
		{
			caseName: "unknown top-level key",
			doc: tree.Map{
				"listn": tree.V(":8080"), // typo of "listen"
			},
			wantErr: "field listn not found",
		},
		{
			caseName: "wrong type on known top-level field",
			doc: tree.Map{
				"routes": tree.V("single"), // want array of Route
			},
			wantErr: "cannot unmarshal",
		},
		{
			caseName: "unknown server field",
			doc: tree.Map{
				"server": tree.Map{"futureField": tree.V("x")},
			},
			wantErr: "field futureField not found",
		},
		{
			caseName: "wrong type on known server field",
			doc: tree.Map{
				"server": tree.Map{"concurrency": tree.V("many")},
			},
			wantErr: "cannot unmarshal !!str `many` into int",
		},
		{
			caseName: "invalid server duration",
			doc: tree.Map{
				"server": tree.Map{"readTimeout": tree.V("not-a-duration")},
			},
			wantErr: "invalid duration",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			_, err := FromTreeMap(tc.doc)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("FromTreeMap returned %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("FromTreeMap returned nil, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestStringSchema_Validate(t *testing.T) {
	testCases := []struct {
		caseName string
		schema   StringSchema
		node     tree.Node
		wantErr  string
	}{
		{caseName: "bare string passes", schema: StringSchema{}, node: tree.V("x")},
		{caseName: "number rejected", schema: StringSchema{}, node: tree.V(1), wantErr: "expected string, got number"},
		{caseName: "enum hit", schema: StringSchema{Enum: []string{"a", "b"}}, node: tree.V("a")},
		{caseName: "enum miss", schema: StringSchema{Enum: []string{"a", "b"}}, node: tree.V("c"), wantErr: "not in allowed set"},
		{caseName: "regex match", schema: StringSchema{Regex: `^\d+$`}, node: tree.V("123")},
		{caseName: "regex miss", schema: StringSchema{Regex: `^\d+$`}, node: tree.V("abc"), wantErr: "does not match"},
		{caseName: "regex invalid", schema: StringSchema{Regex: `[`}, node: tree.V("x"), wantErr: "invalid regex"},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			err := tc.schema.Validate(tc.node)
			checkErr(t, err, tc.wantErr)
		})
	}
}

func TestBoolSchema_Validate(t *testing.T) {
	if err := (BoolSchema{}).Validate(tree.V(true)); err != nil {
		t.Errorf("true: %v", err)
	}
	if err := (BoolSchema{}).Validate(tree.V("true")); err == nil || !strings.Contains(err.Error(), "expected bool") {
		t.Errorf("string: err=%v want 'expected bool'", err)
	}
}

func TestIntSchema_Validate(t *testing.T) {
	testCases := []struct {
		caseName string
		schema   IntSchema
		node     tree.Node
		wantErr  string
	}{
		{caseName: "bare int passes", schema: IntSchema{}, node: tree.V(42)},
		{caseName: "string rejected", schema: IntSchema{}, node: tree.V("42"), wantErr: "expected number"},
		{caseName: "min ok", schema: IntSchema{Min: Int64Ptr(0)}, node: tree.V(1)},
		{caseName: "min violated", schema: IntSchema{Min: Int64Ptr(0)}, node: tree.V(-1), wantErr: "less than min"},
		{caseName: "max ok", schema: IntSchema{Max: Int64Ptr(100)}, node: tree.V(50)},
		{caseName: "max violated", schema: IntSchema{Max: Int64Ptr(100)}, node: tree.V(101), wantErr: "greater than max"},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			err := tc.schema.Validate(tc.node)
			checkErr(t, err, tc.wantErr)
		})
	}
}

func TestDurationSchema_Validate(t *testing.T) {
	testCases := []struct {
		caseName string
		schema   DurationSchema
		node     tree.Node
		wantErr  string
	}{
		{caseName: "string ok", schema: DurationSchema{}, node: tree.V("30s")},
		{caseName: "number ok", schema: DurationSchema{}, node: tree.V(int64(time.Second))},
		{caseName: "invalid string", schema: DurationSchema{}, node: tree.V("not"), wantErr: "invalid duration"},
		{caseName: "bool rejected", schema: DurationSchema{}, node: tree.V(true), wantErr: "expected duration"},
		{caseName: "min ok", schema: DurationSchema{Min: time.Second}, node: tree.V("10s")},
		{caseName: "min violated", schema: DurationSchema{Min: time.Second}, node: tree.V("500ms"), wantErr: "less than min"},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			err := tc.schema.Validate(tc.node)
			checkErr(t, err, tc.wantErr)
		})
	}
}

func TestArraySchema_Validate(t *testing.T) {
	if err := (ArraySchema{}).Validate(tree.A("x")); err != nil {
		t.Errorf("array: %v", err)
	}
	if err := (ArraySchema{}).Validate(tree.V("x")); err == nil || !strings.Contains(err.Error(), "expected array") {
		t.Errorf("scalar: err=%v", err)
	}
}

func TestMapSchema_Validate(t *testing.T) {
	if err := (MapSchema{}).Validate(tree.Map{}); err != nil {
		t.Errorf("map: %v", err)
	}
	if err := (MapSchema{}).Validate(tree.V("x")); err == nil || !strings.Contains(err.Error(), "expected map") {
		t.Errorf("scalar: err=%v", err)
	}
}

func TestAnySchema_Validate(t *testing.T) {
	// AnySchema accepts every node shape without error.
	inputs := []tree.Node{tree.V("x"), tree.V(1), tree.V(true), tree.Map{}, tree.A("x")}
	for _, n := range inputs {
		if err := (AnySchema{}).Validate(n); err != nil {
			t.Errorf("Validate(%v) = %v, want nil", n, err)
		}
	}
	// AnySchema is terminal (skip-descent marker).
	var _ terminalSchema = AnySchema{}
}

func TestMapOfSchema_Validate(t *testing.T) {
	schema := MapOfSchema{Value: StringSchema{}}
	if err := schema.Validate(tree.Map{"a": tree.V("x"), "b": tree.V("y")}); err != nil {
		t.Errorf("all strings: %v", err)
	}
	err := schema.Validate(tree.Map{"a": tree.V("x"), "b": tree.V(1)})
	if err == nil || !strings.Contains(err.Error(), `["b"]`) {
		t.Errorf("mixed: err=%v want key tag", err)
	}
	if err := schema.Validate(tree.V("x")); err == nil || !strings.Contains(err.Error(), "expected map") {
		t.Errorf("scalar: err=%v", err)
	}
	// MapOfSchema is terminal.
	var _ terminalSchema = MapOfSchema{}
}

func TestValidateTree(t *testing.T) {
	schemas := map[string]Schema{
		".known":       StringSchema{},
		".nested.name": StringSchema{},
		".arr[]":       IntSchema{},
	}
	testCases := []struct {
		caseName string
		root     tree.Map
		strict   bool
		wantErrs []string
	}{
		{
			caseName: "strict unknown leaf reported",
			root:     tree.Map{"extra": tree.V("x")},
			strict:   true,
			wantErrs: []string{"extra: unknown key"},
		},
		{
			caseName: "lenient unknown leaf ignored",
			root:     tree.Map{"extra": tree.V("x")},
		},
		{
			caseName: "known leaf validates",
			root:     tree.Map{"known": tree.V(1)},
			strict:   true,
			wantErrs: []string{"known: expected string"},
		},
		{
			caseName: "nested descent works",
			root:     tree.Map{"nested": tree.Map{"name": tree.V(1)}},
			strict:   true,
			wantErrs: []string{"nested.name: expected string"},
		},
		{
			caseName: "array element schema",
			root:     tree.Map{"arr": tree.A("x", "y")},
			strict:   true,
			wantErrs: []string{"arr[0]", "arr[1]"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			errs := validateTree(tc.root, schemas, tc.strict, "")
			if len(tc.wantErrs) == 0 {
				if len(errs) > 0 {
					t.Fatalf("unexpected errors: %v", errs)
				}
				return
			}
			for _, want := range tc.wantErrs {
				found := false
				for _, e := range errs {
					if strings.Contains(e.Error(), want) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("errors %v missing substring %q", errs, want)
				}
			}
		})
	}
}

func TestValidateTree_TerminalSkipsDescent(t *testing.T) {
	// With MapOfSchema (terminal), the walker validates the container
	// then returns tree.SkipWalk, so no "unknown key" reports fire for
	// the user-keyed children inside.
	schemas := map[string]Schema{
		".mapping": MapOfSchema{Value: StringSchema{}},
	}
	root := tree.Map{"mapping": tree.Map{"user-key-1": tree.V("v1"), "user-key-2": tree.V("v2")}}
	if errs := validateTree(root, schemas, true, ""); len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

func TestRenderPath(t *testing.T) {
	keys := []any{"a", "b", 2, "c"}
	if got := renderPathLookup(keys); got != ".a.b[].c" {
		t.Errorf("renderPathLookup = %q, want %q", got, ".a.b[].c")
	}
	if got := renderPathDisplay(keys); got != ".a.b[2].c" {
		t.Errorf("renderPathDisplay = %q, want %q", got, ".a.b[2].c")
	}
}

func TestTypeName(t *testing.T) {
	testCases := []struct {
		caseName string
		node     tree.Node
		want     string
	}{
		{caseName: "string", node: tree.V("x"), want: "string"},
		{caseName: "bool", node: tree.V(true), want: "bool"},
		{caseName: "number", node: tree.V(1), want: "number"},
		{caseName: "array", node: tree.A(), want: "array"},
		{caseName: "map", node: tree.Map{}, want: "map"},
		{caseName: "nil", node: tree.Nil, want: "nil"},
	}
	for _, tc := range testCases {
		t.Run(tc.caseName, func(t *testing.T) {
			if got := typeName(tc.node.Type()); got != tc.want {
				t.Errorf("typeName(%v) = %q, want %q", tc.node, got, tc.want)
			}
		})
	}
}

func TestInt64Ptr(t *testing.T) {
	p := Int64Ptr(42)
	if p == nil || *p != 42 {
		t.Errorf("Int64Ptr(42) = %v, want *42", p)
	}
}

func TestRegisterHandlerSchema_Dispatch(t *testing.T) {
	RegisterHandlerSchema("test-fs-handler", map[string]Schema{
		".type": StringSchema{Enum: []string{"test-fs-handler"}},
		".root": StringSchema{},
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
			wantErr:  ".bogus: unknown key",
		},
		{
			caseName: "not a map",
			handler:  tree.V("scalar"),
			wantErr:  "expected map",
		},
		{
			caseName: "missing type",
			handler:  tree.Map{"root": tree.V("/srv")},
			wantErr:  `handlers["h"]: missing 'type'`,
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
	// `documents[N].` so the user can locate the offending doc.
	docs := []tree.Map{
		{},
		{"handlers": tree.Map{"h": tree.Map{"root": tree.V("/srv")}}},
	}
	err := ValidateTreeMaps(docs)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "documents[1].handlers") {
		t.Errorf("error %q missing documents[1] prefix", err.Error())
	}
}

func TestRegisterFilterSchema_Dispatch(t *testing.T) {
	// Register a throwaway schema and verify ValidateTreeMaps routes
	// filter entries by their "type" field. Also covers
	// RegisterFilterSchema and validateFilter in this package's tests.
	RegisterFilterSchema("test-fs-filter", map[string]Schema{
		".type": StringSchema{Enum: []string{"test-fs-filter"}},
		".name": StringSchema{},
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
			wantErr:  `filters["f"]: missing 'type'`,
		},
		{
			caseName: "unknown key",
			filter:   tree.Map{"type": tree.V("test-fs-filter"), "bogus": tree.V(1)},
			wantErr:  ".bogus: unknown key",
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
