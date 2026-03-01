package lua

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestCompileTOLToTOCRoundTrip(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  storage {
    slot total: u256;
    slot balances: mapping(address => u256);
  }

  event Tick(v: u256);

  fn ping(owner: address, amount: u256) public {
    return;
  }
}
`)
	toc, err := CompileTOLToTOC(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected toc compile error: %v", err)
	}
	if !IsTOC(toc) {
		t.Fatalf("expected toc magic")
	}

	art, err := DecodeTOC(toc)
	if err != nil {
		t.Fatalf("unexpected toc decode error: %v", err)
	}
	if art.Version != TOCFormatVersion {
		t.Fatalf("unexpected toc version: got=%d want=%d", art.Version, TOCFormatVersion)
	}
	if art.ContractName != "Demo" {
		t.Fatalf("unexpected contract name: %q", art.ContractName)
	}
	if art.SourceHash != keccak256Hex(src) {
		t.Fatalf("unexpected source hash: got=%s want=%s", art.SourceHash, keccak256Hex(src))
	}
	if art.BytecodeHash != keccak256Hex(art.Bytecode) {
		t.Fatalf("unexpected bytecode hash: got=%s want=%s", art.BytecodeHash, keccak256Hex(art.Bytecode))
	}
	if _, err := DecodeFunctionProto(art.Bytecode); err != nil {
		t.Fatalf("decoded toc contains invalid bytecode: %v", err)
	}

	var abi struct {
		Functions []struct {
			Name       string   `json:"name"`
			Visibility string   `json:"visibility"`
			Selector   string   `json:"selector"`
			Params     []string `json:"params"`
		} `json:"functions"`
		Events []struct {
			Name string `json:"name"`
		} `json:"events"`
	}
	if err := json.Unmarshal(art.ABIJSON, &abi); err != nil {
		t.Fatalf("invalid abi json: %v", err)
	}
	if len(abi.Functions) != 1 {
		t.Fatalf("unexpected abi function count: %d", len(abi.Functions))
	}
	if abi.Functions[0].Name != "ping" {
		t.Fatalf("unexpected function name: %q", abi.Functions[0].Name)
	}
	wantSel := selectorHexFromSignatureForTOC("ping", []string{"address", "u256"})
	if abi.Functions[0].Selector != wantSel {
		t.Fatalf("unexpected function selector: got=%s want=%s", abi.Functions[0].Selector, wantSel)
	}
	if len(abi.Events) != 1 || abi.Events[0].Name != "Tick" {
		t.Fatalf("unexpected abi events: %+v", abi.Events)
	}

	var storage struct {
		Slots []struct {
			Name          string `json:"name"`
			Type          string `json:"type"`
			CanonicalHash string `json:"canonical_hash"`
		} `json:"slots"`
	}
	if err := json.Unmarshal(art.StorageLayoutJSON, &storage); err != nil {
		t.Fatalf("invalid storage json: %v", err)
	}
	if len(storage.Slots) != 2 {
		t.Fatalf("unexpected storage slot count: %d", len(storage.Slots))
	}
	if storage.Slots[0].Name != "total" || storage.Slots[0].Type != "u256" {
		t.Fatalf("unexpected first storage slot: %+v", storage.Slots[0])
	}
	wantSlotHash := keccak256Hex([]byte("tol.slot.Demo.total"))
	if storage.Slots[0].CanonicalHash != wantSlotHash {
		t.Fatalf("unexpected slot hash: got=%s want=%s", storage.Slots[0].CanonicalHash, wantSlotHash)
	}
}

func TestCompileTOLToTOCDeterministic(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn ping() public {
    return;
  }
}
`)
	a, err := CompileTOLToTOC(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected compile error (first): %v", err)
	}
	b, err := CompileTOLToTOC(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected compile error (second): %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("expected deterministic toc bytes")
	}
}

func TestEncodeTOCRejectsInvalidHash(t *testing.T) {
	_, err := EncodeTOC(&TOCArtifact{
		Version:      TOCFormatVersion,
		Compiler:     "tolang/" + PackageVersion,
		ContractName: "Demo",
		Bytecode:     []byte{1, 2, 3},
		SourceHash:   "0x1234",
		BytecodeHash: keccak256Hex([]byte{1, 2, 3}),
	})
	if err == nil {
		t.Fatalf("expected invalid hash error")
	}
}

func TestDecodeTOCRejectsBytecodeHashMismatch(t *testing.T) {
	toc, err := EncodeTOC(&TOCArtifact{
		Version:      TOCFormatVersion,
		Compiler:     "tolang/" + PackageVersion,
		ContractName: "Demo",
		Bytecode:     []byte{1, 2, 3},
		SourceHash:   keccak256Hex([]byte("src")),
		BytecodeHash: keccak256Hex([]byte{9}),
	})
	if err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}
	if _, err := DecodeTOC(toc); err == nil {
		t.Fatalf("expected bytecode hash mismatch")
	}
}

func TestDecodeTOCRejectsUnsupportedVersion(t *testing.T) {
	toc, err := EncodeTOC(&TOCArtifact{
		Version:      TOCFormatVersion + 1,
		Compiler:     "tolang/" + PackageVersion,
		ContractName: "Demo",
		Bytecode:     []byte{1},
		SourceHash:   keccak256Hex([]byte("src")),
		BytecodeHash: keccak256Hex([]byte{1}),
	})
	if err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}
	if _, err := DecodeTOC(toc); err == nil {
		t.Fatalf("expected unsupported version error")
	}
}

func TestDecodeTOCRejectsInvalidEmbeddedBytecode(t *testing.T) {
	toc, err := EncodeTOC(&TOCArtifact{
		Version:      TOCFormatVersion,
		Compiler:     "tolang/" + PackageVersion,
		ContractName: "Demo",
		Bytecode:     []byte{1, 2, 3},
		SourceHash:   keccak256Hex([]byte("src")),
		BytecodeHash: keccak256Hex([]byte{1, 2, 3}),
	})
	if err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}
	if _, err := DecodeTOC(toc); err == nil {
		t.Fatalf("expected invalid embedded bytecode error")
	}
}

func TestDecodeTOCRejectsEmptyContractName(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn ping() public { return; }
}
`)
	bytecode, err := CompileTOLToBytecode(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}
	var raw bytes.Buffer
	raw.Write(tocMagic[:])
	if err := writeU16(&raw, TOCFormatVersion); err != nil {
		t.Fatalf("write version: %v", err)
	}
	if err := writeString(&raw, "tolang/"+PackageVersion); err != nil {
		t.Fatalf("write compiler: %v", err)
	}
	if err := writeString(&raw, ""); err != nil {
		t.Fatalf("write contract: %v", err)
	}
	if err := writeLenBytes(&raw, bytecode); err != nil {
		t.Fatalf("write bytecode: %v", err)
	}
	if err := writeLenBytes(&raw, nil); err != nil {
		t.Fatalf("write abi: %v", err)
	}
	if err := writeLenBytes(&raw, nil); err != nil {
		t.Fatalf("write storage: %v", err)
	}
	if _, err := raw.Write(make([]byte, 32)); err != nil {
		t.Fatalf("write source hash: %v", err)
	}
	if _, err := raw.Write(keccak256Bytes(bytecode)); err != nil {
		t.Fatalf("write bytecode hash: %v", err)
	}
	if _, err := DecodeTOC(raw.Bytes()); err == nil {
		t.Fatalf("expected empty contract name error")
	}
}

func TestDecodeTOCRejectsEmptyBytecodePayload(t *testing.T) {
	var raw bytes.Buffer
	raw.Write(tocMagic[:])
	if err := writeU16(&raw, TOCFormatVersion); err != nil {
		t.Fatalf("write version: %v", err)
	}
	if err := writeString(&raw, "tolang/"+PackageVersion); err != nil {
		t.Fatalf("write compiler: %v", err)
	}
	if err := writeString(&raw, "Demo"); err != nil {
		t.Fatalf("write contract: %v", err)
	}
	if err := writeLenBytes(&raw, nil); err != nil {
		t.Fatalf("write bytecode: %v", err)
	}
	if err := writeLenBytes(&raw, nil); err != nil {
		t.Fatalf("write abi: %v", err)
	}
	if err := writeLenBytes(&raw, nil); err != nil {
		t.Fatalf("write storage: %v", err)
	}
	if _, err := raw.Write(make([]byte, 32)); err != nil {
		t.Fatalf("write source hash: %v", err)
	}
	if _, err := raw.Write(keccak256Bytes(nil)); err != nil {
		t.Fatalf("write bytecode hash: %v", err)
	}
	if _, err := DecodeTOC(raw.Bytes()); err == nil {
		t.Fatalf("expected empty bytecode error")
	}
}

func TestVerifyTOCSourceHashMatches(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn ping() public { return; }
}
`)
	tocBytes, err := CompileTOLToTOC(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected toc compile error: %v", err)
	}
	toc, err := DecodeTOC(tocBytes)
	if err != nil {
		t.Fatalf("unexpected toc decode error: %v", err)
	}
	if err := VerifyTOCSourceHash(toc, src); err != nil {
		t.Fatalf("unexpected source hash mismatch: %v", err)
	}
}

func TestVerifyTOCSourceHashMismatch(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn ping() public { return; }
}
`)
	tocBytes, err := CompileTOLToTOC(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected toc compile error: %v", err)
	}
	toc, err := DecodeTOC(tocBytes)
	if err != nil {
		t.Fatalf("unexpected toc decode error: %v", err)
	}
	if err := VerifyTOCSourceHash(toc, []byte("other source")); err == nil {
		t.Fatalf("expected source hash mismatch")
	}
}

func TestVerifyTOCSourceHashRejectsNilArtifact(t *testing.T) {
	if err := VerifyTOCSourceHash(nil, []byte("x")); err == nil {
		t.Fatalf("expected nil artifact error")
	}
}

func TestDecodeTOCRejectsInvalidABIJSON(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn ping() public { return; }
}
`)
	bytecode, err := CompileTOLToBytecode(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}
	var raw bytes.Buffer
	raw.Write(tocMagic[:])
	if err := writeU16(&raw, TOCFormatVersion); err != nil {
		t.Fatalf("write version: %v", err)
	}
	if err := writeString(&raw, "tolang/"+PackageVersion); err != nil {
		t.Fatalf("write compiler: %v", err)
	}
	if err := writeString(&raw, "Demo"); err != nil {
		t.Fatalf("write contract: %v", err)
	}
	if err := writeLenBytes(&raw, bytecode); err != nil {
		t.Fatalf("write bytecode: %v", err)
	}
	if err := writeLenBytes(&raw, []byte("{")); err != nil {
		t.Fatalf("write abi: %v", err)
	}
	if err := writeLenBytes(&raw, []byte("{}")); err != nil {
		t.Fatalf("write storage: %v", err)
	}
	if _, err := raw.Write(make([]byte, 32)); err != nil {
		t.Fatalf("write source hash: %v", err)
	}
	if _, err := raw.Write(keccak256Bytes(bytecode)); err != nil {
		t.Fatalf("write bytecode hash: %v", err)
	}
	if _, err := DecodeTOC(raw.Bytes()); err == nil {
		t.Fatalf("expected invalid abi json error")
	}
}

func TestDecodeTOCRejectsInvalidStorageJSON(t *testing.T) {
	src := []byte(`
tol 0.2
contract Demo {
  fn ping() public { return; }
}
`)
	bytecode, err := CompileTOLToBytecode(src, "<tol>")
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}
	var raw bytes.Buffer
	raw.Write(tocMagic[:])
	if err := writeU16(&raw, TOCFormatVersion); err != nil {
		t.Fatalf("write version: %v", err)
	}
	if err := writeString(&raw, "tolang/"+PackageVersion); err != nil {
		t.Fatalf("write compiler: %v", err)
	}
	if err := writeString(&raw, "Demo"); err != nil {
		t.Fatalf("write contract: %v", err)
	}
	if err := writeLenBytes(&raw, bytecode); err != nil {
		t.Fatalf("write bytecode: %v", err)
	}
	if err := writeLenBytes(&raw, []byte("{}")); err != nil {
		t.Fatalf("write abi: %v", err)
	}
	if err := writeLenBytes(&raw, []byte("{")); err != nil {
		t.Fatalf("write storage: %v", err)
	}
	if _, err := raw.Write(make([]byte, 32)); err != nil {
		t.Fatalf("write source hash: %v", err)
	}
	if _, err := raw.Write(keccak256Bytes(bytecode)); err != nil {
		t.Fatalf("write bytecode hash: %v", err)
	}
	if _, err := DecodeTOC(raw.Bytes()); err == nil {
		t.Fatalf("expected invalid storage json error")
	}
}
