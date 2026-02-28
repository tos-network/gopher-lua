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
