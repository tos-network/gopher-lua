package lower

import (
	"fmt"
	"strings"

	"github.com/tos-network/tolang/tol/ast"
	"github.com/tos-network/tolang/tol/diag"
	"github.com/tos-network/tolang/tol/sema"
)

// Program is the backend-agnostic lowered form.
type Program struct {
	ContractName      string
	StorageSlots      []StorageSlot
	Functions         []Function
	HasConstructor    bool
	ConstructorParams []ast.FieldDecl
	ConstructorBody   []ast.Statement
	HasFallback       bool
	FallbackBody      []ast.Statement
}

type StorageSlot struct {
	Name string
	Type string
}

type Function struct {
	Name             string
	SelectorOverride string
	Params           []ast.FieldDecl
	Returns          []ast.FieldDecl
	Modifiers        []string
	Body             []ast.Statement
}

func FromTyped(typed *sema.TypedModule) (*Program, error) {
	if typed == nil || typed.AST == nil || typed.AST.Contract == nil {
		return nil, fmt.Errorf("[%s] invalid typed module", diag.CodeLowerNotImplemented)
	}

	c := typed.AST.Contract
	out := &Program{
		ContractName: c.Name,
	}
	if c.Storage != nil {
		out.StorageSlots = make([]StorageSlot, 0, len(c.Storage.Slots))
		for _, s := range c.Storage.Slots {
			out.StorageSlots = append(out.StorageSlots, StorageSlot{
				Name: s.Name,
				Type: normalizeType(s.Type),
			})
		}
	}

	out.Functions = make([]Function, 0, len(c.Functions))
	for _, fn := range c.Functions {
		out.Functions = append(out.Functions, Function{
			Name:             fn.Name,
			SelectorOverride: fn.SelectorOverride,
			Params:           cloneFields(fn.Params),
			Returns:          cloneFields(fn.Returns),
			Modifiers:        cloneStrings(fn.Modifiers),
			Body:             cloneStatements(fn.Body),
		})
	}
	out.HasConstructor = c.Constructor != nil
	if c.Constructor != nil {
		out.ConstructorParams = cloneFields(c.Constructor.Params)
		out.ConstructorBody = cloneStatements(c.Constructor.Body)
	}
	out.HasFallback = c.Fallback != nil
	if c.Fallback != nil {
		out.FallbackBody = cloneStatements(c.Fallback.Body)
	}
	return out, nil
}

func cloneFields(in []ast.FieldDecl) []ast.FieldDecl {
	if len(in) == 0 {
		return nil
	}
	out := make([]ast.FieldDecl, len(in))
	copy(out, in)
	return out
}

func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneStatements(in []ast.Statement) []ast.Statement {
	if len(in) == 0 {
		return nil
	}
	out := make([]ast.Statement, len(in))
	copy(out, in)
	return out
}

func normalizeType(t string) string {
	return strings.Join(strings.Fields(t), " ")
}
