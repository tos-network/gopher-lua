package codegen

import (
	"testing"

	"github.com/tos-network/tolang/tol/ast"
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

func TestBytecodeSupportsStorageInDirectIR(t *testing.T) {
	p := &lower.Program{
		ContractName: "Demo",
		StorageSlots: []lower.StorageSlot{
			{Name: "x", Type: "u256"},
		},
		Functions: []lower.Function{
			{
				Name:      "setx",
				Modifiers: []string{"public"},
				Params: []ast.FieldDecl{
					{Name: "v", Type: "u256"},
				},
				Body: []ast.Statement{
					{
						Kind: "set",
						Target: &ast.Expr{
							Kind:  "ident",
							Value: "x",
						},
						Expr: &ast.Expr{
							Kind:  "ident",
							Value: "v",
						},
					},
					{Kind: "return"},
				},
			},
		},
	}
	bc, err := Bytecode(p)
	if err != nil {
		t.Fatalf("unexpected codegen error: %v", err)
	}
	if len(bc) == 0 {
		t.Fatalf("expected non-empty bytecode")
	}
}
