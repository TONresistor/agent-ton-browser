package cdp

import (
	"encoding/json"
	"testing"
)

// ---- axRawValueString ----

func TestAxRawValueString_Nil(t *testing.T) {
	if got := axRawValueString(nil); got != "" {
		t.Errorf("axRawValueString(nil) = %q, want ''", got)
	}
}

func TestAxRawValueString_Empty(t *testing.T) {
	if got := axRawValueString(json.RawMessage{}); got != "" {
		t.Errorf("axRawValueString(empty) = %q, want ''", got)
	}
}

func TestAxRawValueString_ValidJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "string value",
			input: `{"type":"string","value":"hello world"}`,
			want:  "hello world",
		},
		{
			name:  "numeric value",
			input: `{"type":"integer","value":42}`,
			want:  "42",
		},
		{
			name:  "null value",
			input: `{"type":"null","value":null}`,
			want:  "",
		},
		{
			name:  "boolean value",
			input: `{"type":"boolean","value":true}`,
			want:  "true",
		},
		{
			name:  "missing value field",
			input: `{"type":"string"}`,
			want:  "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := axRawValueString(json.RawMessage(tc.input))
			if got != tc.want {
				t.Errorf("axRawValueString(%s) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestAxRawValueString_InvalidJSON(t *testing.T) {
	got := axRawValueString(json.RawMessage(`not valid json`))
	if got != "" {
		t.Errorf("axRawValueString(invalid JSON) = %q, want ''", got)
	}
}

// ---- flattenPhantom ----

func TestFlattenPhantom_Empty(t *testing.T) {
	result := flattenPhantom(nil)
	if len(result) != 0 {
		t.Errorf("flattenPhantom(nil) returned %d nodes, want 0", len(result))
	}
}

func TestFlattenPhantom_NoPhantoms(t *testing.T) {
	nodes := []SnapshotNode{
		{Ref: "@e1", Role: "button", Name: "Click me"},
		{Ref: "@e2", Role: "link", Name: "Go"},
	}
	result := flattenPhantom(nodes)
	if len(result) != 2 {
		t.Fatalf("flattenPhantom with no phantoms: got %d nodes, want 2", len(result))
	}
	if result[0].Ref != "@e1" || result[1].Ref != "@e2" {
		t.Errorf("flattenPhantom changed non-phantom nodes: got %v", result)
	}
}

func TestFlattenPhantom_HoistsChildren(t *testing.T) {
	// Phantom (Ref="") wrapping two real children — children are hoisted.
	nodes := []SnapshotNode{
		{
			Ref: "",
			Children: []SnapshotNode{
				{Ref: "@e1", Role: "button", Name: "A"},
				{Ref: "@e2", Role: "link", Name: "B"},
			},
		},
	}
	result := flattenPhantom(nodes)
	if len(result) != 2 {
		t.Fatalf("flattenPhantom: expected 2 hoisted children, got %d", len(result))
	}
	if result[0].Ref != "@e1" {
		t.Errorf("first child: got %q, want '@e1'", result[0].Ref)
	}
	if result[1].Ref != "@e2" {
		t.Errorf("second child: got %q, want '@e2'", result[1].Ref)
	}
}

func TestFlattenPhantom_NestedPhantoms(t *testing.T) {
	// phantom → phantom → real node: should end up with just the real node.
	nodes := []SnapshotNode{
		{
			Ref: "",
			Children: []SnapshotNode{
				{
					Ref: "",
					Children: []SnapshotNode{
						{Ref: "@e1", Role: "textbox", Name: "Input"},
					},
				},
			},
		},
	}
	result := flattenPhantom(nodes)
	if len(result) != 1 {
		t.Fatalf("flattenPhantom nested phantoms: got %d nodes, want 1", len(result))
	}
	if result[0].Ref != "@e1" {
		t.Errorf("expected '@e1', got %q", result[0].Ref)
	}
}

func TestFlattenPhantom_PhantomWithNoChildren(t *testing.T) {
	// Phantom with no children is dropped entirely.
	nodes := []SnapshotNode{
		{Ref: "", Children: nil},
		{Ref: "@e1", Role: "button", Name: "Real"},
	}
	result := flattenPhantom(nodes)
	if len(result) != 1 {
		t.Fatalf("expected 1 node, got %d", len(result))
	}
	if result[0].Ref != "@e1" {
		t.Errorf("expected '@e1', got %q", result[0].Ref)
	}
}

func TestFlattenPhantom_PreservesRealNodeChildren(t *testing.T) {
	// Real node with a phantom child: phantom is unwrapped, grandchildren promoted.
	nodes := []SnapshotNode{
		{
			Ref:  "@e1",
			Role: "list",
			Children: []SnapshotNode{
				{
					Ref: "", // phantom
					Children: []SnapshotNode{
						{Ref: "@e2", Role: "listitem", Name: "Item"},
					},
				},
			},
		},
	}
	result := flattenPhantom(nodes)
	if len(result) != 1 {
		t.Fatalf("expected 1 top-level node, got %d", len(result))
	}
	if len(result[0].Children) != 1 {
		t.Fatalf("expected 1 child after phantom removal, got %d", len(result[0].Children))
	}
	if result[0].Children[0].Ref != "@e2" {
		t.Errorf("expected child '@e2', got %q", result[0].Children[0].Ref)
	}
}

// ---- interactiveRoles ----

func TestInteractiveRoles_Contains(t *testing.T) {
	required := []string{
		"button", "link", "textbox",
		"checkbox", "radio", "combobox",
		"listbox", "menuitem", "option",
		"searchbox", "slider", "tab",
	}
	for _, role := range required {
		if !interactiveRoles[role] {
			t.Errorf("expected %q to be in interactiveRoles", role)
		}
	}
}

func TestInteractiveRoles_DoesNotContainStructural(t *testing.T) {
	structural := []string{"generic", "none", "group", "region", "heading"}
	for _, role := range structural {
		if interactiveRoles[role] {
			t.Errorf("structural role %q should not be in interactiveRoles", role)
		}
	}
}
