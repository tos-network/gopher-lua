package sema

import (
	"strings"
	"testing"

	"github.com/tos-network/tolang/tol/ast"
)

func TestCheckMinimal(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
		},
	}
	typed, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if typed == nil || typed.AST == nil {
		t.Fatalf("expected typed module")
	}
}

func TestCheckAllowsConstructorParams(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Constructor: &ast.ConstructorDecl{
				Params: []ast.FieldDecl{
					{Name: "owner", Type: "address"},
				},
			},
		},
	}
	typed, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if typed == nil || typed.AST == nil || typed.AST.Contract == nil {
		t.Fatalf("expected typed module")
	}
}

func TestCheckRejectsDuplicates(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Storage: &ast.StorageDecl{
				Slots: []ast.StorageSlot{
					{Name: "x", Type: "u256"},
					{Name: "x", Type: "u256"},
				},
			},
			Functions: []ast.FunctionDecl{
				{Name: "transfer"},
				{Name: "transfer"},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
}

func TestCheckBreakContinueOutsideLoop(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{Kind: "break"},
						{Kind: "continue"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
}

func TestCheckSetTargetMustBeAssignable(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind: "set",
							Target: &ast.Expr{
								Kind: "binary",
								Op:   "+",
								Left: &ast.Expr{Kind: "ident", Value: "a"},
								Right: &ast.Expr{
									Kind:  "ident",
									Value: "b",
								},
							},
							Expr: &ast.Expr{Kind: "number", Value: "1"},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
}

func TestCheckRejectsInvalidSelectorOverride(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:             "f",
					SelectorOverride: "0x123",
					Modifiers:        []string{"public"},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2010") {
		t.Fatalf("expected TOL2010, got: %v", diags)
	}
}

func TestCheckRejectsDuplicatePublicExternalSelector(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:             "a",
					SelectorOverride: "0x11111111",
					Modifiers:        []string{"public"},
				},
				{
					Name:             "b",
					SelectorOverride: "0x11111111",
					Modifiers:        []string{"external"},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2011") {
		t.Fatalf("expected TOL2011, got: %v", diags)
	}
}

func TestCheckRejectsSelectorBuiltinNonLiteral(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind: "set",
							Target: &ast.Expr{
								Kind:  "ident",
								Value: "x",
							},
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "selector",
								},
								Args: []*ast.Expr{
									{Kind: "ident", Value: "sig"},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2012") {
		t.Fatalf("expected TOL2012, got: %v", diags)
	}
}

func TestCheckRejectsSelectorMemberUnknownTarget(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind:   "set",
							Target: &ast.Expr{Kind: "ident", Value: "x"},
							Expr: &ast.Expr{
								Kind:   "member",
								Member: "selector",
								Object: &ast.Expr{
									Kind:   "member",
									Member: "missing",
									Object: &ast.Expr{
										Kind:  "ident",
										Value: "this",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2013") {
		t.Fatalf("expected TOL2013, got: %v", diags)
	}
}

func TestCheckRejectsSelectorMemberNonExternalTarget(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "hidden",
					Modifiers: []string{"internal"},
				},
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind:   "set",
							Target: &ast.Expr{Kind: "ident", Value: "x"},
							Expr: &ast.Expr{
								Kind:   "member",
								Member: "selector",
								Object: &ast.Expr{
									Kind:   "member",
									Member: "hidden",
									Object: &ast.Expr{
										Kind:  "ident",
										Value: "Demo",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2013") {
		t.Fatalf("expected TOL2013, got: %v", diags)
	}
}

func TestCheckAcceptsSelectorMemberExternalTarget(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "pub",
					Modifiers: []string{"public"},
				},
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind:   "set",
							Target: &ast.Expr{Kind: "ident", Value: "x"},
							Expr: &ast.Expr{
								Kind:   "member",
								Member: "selector",
								Object: &ast.Expr{
									Kind:   "member",
									Member: "pub",
									Object: &ast.Expr{
										Kind:  "ident",
										Value: "this",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckRejectsUnknownFunctionModifier(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "f",
					Modifiers: []string{"onlyOwner"},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2014") {
		t.Fatalf("expected TOL2014, got: %v", diags)
	}
}

func TestCheckRejectsConflictingVisibilityModifiers(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "f",
					Modifiers: []string{"public", "external"},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2015") {
		t.Fatalf("expected TOL2015, got: %v", diags)
	}
}

func TestCheckRejectsConflictingMutabilityModifiers(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "f",
					Modifiers: []string{"view", "payable"},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2015") {
		t.Fatalf("expected TOL2015, got: %v", diags)
	}
}

func TestCheckRejectsDuplicateFunctionParams(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Params: []ast.FieldDecl{
						{Name: "x", Type: "u256"},
						{Name: "x", Type: "u256"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2016") {
		t.Fatalf("expected TOL2016, got: %v", diags)
	}
}

func TestCheckRejectsDuplicateConstructorParams(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Constructor: &ast.ConstructorDecl{
				Params: []ast.FieldDecl{
					{Name: "owner", Type: "address"},
					{Name: "owner", Type: "address"},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2016") {
		t.Fatalf("expected TOL2016, got: %v", diags)
	}
}

func TestCheckRejectsReturnValueInVoidFunction(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind: "return",
							Expr: &ast.Expr{Kind: "number", Value: "1"},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2017") {
		t.Fatalf("expected TOL2017, got: %v", diags)
	}
}

func TestCheckRejectsMissingReturnValueInNonVoidFunction(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Returns: []ast.FieldDecl{
						{Name: "ok", Type: "bool"},
					},
					Body: []ast.Statement{
						{Kind: "return"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2017") {
		t.Fatalf("expected TOL2017, got: %v", diags)
	}
}

func TestCheckRejectsConstructorReturnValue(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Constructor: &ast.ConstructorDecl{
				Body: []ast.Statement{
					{
						Kind: "return",
						Expr: &ast.Expr{Kind: "number", Value: "1"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2017") {
		t.Fatalf("expected TOL2017, got: %v", diags)
	}
}

func TestCheckRejectsFallbackReturnValue(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Fallback: &ast.FallbackDecl{
				Body: []ast.Statement{
					{
						Kind: "return",
						Expr: &ast.Expr{Kind: "number", Value: "1"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2017") {
		t.Fatalf("expected TOL2017, got: %v", diags)
	}
}
