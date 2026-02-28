package lua

const preloadLimit = 128

var preloads [preloadLimit]LValue

func init() {
	for i := 0; i < preloadLimit; i++ {
		preloads[i] = LNumber(intToDecStr(i))
	}
}

// intToDecStr converts a non-negative int to a decimal string.
func intToDecStr(n int) string {
	if n == 0 {
		return "0"
	}
	b := make([]byte, 0, 20)
	for n > 0 {
		b = append(b, byte('0'+n%10))
		n /= 10
	}
	// reverse
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}

type allocator struct{}

func newAllocator(size int) *allocator {
	return &allocator{}
}

// LNumber2I converts an LNumber to an LValue. With string LNumber, no boxing is needed.
func (al *allocator) LNumber2I(v LNumber) LValue {
	// Use preloaded values for small non-negative integers
	b := lnumToBig(v)
	if b.Sign() >= 0 && b.IsInt64() {
		if n := b.Int64(); n >= 0 && n < preloadLimit {
			return preloads[n]
		}
	}
	return v
}
