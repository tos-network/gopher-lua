package sema

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/tos-network/tolang/tol/ast"
	"github.com/tos-network/tolang/tol/diag"
	"golang.org/x/crypto/sha3"
)

// TypedModule is the semantic-checked representation used by lowering.
type TypedModule struct {
	AST *ast.Module
}

func Check(filename string, m *ast.Module) (*TypedModule, diag.Diagnostics) {
	var diags diag.Diagnostics
	if m == nil {
		return nil, diags
	}

	if m.Version != "0.2" {
		diags = append(diags, diag.Diagnostic{
			Code:    diag.CodeSemaUnsupportedVer,
			Message: "unsupported TOL version (only 0.2 is accepted in this milestone)",
			Span: diag.Span{
				File: filename,
				Start: diag.Position{
					Line:   1,
					Column: 1,
				},
				End: diag.Position{
					Line:   1,
					Column: 1,
				},
			},
		})
	}

	if m.Contract == nil {
		diags = append(diags, diag.Diagnostic{
			Code:    diag.CodeSemaMissingContract,
			Message: "missing contract declaration",
			Span: diag.Span{
				File: filename,
				Start: diag.Position{
					Line:   1,
					Column: 1,
				},
				End: diag.Position{
					Line:   1,
					Column: 1,
				},
			},
		})
	}

	if m.Contract != nil {
		funcVis := map[string]string{}
		for _, fn := range m.Contract.Functions {
			funcVis[fn.Name] = functionVisibility(fn.Modifiers)
		}

		if m.Contract.Storage != nil {
			slotSeen := map[string]struct{}{}
			for _, slot := range m.Contract.Storage.Slots {
				if _, ok := slotSeen[slot.Name]; ok {
					diags = append(diags, diag.Diagnostic{
						Code:    diag.CodeSemaDuplicateSlot,
						Message: fmt.Sprintf("duplicate storage slot '%s'", slot.Name),
						Span: diag.Span{
							File: filename,
							Start: diag.Position{
								Line:   1,
								Column: 1,
							},
							End: diag.Position{
								Line:   1,
								Column: 1,
							},
						},
					})
				} else {
					slotSeen[slot.Name] = struct{}{}
				}
			}
		}

		funcSeen := map[string]struct{}{}
		selectorSeen := map[string]string{}
		for _, fn := range m.Contract.Functions {
			if _, ok := funcSeen[fn.Name]; ok {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaDuplicateFunction,
					Message: fmt.Sprintf("duplicate function '%s' (overload support not implemented yet)", fn.Name),
					Span: diag.Span{
						File: filename,
						Start: diag.Position{
							Line:   1,
							Column: 1,
						},
						End: diag.Position{
							Line:   1,
							Column: 1,
						},
					},
				})
			} else {
				funcSeen[fn.Name] = struct{}{}
			}
			if fn.SelectorOverride != "" && !isValidSelectorOverride(fn.SelectorOverride) {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidSelector,
					Message: fmt.Sprintf("invalid @selector value '%s' (expected 0x followed by 8 hex chars)", fn.SelectorOverride),
					Span:    defaultSpan(filename),
				})
			}
			if key, ok := selectorDispatchKey(fn); ok {
				if prev, exists := selectorSeen[key]; exists {
					diags = append(diags, diag.Diagnostic{
						Code:    diag.CodeSemaDuplicateSelector,
						Message: fmt.Sprintf("duplicate external/public selector key '%s' between functions '%s' and '%s'", key, prev, fn.Name),
						Span:    defaultSpan(filename),
					})
				} else {
					selectorSeen[key] = fn.Name
				}
			}
			checkStatements(filename, m.Contract.Name, funcVis, fn.Body, 0, &diags)
		}

		if m.Contract.Constructor != nil {
			checkStatements(filename, m.Contract.Name, funcVis, m.Contract.Constructor.Body, 0, &diags)
		}
		if m.Contract.Fallback != nil {
			checkStatements(filename, m.Contract.Name, funcVis, m.Contract.Fallback.Body, 0, &diags)
		}
	}

	if diags.HasErrors() {
		return nil, diags
	}
	return &TypedModule{AST: m}, nil
}

func checkStatements(filename string, contractName string, funcVis map[string]string, stmts []ast.Statement, loopDepth int, diags *diag.Diagnostics) {
	for _, s := range stmts {
		checkExpr(contractName, funcVis, filename, s.Expr, diags)
		checkExpr(contractName, funcVis, filename, s.Target, diags)
		checkExpr(contractName, funcVis, filename, s.Cond, diags)
		checkExpr(contractName, funcVis, filename, s.Post, diags)
		if s.Init != nil {
			checkStatements(filename, contractName, funcVis, []ast.Statement{*s.Init}, loopDepth, diags)
		}
		switch s.Kind {
		case "break":
			if loopDepth <= 0 {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaBreakOutsideLoop,
					Message: "break used outside loop",
					Span:    defaultSpan(filename),
				})
			}
		case "continue":
			if loopDepth <= 0 {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaContinueOutsideLoop,
					Message: "continue used outside loop",
					Span:    defaultSpan(filename),
				})
			}
		case "set":
			if !isAssignableTarget(s.Target) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidSetTarget,
					Message: "set target must be identifier, member access, or index access",
					Span:    defaultSpan(filename),
				})
			}
		case "if":
			if s.Cond == nil {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaMissingCondition,
					Message: "if statement requires a condition expression",
					Span:    defaultSpan(filename),
				})
			}
			checkStatements(filename, contractName, funcVis, s.Then, loopDepth, diags)
			checkStatements(filename, contractName, funcVis, s.Else, loopDepth, diags)
		case "while":
			if s.Cond == nil {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaMissingCondition,
					Message: "while statement requires a condition expression",
					Span:    defaultSpan(filename),
				})
			}
			checkStatements(filename, contractName, funcVis, s.Body, loopDepth+1, diags)
		case "for":
			checkStatements(filename, contractName, funcVis, s.Body, loopDepth+1, diags)
		default:
			checkStatements(filename, contractName, funcVis, s.Then, loopDepth, diags)
			checkStatements(filename, contractName, funcVis, s.Else, loopDepth, diags)
			checkStatements(filename, contractName, funcVis, s.Body, loopDepth, diags)
		}
	}
}

func isAssignableTarget(e *ast.Expr) bool {
	if e == nil {
		return false
	}
	switch e.Kind {
	case "ident", "member", "index":
		return true
	default:
		return false
	}
}

func checkExpr(contractName string, funcVis map[string]string, filename string, e *ast.Expr, diags *diag.Diagnostics) {
	if e == nil {
		return
	}
	switch e.Kind {
	case "call":
		if e.Callee != nil && e.Callee.Kind == "ident" && e.Callee.Value == "selector" {
			if len(e.Args) != 1 || e.Args[0] == nil || e.Args[0].Kind != "string" {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidSelectorExpr,
					Message: "selector(...) requires exactly one string literal argument",
					Span:    defaultSpan(filename),
				})
			}
		}
		checkExpr(contractName, funcVis, filename, e.Callee, diags)
		for _, a := range e.Args {
			checkExpr(contractName, funcVis, filename, a, diags)
		}
	case "member":
		// Validate selector member builtin: this.fn.selector / Contract.fn.selector
		if e.Member == "selector" {
			ok := false
			msg := ""
			if e.Object != nil && e.Object.Kind == "member" && e.Object.Object != nil && e.Object.Object.Kind == "ident" {
				scope := e.Object.Object.Value
				fnName := e.Object.Member
				if scope != "this" && scope != contractName {
					msg = fmt.Sprintf("selector member scope must be 'this' or '%s'", contractName)
				} else if vis, exists := funcVis[fnName]; !exists {
					msg = fmt.Sprintf("selector target function '%s' not found", fnName)
				} else if vis != "public" && vis != "external" {
					msg = fmt.Sprintf("selector target function '%s' is not externally dispatchable", fnName)
				} else {
					ok = true
				}
			} else {
				msg = "selector member expression must be 'this.fn.selector' or 'Contract.fn.selector'"
			}
			if !ok {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaSelectorTarget,
					Message: msg,
					Span:    defaultSpan(filename),
				})
			}
		}
		checkExpr(contractName, funcVis, filename, e.Object, diags)
	case "index":
		checkExpr(contractName, funcVis, filename, e.Object, diags)
		checkExpr(contractName, funcVis, filename, e.Index, diags)
	case "binary", "assign":
		checkExpr(contractName, funcVis, filename, e.Left, diags)
		checkExpr(contractName, funcVis, filename, e.Right, diags)
	case "unary":
		checkExpr(contractName, funcVis, filename, e.Right, diags)
	case "paren":
		checkExpr(contractName, funcVis, filename, e.Left, diags)
	default:
		// leaf nodes
	}
}

func functionVisibility(modifiers []string) string {
	vis := ""
	for _, m := range modifiers {
		switch m {
		case "public", "external", "internal", "private":
			vis = m
		}
	}
	return vis
}

func selectorDispatchKey(fn ast.FunctionDecl) (string, bool) {
	visibility := ""
	for _, m := range fn.Modifiers {
		switch m {
		case "public", "external", "internal", "private":
			visibility = m
		}
	}
	if visibility != "public" && visibility != "external" {
		return "", false
	}
	if fn.SelectorOverride != "" {
		return strings.ToLower(fn.SelectorOverride), true
	}
	sig := fmt.Sprintf("%s(%s)", fn.Name, selectorTypeList(fn.Params))
	return selectorHexFromSignature(sig), true
}

func selectorTypeList(params []ast.FieldDecl) string {
	if len(params) == 0 {
		return ""
	}
	types := make([]string, 0, len(params))
	for _, p := range params {
		types = append(types, normalizeSelectorType(p.Type))
	}
	return strings.Join(types, ",")
}

func normalizeSelectorType(t string) string {
	s := strings.Join(strings.Fields(t), " ")
	repl := strings.NewReplacer(
		"( ", "(",
		" )", ")",
		"[ ", "[",
		" ]", "]",
		" ,", ",",
		", ", ",",
		" => ", "=>",
		" =>", "=>",
		"=> ", "=>",
	)
	return repl.Replace(s)
}

func isValidSelectorOverride(sel string) bool {
	if len(sel) != 10 || !strings.HasPrefix(sel, "0x") {
		return false
	}
	for i := 2; i < len(sel); i++ {
		ch := sel[i]
		isHex := (ch >= '0' && ch <= '9') ||
			(ch >= 'a' && ch <= 'f') ||
			(ch >= 'A' && ch <= 'F')
		if !isHex {
			return false
		}
	}
	return true
}

func selectorHexFromSignature(sig string) string {
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write([]byte(sig))
	sum := h.Sum(nil)
	return "0x" + hex.EncodeToString(sum[:4])
}

func defaultSpan(filename string) diag.Span {
	return diag.Span{
		File: filename,
		Start: diag.Position{
			Line:   1,
			Column: 1,
		},
		End: diag.Position{
			Line:   1,
			Column: 1,
		},
	}
}
