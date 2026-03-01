package lua

import (
	"strings"
	"testing"
)

func TestParseTOLModule(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {}
`)
	mod, err := ParseTOLModule(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if mod == nil || mod.Contract == nil || mod.Contract.Name != "Demo" {
		t.Fatalf("unexpected module: %#v", mod)
	}
}

func TestCompileTOLToBytecodeMinimalContract(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {}
`)
	bc, err := CompileTOLToBytecode(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}
	if len(bc) == 0 {
		t.Fatalf("expected non-empty bytecode")
	}
}

func TestBuildIRFromTOLMinimalContract(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {}
`)
	irp, err := BuildIRFromTOL(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected build error: %v", err)
	}
	if irp == nil || irp.Root == nil {
		t.Fatalf("expected non-nil IR")
	}
	if len(irp.Root.Instructions) != 1 {
		t.Fatalf("unexpected instruction count: %d", len(irp.Root.Instructions))
	}
	if irp.Root.Instructions[0].Op != OP_RETURN {
		t.Fatalf("expected RETURN op, got=%d", irp.Root.Instructions[0].Op)
	}
}

func TestBuildIRFromTOLFunctionSubset(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn ping() public { return; }
}
`)
	irp, err := BuildIRFromTOL(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected build error: %v", err)
	}
	if irp == nil || irp.Root == nil {
		t.Fatalf("expected non-nil IR")
	}
	if len(irp.Root.Instructions) < 2 {
		t.Fatalf("expected non-trivial instruction stream")
	}
}

func TestBuildIRFromTOLFallbackSubset(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fallback {
    let x: u256 = 1;
    set x = x + 2;
    if x > 1 {
      emit Tick(x);
    } else {
      revert "bad";
    }
    return;
  }
}
`)
	irp, err := BuildIRFromTOL(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected build error: %v", err)
	}
	if irp == nil || irp.Root == nil {
		t.Fatalf("expected non-nil IR")
	}
	if len(irp.Root.Instructions) < 2 {
		t.Fatalf("expected non-trivial instruction stream")
	}
}

func TestBuildIRFromTOLFallbackForContinueSubset(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fallback {
    let n: u256 = 3;
    for let i: u256 = 0; i < n; i = i + 1 {
      if i == 1 {
        continue;
      }
      emit Tick(i);
    }
    return;
  }
}
`)
	irp, err := BuildIRFromTOL(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected build error: %v", err)
	}
	if irp == nil || irp.Root == nil {
		t.Fatalf("expected non-nil IR")
	}
	if len(irp.Root.Instructions) < 3 {
		t.Fatalf("expected non-trivial instruction stream")
	}
}

func TestBuildIRFromTOLFallbackUnsupportedContinue(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fallback {
    continue;
  }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected unsupported feature error")
	}
	if !strings.Contains(err.Error(), "TOL2007") {
		t.Fatalf("expected TOL2007 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsUnknownFnModifier(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn ping() onlyOwner { return; }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected unsupported modifier error")
	}
	if !strings.Contains(err.Error(), "TOL2014") {
		t.Fatalf("expected TOL2014 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsDuplicateFnVisibilityModifier(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn ping() public public { return; }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected duplicate modifier error")
	}
	if !strings.Contains(err.Error(), "TOL2015") {
		t.Fatalf("expected TOL2015 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsSelectorOverrideOnNonExternalFunction(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  @selector("0x12345678")
  fn f() internal {
    return;
  }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected selector visibility error")
	}
	if !strings.Contains(err.Error(), "TOL2027") {
		t.Fatalf("expected TOL2027 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsSetTargetReservedLiteralIdent(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn run() public {
    set true = 1;
    return;
  }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected set-target error")
	}
	if !strings.Contains(err.Error(), "TOL2008") {
		t.Fatalf("expected TOL2008 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsAssignExprTargetSelectorMember(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn mark() public { return; }
  fn run() public {
    this.mark.selector = 1;
    return;
  }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected set-target error")
	}
	if !strings.Contains(err.Error(), "TOL2008") {
		t.Fatalf("expected TOL2008 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsDuplicateLocalLetInSameScope(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn run() public {
    let x: u256 = 1;
    let x: u256 = 2;
    return;
  }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected duplicate local error")
	}
	if !strings.Contains(err.Error(), "TOL2028") {
		t.Fatalf("expected TOL2028 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsFunctionCallArityMismatch(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn sum(a: u256, b: u256) public { return; }
  fn run() public {
    sum(1);
    return;
  }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected arity error")
	}
	if !strings.Contains(err.Error(), "TOL2019") {
		t.Fatalf("expected TOL2019 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsThisMemberFunctionCallArityMismatch(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn sum(a: u256, b: u256) public { return; }
  fn run() public {
    this.sum(1);
    return;
  }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected arity error")
	}
	if !strings.Contains(err.Error(), "TOL2019") {
		t.Fatalf("expected TOL2019 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsUnknownThisMemberFunctionCallTarget(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn run() public {
    this.missing();
    return;
  }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected unknown call target error")
	}
	if !strings.Contains(err.Error(), "TOL2031") {
		t.Fatalf("expected TOL2031 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsUnknownContractMemberFunctionCallTarget(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn run() public {
    Demo.missing();
    return;
  }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected unknown call target error")
	}
	if !strings.Contains(err.Error(), "TOL2031") {
		t.Fatalf("expected TOL2031 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsThisMemberCallToNonExternalFunction(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn sum() internal { return; }
  fn run() public {
    this.sum();
    return;
  }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected call visibility error")
	}
	if !strings.Contains(err.Error(), "TOL2032") {
		t.Fatalf("expected TOL2032 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsContractMemberCallToNonExternalFunction(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn sum() internal { return; }
  fn run() public {
    Demo.sum();
    return;
  }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected call visibility error")
	}
	if !strings.Contains(err.Error(), "TOL2032") {
		t.Fatalf("expected TOL2032 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsContractMemberFunctionCallArityMismatch(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn sum(a: u256, b: u256) public { return; }
  fn run() public {
    Demo.sum(1);
    return;
  }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected arity error")
	}
	if !strings.Contains(err.Error(), "TOL2019") {
		t.Fatalf("expected TOL2019 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsNonCallAssignExprStatement(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn run() public {
    1 + 2;
    return;
  }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected expression-statement shape error")
	}
	if !strings.Contains(err.Error(), "TOL2020") {
		t.Fatalf("expected TOL2020 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsAssignExprInRequireExpr(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn run() public {
    require((x = 1), "BAD");
    return;
  }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected assignment-placement error")
	}
	if !strings.Contains(err.Error(), "TOL2020") {
		t.Fatalf("expected TOL2020 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsAssignExprInEmitPayload(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn run() public {
    emit Tick((x = 1));
    return;
  }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected assignment-placement error")
	}
	if !strings.Contains(err.Error(), "TOL2020") {
		t.Fatalf("expected TOL2020 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsSelectorBuiltinExprStatement(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn run() public {
    selector("transfer(address,u256)");
    return;
  }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected statement-shape error")
	}
	if !strings.Contains(err.Error(), "TOL2021") {
		t.Fatalf("expected TOL2021 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsNestedAssignInExprCallArg(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn run() public {
    foo((x = 1));
    return;
  }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected nested assign placement error")
	}
	if !strings.Contains(err.Error(), "TOL2020") {
		t.Fatalf("expected TOL2020 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsRequireMissingParenExpr(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn run() public {
    require;
    return;
  }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected parse error")
	}
	if !strings.Contains(err.Error(), "TOL1001") {
		t.Fatalf("expected TOL1001 parse error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsRevertNonStringPayload(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn run() public {
    revert err;
  }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected revert payload error")
	}
	if !strings.Contains(err.Error(), "TOL2022") {
		t.Fatalf("expected TOL2022 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsEmitDeclaredEventArityMismatch(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  event Tick(a: u256, b: u256)
  fn run() public {
    emit Tick(1);
    return;
  }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected emit arity error")
	}
	if !strings.Contains(err.Error(), "TOL2023") {
		t.Fatalf("expected TOL2023 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsEmitMemberCallPayload(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn run() public {
    emit obj.Tick(1);
    return;
  }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected emit payload shape error")
	}
	if !strings.Contains(err.Error(), "TOL2021") {
		t.Fatalf("expected TOL2021 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsDuplicateEventDeclarations(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  event Tick(a: u256)
  event Tick(b: u256)
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected duplicate event error")
	}
	if !strings.Contains(err.Error(), "TOL2024") {
		t.Fatalf("expected TOL2024 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsDuplicateEventParams(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  event Tick(a: u256, a: u256)
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected duplicate event param error")
	}
	if !strings.Contains(err.Error(), "TOL2016") {
		t.Fatalf("expected TOL2016 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsFunctionParamReturnNameCollision(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn f(x: u256) -> (x: u256) public {
    return 1;
  }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected param/return collision error")
	}
	if !strings.Contains(err.Error(), "TOL2029") {
		t.Fatalf("expected TOL2029 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsNonVoidFunctionMissingReturnPath(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn f(x: u256) -> (out: u256) public {
    if x > 0 {
      return 1;
    }
  }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected non-void return-path error")
	}
	if !strings.Contains(err.Error(), "TOL2017") {
		t.Fatalf("expected TOL2017 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsUnreachableStmtAfterReturn(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn run() public {
    return;
    let x: u256 = 1;
  }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected unreachable statement error")
	}
	if !strings.Contains(err.Error(), "TOL2030") {
		t.Fatalf("expected TOL2030 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsUnreachableStmtAfterBreakInLoop(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn run() public {
    while true {
      break;
      let x: u256 = 1;
    }
    return;
  }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected unreachable statement error")
	}
	if !strings.Contains(err.Error(), "TOL2030") {
		t.Fatalf("expected TOL2030 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsEmitUnknownDeclaredEventSet(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  event Tick(a: u256)
  fn run() public {
    emit Other(1);
    return;
  }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected unknown emit event error")
	}
	if !strings.Contains(err.Error(), "TOL2025") {
		t.Fatalf("expected TOL2025 sema error, got: %v", err)
	}
}

func TestBuildIRFromTOLRejectsEventFunctionNameCollision(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  event Tick(a: u256)
  fn Tick() public { return; }
}
`)
	_, err := BuildIRFromTOL(src, "<tol>")
	if err == nil {
		t.Fatalf("expected name collision error")
	}
	if !strings.Contains(err.Error(), "TOL2026") {
		t.Fatalf("expected TOL2026 sema error, got: %v", err)
	}
}

func TestCompileTOLToBytecodeOnInvokeDispatchesByDefaultSelector(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn add(a: u256, b: u256) public {
    set got = a + b;
    return;
  }
}
`)
	bc, err := CompileTOLToBytecode(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}
	L := NewState()
	defer L.Close()
	if err := L.DoBytecode(bc); err != nil {
		t.Fatalf("DoBytecode failed: %v", err)
	}

	tos := L.GetGlobal("tos")
	oninvoke := L.GetField(tos, "oninvoke")
	if oninvoke == LNil {
		t.Fatalf("expected tos.oninvoke wrapper")
	}

	L.Push(oninvoke)
	L.Push(LString(selectorHexFromSignature("add(u256,u256)")))
	L.Push(lNumberFromInt(3))
	L.Push(lNumberFromInt(4))
	if err := L.PCall(3, 0, nil); err != nil {
		t.Fatalf("oninvoke call failed: %v", err)
	}

	if got := LVAsString(L.GetGlobal("got")); got != "7" {
		t.Fatalf("unexpected result: got=%s want=7", got)
	}
}

func TestCompileTOLToBytecodeOnInvokeDispatchesBySelectorOverride(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  @selector("0xdeadbeef")
  fn add(a: u256, b: u256) public {
    set got = a + b;
    return;
  }
}
`)
	bc, err := CompileTOLToBytecode(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}
	L := NewState()
	defer L.Close()
	if err := L.DoBytecode(bc); err != nil {
		t.Fatalf("DoBytecode failed: %v", err)
	}

	tos := L.GetGlobal("tos")
	oninvoke := L.GetField(tos, "oninvoke")
	if oninvoke == LNil {
		t.Fatalf("expected tos.oninvoke wrapper")
	}

	L.Push(oninvoke)
	L.Push(LString("0xdeadbeef"))
	L.Push(lNumberFromInt(8))
	L.Push(lNumberFromInt(9))
	if err := L.PCall(3, 0, nil); err != nil {
		t.Fatalf("oninvoke call failed: %v", err)
	}

	if got := LVAsString(L.GetGlobal("got")); got != "17" {
		t.Fatalf("unexpected result: got=%s want=17", got)
	}
}

func TestCompileTOLToBytecodeOnInvokeFallsBack(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn ping() public {
    set called = 1;
    return;
  }
  fallback {
    set called = 9;
    return;
  }
}
`)
	bc, err := CompileTOLToBytecode(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}
	L := NewState()
	defer L.Close()
	if err := L.DoBytecode(bc); err != nil {
		t.Fatalf("DoBytecode failed: %v", err)
	}

	tos := L.GetGlobal("tos")
	oninvoke := L.GetField(tos, "oninvoke")
	if oninvoke == LNil {
		t.Fatalf("expected tos.oninvoke wrapper")
	}

	L.Push(oninvoke)
	L.Push(LString("unknown()"))
	if err := L.PCall(1, 0, nil); err != nil {
		t.Fatalf("unexpected oninvoke error: %v", err)
	}

	if got := LVAsString(L.GetGlobal("called")); got != "9" {
		t.Fatalf("unexpected fallback result: got=%s want=9", got)
	}
}

func TestCompileTOLToBytecodeOnInvokeUnknownSelectorWithoutFallback(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn ping() public { return; }
}
`)
	bc, err := CompileTOLToBytecode(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}
	L := NewState()
	defer L.Close()
	if err := L.DoBytecode(bc); err != nil {
		t.Fatalf("DoBytecode failed: %v", err)
	}

	tos := L.GetGlobal("tos")
	oninvoke := L.GetField(tos, "oninvoke")
	if oninvoke == LNil {
		t.Fatalf("expected tos.oninvoke wrapper")
	}

	L.Push(oninvoke)
	L.Push(LString("missing()"))
	err = L.PCall(1, 0, nil)
	if err == nil {
		t.Fatalf("expected UNKNOWN_SELECTOR error")
	}
	if !strings.Contains(err.Error(), "UNKNOWN_SELECTOR") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompileTOLToBytecodeOnCreateCallsConstructor(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  constructor {
    set booted = 1;
    return;
  }
}
`)
	bc, err := CompileTOLToBytecode(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}
	L := NewState()
	defer L.Close()
	if err := L.DoBytecode(bc); err != nil {
		t.Fatalf("DoBytecode failed: %v", err)
	}

	tos := L.GetGlobal("tos")
	oncreate := L.GetField(tos, "oncreate")
	if oncreate == LNil {
		t.Fatalf("expected tos.oncreate wrapper")
	}

	L.Push(oncreate)
	if err := L.PCall(0, 0, nil); err != nil {
		t.Fatalf("oncreate call failed: %v", err)
	}
	if got := LVAsString(L.GetGlobal("booted")); got != "1" {
		t.Fatalf("unexpected constructor side effect: got=%s want=1", got)
	}
}

func TestCompileTOLToBytecodeOnCreatePassesConstructorArgs(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  constructor(owner: u256, supply: u256) {
    set owner_copy = owner;
    set supply_copy = supply;
    return;
  }
}
`)
	bc, err := CompileTOLToBytecode(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}
	L := NewState()
	defer L.Close()
	if err := L.DoBytecode(bc); err != nil {
		t.Fatalf("DoBytecode failed: %v", err)
	}

	tos := L.GetGlobal("tos")
	oncreate := L.GetField(tos, "oncreate")
	if oncreate == LNil {
		t.Fatalf("expected tos.oncreate wrapper")
	}

	L.Push(oncreate)
	L.Push(lNumberFromInt(11))
	L.Push(lNumberFromInt(22))
	if err := L.PCall(2, 0, nil); err != nil {
		t.Fatalf("oncreate call failed: %v", err)
	}
	if got := LVAsString(L.GetGlobal("owner_copy")); got != "11" {
		t.Fatalf("unexpected owner copy: got=%s want=11", got)
	}
	if got := LVAsString(L.GetGlobal("supply_copy")); got != "22" {
		t.Fatalf("unexpected supply copy: got=%s want=22", got)
	}
}

func TestCompileTOLToBytecodeSelectorBuiltinLiteral(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn mark() public {
    set sel = selector("transfer(address,u256)");
    return;
  }
}
`)
	bc, err := CompileTOLToBytecode(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}
	L := NewState()
	defer L.Close()
	if err := L.DoBytecode(bc); err != nil {
		t.Fatalf("DoBytecode failed: %v", err)
	}

	tos := L.GetGlobal("tos")
	oninvoke := L.GetField(tos, "oninvoke")
	if oninvoke == LNil {
		t.Fatalf("expected tos.oninvoke wrapper")
	}

	L.Push(oninvoke)
	L.Push(LString(selectorHexFromSignature("mark()")))
	if err := L.PCall(1, 0, nil); err != nil {
		t.Fatalf("oninvoke call failed: %v", err)
	}
	want := selectorHexFromSignature("transfer(address,u256)")
	if got := LVAsString(L.GetGlobal("sel")); got != want {
		t.Fatalf("unexpected selector result: got=%s want=%s", got, want)
	}
}

func TestCompileTOLToBytecodeSelectorBuiltinRejectsNonLiteralArg(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn bad(sig: string) public {
    set sel = selector(sig);
    return;
  }
}
`)
	_, err := CompileTOLToBytecode(src, "<tol>")
	if err == nil {
		t.Fatalf("expected compile error")
	}
	if !strings.Contains(err.Error(), "TOL2012") {
		t.Fatalf("expected TOL2012 error, got: %v", err)
	}
}

func TestCompileTOLToBytecodeSelectorMemberThisAndContract(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn mark() public {
    set s1 = this.mark.selector;
    set s2 = Demo.mark.selector;
    return;
  }
}
`)
	bc, err := CompileTOLToBytecode(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}
	L := NewState()
	defer L.Close()
	if err := L.DoBytecode(bc); err != nil {
		t.Fatalf("DoBytecode failed: %v", err)
	}

	tos := L.GetGlobal("tos")
	oninvoke := L.GetField(tos, "oninvoke")
	if oninvoke == LNil {
		t.Fatalf("expected tos.oninvoke wrapper")
	}

	L.Push(oninvoke)
	L.Push(LString(selectorHexFromSignature("mark()")))
	if err := L.PCall(1, 0, nil); err != nil {
		t.Fatalf("oninvoke call failed: %v", err)
	}
	want := selectorHexFromSignature("mark()")
	if got := LVAsString(L.GetGlobal("s1")); got != want {
		t.Fatalf("unexpected s1 selector: got=%s want=%s", got, want)
	}
	if got := LVAsString(L.GetGlobal("s2")); got != want {
		t.Fatalf("unexpected s2 selector: got=%s want=%s", got, want)
	}
}

func TestCompileTOLToBytecodeSelectorMemberRespectsOverride(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  @selector("0xfeedbeef")
  fn mark() public {
    set sel = this.mark.selector;
    return;
  }
}
`)
	bc, err := CompileTOLToBytecode(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}
	L := NewState()
	defer L.Close()
	if err := L.DoBytecode(bc); err != nil {
		t.Fatalf("DoBytecode failed: %v", err)
	}

	tos := L.GetGlobal("tos")
	oninvoke := L.GetField(tos, "oninvoke")
	if oninvoke == LNil {
		t.Fatalf("expected tos.oninvoke wrapper")
	}

	L.Push(oninvoke)
	L.Push(LString("0xfeedbeef"))
	if err := L.PCall(1, 0, nil); err != nil {
		t.Fatalf("oninvoke call failed: %v", err)
	}
	if got := LVAsString(L.GetGlobal("sel")); got != "0xfeedbeef" {
		t.Fatalf("unexpected selector override: got=%s want=0xfeedbeef", got)
	}
}

func TestCompileTOLToBytecodeStorageScalarSlot(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  storage {
    slot total: u256;
  }
  fn add(v: u256) public {
    set total = total + v;
    return;
  }
  fn read() public {
    set got = total;
    return;
  }
}
`)
	bc, err := CompileTOLToBytecode(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}
	L := NewState()
	defer L.Close()
	if err := L.DoBytecode(bc); err != nil {
		t.Fatalf("DoBytecode failed: %v", err)
	}

	tos := L.GetGlobal("tos")
	oninvoke := L.GetField(tos, "oninvoke")
	if oninvoke == LNil {
		t.Fatalf("expected tos.oninvoke wrapper")
	}

	L.Push(oninvoke)
	L.Push(LString(selectorHexFromSignature("add(u256)")))
	L.Push(lNumberFromInt(5))
	if err := L.PCall(2, 0, nil); err != nil {
		t.Fatalf("oninvoke call failed: %v", err)
	}
	L.Push(oninvoke)
	L.Push(LString(selectorHexFromSignature("add(u256)")))
	L.Push(lNumberFromInt(7))
	if err := L.PCall(2, 0, nil); err != nil {
		t.Fatalf("oninvoke call failed: %v", err)
	}

	L.Push(oninvoke)
	L.Push(LString(selectorHexFromSignature("read()")))
	if err := L.PCall(1, 0, nil); err != nil {
		t.Fatalf("oninvoke call failed: %v", err)
	}
	if got := LVAsString(L.GetGlobal("got")); got != "12" {
		t.Fatalf("unexpected storage read result: got=%s want=12", got)
	}
}

func TestCompileTOLToBytecodeStorageMappingSlot(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  storage {
    slot balances: mapping(address => u256);
  }
  fn add(who: address, amount: u256) public {
    let cur: u256 = balances[who];
    set balances[who] = cur + amount;
    set got = balances[who];
    return;
  }
}
`)
	bc, err := CompileTOLToBytecode(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}
	L := NewState()
	defer L.Close()
	if err := L.DoBytecode(bc); err != nil {
		t.Fatalf("DoBytecode failed: %v", err)
	}

	tos := L.GetGlobal("tos")
	oninvoke := L.GetField(tos, "oninvoke")
	if oninvoke == LNil {
		t.Fatalf("expected tos.oninvoke wrapper")
	}

	L.Push(oninvoke)
	L.Push(LString(selectorHexFromSignature("add(address,u256)")))
	L.Push(lNumberFromInt(11))
	L.Push(lNumberFromInt(3))
	if err := L.PCall(3, 0, nil); err != nil {
		t.Fatalf("oninvoke call failed: %v", err)
	}
	if got := LVAsString(L.GetGlobal("got")); got != "3" {
		t.Fatalf("unexpected first mapping result: got=%s want=3", got)
	}

	L.Push(oninvoke)
	L.Push(LString(selectorHexFromSignature("add(address,u256)")))
	L.Push(lNumberFromInt(11))
	L.Push(lNumberFromInt(4))
	if err := L.PCall(3, 0, nil); err != nil {
		t.Fatalf("oninvoke call failed: %v", err)
	}
	if got := LVAsString(L.GetGlobal("got")); got != "7" {
		t.Fatalf("unexpected second mapping result: got=%s want=7", got)
	}
}

func TestCompileTOLToBytecodeStorageArraySlot(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  storage {
    slot xs: u256[];
  }
  fn append(v: u256) public {
    xs.push(v);
    set len_out = xs.length;
    return;
  }
  fn read(i: u256) public {
    set got = xs[i];
    return;
  }
}
`)
	bc, err := CompileTOLToBytecode(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}
	L := NewState()
	defer L.Close()
	if err := L.DoBytecode(bc); err != nil {
		t.Fatalf("DoBytecode failed: %v", err)
	}

	tos := L.GetGlobal("tos")
	oninvoke := L.GetField(tos, "oninvoke")
	if oninvoke == LNil {
		t.Fatalf("expected tos.oninvoke wrapper")
	}

	L.Push(oninvoke)
	L.Push(LString(selectorHexFromSignature("append(u256)")))
	L.Push(lNumberFromInt(7))
	if err := L.PCall(2, 0, nil); err != nil {
		t.Fatalf("oninvoke call failed: %v", err)
	}
	if got := LVAsString(L.GetGlobal("len_out")); got != "1" {
		t.Fatalf("unexpected len after first push: got=%s want=1", got)
	}

	L.Push(oninvoke)
	L.Push(LString(selectorHexFromSignature("append(u256)")))
	L.Push(lNumberFromInt(9))
	if err := L.PCall(2, 0, nil); err != nil {
		t.Fatalf("oninvoke call failed: %v", err)
	}
	if got := LVAsString(L.GetGlobal("len_out")); got != "2" {
		t.Fatalf("unexpected len after second push: got=%s want=2", got)
	}

	L.Push(oninvoke)
	L.Push(LString(selectorHexFromSignature("read(u256)")))
	L.Push(lNumberFromInt(1))
	if err := L.PCall(2, 0, nil); err != nil {
		t.Fatalf("oninvoke call failed: %v", err)
	}
	if got := LVAsString(L.GetGlobal("got")); got != "9" {
		t.Fatalf("unexpected array index result: got=%s want=9", got)
	}
}

func TestCompileTOLToBytecodeStorageNestedMappingSlot(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  storage {
    slot allowances: mapping(address => mapping(address => u256));
  }
  fn add(owner: address, spender: address, amount: u256) public {
    let cur: u256 = allowances[owner][spender];
    set allowances[owner][spender] = cur + amount;
    set got = allowances[owner][spender];
    return;
  }
}
`)
	bc, err := CompileTOLToBytecode(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}
	L := NewState()
	defer L.Close()
	if err := L.DoBytecode(bc); err != nil {
		t.Fatalf("DoBytecode failed: %v", err)
	}

	tos := L.GetGlobal("tos")
	oninvoke := L.GetField(tos, "oninvoke")
	if oninvoke == LNil {
		t.Fatalf("expected tos.oninvoke wrapper")
	}

	L.Push(oninvoke)
	L.Push(LString(selectorHexFromSignature("add(address,address,u256)")))
	L.Push(lNumberFromInt(1))
	L.Push(lNumberFromInt(2))
	L.Push(lNumberFromInt(3))
	if err := L.PCall(4, 0, nil); err != nil {
		t.Fatalf("oninvoke call failed: %v", err)
	}
	if got := LVAsString(L.GetGlobal("got")); got != "3" {
		t.Fatalf("unexpected first nested mapping result: got=%s want=3", got)
	}

	L.Push(oninvoke)
	L.Push(LString(selectorHexFromSignature("add(address,address,u256)")))
	L.Push(lNumberFromInt(1))
	L.Push(lNumberFromInt(2))
	L.Push(lNumberFromInt(4))
	if err := L.PCall(4, 0, nil); err != nil {
		t.Fatalf("oninvoke call failed: %v", err)
	}
	if got := LVAsString(L.GetGlobal("got")); got != "7" {
		t.Fatalf("unexpected second nested mapping result: got=%s want=7", got)
	}
}

func TestCompileTOLToBytecodeStorageNestedMappingRejectsPartialIndex(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  storage {
    slot allowances: mapping(address => mapping(address => u256));
  }
  fn bad(owner: address) public {
    set got = allowances[owner];
    return;
  }
}
`)
	_, err := CompileTOLToBytecode(src, "<tol>")
	if err == nil {
		t.Fatalf("expected compile error")
	}
	if !strings.Contains(err.Error(), "TOL2018") {
		t.Fatalf("expected TOL2018 error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "requires exactly 2 index key(s)") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompileTOLToBytecodeStorageRejectsSetArrayLengthTarget(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  storage {
    slot xs: u256[];
  }
  fn bad() public {
    set xs.length = 1;
    return;
  }
}
`)
	_, err := CompileTOLToBytecode(src, "<tol>")
	if err == nil {
		t.Fatalf("expected compile error")
	}
	if !strings.Contains(err.Error(), "TOL2018") {
		t.Fatalf("expected TOL2018 error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("unexpected error: %v", err)
	}
}
