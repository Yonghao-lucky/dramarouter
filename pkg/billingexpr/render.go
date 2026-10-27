package billingexpr

import (
	"math"
	"strings"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

// RenderTierCalculation rewrites the tier body that billed a task request so
// every u("key") usage lookup carries the value the request recorded, e.g.
//
//	max(0, 0 - 5) * 0.03 + 0 * 0.075 + 4 * 0.075
//
// A reviewer reading a consume log can then check the arithmetic instead of
// reconstructing it from a parameter vector. Values the expression probes but
// the usage vector does not carry (request/header probes) stay symbolic, and a
// usage key absent from the map is left as u("key") rather than raising.
//
// The caller owns presentation: units, totals and the group-ratio stage are
// outside the expression language, so this returns the arithmetic only. It
// reports false when the expression cannot be parsed or holds no tier with the
// given label, letting the caller fall back to a plain parameter list.
func RenderTierCalculation(exprStr, tierLabel string, usage map[string]any) (string, bool) {
	if strings.TrimSpace(exprStr) == "" {
		return "", false
	}
	_, body := ParseExprVersion(exprStr)
	tree, err := parser.Parse(body)
	if err != nil || tree == nil || tree.Node == nil {
		return "", false
	}
	node := tree.Node
	label := strings.TrimSpace(tierLabel)
	if label != "" {
		call := findTierCall(node, label)
		if call == nil {
			return "", false
		}
		node = call.Arguments[1]
	}
	if len(usage) > 0 {
		ast.Walk(&node, usageSubstituter{usage: usage})
	}
	return strings.TrimSpace(node.String()), true
}

// findTierCall returns the tier("label", body) call whose label matches.
// Tier labels are unique per expression, so the first match is the billed one.
func findTierCall(node ast.Node, label string) *ast.CallNode {
	var found *ast.CallNode
	ast.Find(node, func(candidate ast.Node) bool {
		if found != nil {
			return false
		}
		call, ok := candidate.(*ast.CallNode)
		if !ok || len(call.Arguments) != 2 {
			return false
		}
		callee, ok := call.Callee.(*ast.IdentifierNode)
		if !ok || callee.Value != "tier" {
			return false
		}
		tierLabel, ok := call.Arguments[0].(*ast.StringNode)
		if !ok || strings.TrimSpace(tierLabel.Value) != label {
			return false
		}
		found = call
		return true
	})
	return found
}

// usageSubstituter replaces u("key") calls with the literal the request
// recorded. Walk visits children before parents, so a nested call is patched
// from the inside out and the enclosing node keeps a consistent subtree.
type usageSubstituter struct {
	usage map[string]any
}

func (s usageSubstituter) Visit(node *ast.Node) {
	call, ok := (*node).(*ast.CallNode)
	if !ok || len(call.Arguments) != 1 {
		return
	}
	callee, ok := call.Callee.(*ast.IdentifierNode)
	if !ok || callee.Value != "u" {
		return
	}
	key, ok := call.Arguments[0].(*ast.StringNode)
	if !ok {
		return
	}
	value, present := s.usage[strings.TrimSpace(key.Value)]
	if !present {
		return
	}
	if literal, ok := usageLiteral(value); ok {
		ast.Patch(node, literal)
	}
}

// usageLiteral builds an AST literal for a usage fact. Plugin facts arrive as
// sobek values and deserialized snapshots arrive as JSON floats, so integers
// must stay integers: 4s must render as 4, not 4.0.
func usageLiteral(value any) (ast.Node, bool) {
	switch typed := value.(type) {
	case float64:
		return numberLiteral(typed), true
	case float32:
		return numberLiteral(float64(typed)), true
	case int:
		return &ast.IntegerNode{Value: typed}, true
	case int32:
		return &ast.IntegerNode{Value: int(typed)}, true
	case int64:
		if typed <= math.MaxInt32 && typed >= math.MinInt32 {
			return &ast.IntegerNode{Value: int(typed)}, true
		}
		return &ast.FloatNode{Value: float64(typed)}, true
	case string:
		return &ast.StringNode{Value: typed}, true
	case bool:
		return &ast.BoolNode{Value: typed}, true
	}
	return nil, false
}

func numberLiteral(value float64) ast.Node {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return &ast.FloatNode{Value: value}
	}
	if value == math.Trunc(value) && math.Abs(value) < 1e15 {
		return &ast.IntegerNode{Value: int(value)}
	}
	return &ast.FloatNode{Value: value}
}
