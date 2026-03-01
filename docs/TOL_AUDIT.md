# TOL Security Design Principles

Status: Draft v0.1 (2026-03-01)
Owner: GTOS/Tolang engineering
Scope: Security architecture of the TOL language and runtime

---

## 1. Philosophy

TOL's security model is built on a single principle:

> **Make dangerous operations inexpressible, not just inadvisable.**

Solidity's approach relies on auditors, linters (Slither, Mythril), and conventions
(OpenZeppelin) to catch security bugs after the fact. TOL eliminates entire vulnerability
classes at the language design level, so they cannot appear in compiled contracts regardless
of programmer skill.

---

## 2. Eliminated Vulnerability Classes

### 2.1 Reentrancy

**Root cause in Solidity:** External calls (`call`, `delegatecall`) can execute arbitrary
code that re-enters the calling contract before its state is updated.

**TOL elimination:**
- No `call` opcode or arbitrary external code execution primitive.
- Cross-contract interaction is message-passing only; the callee cannot execute code
  in the context of the caller.
- No callback mechanism exists that could re-enter a contract mid-execution.

### 2.2 Integer Overflow and Underflow

**Root cause in Solidity:** Pre-0.8 Solidity had silent wrapping arithmetic. Post-0.8
introduced checked arithmetic by default, but wrapping is still opt-in via `unchecked {}`.

**TOL elimination:**
- All arithmetic is **checked by default**: overflow or underflow causes an immediate revert.
- Wrapping arithmetic requires an explicit annotation (`wrapping` keyword); auditors can
  grep for every wrapping operation site in a contract.
- No implicit type coercion that silently truncates values.

### 2.3 tx.origin Authentication Bypass

**Root cause in Solidity:** `tx.origin` returns the original transaction sender, not the
immediate caller, enabling phishing attacks where a malicious contract tricks a user into
authorizing an action on a target contract.

**TOL elimination:**
- `tx.origin` is not exposed. Only `msg.sender` is available.
- Authentication logic written against `msg.sender` is always correct with respect to the
  immediate call chain.

### 2.4 selfdestruct

**Root cause in Solidity:** `selfdestruct` forcibly sends ETH to any address and destroys
contract storage, enabling griefing attacks and unexpected fund transfers.

**TOL elimination:**
- No `selfdestruct` primitive. Contracts cannot be destroyed.
- Storage is permanent and auditable for the lifetime of the contract.

### 2.5 delegatecall and Storage Layout Collisions

**Root cause in Solidity:** `delegatecall` executes code from another contract in the
caller's storage context. Proxy contracts that use `delegatecall` are vulnerable to storage
slot collisions between the proxy and the implementation.

**TOL elimination:**
- No `delegatecall`. All contract calls execute in their own storage context.
- TOL's storage model uses canonical keccak256-derived slot addresses
  (`keccak256("tol.slot.<Contract>.<name>")`), making slot identity explicit and
  collision-free by construction.

### 2.6 Dynamic Code Loading

**Root cause in Solidity/EVM:** `CREATE` and `CREATE2` can deploy arbitrary bytecode,
and inline assembly gives unrestricted EVM opcode access.

**TOL elimination:**
- No inline assembly.
- No arbitrary bytecode deployment. `create`/`create2` accept only compiled TOL contracts.
- The Lua runtime base library removes `load`, `loadstring`, `dofile`, and `require`
  (dynamic module loading), preventing any runtime code injection.

### 2.7 Non-Deterministic Host APIs

**Root cause:** Contracts that read wall-clock time, system randomness, or host environment
variables produce different outputs on different validator nodes, breaking consensus.

**TOL elimination:**
- The Lua runtime strips: `os`, `io`, `coroutine`, `debug`, `math.random`,
  `math.randomseed`, `collectgarbage`, `dofile`, `require`.
- All observable behavior depends only on: block context fields, transaction input,
  contract storage, and deterministic builtins.

---

## 3. Language-Level Safety Enforcements

### 3.1 Explicit State Mutation

Storage writes require the `set` keyword:

```tol
set balances[from] = from_bal - amount;
```

Reads and writes are syntactically distinct. An auditor can identify every state-modifying
operation by searching for `set`. Implicit mutation does not exist.

### 3.2 Typed Storage Slots

Each storage slot has a declared type fixed at contract definition time:

```tol
storage {
    slot total_supply: u256;
    slot balances: mapping(address => u256);
}
```

Type mismatches are rejected at compile time. A `u256` slot cannot be read as `address`
or vice versa.

### 3.3 Explicit Fallback with Default Revert

Contracts that do not declare a `fallback` block automatically revert on unknown selectors.
Contracts that declare `fallback` must do so explicitly:

```tol
fallback { revert "UNKNOWN_SELECTOR"; }
```

There is no silent acceptance of unknown calls or accidental ETH reception.

### 3.4 No Implicit Fallback Value Transfer

Contracts do not receive value transfers unless they explicitly handle `msg.value` in a
declared function. There is no payable-by-default behavior.

---

## 4. Verifier-Enforced Invariants

The TOL compiler verifier (mandatory before codegen) enforces:

### 4.1 Return Coverage

Every non-void function must have a `return` or `revert` on every control flow path.
Functions that silently fall off the end (a common source of undefined behavior) are
rejected at compile time.

### 4.2 Unreachable Code Detection

Statements after `return`, `revert`, `break`, or `continue` are rejected. Dead code is
never compiled into a deployed contract.

### 4.3 Variable Scope Uniqueness

Re-declaration of a local variable in the same scope is rejected. Variable shadowing
across nested scopes is flagged.

### 4.4 Checks-Effects-Interactions Order (planned)

The verifier will enforce that within a function, all storage reads and condition checks
(`require`/`assert`) precede all storage writes, which precede all external calls.
Violations of CEI order will be compile-time errors, not audit recommendations.

---

## 5. Auditor's Reference

### What to look for in a TOL contract

| Concern | What to check |
|---------|---------------|
| Arithmetic safety | All arithmetic is checked by default. Search for `wrapping` to find every opt-out site. |
| State mutation | Every state write is a `set` statement. No hidden mutations exist. |
| Access control | Authentication uses `msg.sender` only. No `tx.origin` risk. |
| Fallback behavior | Explicit `fallback` block required. Implicit revert if absent. |
| Reentrancy | Not possible. No callback or re-entry mechanism exists. |
| Storage collisions | Slot addresses are canonical keccak256 hashes. Collisions are cryptographically infeasible. |

### What TOL cannot protect against

1. **Logic errors** — Incorrect business logic (wrong amount calculations, missing access
   control checks) are not detectable by the language or verifier.
2. **Oracle manipulation** — If a contract trusts external data feeds, manipulating those
   feeds is outside TOL's scope.
3. **Economic attacks** — Flash loan attacks, sandwich attacks, and MEV exploitation depend
   on economic incentives, not language semantics.

---

## 6. Comparison with Solidity

| Property | Solidity | TOL |
|----------|----------|-----|
| Reentrancy possible | Yes (requires CEI discipline) | No (no re-entry mechanism) |
| Integer overflow | Checked by default (0.8+), wrapping opt-in | Checked always, wrapping explicit |
| tx.origin exposure | Yes | No |
| selfdestruct | Yes | No |
| delegatecall | Yes | No |
| Inline assembly | Yes | No |
| Dynamic code loading | Yes (CREATE + assembly) | No |
| Non-deterministic APIs | Yes (block.timestamp, etc.) | Restricted to audited host builtins |
| Storage layout | Implicit slot numbering | Canonical keccak256 slot identity |
| Fallback default | Silent accept or revert depending on version | Always revert unless explicitly handled |
| Audit surface | Large (full EVM opcode set) | Small (restricted language + runtime) |

---

## 7. Design Constraint Policy

The security properties described in this document are **language-level design constraints**,
not implementation recommendations. Future versions of the TOL compiler and runtime must
preserve these constraints. Any feature addition that weakens a listed property requires
explicit revision of this document and a security rationale.
