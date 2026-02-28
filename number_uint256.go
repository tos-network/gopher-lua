package lua

import (
	"errors"
	"math/big"
	"strconv"
	"strings"
)

var (
	uint256Mod = new(big.Int).Lsh(big.NewInt(1), 256)
	uint256Max = new(big.Int).Sub(new(big.Int).Set(uint256Mod), big.NewInt(1))

	errInvalidNumber  = errors.New("invalid number")
	errNegativeNumber = errors.New("negative numbers are not supported")
	errNumberOverflow = errors.New("number out of uint256 range")
)

func parseUint256(number string) (LNumber, error) {
	s := strings.TrimSpace(number)
	if s == "" {
		return LNumberZero, errInvalidNumber
	}
	v, ok := new(big.Int).SetString(s, 0)
	if !ok {
		return LNumberZero, errInvalidNumber
	}
	if v.Sign() < 0 {
		return LNumberZero, errNegativeNumber
	}
	if v.Cmp(uint256Max) > 0 {
		return LNumberZero, errNumberOverflow
	}
	return LNumber(v.Text(10)), nil
}

func parseUint256Base(number string, base int) (LNumber, error) {
	s := strings.TrimSpace(number)
	if s == "" {
		return LNumberZero, errInvalidNumber
	}
	if base == 16 && (strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X")) {
		s = s[2:]
	}
	v, ok := new(big.Int).SetString(s, base)
	if !ok {
		return LNumberZero, errInvalidNumber
	}
	if v.Sign() < 0 {
		return LNumberZero, errNegativeNumber
	}
	if v.Cmp(uint256Max) > 0 {
		return LNumberZero, errNumberOverflow
	}
	return LNumber(v.Text(10)), nil
}

func lNumberToBigInt(n LNumber) *big.Int {
	if v, ok := new(big.Int).SetString(string(n), 10); ok {
		return v
	}
	return new(big.Int)
}

func lNumberFromInt(v int) LNumber {
	if v <= 0 {
		return LNumberZero
	}
	return LNumber(strconv.Itoa(v))
}

func lNumberFromInt64(v int64) LNumber {
	if v <= 0 {
		return LNumberZero
	}
	return LNumber(strconv.FormatInt(v, 10))
}

func lNumberToInt(v LNumber) (int, bool) {
	b := lNumberToBigInt(v)
	if b.Sign() < 0 || !b.IsInt64() {
		return 0, false
	}
	i64 := b.Int64()
	i := int(i64)
	if int64(i) != i64 {
		return 0, false
	}
	return i, true
}

func lNumberToInt64(v LNumber) (int64, bool) {
	b := lNumberToBigInt(v)
	if b.Sign() < 0 || !b.IsInt64() {
		return 0, false
	}
	return b.Int64(), true
}

func lNumberCmp(lhs, rhs LNumber) int {
	return lNumberToBigInt(lhs).Cmp(lNumberToBigInt(rhs))
}

func lNumberIsZero(v LNumber) bool {
	return lNumberCmp(v, LNumberZero) == 0
}

func wrapUint256(v *big.Int) LNumber {
	v.Mod(v, uint256Mod)
	if v.Sign() < 0 {
		v.Add(v, uint256Mod)
	}
	return LNumber(v.Text(10))
}

func lNumberAdd(lhs, rhs LNumber) LNumber {
	return wrapUint256(new(big.Int).Add(lNumberToBigInt(lhs), lNumberToBigInt(rhs)))
}

func lNumberSub(lhs, rhs LNumber) LNumber {
	return wrapUint256(new(big.Int).Sub(lNumberToBigInt(lhs), lNumberToBigInt(rhs)))
}

func lNumberMul(lhs, rhs LNumber) LNumber {
	return wrapUint256(new(big.Int).Mul(lNumberToBigInt(lhs), lNumberToBigInt(rhs)))
}

func lNumberDiv(lhs, rhs LNumber) LNumber {
	// callers must check for zero divisor before calling
	return LNumber(new(big.Int).Quo(lNumberToBigInt(lhs), lNumberToBigInt(rhs)).Text(10))
}

func lNumberMod(lhs, rhs LNumber) LNumber {
	// callers must check for zero divisor before calling
	return LNumber(new(big.Int).Mod(lNumberToBigInt(lhs), lNumberToBigInt(rhs)).Text(10))
}

func lNumberPow(lhs, rhs LNumber) LNumber {
	return LNumber(new(big.Int).Exp(lNumberToBigInt(lhs), lNumberToBigInt(rhs), uint256Mod).Text(10))
}
