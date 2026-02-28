# gopher-lua — Blockchain-Safe Fork

This is a **permanently restricted** fork of gopher-lua for the TOS blockchain.
All changes below are intentional design decisions. **Do not revert them.**

---

## Libraries: what is removed and must stay removed

### Deleted files (do not re-create)
- `iolib.go` — file I/O is non-deterministic
- `oslib.go` — syscalls / time are non-deterministic
- `loadlib.go` — `require` / filesystem module search allows arbitrary code loading
- `channellib.go` — goroutine channels are non-deterministic
- `debuglib.go` — `debug.setlocal` / `debug.setupvalue` break all abstraction boundaries

### `linit.go` — loaded libraries
The `luaLibs` slice MUST contain exactly these four entries:
```go
var luaLibs = []luaLib{
    luaLib{BaseLibName, OpenBase},
    luaLib{TabLibName, OpenTable},
    luaLib{StringLibName, OpenString},
    luaLib{MathLibName, OpenMath},
    // DebugLibName/OpenDebug    REMOVED
    // CoroutineLibName/OpenCoroutine REMOVED
}
```
**Do NOT add `CoroutineLibName/OpenCoroutine` or any other library.**
Coroutines have no Solidity/EVM analog and add non-deterministic execution complexity.

---

## Functions removed from `baselib.go` (do not restore)

These are removed from `baseFuncs` and their function bodies deleted:
- `dofile` — filesystem exec
- `loadfile` — filesystem load
- `load` / `loadstring` — runtime eval (equivalent to JavaScript's `eval`)
- `require` — delegates to deleted loadlib
- `module` — package infrastructure (loadlib-dependent)
- `print` — stdout side-effect is non-deterministic across validators
- `collectgarbage` — exposes host GC timing/runtime behavior; non-consensus surface
- `_printregs` — debug introspection
- `getfenv` / `setfenv` — environment manipulation
- `newproxy` — deprecated proxy API

### Deterministic stringification

Default `String()` representations for non-primitive values must NOT include memory
addresses (`%p`) or any runtime-allocated pointer text.
Keep them as stable type labels (`"table"`, `"function"`, `"thread"`, `"userdata"`, `"channel"`).

### Deterministic map handling

Do NOT depend on Go map iteration order in contract-visible paths.
- `RegisterModule` / `SetFuncs` must use sorted key order.
- Table traversal helpers must not iterate `strdict`/`dict` maps directly.

---

## Functions removed from `stringlib.go`

- `string.dump` — completely removed (serializes function bytecode, security risk)

---

## Functions removed from `mathlib.go`

- `math.random` — non-deterministic PRNG
- `math.randomseed` — same

---

## Test files

### `script_test.go`
`gluaTests` must NOT include scripts that use removed functions.
Currently disabled (do not re-enable without updating the script first):
- `coroutine.lua` — uses `coroutine.*` (lib removed)
- `base.lua` — uses `dofile`
- `db.lua` — uses debug lib (removed)
- `issues.lua` — uses `math.random`
- `os.lua` — uses os lib (removed)
- `math.lua` — uses float literals and removed functions
- `vm.lua` — uses `loadstring` (removed)
- `goto.lua` — uses `loadstring`
- `strings.lua` — tests `string.dump` error path (needs coroutine lib)

`luaTests` must NOT include:
- `constructs.lua` — uses float literals (1.25)
- Any script using `require`, `print`, `loadstring`, `io`, or `os`

### `state_test.go`
- `TestCoroutineApi1` — must stay skipped (`t.Skip("coroutine library removed")`)
- `TestContextWithCroutine` — must stay skipped

---

## LNumber type

`LNumber` is `type LNumber string` (NOT `float64`). It is backed by `math/big.Int`.
All arithmetic is **integer-only** (no floats). Helpers are in `number_uint256.go`.
Do not change `LNumber` back to `float64`.

---

## Gas metering

`LState` has `gasLimit` / `gasUsed` fields and `SetGasLimit()` / `GasUsed()` methods.
The VM loop checks gas on every instruction. Do not remove this.
