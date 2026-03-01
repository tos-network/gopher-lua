package main

import (
	"os"
	"path/filepath"
	"testing"

	lua "github.com/tos-network/tolang"
)

func TestDefaultArtifactPath(t *testing.T) {
	input := "/tmp/contract.tol"
	if got, want := defaultArtifactPath(input, "toc"), "/tmp/contract.toc"; got != want {
		t.Fatalf("defaultArtifactPath toc: got=%q want=%q", got, want)
	}
	if got, want := defaultArtifactPath(input, "toi"), "/tmp/contract.toi"; got != want {
		t.Fatalf("defaultArtifactPath toi: got=%q want=%q", got, want)
	}
	if got, want := defaultArtifactPath(input, "tor"), "/tmp/contract.tor"; got != want {
		t.Fatalf("defaultArtifactPath tor: got=%q want=%q", got, want)
	}
}

func TestDetectArtifactKindMagicFallback(t *testing.T) {
	src := []byte("tol 0.2\n\ncontract Sample {\n  fn ping() public {\n  }\n}\n")
	toc, err := lua.CompileTOLToTOC(src, "sample.tol")
	if err != nil {
		t.Fatalf("compile toc: %v", err)
	}
	if got := detectArtifactKind("artifact.bin", toc); got != artifactTOC {
		t.Fatalf("detect toc by magic: got=%v want=%v", got, artifactTOC)
	}

	toi, err := lua.CompileTOLToTOI(src, "sample.tol")
	if err != nil {
		t.Fatalf("compile toi: %v", err)
	}
	if got := detectArtifactKind("artifact.bin", toi); got != artifactTOI {
		t.Fatalf("detect toi by text validation: got=%v want=%v", got, artifactTOI)
	}
}

func TestCmdCompileDefaultTOCOutput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "sample.tol")
	if err := os.WriteFile(input, []byte("tol 0.2\n\ncontract Sample {\n  fn ping() public {\n  }\n}\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	if code := cmdCompile([]string{input}); code != 0 {
		t.Fatalf("cmdCompile exit code: got=%d want=0", code)
	}

	out := filepath.Join(dir, "sample.toc")
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output toc: %v", err)
	}
	if _, err := lua.DecodeTOC(body); err != nil {
		t.Fatalf("decode output toc: %v", err)
	}
}

func TestCmdVerifyTOCSourceMismatchExitCode(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "sample.tol")
	if err := os.WriteFile(srcPath, []byte("tol 0.2\n\ncontract Sample {\n  fn ping() public {\n  }\n}\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	tocPath := filepath.Join(dir, "sample.toc")
	if code := cmdCompile([]string{"-o", tocPath, srcPath}); code != 0 {
		t.Fatalf("compile toc exit code: got=%d want=0", code)
	}

	mismatchPath := filepath.Join(dir, "mismatch.tol")
	if err := os.WriteFile(mismatchPath, []byte("tol 0.2\n\ncontract Sample {\n  fn pong() public {\n  }\n}\n"), 0o644); err != nil {
		t.Fatalf("write mismatch source: %v", err)
	}

	if code := cmdVerify([]string{"--source", mismatchPath, tocPath}); code != 2 {
		t.Fatalf("verify mismatch exit code: got=%d want=2", code)
	}
}
