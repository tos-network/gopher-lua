package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	lua "github.com/tos-network/tolang"
)

func dispatchSubcommand(args []string) (bool, int) {
	if len(args) == 0 {
		return false, 0
	}
	switch args[0] {
	case "compile":
		return true, cmdCompile(args[1:])
	case "pack":
		return true, cmdPack(args[1:])
	case "inspect":
		return true, cmdInspect(args[1:])
	case "verify":
		return true, cmdVerify(args[1:])
	case "--version":
		fmt.Println(lua.PackageCopyRight)
		return true, 0
	case "--help", "-h", "help":
		printRootSubcommandUsage()
		return true, 0
	default:
		return false, 0
	}
}

func printRootSubcommandUsage() {
	fmt.Print(`Usage:
  tol <subcommand> [flags] <inputs...>
  tol [lua-options] [script [args]]

Subcommands:
  compile   compile .tol source to .toc/.toi/.tor
  pack      package a directory with manifest.json into .tor
  inspect   inspect .toc/.toi/.tor metadata
  verify    verify .toc/.toi/.tor integrity

Global:
  --version print version
  --help    print this help
`)
}

func cmdCompile(args []string) int {
	fs := flag.NewFlagSet("compile", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var emit, output, name, packageName, packageVersion string
	var includeSource, emitABI, dumpAST bool
	fs.StringVar(&emit, "emit", "toc", "emit format: toc|toi|tor")
	fs.StringVar(&output, "o", "", "output artifact path")
	fs.StringVar(&output, "output", "", "output artifact path")
	fs.StringVar(&name, "name", "", "interface name override (toi/tor)")
	fs.StringVar(&packageName, "package-name", "", "package name override (tor)")
	fs.StringVar(&packageVersion, "package-version", "0.0.0", "package version override (tor)")
	fs.BoolVar(&includeSource, "include-source", false, "include source in .tor")
	fs.BoolVar(&emitABI, "abi", false, "write .abi.json alongside .toc")
	fs.BoolVar(&dumpAST, "ast", false, "dump parsed TOL module")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: tol compile [--emit toc|toi|tor] [-o <output>] [options] <input.tol>")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 1
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "compile requires exactly one input .tol file")
		fs.Usage()
		return 1
	}

	input := fs.Arg(0)
	source, err := os.ReadFile(input)
	if err != nil {
		fmt.Println(err.Error())
		return 1
	}

	if dumpAST {
		mod, err := lua.ParseTOLModule(source, input)
		if err != nil {
			fmt.Println(err.Error())
			return 1
		}
		fmt.Println(mod.String())
		// Keep `tol compile --ast input.tol` as inspect-only unless explicitly asked to emit files.
		if output == "" && !emitABI && strings.EqualFold(strings.TrimSpace(emit), "toc") {
			return 0
		}
	}

	emit = strings.ToLower(strings.TrimSpace(emit))
	if emit == "" {
		emit = "toc"
	}
	switch emit {
	case "toc", "toi", "tor":
	default:
		fmt.Printf("unsupported --emit value %q (expected toc|toi|tor)\n", emit)
		return 1
	}

	if output == "" {
		output = defaultArtifactPath(input, emit)
	}
	if emitABI && emit != "toc" {
		fmt.Println("--abi is only valid with --emit toc")
		return 1
	}
	if emit != "tor" && (strings.TrimSpace(packageName) != "" || includeSource || fs.Lookup("package-version").Value.String() != "0.0.0") {
		fmt.Println("--package-name/--package-version/--include-source are only valid with --emit tor")
		return 1
	}

	switch emit {
	case "toc":
		toc, err := lua.CompileTOLToTOC(source, input)
		if err != nil {
			fmt.Println(err.Error())
			return 1
		}
		if err := os.WriteFile(output, toc, 0o644); err != nil {
			fmt.Println(err.Error())
			return 1
		}
		if emitABI {
			decoded, err := lua.DecodeTOC(toc)
			if err != nil {
				fmt.Println(err.Error())
				return 1
			}
			abiPath := strings.TrimSuffix(output, filepath.Ext(output)) + ".abi.json"
			abi := decoded.ABIJSON
			if len(abi) == 0 {
				abi = []byte("{}")
			}
			if err := os.WriteFile(abiPath, abi, 0o644); err != nil {
				fmt.Println(err.Error())
				return 1
			}
		}
	case "toi":
		toi, err := lua.CompileTOLToTOIWithOptions(source, input, &lua.TOICompileOptions{
			InterfaceName: strings.TrimSpace(name),
		})
		if err != nil {
			fmt.Println(err.Error())
			return 1
		}
		if err := os.WriteFile(output, toi, 0o644); err != nil {
			fmt.Println(err.Error())
			return 1
		}
	case "tor":
		if strings.TrimSpace(packageName) == "" {
			packageName = inputStem(input)
		}
		tor, err := lua.CompileTOLToTOR(source, input, &lua.TORCompileOptions{
			PackageName:      strings.TrimSpace(packageName),
			PackageVersion:   strings.TrimSpace(packageVersion),
			TOIInterfaceName: strings.TrimSpace(name),
			IncludeSource:    includeSource,
		})
		if err != nil {
			fmt.Println(err.Error())
			return 1
		}
		if err := os.WriteFile(output, tor, 0o644); err != nil {
			fmt.Println(err.Error())
			return 1
		}
	}
	return 0
}

func cmdPack(args []string) int {
	fs := flag.NewFlagSet("pack", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var output string
	fs.StringVar(&output, "o", "", "output .tor path")
	fs.StringVar(&output, "output", "", "output .tor path")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: tol pack -o <output.tor> <directory>")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 1
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "pack requires exactly one input directory")
		fs.Usage()
		return 1
	}
	if strings.TrimSpace(output) == "" {
		fmt.Fprintln(os.Stderr, "pack requires -o/--output")
		return 1
	}
	input := fs.Arg(0)
	info, err := os.Stat(input)
	if err != nil {
		fmt.Println(err.Error())
		return 1
	}
	if !info.IsDir() {
		fmt.Println("pack input must be a directory")
		return 1
	}
	manifest, files, err := collectTORPackageInputs(input)
	if err != nil {
		fmt.Println(err.Error())
		return 1
	}
	tor, err := lua.EncodeTOR(manifest, files)
	if err != nil {
		fmt.Println(err.Error())
		return 1
	}
	if err := os.WriteFile(output, tor, 0o644); err != nil {
		fmt.Println(err.Error())
		return 1
	}
	return 0
}

func cmdInspect(args []string) int {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var asJSON bool
	fs.BoolVar(&asJSON, "json", false, "output JSON")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: tol inspect [--json] <artifact>")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 1
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "inspect requires exactly one artifact path")
		fs.Usage()
		return 1
	}

	path := fs.Arg(0)
	body, err := os.ReadFile(path)
	if err != nil {
		fmt.Println(err.Error())
		return 1
	}
	switch detectArtifactKind(path, body) {
	case artifactTOC:
		return inspectTOC(body, asJSON)
	case artifactTOI:
		return inspectTOI(body, asJSON)
	case artifactTOR:
		return inspectTOR(body, asJSON)
	default:
		fmt.Println("unknown artifact type (expected .toc, .toi, or .tor)")
		return 1
	}
}

func cmdVerify(args []string) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var sourcePath string
	fs.StringVar(&sourcePath, "source", "", "source file for TOC source_hash verification")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: tol verify [--source <file>] <artifact>")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 1
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "verify requires exactly one artifact path")
		fs.Usage()
		return 1
	}

	path := fs.Arg(0)
	body, err := os.ReadFile(path)
	if err != nil {
		fmt.Println(err.Error())
		return 1
	}

	switch detectArtifactKind(path, body) {
	case artifactTOC:
		toc, err := lua.DecodeTOC(body)
		if err != nil {
			fmt.Println(err.Error())
			return 1
		}
		if strings.TrimSpace(sourcePath) != "" {
			source, err := os.ReadFile(sourcePath)
			if err != nil {
				fmt.Println(err.Error())
				return 1
			}
			if err := lua.VerifyTOCSourceHash(toc, source); err != nil {
				fmt.Println(err.Error())
				return 2
			}
		}
		fmt.Println("TOC: ok")
		return 0
	case artifactTOI:
		if strings.TrimSpace(sourcePath) != "" {
			fmt.Println("--source is only valid for .toc artifacts")
			return 1
		}
		if err := lua.ValidateTOIText(body); err != nil {
			fmt.Println(err.Error())
			return 1
		}
		fmt.Println("TOI: ok")
		return 0
	case artifactTOR:
		if strings.TrimSpace(sourcePath) != "" {
			fmt.Println("--source is only valid for .toc artifacts")
			return 1
		}
		if _, err := lua.DecodeTOR(body); err != nil {
			fmt.Println(err.Error())
			return 1
		}
		fmt.Println("TOR: ok")
		return 0
	default:
		fmt.Println("unknown artifact type (expected .toc, .toi, or .tor)")
		return 1
	}
}

func defaultArtifactPath(input, emit string) string {
	base := strings.TrimSuffix(input, filepath.Ext(input))
	return base + "." + emit
}

func inputStem(input string) string {
	base := filepath.Base(input)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

type artifactKind int

const (
	artifactUnknown artifactKind = iota
	artifactTOC
	artifactTOI
	artifactTOR
)

func detectArtifactKind(path string, body []byte) artifactKind {
	switch strings.ToLower(strings.TrimSpace(filepath.Ext(path))) {
	case ".toc":
		return artifactTOC
	case ".toi":
		return artifactTOI
	case ".tor":
		return artifactTOR
	}
	if lua.IsTOC(body) {
		return artifactTOC
	}
	if lua.IsTOR(body) {
		return artifactTOR
	}
	if err := lua.ValidateTOIText(body); err == nil {
		return artifactTOI
	}
	return artifactUnknown
}

func inspectTOC(body []byte, asJSON bool) int {
	toc, err := lua.DecodeTOC(body)
	if err != nil {
		fmt.Println(err.Error())
		return 1
	}
	if _, err := lua.DecodeFunctionProto(toc.Bytecode); err != nil {
		fmt.Printf("invalid embedded bytecode: %v\n", err)
		return 1
	}
	if asJSON {
		out := struct {
			Version       uint16          `json:"version"`
			Compiler      string          `json:"compiler"`
			ContractName  string          `json:"contract_name"`
			BytecodeBytes int             `json:"bytecode_bytes"`
			SourceHash    string          `json:"source_hash"`
			BytecodeHash  string          `json:"bytecode_hash"`
			ABIJSON       json.RawMessage `json:"abi_json,omitempty"`
			StorageJSON   json.RawMessage `json:"storage_json,omitempty"`
		}{
			Version:       toc.Version,
			Compiler:      toc.Compiler,
			ContractName:  toc.ContractName,
			BytecodeBytes: len(toc.Bytecode),
			SourceHash:    toc.SourceHash,
			BytecodeHash:  toc.BytecodeHash,
		}
		if len(toc.ABIJSON) > 0 {
			out.ABIJSON = json.RawMessage(toc.ABIJSON)
		}
		if len(toc.StorageLayoutJSON) > 0 {
			out.StorageJSON = json.RawMessage(toc.StorageLayoutJSON)
		}
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			fmt.Println(err.Error())
			return 1
		}
		fmt.Println(string(b))
		return 0
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

func inspectTOI(body []byte, asJSON bool) int {
	info, err := lua.InspectTOIText(body)
	if err != nil {
		fmt.Println(err.Error())
		return 1
	}
	if asJSON {
		out := struct {
			Version       string `json:"version"`
			InterfaceName string `json:"interface_name"`
			FunctionCount int    `json:"function_count"`
			EventCount    int    `json:"event_count"`
		}{
			Version:       info.Version,
			InterfaceName: info.InterfaceName,
			FunctionCount: info.FunctionCount,
			EventCount:    info.EventCount,
		}
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			fmt.Println(err.Error())
			return 1
		}
		fmt.Println(string(b))
		return 0
	}
	fmt.Printf("TOI version: %s\n", info.Version)
	fmt.Printf("Interface: %s\n", info.InterfaceName)
	fmt.Printf("Functions: %d\n", info.FunctionCount)
	fmt.Printf("Events: %d\n", info.EventCount)
	return 0
}

func inspectTOR(body []byte, asJSON bool) int {
	tor, err := lua.DecodeTOR(body)
	if err != nil {
		fmt.Println(err.Error())
		return 1
	}
	if asJSON {
		type torFileInfo struct {
			Path  string `json:"path"`
			Bytes int    `json:"bytes"`
		}
		names := make([]string, 0, len(tor.Files))
		for name := range tor.Files {
			names = append(names, name)
		}
		sort.Strings(names)
		infos := make([]torFileInfo, 0, len(names))
		for _, name := range names {
			infos = append(infos, torFileInfo{
				Path:  name,
				Bytes: len(tor.Files[name]),
			})
		}
		out := struct {
			ManifestJSON json.RawMessage `json:"manifest_json"`
			FileCount    int             `json:"file_count"`
			Files        []torFileInfo   `json:"files"`
			PackageHash  string          `json:"package_hash"`
		}{
			ManifestJSON: json.RawMessage(tor.ManifestJSON),
			FileCount:    len(tor.Files),
			Files:        infos,
			PackageHash:  lua.TORPackageHash(body),
		}
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			fmt.Println(err.Error())
			return 1
		}
		fmt.Println(string(b))
		return 0
	}

	fmt.Printf("Manifest JSON: %s\n", string(tor.ManifestJSON))
	fmt.Printf("Files: %d\n", len(tor.Files))
	names := make([]string, 0, len(tor.Files))
	for name := range tor.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Printf(" - %s (%d bytes)\n", name, len(tor.Files[name]))
	}
	fmt.Printf("Package hash: %s\n", lua.TORPackageHash(body))
	return 0
}
