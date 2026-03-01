package main

import (
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"testing"

	lua "github.com/tos-network/tolang"
)

func runMainAuxWithArgs(t *testing.T, args ...string) int {
	t.Helper()

	oldArgs := os.Args
	oldFlagSet := flag.CommandLine

	os.Args = append([]string{"tol"}, args...)
	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	flag.CommandLine = fs

	defer func() {
		os.Args = oldArgs
		flag.CommandLine = oldFlagSet
	}()

	return mainAux()
}

func TestLegacyFlatFlagsTOIAndTOCStillWork(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "sample.tol")
	src := []byte("tol 0.2\n\ncontract Sample {\n  fn ping() public {\n  }\n}\n")
	if err := os.WriteFile(srcPath, src, 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	toiPath := filepath.Join(dir, "sample.toi")
	if code := runMainAuxWithArgs(t, "-ctoi", toiPath, srcPath); code != 0 {
		t.Fatalf("legacy -ctoi failed: got=%d want=0", code)
	}
	toi, err := os.ReadFile(toiPath)
	if err != nil {
		t.Fatalf("read toi: %v", err)
	}
	if err := lua.ValidateTOIText(toi); err != nil {
		t.Fatalf("validate toi: %v", err)
	}

	if code := runMainAuxWithArgs(t, "-vtoi", toiPath); code != 0 {
		t.Fatalf("legacy -vtoi failed: got=%d want=0", code)
	}

	tocPath := filepath.Join(dir, "sample.toc")
	if code := runMainAuxWithArgs(t, "-ctoc", tocPath, srcPath); code != 0 {
		t.Fatalf("legacy -ctoc failed: got=%d want=0", code)
	}
	toc, err := os.ReadFile(tocPath)
	if err != nil {
		t.Fatalf("read toc: %v", err)
	}
	if _, err := lua.DecodeTOC(toc); err != nil {
		t.Fatalf("decode toc: %v", err)
	}

	if code := runMainAuxWithArgs(t, "-vtoc", tocPath); code != 0 {
		t.Fatalf("legacy -vtoc failed: got=%d want=0", code)
	}
	if code := runMainAuxWithArgs(t, "-vtoc", "-vtocsrc", srcPath, tocPath); code != 0 {
		t.Fatalf("legacy -vtoc -vtocsrc failed: got=%d want=0", code)
	}
}

func TestLegacyVTOCSourceMismatchStillFails(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "sample.tol")
	src := []byte("tol 0.2\n\ncontract Sample {\n  fn ping() public {\n  }\n}\n")
	if err := os.WriteFile(srcPath, src, 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	tocPath := filepath.Join(dir, "sample.toc")
	if code := runMainAuxWithArgs(t, "-ctoc", tocPath, srcPath); code != 0 {
		t.Fatalf("legacy -ctoc failed: got=%d want=0", code)
	}

	mismatchPath := filepath.Join(dir, "mismatch.tol")
	mismatch := []byte("tol 0.2\n\ncontract Sample {\n  fn pong() public {\n  }\n}\n")
	if err := os.WriteFile(mismatchPath, mismatch, 0o644); err != nil {
		t.Fatalf("write mismatch source: %v", err)
	}

	if code := runMainAuxWithArgs(t, "-vtoc", "-vtocsrc", mismatchPath, tocPath); code != 1 {
		t.Fatalf("legacy -vtoc mismatch should fail: got=%d want=1", code)
	}
}

func TestLegacyCTORFromDirAndOneShotStillWork(t *testing.T) {
	dir := t.TempDir()

	// Build minimal package dir for legacy `-ctor out.tor <dir>`.
	pkgDir := filepath.Join(dir, "pkg")
	if err := os.MkdirAll(filepath.Join(pkgDir, "bytecode"), 0o755); err != nil {
		t.Fatalf("mkdir bytecode: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(pkgDir, "interfaces"), 0o755); err != nil {
		t.Fatalf("mkdir interfaces: %v", err)
	}
	src := []byte("tol 0.2\n\ncontract Sample {\n  fn ping() public {\n  }\n}\n")
	toc, err := lua.CompileTOLToTOC(src, "sample.tol")
	if err != nil {
		t.Fatalf("compile toc: %v", err)
	}
	toi, err := lua.CompileTOLToTOI(src, "sample.tol")
	if err != nil {
		t.Fatalf("compile toi: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "bytecode", "Sample.toc"), toc, 0o644); err != nil {
		t.Fatalf("write toc: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "interfaces", "ISample.toi"), toi, 0o644); err != nil {
		t.Fatalf("write toi: %v", err)
	}
	manifest := []byte(`{
  "name":"legacy-pack",
  "version":"1.0.0",
  "contracts":[{"name":"Sample","toc":"bytecode/Sample.toc","toi":"interfaces/ISample.toi"}]
}`)
	if err := os.WriteFile(filepath.Join(pkgDir, "manifest.json"), manifest, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	outDirTOR := filepath.Join(dir, "from-dir.tor")
	if code := runMainAuxWithArgs(t, "-ctor", outDirTOR, pkgDir); code != 0 {
		t.Fatalf("legacy -ctor from dir failed: got=%d want=0", code)
	}
	torBody, err := os.ReadFile(outDirTOR)
	if err != nil {
		t.Fatalf("read tor from dir: %v", err)
	}
	if _, err := lua.DecodeTOR(torBody); err != nil {
		t.Fatalf("decode tor from dir: %v", err)
	}

	// Legacy one-shot `-ctor out.tor in.tol` with override flags.
	srcPath := filepath.Join(dir, "sample.tol")
	if err := os.WriteFile(srcPath, src, 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	outOneShotTOR := filepath.Join(dir, "oneshot.tor")
	if code := runMainAuxWithArgs(
		t,
		"-ctorpkg", "legacy-oneshot",
		"-ctorver", "2.0.0",
		"-ctorifacename", "ISampleLegacy",
		"-ctorsrc",
		"-ctor", outOneShotTOR,
		srcPath,
	); code != 0 {
		t.Fatalf("legacy -ctor one-shot failed: got=%d want=0", code)
	}
	oneShotBody, err := os.ReadFile(outOneShotTOR)
	if err != nil {
		t.Fatalf("read one-shot tor: %v", err)
	}
	oneShot, err := lua.DecodeTOR(oneShotBody)
	if err != nil {
		t.Fatalf("decode one-shot tor: %v", err)
	}
	var m struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(oneShot.ManifestJSON, &m); err != nil {
		t.Fatalf("decode one-shot manifest: %v", err)
	}
	if m.Name != "legacy-oneshot" || m.Version != "2.0.0" {
		t.Fatalf("one-shot manifest overrides not applied: %+v", m)
	}
}
