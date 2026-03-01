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

type storageSlotKind string

const (
	storageKindScalar  storageSlotKind = "scalar"
	storageKindMapping storageSlotKind = "mapping"
	storageKindArray   storageSlotKind = "array"
)

type storageSlotInfo struct {
	name         string
	kind         storageSlotKind
	typeName     string
	mappingDepth int
}

type storageCheckCtx struct {
	slots  map[string]storageSlotInfo
	scopes []map[string]struct{}
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
		funcArity := map[string]int{}
		eventArity := map[string]int{}
		for _, fn := range m.Contract.Functions {
			vis, modDiags := validateFunctionModifiers(filename, fn.Name, fn.Modifiers)
			diags = append(diags, modDiags...)
			funcVis[fn.Name] = vis
			funcArity[fn.Name] = len(fn.Params)
		}
		for _, ev := range m.Contract.Events {
			diags = append(diags, duplicateParamDiagnostics(filename, "event", ev.Name, ev.Params)...)
			if _, exists := eventArity[ev.Name]; exists {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaDuplicateEvent,
					Message: fmt.Sprintf("duplicate event '%s'", ev.Name),
					Span:    defaultSpan(filename),
				})
				continue
			}
			eventArity[ev.Name] = len(ev.Params)
		}
		slotInfos := map[string]storageSlotInfo{}

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
					slotInfos[slot.Name] = buildStorageSlotInfo(slot)
				}
			}
		}
		diags = append(diags, checkContractNameCollisions(filename, slotInfos, funcArity, eventArity)...)

		funcSeen := map[string]struct{}{}
		selectorSeen := map[string]string{}
		for _, fn := range m.Contract.Functions {
			diags = append(diags, duplicateParamDiagnostics(filename, "function", fn.Name, fn.Params)...)
			diags = append(diags, duplicateParamDiagnostics(filename, "returns", fn.Name, fn.Returns)...)
			diags = append(diags, checkParamReturnNameCollisions(filename, fn.Name, fn.Params, fn.Returns)...)
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
			if fn.SelectorOverride != "" {
				vis := funcVis[fn.Name]
				if vis != "public" && vis != "external" {
					diags = append(diags, diag.Diagnostic{
						Code:    diag.CodeSemaSelectorVisibility,
						Message: fmt.Sprintf("@selector is only allowed on public/external functions (got '%s' on '%s')", vis, fn.Name),
						Span:    defaultSpan(filename),
					})
				}
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
			checkStatements(filename, m.Contract.Name, funcVis, funcArity, eventArity, fn.Body, 0, &diags)
			checkReturnStatements(filename, "function", fn.Name, len(fn.Returns) > 0, fn.Body, &diags)
			checkDuplicateLocals(filename, "function", fn.Name, fn.Params, fn.Body, &diags)
			if len(fn.Returns) > 0 && !containsReturnValueStmt(fn.Body) {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidReturn,
					Message: fmt.Sprintf("function '%s' requires at least one return statement with value in current verifier stage", fn.Name),
					Span:    defaultSpan(filename),
				})
			}
			checkStorageFunctionBody(filename, slotInfos, fn.Params, fn.Body, &diags)
		}

		if m.Contract.Constructor != nil {
			diags = append(diags, validateConstructorModifiers(filename, m.Contract.Constructor.Modifiers)...)
			diags = append(diags, duplicateParamDiagnostics(filename, "constructor", "", m.Contract.Constructor.Params)...)
			checkStatements(filename, m.Contract.Name, funcVis, funcArity, eventArity, m.Contract.Constructor.Body, 0, &diags)
			checkReturnStatements(filename, "constructor", "", false, m.Contract.Constructor.Body, &diags)
			checkDuplicateLocals(filename, "constructor", "", m.Contract.Constructor.Params, m.Contract.Constructor.Body, &diags)
			checkStorageFunctionBody(filename, slotInfos, m.Contract.Constructor.Params, m.Contract.Constructor.Body, &diags)
		}
		if m.Contract.Fallback != nil {
			checkStatements(filename, m.Contract.Name, funcVis, funcArity, eventArity, m.Contract.Fallback.Body, 0, &diags)
			checkReturnStatements(filename, "fallback", "", false, m.Contract.Fallback.Body, &diags)
			checkDuplicateLocals(filename, "fallback", "", nil, m.Contract.Fallback.Body, &diags)
			checkStorageFunctionBody(filename, slotInfos, nil, m.Contract.Fallback.Body, &diags)
		}
	}

	if diags.HasErrors() {
		return nil, diags
	}
	return &TypedModule{AST: m}, nil
}

func checkStatements(filename string, contractName string, funcVis map[string]string, funcArity map[string]int, eventArity map[string]int, stmts []ast.Statement, loopDepth int, diags *diag.Diagnostics) {
	for _, s := range stmts {
		checkExpr(contractName, funcVis, funcArity, filename, s.Expr, diags)
		checkExpr(contractName, funcVis, funcArity, filename, s.Target, diags)
		checkExpr(contractName, funcVis, funcArity, filename, s.Cond, diags)
		checkExpr(contractName, funcVis, funcArity, filename, s.Post, diags)
		if s.Init != nil {
			checkStatements(filename, contractName, funcVis, funcArity, eventArity, []ast.Statement{*s.Init}, loopDepth, diags)
		}
		switch s.Kind {
		case "let":
			if containsAssignExpr(s.Expr) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidAssignExpr,
					Message: "assignment expressions are not allowed in let initializer",
					Span:    defaultSpan(filename),
				})
			}
		case "return":
			if containsAssignExpr(s.Expr) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidAssignExpr,
					Message: "assignment expressions are not allowed in return expression",
					Span:    defaultSpan(filename),
				})
			}
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
		case "require", "assert":
			if s.Expr == nil {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidStmtShape,
					Message: s.Kind + " statement requires an expression argument in current stage",
					Span:    defaultSpan(filename),
				})
			}
		case "revert":
			if s.Expr != nil && !isStringLiteralExpr(s.Expr) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidRevert,
					Message: "revert payload must be a string literal in current stage",
					Span:    defaultSpan(filename),
				})
			}
		case "emit":
			if s.Expr == nil || !isCallExpr(s.Expr) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidStmtShape,
					Message: "emit statement requires a call-like payload (e.g. emit EventName(...))",
					Span:    defaultSpan(filename),
				})
			} else {
				name, argc, ok := emitCallInfo(s.Expr)
				if !ok {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaInvalidStmtShape,
						Message: "emit statement payload must call an event identifier (e.g. EventName(...))",
						Span:    defaultSpan(filename),
					})
				} else if want, exists := eventArity[name]; exists {
					if argc != want {
						*diags = append(*diags, diag.Diagnostic{
							Code:    diag.CodeSemaEmitArity,
							Message: fmt.Sprintf("emit event '%s' expects %d argument(s), got %d", name, want, argc),
							Span:    defaultSpan(filename),
						})
					}
				} else if len(eventArity) > 0 {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaUnknownEmitEvent,
						Message: fmt.Sprintf("emit event '%s' is not declared in contract", name),
						Span:    defaultSpan(filename),
					})
				}
			}
		case "set":
			if !isAssignableTarget(s.Target) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidSetTarget,
					Message: "set target must be identifier, member access, or index access",
					Span:    defaultSpan(filename),
				})
			} else if isReadOnlyIdentTarget(s.Target) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidSetTarget,
					Message: "set target cannot be 'true', 'false', or 'nil'",
					Span:    defaultSpan(filename),
				})
			}
			if isSelectorMemberExpr(s.Target) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidSetTarget,
					Message: "selector member expression is read-only and cannot be assignment target",
					Span:    defaultSpan(filename),
				})
			}
			if containsAssignExpr(s.Expr) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidAssignExpr,
					Message: "assignment expressions are not allowed in set value expression",
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
			if containsAssignExpr(s.Cond) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidAssignExpr,
					Message: "assignment expressions are not allowed in if condition",
					Span:    defaultSpan(filename),
				})
			}
			checkStatements(filename, contractName, funcVis, funcArity, eventArity, s.Then, loopDepth, diags)
			checkStatements(filename, contractName, funcVis, funcArity, eventArity, s.Else, loopDepth, diags)
		case "while":
			if s.Cond == nil {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaMissingCondition,
					Message: "while statement requires a condition expression",
					Span:    defaultSpan(filename),
				})
			}
			if containsAssignExpr(s.Cond) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidAssignExpr,
					Message: "assignment expressions are not allowed in while condition",
					Span:    defaultSpan(filename),
				})
			}
			checkStatements(filename, contractName, funcVis, funcArity, eventArity, s.Body, loopDepth+1, diags)
		case "for":
			if s.Cond != nil && containsAssignExpr(s.Cond) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidAssignExpr,
					Message: "assignment expressions are not allowed in for condition",
					Span:    defaultSpan(filename),
				})
			}
			if s.Post != nil && !isExprStatementExpr(s.Post) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidAssignExpr,
					Message: "for post expression must be a function call or assignment expression",
					Span:    defaultSpan(filename),
				})
			}
			if isSelectorBuiltinCallExpr(s.Post) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidStmtShape,
					Message: "selector(...) cannot be used as for post expression statement",
					Span:    defaultSpan(filename),
				})
			}
			if hasIllegalNestedAssignInStmtExpr(s.Post) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidAssignExpr,
					Message: "nested assignment expressions are not allowed in for post expression",
					Span:    defaultSpan(filename),
				})
			}
			checkStatements(filename, contractName, funcVis, funcArity, eventArity, s.Body, loopDepth+1, diags)
		case "expr":
			if s.Expr == nil || !isExprStatementExpr(s.Expr) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidAssignExpr,
					Message: "expression statement must be a function call or assignment expression",
					Span:    defaultSpan(filename),
				})
			}
			if hasIllegalNestedAssignInStmtExpr(s.Expr) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidAssignExpr,
					Message: "nested assignment expressions are not allowed in expression statement",
					Span:    defaultSpan(filename),
				})
			}
			if isSelectorBuiltinCallExpr(s.Expr) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidStmtShape,
					Message: "selector(...) cannot be used as standalone expression statement",
					Span:    defaultSpan(filename),
				})
			}
		default:
			checkStatements(filename, contractName, funcVis, funcArity, eventArity, s.Then, loopDepth, diags)
			checkStatements(filename, contractName, funcVis, funcArity, eventArity, s.Else, loopDepth, diags)
			checkStatements(filename, contractName, funcVis, funcArity, eventArity, s.Body, loopDepth, diags)
		}
	}
}

func isExprStatementExpr(e *ast.Expr) bool {
	if e == nil {
		return false
	}
	if e.Kind == "call" || e.Kind == "assign" {
		return true
	}
	if e.Kind == "paren" {
		return isExprStatementExpr(e.Left)
	}
	return false
}

func isCallExpr(e *ast.Expr) bool {
	root := stripParens(e)
	return root != nil && root.Kind == "call"
}

func isSelectorBuiltinCallExpr(e *ast.Expr) bool {
	root := stripParens(e)
	if root == nil || root.Kind != "call" {
		return false
	}
	callee := stripParens(root.Callee)
	return callee != nil && callee.Kind == "ident" && strings.TrimSpace(callee.Value) == "selector"
}

func isSelectorMemberExpr(e *ast.Expr) bool {
	root := stripParens(e)
	return root != nil && root.Kind == "member" && root.Member == "selector"
}

func isReadOnlyIdentTarget(e *ast.Expr) bool {
	root := stripParens(e)
	if root == nil || root.Kind != "ident" {
		return false
	}
	switch strings.TrimSpace(root.Value) {
	case "true", "false", "nil":
		return true
	default:
		return false
	}
}

func emitCallInfo(e *ast.Expr) (string, int, bool) {
	root := stripParens(e)
	if root == nil || root.Kind != "call" {
		return "", 0, false
	}
	callee := stripParens(root.Callee)
	if callee == nil || callee.Kind != "ident" {
		return "", 0, false
	}
	name := strings.TrimSpace(callee.Value)
	if name == "" {
		return "", 0, false
	}
	return name, len(root.Args), true
}

func isStringLiteralExpr(e *ast.Expr) bool {
	root := stripParens(e)
	return root != nil && root.Kind == "string"
}

func stripParens(e *ast.Expr) *ast.Expr {
	cur := e
	for cur != nil && cur.Kind == "paren" {
		cur = cur.Left
	}
	return cur
}

func hasIllegalNestedAssignInStmtExpr(e *ast.Expr) bool {
	root := stripParens(e)
	if root == nil {
		return false
	}
	switch root.Kind {
	case "assign":
		return containsAssignExpr(root.Left) || containsAssignExpr(root.Right)
	case "call":
		if containsAssignExpr(root.Callee) {
			return true
		}
		for _, a := range root.Args {
			if containsAssignExpr(a) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func containsAssignExpr(e *ast.Expr) bool {
	if e == nil {
		return false
	}
	if e.Kind == "assign" {
		return true
	}
	switch e.Kind {
	case "paren":
		return containsAssignExpr(e.Left)
	case "call":
		if containsAssignExpr(e.Callee) {
			return true
		}
		for _, a := range e.Args {
			if containsAssignExpr(a) {
				return true
			}
		}
		return false
	case "member":
		return containsAssignExpr(e.Object)
	case "index":
		return containsAssignExpr(e.Object) || containsAssignExpr(e.Index)
	case "binary":
		return containsAssignExpr(e.Left) || containsAssignExpr(e.Right)
	case "unary":
		return containsAssignExpr(e.Right)
	default:
		return false
	}
}

func checkReturnStatements(filename, ownerKind, ownerName string, expectsValue bool, stmts []ast.Statement, diags *diag.Diagnostics) {
	for _, s := range stmts {
		if s.Kind == "return" {
			switch {
			case expectsValue && s.Expr == nil:
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidReturn,
					Message: fmt.Sprintf("%s requires a return value", ownerLabel(ownerKind, ownerName)),
					Span:    defaultSpan(filename),
				})
			case !expectsValue && s.Expr != nil:
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidReturn,
					Message: fmt.Sprintf("%s must not return a value", ownerLabel(ownerKind, ownerName)),
					Span:    defaultSpan(filename),
				})
			}
		}
		if s.Init != nil {
			checkReturnStatements(filename, ownerKind, ownerName, expectsValue, []ast.Statement{*s.Init}, diags)
		}
		checkReturnStatements(filename, ownerKind, ownerName, expectsValue, s.Then, diags)
		checkReturnStatements(filename, ownerKind, ownerName, expectsValue, s.Else, diags)
		checkReturnStatements(filename, ownerKind, ownerName, expectsValue, s.Body, diags)
	}
}

func containsReturnValueStmt(stmts []ast.Statement) bool {
	for _, s := range stmts {
		if s.Kind == "return" && s.Expr != nil {
			return true
		}
		if s.Init != nil && containsReturnValueStmt([]ast.Statement{*s.Init}) {
			return true
		}
		if containsReturnValueStmt(s.Then) || containsReturnValueStmt(s.Else) || containsReturnValueStmt(s.Body) {
			return true
		}
	}
	return false
}

type localScope struct {
	names map[string]struct{}
}

func checkDuplicateLocals(filename, ownerKind, ownerName string, params []ast.FieldDecl, body []ast.Statement, diags *diag.Diagnostics) {
	scopes := []localScope{{names: map[string]struct{}{}}}
	declare := func(name string) bool {
		n := strings.TrimSpace(name)
		if n == "" {
			return true
		}
		cur := &scopes[len(scopes)-1]
		if _, exists := cur.names[n]; exists {
			return false
		}
		cur.names[n] = struct{}{}
		return true
	}
	for _, p := range params {
		_ = declare(p.Name)
	}
	checkDuplicateLocalsInStmts(filename, ownerKind, ownerName, body, &scopes, declare, diags)
}

func checkDuplicateLocalsInStmts(
	filename, ownerKind, ownerName string,
	stmts []ast.Statement,
	scopes *[]localScope,
	declare func(string) bool,
	diags *diag.Diagnostics,
) {
	push := func() {
		*scopes = append(*scopes, localScope{names: map[string]struct{}{}})
	}
	pop := func() {
		if len(*scopes) > 1 {
			*scopes = (*scopes)[:len(*scopes)-1]
		}
	}
	for _, s := range stmts {
		if s.Kind == "let" && !declare(s.Name) {
			subject := ownerKind
			if ownerKind == "function" {
				subject = fmt.Sprintf("function '%s'", ownerName)
			}
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaDuplicateLocal,
				Message: fmt.Sprintf("duplicate local variable '%s' in %s scope", strings.TrimSpace(s.Name), subject),
				Span:    defaultSpan(filename),
			})
		}
		switch s.Kind {
		case "if":
			push()
			checkDuplicateLocalsInStmts(filename, ownerKind, ownerName, s.Then, scopes, declare, diags)
			pop()
			push()
			checkDuplicateLocalsInStmts(filename, ownerKind, ownerName, s.Else, scopes, declare, diags)
			pop()
		case "while":
			push()
			checkDuplicateLocalsInStmts(filename, ownerKind, ownerName, s.Body, scopes, declare, diags)
			pop()
		case "for":
			push()
			if s.Init != nil {
				checkDuplicateLocalsInStmts(filename, ownerKind, ownerName, []ast.Statement{*s.Init}, scopes, declare, diags)
			}
			checkDuplicateLocalsInStmts(filename, ownerKind, ownerName, s.Body, scopes, declare, diags)
			pop()
		default:
			if len(s.Then) > 0 {
				push()
				checkDuplicateLocalsInStmts(filename, ownerKind, ownerName, s.Then, scopes, declare, diags)
				pop()
			}
			if len(s.Else) > 0 {
				push()
				checkDuplicateLocalsInStmts(filename, ownerKind, ownerName, s.Else, scopes, declare, diags)
				pop()
			}
			if len(s.Body) > 0 {
				push()
				checkDuplicateLocalsInStmts(filename, ownerKind, ownerName, s.Body, scopes, declare, diags)
				pop()
			}
		}
	}
}

func ownerLabel(ownerKind, ownerName string) string {
	if ownerKind == "function" && strings.TrimSpace(ownerName) != "" {
		return fmt.Sprintf("function '%s'", ownerName)
	}
	return ownerKind
}

func buildStorageSlotInfo(slot ast.StorageSlot) storageSlotInfo {
	typeName := strings.TrimSpace(slot.Type)
	kind := classifyStorageKind(typeName)
	return storageSlotInfo{
		name:         slot.Name,
		kind:         kind,
		typeName:     typeName,
		mappingDepth: mappingTypeDepth(typeName),
	}
}

func classifyStorageKind(typeName string) storageSlotKind {
	compact := strings.ReplaceAll(normalizeSelectorType(typeName), " ", "")
	switch {
	case strings.HasPrefix(compact, "mapping("):
		return storageKindMapping
	case strings.HasSuffix(compact, "]"):
		return storageKindArray
	default:
		return storageKindScalar
	}
}

func mappingTypeDepth(typeName string) int {
	compact := strings.ReplaceAll(normalizeSelectorType(typeName), " ", "")
	return strings.Count(compact, "mapping(")
}

func newStorageCheckCtx(slots map[string]storageSlotInfo, params []ast.FieldDecl) *storageCheckCtx {
	c := &storageCheckCtx{
		slots:  slots,
		scopes: []map[string]struct{}{},
	}
	c.pushScope()
	for _, p := range params {
		c.declareLocal(p.Name)
	}
	return c
}

func (c *storageCheckCtx) pushScope() {
	c.scopes = append(c.scopes, map[string]struct{}{})
}

func (c *storageCheckCtx) popScope() {
	if len(c.scopes) == 0 {
		return
	}
	c.scopes = c.scopes[:len(c.scopes)-1]
}

func (c *storageCheckCtx) declareLocal(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	if len(c.scopes) == 0 {
		c.pushScope()
	}
	c.scopes[len(c.scopes)-1][name] = struct{}{}
}

func (c *storageCheckCtx) isLocal(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for i := len(c.scopes) - 1; i >= 0; i-- {
		if _, ok := c.scopes[i][name]; ok {
			return true
		}
	}
	return false
}

func (c *storageCheckCtx) storagePathFromExpr(e *ast.Expr) (string, []*ast.Expr, bool) {
	if c == nil || e == nil {
		return "", nil, false
	}
	switch e.Kind {
	case "paren":
		return c.storagePathFromExpr(e.Left)
	case "ident":
		name := strings.TrimSpace(e.Value)
		if name == "" || c.isLocal(name) {
			return "", nil, false
		}
		if _, ok := c.slots[name]; !ok {
			return "", nil, false
		}
		return name, []*ast.Expr{}, true
	case "index":
		slot, keys, ok := c.storagePathFromExpr(e.Object)
		if !ok {
			return "", nil, false
		}
		out := make([]*ast.Expr, 0, len(keys)+1)
		out = append(out, keys...)
		out = append(out, e.Index)
		return slot, out, true
	default:
		return "", nil, false
	}
}

type storageExprUse int

const (
	storageUseValue storageExprUse = iota
	storageUseIndexObject
	storageUseMemberObject
	storageUseCallCallee
)

func checkStorageFunctionBody(filename string, slots map[string]storageSlotInfo, params []ast.FieldDecl, body []ast.Statement, diags *diag.Diagnostics) {
	if len(slots) == 0 {
		return
	}
	ctx := newStorageCheckCtx(slots, params)
	checkStorageStatements(filename, ctx, body, diags)
}

func checkStorageStatements(filename string, ctx *storageCheckCtx, stmts []ast.Statement, diags *diag.Diagnostics) {
	for _, s := range stmts {
		switch s.Kind {
		case "let":
			checkStorageExpr(filename, ctx, s.Expr, storageUseValue, diags)
			ctx.declareLocal(s.Name)
		case "set":
			checkStorageSetTarget(filename, ctx, s.Target, diags)
			checkStorageExpr(filename, ctx, s.Expr, storageUseValue, diags)
		case "if":
			checkStorageExpr(filename, ctx, s.Cond, storageUseValue, diags)
			ctx.pushScope()
			checkStorageStatements(filename, ctx, s.Then, diags)
			ctx.popScope()
			ctx.pushScope()
			checkStorageStatements(filename, ctx, s.Else, diags)
			ctx.popScope()
		case "while":
			checkStorageExpr(filename, ctx, s.Cond, storageUseValue, diags)
			ctx.pushScope()
			checkStorageStatements(filename, ctx, s.Body, diags)
			ctx.popScope()
		case "for":
			ctx.pushScope()
			if s.Init != nil {
				checkStorageStatements(filename, ctx, []ast.Statement{*s.Init}, diags)
			}
			checkStorageExpr(filename, ctx, s.Cond, storageUseValue, diags)
			checkStorageExpr(filename, ctx, s.Post, storageUseValue, diags)
			checkStorageStatements(filename, ctx, s.Body, diags)
			ctx.popScope()
		default:
			checkStorageExpr(filename, ctx, s.Expr, storageUseValue, diags)
			checkStorageExpr(filename, ctx, s.Target, storageUseValue, diags)
			checkStorageExpr(filename, ctx, s.Cond, storageUseValue, diags)
			checkStorageExpr(filename, ctx, s.Post, storageUseValue, diags)
			if s.Init != nil {
				checkStorageStatements(filename, ctx, []ast.Statement{*s.Init}, diags)
			}
			if len(s.Then) > 0 {
				ctx.pushScope()
				checkStorageStatements(filename, ctx, s.Then, diags)
				ctx.popScope()
			}
			if len(s.Else) > 0 {
				ctx.pushScope()
				checkStorageStatements(filename, ctx, s.Else, diags)
				ctx.popScope()
			}
			if len(s.Body) > 0 {
				ctx.pushScope()
				checkStorageStatements(filename, ctx, s.Body, diags)
				ctx.popScope()
			}
		}
	}
}

func checkStorageSetTarget(filename string, ctx *storageCheckCtx, target *ast.Expr, diags *diag.Diagnostics) {
	if target == nil {
		return
	}
	if slotName, ok := storageArrayLengthMemberTarget(ctx, target); ok {
		reportStorageAccess(filename, fmt.Sprintf("storage array length on slot '%s' is read-only in current stage", slotName), diags)
		return
	}
	if slotName, keys, ok := ctx.storagePathFromExpr(target); ok {
		info := ctx.slots[slotName]
		checkStorageKeys(filename, ctx, keys, diags)
		validateStorageWrite(filename, info, keys, diags)
		return
	}
	checkStorageExpr(filename, ctx, target, storageUseValue, diags)
}

func checkStorageExpr(filename string, ctx *storageCheckCtx, e *ast.Expr, use storageExprUse, diags *diag.Diagnostics) {
	if e == nil {
		return
	}
	if slotName, keys, ok := ctx.storagePathFromExpr(e); ok {
		info := ctx.slots[slotName]
		switch use {
		case storageUseValue:
			validateStorageRead(filename, info, keys, diags)
		case storageUseIndexObject:
			validateStorageIndexObject(filename, info, keys, diags)
		case storageUseCallCallee:
			reportStorageAccess(filename, fmt.Sprintf("storage slot '%s' is not callable", info.name), diags)
		}
	}

	switch e.Kind {
	case "call":
		if e.Callee != nil && e.Callee.Kind == "member" && e.Callee.Member == "push" {
			if slotName, keys, ok := ctx.storagePathFromExpr(e.Callee.Object); ok {
				info := ctx.slots[slotName]
				checkStorageKeys(filename, ctx, keys, diags)
				validateStoragePush(filename, info, keys, len(e.Args), diags)
				for _, a := range e.Args {
					checkStorageExpr(filename, ctx, a, storageUseValue, diags)
				}
				return
			}
		}
		checkStorageExpr(filename, ctx, e.Callee, storageUseCallCallee, diags)
		for _, a := range e.Args {
			checkStorageExpr(filename, ctx, a, storageUseValue, diags)
		}
	case "member":
		if e.Member == "length" {
			if slotName, keys, ok := ctx.storagePathFromExpr(e.Object); ok {
				info := ctx.slots[slotName]
				checkStorageKeys(filename, ctx, keys, diags)
				validateStorageLength(filename, info, keys, diags)
				return
			}
		}
		if slotName, _, ok := ctx.storagePathFromExpr(e.Object); ok && e.Member != "selector" {
			info := ctx.slots[slotName]
			reportStorageAccess(filename, fmt.Sprintf("unsupported member access '.%s' on storage slot '%s'", e.Member, info.name), diags)
		}
		checkStorageExpr(filename, ctx, e.Object, storageUseMemberObject, diags)
	case "index":
		checkStorageExpr(filename, ctx, e.Object, storageUseIndexObject, diags)
		checkStorageExpr(filename, ctx, e.Index, storageUseValue, diags)
	case "binary", "assign":
		checkStorageExpr(filename, ctx, e.Left, storageUseValue, diags)
		checkStorageExpr(filename, ctx, e.Right, storageUseValue, diags)
	case "unary":
		checkStorageExpr(filename, ctx, e.Right, storageUseValue, diags)
	case "paren":
		checkStorageExpr(filename, ctx, e.Left, use, diags)
	default:
		// leaf nodes
	}
}

func storageArrayLengthMemberTarget(ctx *storageCheckCtx, e *ast.Expr) (string, bool) {
	root := stripParens(e)
	if root == nil || root.Kind != "member" || root.Member != "length" {
		return "", false
	}
	slotName, keys, ok := ctx.storagePathFromExpr(root.Object)
	if !ok || len(keys) != 0 {
		return "", false
	}
	info := ctx.slots[slotName]
	if info.kind != storageKindArray {
		return "", false
	}
	return slotName, true
}

func checkStorageKeys(filename string, ctx *storageCheckCtx, keys []*ast.Expr, diags *diag.Diagnostics) {
	for _, k := range keys {
		checkStorageExpr(filename, ctx, k, storageUseValue, diags)
	}
}

func validateStorageRead(filename string, info storageSlotInfo, keys []*ast.Expr, diags *diag.Diagnostics) {
	switch info.kind {
	case storageKindScalar:
		if len(keys) > 0 {
			reportStorageAccess(filename, fmt.Sprintf("storage slot '%s' of type '%s' does not support indexed read", info.name, info.typeName), diags)
		}
	case storageKindMapping:
		want := info.mappingDepth
		if want <= 0 {
			want = 1
		}
		if len(keys) != want {
			reportStorageAccess(filename, fmt.Sprintf("storage mapping slot '%s' requires exactly %d index key(s), got %d", info.name, want, len(keys)), diags)
		}
	case storageKindArray:
		switch len(keys) {
		case 0:
			reportStorageAccess(filename, fmt.Sprintf("direct storage array value read is not supported on slot '%s'; use index or .length", info.name), diags)
		case 1:
			// ok
		default:
			reportStorageAccess(filename, fmt.Sprintf("nested storage array indexing is not supported on slot '%s'", info.name), diags)
		}
	}
}

func validateStorageWrite(filename string, info storageSlotInfo, keys []*ast.Expr, diags *diag.Diagnostics) {
	switch info.kind {
	case storageKindScalar:
		if len(keys) > 0 {
			reportStorageAccess(filename, fmt.Sprintf("storage slot '%s' of type '%s' does not support indexed write", info.name, info.typeName), diags)
		}
	case storageKindMapping:
		want := info.mappingDepth
		if want <= 0 {
			want = 1
		}
		if len(keys) != want {
			reportStorageAccess(filename, fmt.Sprintf("storage mapping slot '%s' requires exactly %d index key(s), got %d", info.name, want, len(keys)), diags)
		}
	case storageKindArray:
		if len(keys) != 1 {
			reportStorageAccess(filename, fmt.Sprintf("storage array slot '%s' write requires exactly one index in current stage", info.name), diags)
		}
	}
}

func validateStorageIndexObject(filename string, info storageSlotInfo, keys []*ast.Expr, diags *diag.Diagnostics) {
	switch info.kind {
	case storageKindScalar:
		reportStorageAccess(filename, fmt.Sprintf("storage slot '%s' of type '%s' is not indexable", info.name, info.typeName), diags)
	case storageKindMapping:
		want := info.mappingDepth
		if want <= 0 {
			want = 1
		}
		if len(keys) >= want {
			reportStorageAccess(filename, fmt.Sprintf("mapping value of slot '%s' is not indexable beyond declared depth %d", info.name, want), diags)
		}
	case storageKindArray:
		if len(keys) != 0 {
			reportStorageAccess(filename, fmt.Sprintf("nested storage array indexing is not supported on slot '%s'", info.name), diags)
		}
	}
}

func validateStorageLength(filename string, info storageSlotInfo, keys []*ast.Expr, diags *diag.Diagnostics) {
	if info.kind != storageKindArray || len(keys) != 0 {
		reportStorageAccess(filename, fmt.Sprintf("'.length' is supported only for top-level storage arrays (slot '%s')", info.name), diags)
	}
}

func validateStoragePush(filename string, info storageSlotInfo, keys []*ast.Expr, argCount int, diags *diag.Diagnostics) {
	if info.kind != storageKindArray || len(keys) != 0 {
		reportStorageAccess(filename, fmt.Sprintf("'.push(v)' is supported only for top-level storage arrays (slot '%s')", info.name), diags)
		return
	}
	if argCount != 1 {
		reportStorageAccess(filename, "storage array push requires exactly one argument", diags)
	}
}

func reportStorageAccess(filename, msg string, diags *diag.Diagnostics) {
	*diags = append(*diags, diag.Diagnostic{
		Code:    diag.CodeSemaStorageAccess,
		Message: msg,
		Span:    defaultSpan(filename),
	})
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

func checkExpr(contractName string, funcVis map[string]string, funcArity map[string]int, filename string, e *ast.Expr, diags *diag.Diagnostics) {
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
		if e.Callee != nil && e.Callee.Kind == "ident" {
			if want, ok := funcArity[e.Callee.Value]; ok && len(e.Args) != want {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaCallArity,
					Message: fmt.Sprintf("function '%s' expects %d argument(s), got %d", e.Callee.Value, want, len(e.Args)),
					Span:    defaultSpan(filename),
				})
			}
		}
		checkExpr(contractName, funcVis, funcArity, filename, e.Callee, diags)
		for _, a := range e.Args {
			checkExpr(contractName, funcVis, funcArity, filename, a, diags)
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
		checkExpr(contractName, funcVis, funcArity, filename, e.Object, diags)
	case "index":
		checkExpr(contractName, funcVis, funcArity, filename, e.Object, diags)
		checkExpr(contractName, funcVis, funcArity, filename, e.Index, diags)
	case "binary":
		checkExpr(contractName, funcVis, funcArity, filename, e.Left, diags)
		checkExpr(contractName, funcVis, funcArity, filename, e.Right, diags)
	case "assign":
		if !isAssignableTarget(e.Left) {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaInvalidSetTarget,
				Message: "assignment target must be identifier, member access, or index access",
				Span:    defaultSpan(filename),
			})
		} else if isReadOnlyIdentTarget(e.Left) {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaInvalidSetTarget,
				Message: "assignment target cannot be 'true', 'false', or 'nil'",
				Span:    defaultSpan(filename),
			})
		}
		checkExpr(contractName, funcVis, funcArity, filename, e.Left, diags)
		checkExpr(contractName, funcVis, funcArity, filename, e.Right, diags)
	case "unary":
		checkExpr(contractName, funcVis, funcArity, filename, e.Right, diags)
	case "paren":
		checkExpr(contractName, funcVis, funcArity, filename, e.Left, diags)
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

func checkContractNameCollisions(filename string, slots map[string]storageSlotInfo, funcs map[string]int, events map[string]int) diag.Diagnostics {
	var out diag.Diagnostics
	for name := range events {
		if _, exists := funcs[name]; exists {
			out = append(out, diag.Diagnostic{
				Code:    diag.CodeSemaNameCollision,
				Message: fmt.Sprintf("name collision: event '%s' conflicts with function '%s'", name, name),
				Span:    defaultSpan(filename),
			})
		}
		if _, exists := slots[name]; exists {
			out = append(out, diag.Diagnostic{
				Code:    diag.CodeSemaNameCollision,
				Message: fmt.Sprintf("name collision: event '%s' conflicts with storage slot '%s'", name, name),
				Span:    defaultSpan(filename),
			})
		}
	}
	for name := range funcs {
		if _, exists := slots[name]; exists {
			out = append(out, diag.Diagnostic{
				Code:    diag.CodeSemaNameCollision,
				Message: fmt.Sprintf("name collision: function '%s' conflicts with storage slot '%s'", name, name),
				Span:    defaultSpan(filename),
			})
		}
	}
	return out
}

func validateFunctionModifiers(filename string, fnName string, modifiers []string) (string, diag.Diagnostics) {
	var diags diag.Diagnostics
	vis := ""
	hasView := false
	hasPure := false
	hasPayable := false

	for _, m := range modifiers {
		switch m {
		case "public", "external", "internal", "private":
			if vis != "" && vis != m {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaConflictingModifier,
					Message: fmt.Sprintf("conflicting visibility modifiers '%s' and '%s' on function '%s'", vis, m, fnName),
					Span:    defaultSpan(filename),
				})
			}
			vis = m
		case "view":
			if hasPayable {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaConflictingModifier,
					Message: fmt.Sprintf("conflicting modifiers 'view' and 'payable' on function '%s'", fnName),
					Span:    defaultSpan(filename),
				})
			}
			if hasPure {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaConflictingModifier,
					Message: fmt.Sprintf("conflicting modifiers 'view' and 'pure' on function '%s'", fnName),
					Span:    defaultSpan(filename),
				})
			}
			hasView = true
		case "pure":
			if hasPayable {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaConflictingModifier,
					Message: fmt.Sprintf("conflicting modifiers 'pure' and 'payable' on function '%s'", fnName),
					Span:    defaultSpan(filename),
				})
			}
			if hasView {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaConflictingModifier,
					Message: fmt.Sprintf("conflicting modifiers 'pure' and 'view' on function '%s'", fnName),
					Span:    defaultSpan(filename),
				})
			}
			hasPure = true
		case "payable":
			if hasView {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaConflictingModifier,
					Message: fmt.Sprintf("conflicting modifiers 'payable' and 'view' on function '%s'", fnName),
					Span:    defaultSpan(filename),
				})
			}
			if hasPure {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaConflictingModifier,
					Message: fmt.Sprintf("conflicting modifiers 'payable' and 'pure' on function '%s'", fnName),
					Span:    defaultSpan(filename),
				})
			}
			hasPayable = true
		default:
			diags = append(diags, diag.Diagnostic{
				Code:    diag.CodeSemaInvalidFnModifier,
				Message: fmt.Sprintf("unsupported function modifier '%s' on function '%s'", m, fnName),
				Span:    defaultSpan(filename),
			})
		}
	}

	return vis, diags
}

func validateConstructorModifiers(filename string, modifiers []string) diag.Diagnostics {
	var diags diag.Diagnostics
	vis := ""
	hasPayable := false
	for _, m := range modifiers {
		switch m {
		case "public", "internal":
			if vis != "" && vis != m {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaConflictingModifier,
					Message: fmt.Sprintf("conflicting constructor visibility modifiers '%s' and '%s'", vis, m),
					Span:    defaultSpan(filename),
				})
			}
			vis = m
		case "payable":
			if hasPayable {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaConflictingModifier,
					Message: "duplicate constructor modifier 'payable'",
					Span:    defaultSpan(filename),
				})
			}
			hasPayable = true
		default:
			diags = append(diags, diag.Diagnostic{
				Code:    diag.CodeSemaInvalidFnModifier,
				Message: fmt.Sprintf("unsupported constructor modifier '%s'", m),
				Span:    defaultSpan(filename),
			})
		}
	}
	return diags
}

func duplicateParamDiagnostics(filename, ownerKind, ownerName string, params []ast.FieldDecl) diag.Diagnostics {
	var out diag.Diagnostics
	seen := map[string]struct{}{}
	for _, p := range params {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			subject := ownerKind
			switch ownerKind {
			case "function":
				subject = fmt.Sprintf("function '%s'", ownerName)
			case "event":
				subject = fmt.Sprintf("event '%s'", ownerName)
			case "returns":
				subject = fmt.Sprintf("return list of function '%s'", ownerName)
			}
			out = append(out, diag.Diagnostic{
				Code:    diag.CodeSemaDuplicateParam,
				Message: fmt.Sprintf("duplicate parameter '%s' in %s", name, subject),
				Span:    defaultSpan(filename),
			})
			continue
		}
		seen[name] = struct{}{}
	}
	return out
}

func checkParamReturnNameCollisions(filename, fnName string, params, returns []ast.FieldDecl) diag.Diagnostics {
	var out diag.Diagnostics
	if len(params) == 0 || len(returns) == 0 {
		return out
	}
	paramNames := map[string]struct{}{}
	for _, p := range params {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			continue
		}
		paramNames[name] = struct{}{}
	}
	for _, r := range returns {
		name := strings.TrimSpace(r.Name)
		if name == "" {
			continue
		}
		if _, ok := paramNames[name]; ok {
			out = append(out, diag.Diagnostic{
				Code:    diag.CodeSemaParamReturnCollision,
				Message: fmt.Sprintf("function '%s' has name collision between parameter and return field '%s'", fnName, name),
				Span:    defaultSpan(filename),
			})
		}
	}
	return out
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
