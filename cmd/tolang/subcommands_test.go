package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	lua "github.com/tos-network/tolang"
)

func TestDefaultArtifactPath(t *testing.T) {
	input := "/tmp/contract.tol"
	if got, want := defaultArtifactPath(input, "toc"), "/tmp/contract.toc"; got != want {
		t.Fatalf("defaultArtifactPath toc: got=%q want=%q", got, want)
	}
	if got, want := defaultArtifactPath(input, "toi"), "/tmp/contract.toi"; got != want {
		t.Fatalf("defaultArtifactPath toi: got=%q want=%q", got, want)
	}
	if got, want := defaultArtifactPath(input, "tor"), "/tmp/contract.tor"; got != want {
		t.Fatalf("defaultArtifactPath tor: got=%q want=%q", got, want)
	}
}

func TestDispatchSubcommandRouting(t *testing.T) {
	if handled, _ := dispatchSubcommand(nil); handled {
		t.Fatalf("empty args should not be handled by subcommand dispatcher")
	}
	if handled, _ := dispatchSubcommand([]string{"unknown"}); handled {
		t.Fatalf("unknown subcommand should fall back to legacy handler")
	}
	if handled, code := dispatchSubcommand([]string{"--help"}); !handled || code != 0 {
		t.Fatalf("--help should be handled with code 0, got handled=%v code=%d", handled, code)
	}
	if handled, code := dispatchSubcommand([]string{"--version"}); !handled || code != 0 {
		t.Fatalf("--version should be handled with code 0, got handled=%v code=%d", handled, code)
	}
}

func TestSubcommandHelpExitCodes(t *testing.T) {
	if code := cmdCompile([]string{"--help"}); code != 0 {
		t.Fatalf("compile --help: got=%d want=0", code)
	}
	if code := cmdPack([]string{"--help"}); code != 0 {
		t.Fatalf("pack --help: got=%d want=0", code)
	}
	if code := cmdInspect([]string{"--help"}); code != 0 {
		t.Fatalf("inspect --help: got=%d want=0", code)
	}
	if code := cmdVerify([]string{"--help"}); code != 0 {
		t.Fatalf("verify --help: got=%d want=0", code)
	}
}

func TestDetectArtifactKindMagicFallback(t *testing.T) {
	src := []byte("tol 0.2\n\ncontract Sample {\n  fn ping() public {\n  }\n}\n")
	toc, err := lua.CompileTOLToTOC(src, "sample.tol")
	if err != nil {
		t.Fatalf("compile toc: %v", err)
	}
	if got := detectArtifactKind("artifact.bin", toc); got != artifactTOC {
		t.Fatalf("detect toc by magic: got=%v want=%v", got, artifactTOC)
	}

	toi, err := lua.CompileTOLToTOI(src, "sample.tol")
	if err != nil {
		t.Fatalf("compile toi: %v", err)
	}
	if got := detectArtifactKind("artifact.bin", toi); got != artifactTOI {
		t.Fatalf("detect toi by text validation: got=%v want=%v", got, artifactTOI)
	}
}

func TestCmdCompileDefaultTOCOutput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "sample.tol")
	if err := os.WriteFile(input, []byte("tol 0.2\n\ncontract Sample {\n  fn ping() public {\n  }\n}\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	if code := cmdCompile([]string{input}); code != 0 {
		t.Fatalf("cmdCompile exit code: got=%d want=0", code)
	}

	out := filepath.Join(dir, "sample.toc")
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output toc: %v", err)
	}
	if _, err := lua.DecodeTOC(body); err != nil {
		t.Fatalf("decode output toc: %v", err)
	}
}

func TestCmdCompileDefaultTOIAndTOROutput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "sample.tol")
	if err := os.WriteFile(input, []byte("tol 0.2\n\ncontract Sample {\n  fn ping() public {\n  }\n}\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	if code := cmdCompile([]string{"--emit", "toi", input}); code != 0 {
		t.Fatalf("cmdCompile toi exit code: got=%d want=0", code)
	}
	toiPath := filepath.Join(dir, "sample.toi")
	toiBody, err := os.ReadFile(toiPath)
	if err != nil {
		t.Fatalf("read output toi: %v", err)
	}
	if err := lua.ValidateTOIText(toiBody); err != nil {
		t.Fatalf("validate output toi: %v", err)
	}

	if code := cmdCompile([]string{"--emit", "tor", input}); code != 0 {
		t.Fatalf("cmdCompile tor exit code: got=%d want=0", code)
	}
	torPath := filepath.Join(dir, "sample.tor")
	torBody, err := os.ReadFile(torPath)
	if err != nil {
		t.Fatalf("read output tor: %v", err)
	}
	if _, err := lua.DecodeTOR(torBody); err != nil {
		t.Fatalf("decode output tor: %v", err)
	}
}

func TestCmdVerifyTOCSourceMismatchExitCode(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "sample.tol")
	if err := os.WriteFile(srcPath, []byte("tol 0.2\n\ncontract Sample {\n  fn ping() public {\n  }\n}\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	tocPath := filepath.Join(dir, "sample.toc")
	if code := cmdCompile([]string{"-o", tocPath, srcPath}); code != 0 {
		t.Fatalf("compile toc exit code: got=%d want=0", code)
	}

	mismatchPath := filepath.Join(dir, "mismatch.tol")
	if err := os.WriteFile(mismatchPath, []byte("tol 0.2\n\ncontract Sample {\n  fn pong() public {\n  }\n}\n"), 0o644); err != nil {
		t.Fatalf("write mismatch source: %v", err)
	}

	if code := cmdVerify([]string{"--source", mismatchPath, tocPath}); code != 2 {
		t.Fatalf("verify mismatch exit code: got=%d want=2", code)
	}
}

func TestCmdPackDirectory(t *testing.T) {
	dir := t.TempDir()
	pkgDir := filepath.Join(dir, "pkg")
	if err := os.MkdirAll(filepath.Join(pkgDir, "bytecode"), 0o755); err != nil {
		t.Fatalf("mkdir bytecode: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(pkgDir, "interfaces"), 0o755); err != nil {
		t.Fatalf("mkdir interfaces: %v", err)
	}

	src := []byte("tol 0.2\n\ncontract Sample {\n  fn ping() public {\n  }\n}\n")
	toc, err := lua.CompileTOLToTOC(src, "sample.tol")
	if err != nil {
		t.Fatalf("compile toc: %v", err)
	}
	toi, err := lua.CompileTOLToTOI(src, "sample.tol")
	if err != nil {
		t.Fatalf("compile toi: %v", err)
	}

	tocPath := filepath.Join(pkgDir, "bytecode", "Sample.toc")
	toiPath := filepath.Join(pkgDir, "interfaces", "ISample.toi")
	if err := os.WriteFile(tocPath, toc, 0o644); err != nil {
		t.Fatalf("write toc: %v", err)
	}
	if err := os.WriteFile(toiPath, toi, 0o644); err != nil {
		t.Fatalf("write toi: %v", err)
	}

	manifest := []byte(`{
  "name": "sample-pack",
  "version": "1.0.0",
  "contracts": [
    {"name":"Sample","toc":"bytecode/Sample.toc","toi":"interfaces/ISample.toi"}
  ]
}`)
	if err := os.WriteFile(filepath.Join(pkgDir, "manifest.json"), manifest, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	out := filepath.Join(dir, "out.tor")
	if code := cmdPack([]string{"-o", out, pkgDir}); code != 0 {
		t.Fatalf("cmdPack exit code: got=%d want=0", code)
	}
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read packed tor: %v", err)
	}
	if _, err := lua.DecodeTOR(body); err != nil {
		t.Fatalf("decode packed tor: %v", err)
	}
}

func TestCmdCompileTOCWithABISidecar(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "sample.tol")
	if err := os.WriteFile(input, []byte("tol 0.2\n\ncontract Sample {\n  fn ping() public {\n  }\n}\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	out := filepath.Join(dir, "out.toc")
	if code := cmdCompile([]string{"--abi", "-o", out, input}); code != 0 {
		t.Fatalf("compile with --abi exit code: got=%d want=0", code)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("toc output missing: %v", err)
	}
	abiPath := filepath.Join(dir, "out.abi.json")
	abi, err := os.ReadFile(abiPath)
	if err != nil {
		t.Fatalf("abi sidecar missing: %v", err)
	}
	if !json.Valid(abi) {
		t.Fatalf("abi sidecar must be valid json: %s", string(abi))
	}
}

func TestCmdCompileTOINameOverride(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "sample.tol")
	if err := os.WriteFile(input, []byte("tol 0.2\n\ncontract Sample {\n  fn ping() public {\n  }\n}\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	out := filepath.Join(dir, "iface.toi")
	if code := cmdCompile([]string{"--emit", "toi", "--name", "ISampleX", "-o", out, input}); code != 0 {
		t.Fatalf("compile toi exit code: got=%d want=0", code)
	}
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read toi output: %v", err)
	}
	info, err := lua.InspectTOIText(body)
	if err != nil {
		t.Fatalf("inspect toi: %v", err)
	}
	if info.InterfaceName != "ISampleX" {
		t.Fatalf("toi name override: got=%q want=%q", info.InterfaceName, "ISampleX")
	}
}

func TestCmdCompileTORDefaultsAndNameOverride(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "my_contract.tol")
	if err := os.WriteFile(input, []byte("tol 0.2\n\ncontract Sample {\n  fn ping() public {\n  }\n}\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	out := filepath.Join(dir, "out.tor")
	if code := cmdCompile([]string{"--emit", "tor", "--name", "ISampleZ", "-o", out, input}); code != 0 {
		t.Fatalf("compile tor exit code: got=%d want=0", code)
	}
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read tor output: %v", err)
	}
	tor, err := lua.DecodeTOR(body)
	if err != nil {
		t.Fatalf("decode tor: %v", err)
	}

	var manifest struct {
		Name      string `json:"name"`
		Version   string `json:"version"`
		Contracts []struct {
			Name string `json:"name"`
			TOI  string `json:"toi"`
		} `json:"contracts"`
	}
	if err := json.Unmarshal(tor.ManifestJSON, &manifest); err != nil {
		t.Fatalf("decode manifest json: %v", err)
	}
	if manifest.Name != "my_contract" {
		t.Fatalf("default tor package name: got=%q want=%q", manifest.Name, "my_contract")
	}
	if manifest.Version != "0.0.0" {
		t.Fatalf("default tor package version: got=%q want=%q", manifest.Version, "0.0.0")
	}
	if len(manifest.Contracts) != 1 {
		t.Fatalf("manifest contracts len: got=%d want=1", len(manifest.Contracts))
	}
	toiPath := manifest.Contracts[0].TOI
	toiBody, ok := tor.Files[toiPath]
	if !ok {
		t.Fatalf("manifest toi path %q missing from archive files", toiPath)
	}
	info, err := lua.InspectTOIText(toiBody)
	if err != nil {
		t.Fatalf("inspect tor toi: %v", err)
	}
	if info.InterfaceName != "ISampleZ" {
		t.Fatalf("tor toi name override: got=%q want=%q", info.InterfaceName, "ISampleZ")
	}
}

func TestCmdInspectAndVerifyWithoutExtension(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "sample.tol")
	src := []byte("tol 0.2\n\ncontract Sample {\n  fn ping() public {\n  }\n}\n")
	if err := os.WriteFile(srcPath, src, 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	toc, err := lua.CompileTOLToTOC(src, srcPath)
	if err != nil {
		t.Fatalf("compile toc: %v", err)
	}
	toi, err := lua.CompileTOLToTOI(src, srcPath)
	if err != nil {
		t.Fatalf("compile toi: %v", err)
	}
	tor, err := lua.CompileTOLToTOR(src, srcPath, &lua.TORCompileOptions{})
	if err != nil {
		t.Fatalf("compile tor: %v", err)
	}

	tocPath := filepath.Join(dir, "toc.bin")
	toiPath := filepath.Join(dir, "toi.bin")
	torPath := filepath.Join(dir, "tor.bin")
	if err := os.WriteFile(tocPath, toc, 0o644); err != nil {
		t.Fatalf("write toc: %v", err)
	}
	if err := os.WriteFile(toiPath, toi, 0o644); err != nil {
		t.Fatalf("write toi: %v", err)
	}
	if err := os.WriteFile(torPath, tor, 0o644); err != nil {
		t.Fatalf("write tor: %v", err)
	}

	if code := cmdInspect([]string{tocPath}); code != 0 {
		t.Fatalf("inspect toc without extension: got=%d want=0", code)
	}
	if code := cmdInspect([]string{toiPath}); code != 0 {
		t.Fatalf("inspect toi without extension: got=%d want=0", code)
	}
	if code := cmdInspect([]string{torPath}); code != 0 {
		t.Fatalf("inspect tor without extension: got=%d want=0", code)
	}
	if code := cmdVerify([]string{tocPath}); code != 0 {
		t.Fatalf("verify toc without extension: got=%d want=0", code)
	}
	if code := cmdVerify([]string{toiPath}); code != 0 {
		t.Fatalf("verify toi without extension: got=%d want=0", code)
	}
	if code := cmdVerify([]string{torPath}); code != 0 {
		t.Fatalf("verify tor without extension: got=%d want=0", code)
	}
}

func TestCmdVerifySourceFlagRejectedForNonTOC(t *testing.T) {
	dir := t.TempDir()
	src := []byte("tol 0.2\n\ncontract Sample {\n  fn ping() public {\n  }\n}\n")
	srcPath := filepath.Join(dir, "sample.tol")
	if err := os.WriteFile(srcPath, src, 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	toi, err := lua.CompileTOLToTOI(src, srcPath)
	if err != nil {
		t.Fatalf("compile toi: %v", err)
	}
	toiPath := filepath.Join(dir, "sample.toi")
	if err := os.WriteFile(toiPath, toi, 0o644); err != nil {
		t.Fatalf("write toi: %v", err)
	}
	if code := cmdVerify([]string{"--source", srcPath, toiPath}); code != 1 {
		t.Fatalf("verify toi with --source: got=%d want=1", code)
	}

	tor, err := lua.CompileTOLToTOR(src, srcPath, &lua.TORCompileOptions{})
	if err != nil {
		t.Fatalf("compile tor: %v", err)
	}
	torPath := filepath.Join(dir, "sample.tor")
	if err := os.WriteFile(torPath, tor, 0o644); err != nil {
		t.Fatalf("write tor: %v", err)
	}
	if code := cmdVerify([]string{"--source", srcPath, torPath}); code != 1 {
		t.Fatalf("verify tor with --source: got=%d want=1", code)
	}
}

func TestCmdCompileRejectsInvalidFlagCombinations(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "sample.tol")
	if err := os.WriteFile(input, []byte("tol 0.2\n\ncontract Sample {\n  fn ping() public {\n  }\n}\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	if code := cmdCompile([]string{"--emit", "toi", "--abi", input}); code != 1 {
		t.Fatalf("--abi with emit=toi: got=%d want=1", code)
	}
	if code := cmdCompile([]string{"--emit", "toc", "--package-name", "x", input}); code != 1 {
		t.Fatalf("--package-name with emit=toc: got=%d want=1", code)
	}
	if code := cmdCompile([]string{"--emit", "toc", "--include-source", input}); code != 1 {
		t.Fatalf("--include-source with emit=toc: got=%d want=1", code)
	}
}
