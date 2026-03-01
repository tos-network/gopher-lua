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

// ValidateTOIText performs a lightweight structural validation for textual .toi content.
func ValidateTOIText(data []byte) error {
	_, err := parseTOIInfo(data)
	return err
}

// TOIInfo is lightweight metadata extracted from textual .toi content.
type TOIInfo struct {
	Version       string
	InterfaceName string
	FunctionCount int
	EventCount    int
}

// InspectTOIText validates and extracts lightweight metadata from textual .toi content.
func InspectTOIText(data []byte) (*TOIInfo, error) {
	return parseTOIInfo(data)
}

func parseTOIInfo(data []byte) (*TOIInfo, error) {
	s := strings.TrimSpace(string(data))
	if s == "" {
		return nil, fmt.Errorf("toi text is empty")
	}
	lines := strings.Split(s, "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("toi text is empty")
	}

	first := ""
	start := 0
	for i, raw := range lines {
		line := normalizeTOILine(raw)
		if line == "" {
			continue
		}
		first = line
		start = i + 1
		break
	}
	if first == "" {
		return nil, fmt.Errorf("toi text is empty")
	}
	if !strings.HasPrefix(first, "tol ") {
		return nil, fmt.Errorf("toi text must start with 'tol <version>' header")
	}
	version := strings.TrimSpace(strings.TrimPrefix(first, "tol "))
	if version == "" {
		return nil, fmt.Errorf("toi header missing version")
	}

	info := &TOIInfo{Version: version}
	seenInterface := false
	inInterface := false
	selectorPending := false

	for i := start; i < len(lines); i++ {
		line := normalizeTOILine(lines[i])
		if line == "" {
			continue
		}
		if !seenInterface {
			if !strings.HasPrefix(line, "interface ") {
				return nil, fmt.Errorf("toi text must contain interface declaration")
			}
			nameAndBrace := strings.TrimSpace(strings.TrimPrefix(line, "interface "))
			if !strings.HasSuffix(nameAndBrace, "{") {
				return nil, fmt.Errorf("toi interface declaration must end with '{'")
			}
			name := strings.TrimSpace(strings.TrimSuffix(nameAndBrace, "{"))
			if name == "" {
				return nil, fmt.Errorf("toi interface name not found")
			}
			info.InterfaceName = name
			seenInterface = true
			inInterface = true
			continue
		}
		if !inInterface {
			return nil, fmt.Errorf("toi text has unexpected content after interface block")
		}

		if line == "}" {
			inInterface = false
			continue
		}
		if strings.HasPrefix(line, "interface ") {
			return nil, fmt.Errorf("toi text supports exactly one interface declaration")
		}
		if strings.HasPrefix(line, "@selector(") {
			if !strings.HasSuffix(line, ")") {
				return nil, fmt.Errorf("toi selector annotation must end with ')'")
			}
			selectorPending = true
			continue
		}
		if strings.HasPrefix(line, "fn ") {
			if !strings.HasSuffix(line, ";") {
				return nil, fmt.Errorf("toi function declaration must end with ';'")
			}
			if !strings.Contains(line, "(") || !strings.Contains(line, ")") {
				return nil, fmt.Errorf("toi function declaration must contain parameter list")
			}
			info.FunctionCount++
			selectorPending = false
			continue
		}
		if strings.HasPrefix(line, "event ") {
			if selectorPending {
				return nil, fmt.Errorf("toi selector annotation must be followed by function declaration")
			}
			if !strings.HasSuffix(line, ";") {
				return nil, fmt.Errorf("toi event declaration must end with ';'")
			}
			if !strings.Contains(line, "(") || !strings.Contains(line, ")") {
				return nil, fmt.Errorf("toi event declaration must contain parameter list")
			}
			info.EventCount++
			continue
		}
		return nil, fmt.Errorf("toi interface block contains unsupported line: %q", line)
	}

	if !seenInterface {
		return nil, fmt.Errorf("toi text must contain interface declaration")
	}
	if inInterface {
		return nil, fmt.Errorf("toi text has unbalanced braces")
	}
	if selectorPending {
		return nil, fmt.Errorf("toi selector annotation must be followed by function declaration")
	}
	return info, nil
}

func normalizeTOILine(raw string) string {
	line := strings.TrimSpace(raw)
	if line == "" {
		return ""
	}
	if idx := strings.Index(line, "--"); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}
	return line
}
