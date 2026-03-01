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
