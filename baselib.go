package lua

import (
	"strings"
)

/* basic functions {{{ */

func OpenBase(L *LState) int {
	global := L.Get(GlobalsIndex).(*LTable)
	L.SetGlobal("_G", global)
	L.SetGlobal("_VERSION", LString(LuaVersion))
	L.SetGlobal("_GOPHER_LUA_VERSION", LString(PackageName+" "+PackageVersion))
	basemod := L.RegisterModule("_G", baseFuncs)
	openMapping(L)
	openCrypto(L)
	global.RawSetString("ipairs", L.NewClosure(baseIpairs, L.NewFunction(ipairsaux)))
	global.RawSetString("pairs", L.NewClosure(basePairs, L.NewFunction(pairsaux)))
	L.Push(basemod)
	return 1
}

var baseFuncs = map[string]LGFunction{
	"address": baseAddress,
	"assert":  baseAssert,
	// collectgarbage REMOVED: runtime GC behavior is not consensus-critical
	// and can leak host runtime nondeterminism into contracts.
	"error": baseError,
	// getfenv    REMOVED: Lua 5.1 legacy environment API, no use in contracts
	"getmetatable": baseGetMetatable,
	// load       REMOVED: runtime dynamic code loading (eval equivalent)
	// loadstring REMOVED: runtime dynamic code loading (eval equivalent)
	"next":  baseNext,
	"pcall": basePCall,
	// print      REMOVED: stdout side-effect, non-deterministic across validators
	"rawequal": baseRawEqual,
	"rawget":   baseRawGet,
	"rawset":   baseRawSet,
	"select":   baseSelect,
	// _printregs REMOVED: internal VM debug dump
	// setfenv    REMOVED: mutates function environments, attack surface
	"setmetatable": baseSetMetatable,
	"tonumber":     baseToNumber,
	"tostring":     baseToString,
	"type":         baseType,
	"unpack":       baseUnpack,
	"xpcall":       baseXPCall,
	// newproxy   REMOVED: undocumented userdata proxy, no use in contracts
}

func baseAssert(L *LState) int {
	if !L.ToBool(1) {
		L.RaiseError(L.OptString(2, "assertion failed!"))
		return 0
	}
	return L.GetTop()
}

func baseError(L *LState) int {
	obj := L.CheckAny(1)
	level := L.OptInt(2, 1)
	L.Error(obj, level)
	return 0
}

func baseGetMetatable(L *LState) int {
	L.Push(L.GetMetatable(L.CheckAny(1)))
	return 1
}

func ipairsaux(L *LState) int {
	tb := L.CheckTable(1)
	i := L.CheckInt(2)
	i++
	v := tb.RawGetInt(i)
	if v == LNil {
		return 0
	} else {
		L.Pop(1)
		L.Push(lNumberFromInt(i))
		L.Push(lNumberFromInt(i))
		L.Push(v)
		return 2
	}
}

func baseIpairs(L *LState) int {
	tb := L.CheckTable(1)
	L.Push(L.Get(UpvalueIndex(1)))
	L.Push(tb)
	L.Push(LNumberZero)
	return 3
}

func baseNext(L *LState) int {
	tb := L.CheckTable(1)
	index := LNil
	if L.GetTop() >= 2 {
		index = L.Get(2)
	}
	key, value := tb.Next(index)
	if key == LNil {
		L.Push(LNil)
		return 1
	}
	L.Push(key)
	L.Push(value)
	return 2
}

func pairsaux(L *LState) int {
	tb := L.CheckTable(1)
	key, value := tb.Next(L.Get(2))
	if key == LNil {
		return 0
	} else {
		L.Pop(1)
		L.Push(key)
		L.Push(key)
		L.Push(value)
		return 2
	}
}

func basePairs(L *LState) int {
	tb := L.CheckTable(1)
	L.Push(L.Get(UpvalueIndex(1)))
	L.Push(tb)
	L.Push(LNil)
	return 3
}

func basePCall(L *LState) int {
	L.CheckAny(1)
	v := L.Get(1)
	if v.Type() != LTFunction && L.GetMetaField(v, "__call").Type() != LTFunction {
		L.Push(LFalse)
		L.Push(LString("attempt to call a " + v.Type().String() + " value"))
		return 2
	}
	nargs := L.GetTop() - 1
	if err := L.PCall(nargs, MultRet, nil); err != nil {
		L.Push(LFalse)
		if aerr, ok := err.(*ApiError); ok {
			L.Push(aerr.Object)
		} else {
			L.Push(LString(err.Error()))
		}
		return 2
	} else {
		L.Insert(LTrue, 1)
		return L.GetTop()
	}
}

func baseRawEqual(L *LState) int {
	if L.CheckAny(1) == L.CheckAny(2) {
		L.Push(LTrue)
	} else {
		L.Push(LFalse)
	}
	return 1
}

func baseRawGet(L *LState) int {
	L.Push(L.RawGet(L.CheckTable(1), L.CheckAny(2)))
	return 1
}

func baseRawSet(L *LState) int {
	L.RawSet(L.CheckTable(1), L.CheckAny(2), L.CheckAny(3))
	return 0
}

func baseSelect(L *LState) int {
	L.CheckTypes(1, LTNumber, LTString)
	switch lv := L.Get(1).(type) {
	case LNumber:
		idx, ok := lNumberToInt(lv)
		if !ok {
			L.ArgError(1, "index out of range")
		}
		num := L.GetTop()
		if idx < 0 {
			idx = num + idx
		} else if idx > num {
			idx = num
		}
		if 1 > idx {
			L.ArgError(1, "index out of range")
		}
		return num - idx
	case LString:
		if string(lv) != "#" {
			L.ArgError(1, "invalid string '"+string(lv)+"'")
		}
		L.Push(lNumberFromInt(L.GetTop() - 1))
		return 1
	}
	return 0
}

func baseSetMetatable(L *LState) int {
	L.CheckTypes(2, LTNil, LTTable)
	obj := L.Get(1)
	if obj == LNil {
		L.RaiseError("cannot set metatable to a nil object.")
	}
	mt := L.Get(2)
	if m := L.metatable(obj, true); m != LNil {
		if tb, ok := m.(*LTable); ok && tb.RawGetString("__metatable") != LNil {
			L.RaiseError("cannot change a protected metatable")
		}
	}
	L.SetMetatable(obj, mt)
	L.SetTop(1)
	return 1
}

func baseToNumber(L *LState) int {
	base := L.OptInt(2, 10)
	noBase := L.Get(2) == LNil

	switch lv := L.CheckAny(1).(type) {
	case LNumber:
		L.Push(lv)
	case LString:
		str := strings.Trim(string(lv), " \n\t")
		if noBase {
			if v, err := parseNumber(str); err != nil {
				L.Push(LNil)
			} else {
				L.Push(v)
			}
		} else {
			if base < 2 || base > 36 {
				L.ArgError(2, "base out of range")
			}
			if v, err := parseUint256Base(str, base); err != nil {
				L.Push(LNil)
			} else {
				L.Push(v)
			}
		}
	default:
		L.Push(LNil)
	}
	return 1
}

func baseAddress(L *LState) int {
	addr, err := parseAddressValue(L.CheckAny(1))
	if err != nil {
		L.ArgError(1, err.Error())
	}
	L.Push(addr)
	return 1
}

func baseToString(L *LState) int {
	v1 := L.CheckAny(1)
	L.Push(L.ToStringMeta(v1))
	return 1
}

func baseType(L *LState) int {
	L.Push(LString(L.CheckAny(1).Type().String()))
	return 1
}

func baseUnpack(L *LState) int {
	tb := L.CheckTable(1)
	start := L.OptInt(2, 1)
	end := L.OptInt(3, tb.Len())
	for i := start; i <= end; i++ {
		L.Push(tb.RawGetInt(i))
	}
	ret := end - start + 1
	if ret < 0 {
		return 0
	}
	return ret
}

func baseXPCall(L *LState) int {
	fn := L.CheckFunction(1)
	errfunc := L.CheckFunction(2)

	top := L.GetTop()
	L.Push(fn)
	if err := L.PCall(0, MultRet, errfunc); err != nil {
		L.Push(LFalse)
		if aerr, ok := err.(*ApiError); ok {
			L.Push(aerr.Object)
		} else {
			L.Push(LString(err.Error()))
		}
		return 2
	} else {
		L.Insert(LTrue, top+1)
		return L.GetTop() - top
	}
}

/* }}} */

//
