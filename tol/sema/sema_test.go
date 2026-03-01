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

func TestCheckRejectsSetTargetReservedLiteralIdent(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind:   "set",
							Target: &ast.Expr{Kind: "ident", Value: "true"},
							Expr:   &ast.Expr{Kind: "number", Value: "1"},
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
	if !strings.Contains(diags.Error(), "TOL2008") {
		t.Fatalf("expected TOL2008, got: %v", diags)
	}
}

func TestCheckRejectsSetTargetSelectorMemberExpr(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "mark",
					Modifiers: []string{"public"},
				},
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "set",
							Target: &ast.Expr{
								Kind:   "member",
								Member: "selector",
								Object: &ast.Expr{
									Kind:   "member",
									Member: "mark",
									Object: &ast.Expr{
										Kind:  "ident",
										Value: "this",
									},
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
	if !strings.Contains(diags.Error(), "TOL2008") {
		t.Fatalf("expected TOL2008, got: %v", diags)
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

func TestCheckRejectsUnknownConstructorModifier(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Constructor: &ast.ConstructorDecl{
				Modifiers: []string{"onlyOwner"},
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

func TestCheckRejectsConflictingConstructorVisibility(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Constructor: &ast.ConstructorDecl{
				Modifiers: []string{"public", "internal"},
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

func TestCheckAcceptsConstructorPayableModifier(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Constructor: &ast.ConstructorDecl{
				Modifiers: []string{"payable"},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
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

func TestCheckRejectsNonVoidFunctionWithoutAnyReturnStmt(t *testing.T) {
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
						{
							Kind: "let",
							Name: "x",
							Type: "u256",
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

func TestCheckRejectsPartialNestedMappingIndex(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Storage: &ast.StorageDecl{
				Slots: []ast.StorageSlot{
					{Name: "allowances", Type: "mapping(address => mapping(address => u256))"},
				},
			},
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Params: []ast.FieldDecl{
						{Name: "owner", Type: "address"},
					},
					Body: []ast.Statement{
						{
							Kind:   "set",
							Target: &ast.Expr{Kind: "ident", Value: "got"},
							Expr: &ast.Expr{
								Kind: "index",
								Object: &ast.Expr{
									Kind:  "ident",
									Value: "allowances",
								},
								Index: &ast.Expr{Kind: "ident", Value: "owner"},
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
	if !strings.Contains(diags.Error(), "TOL2018") {
		t.Fatalf("expected TOL2018, got: %v", diags)
	}
}

func TestCheckRejectsStorageScalarIndexRead(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Storage: &ast.StorageDecl{
				Slots: []ast.StorageSlot{
					{Name: "total", Type: "u256"},
				},
			},
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Params: []ast.FieldDecl{
						{Name: "i", Type: "u256"},
					},
					Body: []ast.Statement{
						{
							Kind:   "set",
							Target: &ast.Expr{Kind: "ident", Value: "got"},
							Expr: &ast.Expr{
								Kind: "index",
								Object: &ast.Expr{
									Kind:  "ident",
									Value: "total",
								},
								Index: &ast.Expr{Kind: "ident", Value: "i"},
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
	if !strings.Contains(diags.Error(), "TOL2018") {
		t.Fatalf("expected TOL2018, got: %v", diags)
	}
}

func TestCheckAcceptsStorageArrayPushLengthAndIndex(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Storage: &ast.StorageDecl{
				Slots: []ast.StorageSlot{
					{Name: "xs", Type: "u256[]"},
				},
			},
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Params: []ast.FieldDecl{
						{Name: "v", Type: "u256"},
						{Name: "i", Type: "u256"},
					},
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:   "member",
									Member: "push",
									Object: &ast.Expr{
										Kind:  "ident",
										Value: "xs",
									},
								},
								Args: []*ast.Expr{
									{Kind: "ident", Value: "v"},
								},
							},
						},
						{
							Kind:   "set",
							Target: &ast.Expr{Kind: "ident", Value: "a"},
							Expr: &ast.Expr{
								Kind:   "member",
								Member: "length",
								Object: &ast.Expr{
									Kind:  "ident",
									Value: "xs",
								},
							},
						},
						{
							Kind:   "set",
							Target: &ast.Expr{Kind: "ident", Value: "b"},
							Expr: &ast.Expr{
								Kind: "index",
								Object: &ast.Expr{
									Kind:  "ident",
									Value: "xs",
								},
								Index: &ast.Expr{Kind: "ident", Value: "i"},
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

func TestCheckRejectsSetStorageArrayLengthTarget(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Storage: &ast.StorageDecl{
				Slots: []ast.StorageSlot{
					{Name: "xs", Type: "u256[]"},
				},
			},
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "set",
							Target: &ast.Expr{
								Kind:   "member",
								Member: "length",
								Object: &ast.Expr{
									Kind:  "ident",
									Value: "xs",
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
	if !strings.Contains(diags.Error(), "TOL2018") {
		t.Fatalf("expected TOL2018, got: %v", diags)
	}
}

func TestCheckRejectsFunctionCallArityMismatch(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "sum",
					Params: []ast.FieldDecl{
						{Name: "a", Type: "u256"},
						{Name: "b", Type: "u256"},
					},
					Body: []ast.Statement{{Kind: "return"}},
				},
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind:   "call",
								Callee: &ast.Expr{Kind: "ident", Value: "sum"},
								Args: []*ast.Expr{
									{Kind: "number", Value: "1"},
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
	if !strings.Contains(diags.Error(), "TOL2019") {
		t.Fatalf("expected TOL2019, got: %v", diags)
	}
}

func TestCheckAcceptsFunctionCallArityMatch(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "sum",
					Params: []ast.FieldDecl{
						{Name: "a", Type: "u256"},
						{Name: "b", Type: "u256"},
					},
					Body: []ast.Statement{{Kind: "return"}},
				},
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind:   "call",
								Callee: &ast.Expr{Kind: "ident", Value: "sum"},
								Args: []*ast.Expr{
									{Kind: "number", Value: "1"},
									{Kind: "number", Value: "2"},
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

func TestCheckRejectsInvalidAssignmentExprTarget(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "for",
							Init: &ast.Statement{
								Kind: "let",
								Name: "i",
								Type: "u256",
								Expr: &ast.Expr{Kind: "number", Value: "0"},
							},
							Cond: &ast.Expr{
								Kind:  "binary",
								Op:    "<",
								Left:  &ast.Expr{Kind: "ident", Value: "i"},
								Right: &ast.Expr{Kind: "number", Value: "3"},
							},
							Post: &ast.Expr{
								Kind: "assign",
								Op:   "=",
								Left: &ast.Expr{
									Kind:  "binary",
									Op:    "+",
									Left:  &ast.Expr{Kind: "ident", Value: "i"},
									Right: &ast.Expr{Kind: "number", Value: "1"},
								},
								Right: &ast.Expr{Kind: "number", Value: "1"},
							},
							Body: []ast.Statement{{Kind: "break"}},
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
	if !strings.Contains(diags.Error(), "TOL2008") {
		t.Fatalf("expected TOL2008, got: %v", diags)
	}
}

func TestCheckRejectsAssignExprInLetInitializer(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "let",
							Name: "x",
							Type: "u256",
							Expr: &ast.Expr{
								Kind: "assign",
								Op:   "=",
								Left: &ast.Expr{Kind: "ident", Value: "a"},
								Right: &ast.Expr{
									Kind:  "number",
									Value: "1",
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
	if !strings.Contains(diags.Error(), "TOL2020") {
		t.Fatalf("expected TOL2020, got: %v", diags)
	}
}

func TestCheckRejectsNonCallAssignExprStatement(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind:  "binary",
								Op:    "+",
								Left:  &ast.Expr{Kind: "number", Value: "1"},
								Right: &ast.Expr{Kind: "number", Value: "2"},
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
	if !strings.Contains(diags.Error(), "TOL2020") {
		t.Fatalf("expected TOL2020, got: %v", diags)
	}
}

func TestCheckRejectsForPostNonCallAssignExpr(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "for",
							Init: &ast.Statement{
								Kind: "let",
								Name: "i",
								Type: "u256",
								Expr: &ast.Expr{Kind: "number", Value: "0"},
							},
							Cond: &ast.Expr{
								Kind:  "binary",
								Op:    "<",
								Left:  &ast.Expr{Kind: "ident", Value: "i"},
								Right: &ast.Expr{Kind: "number", Value: "3"},
							},
							Post: &ast.Expr{
								Kind:  "binary",
								Op:    "+",
								Left:  &ast.Expr{Kind: "ident", Value: "i"},
								Right: &ast.Expr{Kind: "number", Value: "1"},
							},
							Body: []ast.Statement{{Kind: "break"}},
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
	if !strings.Contains(diags.Error(), "TOL2020") {
		t.Fatalf("expected TOL2020, got: %v", diags)
	}
}

func TestCheckAcceptsCallExprStatementAndForPostAssign(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "tick",
					Body: []ast.Statement{{Kind: "return"}},
				},
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind:   "call",
								Callee: &ast.Expr{Kind: "ident", Value: "tick"},
							},
						},
						{
							Kind: "for",
							Init: &ast.Statement{
								Kind: "let",
								Name: "i",
								Type: "u256",
								Expr: &ast.Expr{Kind: "number", Value: "0"},
							},
							Cond: &ast.Expr{
								Kind:  "binary",
								Op:    "<",
								Left:  &ast.Expr{Kind: "ident", Value: "i"},
								Right: &ast.Expr{Kind: "number", Value: "3"},
							},
							Post: &ast.Expr{
								Kind: "assign",
								Op:   "=",
								Left: &ast.Expr{Kind: "ident", Value: "i"},
								Right: &ast.Expr{
									Kind:  "binary",
									Op:    "+",
									Left:  &ast.Expr{Kind: "ident", Value: "i"},
									Right: &ast.Expr{Kind: "number", Value: "1"},
								},
							},
							Body: []ast.Statement{{Kind: "break"}},
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

func TestCheckRejectsRequireWithoutExpr(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{Kind: "require"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2021") {
		t.Fatalf("expected TOL2021, got: %v", diags)
	}
}

func TestCheckRejectsEmitNonCallExpr(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "emit",
							Expr: &ast.Expr{
								Kind:  "ident",
								Value: "x",
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
	if !strings.Contains(diags.Error(), "TOL2021") {
		t.Fatalf("expected TOL2021, got: %v", diags)
	}
}

func TestCheckRejectsEmitMemberCallPayload(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "emit",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:   "member",
									Member: "Tick",
									Object: &ast.Expr{
										Kind:  "ident",
										Value: "obj",
									},
								},
								Args: []*ast.Expr{
									{Kind: "number", Value: "1"},
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
	if !strings.Contains(diags.Error(), "TOL2021") {
		t.Fatalf("expected TOL2021, got: %v", diags)
	}
}

func TestCheckAcceptsEmitCallExpr(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "emit",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "Tick",
								},
								Args: []*ast.Expr{
									{Kind: "number", Value: "1"},
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

func TestCheckRejectsEmitDeclaredEventArityMismatch(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Events: []ast.EventDecl{
				{
					Name: "Tick",
					Params: []ast.FieldDecl{
						{Name: "a", Type: "u256"},
						{Name: "b", Type: "u256"},
					},
				},
			},
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "emit",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "Tick",
								},
								Args: []*ast.Expr{
									{Kind: "number", Value: "1"},
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
	if !strings.Contains(diags.Error(), "TOL2023") {
		t.Fatalf("expected TOL2023, got: %v", diags)
	}
}

func TestCheckAcceptsEmitDeclaredEventArityMatch(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Events: []ast.EventDecl{
				{
					Name: "Tick",
					Params: []ast.FieldDecl{
						{Name: "a", Type: "u256"},
						{Name: "b", Type: "u256"},
					},
				},
			},
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "emit",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "Tick",
								},
								Args: []*ast.Expr{
									{Kind: "number", Value: "1"},
									{Kind: "number", Value: "2"},
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

func TestCheckRejectsDuplicateEventDeclarations(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Events: []ast.EventDecl{
				{Name: "Tick", Params: []ast.FieldDecl{{Name: "a", Type: "u256"}}},
				{Name: "Tick", Params: []ast.FieldDecl{{Name: "b", Type: "u256"}}},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2024") {
		t.Fatalf("expected TOL2024, got: %v", diags)
	}
}

func TestCheckRejectsEmitUnknownDeclaredEventSet(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Events: []ast.EventDecl{
				{Name: "Tick", Params: []ast.FieldDecl{{Name: "a", Type: "u256"}}},
			},
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "emit",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "Other",
								},
								Args: []*ast.Expr{
									{Kind: "number", Value: "1"},
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
	if !strings.Contains(diags.Error(), "TOL2025") {
		t.Fatalf("expected TOL2025, got: %v", diags)
	}
}

func TestCheckAllowsEmitWhenNoEventsDeclared(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "emit",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "Tick",
								},
								Args: []*ast.Expr{
									{Kind: "number", Value: "1"},
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

func TestCheckRejectsRevertNonStringPayload(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "revert",
							Expr: &ast.Expr{
								Kind:  "ident",
								Value: "err",
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
	if !strings.Contains(diags.Error(), "TOL2022") {
		t.Fatalf("expected TOL2022, got: %v", diags)
	}
}

func TestCheckAcceptsRevertStringOrEmpty(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "a",
					Body: []ast.Statement{
						{Kind: "revert"},
					},
				},
				{
					Name: "b",
					Body: []ast.Statement{
						{
							Kind: "revert",
							Expr: &ast.Expr{
								Kind:  "string",
								Value: "\"ERR\"",
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

func TestCheckRejectsNestedAssignInExprStatementCallArg(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "foo",
								},
								Args: []*ast.Expr{
									{
										Kind: "assign",
										Op:   "=",
										Left: &ast.Expr{Kind: "ident", Value: "x"},
										Right: &ast.Expr{
											Kind:  "number",
											Value: "1",
										},
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
	if !strings.Contains(diags.Error(), "TOL2020") {
		t.Fatalf("expected TOL2020, got: %v", diags)
	}
}

func TestCheckRejectsNestedAssignInForPostCallArg(t *testing.T) {
	m := &ast.Module{
		Version: "0.2",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "for",
							Init: &ast.Statement{
								Kind: "let",
								Name: "i",
								Type: "u256",
								Expr: &ast.Expr{Kind: "number", Value: "0"},
							},
							Cond: &ast.Expr{
								Kind:  "binary",
								Op:    "<",
								Left:  &ast.Expr{Kind: "ident", Value: "i"},
								Right: &ast.Expr{Kind: "number", Value: "3"},
							},
							Post: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "tick",
								},
								Args: []*ast.Expr{
									{
										Kind: "assign",
										Op:   "=",
										Left: &ast.Expr{Kind: "ident", Value: "i"},
										Right: &ast.Expr{
											Kind:  "number",
											Value: "1",
										},
									},
								},
							},
							Body: []ast.Statement{{Kind: "break"}},
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
	if !strings.Contains(diags.Error(), "TOL2020") {
		t.Fatalf("expected TOL2020, got: %v", diags)
	}
}
