package lua

import (
	"github.com/tos-network/tolang/tol/ast"
	"github.com/tos-network/tolang/tol/lower"
	"github.com/tos-network/tolang/tol/parser"
	"github.com/tos-network/tolang/tol/sema"
)

// ParseTOLModule parses TOL source into a syntax tree.
func ParseTOLModule(source []byte, name string) (*ast.Module, error) {
	mod, diags := parser.ParseFile(name, source)
	if diags.HasErrors() {
		return nil, diags
	}
	return mod, nil
}

// BuildIRFromTOL parses and type-checks TOL source and prepares lowering to VM IR.
func BuildIRFromTOL(source []byte, name string) (*IRProgram, error) {
	mod, err := ParseTOLModule(source, name)
	if err != nil {
		return nil, err
	}
	typed, diags := sema.Check(name, mod)
	if diags.HasErrors() {
		return nil, diags
	}
	prog, err := lower.FromTyped(typed)
	if err != nil {
		return nil, err
	}
	return BuildIRFromLoweredTOL(prog, name)
}

// CompileTOLToBytecode compiles TOL source into deterministic bytecode.
func CompileTOLToBytecode(source []byte, name string) ([]byte, error) {
	irp, err := BuildIRFromTOL(source, name)
	if err != nil {
		return nil, err
	}
	proto, err := CompileIR(irp)
	if err != nil {
		return nil, err
	}
	return EncodeFunctionProto(proto)
}

// BuildIRFromLoweredTOL lowers a typed/lowered TOL program directly into VM IR.
func BuildIRFromLoweredTOL(prog *lower.Program, name string) (*IRProgram, error) {
	return buildDirectIRFromLowered(prog, name)
}

// CompileLoweredTOLToBytecode compiles a lowered TOL program into deterministic bytecode.
func CompileLoweredTOLToBytecode(prog *lower.Program, name string) ([]byte, error) {
	irp, err := BuildIRFromLoweredTOL(prog, name)
	if err != nil {
		return nil, err
	}
	proto, err := CompileIR(irp)
	if err != nil {
		return nil, err
	}
	return EncodeFunctionProto(proto)
}

// BuildLoweredTOL builds typed and lowered TOL program for diagnostics/testing.
func BuildLoweredTOL(source []byte, name string) (*lower.Program, error) {
	mod, err := ParseTOLModule(source, name)
	if err != nil {
		return nil, err
	}
	typed, diags := sema.Check(name, mod)
	if diags.HasErrors() {
		return nil, diags
	}
	return lower.FromTyped(typed)
}
