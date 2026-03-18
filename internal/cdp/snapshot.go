package cdp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// SnapshotOptions controls how the accessibility tree is captured.
type SnapshotOptions struct {
	// InteractiveOnly filters to only interactive elements (button, link, textbox, etc.)
	InteractiveOnly bool
	// Compact removes empty structural nodes (generic, none, group with no name)
	Compact bool
	// MaxDepth limits the tree depth (0 = unlimited)
	MaxDepth int
}

// SnapshotNode is a single node in the accessibility tree.
type SnapshotNode struct {
	Ref      string         // @e1, @e2, etc.
	Role     string         // button, link, textbox, heading, etc.
	Name     string         // accessible name
	Value    string         // current value (for inputs)
	Level    int            // nesting depth
	Children []SnapshotNode
}

// SnapshotResult contains the accessibility tree.
type SnapshotResult struct {
	Nodes []SnapshotNode
}

// interactiveRoles is the set of roles considered interactive.
var interactiveRoles = map[string]bool{
	"button":            true,
	"link":              true,
	"textbox":           true,
	"checkbox":          true,
	"radio":             true,
	"combobox":          true,
	"listbox":           true,
	"menuitem":          true,
	"menuitemcheckbox":  true,
	"menuitemradio":     true,
	"option":            true,
	"searchbox":         true,
	"slider":            true,
	"spinbutton":        true,
	"switch":            true,
	"tab":               true,
	"treeitem":          true,
}

// structuralRoles are removed in Compact mode when they have no name.
var structuralRoles = map[string]bool{
	"generic": true,
	"none":    true,
	"group":   true,
	"region":  true,
}

// rawAXNode is a permissive representation of a CDP accessibility node.
// Using raw JSON avoids strict enum parsing that breaks with newer Chromium versions.
type rawAXNode struct {
	NodeID           string          `json:"nodeId"`
	Ignored          bool            `json:"ignored"`
	Role             json.RawMessage `json:"role"`
	Name             json.RawMessage `json:"name"`
	Value            json.RawMessage `json:"value"`
	ParentID         string          `json:"parentId"`
	ChildIDs         []string        `json:"childIds"`
	BackendDOMNodeID int64           `json:"backendDOMNodeId"`
}

// axRawValueString extracts the "value" field from a raw AX property JSON.
// AX properties have the shape: {"type":"string","value":"actual text"}
func axRawValueString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var prop struct {
		Value any `json:"value"`
	}
	if err := json.Unmarshal(raw, &prop); err != nil {
		return ""
	}
	if prop.Value == nil {
		return ""
	}
	return fmt.Sprintf("%v", prop.Value)
}

// Snapshot captures the accessibility tree of the current page.
// Refs (@e1, @e2, ...) are assigned in pre-order traversal and stored in
// the session's refMap for use by subsequent action calls.
// Uses raw CDP calls to avoid strict enum parsing issues with newer Chromium.
func Snapshot(s *Session, opts SnapshotOptions) (*SnapshotResult, error) {
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()

	// Use raw CDP executor to call Accessibility.getFullAXTree permissively.
	// This avoids cdproto's strict enum parsing which breaks with newer Chromium.
	var nodes []rawAXNode
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		// Enable accessibility domain first
		executor := chromedp.FromContext(ctx)
		if executor != nil {
			_ = executor.Target.Execute(ctx, "Accessibility.enable", nil, nil)
		}
		var err error
		nodes, err = cdpGetFullAXTree(ctx)
		return err
	})); err != nil {
		return nil, fmt.Errorf("snapshot: %w", err)
	}

	// Build a map from nodeID → Node for tree reconstruction
	nodeMap := make(map[string]*rawAXNode, len(nodes))
	for i := range nodes {
		nodeMap[nodes[i].NodeID] = &nodes[i]
	}

	// Find root nodes: those not referenced as a child of any other node
	childSet := make(map[string]bool)
	for i := range nodes {
		for _, cid := range nodes[i].ChildIDs {
			childSet[cid] = true
		}
	}
	var roots []*rawAXNode
	for i := range nodes {
		if !childSet[nodes[i].NodeID] {
			roots = append(roots, &nodes[i])
		}
	}

	// Assign refs in pre-order traversal
	refCounter := 0
	refMap := make(map[string]int64)

	var buildTree func(n *rawAXNode, depth int) (SnapshotNode, bool)
	buildTree = func(n *rawAXNode, depth int) (SnapshotNode, bool) {
		if opts.MaxDepth > 0 && depth > opts.MaxDepth {
			return SnapshotNode{}, false
		}

		if n.Ignored {
			return SnapshotNode{}, false
		}

		role := axRawValueString(n.Role)
		name := axRawValueString(n.Name)
		value := axRawValueString(n.Value)

		// Build children first regardless of filter decisions
		var children []SnapshotNode
		for _, cid := range n.ChildIDs {
			if child, ok := nodeMap[cid]; ok {
				if childNode, ok2 := buildTree(child, depth+1); ok2 {
					children = append(children, childNode)
				}
			}
		}

		// InteractiveOnly: skip non-interactive nodes but hoist children
		if opts.InteractiveOnly && !interactiveRoles[role] {
			if len(children) > 0 {
				return SnapshotNode{Children: children}, true
			}
			return SnapshotNode{}, false
		}

		// Compact: skip structural nodes with no name, hoist children
		if opts.Compact && structuralRoles[role] && name == "" {
			if len(children) > 0 {
				return SnapshotNode{Children: children}, true
			}
			return SnapshotNode{}, false
		}

		// Assign ref to this node
		refCounter++
		ref := fmt.Sprintf("@e%d", refCounter)

		if n.BackendDOMNodeID != 0 {
			refMap[ref] = n.BackendDOMNodeID
		}

		node := SnapshotNode{
			Ref:      ref,
			Role:     role,
			Name:     name,
			Value:    value,
			Level:    depth,
			Children: children,
		}
		return node, true
	}

	var topNodes []SnapshotNode
	for _, r := range roots {
		if node, ok := buildTree(r, 0); ok {
			topNodes = append(topNodes, flattenPhantom([]SnapshotNode{node})...)
		}
	}

	// Store refMap in session for subsequent action resolution
	s.refMap = refMap

	return &SnapshotResult{
		Nodes: topNodes,
	}, nil
}

// rawCDPResult is a permissive container for receiving raw CDP responses.
type rawCDPResult struct {
	Nodes []rawAXNode `json:"nodes"`
}

// cdpGetFullAXTree calls Accessibility.getFullAXTree via chromedp's executor
// using a permissive result type that tolerates unknown enum values.
func cdpGetFullAXTree(ctx context.Context) ([]rawAXNode, error) {
	executor := chromedp.FromContext(ctx)
	if executor == nil {
		return nil, fmt.Errorf("no chromedp executor in context")
	}
	var result rawCDPResult
	if err := executor.Target.Execute(ctx, "Accessibility.getFullAXTree", nil, &result); err != nil {
		return nil, err
	}
	return result.Nodes, nil
}

// flattenPhantom unwraps phantom nodes (Ref == "") by hoisting their children.
func flattenPhantom(nodes []SnapshotNode) []SnapshotNode {
	var result []SnapshotNode
	for _, n := range nodes {
		if n.Ref == "" {
			result = append(result, flattenPhantom(n.Children)...)
		} else {
			n.Children = flattenPhantom(n.Children)
			result = append(result, n)
		}
	}
	return result
}

// FormatText returns a human-readable text representation of the snapshot.
//
// Example:
//
//	@e1 heading "Welcome to TON"
//	  @e2 link "resistance.ton"
//	  @e3 textbox "Search..." value=""
//	  @e4 button "Go"
func (r *SnapshotResult) FormatText() string {
	var sb strings.Builder
	var writeNode func(n SnapshotNode, indent int)
	writeNode = func(n SnapshotNode, indent int) {
		prefix := strings.Repeat("  ", indent)
		line := fmt.Sprintf("%s%s %s %q", prefix, n.Ref, n.Role, n.Name)
		if n.Value != "" {
			line += fmt.Sprintf(" value=%q", n.Value)
		}
		sb.WriteString(line)
		sb.WriteByte('\n')
		for _, c := range n.Children {
			writeNode(c, indent+1)
		}
	}
	for _, n := range r.Nodes {
		writeNode(n, 0)
	}
	return sb.String()
}

// ResolveRef looks up a ref string like "@e5" in the last snapshot's refMap
// and returns the corresponding BackendDOMNodeID.
func (s *Session) ResolveRef(ref string) (int64, error) {
	id, ok := s.refMap[ref]
	if !ok {
		return 0, fmt.Errorf("ref %q not found in last snapshot", ref)
	}
	return id, nil
}
