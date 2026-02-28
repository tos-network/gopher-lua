# gopher-lua — Blockchain-Safe Lua for TOS

A fork of [gopher-lua](https://github.com/yuin/gopher-lua) hardened for execution
inside a Byzantine-fault-tolerant blockchain (TOS). Every validator must produce
**identical results** from identical inputs; the original library's I/O, randomness,
channel primitives, and floating-point arithmetic make that impossible — they are
removed or replaced here.

Redis has run a sandboxed Lua engine in production for over a decade under exactly
these constraints. This fork applies the same discipline to a Go blockchain node.

---

## What was removed and why

| Removed | Reason |
|---------|--------|
| `io` library | File open/read/write — non-deterministic across nodes |
| `os` library | `os.time`, `os.clock`, `os.execute`, `os.exit` — wall-clock and syscalls |
| `loadlib` / `require` / `module` | Filesystem module loading — arbitrary code injection |
| `channel` library | `reflect.Select` on goroutines — non-deterministic scheduling |
| `math.random` / `math.randomseed` | PRNG seeded from runtime entropy — non-deterministic |
| `dofile` / `loadfile` | Filesystem execution from Lua scripts |
| `LState.DoFile` / `LState.LoadFile` | Go-level file loader methods |
| Float-point arithmetic | `float64` is non-deterministic across CPU/platform; replaced by arbitrary-precision integers |
| Most `math` functions | Trig, exp, log, pow, sqrt, etc. — all float-based, all removed |
| `math.pi` / `math.huge` | Float constants — removed |

## What was kept

| Library | Status | Notes |
|---------|--------|-------|
| `base` | Modified | Removed `dofile`, `loadfile`, `require`, `module`. Kept `collectgarbage` (calls `runtime.GC()` — pure Go, consensus-safe) |
| `table` | Unchanged | Deterministic |
| `string` | Unchanged | Deterministic |
| `math` | Heavily trimmed | Only `max`, `min`, `mod` — all integer-safe |
| `debug` | Unchanged | Stack introspection only, no I/O |
| `coroutine` | Unchanged | Cooperative scheduling — fully deterministic |

---

## Integer-Only Numbers (`LNumber`)

Lua normally uses `float64` for all numbers. This fork replaces the numeric type
with **arbitrary-precision integers** backed by `math/big.Int`.

### Why not float64?

`float64` cannot exactly represent wei-denominated balances:

```
1 TOS = 10¹⁸ wei
float64 max exact integer ≈ 9 × 10¹⁵  →  loses precision above ~9 TOS
FPMM invariant product ≈ pool_yes × pool_no  →  can exceed 10³⁶
```

### Why not uint64?

`uint64` max ≈ 1.8 × 10¹⁹ — only ~18 TOS in wei units, insufficient for realistic
pool and treasury balances.

### The solution: string-backed big.Int

`LNumber` is now defined as `type LNumber string`. The decimal string is the
canonical representation; all arithmetic converts to `*big.Int`, operates, and
converts back:

```lua
local bal  = tos.balance(tos.caller())   -- "1000000000000000000" (1 TOS in wei)
local half = bal / 2                     -- "500000000000000000"
local fee  = bal * 3 / 1000             -- "3000000000000000"
```

### Arithmetic rules

| Operation | Behaviour |
|-----------|-----------|
| `+` `-` `*` | Exact big integer arithmetic — no overflow |
| `/` | Integer division, truncates toward zero (`7/2 == 3`) |
| `%` | Integer modulo, sign follows divisor |
| `^` | **Removed** — raises a runtime error |
| Negative numbers | Supported (`-1`, `-n`) |
| Float literals | **Rejected** at parse time (`3.14`, `1e5` are syntax errors) |

### Math library (trimmed)

Only three functions remain in `math`:

| Function | Description |
|----------|-------------|
| `math.max(a, b, ...)` | Largest argument |
| `math.min(a, b, ...)` | Smallest argument |
| `math.mod(a, b)` | Same as `a % b` |

All trigonometric, exponential, logarithmic, and rounding functions are removed.
The constants `math.pi` and `math.huge` are removed.

### `tonumber` behaviour

`tonumber` accepts only integer strings. Float strings are rejected:

```lua
tonumber("42")    -- 42
tonumber("0xff")  -- 255
tonumber("3.14")  -- nil  (rejected)
tonumber("1e5")   -- nil  (rejected)
```

---

## Gas Metering

Every blockchain transaction has a gas budget. Scripts that loop forever must be
killed before they stall a validator. Gas metering counts VM instructions and
aborts execution when the budget is exhausted.

### API

```go
L := lua.NewState()
defer L.Close()

// Set the gas limit before running any script.
// Zero means unlimited (default, for trusted internal use).
L.SetGasLimit(1_000_000)

err := L.DoString(src)
if err != nil {
    // err.Error() contains "lua: gas limit exceeded" if the budget ran out
}

// How many instructions were consumed:
fmt.Println("gas used:", L.GasUsed())
```

`SetGasLimit` resets `GasUsed` to zero. Call it once per transaction, before
`DoString`.

### Error string

When the gas budget is exceeded the VM raises:

```
lua: gas limit exceeded
```

The TOS executor catches this string and maps it to `ErrIntrinsicGas`.

---

## Injecting Host Primitives

Lua scripts interact with the blockchain through Go functions registered as a
module. This is the standard gopher-lua `LGFunction` + `RegisterModule` pattern —
no changes to the VM required.

```go
L := lua.NewState()
defer L.Close()
L.SetGasLimit(gasRemaining)

L.RegisterModule("tos", map[string]lua.LGFunction{
    "get":      tosGet,      // read contract storage
    "set":      tosSet,      // write contract storage
    "transfer": tosTransfer, // transfer TOS between accounts
    "balance":  tosBalance,  // query TOS balance
    "caller":   tosCaller,   // msg.From address
    "value":    tosValue,    // msg.Value in wei
})

if err := L.DoString(string(contractSource)); err != nil {
    // handle error
}
```

A Lua contract calling these primitives looks like:

```lua
local bal = tos.balance(tos.caller())
local min_bal = 1000000000000000000   -- 1 TOS in wei
tos.require(bal >= min_bal, "insufficient balance")
tos.set("initialized", "1")
tos.transfer("0x...", "500000000000000000")
```

Note: all balance comparisons are exact integer comparisons — no float rounding.

---

## Running Scripts

Scripts are always passed as strings (source code stored on-chain via `code_put_ttl`).
There is no file loading — use `DoString`:

```go
L := lua.NewState()
defer L.Close()
L.SetGasLimit(500_000)

err := L.DoString(`
    local x = 0
    for i = 1, 100 do
        x = x + i
    end
    tos.set("result", tostring(x))
`)
```

---

## Calling Go from Lua

Any Go function can be exposed to Lua as an `LGFunction`:

```go
func myFunc(L *lua.LState) int {
    arg := L.CheckString(1)   // get first argument
    L.Push(lua.LString("hello " + arg))  // push return value
    return 1                  // number of return values
}

L.SetGlobal("myFunc", L.NewFunction(myFunc))
```

To push a numeric result, use `lua.LNumber`:

```go
func tosBalance(L *lua.LState) int {
    addr := L.CheckString(1)
    wei := getBalanceWei(addr)          // returns *big.Int
    L.Push(lua.LNumber(wei.String()))   // push as decimal string
    return 1
}
```

---

## Context / Timeout

The upstream context cancellation mechanism is preserved and works alongside gas
metering. Use gas metering for deterministic termination (same limit on every
validator). Use context only for wall-clock timeouts in off-chain tooling.

```go
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
defer cancel()
L.SetContext(ctx)
L.SetGasLimit(10_000_000)
err := L.DoString(src)
```

---

## Data Types

| Lua type | Go type | Notes |
|----------|---------|-------|
| `nil` | `lua.LNil` | constant |
| `bool` | `lua.LBool` | `lua.LTrue`, `lua.LFalse` |
| `number` | `lua.LNumber` | `string` containing a decimal integer; backed by `math/big.Int` |
| `string` | `lua.LString` | `string` |
| `table` | `*lua.LTable` | |
| `function` | `*lua.LFunction` | |
| `userdata` | `*lua.LUserData` | for Go-defined types |
| `thread` | `*lua.LState` | coroutines |

### Constructing LNumber from Go

```go
// From an int
lua.LNumber("42")

// From a *big.Int
lua.LNumber(bigIntValue.String())

// From a wei amount
wei := new(big.Int).Mul(big.NewInt(5), params.TOS)  // 5 TOS in wei
lua.LNumber(wei.String())
```

---

## LState Options

```go
L := lua.NewState(lua.Options{
    RegistrySize:        1024 * 20,
    RegistryMaxSize:     1024 * 80,
    RegistryGrowStep:    32,
    CallStackSize:       256,
    SkipOpenLibs:        false,
    IncludeGoStackTrace: false,
})
```

---

## glua CLI

A minimal REPL / script runner for local testing (not for on-chain use):

```bash
go build ./cmd/glua
./glua script.lua
./glua -e 'print(1000000000000000000 + 1)'
```

---

## Module

```
github.com/tos-network/gopher-lua
```

Forked from [yuin/gopher-lua](https://github.com/yuin/gopher-lua) (MIT).
Modifications © TOS Network, MIT License.
