package lua

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/tos-network/glua/parse"
)

func TestIRPipelineAndBytecodeRoundTrip(t *testing.T) {
	src := `
local x = (2 ^ 8) + (0xF0 | 0x0F)
local t = {3, 4}
_result = x + t[1]
`
	chunk, err := parse.Parse(strings.NewReader(src), "<ir>")
	if err != nil {
		t.Fatal(err)
	}

	irp, err := BuildIR(chunk, "<ir>")
	if err != nil {
		t.Fatal(err)
	}
	if irp == nil || irp.Root == nil || len(irp.Root.Instructions) == 0 {
		t.Fatal("expected non-empty IR")
	}

	proto, err := CompileIR(irp)
	if err != nil {
		t.Fatal(err)
	}
	if len(proto.Code) == 0 {
		t.Fatal("expected non-empty bytecode")
	}

	bc, err := EncodeFunctionProto(proto)
	if err != nil {
		t.Fatal(err)
	}
	if len(bc) == 0 {
		t.Fatal("expected non-empty encoded bytecode")
	}

	L := NewState()
	defer L.Close()
	if err := L.DoBytecode(bc); err != nil {
		t.Fatalf("DoBytecode failed: %v", err)
	}

	got := LVAsString(L.GetGlobal("_result"))
	if got != "514" {
		t.Fatalf("unexpected result: got=%s want=514", got)
	}
}

func TestCompileSourceToBytecodeAndExecute(t *testing.T) {
	src := []byte(`_result = (100 // 3) + (7 % 5)`)
	bc, err := CompileSourceToBytecode(src, "<src>")
	if err != nil {
		t.Fatal(err)
	}

	L := NewState()
	defer L.Close()
	if err := L.DoBytecode(bc); err != nil {
		t.Fatal(err)
	}

	if got := LVAsString(L.GetGlobal("_result")); got != "35" {
		t.Fatalf("unexpected result: got=%s want=35", got)
	}
}

func TestDecodeFunctionProtoRejectsInvalidMagic(t *testing.T) {
	if _, err := DecodeFunctionProto([]byte("not-bytecode")); err == nil {
		t.Fatal("expected decode error for invalid magic")
	}
}

func TestLoadAutoDetectsBytecode(t *testing.T) {
	src := []byte(`_result = (5 << 8) + 7`)
	bc, err := CompileSourceToBytecode(src, "<bc>")
	if err != nil {
		t.Fatal(err)
	}
	if !IsBytecode(bc) {
		t.Fatal("expected IsBytecode=true")
	}

	L := NewState()
	defer L.Close()
	fn, err := L.Load(bytes.NewReader(bc), "<bc>")
	if err != nil {
		t.Fatal(err)
	}
	L.Push(fn)
	if err := L.PCall(0, MultRet, nil); err != nil {
		t.Fatal(err)
	}
	if got := LVAsString(L.GetGlobal("_result")); got != "1287" {
		t.Fatalf("unexpected result: got=%s want=1287", got)
	}
}

func TestLoadSourceStripsShebang(t *testing.T) {
	src := []byte("#!/usr/bin/env glua\n_result = 9 + 1\n")
	L := NewState()
	defer L.Close()
	fn, err := L.Load(bytes.NewReader(src), "<src>")
	if err != nil {
		t.Fatal(err)
	}
	L.Push(fn)
	if err := L.PCall(0, MultRet, nil); err != nil {
		t.Fatal(err)
	}
	if got := LVAsString(L.GetGlobal("_result")); got != "10" {
		t.Fatalf("unexpected result: got=%s want=10", got)
	}
}

func TestDecodeRejectsTamperedChecksum(t *testing.T) {
	bc, err := CompileSourceToBytecode([]byte(`_result = 1 + 2`), "<sum>")
	if err != nil {
		t.Fatal(err)
	}
	bc[len(bc)-1] ^= 0xFF
	if _, err := DecodeFunctionProto(bc); err == nil {
		t.Fatal("expected checksum mismatch error")
	}
}

func TestDecodeRejectsMismatchedVMID(t *testing.T) {
	bc, err := CompileSourceToBytecode([]byte(`_result = 11`), "<vmid>")
	if err != nil {
		t.Fatal(err)
	}
	// Layout: magic(4) + version(2) + vmidLen(4) + vmid + payloadLen(4) + payload + hash(32)
	const header = 4 + 2
	vmLen := int(binary.BigEndian.Uint32(bc[header : header+4]))
	vmStart := header + 4
	if vmLen == 0 || vmStart+vmLen > len(bc) {
		t.Fatal("unexpected bytecode vmid layout")
	}
	bc[vmStart] ^= 0x01 // alter vmid only (payload hash unchanged)
	if _, err := DecodeFunctionProto(bc); err == nil {
		t.Fatal("expected vm mismatch error")
	}
}

func TestDecodeRejectsLegacyHeaderLayout(t *testing.T) {
	src := []byte(`_result = 1`)
	bc, err := CompileSourceToBytecode(src, "<legacy>")
	if err != nil {
		t.Fatal(err)
	}
	// Mutate the version to v1 and drop v2 header fields to mimic old layout.
	var legacy bytes.Buffer
	legacy.Write([]byte{'G', 'L', 'B', 'C', 0x01, 0x00})
	// append only payload section (skip new header and checksum)
	const hdr = 4 + 2
	vmLen := int(binary.BigEndian.Uint32(bc[hdr : hdr+4]))
	payloadLenOff := hdr + 4 + vmLen
	payloadLen := int(binary.BigEndian.Uint32(bc[payloadLenOff : payloadLenOff+4]))
	payloadOff := payloadLenOff + 4
	legacy.Write(bc[payloadOff : payloadOff+payloadLen])
	if _, err := DecodeFunctionProto(legacy.Bytes()); err == nil {
		t.Fatal("expected legacy format rejection")
	}
}
