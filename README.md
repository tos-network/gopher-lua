# gopher-lua - Blockchain-Safe Deterministic Lua

A hardened fork of [gopher-lua](https://github.com/yuin/gopher-lua) for
blockchain and other consensus-critical runtimes.

This project prioritizes two properties:

1. Determinism: identical input must produce identical output on all nodes.
2. Security: contract code must not access host resources or dynamic loaders.

The runtime is adapted to a Lua 5.4-oriented contract subset while keeping the
Go embedding model of gopher-lua.

## Deterministic and Security Changes

The following host-dependent or non-deterministic surfaces are removed:

- `io` library
- `os` library
- `loadlib` / `require` / `module`
- channel library
- debug library
- coroutine library
- `math.random` / `math.randomseed`
- `dofile` / `loadfile`
- `string.dump`
- context timeout cancellation (`SetContext`/`Context`/`RemoveContext`)

Execution termination is gas-driven only.

## Numeric Model: uint256 Integer-Only

`LNumber` is no longer `float64`. It is defined as `type LNumber string` with
uint256 semantics backed by `math/big.Int` internally.

- Range: `0 .. 2^256-1`
- Overflow/underflow: wrapped modulo `2^256`
- Division: integer division
- No floating-point arithmetic in VM
- Float/scientific literals are rejected at compile time (`3.14`, `1e5`)

`tonumber` accepts integer strings (`10`, `0xff`) and rejects float forms.

## Lua 5.4-Oriented Language Alignment

This fork includes key Lua 5.4 language features needed for contracts:

- bitwise operators: `& | ~ << >>`
- floor division: `//`
- `goto`
- local variable attributes: `<const>`, `<close>`
- string escapes: `\u{...}`, `\z`

Compatibility target is a curated, deterministic subset, not full stock Lua 5.4
standard-library parity.

## Built-in Libraries

Kept:

- base (restricted)
- table
- string
- math (integer-safe subset)

Removed:

- io
- os
- debug
- coroutine
- channel

## Gas Metering

Instruction-level gas metering is built into VM execution.

```go
L := lua.NewState()
defer L.Close()

L.SetGasLimit(1_000_000)
err := L.DoString(src)
if err != nil {
    // contains "lua: gas limit exceeded" when budget is exhausted
}
used := L.GasUsed()
_ = used
```

## Embedding Host Primitives

Expose deterministic host APIs through registered modules/functions.

```go
L := lua.NewState()
defer L.Close()

L.RegisterModule("chain", map[string]lua.LGFunction{
    "get": getStorage,
    "set": setStorage,
})

if err := L.DoString(`
    local v = chain.get("k")
    chain.set("k", (v or 0) + 1)
`); err != nil {
    panic(err)
}
```

## Running Tests

Project tests:

```bash
go test ./...
```

Lua 5.4 subset compatibility tests:

```bash
make lua54-subset-test
```

The test suite is stored in `./_lua54-subset-test` and controlled by
`manifest.tsv` (runtime/compile modes).

## Contract Bytecode Workflow

Compile source to bytecode:

```bash
glua -c contract.glbc contract.lua
```

Execute bytecode:

```bash
glua -bc contract.glbc
```

Dump IR from source or bytecode:

```bash
glua -di contract.lua
glua -di -bc contract.glbc
```

Programmatic APIs:

- `BuildIR` / `CompileIR` for `AST -> IR -> bytecode`
- `CompileSourceToBytecode` to produce bytecode blobs
- `LState.DoBytecode` / `LState.LoadBytecode` to execute precompiled contracts
- `LState.Load` auto-detects bytecode by magic header

Bytecode safety checks:

- format version check
- VM fingerprint check (package/language/numeric/opcode profile)
- payload SHA-256 integrity check

## glua CLI

```bash
go build ./cmd/glua
./glua script.lua
```

## Module

```
github.com/tos-network/glua
```

Forked from [yuin/gopher-lua](https://github.com/yuin/gopher-lua) (MIT).
