package parser

import "testing"

func TestParseMinimalModule(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if mod == nil || mod.Contract == nil {
		t.Fatalf("expected contract in AST")
	}
	if mod.Version != "0.2" {
		t.Fatalf("unexpected version: %s", mod.Version)
	}
	if mod.Contract.Name != "Demo" {
		t.Fatalf("unexpected contract name: %s", mod.Contract.Name)
	}
}

func TestParseContractSubset(t *testing.T) {
	src := []byte(`
tol 0.2
interface IERC20 { fn transfer(to: address, amount: u256) public; }
library MathX { fn dummy() { } }
contract Demo {
  storage {
    slot balances: mapping(address => u256);
    slot total_supply: u256;
  }

  event Transfer(from: address indexed, to: address indexed, value: u256)

  fn transfer(to: address, amount: u256) -> (ok: bool) public {
    if amount > 0 { return true; }
    return false;
  }

  constructor(owner: address) public { }
  fallback { revert "UNKNOWN_SELECTOR"; }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if mod == nil || mod.Contract == nil {
		t.Fatalf("expected contract in AST")
	}
	if len(mod.SkippedTopDecls) != 2 {
		t.Fatalf("unexpected skipped top decl count: %d", len(mod.SkippedTopDecls))
	}
	if mod.Contract.Storage == nil || len(mod.Contract.Storage.Slots) != 2 {
		t.Fatalf("unexpected storage parse result: %#v", mod.Contract.Storage)
	}
	if len(mod.Contract.Events) != 1 || mod.Contract.Events[0].Name != "Transfer" {
		t.Fatalf("unexpected events parse result: %#v", mod.Contract.Events)
	}
	if len(mod.Contract.Functions) != 1 || mod.Contract.Functions[0].Name != "transfer" {
		t.Fatalf("unexpected functions parse result: %#v", mod.Contract.Functions)
	}
	if len(mod.Contract.Functions[0].Body) != 2 {
		t.Fatalf("unexpected function body stmt count: %d", len(mod.Contract.Functions[0].Body))
	}
	if mod.Contract.Functions[0].Body[0].Kind != "if" {
		t.Fatalf("unexpected first stmt kind: %s", mod.Contract.Functions[0].Body[0].Kind)
	}
	if len(mod.Contract.Functions[0].Body[0].Then) != 1 || mod.Contract.Functions[0].Body[0].Then[0].Kind != "return" {
		t.Fatalf("unexpected if-then body: %#v", mod.Contract.Functions[0].Body[0].Then)
	}
	if mod.Contract.Functions[0].Body[1].Kind != "return" {
		t.Fatalf("unexpected second stmt kind: %s", mod.Contract.Functions[0].Body[1].Kind)
	}
	if mod.Contract.Constructor == nil {
		t.Fatalf("expected constructor")
	}
	if len(mod.Contract.Constructor.Body) != 0 {
		t.Fatalf("unexpected constructor body stmt count: %d", len(mod.Contract.Constructor.Body))
	}
	if mod.Contract.Fallback == nil {
		t.Fatalf("expected fallback")
	}
	if len(mod.Contract.Fallback.Body) != 1 || mod.Contract.Fallback.Body[0].Kind != "revert" {
		t.Fatalf("unexpected fallback body: %#v", mod.Contract.Fallback.Body)
	}
}

func TestParseFunctionSelectorAttribute(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  @selector("0x1234abcd")
  fn ping(a: u256) public {
    return;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if mod == nil || mod.Contract == nil || len(mod.Contract.Functions) != 1 {
		t.Fatalf("unexpected parse result: %#v", mod)
	}
	fn := mod.Contract.Functions[0]
	if fn.Name != "ping" {
		t.Fatalf("unexpected function name: %s", fn.Name)
	}
	if fn.SelectorOverride != "0x1234abcd" {
		t.Fatalf("unexpected selector override: %q", fn.SelectorOverride)
	}
}

func TestParseSkippedContractDecls(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  error Unauthorized(sender: address);
  enum Mode { A, B }
  modifier onlyOwner() { _; }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if mod == nil || mod.Contract == nil {
		t.Fatalf("expected contract")
	}
	if len(mod.Contract.SkippedDecls) != 3 {
		t.Fatalf("unexpected skipped decl count: %d", len(mod.Contract.SkippedDecls))
	}
}

func TestParseMissingHeader(t *testing.T) {
	src := []byte(`contract Demo {}`)
	_, diags := ParseFile("<test>", src)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics for missing tol header")
	}
}

func TestParseLoopStatements(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn run(n: u256) public {
    let i: u256 = 0;
    while i < n {
      if i == 5 {
        break;
      } else {
        set i = i + 1;
        continue;
      }
    }
    for let j: u256 = 0; j < n; j = j + 1 {
      emit Tick(j);
    }
    return;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if mod == nil || mod.Contract == nil || len(mod.Contract.Functions) != 1 {
		t.Fatalf("unexpected module parse result")
	}
	body := mod.Contract.Functions[0].Body
	if len(body) != 4 {
		t.Fatalf("unexpected top-level stmt count: %d", len(body))
	}
	if body[0].Kind != "let" || body[0].Name != "i" || body[0].Type != "u256" {
		t.Fatalf("unexpected let stmt: %#v", body[0])
	}
	if body[0].Expr == nil || body[0].Expr.Kind != "number" || body[0].Expr.Value != "0" {
		t.Fatalf("unexpected let init expr: %#v", body[0].Expr)
	}
	if body[1].Kind != "while" {
		t.Fatalf("expected while stmt, got: %s", body[1].Kind)
	}
	if body[1].Cond == nil || body[1].Cond.Kind != "binary" || body[1].Cond.Op != "<" {
		t.Fatalf("unexpected while cond: %#v", body[1].Cond)
	}
	if len(body[1].Body) != 1 || body[1].Body[0].Kind != "if" {
		t.Fatalf("unexpected while body: %#v", body[1].Body)
	}
	if body[2].Kind != "for" {
		t.Fatalf("expected for stmt, got: %s", body[2].Kind)
	}
	if body[2].Init == nil || body[2].Init.Kind != "let" || body[2].Init.Name != "j" {
		t.Fatalf("unexpected for init: %#v", body[2].Init)
	}
	if body[2].Cond == nil || body[2].Cond.Kind != "binary" || body[2].Cond.Op != "<" {
		t.Fatalf("unexpected for cond: %#v", body[2].Cond)
	}
	if body[2].Post == nil || body[2].Post.Kind != "assign" || body[2].Post.Op != "=" {
		t.Fatalf("unexpected for post: %#v", body[2].Post)
	}
	if len(body[2].Body) != 1 || body[2].Body[0].Kind != "emit" {
		t.Fatalf("unexpected for body: %#v", body[2].Body)
	}
	if body[2].Body[0].Expr == nil || body[2].Body[0].Expr.Kind != "call" {
		t.Fatalf("unexpected emit expr: %#v", body[2].Body[0].Expr)
	}
	if body[3].Kind != "return" {
		t.Fatalf("expected return stmt, got: %s", body[3].Kind)
	}
}

func TestParseExpressionPrecedence(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn run() public {
    let x: u256 = a + b * c;
    set x = (x + 1) * foo(2, arr[i]).v;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	fn := mod.Contract.Functions[0]
	if len(fn.Body) != 2 {
		t.Fatalf("unexpected stmt count: %d", len(fn.Body))
	}

	letExpr := fn.Body[0].Expr
	if letExpr == nil || letExpr.Kind != "binary" || letExpr.Op != "+" {
		t.Fatalf("unexpected let expr: %#v", letExpr)
	}
	if letExpr.Right == nil || letExpr.Right.Kind != "binary" || letExpr.Right.Op != "*" {
		t.Fatalf("expected multiplication on right side due precedence, got: %#v", letExpr.Right)
	}

	setStmt := fn.Body[1]
	if setStmt.Kind != "set" || setStmt.Expr == nil {
		t.Fatalf("unexpected set stmt: %#v", setStmt)
	}
	if setStmt.Expr.Kind != "binary" || setStmt.Expr.Op != "*" {
		t.Fatalf("unexpected set rhs expr: %#v", setStmt.Expr)
	}
	if setStmt.Expr.Right == nil || setStmt.Expr.Right.Kind != "member" || setStmt.Expr.Right.Member != "v" {
		t.Fatalf("unexpected chained member expr: %#v", setStmt.Expr.Right)
	}
}

func TestParseBitwiseAndShiftExpressions(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn run() public {
    let x: u256 = a | b & c ^ d;
    set x = ~x << 2 >> 1;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	fn := mod.Contract.Functions[0]
	if len(fn.Body) != 2 {
		t.Fatalf("unexpected stmt count: %d", len(fn.Body))
	}

	letExpr := fn.Body[0].Expr
	if letExpr == nil || letExpr.Kind != "binary" || letExpr.Op != "|" {
		t.Fatalf("unexpected bitwise root expr: %#v", letExpr)
	}
	if letExpr.Right == nil || letExpr.Right.Kind != "binary" || letExpr.Right.Op != "^" {
		t.Fatalf("unexpected right branch expr: %#v", letExpr.Right)
	}
	if letExpr.Right.Left == nil || letExpr.Right.Left.Kind != "binary" || letExpr.Right.Left.Op != "&" {
		t.Fatalf("unexpected bit-and expr: %#v", letExpr.Right.Left)
	}

	setExpr := fn.Body[1].Expr
	if setExpr == nil || setExpr.Kind != "binary" || setExpr.Op != ">>" {
		t.Fatalf("unexpected set expr root: %#v", setExpr)
	}
	if setExpr.Left == nil || setExpr.Left.Kind != "binary" || setExpr.Left.Op != "<<" {
		t.Fatalf("unexpected shift-left branch: %#v", setExpr.Left)
	}
	if setExpr.Left.Left == nil || setExpr.Left.Left.Kind != "unary" || setExpr.Left.Left.Op != "~" {
		t.Fatalf("unexpected unary bit-not branch: %#v", setExpr.Left.Left)
	}
}
