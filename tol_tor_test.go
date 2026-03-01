package lua

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestEncodeDecodeTORRoundTrip(t *testing.T) {
	manifest := []byte(`{"name":"demo","version":"1.0.0","contracts":[{"name":"Demo","toc":"bytecode/Demo.toc"}]}`)
	files := map[string][]byte{
		"bytecode/Demo.toc":    []byte("toc-bytes"),
		"interfaces/IDemo.toi": []byte("interface-bytes"),
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
	if got := string(decoded.Files["bytecode/Demo.toc"]); got != "toc-bytes" {
		t.Fatalf("unexpected bytecode entry: %q", got)
	}
	if got := string(decoded.Files["interfaces/IDemo.toi"]); got != "interface-bytes" {
		t.Fatalf("unexpected interface entry: %q", got)
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
