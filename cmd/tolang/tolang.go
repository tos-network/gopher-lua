package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/chzyer/readline"
	"github.com/tos-network/tolang"
	"github.com/tos-network/tolang/parse"
	"os"
	"runtime/pprof"
)

func main() {
	os.Exit(mainAux())
}

func mainAux() int {
	if handled, status := dispatchSubcommand(os.Args[1:]); handled {
		return status
	}
	if removed, hint, ok := detectRemovedTOLFlag(os.Args[1:]); ok {
		if hint != "" {
			fmt.Printf("legacy TOL flag %s is removed; use %s\n", removed, hint)
		} else {
			fmt.Printf("legacy TOL flag %s is removed; use subcommands (tol compile|pack|inspect|verify)\n", removed)
		}
		return 1
	}

	var opt_e, opt_l, opt_p, opt_c string
	var opt_i, opt_v, opt_dt, opt_dc, opt_di, opt_bc bool
	flag.StringVar(&opt_e, "e", "", "")
	flag.StringVar(&opt_l, "l", "", "")
	flag.StringVar(&opt_p, "p", "", "")
	flag.StringVar(&opt_c, "c", "", "")
	flag.BoolVar(&opt_i, "i", false, "")
	flag.BoolVar(&opt_v, "v", false, "")
	flag.BoolVar(&opt_dt, "dt", false, "")
	flag.BoolVar(&opt_dc, "dc", false, "")
	flag.BoolVar(&opt_di, "di", false, "")
	flag.BoolVar(&opt_bc, "bc", false, "")
	flag.Usage = func() {
		fmt.Println(`Usage:
  tol <subcommand> [flags] <inputs...>
  tol [options] [script [args]]

Subcommands:
  compile   compile .tol source to .toc/.toi/.tor
  pack      package a directory with manifest.json into .tor
  inspect   inspect .toc/.toi/.tor metadata
  verify    verify .toc/.toi/.tor integrity

Lua/VM options:
	Available options are:
	  -e stat  execute string 'stat'
	  -l name  require library 'name'
	  -c file  compile source script to bytecode file
	  -bc      treat input script as bytecode
	  -dt      dump AST trees
	  -dc      dump VM codes
	  -di      dump IR
	  -i       enter interactive mode after executing 'script'
  -p file  write cpu profiles to the file
  -v       show version information`)
	}
	flag.Parse()
	if len(opt_p) != 0 {
		f, err := os.Create(opt_p)
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}
	if len(opt_e) == 0 && !opt_i && !opt_v && flag.NArg() == 0 {
		opt_i = true
	}

	status := 0

	L := lua.NewState()
	defer L.Close()

	if opt_v || opt_i {
		fmt.Println(lua.PackageCopyRight)
	}

	if len(opt_l) > 0 {
		src, err := os.ReadFile(opt_l)
		if err != nil {
			fmt.Println(err.Error())
		} else if err := L.DoString(string(src)); err != nil {
			fmt.Println(err.Error())
		}
	}

	if len(opt_c) > 0 {
		if flag.NArg() == 0 {
			fmt.Println("compile mode requires an input source script")
			return 1
		}
		input := flag.Arg(0)
		src, err := os.ReadFile(input)
		if err != nil {
			fmt.Println(err.Error())
			return 1
		}
		bc, err := lua.CompileSourceToBytecode(src, input)
		if err != nil {
			fmt.Println(err.Error())
			return 1
		}
		if err := os.WriteFile(opt_c, bc, 0o644); err != nil {
			fmt.Println(err.Error())
			return 1
		}
		return 0
	}
	if nargs := flag.NArg(); nargs > 0 {
		script := flag.Arg(0)
		argtb := L.NewTable()
		for i := 1; i < nargs; i++ {
			L.RawSet(argtb, lua.LNumber(strconv.Itoa(i)), lua.LString(flag.Arg(i)))
		}
		L.SetGlobal("arg", argtb)
		src, err := os.ReadFile(script)
		if err != nil {
			fmt.Println(err.Error())
			status = 1
		} else {
			if opt_dt || opt_dc || opt_di {
				if opt_bc {
					if opt_dt {
						fmt.Println("-dt is source-only (AST unavailable for bytecode input)")
						return 1
					}
					proto, err := lua.DecodeFunctionProto(src)
					if err != nil {
						fmt.Println(err.Error())
						return 1
					}
					if opt_dc {
						fmt.Println(proto.String())
					}
					if opt_di {
						fmt.Println(lua.BuildIRFromProto(proto, script).String())
					}
				} else {
					chunk, err := parse.Parse(bytes.NewReader(src), script)
					if err != nil {
						fmt.Println(err.Error())
						return 1
					}
					if opt_dt {
						fmt.Println(parse.Dump(chunk))
					}
					if opt_dc {
						proto, err := lua.Compile(chunk, script)
						if err != nil {
							fmt.Println(err.Error())
							return 1
						}
						fmt.Println(proto.String())
					}
					if opt_di {
						program, err := lua.BuildIR(chunk, script)
						if err != nil {
							fmt.Println(err.Error())
							return 1
						}
						fmt.Println(program.String())
					}
				}
			}
			if opt_bc {
				if err := L.DoBytecode(src); err != nil {
					fmt.Println(err.Error())
					status = 1
				}
			} else if err := L.DoString(string(src)); err != nil {
				fmt.Println(err.Error())
				status = 1
			}
		}
	}

	if len(opt_e) > 0 {
		if err := L.DoString(opt_e); err != nil {
			fmt.Println(err.Error())
			status = 1
		}
	}

	if opt_i {
		doREPL(L)
	}
	return status
}

func collectTORPackageInputs(root string) ([]byte, map[string][]byte, error) {
	var manifest []byte
	files := map[string][]byte{}
	if err := filepath.WalkDir(root, func(fullPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, fullPath)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		rel = strings.TrimSpace(rel)
		body, err := os.ReadFile(fullPath)
		if err != nil {
			return err
		}
		if rel == "manifest.json" {
			manifest = body
			return nil
		}
		files[rel] = body
		return nil
	}); err != nil {
		return nil, nil, err
	}
	if len(manifest) == 0 {
		return nil, nil, fmt.Errorf("tor package directory must include manifest.json")
	}
	return manifest, files, nil
}

func detectRemovedTOLFlag(args []string) (removed string, hint string, ok bool) {
	removedHints := map[string]string{
		"-ctol":          "tol compile -o <out.toc> <input.tol>",
		"-ctoi":          "tol compile --emit toi -o <out.toi> <input.tol>",
		"-ctoiname":      "tol compile --emit toi --name <Name> -o <out.toi> <input.tol>",
		"-ctoc":          "tol compile -o <out.toc> <input.tol>",
		"-ctor":          "tol compile --emit tor -o <out.tor> <input.tol>  or  tol pack -o <out.tor> <dir>",
		"-ctorpkg":       "tol compile --emit tor --package-name <name> ...",
		"-ctorver":       "tol compile --emit tor --package-version <version> ...",
		"-ctorifacename": "tol compile --emit tor --name <Interface> ...",
		"-ctorsrc":       "tol compile --emit tor --include-source ...",
		"-dtol":          "tol compile --ast <input.tol>",
		"-dtoi":          "tol inspect <artifact.toi>",
		"-dtoc":          "tol inspect <artifact.toc>",
		"-dtocj":         "tol inspect --json <artifact.toc>",
		"-dtor":          "tol inspect <artifact.tor>",
		"-dtorj":         "tol inspect --json <artifact.tor>",
		"-vtoc":          "tol verify <artifact.toc>",
		"-vtocsrc":       "tol verify --source <source.tol> <artifact.toc>",
		"-vtor":          "tol verify <artifact.tor>",
		"-vtoi":          "tol verify <artifact.toi>",
	}
	for _, raw := range args {
		a := strings.TrimSpace(raw)
		if !strings.HasPrefix(a, "-") {
			continue
		}
		if i := strings.Index(a, "="); i >= 0 {
			a = a[:i]
		}
		if hint, exists := removedHints[a]; exists {
			return a, hint, true
		}
	}
	return "", "", false
}

// do read/eval/print/loop
func doREPL(L *lua.LState) {
	rl, err := readline.New("> ")
	if err != nil {
		panic(err)
	}
	defer rl.Close()
	for {
		if str, err := loadline(rl, L); err == nil {
			if err := L.DoString(str); err != nil {
				fmt.Println(err)
			}
		} else { // error on loadline
			fmt.Println(err)
			return
		}
	}
}

func incomplete(err error) bool {
	if lerr, ok := err.(*lua.ApiError); ok {
		if perr, ok := lerr.Cause.(*parse.Error); ok {
			return perr.Pos.Line == parse.EOF
		}
	}
	return false
}

func loadline(rl *readline.Instance, L *lua.LState) (string, error) {
	rl.SetPrompt("> ")
	if line, err := rl.Readline(); err == nil {
		if _, err := L.LoadString("return " + line); err == nil { // try add return <...> then compile
			return line, nil
		} else {
			return multiline(line, rl, L)
		}
	} else {
		return "", err
	}
}

func multiline(ml string, rl *readline.Instance, L *lua.LState) (string, error) {
	for {
		if _, err := L.LoadString(ml); err == nil { // try compile
			return ml, nil
		} else if !incomplete(err) { // syntax error , but not EOF
			return ml, nil
		} else {
			rl.SetPrompt(">> ")
			if line, err := rl.Readline(); err == nil {
				ml = ml + "\n" + line
			} else {
				return "", err
			}
		}
	}
}
