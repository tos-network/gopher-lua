package lua

func OpenMath(L *LState) int {
	mod := L.RegisterModule(MathLibName, mathFuncs).(*LTable)
	L.Push(mod)
	return 1
}

var mathFuncs = map[string]LGFunction{
	"abs":   mathAbs,
	"ceil":  mathCeil,
	"floor": mathFloor,
	"fmod":  mathFmod,
	"max":   mathMax,
	"min":   mathMin,
	"mod":   mathMod,
	"pow":   mathPow,
}

func mathAbs(L *LState) int {
	L.Push(L.CheckNumber(1))
	return 1
}

func mathCeil(L *LState) int {
	L.Push(L.CheckNumber(1))
	return 1
}

func mathFloor(L *LState) int {
	L.Push(L.CheckNumber(1))
	return 1
}

func mathFmod(L *LState) int {
	rhs := L.CheckNumber(2)
	if lNumberIsZero(rhs) {
		L.RaiseError("attempt to perform 'n%%0'")
	}
	L.Push(lNumberMod(L.CheckNumber(1), rhs))
	return 1
}

func mathMax(L *LState) int {
	if L.GetTop() == 0 {
		L.RaiseError("wrong number of arguments")
	}
	if L.GetTop() == 1 {
		if tb, ok := L.Get(1).(*LTable); ok {
			max, ok := tableExtremum(L, tb, true)
			if !ok {
				return 0
			}
			L.Push(max)
			return 1
		}
	}
	max := L.CheckNumber(1)
	top := L.GetTop()
	for i := 2; i <= top; i++ {
		v := L.CheckNumber(i)
		if lNumberCmp(v, max) > 0 {
			max = v
		}
	}
	L.Push(max)
	return 1
}

func mathMin(L *LState) int {
	if L.GetTop() == 0 {
		L.RaiseError("wrong number of arguments")
	}
	if L.GetTop() == 1 {
		if tb, ok := L.Get(1).(*LTable); ok {
			min, ok := tableExtremum(L, tb, false)
			if !ok {
				return 0
			}
			L.Push(min)
			return 1
		}
	}
	min := L.CheckNumber(1)
	top := L.GetTop()
	for i := 2; i <= top; i++ {
		v := L.CheckNumber(i)
		if lNumberCmp(v, min) < 0 {
			min = v
		}
	}
	L.Push(min)
	return 1
}

func mathMod(L *LState) int {
	rhs := L.CheckNumber(2)
	if lNumberIsZero(rhs) {
		L.RaiseError("attempt to perform 'n%%0'")
	}
	L.Push(lNumberMod(L.CheckNumber(1), rhs))
	return 1
}

func mathPow(L *LState) int {
	L.Push(lNumberPow(L.CheckNumber(1), L.CheckNumber(2)))
	return 1
}

func tableExtremum(L *LState, tb *LTable, wantMax bool) (LNumber, bool) {
	n := tb.Len()
	if n == 0 {
		L.RaiseError("math table argument must not be empty")
		return LNumberZero, false
	}

	first := tb.RawGetInt(1)
	out, ok := lvalueToNumber(first)
	if !ok {
		L.RaiseError("math table argument must contain only numbers")
		return LNumberZero, false
	}

	for i := 2; i <= n; i++ {
		v, ok := lvalueToNumber(tb.RawGetInt(i))
		if !ok {
			L.RaiseError("math table argument must contain only numbers")
			return LNumberZero, false
		}
		c := lNumberCmp(v, out)
		if (wantMax && c > 0) || (!wantMax && c < 0) {
			out = v
		}
	}
	return out, true
}

func lvalueToNumber(v LValue) (LNumber, bool) {
	switch lv := v.(type) {
	case LNumber:
		return lv, true
	case LString:
		n, err := parseNumber(string(lv))
		if err == nil {
			return n, true
		}
	}
	return LNumberZero, false
}
