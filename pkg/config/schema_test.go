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
