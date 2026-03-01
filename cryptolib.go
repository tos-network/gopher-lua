package lua

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"golang.org/x/crypto/sha3"
)

// openCrypto registers deterministic crypto builtins as Lua globals.
// These are required for TOL canonical storage key derivation (spec §8.3).
func openCrypto(L *LState) {
	L.SetGlobal("keccak256", L.NewFunction(cryptoKeccak256))
	L.SetGlobal("__tol_enc", L.NewFunction(cryptoTolEnc))
	L.SetGlobal("uint256_add_hex", L.NewFunction(cryptoUint256AddHex))
}

// cryptoKeccak256 implements keccak256(hex_input: string) -> bytes32_hex.
// hex_input must be "0x" followed by even hex chars; the raw bytes are hashed.
// Returns "0x" + 64 hex chars.
func cryptoKeccak256(L *LState) int {
	s := strings.TrimSpace(L.CheckString(1))
	if !strings.HasPrefix(s, "0x") && !strings.HasPrefix(s, "0X") {
		L.RaiseError("keccak256: input must start with 0x, got: %q", s)
	}
	data, err := hex.DecodeString(s[2:])
	if err != nil {
		L.RaiseError("keccak256: invalid hex input: %s", err)
	}
	h := sha3.NewLegacyKeccak256()
	h.Write(data)
	L.Push(LString("0x" + hex.EncodeToString(h.Sum(nil))))
	return 1
}

// cryptoTolEnc implements __tol_enc(value) -> 64-char hex string (no 0x, 32 bytes).
// Encodes a TOL key value for canonical mapping slot derivation (spec §8.3):
//   - LAddress / LString "0x...": hex decode, right-align in 32 bytes
//   - LNumber / LString decimal: big-endian 32-byte u256
//   - LBool: 32 zero bytes with LSB = 1 (true) or 0 (false)
func cryptoTolEnc(L *LState) int {
	v := L.CheckAny(1)
	encoded, err := tolEncodeKey(v)
	if err != nil {
		L.RaiseError("__tol_enc: %s", err)
	}
	L.Push(LString(encoded))
	return 1
}

// cryptoUint256AddHex implements uint256_add_hex(base_hex, offset) -> bytes32_hex.
// Adds a non-negative integer offset to a hex-encoded u256, wrapping mod 2^256.
// Used for array element slot computation: H(base_slot) + index.
func cryptoUint256AddHex(L *LState) int {
	baseStr := strings.TrimSpace(L.CheckString(1))
	if strings.HasPrefix(baseStr, "0x") || strings.HasPrefix(baseStr, "0X") {
		baseStr = baseStr[2:]
	}
	base, ok := new(big.Int).SetString(baseStr, 16)
	if !ok {
		L.RaiseError("uint256_add_hex: invalid hex base: %q", baseStr)
	}
	var offset *big.Int
	switch v := L.CheckAny(2).(type) {
	case LNumber:
		offset, ok = new(big.Int).SetString(string(v), 10)
		if !ok {
			L.RaiseError("uint256_add_hex: invalid LNumber offset: %q", string(v))
		}
	case LString:
		offset, ok = new(big.Int).SetString(strings.TrimSpace(string(v)), 10)
		if !ok {
			L.RaiseError("uint256_add_hex: invalid string offset: %q", string(v))
		}
	default:
		L.RaiseError("uint256_add_hex: unsupported offset type %T", v)
	}
	result := new(big.Int).And(new(big.Int).Add(base, offset), uint256Max)
	L.Push(LString(fmt.Sprintf("0x%064x", result)))
	return 1
}

// tolEncodeKey encodes a Lua value to a 64-char hex string (no 0x prefix) for
// use in TOL canonical storage key derivation per spec §8.3.
func tolEncodeKey(v LValue) (string, error) {
	var buf [32]byte
	switch val := v.(type) {
	case LBool:
		if bool(val) {
			buf[31] = 1
		}
		return hex.EncodeToString(buf[:]), nil
	case LAddress:
		return encodeHexTo32(string(val))
	case LString:
		s := strings.TrimSpace(string(val))
		if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
			return encodeHexTo32(s)
		}
		return encodeDecimalTo32(s)
	case LNumber:
		return encodeDecimalTo32(string(val))
	default:
		return "", fmt.Errorf("unsupported key type %T", v)
	}
}

func encodeHexTo32(s string) (string, error) {
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		s = s[2:]
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return "", fmt.Errorf("invalid hex value: %s", err)
	}
	if len(b) > 32 {
		return "", fmt.Errorf("hex value exceeds 32 bytes (%d bytes)", len(b))
	}
	var buf [32]byte
	copy(buf[32-len(b):], b)
	return hex.EncodeToString(buf[:]), nil
}

func encodeDecimalTo32(s string) (string, error) {
	n, ok := new(big.Int).SetString(strings.TrimSpace(s), 10)
	if !ok {
		return "", fmt.Errorf("cannot parse as u256 decimal: %q", s)
	}
	if n.Sign() < 0 {
		return "", fmt.Errorf("negative values not supported in key encoding")
	}
	b := n.Bytes()
	if len(b) > 32 {
		return "", fmt.Errorf("value overflows 32 bytes")
	}
	var buf [32]byte
	copy(buf[32-len(b):], b)
	return hex.EncodeToString(buf[:]), nil
}
