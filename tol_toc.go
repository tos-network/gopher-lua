package lua

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	tolast "github.com/tos-network/tolang/tol/ast"
	"golang.org/x/crypto/sha3"
)

var tocMagic = [4]byte{'T', 'O', 'C', 0}

// TOCFormatVersion is the binary format version for .toc artifacts.
const TOCFormatVersion uint16 = 1

// TOCArtifact is a decoded .toc payload.
type TOCArtifact struct {
	Version           uint16
	Compiler          string
	ContractName      string
	Bytecode          []byte
	ABIJSON           []byte
	StorageLayoutJSON []byte
	SourceHash        string
	BytecodeHash      string
}

type tocABI struct {
	Functions []tocABIFunction `json:"functions"`
	Events    []tocABIEvent    `json:"events"`
}

type tocABIFunction struct {
	Name       string   `json:"name"`
	Visibility string   `json:"visibility"`
	Selector   string   `json:"selector"`
	Params     []string `json:"params,omitempty"`
	Returns    []string `json:"returns,omitempty"`
}

type tocABIEvent struct {
	Name   string   `json:"name"`
	Params []string `json:"params,omitempty"`
}

type tocStorageLayout struct {
	Slots []tocStorageSlot `json:"slots"`
}

type tocStorageSlot struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	CanonicalHash string `json:"canonical_hash"`
}

// IsTOC reports whether the input starts with .toc magic bytes.
func IsTOC(data []byte) bool {
	if len(data) < len(tocMagic) {
		return false
	}
	for i := range tocMagic {
		if data[i] != tocMagic[i] {
			return false
		}
	}
	return true
}

// CompileTOLToTOC compiles TOL source into a .toc artifact.
func CompileTOLToTOC(source []byte, name string) ([]byte, error) {
	bytecode, err := CompileTOLToBytecode(source, name)
	if err != nil {
		return nil, err
	}
	mod, err := ParseTOLModule(source, name)
	if err != nil {
		return nil, err
	}
	contractName, abiJSON, storageJSON, err := buildTOCMetadata(mod)
	if err != nil {
		return nil, err
	}
	return EncodeTOC(&TOCArtifact{
		Version:           TOCFormatVersion,
		Compiler:          "tolang/" + PackageVersion,
		ContractName:      contractName,
		Bytecode:          bytecode,
		ABIJSON:           abiJSON,
		StorageLayoutJSON: storageJSON,
		SourceHash:        keccak256Hex(source),
		BytecodeHash:      keccak256Hex(bytecode),
	})
}

// EncodeTOC serializes a TOC artifact into deterministic binary bytes.
func EncodeTOC(a *TOCArtifact) ([]byte, error) {
	if a == nil {
		return nil, fmt.Errorf("nil toc artifact")
	}
	if strings.TrimSpace(a.ContractName) == "" {
		return nil, fmt.Errorf("toc contract name is required")
	}
	if len(a.Bytecode) == 0 {
		return nil, fmt.Errorf("toc bytecode is required")
	}
	version := a.Version
	if version == 0 {
		version = TOCFormatVersion
	}
	if a.Compiler == "" {
		a.Compiler = "tolang/" + PackageVersion
	}
	sourceHash, err := decodeHashHex(a.SourceHash)
	if err != nil {
		return nil, fmt.Errorf("invalid source hash: %w", err)
	}
	bytecodeHash, err := decodeHashHex(a.BytecodeHash)
	if err != nil {
		return nil, fmt.Errorf("invalid bytecode hash: %w", err)
	}

	var buf bytes.Buffer
	buf.Write(tocMagic[:])
	if err := writeU16(&buf, version); err != nil {
		return nil, err
	}
	if err := writeString(&buf, a.Compiler); err != nil {
		return nil, err
	}
	if err := writeString(&buf, strings.TrimSpace(a.ContractName)); err != nil {
		return nil, err
	}
	if err := writeLenBytes(&buf, a.Bytecode); err != nil {
		return nil, err
	}
	if err := writeLenBytes(&buf, a.ABIJSON); err != nil {
		return nil, err
	}
	if err := writeLenBytes(&buf, a.StorageLayoutJSON); err != nil {
		return nil, err
	}
	if _, err := buf.Write(sourceHash); err != nil {
		return nil, err
	}
	if _, err := buf.Write(bytecodeHash); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// DecodeTOC deserializes a .toc payload into a structured artifact.
func DecodeTOC(data []byte) (*TOCArtifact, error) {
	r := &byteReader{b: data}
	var magic [4]byte
	if _, err := io.ReadFull(r, magic[:]); err != nil {
		return nil, fmt.Errorf("invalid toc header: %w", err)
	}
	if magic != tocMagic {
		return nil, fmt.Errorf("invalid toc magic")
	}
	version, err := readU16(r)
	if err != nil {
		return nil, fmt.Errorf("invalid toc version: %w", err)
	}
	if version != TOCFormatVersion {
		return nil, fmt.Errorf("unsupported toc version: got=%d want=%d", version, TOCFormatVersion)
	}
	compiler, err := readString(r)
	if err != nil {
		return nil, fmt.Errorf("invalid toc compiler: %w", err)
	}
	contractName, err := readString(r)
	if err != nil {
		return nil, fmt.Errorf("invalid toc contract name: %w", err)
	}
	bytecode, err := readLenBytes(r)
	if err != nil {
		return nil, fmt.Errorf("invalid toc bytecode payload: %w", err)
	}
	abiJSON, err := readLenBytes(r)
	if err != nil {
		return nil, fmt.Errorf("invalid toc abi payload: %w", err)
	}
	storageJSON, err := readLenBytes(r)
	if err != nil {
		return nil, fmt.Errorf("invalid toc storage payload: %w", err)
	}
	sourceHash, err := readFixedBytes(r, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid toc source hash: %w", err)
	}
	bytecodeHash, err := readFixedBytes(r, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid toc bytecode hash: %w", err)
	}
	if r.n != len(data) {
		return nil, fmt.Errorf("trailing bytes in toc payload")
	}
	if strings.TrimSpace(contractName) == "" {
		return nil, fmt.Errorf("toc contract name is empty")
	}
	if len(bytecode) == 0 {
		return nil, fmt.Errorf("toc bytecode payload is empty")
	}
	gotBytecodeHash := keccak256Bytes(bytecode)
	if !bytes.Equal(gotBytecodeHash, bytecodeHash) {
		return nil, fmt.Errorf("toc bytecode hash mismatch")
	}
	if _, err := DecodeFunctionProto(bytecode); err != nil {
		return nil, fmt.Errorf("toc embedded bytecode decode failed: %w", err)
	}
	return &TOCArtifact{
		Version:           version,
		Compiler:          compiler,
		ContractName:      contractName,
		Bytecode:          bytecode,
		ABIJSON:           abiJSON,
		StorageLayoutJSON: storageJSON,
		SourceHash:        "0x" + hex.EncodeToString(sourceHash),
		BytecodeHash:      "0x" + hex.EncodeToString(bytecodeHash),
	}, nil
}

// VerifyTOCSourceHash checks whether a decoded TOC artifact matches the given source bytes.
func VerifyTOCSourceHash(toc *TOCArtifact, source []byte) error {
	if toc == nil {
		return fmt.Errorf("nil toc artifact")
	}
	want := keccak256Hex(source)
	got := strings.ToLower(strings.TrimSpace(toc.SourceHash))
	if got != want {
		return fmt.Errorf("toc source hash mismatch: got=%s want=%s", toc.SourceHash, want)
	}
	return nil
}

func buildTOCMetadata(mod *tolast.Module) (string, []byte, []byte, error) {
	if mod == nil || mod.Contract == nil {
		return "", nil, nil, fmt.Errorf("toc metadata requires a contract module")
	}
	contractName := strings.TrimSpace(mod.Contract.Name)
	if contractName == "" {
		return "", nil, nil, fmt.Errorf("toc metadata requires contract name")
	}

	abi := tocABI{
		Functions: make([]tocABIFunction, 0, len(mod.Contract.Functions)),
		Events:    make([]tocABIEvent, 0, len(mod.Contract.Events)),
	}
	for _, fn := range mod.Contract.Functions {
		vis := functionVisibilityFromModifiers(fn.Modifiers)
		if vis != "public" && vis != "external" {
			continue
		}
		paramTypes := make([]string, 0, len(fn.Params))
		for _, p := range fn.Params {
			paramTypes = append(paramTypes, normalizeTOCType(p.Type))
		}
		returnTypes := make([]string, 0, len(fn.Returns))
		for _, r := range fn.Returns {
			returnTypes = append(returnTypes, normalizeTOCType(r.Type))
		}
		selector := strings.ToLower(strings.TrimSpace(fn.SelectorOverride))
		if selector == "" {
			selector = selectorHexFromSignatureForTOC(fn.Name, paramTypes)
		}
		abi.Functions = append(abi.Functions, tocABIFunction{
			Name:       fn.Name,
			Visibility: vis,
			Selector:   selector,
			Params:     paramTypes,
			Returns:    returnTypes,
		})
	}
	for _, ev := range mod.Contract.Events {
		paramTypes := make([]string, 0, len(ev.Params))
		for _, p := range ev.Params {
			paramTypes = append(paramTypes, normalizeTOCType(p.Type))
		}
		abi.Events = append(abi.Events, tocABIEvent{
			Name:   ev.Name,
			Params: paramTypes,
		})
	}
	storage := tocStorageLayout{
		Slots: make([]tocStorageSlot, 0),
	}
	if mod.Contract.Storage != nil {
		storage.Slots = make([]tocStorageSlot, 0, len(mod.Contract.Storage.Slots))
		for _, s := range mod.Contract.Storage.Slots {
			name := strings.TrimSpace(s.Name)
			typ := normalizeTOCType(s.Type)
			storage.Slots = append(storage.Slots, tocStorageSlot{
				Name:          name,
				Type:          typ,
				CanonicalHash: keccak256Hex([]byte(fmt.Sprintf("tol.slot.%s.%s", contractName, name))),
			})
		}
	}

	abiJSON, err := json.Marshal(abi)
	if err != nil {
		return "", nil, nil, err
	}
	storageJSON, err := json.Marshal(storage)
	if err != nil {
		return "", nil, nil, err
	}
	return contractName, abiJSON, storageJSON, nil
}

func functionVisibilityFromModifiers(modifiers []string) string {
	vis := ""
	for _, m := range modifiers {
		switch m {
		case "public", "external", "internal", "private":
			vis = m
		}
	}
	return vis
}

func normalizeTOCType(t string) string {
	s := strings.Join(strings.Fields(t), " ")
	repl := strings.NewReplacer(
		"( ", "(",
		" )", ")",
		"[ ", "[",
		" ]", "]",
		" ,", ",",
		", ", ",",
		" => ", "=>",
		" =>", "=>",
		"=> ", "=>",
	)
	return repl.Replace(s)
}

func selectorHexFromSignatureForTOC(name string, paramTypes []string) string {
	sig := fmt.Sprintf("%s(%s)", strings.TrimSpace(name), strings.Join(paramTypes, ","))
	sum := keccak256(sig)
	return "0x" + hex.EncodeToString(sum[:4])
}

func keccak256Hex(data []byte) string {
	sum := keccak256Bytes(data)
	return "0x" + hex.EncodeToString(sum)
}

func keccak256(s string) []byte {
	return keccak256Bytes([]byte(s))
}

func keccak256Bytes(data []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write(data)
	return h.Sum(nil)
}

func writeLenBytes(w io.Writer, b []byte) error {
	if err := writeU32(w, uint32(len(b))); err != nil {
		return err
	}
	if len(b) == 0 {
		return nil
	}
	_, err := w.Write(b)
	return err
}

func readLenBytes(r *byteReader) ([]byte, error) {
	n, err := readU32(r)
	if err != nil {
		return nil, err
	}
	if int(n) < 0 || int(n) > len(r.b)-r.n {
		return nil, io.ErrUnexpectedEOF
	}
	out := make([]byte, int(n))
	copy(out, r.b[r.n:r.n+int(n)])
	r.n += int(n)
	return out, nil
}

func readFixedBytes(r *byteReader, n int) ([]byte, error) {
	if n < 0 || r.n+n > len(r.b) {
		return nil, io.ErrUnexpectedEOF
	}
	out := make([]byte, n)
	copy(out, r.b[r.n:r.n+n])
	r.n += n
	return out, nil
}

func decodeHashHex(v string) ([]byte, error) {
	s := strings.TrimSpace(strings.ToLower(v))
	if s == "" {
		return nil, fmt.Errorf("empty hash")
	}
	if !strings.HasPrefix(s, "0x") {
		return nil, fmt.Errorf("hash must start with 0x")
	}
	raw := s[2:]
	if len(raw) != 64 {
		return nil, fmt.Errorf("hash must be 32 bytes")
	}
	out, err := hex.DecodeString(raw)
	if err != nil {
		return nil, err
	}
	return out, nil
}
