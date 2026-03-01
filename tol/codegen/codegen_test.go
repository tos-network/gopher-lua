package codegen

import (
	"strings"
	"testing"

	"github.com/tos-network/tolang/tol/lower"
)

func TestBytecodeMinimalLoweredProgram(t *testing.T) {
	p := &lower.Program{
		ContractName: "Demo",
	}
	bc, err := Bytecode(p)
	if err != nil {
		t.Fatalf("unexpected codegen error: %v", err)
	}
	if len(bc) == 0 {
		t.Fatalf("expected non-empty bytecode")
	}
}

func TestBytecodeRejectsUnsupportedStorageInCurrentStage(t *testing.T) {
	p := &lower.Program{
		ContractName: "Demo",
		StorageSlots: []lower.StorageSlot{
			{Name: "x", Type: "u256"},
		},
	}
	_, err := Bytecode(p)
	if err == nil {
		t.Fatalf("expected codegen error")
	}
	if !strings.Contains(err.Error(), "TOL3002") {
		t.Fatalf("expected TOL3002 error, got: %v", err)
	}
}
