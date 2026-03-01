package main

import (
	"bytes"
	"flag"
	"fmt"
	"strconv"

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
	var opt_e, opt_l, opt_p, opt_c, opt_ctol, opt_ctoc string
	var opt_i, opt_v, opt_dt, opt_dc, opt_di, opt_bc, opt_dtol, opt_dtoc bool
	flag.StringVar(&opt_e, "e", "", "")
	flag.StringVar(&opt_l, "l", "", "")
	flag.StringVar(&opt_p, "p", "", "")
	flag.StringVar(&opt_c, "c", "", "")
	flag.StringVar(&opt_ctol, "ctol", "", "")
	flag.StringVar(&opt_ctoc, "ctoc", "", "")
	flag.BoolVar(&opt_i, "i", false, "")
	flag.BoolVar(&opt_v, "v", false, "")
	flag.BoolVar(&opt_dt, "dt", false, "")
	flag.BoolVar(&opt_dc, "dc", false, "")
	flag.BoolVar(&opt_di, "di", false, "")
	flag.BoolVar(&opt_bc, "bc", false, "")
	flag.BoolVar(&opt_dtol, "dtol", false, "")
	flag.BoolVar(&opt_dtoc, "dtoc", false, "")
	flag.Usage = func() {
		fmt.Println(`Usage: tolang [options] [script [args]].
	Available options are:
	  -e stat  execute string 'stat'
	  -l name  require library 'name'
	  -c file  compile source script to bytecode file
	  -ctol file  compile TOL source script to bytecode file (skeleton path)
	  -ctoc file  compile TOL source script to .toc artifact file
	  -bc      treat input script as bytecode
	  -dt      dump AST trees
	  -dc      dump VM codes
	  -di      dump IR
	  -dtol    dump parsed TOL module
	  -dtoc    dump parsed TOC artifact metadata
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
	if len(opt_c) > 0 && (len(opt_ctol) > 0 || len(opt_ctoc) > 0) {
		fmt.Println("cannot use -c with -ctol/-ctoc together")
		return 1
	}
	if len(opt_ctol) > 0 && len(opt_ctoc) > 0 {
		fmt.Println("cannot use -ctol and -ctoc together")
		return 1
	}
	if opt_bc && (len(opt_ctol) > 0 || len(opt_ctoc) > 0 || opt_dtol || opt_dtoc) {
		fmt.Println("-bc cannot be combined with -ctol, -ctoc, -dtol, or -dtoc")
		return 1
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
	if len(opt_ctol) > 0 {
		if flag.NArg() == 0 {
			fmt.Println("TOL compile mode requires an input source script")
			return 1
		}
		input := flag.Arg(0)
		src, err := os.ReadFile(input)
		if err != nil {
			fmt.Println(err.Error())
			return 1
		}
		bc, err := lua.CompileTOLToBytecode(src, input)
		if err != nil {
			fmt.Println(err.Error())
			return 1
		}
		if err := os.WriteFile(opt_ctol, bc, 0o644); err != nil {
			fmt.Println(err.Error())
			return 1
		}
		return 0
	}
	if len(opt_ctoc) > 0 {
		if flag.NArg() == 0 {
			fmt.Println("TOL .toc compile mode requires an input source script")
			return 1
		}
		input := flag.Arg(0)
		src, err := os.ReadFile(input)
		if err != nil {
			fmt.Println(err.Error())
			return 1
		}
		toc, err := lua.CompileTOLToTOC(src, input)
		if err != nil {
			fmt.Println(err.Error())
			return 1
		}
		if err := os.WriteFile(opt_ctoc, toc, 0o644); err != nil {
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
			if opt_dtol {
				mod, err := lua.ParseTOLModule(src, script)
				if err != nil {
					fmt.Println(err.Error())
					return 1
				}
				fmt.Println(mod.String())
				return 0
			}
			if opt_dtoc {
				toc, err := lua.DecodeTOC(src)
				if err != nil {
					fmt.Println(err.Error())
					return 1
				}
				if _, err := lua.DecodeFunctionProto(toc.Bytecode); err != nil {
					fmt.Printf("invalid embedded bytecode: %v\n", err)
					return 1
				}
				fmt.Printf("TOC version: %d\n", toc.Version)
				fmt.Printf("Compiler: %s\n", toc.Compiler)
				fmt.Printf("Contract: %s\n", toc.ContractName)
				fmt.Printf("Bytecode bytes: %d\n", len(toc.Bytecode))
				fmt.Printf("Bytecode decode: ok\n")
				fmt.Printf("Source hash: %s\n", toc.SourceHash)
				fmt.Printf("Bytecode hash: %s\n", toc.BytecodeHash)
				if len(toc.ABIJSON) > 0 {
					fmt.Printf("ABI JSON: %s\n", string(toc.ABIJSON))
				}
				if len(toc.StorageLayoutJSON) > 0 {
					fmt.Printf("Storage JSON: %s\n", string(toc.StorageLayoutJSON))
				}
				return 0
			}
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
