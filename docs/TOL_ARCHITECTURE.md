# TOL Architecture Lock (M0)

Status: Active snapshot (M0 lock + incremental implementation)  
Date: 2026-03-01

## Package Boundaries

Canonical compiler path packages:

1. `tol/lexer` - tokenization with source positions.
2. `tol/parser` - syntax parsing to `tol/ast`.
3. `tol/ast` - TOL syntax tree nodes.
4. `tol/sema` - symbol/type/semantic validation into typed form.
5. `tol/lower` - typed TOL lowering into backend-neutral program form.
6. `tol/codegen` - backend program to executable bytecode.
7. `tol/diag` - structured diagnostics (error code + span).

Root package integration APIs (stable entry points):

1. `ParseTOLModule(source, name)`.
2. `BuildIRFromTOL(source, name)`.
3. `CompileTOLToBytecode(source, name)`.

## Error Model

Diagnostics are structured and deterministic:

1. Error code: stable identifier (example: `TOL1001`).
2. Message: human-readable explanation.
3. Source span: `file:line:column`.

Current code ranges:

1. `TOL1xxx` parser/lexer.
2. `TOL2xxx` semantic checks.
3. `TOL3xxx` lowering.
4. `TOL4xxx` code generation.

## Architecture Policy

1. Public deploy pipeline remains locked to
   `TOL source -> typed/lowered TOL -> direct IR -> bytecode`.
2. Transpile bridge routes are not part of the supported architecture.
3. Deterministic constraints in `AGENTS.md` remain hard requirements and are not relaxed by TOL work.

## Current Milestone Scope

Current branch keeps M0 package boundaries and evolves the canonical backend path
incrementally on direct IR lowering (`typed/lowered -> IR -> bytecode`).

Current parser/sema coverage in this branch:

1. Top-level: `tol`, `interface` (skip), `library` (skip), `contract`.
2. Contract members: `storage`, `event`, `fn`, `constructor`, `fallback`,
   plus `error`/`enum`/`modifier` in skip mode.
3. Statement AST subset inside function-like bodies:
   `let`, `set`, `if/else`, `while`, `for`, `break`, `continue`,
   `return`, `require`, `assert`, `revert`, `emit`, expression statement.
4. Expression AST subset:
   literals/identifiers, unary (`+`, `-`, `!`), binary (`=`, `||`, `&&`,
   equality/relational, bitwise/shift (`&`, `|`, `^`, `~`, `<<`, `>>`),
   `+`, `-`, `*`, `/`, `%`), call/member/index postfix, selector builtins
   (`selector("sig")`, `this.fn.selector`, `Contract.fn.selector`) for
   externally dispatchable targets.
5. Early semantic checks:
   version gate, missing contract, duplicate storage slots, duplicate function
   names (no overload yet), break/continue loop-context validation,
   `set` target assignability checks,
   function modifier validation (unsupported/conflicting/duplicate combinations),
   constructor modifier validation (supported subset + conflict/duplicate checks),
   duplicate parameter-name checks, return-value shape checks
   (void/non-void/constructor/fallback),
   storage-access shape checks for implemented subset
   (mapping depth arity, scalar indexing rejection, array `.length`/`.push` constraints,
   including `.length` read-only target checks),
   contract-local function call arity checks
   (`fn(...)`, `this.fn(...)`, `Contract.fn(...)`), assignment-expression target checks,
   direct unscoped-call visibility checks (`fn(...)` cannot target `external`),
   contract-scoped member-call target resolution checks,
   contract-scoped member-call visibility checks (`public/external` only),
   literal-identifier assignment-target rejection (`true`/`false`/`nil`),
   selector-member assignment-target rejection (`*.selector` is read-only),
   assignment-expression placement checks (value-context rejection, including
   `require/assert` expressions, `revert` payloads, and `emit` payload arguments),
   non-void function return-path checks for current structured subset
   (all paths must value-return or `revert`; loops still conservative),
   statement-shape checks for current subset (`require/assert` condition + string-literal message payload,
   `emit` identifier-call payload, unknown statement/expression-kind rejection),
   expression-only builtin statement rejection (`selector(...)` standalone/post usage
   and `emit selector(...)` payload target rejection),
   `revert` payload shape checks (empty or string literal in current stage),
   declared-event `emit` arity checks,
   declared-event-name resolution checks for `emit`,
   duplicate event-name checks,
   cross-namespace identifier collision checks (`event`/`fn`/`storage`),
   duplicate-name checks for event params and function return-name lists,
   function parameter/return-field name collision checks,
   lexical-scope duplicate local declaration checks (`let`),
   unreachable-statement checks after terminal control flow
   (`return`/`revert`, loop-body `break`/`continue`, terminating `if/else`),
   selector-override visibility checks (`@selector` only on public/external),
   reserved builtin/internal-name checks (`selector`/`this` and `__tol_*`
   cannot be declared as contract name/member names),
   `@selector("0x........")` format/uniqueness checks for
   `public`/`external` dispatch entries, and selector expression validation
   (`selector("sig")` signature-form literal-only with non-empty canonical arg entries,
   `this/Contract.fn.selector` target
   existence + visibility, and selector-value non-callability).
6. Lowering now builds a backend-neutral `lower.Program` skeleton
   (contract/storage/functions/constructor/fallback metadata) and lowers
   directly to VM IR in the compiler backend (no Lua transpile stage in the
   canonical pipeline).
   Current direct-IR backend supports empty contracts, function definitions,
   constructor bodies (with runtime argument forwarding), and fallback bodies
   with restricted modifier set
   (`public/external/internal/private/view/pure/payable`) and
   statement/expression subset lowering (`let/set/if/while/for/break/continue/
   return/call` paths).
   Runtime wrappers generated by direct IR lowering now expose:
   - `tos.oncreate(...)` -> `__tol_constructor(...)`
   - `tos.oninvoke(selector, ...)` -> selector-string dispatch for
     `public/external` functions (default `keccak4(signature)` in
     `0x????????` form, supports `@selector("0x........")` override),
     fallback, or deterministic `UNKNOWN_SELECTOR`.
   Storage lowering in this direct-IR path now supports deterministic subset
   operations (scalar/mapping/array core access), while canonical slot hashing
   and persistent host-backed storage semantics remain to be completed.
