# TOL Built-in Testing Framework

Status: Design Draft v0.1 (2026-03-01)
Owner: GTOS/Tolang engineering
Scope: Language-level test syntax, runner semantics, and coverage model for TOL contracts

---

## 1. Motivation

Go and Rust ship testing as a first-class language feature (`_test.go`, `#[test]`).
Java's commercial success owes much to mature test tooling (JUnit, JTest, JCoverage).
TOL adopts the same philosophy: testing must be a language-level primitive, not an
afterthought bolted on via external frameworks.

Smart-contract testing has requirements beyond ordinary unit testing:

1. **Deployment state** — a contract must be instantiated (constructor) before calls.
2. **Call context** — each invocation carries `msg.sender`, `msg.value`, `block.*`.
3. **Revert assertions** — a test must be able to assert that a call fails with a
   specific message, and that state is unchanged after the revert.
4. **Event assertions** — contracts communicate side-effects via events; tests must
   verify that the right events were emitted with the right arguments.
5. **Storage isolation** — each test runs against a fresh contract instance; state
   does not bleed between tests.
6. **Coverage** — the runner reports which functions and storage paths were exercised.

---

## 2. File Convention

Test files use the `_test.tol` suffix and are **never included in production bytecode**:

```
trc20.tol          ← production contract
trc20_test.tol     ← test file (excluded from deployment artifact)
```

The compiler and runner treat `*_test.tol` files identically to how Go treats
`*_test.go`: they are compiled only when the test runner is invoked
(`tol test ./...`), and they may import and instantiate contracts from sibling `.tol`
files.

---

## 3. Test File Structure

```tol
tol 0.2

import TRC20 from "trc20.tol"

test TRC20Test {

  -- shared address constants (available to all test functions)
  let alice:   address = 0x000000000000000000000000000000000000000000000000000000000000a11c;
  let bob:     address = 0x000000000000000000000000000000000000000000000000000000000000b0b0;
  let charlie: address = 0x000000000000000000000000000000000000000000000000000000000000c4a0;

  -- setup() runs before each test function with a fresh contract instance
  setup -> (token: TRC20) {
    deploy TRC20(owner: alice, supply: 1000u256) -> token;
  }

  fn test_constructor_sets_supply(token: TRC20) {
    assert_eq token.totalSupply(), 1000u256;
  }

  fn test_constructor_credits_owner(token: TRC20) {
    assert_eq token.balanceOf(alice), 1000u256;
    assert_eq token.balanceOf(bob),   0u256;
  }

  fn test_transfer_moves_balance(token: TRC20) {
    with msg.sender = alice {
      token.transfer(bob, 300u256);
    }
    assert_eq token.balanceOf(alice), 700u256;
    assert_eq token.balanceOf(bob),   300u256;
  }

  fn test_transfer_insufficient_reverts(token: TRC20) {
    with msg.sender = alice {
      assert_revert("INSUFFICIENT_BALANCE") {
        token.transfer(bob, 9999u256);
      }
    }
    -- state must be unchanged after revert
    assert_eq token.balanceOf(alice), 1000u256;
  }

  fn test_transfer_emits_event(token: TRC20) {
    with msg.sender = alice {
      token.transfer(bob, 100u256);
    }
    assert_event Transfer(from: alice, to: bob, value: 100u256);
  }

  fn test_approve_and_transfer_from(token: TRC20) {
    with msg.sender = alice {
      token.approve(charlie, 400u256);
    }
    with msg.sender = charlie {
      token.transferFrom(alice, bob, 100u256);
    }
    assert_eq token.balanceOf(bob),   100u256;
    assert_eq token.balanceOf(alice), 900u256;
  }

}
```

---

## 4. Syntax Reference

### 4.1 Test block

```
test <Name> {
  <let-decls>
  setup -> (<bindings>) { <body> }
  teardown (<bindings>) { <body> }       -- optional, runs after each test
  fn test_<name>(<bindings>) { <body> }  -- one or more test functions
}
```

- `<Name>` is a label used for reporting; it does not declare a TOL contract.
- Any number of `fn test_*` functions may appear.
- Only functions prefixed `test_` are executed by the runner.
- Helper functions without the `test_` prefix are compiled but not run directly.

### 4.2 Setup and teardown

```tol
setup -> (token: TRC20, other: SomeContract) {
  deploy TRC20(owner: alice, supply: 1000u256) -> token;
  deploy SomeContract() -> other;
}

teardown (token: TRC20) {
  -- cleanup if needed (usually not necessary; state is isolated per test)
}
```

- `setup` is called before each test function.
- The return bindings of `setup` are passed as parameters to each test function
  by name matching.
- If a test function omits a binding, that contract is still deployed but not
  passed to the function.
- `teardown` receives the same bindings and runs after each test (pass or fail).

### 4.3 deploy statement

```tol
deploy <ContractName>(<arg>, ...) -> <binding>;
```

- Calls the contract constructor with the given arguments.
- The result is a test handle bound to `<binding>`.
- Each `deploy` produces an isolated storage namespace; two deployed instances
  of the same contract do not share storage.

### 4.4 Call context block

```tol
with msg.sender = <addr> { <stmts> }
with msg.sender = <addr>, msg.value = <val> { <stmts> }
```

- Overrides call context fields for the duration of the block.
- Context is restored to its previous value on block exit (even on revert).
- Nesting is allowed:
  ```tol
  with msg.sender = alice {
    with msg.value = 100u256 {
      token.deposit();
    }
  }
  ```

### 4.5 Assertions

#### assert_eq / assert_ne

```tol
assert_eq <expr>, <expr>;
assert_ne <expr>, <expr>;
```

Fails the test immediately with a descriptive message if the condition does not hold.

#### assert_revert

```tol
assert_revert(<message>) { <stmts> }
assert_revert { <stmts> }   -- accepts any revert
```

- Asserts that the body reverts.
- If `<message>` is given, asserts the revert message matches exactly.
- After a successful `assert_revert`, storage state is verified to be unchanged
  (the runner snapshots state before the block and diffs after).
- If the body does **not** revert, the test fails.

#### assert_event

```tol
assert_event <EventName>(<field>: <value>, ...);
```

- Asserts that the most recently emitted event matches the given name and field values.
- Fields may be omitted to perform partial matching.
- `assert_no_event` asserts that no event was emitted since the last check.

#### assert / assert_true / assert_false

```tol
assert <bool_expr>;
assert_true  <bool_expr>;
assert_false <bool_expr>;
```

### 4.6 Direct storage inspection (white-box testing)

Within a test file, storage slots of a deployed contract instance are readable
via the `inspect` operator:

```tol
let supply: u256 = inspect token.total_supply;
assert_eq supply, 1000u256;
```

`inspect` is available only in test files and is not part of the production language.
It bypasses the function call interface to read storage directly — useful for
verifying invariants without requiring a public getter.

---

## 5. Runner Semantics

### 5.1 Invocation

```sh
tol test ./...              # run all *_test.tol files recursively
tol test ./trc20_test.tol   # run a specific test file
tol test -run test_transfer  # run tests whose name matches a pattern
tol test -v                  # verbose: print each test name as it runs
```

### 5.2 Execution model

For each test function:

1. A fresh Lua VM state is created.
2. `setup` is called; its bound contracts are deployed into isolated storage.
3. The test function runs.
4. `teardown` (if declared) runs.
5. The VM state is discarded; no state persists to the next test.

Tests within a `test` block run sequentially in declaration order.
Parallel execution may be added in a future version.

### 5.3 Output format

```
--- PASS: TRC20Test/test_constructor_sets_supply     (0.001s)
--- PASS: TRC20Test/test_constructor_credits_owner   (0.001s)
--- PASS: TRC20Test/test_transfer_moves_balance       (0.002s)
--- FAIL: TRC20Test/test_transfer_insufficient_reverts
    trc20_test.tol:41: assert_eq failed: got 999, want 1000
--- PASS: TRC20Test/test_transfer_emits_event         (0.001s)

FAIL    trc20_test.tol  [1 failure]
```

### 5.4 Exit codes

- `0` — all tests passed
- `1` — one or more tests failed
- `2` — compilation error in test or contract file

---

## 6. Coverage

The runner collects coverage automatically when invoked with `-cover`:

```sh
tol test -cover ./...
```

Coverage is reported at three granularities:

### 6.1 Function coverage

Which public functions were called at least once across the test suite.

```
trc20.tol  function coverage:
  totalSupply()                    called  ✓
  balanceOf(address)               called  ✓
  transfer(address, u256)          called  ✓
  approve(address, u256)           called  ✓
  transferFrom(address, address, u256)  called  ✓
  fallback                         not called  ✗

Function coverage: 5/6 (83%)
```

### 6.2 Branch coverage

Which conditional branches (`if`/`else`, `require` pass/fail paths) were taken.

```
trc20.tol  branch coverage:
  transfer:31  require pass    ✓
  transfer:31  require fail    ✓
  transferFrom:56  require pass  ✓
  transferFrom:56  require fail  ✓
  transferFrom:61  require pass  ✓
  transferFrom:61  require fail  ✗   -- not tested

Branch coverage: 5/6 (83%)
```

### 6.3 Storage slot coverage

Which storage slots were read and written during the test suite.

```
trc20.tol  storage slot coverage:
  total_supply     read ✓  written ✓
  balances         read ✓  written ✓
  allowances       read ✓  written ✓
```

### 6.4 Coverage output formats

```sh
tol test -cover -coverformat=text   # terminal table (default)
tol test -cover -coverformat=json   # machine-readable JSON
tol test -cover -coverformat=html   # annotated source HTML
```

---

## 7. Design Constraints

1. **No production impact** — `*_test.tol` files are never compiled into deployment
   artifacts. The `inspect` operator, `deploy` statement, `assert_*` builtins, and
   `with` context blocks are test-only and rejected by the production compiler.

2. **Storage isolation** — each test function runs against a fresh VM state.
   No global mutable state is shared between tests.

3. **Determinism** — tests are deterministic by construction (same as production
   contracts). No wall-clock time, randomness, or host OS access.

4. **Revert isolation** — a revert inside `assert_revert { }` does not propagate
   to the test function. State is snapshotted and restored automatically.

5. **Minimal new syntax** — `test`, `setup`, `teardown`, `deploy`, `with`,
   `inspect`, `assert_eq`, `assert_ne`, `assert_revert`, `assert_event` are the
   complete set of test-only additions. No other test-specific keywords are needed.

---

## 8. Comparison with Existing Approaches

| Feature | Go (`testing`) | Rust (`#[test]`) | Hardhat/Foundry | TOL |
|---------|---------------|-----------------|-----------------|-----|
| Built into language | Yes | Yes | No (JS/TS framework) | Yes |
| File isolation | `_test.go` | same file or `tests/` | separate `.js/.ts` | `_test.tol` |
| Contract deploy | N/A | N/A | `deployContract()` | `deploy` statement |
| Call context | N/A | N/A | `connect(signer)` | `with msg.sender` |
| Revert assertion | N/A | `#[should_panic]` | `expect(...).to.be.reverted` | `assert_revert { }` |
| Event assertion | N/A | N/A | `expect(...).to.emit` | `assert_event` |
| Storage inspection | N/A | N/A | `ethers.provider.getStorage` | `inspect` |
| Coverage built-in | Yes (`-cover`) | Yes (`cargo llvm-cov`) | No (external) | Yes (`-cover`) |
| State isolation | per-test function | per-test function | manual `beforeEach` | automatic per-test |

---

## 9. Implementation Roadmap

| Phase | Deliverable |
|-------|-------------|
| P0 | `*_test.tol` file recognition; `test` block parsing; `deploy`/`with`/`assert_eq`/`assert_revert` |
| P1 | `setup`/`teardown` lifecycle; storage isolation per test; basic pass/fail reporting |
| P2 | `assert_event`; `inspect` operator; coverage (function + branch) |
| P3 | Coverage HTML output; `-run` pattern filter; parallel test execution |
| P4 | Differential testing against reference Solidity implementation |
