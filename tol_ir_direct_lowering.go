package lua

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	luast "github.com/tos-network/tolang/ast"
	tolast "github.com/tos-network/tolang/tol/ast"
	"github.com/tos-network/tolang/tol/diag"
	"github.com/tos-network/tolang/tol/lower"
	"golang.org/x/crypto/sha3"
)

// buildDirectIRFromLowered converts lowered TOL program into VM IR.
// Current direct-IR bootstrap supports:
// 1) empty contracts
// 2) function/fallback/constructor wrappers with a restricted statement/expression subset
func buildDirectIRFromLowered(p *lower.Program, sourceName string) (*IRProgram, error) {
	if p == nil {
		return nil, fmt.Errorf("[%s] nil lowered program", diag.CodeLowerNotImplemented)
	}
	if len(p.StorageSlots) > 0 {
		return nil, fmt.Errorf("[%s] lowered features not yet supported by direct IR lowering: storage=%d functions=%d constructor=%v fallback=%v",
			diag.CodeLowerUnsupportedFeature,
			len(p.StorageSlots),
			len(p.Functions),
			p.HasConstructor,
			p.HasFallback,
		)
	}

	if sourceName == "" {
		sourceName = p.ContractName
	}

	if p.HasConstructor || p.HasFallback || len(p.Functions) > 0 {
		chunk, err := buildBootstrapChunkFromLowered(p)
		if err != nil {
			return nil, err
		}
		return BuildIR(chunk, sourceName)
	}

	// Empty contract: emit a trivial return-only program.
	return BuildIR([]luast.Stmt{}, sourceName)
}

func buildBootstrapChunkFromLowered(p *lower.Program) ([]luast.Stmt, error) {
	if p == nil {
		return nil, fmt.Errorf("[%s] nil lowered program", diag.CodeLowerNotImplemented)
	}
	if !p.HasConstructor && !p.HasFallback && len(p.Functions) == 0 {
		return []luast.Stmt{}, nil
	}
	if len(p.StorageSlots) != 0 {
		return nil, fmt.Errorf("[%s] direct IR bootstrap requires storage to be empty in current stage", diag.CodeLowerUnsupportedFeature)
	}

	dispatchFns, err := collectDispatchFuncs(p.Functions)
	if err != nil {
		return nil, err
	}
	env := buildLoweringEnv(p.ContractName, dispatchFns)

	chunk := make([]luast.Stmt, 0, len(p.Functions)+6)
	for _, fn := range p.Functions {
		st, err := lowerFunctionToLua(fn, env)
		if err != nil {
			return nil, err
		}
		chunk = append(chunk, st)
	}
	if p.HasConstructor {
		st, err := lowerConstructorToLua(p.ConstructorParams, p.ConstructorBody, env)
		if err != nil {
			return nil, err
		}
		chunk = append(chunk, st)
	}
	if p.HasFallback {
		st, err := lowerFallbackToLua(p.FallbackBody, env)
		if err != nil {
			return nil, err
		}
		chunk = append(chunk, st)
	}

	if p.HasConstructor || p.HasFallback || len(dispatchFns) > 0 {
		chunk = append(chunk, buildTosInitStmt())
	}
	if p.HasConstructor {
		chunk = append(chunk, buildOnCreateAssignStmt())
	}
	if p.HasFallback || len(dispatchFns) > 0 {
		chunk = append(chunk, buildOnInvokeAssignStmt(dispatchFns, p.HasFallback))
	}
	return chunk, nil
}

type dispatchFunc struct {
	Name      string
	Signature string
}

type loweringEnv struct {
	contractName       string
	selectorByFunction map[string]string
}

func buildLoweringEnv(contractName string, dispatchFns []dispatchFunc) *loweringEnv {
	m := make(map[string]string, len(dispatchFns))
	for _, df := range dispatchFns {
		m[df.Name] = df.Signature
	}
	return &loweringEnv{
		contractName:       contractName,
		selectorByFunction: m,
	}
}

func collectDispatchFuncs(funcs []lower.Function) ([]dispatchFunc, error) {
	out := make([]dispatchFunc, 0, len(funcs))
	for _, fn := range funcs {
		visibility, err := classifyDirectIRFnModifiers(fn.Modifiers)
		if err != nil {
			return nil, err
		}
		if visibility != "public" && visibility != "external" {
			continue
		}
		sig, err := dispatchSelectorForFunction(fn)
		if err != nil {
			return nil, err
		}
		out = append(out, dispatchFunc{
			Name:      fn.Name,
			Signature: sig,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Signature == out[j].Signature {
			return out[i].Name < out[j].Name
		}
		return out[i].Signature < out[j].Signature
	})
	for i := 1; i < len(out); i++ {
		if out[i-1].Signature == out[i].Signature {
			return nil, fmt.Errorf("[%s] duplicate dispatch selector signature '%s'", diag.CodeLowerUnsupportedFeature, out[i].Signature)
		}
	}
	return out, nil
}

func dispatchSelectorForFunction(fn lower.Function) (string, error) {
	if strings.TrimSpace(fn.SelectorOverride) != "" {
		return strings.ToLower(strings.TrimSpace(fn.SelectorOverride)), nil
	}
	sig, err := selectorSignatureForFunction(fn)
	if err != nil {
		return "", err
	}
	return selectorHexFromSignature(sig), nil
}

func selectorHexFromSignature(sig string) string {
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write([]byte(sig))
	sum := h.Sum(nil)
	return "0x" + hex.EncodeToString(sum[:4])
}

func buildTosInitStmt() luast.Stmt {
	return withLineStmt(&luast.AssignStmt{
		Lhs: []luast.Expr{
			withLineExpr(&luast.IdentExpr{Value: "tos"}),
		},
		Rhs: []luast.Expr{
			withLineExpr(&luast.LogicalOpExpr{
				Operator: "or",
				Lhs:      withLineExpr(&luast.IdentExpr{Value: "tos"}),
				Rhs:      withLineExpr(&luast.TableExpr{Fields: []*luast.Field{}}),
			}),
		},
	})
}

func buildOnCreateAssignStmt() luast.Stmt {
	call := withLineExpr(&luast.FuncCallExpr{
		Func: withLineExpr(&luast.IdentExpr{Value: "__tol_constructor"}),
		Args: []luast.Expr{
			withLineExpr(&luast.Comma3Expr{AdjustRet: false}),
		},
		// Keep return arity unchanged when constructor returns values.
		AdjustRet: false,
	})
	fn := withLineExpr(&luast.FunctionExpr{
		ParList: &luast.ParList{
			HasVargs: true,
			Names:    []string{},
		},
		Stmts: []luast.Stmt{
			withLineStmt(&luast.ReturnStmt{Exprs: []luast.Expr{call}}),
		},
	})
	return withLineStmt(&luast.AssignStmt{
		Lhs: []luast.Expr{
			withLineExpr(&luast.AttrGetExpr{
				Object: withLineExpr(&luast.IdentExpr{Value: "tos"}),
				Key:    withLineExpr(&luast.StringExpr{Value: "oncreate"}),
			}),
		},
		Rhs: []luast.Expr{fn},
	})
}

func buildOnInvokeAssignStmt(dispatchFns []dispatchFunc, hasFallback bool) luast.Stmt {
	body := make([]luast.Stmt, 0, len(dispatchFns)+2)
	for _, fn := range dispatchFns {
		cond := withLineExpr(&luast.RelationalOpExpr{
			Operator: "==",
			Lhs:      withLineExpr(&luast.IdentExpr{Value: "selector"}),
			Rhs:      withLineExpr(&luast.StringExpr{Value: fn.Signature}),
		})
		call := withLineExpr(&luast.FuncCallExpr{
			Func: withLineExpr(&luast.IdentExpr{Value: fn.Name}),
			Args: []luast.Expr{
				withLineExpr(&luast.Comma3Expr{AdjustRet: false}),
			},
			AdjustRet: false,
		})
		body = append(body, withLineStmt(&luast.IfStmt{
			Condition: cond,
			Then: []luast.Stmt{
				withLineStmt(&luast.ReturnStmt{Exprs: []luast.Expr{call}}),
			},
			Else: []luast.Stmt{},
		}))
	}
	if hasFallback {
		call := withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_fallback"}),
			Args:      []luast.Expr{},
			AdjustRet: false,
		})
		body = append(body, withLineStmt(&luast.ReturnStmt{Exprs: []luast.Expr{call}}))
	} else {
		body = append(body, withLineStmt(&luast.FuncCallStmt{
			Expr: withLineExpr(&luast.FuncCallExpr{
				Func: withLineExpr(&luast.IdentExpr{Value: "error"}),
				Args: []luast.Expr{
					withLineExpr(&luast.StringExpr{Value: "UNKNOWN_SELECTOR"}),
				},
				AdjustRet: true,
			}),
		}))
	}
	fn := withLineExpr(&luast.FunctionExpr{
		ParList: &luast.ParList{
			HasVargs: true,
			Names:    []string{"selector"},
		},
		Stmts: body,
	})
	return withLineStmt(&luast.AssignStmt{
		Lhs: []luast.Expr{
			withLineExpr(&luast.AttrGetExpr{
				Object: withLineExpr(&luast.IdentExpr{Value: "tos"}),
				Key:    withLineExpr(&luast.StringExpr{Value: "oninvoke"}),
			}),
		},
		Rhs: []luast.Expr{fn},
	})
}

func selectorSignatureForFunction(fn lower.Function) (string, error) {
	if strings.TrimSpace(fn.Name) == "" {
		return "", fmt.Errorf("[%s] function name cannot be empty", diag.CodeLowerUnsupportedFeature)
	}
	types := make([]string, 0, len(fn.Params))
	for _, p := range fn.Params {
		t := normalizeSelectorType(p.Type)
		if t == "" {
			return "", fmt.Errorf("[%s] function parameter type cannot be empty for '%s'", diag.CodeLowerUnsupportedFeature, fn.Name)
		}
		types = append(types, t)
	}
	return fmt.Sprintf("%s(%s)", fn.Name, strings.Join(types, ",")), nil
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

func lowerFunctionToLua(fn lower.Function, env *loweringEnv) (luast.Stmt, error) {
	if strings.TrimSpace(fn.Name) == "" {
		return nil, fmt.Errorf("[%s] function name cannot be empty", diag.CodeLowerUnsupportedFeature)
	}
	if _, err := classifyDirectIRFnModifiers(fn.Modifiers); err != nil {
		return nil, err
	}

	parNames := make([]string, 0, len(fn.Params))
	for _, p := range fn.Params {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			return nil, fmt.Errorf("[%s] function parameter name cannot be empty", diag.CodeLowerUnsupportedFeature)
		}
		parNames = append(parNames, name)
	}

	body, err := tolStmtsToLuaWithCtx(newLoweringCtx(env), fn.Body)
	if err != nil {
		return nil, err
	}

	nameExpr := withLineExpr(&luast.IdentExpr{Value: fn.Name})
	fnExpr := withLineExpr(&luast.FunctionExpr{
		ParList: &luast.ParList{
			HasVargs: false,
			Names:    parNames,
		},
		Stmts: body,
	})
	name := &luast.FuncName{
		Func: nameExpr,
	}
	return withLineStmt(&luast.FuncDefStmt{
		Name: name,
		Func: fnExpr,
	}), nil
}

func lowerConstructorToLua(params []tolast.FieldDecl, body []tolast.Statement, env *loweringEnv) (luast.Stmt, error) {
	parNames := make([]string, 0, len(params))
	for _, p := range params {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			return nil, fmt.Errorf("[%s] constructor parameter name cannot be empty", diag.CodeLowerUnsupportedFeature)
		}
		parNames = append(parNames, name)
	}

	stmts, err := tolStmtsToLuaWithCtx(newLoweringCtx(env), body)
	if err != nil {
		return nil, err
	}
	nameExpr := withLineExpr(&luast.IdentExpr{Value: "__tol_constructor"})
	fnExpr := withLineExpr(&luast.FunctionExpr{
		ParList: &luast.ParList{
			HasVargs: false,
			Names:    parNames,
		},
		Stmts: stmts,
	})
	name := &luast.FuncName{
		Func: nameExpr,
	}
	return withLineStmt(&luast.FuncDefStmt{
		Name: name,
		Func: fnExpr,
	}), nil
}

func classifyDirectIRFnModifiers(mods []string) (string, error) {
	visibility := ""
	for _, m := range mods {
		if !isAllowedDirectIRFnModifier(m) {
			return "", fmt.Errorf("[%s] unsupported function modifier token '%s' in direct IR lowering", diag.CodeLowerUnsupportedFeature, m)
		}
		switch m {
		case "public", "external", "internal", "private":
			if visibility != "" && visibility != m {
				return "", fmt.Errorf("[%s] conflicting function visibility modifiers '%s' and '%s'", diag.CodeLowerUnsupportedFeature, visibility, m)
			}
			visibility = m
		}
	}
	return visibility, nil
}

func isAllowedDirectIRFnModifier(m string) bool {
	switch m {
	case "public", "external", "internal", "private", "view", "pure", "payable":
		return true
	default:
		return false
	}
}

func lowerFallbackToLua(body []tolast.Statement, env *loweringEnv) (luast.Stmt, error) {
	stmts, err := tolStmtsToLuaWithCtx(newLoweringCtx(env), body)
	if err != nil {
		return nil, err
	}
	nameExpr := withLineExpr(&luast.IdentExpr{Value: "__tol_fallback"})
	fnExpr := withLineExpr(&luast.FunctionExpr{
		ParList: &luast.ParList{
			HasVargs: false,
			Names:    []string{},
		},
		Stmts: stmts,
	})
	name := &luast.FuncName{
		Func: nameExpr,
	}
	return withLineStmt(&luast.FuncDefStmt{
		Name: name,
		Func: fnExpr,
	}), nil
}

type loweringLoop struct {
	continueLabel string
}

type loweringCtx struct {
	labelSeq int
	loops    []loweringLoop
	env      *loweringEnv
}

func newLoweringCtx(env *loweringEnv) *loweringCtx {
	return &loweringCtx{
		labelSeq: 0,
		loops:    nil,
		env:      env,
	}
}

func (c *loweringCtx) newLabel(prefix string) string {
	c.labelSeq++
	return fmt.Sprintf("%s_%d", prefix, c.labelSeq)
}

func (c *loweringCtx) pushLoop(continueLabel string) {
	c.loops = append(c.loops, loweringLoop{continueLabel: continueLabel})
}

func (c *loweringCtx) popLoop() {
	if len(c.loops) == 0 {
		return
	}
	c.loops = c.loops[:len(c.loops)-1]
}

func (c *loweringCtx) currentContinueLabel() string {
	if len(c.loops) == 0 {
		return ""
	}
	return c.loops[len(c.loops)-1].continueLabel
}

func tolStmtToLua(ctx *loweringCtx, stmt tolast.Statement) (luast.Stmt, error) {
	switch stmt.Kind {
	case "let":
		exprs := []luast.Expr{}
		if stmt.Expr != nil {
			ex, err := tolExprToLua(ctx, stmt.Expr)
			if err != nil {
				return nil, err
			}
			exprs = append(exprs, ex)
		}
		return withLineStmt(&luast.LocalAssignStmt{
			Names: []string{stmt.Name},
			Exprs: exprs,
		}), nil
	case "set":
		lhs, err := tolExprToLua(ctx, stmt.Target)
		if err != nil {
			return nil, err
		}
		rhs, err := tolExprToLua(ctx, stmt.Expr)
		if err != nil {
			return nil, err
		}
		return withLineStmt(&luast.AssignStmt{
			Lhs: []luast.Expr{lhs},
			Rhs: []luast.Expr{rhs},
		}), nil
	case "return":
		exprs := []luast.Expr{}
		if stmt.Expr != nil {
			ex, err := tolExprToLua(ctx, stmt.Expr)
			if err != nil {
				return nil, err
			}
			exprs = append(exprs, ex)
		}
		return withLineStmt(&luast.ReturnStmt{Exprs: exprs}), nil
	case "if":
		cond, err := tolExprToLua(ctx, stmt.Cond)
		if err != nil {
			return nil, err
		}
		thenStmts, err := tolStmtsToLuaWithCtx(ctx, stmt.Then)
		if err != nil {
			return nil, err
		}
		elseStmts, err := tolStmtsToLuaWithCtx(ctx, stmt.Else)
		if err != nil {
			return nil, err
		}
		return withLineStmt(&luast.IfStmt{
			Condition: cond,
			Then:      thenStmts,
			Else:      elseStmts,
		}), nil
	case "while":
		cond, err := tolExprToLua(ctx, stmt.Cond)
		if err != nil {
			return nil, err
		}
		continueLabel := ctx.newLabel("tol_continue")
		ctx.pushLoop(continueLabel)
		body, err := tolStmtsToLuaWithCtx(ctx, stmt.Body)
		ctx.popLoop()
		if err != nil {
			return nil, err
		}
		body = append(body, withLineStmt(&luast.LabelStmt{Name: continueLabel}))
		return withLineStmt(&luast.WhileStmt{
			Condition: cond,
			Stmts:     body,
		}), nil
	case "break":
		return withLineStmt(&luast.BreakStmt{}), nil
	case "continue":
		lbl := ctx.currentContinueLabel()
		if lbl == "" {
			return nil, fmt.Errorf("[%s] continue used outside lowered loop context", diag.CodeLowerUnsupportedFeature)
		}
		return withLineStmt(&luast.GotoStmt{Label: lbl}), nil
	case "for":
		block := make([]luast.Stmt, 0, 2)
		if stmt.Init != nil {
			initStmt, err := tolStmtToLua(ctx, *stmt.Init)
			if err != nil {
				return nil, err
			}
			block = append(block, initStmt)
		}

		cond := luast.Expr(withLineExpr(&luast.TrueExpr{}))
		if stmt.Cond != nil {
			ce, err := tolExprToLua(ctx, stmt.Cond)
			if err != nil {
				return nil, err
			}
			cond = ce
		}

		continueLabel := ctx.newLabel("tol_for_continue")
		ctx.pushLoop(continueLabel)
		body, err := tolStmtsToLuaWithCtx(ctx, stmt.Body)
		ctx.popLoop()
		if err != nil {
			return nil, err
		}
		body = append(body, withLineStmt(&luast.LabelStmt{Name: continueLabel}))
		if stmt.Post != nil {
			postStmt, err := tolExprStmtToLua(ctx, stmt.Post)
			if err != nil {
				return nil, err
			}
			body = append(body, postStmt)
		}
		block = append(block, withLineStmt(&luast.WhileStmt{
			Condition: cond,
			Stmts:     body,
		}))

		return withLineStmt(&luast.DoBlockStmt{Stmts: block}), nil
	case "expr":
		return tolExprStmtToLua(ctx, stmt.Expr)
	case "emit", "require", "assert", "revert":
		// Lower these to host-visible calls in the current bootstrap stage.
		fnName := stmt.Kind
		if stmt.Kind == "revert" {
			fnName = "error"
		}
		args := []luast.Expr{}
		if stmt.Expr != nil {
			ex, err := tolExprToLua(ctx, stmt.Expr)
			if err != nil {
				return nil, err
			}
			args = append(args, ex)
		}
		call := withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: fnName}),
			Args:      args,
			AdjustRet: true,
		})
		return withLineStmt(&luast.FuncCallStmt{Expr: call}), nil
	default:
		return nil, fmt.Errorf("[%s] unsupported statement kind '%s'", diag.CodeLowerUnsupportedFeature, stmt.Kind)
	}
}

func tolStmtsToLua(in []tolast.Statement) ([]luast.Stmt, error) {
	return tolStmtsToLuaWithCtx(newLoweringCtx(nil), in)
}

func tolStmtsToLuaWithCtx(ctx *loweringCtx, in []tolast.Statement) ([]luast.Stmt, error) {
	if len(in) == 0 {
		return []luast.Stmt{}, nil
	}
	out := make([]luast.Stmt, 0, len(in))
	for _, s := range in {
		ls, err := tolStmtToLua(ctx, s)
		if err != nil {
			return nil, err
		}
		out = append(out, ls)
	}
	return out, nil
}

func tolExprStmtToLua(ctx *loweringCtx, e *tolast.Expr) (luast.Stmt, error) {
	if e == nil {
		return nil, fmt.Errorf("[%s] nil expression statement", diag.CodeLowerUnsupportedFeature)
	}
	if e.Kind == "assign" {
		lhs, err := tolExprToLua(ctx, e.Left)
		if err != nil {
			return nil, err
		}
		rhs, err := tolExprToLua(ctx, e.Right)
		if err != nil {
			return nil, err
		}
		return withLineStmt(&luast.AssignStmt{
			Lhs: []luast.Expr{lhs},
			Rhs: []luast.Expr{rhs},
		}), nil
	}
	ex, err := tolExprToLua(ctx, e)
	if err != nil {
		return nil, err
	}
	call, ok := ex.(*luast.FuncCallExpr)
	if !ok {
		return nil, fmt.Errorf("[%s] expression statement must be a function call or assignment", diag.CodeLowerUnsupportedFeature)
	}
	return withLineStmt(&luast.FuncCallStmt{Expr: call}), nil
}

func tolExprToLua(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, error) {
	if e == nil {
		return nil, fmt.Errorf("[%s] nil expression", diag.CodeLowerUnsupportedFeature)
	}
	switch e.Kind {
	case "ident":
		switch e.Value {
		case "true":
			return withLineExpr(&luast.TrueExpr{}), nil
		case "false":
			return withLineExpr(&luast.FalseExpr{}), nil
		case "nil":
			return withLineExpr(&luast.NilExpr{}), nil
		default:
			return withLineExpr(&luast.IdentExpr{Value: e.Value}), nil
		}
	case "number":
		return withLineExpr(&luast.NumberExpr{Value: e.Value}), nil
	case "string":
		return withLineExpr(&luast.StringExpr{Value: unquoteIfNeeded(e.Value)}), nil
	case "paren":
		return tolExprToLua(ctx, e.Left)
	case "unary":
		inner, err := tolExprToLua(ctx, e.Right)
		if err != nil {
			return nil, err
		}
		switch e.Op {
		case "-":
			return withLineExpr(&luast.UnaryMinusOpExpr{Expr: inner}), nil
		case "!":
			return withLineExpr(&luast.UnaryNotOpExpr{Expr: inner}), nil
		case "~":
			return withLineExpr(&luast.UnaryBitNotOpExpr{Expr: inner}), nil
		case "+":
			return inner, nil
		default:
			return nil, fmt.Errorf("[%s] unsupported unary operator '%s'", diag.CodeLowerUnsupportedFeature, e.Op)
		}
	case "binary":
		lhs, err := tolExprToLua(ctx, e.Left)
		if err != nil {
			return nil, err
		}
		rhs, err := tolExprToLua(ctx, e.Right)
		if err != nil {
			return nil, err
		}
		switch e.Op {
		case "&&":
			return withLineExpr(&luast.LogicalOpExpr{Operator: "and", Lhs: lhs, Rhs: rhs}), nil
		case "||":
			return withLineExpr(&luast.LogicalOpExpr{Operator: "or", Lhs: lhs, Rhs: rhs}), nil
		case "==", "!=", "<", "<=", ">", ">=":
			op := e.Op
			if op == "!=" {
				op = "~="
			}
			return withLineExpr(&luast.RelationalOpExpr{Operator: op, Lhs: lhs, Rhs: rhs}), nil
		case "+", "-", "*", "/", "%", "&", "|", "^", "<<", ">>":
			return withLineExpr(&luast.ArithmeticOpExpr{Operator: e.Op, Lhs: lhs, Rhs: rhs}), nil
		default:
			return nil, fmt.Errorf("[%s] unsupported binary operator '%s'", diag.CodeLowerUnsupportedFeature, e.Op)
		}
	case "assign":
		return nil, fmt.Errorf("[%s] assignment expressions are not supported in expression lowering", diag.CodeLowerUnsupportedFeature)
	case "call":
		if selExpr, ok, err := lowerSelectorBuiltinExpr(e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return selExpr, nil
		}
		callee, err := tolExprToLua(ctx, e.Callee)
		if err != nil {
			return nil, err
		}
		args := make([]luast.Expr, 0, len(e.Args))
		for _, a := range e.Args {
			ex, err := tolExprToLua(ctx, a)
			if err != nil {
				return nil, err
			}
			args = append(args, ex)
		}
		return withLineExpr(&luast.FuncCallExpr{
			Func:      callee,
			Args:      args,
			AdjustRet: true,
		}), nil
	case "member":
		if sel, ok, err := lowerSelectorMemberExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return sel, nil
		}
		obj, err := tolExprToLua(ctx, e.Object)
		if err != nil {
			return nil, err
		}
		return withLineExpr(&luast.AttrGetExpr{
			Object: obj,
			Key:    withLineExpr(&luast.StringExpr{Value: e.Member}),
		}), nil
	case "index":
		obj, err := tolExprToLua(ctx, e.Object)
		if err != nil {
			return nil, err
		}
		idx, err := tolExprToLua(ctx, e.Index)
		if err != nil {
			return nil, err
		}
		return withLineExpr(&luast.AttrGetExpr{
			Object: obj,
			Key:    idx,
		}), nil
	default:
		return nil, fmt.Errorf("[%s] unsupported expression kind '%s'", diag.CodeLowerUnsupportedFeature, e.Kind)
	}
}

func lowerSelectorBuiltinExpr(e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "call" || e.Callee == nil || e.Callee.Kind != "ident" || e.Callee.Value != "selector" {
		return nil, false, nil
	}
	if len(e.Args) != 1 {
		return nil, true, fmt.Errorf("[%s] selector(...) requires exactly one string literal argument", diag.CodeLowerUnsupportedFeature)
	}
	arg := e.Args[0]
	if arg == nil || arg.Kind != "string" {
		return nil, true, fmt.Errorf("[%s] selector(...) argument must be a string literal", diag.CodeLowerUnsupportedFeature)
	}
	sig := unquoteIfNeeded(arg.Value)
	return withLineExpr(&luast.StringExpr{Value: selectorHexFromSignature(sig)}), true, nil
}

func lowerSelectorMemberExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "member" || e.Member != "selector" {
		return nil, false, nil
	}
	if ctx == nil || ctx.env == nil {
		return nil, true, fmt.Errorf("[%s] selector member expression requires contract lowering context", diag.CodeLowerUnsupportedFeature)
	}
	fnRef := e.Object
	if fnRef == nil || fnRef.Kind != "member" || fnRef.Object == nil || fnRef.Object.Kind != "ident" {
		return nil, true, fmt.Errorf("[%s] selector member expression must be 'this.fn.selector' or 'Contract.fn.selector'", diag.CodeLowerUnsupportedFeature)
	}
	scope := strings.TrimSpace(fnRef.Object.Value)
	switch scope {
	case "this":
		// ok
	case ctx.env.contractName:
		// ok
	default:
		return nil, true, fmt.Errorf("[%s] selector scope '%s' is unsupported (expected 'this' or '%s')", diag.CodeLowerUnsupportedFeature, scope, ctx.env.contractName)
	}

	fnName := strings.TrimSpace(fnRef.Member)
	if fnName == "" {
		return nil, true, fmt.Errorf("[%s] selector target function name cannot be empty", diag.CodeLowerUnsupportedFeature)
	}
	sel, ok := ctx.env.selectorByFunction[fnName]
	if !ok {
		return nil, true, fmt.Errorf("[%s] selector target '%s' is not externally dispatchable in current stage", diag.CodeLowerUnsupportedFeature, fnName)
	}
	return withLineExpr(&luast.StringExpr{Value: sel}), true, nil
}

func withLineExpr[T luast.Expr](e T) T {
	e.SetLine(1)
	e.SetLastLine(1)
	return e
}

func withLineStmt[T luast.Stmt](s T) T {
	s.SetLine(1)
	s.SetLastLine(1)
	return s
}

func unquoteIfNeeded(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && ((s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'')) {
		if uq, err := strconv.Unquote(s); err == nil {
			return uq
		}
		return s[1 : len(s)-1]
	}
	return s
}
