package ast

import "fmt"

// Module is the root node for a TOL source file.
type Module struct {
	Version         string
	SkippedTopDecls []SkippedTopDecl
	Contract        *ContractDecl
}

type SkippedTopDecl struct {
	Kind string
	Name string
}

// ContractDecl is a contract declaration node.
type ContractDecl struct {
	Name         string
	SkippedDecls []SkippedContractDecl
	Storage      *StorageDecl
	Events       []EventDecl
	Functions    []FunctionDecl
	Constructor  *ConstructorDecl
	Fallback     *FallbackDecl
}

type SkippedContractDecl struct {
	Kind string
	Name string
}

type StorageDecl struct {
	Slots []StorageSlot
}

type StorageSlot struct {
	Name string
	Type string
}

type EventDecl struct {
	Name   string
	Params []FieldDecl
}

type FunctionDecl struct {
	Name             string
	SelectorOverride string
	Params           []FieldDecl
	Returns          []FieldDecl
	Modifiers        []string
	Body             []Statement
}

type ConstructorDecl struct {
	Params    []FieldDecl
	Modifiers []string
	Body      []Statement
}

type FallbackDecl struct {
	Body []Statement
}

type FieldDecl struct {
	Name    string
	Type    string
	Indexed bool
}

type Statement struct {
	Kind   string
	Name   string
	Type   string
	Text   string
	Expr   *Expr
	Target *Expr
	Cond   *Expr
	Init   *Statement
	Post   *Expr
	Then   []Statement
	Else   []Statement
	Body   []Statement
}

type Expr struct {
	Kind   string
	Value  string
	Op     string
	Left   *Expr
	Right  *Expr
	Callee *Expr
	Args   []*Expr
	Object *Expr
	Member string
	Index  *Expr
}

func (m *Module) String() string {
	if m == nil {
		return "<nil>"
	}
	if m.Contract == nil {
		return fmt.Sprintf("tol %s\n<no contract>", m.Version)
	}
	out := fmt.Sprintf("tol %s\n", m.Version)
	for _, d := range m.SkippedTopDecls {
		out += fmt.Sprintf("%s %s { ... }\n", d.Kind, d.Name)
	}
	out += fmt.Sprintf("contract %s {\n", m.Contract.Name)

	if m.Contract.Storage != nil {
		out += "  storage {\n"
		for _, slot := range m.Contract.Storage.Slots {
			out += fmt.Sprintf("    slot %s: %s;\n", slot.Name, slot.Type)
		}
		out += "  }\n"
	}

	for _, d := range m.Contract.SkippedDecls {
		out += fmt.Sprintf("  %s %s { ... }\n", d.Kind, d.Name)
	}

	for _, ev := range m.Contract.Events {
		out += fmt.Sprintf("  event %s(", ev.Name)
		for i, p := range ev.Params {
			if i > 0 {
				out += ", "
			}
			out += fmt.Sprintf("%s: %s", p.Name, p.Type)
			if p.Indexed {
				out += " indexed"
			}
		}
		out += ")\n"
	}

	for _, fn := range m.Contract.Functions {
		if fn.SelectorOverride != "" {
			out += fmt.Sprintf("  @selector(%q)\n", fn.SelectorOverride)
		}
		out += fmt.Sprintf("  fn %s(", fn.Name)
		for i, p := range fn.Params {
			if i > 0 {
				out += ", "
			}
			out += fmt.Sprintf("%s: %s", p.Name, p.Type)
		}
		out += ")"
		if len(fn.Returns) > 0 {
			out += " -> ("
			for i, r := range fn.Returns {
				if i > 0 {
					out += ", "
				}
				out += fmt.Sprintf("%s: %s", r.Name, r.Type)
			}
			out += ")"
		}
		for _, mod := range fn.Modifiers {
			out += " " + mod
		}
		out += fmt.Sprintf(" { ... } // stmts=%d\n", len(fn.Body))
	}

	if m.Contract.Constructor != nil {
		out += "  constructor("
		for i, p := range m.Contract.Constructor.Params {
			if i > 0 {
				out += ", "
			}
			out += fmt.Sprintf("%s: %s", p.Name, p.Type)
		}
		out += ")"
		for _, mod := range m.Contract.Constructor.Modifiers {
			out += " " + mod
		}
		out += fmt.Sprintf(" { ... } // stmts=%d\n", len(m.Contract.Constructor.Body))
	}

	if m.Contract.Fallback != nil {
		out += fmt.Sprintf("  fallback { ... } // stmts=%d\n", len(m.Contract.Fallback.Body))
	}

	out += "}"
	return out
}
