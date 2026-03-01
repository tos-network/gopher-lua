# TOL Specification (Yul-Inspired, Chain-Safe)

Status: Draft v0.2 (implementation snapshot updated on 2026-03-01)
Owner: GTOS/Tolang engineering
Scope: Deterministic smart-contract IR for on-chain bytecode generation

---

## 1. Positioning

TOL is a contract-focused IR language inspired by Solidity Yul principles:

- small core language
- explicit control flow
- explicit state access
- deterministic semantics
- stable compiler target

But TOL is not Yul syntax-compatible. It is designed for GTOS/Tolang runtime constraints:

- integer-only arithmetic domain (uN + iN)
- 32-byte address model
- no floating point
- no non-deterministic runtime APIs
- canonical compile target: `TOL (typed/lowered) -> direct IR -> bytecode`

Compatibility target for v0.2:

- full semantic coverage for the contracts in
  `00-gtos/conditional-tokens-market-makers/contracts/*.sol`

Target pipeline:

1. `TOL text -> TOL (typed/lowered) -> direct IR -> bytecode`
   (primary/canonical and only public path)
2. compiler-internal lowering detail:
   `TOL (typed/lowered) -> direct IR -> bytecode`

Pipeline policy on 2026-03-01:

1. The deploy pipeline is locked to direct compilation:
   `TOL text -> TOL (typed/lowered) -> direct IR -> bytecode`.
2. Transpile bridge routes are not part of the supported architecture.
3. Remaining implementation gaps are tracked in Section 20.

---

## 2. Design Goals

1. Determinism-first execution semantics.
2. Human-writable IR (for agent-generated contracts and auditing).
3. Stable, versioned contract artifact independent from source language.
4. Strong static verification before bytecode generation.
5. Easy lowering to current Tolang runtime.

## 3. Non-Goals

1. Full source-level compatibility with every Solidity feature/version.
2. Dynamic reflection/meta-programming.
3. Floating-point numerics.
4. Host-dependent behavior (time, filesystem, random, goroutines/channels).

---

## 4. Execution and Safety Baseline

TOL contracts execute in a deterministic, single-threaded VM environment.

Hard rules:

1. No wall-clock time access.
2. No system randomness.
3. No network or filesystem access.
4. No concurrency primitives.
5. No iteration order dependency on hash maps.

All observable behavior must depend only on:

- block context fields exposed by host
- transaction input
- contract storage
- deterministic builtins

---

## 5. Module Structure

A TOL file defines one module with one deployable contract and optional
support declarations (`interface`, `library`, `enum`, `modifier`).

```tol
tol 0.2

contract TRC20 {
  storage {
    slot name: bytes;
    slot symbol: bytes;
    slot decimals: u8;
    slot total_supply: u256;
    slot balances: mapping(address => u256);
    slot allowances: mapping(address => mapping(address => u256));
  }

  event Transfer(from: address indexed, to: address indexed, value: u256)
  event Approval(owner: address indexed, spender: address indexed, value: u256)

  fn transfer(to: address, amount: u256) -> (ok: bool) public { ... }
  fallback { revert "UNKNOWN_SELECTOR" }
}
```

Top-level elements:

1. `tol <version>` header (required).
2. optional `interface <Name> { ... }` declarations.
3. optional `library <Name> { ... }` declarations.
4. exactly one deployable `contract <Name> { ... }`.
5. inside contract: `storage`, `event`, `error`, `enum`, `modifier`, `fn`.
6. optional `constructor` and `fallback`.

---

## 6. Type System

### 6.1 Primitive Types

1. unsigned integers: `u8`, `u16`, `u32`, `u64`, `u128`, `u256`
2. signed integers: `i8`, `i16`, `i32`, `i64`, `i128`, `i256`
3. `bool`
4. `address` (fixed 32 bytes in GTOS)
5. `bytes4`, `bytes32`
6. `bytes` (dynamic byte array)
7. `string` (UTF-8 alias over `bytes`)

### 6.2 Composite Types

1. `mapping(K => V)` (recursive/nestable)
2. arrays: `T[]` (dynamic), `T[N]` (fixed)
3. enums
4. static tuple returns via function signatures

### 6.3 Type Rules

1. No implicit narrowing casts.
2. Widening casts within signed domain or unsigned domain may be implicit.
3. Signed/unsigned casts must be explicit (`as_i256`, `as_u256`, etc.).
4. `address`, `bytes4`, and `bytes32` are distinct types.
5. `bytes`/`string` are not comparable by `==`; use `bytes_eq`/`string_eq`.
6. Mapping key type `K` must be one of: `u*`, `i*`, `bool`, `address`, `bytes32`.
7. Nested mapping depth is inferred from type declaration, e.g.
   `mapping(address => mapping(address => u256))` requires exactly two index keys.
8. Array index type is `u256`.
9. In checked arithmetic mode, signed overflow/underflow reverts.

### 6.4 Data Locations

TOL models Solidity-like data locations:

1. `storage`: persistent contract state.
2. `memory`: transient per-call memory.
3. `calldata`: immutable call input view.

Rules:

1. External function params default to `calldata` for dynamic types.
2. Internal function params default to `memory` for dynamic types.
3. `new T[](n)` allocates in `memory`.
4. Storage dynamic arrays support `.length`, indexed get/set, and `.push(v)`.

---

## 7. Arithmetic and Bit Semantics

All arithmetic is integer-only and deterministic.

### 7.1 Overflow Modes

TOL requires explicit mode selection per contract or per function:

- `arith checked` (default): overflow/underflow reverts
- `arith wrapping`: modulo 2^N behavior

For all integer widths in checked mode:

- `add`, `sub`, `mul`, `pow` overflow => revert
- `div` by zero => revert
- `mod` by zero => revert

Signed arithmetic (`i*`) follows Solidity-compatible two's complement rules.

Division and modulo are defined as:

1. `a / b`: truncates toward zero.
2. `a % b`: remainder with the same sign as `a`.
3. `min_signed / -1` in checked mode reverts.

### 7.2 Operators

Arithmetic:

- `+ - * / %`
- `pow(a,b)` builtin (integer exponent, checked constraints)

Bitwise:

- `& | ^ ~ << >>`

Shift rules:

1. Left shift is logical for all integer types.
2. Right shift is logical for `u*`, arithmetic for `i*`.
3. Shift amount is `u256`; oversized shift yields zero (`u*`) or sign-fill (`i*`).

Comparison:

- `== != < <= > >=`

Boolean:

- `&& || !` (short-circuit)

---

## 8. Storage Model

### 8.1 Slot Declarations

`slot x: u256;` declares a named storage variable.

### 8.2 Mapping Declarations

Use Solidity-style type syntax in storage slots:

1. `slot balances: mapping(address => u256);`
2. `slot allowances: mapping(address => mapping(address => u256));`

### 8.3 Canonical Addressing

Compiler must derive deterministic storage keys using canonical encoding.

Recommended v0.2 key derivation:

1. `slot`: `H("tol.slot." ++ contract ++ "." ++ name)`
2. `mapping` access with keys `k1..kn`:
   `h0 = base_slot_hash`
   `h1 = H(encode(k1) ++ h0)`
   `h2 = H(encode(k2) ++ h1)`
   ...
   `hn = H(encode(kn) ++ h(n-1))`
   final slot is `hn`

`encode(key)` must be type-stable:

- `u*`: 32-byte big-endian
- `i*`: 32-byte two's complement
- `address`: 32-byte raw
- `bytes32`: raw 32 bytes

### 8.4 Storage Dynamic Arrays

Storage layout for dynamic array `slot arr: T[];`:

1. length stored at base slot `p`.
2. data base is `H(p)`.
3. element `arr[i]` at `H(p) + i` for value types.
4. nested dynamic element addressing is recursive from element slot root.

`arr.push(v)`:

1. reads current length `n`.
2. writes `v` to slot `H(p) + n`.
3. stores new length `n + 1`.

### 8.5 Storage Access Ops

1. `sload(slot_name)` / `sstore(slot_name, value)`
2. Generic mapping ops:
   `mload(slot_name, k1, ..., kn)` / `mstore(slot_name, k1, ..., kn, value)`
   where `n` is inferred from declared nested mapping depth.
3. Surface syntax sugar (recommended):
   `balances[owner]`
   `allowances[owner][spender]`
   Compiler lowers indexed access to `mload/mstore` with type-checked key arity.
4. Array ops:
   `arr.length`, `arr[i]`, `arr.push(v)` for storage arrays.

---

## 9. ABI and Dispatch

TOL external functions are ABI-dispatchable.

Function modifiers:

1. `public`: callable internally and by selector dispatch
2. `external`: callable by selector dispatch only
3. `internal`: callable only inside module/inheritance tree
4. `private`: callable only inside declaring contract
5. `view`: state writes forbidden
6. `pure`: state reads and writes forbidden
7. `payable`: value accepted; non-payable reverts on non-zero value

Selector model:

- default selector: first 4 bytes of `keccak256("name(type1,type2,...)")`
- optional override: `@selector("0x12345678")`

ABI payload model follows Ethereum-compatible ABI unless chain profile overrides.

Builtins:

1. `abi.decode<T...>(msg.data[4:])`
2. `abi.encode<T...>(...)`
3. `abi.encodePacked<T...>(...)`
4. `abi.encodeWithSelector(sel: bytes4, ...args)`
5. `abi.encodeWithSignature(sig: string, ...args)` (compile-time string required)
6. `ret abi.encode<T...>(...)`

Selector/member builtins:

1. `selector("transfer(address,u256)") -> bytes4`
2. `ContractName.fnName.selector -> bytes4`
3. `this.fnName.selector -> bytes4`

Tuple decode and assignment are required:

`let (a: u256, b: bytes32[]) = abi.decode<(u256,bytes32[])>(data);`

---

## 10. Host Interface (Deterministic Builtins)

Environment read-only:

1. `msg.sender: address`
2. `msg.value: u256`
3. `msg.data: bytes`
4. `tx.origin: address`
5. `tx.gasprice: u256`
6. `block.number: u64`
7. `block.timestamp: u64` (if chain uses deterministic block timestamp)
8. `gas.left(): u64`

State-changing host calls:

1. `call(addr, value, data) -> (ok: bool, ret: bytes)`
2. `staticcall(addr, data) -> (ok: bool, ret: bytes)`
3. `delegatecall(addr, data) -> (ok: bool, ret: bytes)`
4. `create(value, init_code: bytes) -> (addr: address)`
5. `create2(value, salt: bytes32, init_code: bytes) -> (addr: address)`
6. `emit EventName(...)`
7. `transfer(addr, amount)`

Crypto/hash builtins:

1. `keccak256(data: bytes) -> bytes32`
2. `sha256(data: bytes) -> bytes32`
3. `ripemd160(data: bytes) -> bytes32`
4. `ecrecover(hash: bytes32, v: u8, r: bytes32, s: bytes32) -> address`

Contract creation constraints:

1. `create/create2` are deterministic given `(sender, nonce|salt, init_code, value)`.
2. Failure returns zero-address and may be wrapped by `require`.
3. Inline assembly is not part of TOL surface language; use these builtins instead.

### 10.1 Deterministic Integer `log/exp` Intrinsics (for LMSR)

TOL v0.2 provides deterministic integer transcendental intrinsics without
any fixed-point library type.

Scaling policy:

1. TOL runtime always operates on integers.
2. Scaling factor is user-defined by contract code (for example `ONE = 10^18` or `2^64`).
3. Intrinsics receive `scale` explicitly, so no global fixed scaling is imposed by TOL.

Required builtins:

1. `math.binaryLog(x_scaled: u256, scale: u256, mode: EstimationMode) -> i256`
2. `math.pow2(x_scaled: i256, scale: u256, mode: EstimationMode) -> u256`
3. `math.max(xs: i256[]) -> i256`

Interpretation:

1. `binaryLog`: treats real input as `x_scaled / scale`, returns
   `round_mode(log2(x_scaled / scale) * scale)`.
2. `pow2`: treats real input as `x_scaled / scale`, returns
   `round_mode(2^(x_scaled / scale) * scale)`.

`EstimationMode` enum:

1. `LowerBound`
2. `Midpoint`
3. `UpperBound`

Determinism rules:

1. Must be fully integer arithmetic; no float or host FPU use.
2. Rounding behavior is mode-dependent and must be bit-identical across nodes.
3. Overflow/invalid domain handling must follow checked arithmetic policy.
4. Domain errors are explicit:
   - `binaryLog` requires `x_scaled > 0` and `scale > 0`
   - `pow2` requires `scale > 0`
   otherwise revert.

Forbidden in v0.2:

- random
- file/network
- thread/channel primitives

---

## 11. Control Flow Model

Structured control flow only in v0.2:

1. `if/else`
2. `for` / `while`
3. `break` / `continue`
4. `return`
5. `revert`

No unrestricted `goto` in text-level TOL v0.2.

Compiler lowering may introduce internal labels/jumps.

---

## 12. Statements and Expressions

### 12.1 Statements

1. `let x: T = expr;`
2. `set lvalue = expr;`
3. `if cond { ... } else { ... }`
4. `while cond { ... }`
5. `for let i: u256 = a; i < b; i = i + 1 { ... }`
6. `return expr_list;`
7. `revert "ERR";`
8. `assert(cond, "ERR");`
9. `require(cond, "ERR");`
10. `emit EventName(...);`
11. local tuple destructuring:
    `let (x: u256, ys: address[]) = abi.decode<(u256,address[])>(data);`

### 12.2 Expression Side Effects

Only function calls and storage ops may have side effects.

Evaluation order is strictly left-to-right.

### 12.3 Mapping and Array Index Semantics

Index chains are resolved from declaration type:

1. If `slot balances: mapping(address => u256)`, then:
   `balances[owner]` is a value lvalue/rvalue.
2. If `slot allowances: mapping(address => mapping(address => u256))`, then:
   `allowances[owner]` is an intermediate mapping value (not storable directly),
   `allowances[owner][spender]` is the final value lvalue/rvalue.
3. Using too few/too many indices is a compile-time error.
4. Index key types are checked at compile time.
5. For `slot xs: u256[]`, `xs[i]` is a value lvalue/rvalue and
   `xs.length` is a read-only rvalue in expressions.
6. `xs.push(v)` is valid only for storage dynamic arrays.

### 12.4 Inheritance, Modifiers, and Internal Calls

1. Contract inheritance is single or multiple (`contract C is A, B`).
2. Linearization uses C3; override resolution follows linearized order.
3. `modifier M(args) { pre; _; post; }` is lowered at compile time.
4. Abstract function declarations are allowed in base contracts/interfaces.
5. `super.fn(...)` is supported for linearized parent dispatch.
6. Function declarations may apply modifiers: `fn f(...) onlyOwner atStage(...) { ... }`.

---

## 13. Error Semantics

Error model supports:

1. plain revert string: `revert "INSUFFICIENT_BALANCE"`
2. typed custom error: `revert ErrorName(arg1, arg2)`

Verifier rules:

1. all revert paths must type-check.
2. all external entry paths either return declared ABI type or revert.

---

## 14. Events

Event declarations:

```tol
event Transfer(from: address indexed, to: address indexed, value: u256)
```

Emission:

```tol
emit Transfer(from, to, amount)
```

Event topic/data encoding follows chain ABI profile.

Rules:

1. Up to 3 indexed event fields are allowed.
2. For dynamic indexed fields (`bytes`, `string`, `T[]`), topic stores `keccak256(abi.encode(field))`.
3. Non-indexed fields are ABI-encoded in event data area.

---

## 15. Verifier (Mandatory Before Codegen)

Verifier stages:

1. Parse + AST validation.
2. Name resolution and symbol table build.
3. Type checking.
4. Storage declaration consistency check.
5. Control-flow validation (structured -> CFG).
6. Definite assignment check.
7. Effect checks (`view` forbids writes/calls with value).
8. Arithmetic policy checks.
9. ABI surface checks (selector uniqueness, param/return validity).
10. Inheritance checks (C3 linearization, override compatibility, diamond conflicts).
11. Modifier checks (placeholder `_` presence, expansion acyclic).
12. Interface conformance checks for declared `interface`.
13. Storage array checks (`.push` only on storage dynamic arrays).
14. Signed/unsigned cast checks and `min_signed / -1` guard insertion.
15. Resource checks (max function size, max local slots, max call depth static hints).

Reject on first error.

---

## 16. Gas and Cost Model

Gas has two layers:

1. VM instruction gas (core interpreter).
2. host primitive gas (`sload/sstore/log/call/deploy/...`).

TOL compiler must emit metadata table for static estimates:

- `max_stack_slots`
- `bytecode_len`
- `contains_unbounded_loop` flag
- optional basic-block static gas lower bounds

No wall-clock timeout semantics. Termination only via gas depletion or return/revert.

---

## 17. Artifact Format

Recommended output artifact (`.glbc`) contains:

1. magic bytes
2. format version
3. vm fingerprint
4. payload length
5. payload bytes
6. payload hash

Compatibility policy:

- strict version check
- strict vm-fingerprint check
- strict payload integrity check

---

## 18. Grammar (EBNF, v0.2 Draft)

```ebnf
File            = Header TopDecl+ ;
Header          = "tol" Version ;
Version         = number "." number ;

TopDecl         = InterfaceDecl | LibraryDecl | ContractDecl ;
InterfaceDecl   = "interface" Ident "{" InterfaceItem* "}" ;
LibraryDecl     = "library" Ident "{" LibraryItem* "}" ;
ContractDecl    = "contract" Ident InheritSpec? "{" ContractItem* "}" ;
InheritSpec     = "is" Ident ("," Ident)* ;

InterfaceItem   = EventDecl | ErrorDecl | FuncSigDecl ;
LibraryItem     = FuncDecl | EventDecl | ErrorDecl ;
ContractItem    = StorageDecl | EventDecl | ErrorDecl | EnumDecl | ModifierDecl
                | FuncDecl | FuncSigDecl | ConstructorDecl | FallbackDecl ;

StorageDecl     = "storage" "{" StorageItem* "}" ;
StorageItem     = SlotDecl ;
SlotDecl        = "slot" Ident ":" Type ";" ;

EventDecl       = "event" Ident "(" EventFieldList? ")" ;
EventFieldList  = EventField ("," EventField)* ;
EventField      = Ident ":" Type ("indexed")? ;

ErrorDecl       = "error" Ident "(" ParamList? ")" ;
EnumDecl        = "enum" Ident "{" Ident ("," Ident)* "}" ;
ModifierDecl    = "modifier" Ident "(" ParamList? ")" Block ;

FuncDecl        = Attr* "fn" Ident "(" ParamList? ")" ReturnSpec? Visibility? StateMut? ModifierUse* Block ;
FuncSigDecl     = Attr* "fn" Ident "(" ParamList? ")" ReturnSpec? Visibility? StateMut? ModifierUse* ";" ;
ConstructorDecl = "constructor" "(" ParamList? ")" Block ;
FallbackDecl    = "fallback" Block ;

Attr            = "@selector" "(" StringLiteral ")" ;
Visibility      = "public" | "external" | "internal" | "private" ;
StateMut        = "view" | "pure" | "payable" ;
ModifierUse     = Ident ("(" ExprList? ")")? ;
ReturnSpec      = "->" "(" ParamList ")" ;
ParamList       = Param ("," Param)* ;
Param           = Ident ":" Type DataLoc? ;
DataLoc         = "storage" | "memory" | "calldata" ;

Type            = PrimitiveType | MappingType | ArrayType | Ident ;
PrimitiveType   = UIntType | IntType | "bool" | "address" | "bytes4" | "bytes32" | "bytes" | "string" ;
UIntType        = "u8" | "u16" | "u32" | "u64" | "u128" | "u256" ;
IntType         = "i8" | "i16" | "i32" | "i64" | "i128" | "i256" ;
MappingType     = "mapping" "(" MappingKeyType "=>" Type ")" ;
MappingKeyType  = UIntType | IntType | "bool" | "address" | "bytes32" ;
ArrayType       = Type "[" "]" | Type "[" number "]" ;

Block           = "{" Stmt* "}" ;
Stmt            = LetStmt | SetStmt | IfStmt | WhileStmt | ForStmt | BreakStmt | ContinueStmt
                | ReturnStmt | RevertStmt | AssertStmt | RequireStmt | EmitStmt
                | PlaceholderStmt | ExprStmt ;

LetStmt         = "let" Ident ":" Type "=" Expr ";"
                | "let" "(" LetBindingList ")" "=" Expr ";" ;
LetBindingList  = LetBinding ("," LetBinding)* ;
LetBinding      = Ident ":" Type ;
SetStmt         = "set" LValue "=" Expr ";" ;
IfStmt          = "if" Expr Block ("else" Block)? ;
WhileStmt       = "while" Expr Block ;
ForStmt         = "for" LetStmt Expr ";" SetStmt Block ;
BreakStmt       = "break" ";" ;
ContinueStmt    = "continue" ";" ;
ReturnStmt      = "return" ExprList? ";" ;
RevertStmt      = "revert" (StringLiteral | Ident "(" ExprList? ")") ";" ;
AssertStmt      = "assert" "(" Expr "," StringLiteral ")" ";" ;
RequireStmt     = "require" "(" Expr "," StringLiteral ")" ";" ;
EmitStmt        = "emit" Ident "(" ExprList? ")" ";" ;
PlaceholderStmt = "_" ";" ;
ExprStmt        = Expr ";" ;

ExprList        = Expr ("," Expr)* ;
Expr            = ... ;  // standard precedence grammar omitted for brevity
LValue          = Ident | IndexExpr | MemberExpr ;
IndexExpr       = PrimaryExpr "[" Expr "]" ("[" Expr "]")* ;
MemberExpr      = PrimaryExpr "." Ident ;
PrimaryExpr     = Ident | Literal | "(" Expr ")" | NewExpr | CallExpr ;
NewExpr         = "new" Type "(" ExprList? ")" ;
CallExpr        = PrimaryExpr "(" ExprList? ")" ;
```

---

## 19. TRC20 Example (TOL Sketch)

```tol
tol 0.2

contract TRC20 {
  storage {
    slot name: bytes;
    slot symbol: bytes;
    slot decimals: u8;
    slot total_supply: u256;
    slot balances: mapping(address => u256);
    slot allowances: mapping(address => mapping(address => u256));
  }

  event Transfer(from: address indexed, to: address indexed, value: u256)
  event Approval(owner: address indexed, spender: address indexed, value: u256)

  constructor(owner: address, supply: u256) {
    sstore(total_supply, supply)
    set balances[owner] = supply;
    emit Transfer(0x0000000000000000000000000000000000000000000000000000000000000000, owner, supply)
  }

  fn balanceOf(owner: address) -> (balance: u256) public view {
    let b: u256 = balances[owner]
    return b;
  }

  fn transfer(to: address, amount: u256) -> (ok: bool) public {
    let from: address = msg.sender
    let from_bal: u256 = balances[from]
    require(from_bal >= amount, "INSUFFICIENT_BALANCE");
    set balances[from] = from_bal - amount;
    let to_bal: u256 = balances[to]
    set balances[to] = to_bal + amount;
    emit Transfer(from, to, amount)
    return true;
  }

  fn approve(spender: address, amount: u256) -> (ok: bool) public {
    let owner: address = msg.sender
    set allowances[owner][spender] = amount;
    emit Approval(owner, spender, amount)
    return true;
  }

  fn transferFrom(from: address, to: address, amount: u256) -> (ok: bool) public {
    let spender: address = msg.sender
    let allow: u256 = allowances[from][spender]
    require(allow >= amount, "INSUFFICIENT_ALLOWANCE");
    set allowances[from][spender] = allow - amount;

    let from_bal: u256 = balances[from]
    require(from_bal >= amount, "INSUFFICIENT_BALANCE");
    set balances[from] = from_bal - amount;

    let to_bal: u256 = balances[to]
    set balances[to] = to_bal + amount;

    emit Transfer(from, to, amount)
    return true;
  }

  fallback {
    revert "UNKNOWN_SELECTOR";
  }
}
```

---

## 20. Implementation Status in Tolang (as of 2026-03-01)

Implemented:

1. TOL lexer/parser/AST package exists in `tolang/tol`.
2. Accepted top-level contract subset:
   `contract`, `storage`, `event`, `fn`, `constructor`, `fallback`.
3. Statement subset works:
   `let`, `set`, `if/else`, `while`, `for`, `break`, `return`,
   `require`, `assert`, `revert "..."`, `emit`, expression statement.
4. Expression subset works:
   unary/binary ops, calls, member/index access, nested mapping index chains.
5. Canonical compiler route is direct:
   `TOL typed/lowered -> IR -> bytecode` (no Lua transpile stage).
6. External/public dispatch wrappers are generated using selector strings:
   default `keccak256("name(type1,type2,...)")[0:4]` in `0x????????` form,
   and `@selector("0x........")` override.
7. Constructor hook works and forwards runtime args via `tos.oncreate(...)`.
8. Toolchain exists:
   - APIs: `BuildIRFromTOL`, `CompileTOLToBytecode`
   - CLI: `tolang -ctol` and `tolang -dtol`
9. Deterministic integer intrinsics available in runtime math lib:
   `math.binaryLog`, `math.pow2` (with `lower/mid/upper` modes) and tests.
10. `selector("sig")` literal calls are lowered as compile-time selector constants
    (`0x????????`).
11. Selector member builtins are supported for externally dispatchable functions:
    `this.fn.selector` and `Contract.fn.selector`.
    `@selector("0x........")` override is accepted only on `public`/`external` functions.
12. Storage lowering in direct-IR path supports a deterministic core subset:
    scalar slot reads/writes, mapping reads/writes (with exact declared key depth),
    and top-level storage dynamic array `arr[i]`, `arr.length`, `arr.push(v)`.
13. Early semantic verifier now rejects unsupported/conflicting function modifiers,
    duplicate function/constructor modifiers, duplicate parameter names,
    and invalid return-value usage
    (void/non-void function and constructor/fallback return shape checks).
    Constructor modifier subset is also validated (allowed subset + conflict checks).
14. Early semantic verifier validates storage-access shape for implemented subset:
    mapping key-depth arity, scalar non-indexability, and array-only `.length`/`.push(v)`
    on top-level storage arrays (`.length` is read-only).
15. Early semantic verifier validates local contract-call arity
    (`fn(...)`, `this.fn(...)`, `Contract.fn(...)`) and assignment-expression
    target assignability in expression context.
    Assignment targets using literal identifiers (`true`/`false`/`nil`) are rejected.
    Selector-member targets (`*.selector`) are read-only and rejected as lvalues.
16. Early semantic verifier restricts assignment-expression placement to supported
    statement contexts (expression statement / `for` post), rejecting value-context use.
17. Non-void functions require all current-stage structured control paths to
    terminate with value-return or `revert` (loops are still conservatively treated).
18. Statement-shape checks enforce current subset contracts:
    `require/assert` must carry expression payload; `emit` must carry identifier-call payload.
    `selector("...")` is expression-only and cannot appear as standalone statement.
19. `revert` payload is constrained to empty or string-literal form in current stage.
20. For declared events, `emit EventName(...)` argument count is verifier-checked
    against the declaration arity.
21. Event declaration names are uniqueness-checked at contract scope.
22. If a contract declares events, `emit` must reference a declared event name.
23. Cross-namespace name collision checks are enforced for this stage
    (`event`/`fn`/`storage slot` identifiers must not collide).
24. Duplicate-name checks apply to event parameter lists and function return-name lists.
25. Function parameter names must not collide with function return-field names.
26. Local `let` declarations are uniqueness-checked per lexical scope
    (same-scope duplicates rejected; nested-scope shadowing allowed).
27. Unreachable statements are rejected after terminal control-flow
    (`return`, `revert`, loop-body `break/continue`, or `if/else` where both branches terminate).
28. Contract-scoped member calls (`this.fn(...)`, `Contract.fn(...)`) must
    resolve to declared contract functions.

Partially implemented:

1. `interface`/`library` declarations are currently parsed in skip mode
   (accepted syntactically, not compiled to semantics).
2. `error`/`enum`/`modifier` declarations are currently skipped, not enforced.
3. Constructor parameters are accepted and forwarded by wrapper call only;
   typed ABI decode/binding semantics are not implemented yet.
4. Storage lowering currently uses an internal deterministic runtime table model;
   full spec-level canonical slot hashing/addressing and host-persistent storage
   builtins are not wired yet.
5. `continue` semantics are lowered via deterministic labels/goto in loops,
   but still need deeper verifier checks for corner-case control flow.
6. Signed type names (`i*`) are accepted in types/signatures, but full Solidity-like
   signed arithmetic/cast verifier semantics are not yet complete.
7. `selector("sig")` currently requires a string literal argument in direct IR mode;
   dynamic selector expressions are not implemented.
8. Selector member builtins currently work only for externally dispatchable
   targets in current stage.
9. `math.binaryLog`/`math.pow2` are implemented; `math.max(xs: i256[])` in spec
   is not implemented as a dedicated array intrinsic.

Not implemented yet (spec gap):

1. Full direct `TOL typed/lowered -> IR -> bytecode` backend completeness for
   all v0.2 mandatory features.
2. Full verifier pipeline (name resolution, CFG/effect checks, selector uniqueness,
   inheritance checks, modifier expansion checks, interface conformance, etc.).
3. ABI high-level typed operations in TOL surface (`abi.decode/encode*`, tuple destructure).
4. Custom error form `revert ErrorName(...)`.
5. Inheritance/C3 linearization/`super` dispatch.
6. Full host-call builtin lowering coverage (`create`, `create2`, `delegatecall`, etc.)
   from TOL surface semantics.

Roadmap reference:

1. Detailed milestone planning is maintained in `45-IR/TOL_ROADMAP.md`.

---

## 21. Migration Guidance

1. Treat TOL as the only stable deploy target for production pipelines.
2. Agent/compiler outputs should be direct TOL, then compiled to bytecode.
3. `TOL -> bytecode` is the canonical and only supported deploy path.

---

## 22. CTMM Coverage Matrix (`contracts/*.sol`)

Mandatory language/runtime coverage to claim full support:

1. Signed integers (`int`, `int[]`) and signed arithmetic semantics:
   needed by `MarketMaker.sol`, `LMSRMarketMaker.sol`.
2. Dynamic and nested arrays (`uint[]`, `bytes32[]`, `bytes32[][]`, `address[]`):
   needed by `FixedProductMarketMaker*.sol`, `Whitelist.sol`, factories.
3. Deployment builtins (`new`, `create2`) without inline assembly:
   needed by `Create2CloneFactory.sol`, `FPMMDeterministicFactory.sol`.
4. Inheritance/interfaces/modifiers/abstract signatures:
   needed across `TRC20.sol`, `MarketMaker.sol`, factory contracts.
5. ABI high-level operations (`decode`, `encodeWithSignature`, selectors, tuple decode):
   needed by clone constructor payload flows and TRC1155 selector handling.
6. Deterministic integer `log/exp` intrinsics (`binaryLog`, `pow2`, rounding modes, scale semantics):
   needed by `LMSRMarketMaker.sol`.

Conformance criterion:

`contracts/*.sol` must lower to TOL v0.2 (or direct equivalent hand-written TOL)
without semantic fallback to unsupported features.

Current status on 2026-03-01:

1. This conformance criterion is not yet met.
2. Main blockers are inheritance/modifiers/interfaces, typed ABI ops, and
   full verifier + direct TOL backend completeness.

End of draft.
