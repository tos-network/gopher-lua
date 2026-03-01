package main

import (
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
