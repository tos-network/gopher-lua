package lua

const coroutineEntryPointKey = "__co_entrypoint"

func OpenCoroutine(L *LState) int {
	mod := L.RegisterModule(CoroutineLibName, coroutineFuncs).(*LTable)
	L.Push(mod)
	return 1
}

var coroutineFuncs = map[string]LGFunction{
	"create":  coroutineCreate,
	"resume":  coroutineResume,
	"running": coroutineRunning,
	"status":  coroutineStatus,
	"wrap":    coroutineWrap,
	"yield":   coroutineYield,
}

func coroutineCreate(L *LState) int {
	fn := L.CheckFunction(1)
	th, _ := L.NewThread()
	th.Env.RawSetString(coroutineEntryPointKey, fn)
	L.Push(th)
	return 1
}

func coroutineResume(L *LState) int {
	th := L.CheckThread(1)
	entry := th.Env.RawGetString(coroutineEntryPointKey)
	fn, ok := entry.(*LFunction)
	if !ok {
		L.RaiseError("coroutine entry point is missing")
		return 0
	}
	args := make([]LValue, 0, L.GetTop()-1)
	for i := 2; i <= L.GetTop(); i++ {
		args = append(args, L.Get(i))
	}
	st, err, values := L.Resume(th, fn, args...)
	if st == ResumeError {
		L.Push(LFalse)
		if err != nil {
			L.Push(LString(err.Error()))
		} else {
			L.Push(LString("resume failed"))
		}
		return 2
	}
	L.Push(LTrue)
	for _, v := range values {
		L.Push(v)
	}
	return len(values) + 1
}

func coroutineRunning(L *LState) int {
	L.Push(L.G.CurrentThread)
	return 1
}

func coroutineStatus(L *LState) int {
	th := L.CheckThread(1)
	L.Push(LString(L.Status(th)))
	return 1
}

func coroutineWrap(L *LState) int {
	fn := L.CheckFunction(1)
	th, _ := L.NewThread()
	th.Env.RawSetString(coroutineEntryPointKey, fn)
	L.Push(L.NewClosure(coroutineWrapAux, th))
	return 1
}

func coroutineWrapAux(L *LState) int {
	th := L.CheckThread(UpvalueIndex(1))
	entry := th.Env.RawGetString(coroutineEntryPointKey)
	fn, ok := entry.(*LFunction)
	if !ok {
		L.RaiseError("coroutine entry point is missing")
		return 0
	}
	args := make([]LValue, 0, L.GetTop())
	for i := 1; i <= L.GetTop(); i++ {
		args = append(args, L.Get(i))
	}
	st, err, values := L.Resume(th, fn, args...)
	if st == ResumeError {
		if err != nil {
			L.RaiseError(err.Error())
		} else {
			L.RaiseError("resume failed")
		}
		return 0
	}
	for _, v := range values {
		L.Push(v)
	}
	return len(values)
}

func coroutineYield(L *LState) int {
	values := make([]LValue, 0, L.GetTop())
	for i := 1; i <= L.GetTop(); i++ {
		values = append(values, L.Get(i))
	}
	return L.Yield(values...)
}
