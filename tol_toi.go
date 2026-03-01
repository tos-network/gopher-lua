package lua

import (
	"fmt"
	"strings"

	tolast "github.com/tos-network/tolang/tol/ast"
)

// TOICompileOptions configures .toi textual generation.
type TOICompileOptions struct {
	InterfaceName string
}

// CompileTOLToTOI compiles TOL source into a textual .toi interface declaration.
func CompileTOLToTOI(source []byte, name string) ([]byte, error) {
	return CompileTOLToTOIWithOptions(source, name, nil)
}

// CompileTOLToTOIWithOptions compiles TOL source into textual .toi with options.
func CompileTOLToTOIWithOptions(source []byte, name string, opts *TOICompileOptions) ([]byte, error) {
	mod, err := ParseTOLModule(source, name)
	if err != nil {
		return nil, err
	}
	return BuildTOIFromModuleWithOptions(mod, opts)
}

// BuildTOIFromModule renders a parsed module into a textual interface declaration.
func BuildTOIFromModule(mod *tolast.Module) ([]byte, error) {
	return BuildTOIFromModuleWithOptions(mod, nil)
}

// BuildTOIFromModuleWithOptions renders a parsed module into textual interface declaration.
func BuildTOIFromModuleWithOptions(mod *tolast.Module, opts *TOICompileOptions) ([]byte, error) {
	if mod == nil || mod.Contract == nil {
		return nil, fmt.Errorf("toi build requires a contract declaration")
	}
	contractName := strings.TrimSpace(mod.Contract.Name)
	if contractName == "" {
		return nil, fmt.Errorf("toi build requires a non-empty contract name")
	}
	version := strings.TrimSpace(mod.Version)
	if version == "" {
		version = "0.2"
	}
	interfaceName := "I" + contractName
	if opts != nil && strings.TrimSpace(opts.InterfaceName) != "" {
		interfaceName = strings.TrimSpace(opts.InterfaceName)
	}

	var b strings.Builder
	b.WriteString("tol ")
	b.WriteString(version)
	b.WriteString("\n\ninterface ")
	b.WriteString(interfaceName)
	b.WriteString(" {\n")

	for _, fn := range mod.Contract.Functions {
		vis := functionVisibilityFromModifiers(fn.Modifiers)
		if vis != "public" && vis != "external" {
			continue
		}
		if strings.TrimSpace(fn.SelectorOverride) != "" {
			b.WriteString("  @selector(")
			b.WriteString(fmt.Sprintf("%q", fn.SelectorOverride))
			b.WriteString(")\n")
		}
		b.WriteString("  fn ")
		b.WriteString(strings.TrimSpace(fn.Name))
		b.WriteString("(")
		b.WriteString(renderTOIFields(fn.Params, "arg"))
		b.WriteString(")")
		if len(fn.Returns) > 0 {
			b.WriteString(" -> (")
			b.WriteString(renderTOIFields(fn.Returns, "ret"))
			b.WriteString(")")
		}
		for _, m := range fn.Modifiers {
			modifier := strings.TrimSpace(m)
			if modifier == "" {
				continue
			}
			b.WriteString(" ")
			b.WriteString(modifier)
		}
		b.WriteString(";\n")
	}

	for _, ev := range mod.Contract.Events {
		b.WriteString("  event ")
		b.WriteString(strings.TrimSpace(ev.Name))
		b.WriteString("(")
		for i, p := range ev.Params {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(renderTOIField(p.Name, p.Type, fmt.Sprintf("arg%d", i+1)))
			if p.Indexed {
				b.WriteString(" indexed")
			}
		}
		b.WriteString(");\n")
	}

	b.WriteString("}\n")
	return []byte(b.String()), nil
}

func renderTOIFields(fields []tolast.FieldDecl, fallbackPrefix string) string {
	if len(fields) == 0 {
		return ""
	}
	out := make([]string, 0, len(fields))
	for i, f := range fields {
		out = append(out, renderTOIField(f.Name, f.Type, fmt.Sprintf("%s%d", fallbackPrefix, i+1)))
	}
	return strings.Join(out, ", ")
}

func renderTOIField(name, typ, fallbackName string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		n = fallbackName
	}
	return fmt.Sprintf("%s: %s", n, normalizeTOCType(typ))
}
