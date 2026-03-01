package lua

import (
	"bytes"
	"strings"
	"testing"
)

func TestCompileTOLToTOIExportsPublicSurface(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  event Tick(v: u256 indexed);

  fn hidden(x: u256) internal {
    return;
  }

  @selector("0x12345678")
  fn ping(owner: address, amount: u256) -> (ok: bool) public view {
    return;
  }

  fn poke() external {
    return;
  }
}
`)
	toi, err := CompileTOLToTOI(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected toi compile error: %v", err)
	}
	out := string(toi)
	if !strings.Contains(out, "interface IDemo") {
		t.Fatalf("expected interface header, got:\n%s", out)
	}
	if strings.Contains(out, "fn hidden(") {
		t.Fatalf("internal function should not be exported, got:\n%s", out)
	}
	if !strings.Contains(out, `@selector("0x12345678")`) {
		t.Fatalf("expected selector override in toi, got:\n%s", out)
	}
	if !strings.Contains(out, "fn ping(owner: address, amount: u256) -> (ok: bool) public view;") {
		t.Fatalf("expected exported ping signature, got:\n%s", out)
	}
	if !strings.Contains(out, "fn poke() external;") {
		t.Fatalf("expected exported poke signature, got:\n%s", out)
	}
	if !strings.Contains(out, "event Tick(v: u256 indexed);") {
		t.Fatalf("expected event signature, got:\n%s", out)
	}
}

func TestCompileTOLToTOIDeterministic(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn ping() public { return; }
}
`)
	a, err := CompileTOLToTOI(src, "<tol>")
	if err != nil {
		t.Fatalf("compile a: %v", err)
	}
	b, err := CompileTOLToTOI(src, "<tol>")
	if err != nil {
		t.Fatalf("compile b: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("expected deterministic toi output")
	}
}

func TestBuildTOIFromModuleRejectsNil(t *testing.T) {
	if _, err := BuildTOIFromModule(nil); err == nil {
		t.Fatalf("expected nil module error")
	}
}

func TestCompileTOLToTOIWithCustomInterfaceName(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn ping() public { return; }
}
`)
	toi, err := CompileTOLToTOIWithOptions(src, "<tol>", &TOICompileOptions{
		InterfaceName: "DemoSurface",
	})
	if err != nil {
		t.Fatalf("unexpected toi compile error: %v", err)
	}
	if !strings.Contains(string(toi), "interface DemoSurface {") {
		t.Fatalf("expected custom interface name, got:\n%s", string(toi))
	}
}

func TestValidateTOITextAcceptsGeneratedTOI(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn ping() public { return; }
}
`)
	toi, err := CompileTOLToTOI(src, "<tol>")
	if err != nil {
		t.Fatalf("compile toi: %v", err)
	}
	if err := ValidateTOIText(toi); err != nil {
		t.Fatalf("expected valid toi text, got: %v", err)
	}
}

func TestValidateTOITextRejectsMalformed(t *testing.T) {
	if err := ValidateTOIText([]byte("not toi")); err == nil {
		t.Fatalf("expected malformed toi error")
	}
}

func TestValidateTOITextRejectsMissingSemicolon(t *testing.T) {
	toi := []byte(`
tol 0.2

interface ISample {
  fn ping() public
}
`)
	if err := ValidateTOIText(toi); err == nil {
		t.Fatalf("expected missing semicolon error")
	}
}

func TestValidateTOITextRejectsDanglingSelector(t *testing.T) {
	toi := []byte(`
tol 0.2

interface ISample {
  @selector("0x12345678")
  event Tick(v: u256);
}
`)
	if err := ValidateTOIText(toi); err == nil {
		t.Fatalf("expected dangling selector error")
	}
}

func TestValidateTOITextRejectsMultipleInterfaces(t *testing.T) {
	toi := []byte(`
tol 0.2

interface IA {
  fn a() public;
}

interface IB {
  fn b() public;
}
`)
	if err := ValidateTOIText(toi); err == nil {
		t.Fatalf("expected multiple interface error")
	}
}

func TestValidateTOITextAcceptsComments(t *testing.T) {
	toi := []byte(`
-- file comment
tol 0.2

interface ISample { -- inline
  fn ping() public; -- fn
  event Tick(v: u256); -- event
}
`)
	if err := ValidateTOIText(toi); err != nil {
		t.Fatalf("expected comments to be accepted: %v", err)
	}
}

func TestInspectTOIText(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  event Tick(v: u256);
  fn ping() public { return; }
}
`)
	toi, err := CompileTOLToTOI(src, "<tol>")
	if err != nil {
		t.Fatalf("compile toi: %v", err)
	}
	info, err := InspectTOIText(toi)
	if err != nil {
		t.Fatalf("inspect toi: %v", err)
	}
	if info.Version != "0.2" {
		t.Fatalf("unexpected version: %s", info.Version)
	}
	if info.InterfaceName != "IDemo" {
		t.Fatalf("unexpected interface: %s", info.InterfaceName)
	}
	if info.FunctionCount != 1 || info.EventCount != 1 {
		t.Fatalf("unexpected counts: %+v", info)
	}
}
