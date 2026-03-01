package main

import "testing"

func TestDetectRemovedTOLFlag(t *testing.T) {
	flagName, hint, ok := detectRemovedTOLFlag([]string{"-ctoc", "out.toc", "in.tol"})
	if !ok {
		t.Fatalf("expected removed flag detection")
	}
	if flagName != "-ctoc" {
		t.Fatalf("unexpected removed flag: %s", flagName)
	}
	if hint == "" {
		t.Fatalf("expected migration hint")
	}
}

func TestDetectRemovedTOLFlagWithEqualsSyntax(t *testing.T) {
	flagName, hint, ok := detectRemovedTOLFlag([]string{"-ctorpkg=demo", "-ctor", "out.tor", "in.tol"})
	if !ok {
		t.Fatalf("expected removed flag detection for equals syntax")
	}
	if flagName != "-ctorpkg" {
		t.Fatalf("unexpected removed flag: %s", flagName)
	}
	if hint == "" {
		t.Fatalf("expected migration hint")
	}
}

func TestDetectRemovedTOLFlagIgnoresCurrentArgs(t *testing.T) {
	if _, _, ok := detectRemovedTOLFlag([]string{"compile", "--emit", "toc", "in.tol"}); ok {
		t.Fatalf("did not expect removed flag detection for current subcommand args")
	}
	if _, _, ok := detectRemovedTOLFlag([]string{"-e", "x=1"}); ok {
		t.Fatalf("did not expect removed flag detection for lua flags")
	}
}
