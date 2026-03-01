package lua

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	luast "github.com/tos-network/tolang/ast"
	"github.com/tos-network/tolang/parse"
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

	dispatchFns, err := collectDispatchFuncs(p.Functions)
	if err != nil {
		return nil, err
	}
	env, err := buildLoweringEnv(p.ContractName, dispatchFns, p.StorageSlots)
	if err != nil {
		return nil, err
	}

	chunk := make([]luast.Stmt, 0, len(p.Functions)+16)
	if len(p.StorageSlots) > 0 {
		prelude, err := buildStoragePreludeFromLowered(env)
		if err != nil {
			return nil, err
		}
		chunk = append(chunk, prelude...)
	}
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
	storageByName      map[string]storageSlotInfo
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
	typ          string
	mappingDepth int
	baseSlotHash string // compile-time keccak256("tol.slot.<Contract>.<name>")
	luaConstName string // "__tol_s_<name>" - Lua local constant name
}

// computeBaseSlotHash returns the canonical base slot hash for a named storage
// slot per TOL spec §8.3: keccak256("tol.slot.<contractName>.<slotName>").
func computeBaseSlotHash(contractName, slotName string) string {
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write([]byte("tol.slot." + contractName + "." + slotName))
	return "0x" + hex.EncodeToString(h.Sum(nil))
}

func buildLoweringEnv(contractName string, dispatchFns []dispatchFunc, storageSlots []lower.StorageSlot) (*loweringEnv, error) {
	m := make(map[string]string, len(dispatchFns))
	for _, df := range dispatchFns {
		m[df.Name] = df.Signature
	}
	sm := make(map[string]storageSlotInfo, len(storageSlots))
	for _, slot := range storageSlots {
		name := strings.TrimSpace(slot.Name)
		if name == "" {
			return nil, fmt.Errorf("[%s] storage slot name cannot be empty", diag.CodeLowerUnsupportedFeature)
		}
		if _, exists := sm[name]; exists {
			return nil, fmt.Errorf("[%s] duplicate storage slot '%s' in lowered program", diag.CodeLowerUnsupportedFeature, name)
		}
		kind := classifyStorageSlotKind(slot.Type)
		sm[name] = storageSlotInfo{
			name:         name,
			kind:         kind,
			typ:          strings.TrimSpace(slot.Type),
			mappingDepth: mappingTypeDepth(slot.Type),
			baseSlotHash: computeBaseSlotHash(contractName, name),
			luaConstName: "__tol_s_" + name,
		}
	}
	return &loweringEnv{
		contractName:       contractName,
		selectorByFunction: m,
		storageByName:      sm,
	}, nil
}

func classifyStorageSlotKind(t string) storageSlotKind {
	norm := normalizeSelectorType(t)
	compact := strings.ReplaceAll(norm, " ", "")
	switch {
	case strings.HasPrefix(compact, "mapping("):
		return storageKindMapping
	case strings.HasSuffix(compact, "]"):
		return storageKindArray
	default:
		return storageKindScalar
	}
}

func mappingTypeDepth(t string) int {
	compact := strings.ReplaceAll(normalizeSelectorType(t), " ", "")
	if compact == "" {
		return 0
	}
	return strings.Count(compact, "mapping(")
}

func buildStoragePreludeFromLowered(env *loweringEnv) ([]luast.Stmt, error) {
	if env == nil || len(env.storageByName) == 0 {
		return []luast.Stmt{}, nil
	}
	names := make([]string, 0, len(env.storageByName))
	for name := range env.storageByName {
		names = append(names, name)
	}
	sort.Strings(names)

	var sb strings.Builder

	// Emit compile-time base slot hash constants (spec §8.3).
	// Each named slot gets a local constant holding its canonical keccak256 hash.
	for _, name := range names {
		info := env.storageByName[name]
		sb.WriteString(fmt.Sprintf("local %s = %q\n", info.luaConstName, info.baseSlotHash))
	}

	// Flat storage table: storage[bytes32_hex] = value.
	// All key derivation is done before the load/store call.
	sb.WriteString(`__tol_storage = __tol_storage or {}

-- Read a slot by its final derived hash key.
function __tol_sload(slot_hash)
  local v = __tol_storage[slot_hash]
  if v == nil then return 0 end
  return v
end

-- Write a slot by its final derived hash key.
function __tol_sstore(slot_hash, value)
  __tol_storage[slot_hash] = value
  return value
end

-- Derive a mapping slot key: keccak256(encode(key) ++ base_hash).
-- Matches spec §8.3: h_n = H(encode(k_n) ++ h_{n-1}).
function __tol_mkey(key, base)
  local base_hex = base:sub(3)       -- strip leading "0x"
  local key_hex  = __tol_enc(key)    -- 64 hex chars, no 0x prefix
  return keccak256("0x" .. key_hex .. base_hex)
end

-- Compute element slot for a storage array: H(base_slot) + index.
-- Matches spec §8.4: element i at keccak256(base_slot) + i.
function __tol_arr_elem(base, idx)
  local data_base = keccak256(base)  -- H(base): hash the 32-byte base slot
  return uint256_add_hex(data_base, idx)
end

-- Read array length (stored at the base slot itself).
function __tol_slen(base)
  return __tol_sload(base)
end

-- Push a value onto a storage dynamic array.
function __tol_spush(base, value)
  local n = __tol_slen(base)
  local elem_slot = __tol_arr_elem(base, n)
  __tol_sstore(elem_slot, value)
  __tol_sstore(base, n + 1)
  return n + 1
end
`)
	chunk, err := parse.Parse(bytes.NewReader([]byte(sb.String())), "<tol-storage-prelude>")
	if err != nil {
		return nil, fmt.Errorf("[%s] failed to build storage prelude: %w", diag.CodeLowerUnsupportedFeature, err)
	}
	return chunk, nil
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

	ctx := newLoweringCtx(env)
	for _, name := range parNames {
		ctx.declareLocal(name)
	}
	body, err := tolStmtsToLuaWithCtx(ctx, fn.Body)
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

	ctx := newLoweringCtx(env)
	for _, name := range parNames {
		ctx.declareLocal(name)
	}
	stmts, err := tolStmtsToLuaWithCtx(ctx, body)
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
	scopes   []map[string]struct{}
}

func newLoweringCtx(env *loweringEnv) *loweringCtx {
	c := &loweringCtx{
		labelSeq: 0,
		loops:    nil,
		env:      env,
		scopes:   nil,
	}
	c.pushScope()
	return c
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

func (c *loweringCtx) pushScope() {
	c.scopes = append(c.scopes, map[string]struct{}{})
}

func (c *loweringCtx) popScope() {
	if len(c.scopes) == 0 {
		return
	}
	c.scopes = c.scopes[:len(c.scopes)-1]
}

func (c *loweringCtx) declareLocal(name string) {
	if len(c.scopes) == 0 {
		c.pushScope()
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	c.scopes[len(c.scopes)-1][name] = struct{}{}
}

func (c *loweringCtx) isLocalName(name string) bool {
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

func (c *loweringCtx) storageInfoByName(name string) (storageSlotInfo, bool) {
	if c == nil || c.env == nil || len(c.env.storageByName) == 0 {
		return storageSlotInfo{}, false
	}
	info, ok := c.env.storageByName[name]
	return info, ok
}

func (c *loweringCtx) storagePathFromExpr(e *tolast.Expr) (string, []*tolast.Expr, bool) {
	if c == nil || e == nil {
		return "", nil, false
	}
	switch e.Kind {
	case "paren":
		return c.storagePathFromExpr(e.Left)
	case "ident":
		name := strings.TrimSpace(e.Value)
		if name == "" || c.isLocalName(name) {
			return "", nil, false
		}
		if _, ok := c.storageInfoByName(name); !ok {
			return "", nil, false
		}
		return name, []*tolast.Expr{}, true
	case "index":
		slot, keys, ok := c.storagePathFromExpr(e.Object)
		if !ok {
			return "", nil, false
		}
		out := make([]*tolast.Expr, 0, len(keys)+1)
		out = append(out, keys...)
		out = append(out, e.Index)
		return slot, out, true
	default:
		return "", nil, false
	}
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
		out := withLineStmt(&luast.LocalAssignStmt{
			Names: []string{stmt.Name},
			Exprs: exprs,
		})
		ctx.declareLocal(stmt.Name)
		return out, nil
	case "set":
		if storageStmt, ok, err := lowerStorageStoreStmt(ctx, stmt.Target, stmt.Expr); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return storageStmt, nil
		}
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
			ctx.pushScope()
			initStmt, err := tolStmtToLua(ctx, *stmt.Init)
			if err != nil {
				ctx.popScope()
				return nil, err
			}
			block = append(block, initStmt)
		} else {
			ctx.pushScope()
		}

		cond := luast.Expr(withLineExpr(&luast.TrueExpr{}))
		if stmt.Cond != nil {
			ce, err := tolExprToLua(ctx, stmt.Cond)
			if err != nil {
				ctx.popScope()
				return nil, err
			}
			cond = ce
		}

		continueLabel := ctx.newLabel("tol_for_continue")
		ctx.pushLoop(continueLabel)
		body, err := tolStmtsToLuaWithCtx(ctx, stmt.Body)
		ctx.popLoop()
		if err != nil {
			ctx.popScope()
			return nil, err
		}
		body = append(body, withLineStmt(&luast.LabelStmt{Name: continueLabel}))
		if stmt.Post != nil {
			postStmt, err := tolExprStmtToLua(ctx, stmt.Post)
			if err != nil {
				ctx.popScope()
				return nil, err
			}
			body = append(body, postStmt)
		}
		block = append(block, withLineStmt(&luast.WhileStmt{
			Condition: cond,
			Stmts:     body,
		}))
		ctx.popScope()

		return withLineStmt(&luast.DoBlockStmt{Stmts: block}), nil
	case "expr":
		return tolExprStmtToLua(ctx, stmt.Expr)
	case "emit":
		// emit EventName(arg1, arg2, ...) → emit("EventName", arg1, arg2, ...)
		// The runtime must provide an emit() host function.
		args := []luast.Expr{}
		if stmt.Expr != nil && stmt.Expr.Kind == "call" && stmt.Expr.Callee != nil {
			// First arg: event name as string literal.
			eventName := ""
			if stmt.Expr.Callee.Kind == "ident" {
				eventName = stmt.Expr.Callee.Value
			}
			args = append(args, withLineExpr(&luast.StringExpr{Value: eventName}))
			for _, a := range stmt.Expr.Args {
				ex, err := tolExprToLua(ctx, a)
				if err != nil {
					return nil, err
				}
				args = append(args, ex)
			}
		} else if stmt.Expr != nil {
			ex, err := tolExprToLua(ctx, stmt.Expr)
			if err != nil {
				return nil, err
			}
			args = append(args, ex)
		}
		call := withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: "emit"}),
			Args:      args,
			AdjustRet: true,
		})
		return withLineStmt(&luast.FuncCallStmt{Expr: call}), nil
	case "require", "assert":
		// require(cond, "msg") → assert(cond, "msg")
		// assert(cond, "msg") → assert(cond, "msg")
		// Lua's assert(v, msg) raises an error with msg if v is falsy.
		args := []luast.Expr{}
		if stmt.Expr != nil {
			ex, err := tolExprToLua(ctx, stmt.Expr)
			if err != nil {
				return nil, err
			}
			args = append(args, ex)
		}
		if stmt.Text != "" {
			args = append(args, withLineExpr(&luast.StringExpr{Value: unquoteIfNeeded(stmt.Text)}))
		}
		call := withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: "assert"}),
			Args:      args,
			AdjustRet: true,
		})
		return withLineStmt(&luast.FuncCallStmt{Expr: call}), nil
	case "revert":
		// revert "msg" → error("msg")
		args := []luast.Expr{}
		if stmt.Expr != nil {
			ex, err := tolExprToLua(ctx, stmt.Expr)
			if err != nil {
				return nil, err
			}
			args = append(args, ex)
		}
		call := withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: "error"}),
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
	ctx.pushScope()
	defer ctx.popScope()
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
		if storageStmt, ok, err := lowerStorageStoreStmt(ctx, e.Left, e.Right); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return storageStmt, nil
		}
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
		if slotName, keys, ok := ctx.storagePathFromExpr(e); ok {
			return lowerStorageLoadExpr(ctx, slotName, keys)
		}
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
		if storageExpr, ok, err := lowerStoragePushCallExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return storageExpr, nil
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
		if storageExpr, ok, err := lowerStorageLengthMemberExpr(ctx, e); ok || err != nil {
			if err != nil {
				return nil, err
			}
			return storageExpr, nil
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
		if slotName, keys, ok := ctx.storagePathFromExpr(e); ok {
			return lowerStorageLoadExpr(ctx, slotName, keys)
		}
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
	if e == nil || e.Kind != "call" {
		return nil, false, nil
	}
	callee := stripTolParens(e.Callee)
	if callee == nil || callee.Kind != "ident" || strings.TrimSpace(callee.Value) != "selector" {
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

func stripTolParens(e *tolast.Expr) *tolast.Expr {
	cur := e
	for cur != nil && cur.Kind == "paren" {
		cur = cur.Left
	}
	return cur
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

// buildHashSlotExpr builds the final Lua expression for a storage slot access.
// For scalars: the compile-time constant ident (__tol_s_<name>).
// For mappings: a chain of __tol_mkey calls, one per key in order.
// For arrays with an index: __tol_arr_elem(base, idx).
func buildHashSlotExpr(ctx *loweringCtx, info storageSlotInfo, keys []*tolast.Expr) (luast.Expr, error) {
	base := luast.Expr(withLineExpr(&luast.IdentExpr{Value: info.luaConstName}))
	switch info.kind {
	case storageKindScalar:
		return base, nil
	case storageKindMapping:
		// Build: __tol_mkey(k_n, __tol_mkey(k_{n-1}, ... __tol_mkey(k_1, base)))
		cur := base
		for _, k := range keys {
			kExpr, err := tolExprToLua(ctx, k)
			if err != nil {
				return nil, err
			}
			cur = withLineExpr(&luast.FuncCallExpr{
				Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_mkey"}),
				Args:      []luast.Expr{kExpr, cur},
				AdjustRet: true,
			})
		}
		return cur, nil
	case storageKindArray:
		if len(keys) == 0 {
			// Base slot holds the array length; return it directly.
			return base, nil
		}
		// arr[i] → __tol_arr_elem(base, i)
		idxExpr, err := tolExprToLua(ctx, keys[0])
		if err != nil {
			return nil, err
		}
		return withLineExpr(&luast.FuncCallExpr{
			Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_arr_elem"}),
			Args:      []luast.Expr{base, idxExpr},
			AdjustRet: true,
		}), nil
	default:
		return nil, fmt.Errorf("[%s] unsupported storage slot kind for '%s'", diag.CodeLowerUnsupportedFeature, info.name)
	}
}

func lowerStorageStoreStmt(ctx *loweringCtx, target *tolast.Expr, valueExpr *tolast.Expr) (luast.Stmt, bool, error) {
	slotName, keys, ok := ctx.storagePathFromExpr(target)
	if !ok {
		return nil, false, nil
	}
	info, _ := ctx.storageInfoByName(slotName)
	if err := validateStorageKeyShape(info, keys, "set"); err != nil {
		return nil, true, err
	}
	value, err := tolExprToLua(ctx, valueExpr)
	if err != nil {
		return nil, true, err
	}
	slotExpr, err := buildHashSlotExpr(ctx, info, keys)
	if err != nil {
		return nil, true, err
	}
	call := withLineExpr(&luast.FuncCallExpr{
		Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_sstore"}),
		Args:      []luast.Expr{slotExpr, value},
		AdjustRet: true,
	})
	return withLineStmt(&luast.FuncCallStmt{Expr: call}), true, nil
}

func lowerStorageLoadExpr(ctx *loweringCtx, slotName string, keys []*tolast.Expr) (luast.Expr, error) {
	info, _ := ctx.storageInfoByName(slotName)
	if err := validateStorageKeyShape(info, keys, "read"); err != nil {
		return nil, err
	}
	slotExpr, err := buildHashSlotExpr(ctx, info, keys)
	if err != nil {
		return nil, err
	}
	return withLineExpr(&luast.FuncCallExpr{
		Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_sload"}),
		Args:      []luast.Expr{slotExpr},
		AdjustRet: true,
	}), nil
}

func lowerStorageLengthMemberExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "member" || e.Member != "length" {
		return nil, false, nil
	}
	slotName, keys, ok := ctx.storagePathFromExpr(e.Object)
	if !ok {
		return nil, false, nil
	}
	info, _ := ctx.storageInfoByName(slotName)
	if info.kind != storageKindArray || len(keys) != 0 {
		return nil, true, fmt.Errorf("[%s] '.length' is supported only for top-level storage arrays in current stage", diag.CodeLowerUnsupportedFeature)
	}
	return withLineExpr(&luast.FuncCallExpr{
		Func:      withLineExpr(&luast.IdentExpr{Value: "__tol_slen"}),
		Args:      []luast.Expr{withLineExpr(&luast.IdentExpr{Value: info.luaConstName})},
		AdjustRet: true,
	}), true, nil
}

func lowerStoragePushCallExpr(ctx *loweringCtx, e *tolast.Expr) (luast.Expr, bool, error) {
	if e == nil || e.Kind != "call" || e.Callee == nil || e.Callee.Kind != "member" || e.Callee.Member != "push" {
		return nil, false, nil
	}
	slotName, keys, ok := ctx.storagePathFromExpr(e.Callee.Object)
	if !ok {
		return nil, false, nil
	}
	info, _ := ctx.storageInfoByName(slotName)
	if info.kind != storageKindArray || len(keys) != 0 {
		return nil, true, fmt.Errorf("[%s] '.push(v)' is supported only for top-level storage arrays in current stage", diag.CodeLowerUnsupportedFeature)
	}
	if len(e.Args) != 1 {
		return nil, true, fmt.Errorf("[%s] storage array push requires exactly one argument", diag.CodeLowerUnsupportedFeature)
	}
	val, err := tolExprToLua(ctx, e.Args[0])
	if err != nil {
		return nil, true, err
	}
	return withLineExpr(&luast.FuncCallExpr{
		Func: withLineExpr(&luast.IdentExpr{Value: "__tol_spush"}),
		Args: []luast.Expr{
			withLineExpr(&luast.IdentExpr{Value: info.luaConstName}),
			val,
		},
		AdjustRet: true,
	}), true, nil
}

func validateStorageKeyShape(info storageSlotInfo, keys []*tolast.Expr, action string) error {
	switch info.kind {
	case storageKindScalar:
		if len(keys) > 0 {
			return fmt.Errorf("[%s] storage slot '%s' of type '%s' does not support indexed %s", diag.CodeLowerUnsupportedFeature, info.name, info.typ, action)
		}
		return nil
	case storageKindMapping:
		want := info.mappingDepth
		if want <= 0 {
			want = 1
		}
		if len(keys) != want {
			return fmt.Errorf("[%s] storage mapping slot '%s' requires exactly %d index key(s), got %d", diag.CodeLowerUnsupportedFeature, info.name, want, len(keys))
		}
		return nil
	case storageKindArray:
		if action == "set" && len(keys) != 1 {
			return fmt.Errorf("[%s] storage array slot '%s' set requires exactly one index in current stage", diag.CodeLowerUnsupportedFeature, info.name)
		}
		if action == "read" && len(keys) > 1 {
			return fmt.Errorf("[%s] nested storage array indexing is not supported on slot '%s'", diag.CodeLowerUnsupportedFeature, info.name)
		}
		return nil
	default:
		return fmt.Errorf("[%s] unsupported storage slot kind for '%s'", diag.CodeLowerUnsupportedFeature, info.name)
	}
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
