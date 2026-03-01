package lua

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func mustCompileTOCTestArtifact(t *testing.T) []byte {
	t.Helper()
	src := []byte(`
tol 0.2
contract Demo {
  fn ping() public { return; }
}
`)
	toc, err := CompileTOLToTOC(src, "<tol>")
	if err != nil {
		t.Fatalf("compile toc: %v", err)
	}
	return toc
}

func mustCompileTOITestArtifact(t *testing.T) []byte {
	t.Helper()
	src := []byte(`
tol 0.2
contract Demo {
  fn ping() public { return; }
}
`)
	toi, err := CompileTOLToTOI(src, "<tol>")
	if err != nil {
		t.Fatalf("compile toi: %v", err)
	}
	return toi
}

func TestEncodeDecodeTORRoundTrip(t *testing.T) {
	manifest := []byte(`{"name":"demo","version":"1.0.0","contracts":[{"name":"Demo","toc":"bytecode/Demo.toc"}]}`)
	toc := mustCompileTOCTestArtifact(t)
	toi := mustCompileTOITestArtifact(t)
	files := map[string][]byte{
		"bytecode/Demo.toc":    toc,
		"interfaces/IDemo.toi": toi,
	}

	torA, err := EncodeTOR(manifest, files)
	if err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}
	torB, err := EncodeTOR(manifest, files)
	if err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}
	if !bytes.Equal(torA, torB) {
		t.Fatalf("expected deterministic tor bytes")
	}
	if !IsTOR(torA) {
		t.Fatalf("expected tor magic")
	}

	decoded, err := DecodeTOR(torA)
	if err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}
	if string(decoded.ManifestJSON) != string(manifest) {
		t.Fatalf("unexpected manifest: %s", string(decoded.ManifestJSON))
	}
	if !bytes.Equal(decoded.Files["bytecode/Demo.toc"], toc) {
		t.Fatalf("unexpected bytecode entry")
	}
	if !bytes.Equal(decoded.Files["interfaces/IDemo.toi"], toi) {
		t.Fatalf("unexpected interface entry")
	}
}

func TestEncodeTORRejectsInvalidManifestJSON(t *testing.T) {
	if _, err := EncodeTOR([]byte("{"), nil); err == nil {
		t.Fatalf("expected invalid manifest json error")
	}
}

func TestEncodeTORRejectsManifestMissingName(t *testing.T) {
	if _, err := EncodeTOR([]byte(`{"version":"1.0.0"}`), nil); err == nil {
		t.Fatalf("expected missing name error")
	}
}

func TestEncodeTORRejectsManifestMissingVersion(t *testing.T) {
	if _, err := EncodeTOR([]byte(`{"name":"demo"}`), nil); err == nil {
		t.Fatalf("expected missing version error")
	}
}

func TestEncodeTORRejectsPathEscape(t *testing.T) {
	manifest := []byte(`{"name":"demo","version":"1.0.0"}`)
	if _, err := EncodeTOR(manifest, map[string][]byte{"../x": []byte("x")}); err == nil {
		t.Fatalf("expected path escape error")
	}
}

func TestEncodeTORRejectsMissingManifestReferencedTOC(t *testing.T) {
	manifest := []byte(`{"name":"demo","version":"1.0.0","contracts":[{"name":"Demo","toc":"bytecode/Demo.toc"}]}`)
	if _, err := EncodeTOR(manifest, map[string][]byte{
		"interfaces/IDemo.toi": []byte("i"),
	}); err == nil {
		t.Fatalf("expected missing manifest referenced toc error")
	}
}

func TestEncodeTORRejectsMissingManifestReferencedTOI(t *testing.T) {
	manifest := []byte(`{"name":"demo","version":"1.0.0","contracts":[{"name":"Demo","toi":"interfaces/IDemo.toi"}]}`)
	if _, err := EncodeTOR(manifest, map[string][]byte{
		"bytecode/Demo.toc": mustCompileTOCTestArtifact(t),
	}); err == nil {
		t.Fatalf("expected missing manifest referenced toi error")
	}
}

func TestDecodeTORRejectsMissingManifest(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("bytecode/Demo.toc")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := w.Write([]byte("x")); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if _, err := DecodeTOR(buf.Bytes()); err == nil {
		t.Fatalf("expected missing manifest error")
	}
}

func TestDecodeTORRejectsInvalidManifestJSON(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	mw, err := zw.Create("manifest.json")
	if err != nil {
		t.Fatalf("create manifest entry: %v", err)
	}
	if _, err := mw.Write([]byte("{")); err != nil {
		t.Fatalf("write manifest entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if _, err := DecodeTOR(buf.Bytes()); err == nil {
		t.Fatalf("expected invalid manifest json error")
	}
}

func TestDecodeTORRejectsManifestMissingRequiredFields(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	mw, err := zw.Create("manifest.json")
	if err != nil {
		t.Fatalf("create manifest entry: %v", err)
	}
	if _, err := mw.Write([]byte(`{"name":"demo"}`)); err != nil {
		t.Fatalf("write manifest entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if _, err := DecodeTOR(buf.Bytes()); err == nil {
		t.Fatalf("expected missing version error")
	}
}

func TestDecodeTORRejectsManifestReferencedMissingFile(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	mw, err := zw.Create("manifest.json")
	if err != nil {
		t.Fatalf("create manifest entry: %v", err)
	}
	if _, err := mw.Write([]byte(`{"name":"demo","version":"1.0.0","contracts":[{"name":"Demo","toc":"bytecode/Demo.toc"}]}`)); err != nil {
		t.Fatalf("write manifest entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if _, err := DecodeTOR(buf.Bytes()); err == nil {
		t.Fatalf("expected manifest referenced missing file error")
	}
}

func TestDecodeTORRejectsInvalidTOCEntry(t *testing.T) {
	manifest := []byte(`{"name":"demo","version":"1.0.0","contracts":[{"name":"Demo","toc":"bytecode/Demo.toc"}]}`)
	tor, err := EncodeTOR(manifest, map[string][]byte{
		"bytecode/Demo.toc": []byte("not-a-toc"),
	})
	if err != nil {
		t.Fatalf("encode tor: %v", err)
	}
	if _, err := DecodeTOR(tor); err == nil {
		t.Fatalf("expected invalid toc entry error")
	}
}

func TestDecodeTORRejectsInvalidTOIEntry(t *testing.T) {
	manifest := []byte(`{"name":"demo","version":"1.0.0","contracts":[{"name":"Demo","toi":"interfaces/IDemo.toi"}]}`)
	tor, err := EncodeTOR(manifest, map[string][]byte{
		"interfaces/IDemo.toi": []byte("not-a-toi"),
	})
	if err != nil {
		t.Fatalf("encode tor: %v", err)
	}
	if _, err := DecodeTOR(tor); err == nil {
		t.Fatalf("expected invalid toi entry error")
	}
}

func TestTORPackageHashStable(t *testing.T) {
	manifest := []byte(`{"name":"demo","version":"1.0.0"}`)
	files := map[string][]byte{"bytecode/Demo.toc": []byte("x")}
	torA, err := EncodeTOR(manifest, files)
	if err != nil {
		t.Fatalf("encode torA: %v", err)
	}
	torB, err := EncodeTOR(manifest, files)
	if err != nil {
		t.Fatalf("encode torB: %v", err)
	}
	if TORPackageHash(torA) != TORPackageHash(torB) {
		t.Fatalf("expected stable tor package hash")
	}
}

func TestCompileTOLToTORRoundTrip(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn ping() public { return; }
}
`)
	tor, err := CompileTOLToTOR(src, "demo.tol", &TORCompileOptions{
		PackageName:    "demo",
		PackageVersion: "1.0.0",
		IncludeSource:  true,
	})
	if err != nil {
		t.Fatalf("compile tor: %v", err)
	}
	decoded, err := DecodeTOR(tor)
	if err != nil {
		t.Fatalf("decode tor: %v", err)
	}
	var manifest struct {
		Name      string `json:"name"`
		Version   string `json:"version"`
		Contracts []struct {
			Name string `json:"name"`
			TOC  string `json:"toc"`
			TOI  string `json:"toi"`
		} `json:"contracts"`
	}
	if err := json.Unmarshal(decoded.ManifestJSON, &manifest); err != nil {
		t.Fatalf("manifest decode: %v", err)
	}
	if manifest.Name != "demo" || manifest.Version != "1.0.0" {
		t.Fatalf("unexpected manifest identity: %+v", manifest)
	}
	if len(manifest.Contracts) != 1 {
		t.Fatalf("unexpected contract entries: %d", len(manifest.Contracts))
	}
	ref := manifest.Contracts[0]
	if _, ok := decoded.Files[ref.TOC]; !ok {
		t.Fatalf("missing referenced toc file: %s", ref.TOC)
	}
	if _, ok := decoded.Files[ref.TOI]; !ok {
		t.Fatalf("missing referenced toi file: %s", ref.TOI)
	}
	if got := string(decoded.Files["sources/demo.tol"]); got == "" {
		t.Fatalf("expected included source file")
	}
}

func TestCompileTOLToTORDeterministic(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn ping() public { return; }
}
`)
	opts := &TORCompileOptions{
		PackageName:    "demo",
		PackageVersion: "1.0.0",
		IncludeSource:  true,
	}
	a, err := CompileTOLToTOR(src, "demo.tol", opts)
	if err != nil {
		t.Fatalf("compile tor a: %v", err)
	}
	b, err := CompileTOLToTOR(src, "demo.tol", opts)
	if err != nil {
		t.Fatalf("compile tor b: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("expected deterministic tor output")
	}
}

func TestCompileTOLToTORCustomPaths(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn ping() public { return; }
}
`)
	tor, err := CompileTOLToTOR(src, "demo.tol", &TORCompileOptions{
		PackageName:    "demo",
		PackageVersion: "1.2.3",
		TOCPath:        "artifacts/Demo.toc",
		TOIPath:        "abi/IDemo.toi",
		IncludeSource:  true,
		SourcePath:     "src/demo.tol",
	})
	if err != nil {
		t.Fatalf("compile tor: %v", err)
	}
	decoded, err := DecodeTOR(tor)
	if err != nil {
		t.Fatalf("decode tor: %v", err)
	}
	if _, ok := decoded.Files["artifacts/Demo.toc"]; !ok {
		t.Fatalf("missing custom toc path")
	}
	if _, ok := decoded.Files["abi/IDemo.toi"]; !ok {
		t.Fatalf("missing custom toi path")
	}
	if _, ok := decoded.Files["src/demo.tol"]; !ok {
		t.Fatalf("missing custom source path")
	}
}

func TestCompileTOLToTORDefaultNoSource(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn ping() public { return; }
}
`)
	tor, err := CompileTOLToTOR(src, "demo.tol", nil)
	if err != nil {
		t.Fatalf("compile tor: %v", err)
	}
	decoded, err := DecodeTOR(tor)
	if err != nil {
		t.Fatalf("decode tor: %v", err)
	}
	for name := range decoded.Files {
		if strings.HasPrefix(name, "sources/") {
			t.Fatalf("unexpected source entry in default mode: %s", name)
		}
	}
}
