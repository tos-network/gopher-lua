# TOL Built-in Testing Framework

Status: Design Draft v0.2 (2026-03-01)
Owner: GTOS/Tolang engineering
Scope: Language-level test syntax, runner semantics, and coverage model for TOL contracts

---

## 1. Motivation

Go and Rust ship testing as a first-class language feature (`_test.go`, `#[test]`).
Java's commercial success owes much to mature test tooling (JUnit, JTest/Parasoft,
JaCoCo/JCoverage). TOL adopts the same philosophy: testing must be a language-level
primitive, not an afterthought bolted on via external frameworks.

Smart-contract testing has requirements beyond ordinary unit testing:

1. **Deployment state** — a contract must be instantiated (constructor) before calls.
2. **Call context** — each invocation carries `msg.sender`, `msg.value`, `block.*`.
3. **Revert assertions** — a test must assert that a call fails with a specific message,
   and that storage is unchanged after the revert.
4. **Event assertions** — contracts communicate side-effects via events; tests must
   verify that the right events were emitted with the right arguments.
5. **Storage isolation** — each test runs against a fresh contract instance; state
   does not bleed between tests.
6. **Coverage** — the runner reports which functions, branches, and storage paths
   were exercised.

---

## 2. File Convention

Test files use the `_test.tol` suffix and are **never included in production bytecode**:

```
trc20.tol          ← production contract
trc20_test.tol     ← test file (excluded from deployment artifact)
```

The compiler and runner treat `*_test.tol` files identically to how Go treats
`*_test.go`: compiled only when the test runner is invoked (`tol test ./...`),
and may import and instantiate contracts from sibling `.tol` files.

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

  -- setup_suite() runs once before all tests in this block (JUnit @BeforeClass equivalent)
  setup_suite -> (registry: Registry) {
    deploy Registry() -> registry;
  }

  -- setup() runs before each test function with a fresh contract instance
  setup -> (token: TRC20) {
    deploy TRC20(owner: alice, supply: 1000u256) -> token;
  }

  -- teardown() runs after each test (pass or fail)
  teardown (token: TRC20) { }

  -- teardown_suite() runs once after all tests (JUnit @AfterClass equivalent)
  teardown_suite (registry: Registry) { }

  fn test_constructor_sets_supply(token: TRC20) {
    assert_eq token.totalSupply(), 1000u256;
  }

  fn test_constructor_credits_owner(token: TRC20) {
    assert_eq token.balanceOf(alice), 1000u256;
    assert_eq token.balanceOf(bob),   0u256;
  }

  #[skip]
  fn test_wip_feature(token: TRC20) {
    -- known failing, excluded from run without being deleted
  }

  #[tag("slow")]
  fn test_many_transfers(token: TRC20) {
    -- tagged; run with: tol test -tag slow
  }

  fn test_transfer_moves_balance(token: TRC20) {
    with msg.sender = alice {
      token.transfer(bob, 300u256);
    }
    assert_eq token.balanceOf(alice), 700u256, "alice balance after transfer";
    assert_eq token.balanceOf(bob),   300u256, "bob balance after transfer";
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

  -- Parameterized test: runs once per row in the cases table
  #[cases]
  fn test_transfer_various_amounts(token: TRC20,
      amount: u256, alice_after: u256, bob_after: u256) {
    with msg.sender = alice {
      token.transfer(bob, amount);
    }
    assert_eq token.balanceOf(alice), alice_after;
    assert_eq token.balanceOf(bob),   bob_after;
  } cases {
    | amount  | alice_after | bob_after |
    | 1u256   | 999u256     | 1u256     |
    | 500u256 | 500u256     | 500u256   |
    | 1000u256| 0u256       | 1000u256  |
  }

  -- assert_all: all sub-assertions are evaluated; all failures reported together
  fn test_balances_consistent(token: TRC20) {
    with msg.sender = alice {
      token.transfer(bob, 400u256);
    }
    assert_all {
      assert_eq   token.balanceOf(alice), 600u256, "alice";
      assert_eq   token.balanceOf(bob),   400u256, "bob";
      assert_eq   token.totalSupply(),    1000u256, "supply unchanged";
      assert_gt   token.balanceOf(alice), 0u256,    "alice non-zero";
    }
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

  -- Instruction limit: assert function executes within N VM instructions
  fn test_transfer_gas_bound(token: TRC20) {
    with msg.sender = alice {
      assert_instructions_le(5000) {
        token.transfer(bob, 100u256);
      }
    }
  }

  -- Fuzz entry point: runner feeds mutated inputs, detects panics/unexpected reverts
  #[fuzz]
  fn fuzz_transfer(token: TRC20, amount: u256) {
    with msg.sender = alice {
      -- must not panic (unexpected revert allowed; assert_revert is separate)
      token.transfer(bob, amount);
    }
  }

}
```

---

## 4. Syntax Reference

### 4.1 Test block

```
test <Name> {
  <let-decls>
  setup_suite -> (<bindings>) { <body> }    -- once before all tests
  setup       -> (<bindings>) { <body> }    -- before each test
  teardown       (<bindings>) { <body> }    -- after each test
  teardown_suite (<bindings>) { <body> }    -- once after all tests

  #[attr]
  fn test_<name>(<bindings>) { <body> }     -- test functions
  fn helper_<name>(...) { <body> }          -- helper (not run directly)

  #[cases]
  fn test_<name>(<bindings>, <params>) { <body> } cases { <table> }
  #[fuzz]
  fn fuzz_<name>(<bindings>, <params>) { <body> }
}
```

- Only functions prefixed `test_` or `fuzz_` are executed by the runner.
- Helper functions (any other prefix) are compiled but not run directly.

### 4.2 Lifecycle hooks

| Hook | JUnit equivalent | When it runs |
|------|-----------------|--------------|
| `setup_suite` | `@BeforeClass` | Once, before any test in the block |
| `setup` | `@Before` | Before each `test_*` function |
| `teardown` | `@After` | After each `test_*` function (pass or fail) |
| `teardown_suite` | `@AfterClass` | Once, after all tests in the block |

`setup_suite` bindings are shared across all test functions (read-only contracts,
global registries, etc.). `setup` bindings are fresh per test.

### 4.3 deploy statement

```tol
deploy <ContractName>(<arg>, ...) -> <binding>;
```

- Calls the contract constructor with the given arguments.
- Each `deploy` produces an isolated storage namespace.
- Two deployed instances of the same contract do not share storage.

### 4.4 Call context block

```tol
with msg.sender = <addr> { <stmts> }
with msg.sender = <addr>, msg.value = <val> { <stmts> }
with block.number = <n> { <stmts> }
```

- Overrides call context fields for the duration of the block.
- Context is restored to its previous value on block exit (even on revert).
- Nesting is allowed.

### 4.5 Assertions

#### Equality and comparison

```tol
assert_eq  <expr>, <expr>;
assert_eq  <expr>, <expr>, <"message">;   -- custom failure message
assert_ne  <expr>, <expr>;
assert_ne  <expr>, <expr>, <"message">;
assert_gt  <expr>, <expr>;                -- greater than (u256/i256)
assert_ge  <expr>, <expr>;                -- greater or equal
assert_lt  <expr>, <expr>;                -- less than
assert_le  <expr>, <expr>;                -- less or equal
assert_between <expr>, <low>, <high>;     -- low <= expr <= high
```

#### Boolean

```tol
assert       <bool_expr>;
assert       <bool_expr>, <"message">;
assert_true  <bool_expr>;
assert_false <bool_expr>;
```

#### Grouped assertions (report all failures)

```tol
assert_all {
  assert_eq a, b, "label a";
  assert_eq c, d, "label d";
  assert_gt e, 0u256;
}
```

Unlike sequential assertions that stop at the first failure, `assert_all` runs
every sub-assertion and reports all failures together — equivalent to JUnit 5's
`assertAll()`.

#### Revert assertion

```tol
assert_revert(<message>) { <stmts> }  -- exact message match
assert_revert                { <stmts> }  -- any revert
```

- Storage state is snapshotted before the block and verified unchanged after.
- If the body does **not** revert, the test fails.

#### Event assertion

```tol
assert_event <EventName>(<field>: <value>, ...);   -- full match
assert_event <EventName>();                          -- name only (partial)
assert_no_event;                                     -- nothing emitted
```

#### Instruction limit (gas budget)

```tol
assert_instructions_le(<n>) { <stmts> }
```

Asserts the VM executes at most `<n>` instructions for the enclosed statements.
Equivalent to JTest performance assertions; ensures critical paths stay within
predictable cost bounds.

### 4.6 Test attributes

```tol
#[skip]              -- exclude from run (JUnit @Disabled)
#[tag("name")]       -- group by tag; tol test -tag name
#[cases]             -- parameterized (see §4.7)
#[fuzz]              -- fuzz entry point (see §4.8)
#[timeout(ms)]       -- fail if setup+test+teardown exceeds wall-clock ms
```

### 4.7 Parameterized tests

```tol
#[cases]
fn test_<name>(token: TRC20, col1: type1, col2: type2) { ... }
cases {
  | col1   | col2   |
  | val1a  | val2a  |
  | val1b  | val2b  |
}
```

The runner expands each row into a separate named test:
`TRC20Test/test_transfer_various_amounts[0]`, `[1]`, `[2]`, ...

Equivalent to JUnit 5 `@ParameterizedTest` + `@CsvSource`.

### 4.8 Fuzz tests

```tol
#[fuzz]
fn fuzz_<name>(token: TRC20, param: u256) { ... }
```

- The runner feeds randomly mutated inputs, looking for unexpected panics or
  invariant violations.
- Fuzz functions must not use `assert_revert`; an unexpected revert (one not
  triggered by a business-logic `require`) counts as a failure.
- Run with: `tol test -fuzz fuzz_transfer -fuzztime 30s`
- Deterministic: the fuzz corpus is seeded with a fixed seed; results are
  reproducible given the same seed and corpus.

### 4.9 Mock contracts

```tol
mock TRC20Mock : TRC20 {
  fn transfer(to: address, amount: u256) -> (ok: bool) {
    return true;   -- always succeeds without touching storage
  }
}
```

- `mock` declares a stub contract that implements the same interface as the real
  contract but with controlled, simplified behavior.
- Useful for testing contracts that call external TRC20 tokens without deploying
  the full token implementation.
- Equivalent to Mockito `mock()` + `when().thenReturn()`.

### 4.10 Direct storage inspection (white-box testing)

```tol
let supply: u256 = inspect token.total_supply;
assert_eq supply, 1000u256;
```

`inspect` is available only in test files. It bypasses the function call interface
to read storage directly — useful for verifying invariants without a public getter.

---

## 5. Runner Semantics

### 5.1 Invocation

```sh
tol test ./...                        # run all *_test.tol files recursively
tol test ./trc20_test.tol             # run a specific test file
tol test -run test_transfer           # name pattern filter
tol test -tag slow                    # run only tests tagged "slow"
tol test -skip slow                   # exclude tests tagged "slow"
tol test -v                           # verbose output
tol test -cover                       # enable coverage collection
tol test -covermin 80                 # fail if coverage < 80%
tol test -fuzz fuzz_transfer -fuzztime 30s   # fuzz mode
```

### 5.2 Execution model

For each test function:

1. A fresh Lua VM state is created.
2. `setup_suite` bindings (if any) are passed in from the suite-level deployment.
3. `setup` is called; its bound contracts are deployed into isolated storage.
4. The test function runs.
5. `teardown` runs (pass or fail).
6. The VM state is discarded; no state persists to the next test.
7. After all tests, `teardown_suite` runs.

Tests within a `test` block run sequentially in declaration order.

### 5.3 Output format

```
--- PASS: TRC20Test/test_constructor_sets_supply          (0.001s)
--- PASS: TRC20Test/test_constructor_credits_owner        (0.001s)
--- PASS: TRC20Test/test_transfer_moves_balance            (0.002s)
--- PASS: TRC20Test/test_transfer_various_amounts[0]      (0.001s)
--- PASS: TRC20Test/test_transfer_various_amounts[1]      (0.001s)
--- PASS: TRC20Test/test_transfer_various_amounts[2]      (0.001s)
--- FAIL: TRC20Test/test_balances_consistent
    trc20_test.tol:58: assert_all failed (2 of 4):
      alice: assert_eq failed: got 601, want 600
      bob:   assert_eq failed: got 399, want 400
--- SKIP: TRC20Test/test_wip_feature                      (#[skip])

FAIL    trc20_test.tol  [1 failure, 1 skip]
```

### 5.4 Exit codes

| Code | Meaning |
|------|---------|
| `0` | All tests passed |
| `1` | One or more tests failed |
| `2` | Compilation error in test or contract file |
| `3` | Coverage below `-covermin` threshold |

---

## 6. Coverage

The runner collects coverage automatically when invoked with `-cover`.

### 6.1 Function coverage

Which public functions were called at least once.

```
trc20.tol  function coverage:
  totalSupply()                          called  ✓
  balanceOf(address)                     called  ✓
  transfer(address, u256)                called  ✓
  approve(address, u256)                 called  ✓
  transferFrom(address, address, u256)   called  ✓
  fallback                               NOT called  ✗

Function coverage: 5/6 (83%)
```

### 6.2 Line coverage

Which source lines were executed at least once. Requires the compiler to emit
a source map (`tol compile --sourcemap`) that links each Lua bytecode instruction
back to a `.tol` source line.

```
trc20.tol  line coverage:
  line 28  fn totalSupply()              ✓
  line 29    let s: u256 = total_supply  ✓
  line 30    return s                    ✓
  line 33  fn balanceOf(owner)           ✓
  ...
  line 70  fallback                      ✗   (not reached)

Line coverage: 18/20 (90%)
```

The HTML report annotates the source file with colors:
- **Green** — line executed
- **Yellow** — line partially covered (branch not fully taken)
- **Red** — line never executed

This is the primary visual output of JaCoCo and the most useful for auditors.
Implementation requires source-map support in the compiler (planned for P4).

### 6.3 Branch coverage

Which conditional branches (`if`/`else`, `require` pass/fail) were taken.

```
trc20.tol  branch coverage:
  transfer:31  require pass   ✓
  transfer:31  require fail   ✓
  transferFrom:56  require pass  ✓
  transferFrom:56  require fail  ✓
  transferFrom:61  require pass  ✓
  transferFrom:61  require fail  ✗

Branch coverage: 5/6 (83%)
```

### 6.4 Storage slot coverage

Which storage slots were read and written.

```
trc20.tol  storage slot coverage:
  total_supply     read ✓  written ✓
  balances         read ✓  written ✓
  allowances       read ✓  written ✓
```

### 6.5 Cyclomatic complexity

The runner reports cyclomatic complexity (CC) per function — the number of
linearly independent paths through the function. Functions with CC > 10 are
flagged as candidates for additional test cases.

```
trc20.tol  cyclomatic complexity:
  totalSupply()                        CC=1
  balanceOf(address)                   CC=1
  transfer(address, u256)              CC=3
  approve(address, u256)               CC=1
  transferFrom(address, address, u256) CC=5
```

Equivalent to JaCoCo's complexity metric. High CC means the function has many
possible execution paths and likely needs more parameterized test cases.

### 6.6 Coverage threshold enforcement

```sh
tol test -cover -covermin 80
tol test -cover -covermin line=90,branch=80,function=100
```

Exits with code 3 if any coverage dimension falls below the specified percentage.
Dimensions can be set independently: `line`, `branch`, `function`.
Equivalent to JaCoCo Maven plugin `<haltOnFailure>` with per-counter minimum ratios.

### 6.7 Coverage output formats

```sh
tol test -cover -coverformat=text    # terminal summary table (default)
tol test -cover -coverformat=json    # machine-readable JSON (all dimensions)
tol test -cover -coverformat=xml     # JaCoCo-compatible XML (for SonarQube, Codecov)
tol test -cover -coverformat=html    # annotated source HTML (line-level colors)
tol test -cover -coverformat=lcov    # LCOV format (for genhtml, VS Code extensions)
```

**HTML report** (JaCoCo equivalent):
- Each source line colored: green (covered), yellow (partial branch), red (not covered).
- Drill-down from project → contract → function → source line.
- Requires compiler `--sourcemap` output to map bytecode instructions to source lines.

**XML report** (JaCoCo-compatible schema):
- Consumed by SonarQube, Codecov, GitHub Actions coverage annotations, and CI dashboards.
- Example:
```xml
<report name="trc20.tol">
  <counter type="LINE"     missed="2"  covered="18"/>
  <counter type="BRANCH"   missed="1"  covered="5"/>
  <counter type="METHOD"   missed="1"  covered="5"/>
  <counter type="COMPLEXITY" missed="0" covered="11"/>
</report>
```

### 6.8 Project-level summary

After running all test files, the runner prints a rollup across all contracts:

```
Coverage summary:
  File                   Lines    Branches  Functions  Complexity
  trc20.tol              90%      83%       83%        CC avg=2.2
  trc721.tol             75%      70%       80%        CC avg=3.1
  ─────────────────────────────────────────────────────────────
  TOTAL                  83%      77%       81%

FAIL: branch coverage 77% is below -covermin 80%
```

Equivalent to JaCoCo's project-level aggregated report.

---

## 7. Design Constraints

1. **No production impact** — `*_test.tol` files are never compiled into deployment
   artifacts. All test-only constructs (`inspect`, `deploy`, `assert_*`, `with`,
   `mock`, `#[fuzz]`, `#[cases]`) are rejected by the production compiler.

2. **Storage isolation** — each `test_*` function runs against a fresh VM state.
   No global mutable state is shared between test functions.

3. **Determinism** — tests are deterministic by construction. No wall-clock time,
   OS randomness, or non-deterministic host access. Fuzz seeds are explicit and
   reproducible.

4. **Revert isolation** — a revert inside `assert_revert { }` does not propagate
   to the test function. State is snapshotted and restored automatically.

5. **Fail fast vs. report all** — sequential assertions fail immediately on the
   first failure. `assert_all { }` collects all failures and reports them together.

---

## 8. Comparison with JTest, JaCoCo, and Other Frameworks

| Feature | JUnit 5 | JTest (Parasoft) | JaCoCo | Hardhat/Foundry | TOL |
|---------|---------|-----------------|--------|-----------------|-----|
| Built into language | No (library) | No (tool) | No (tool) | No (framework) | **Yes** |
| File isolation | same file | same file | N/A | separate `.ts` | `_test.tol` |
| Per-test setup | `@Before` | `@Before` | N/A | `beforeEach` | `setup` |
| Suite-level setup | `@BeforeClass` | `@BeforeClass` | N/A | `before` | `setup_suite` |
| Skip test | `@Disabled` | `@Disabled` | N/A | `.skip` | `#[skip]` |
| Test tags | `@Tag` | `@Category` | N/A | custom | `#[tag]` |
| Parameterized | `@ParameterizedTest` | `@DataProvider` | N/A | manual loop | `#[cases]` |
| Fuzz testing | No | Yes (auto-gen) | N/A | Foundry `-fuzz` | `#[fuzz]` |
| Custom assert message | Yes | Yes | N/A | Yes | Yes |
| Grouped assertions | `assertAll()` | No | N/A | No | `assert_all {}` |
| Exception/revert | `assertThrows` | `assertThrows` | N/A | `.to.be.reverted` | `assert_revert {}` |
| State unchanged on revert | No (manual) | No | N/A | No (manual) | **Automatic** |
| Event assertion | No | No | N/A | `.to.emit` | `assert_event` |
| Mock objects | Mockito | Built-in | N/A | Manual stubs | `mock` contract |
| Storage inspection | No | No | N/A | `getStorage()` | `inspect` |
| Contract deploy | N/A | N/A | N/A | `deployContract` | `deploy` statement |
| Call context override | N/A | N/A | N/A | `connect(signer)` | `with msg.sender` |
| Instruction limit | `@Timeout` (wall) | Perf assert | N/A | No | `assert_instructions_le` |
| Line coverage | No | Yes | **Yes** | No | Yes (needs sourcemap) |
| Branch coverage | No | Yes | **Yes** | No | Yes |
| Function coverage | No | Yes | Yes | No | Yes |
| Storage slot coverage | N/A | N/A | N/A | N/A | **Yes (unique)** |
| Cyclomatic complexity | No | Yes | Yes | No | Yes |
| Coverage threshold | No | Yes | Yes | No | `-covermin` |
| Coverage HTML (line colors) | No | Yes | **Yes** | No | Yes |
| Coverage XML (SonarQube) | No | Yes | **Yes** | No | Yes |
| Coverage LCOV | No | No | No | No | Yes |
| Project-level rollup | No | Yes | **Yes** | No | Yes |
| Auto test generation | No | **Yes** | N/A | No | Not yet (P5) |
| State isolation | Manual | Manual | N/A | Manual `beforeEach` | **Automatic** |
| Determinism guarantee | No | No | N/A | No | **Yes** |

### What JTest has that TOL does not (yet)

1. **Automatic test generation** — JTest analyzes code paths and generates test
   cases automatically. This is JTest's most powerful enterprise feature. TOL plans
   this as a future milestone (P5: AI-assisted test generation from contract source).

2. **Static analysis integration** — JTest runs static analysis (null pointer,
   resource leak, OWASP checks) alongside tests. TOL's equivalent is the
   compiler verifier (§15 of TOL_SPEC.md), which runs before codegen.

3. **Compliance rule sets** — JTest supports MISRA, CERT, OWASP profiles. TOL
   could add a compliance profile for common DeFi vulnerability patterns (see
   TOL_AUDIT.md), but this is not in the current design.

### What TOL has that JTest does not

1. **Storage slot coverage** — tracking which storage slots were read/written is
   unique to TOL and has no JTest equivalent. It directly answers "did any test
   exercise the allowances mapping?" at the storage layer.

2. **Automatic revert state verification** — `assert_revert {}` automatically
   snapshots and verifies storage unchanged. JTest has no concept of transactional
   state rollback in tests.

3. **Call context injection** — `with msg.sender = X { }` is native syntax.
   Hardhat requires `contract.connect(signer)` which is external framework glue.

4. **Determinism by construction** — TOL tests are deterministic because the
   runtime is deterministic. JTest tests can depend on system time, random seeds,
   OS scheduling, etc.

---

## 9. Implementation Roadmap

| Phase | Deliverable |
|-------|-------------|
| P0 | `*_test.tol` file recognition; `test` block parsing; `deploy`/`with`/`assert_eq`/`assert_revert` |
| P1 | `setup`/`teardown` lifecycle; storage isolation per test; basic pass/fail reporting |
| P2 | `setup_suite`/`teardown_suite`; `assert_event`; `inspect`; `#[skip]`; `#[tag]`; custom messages |
| P3 | `assert_all`; `assert_gt/lt/between`; `assert_instructions_le`; `#[cases]` parameterized tests |
| P4 | Coverage: function + branch + storage slot + cyclomatic complexity + **line** (requires sourcemap); HTML line-annotated output; XML (JaCoCo-compatible); LCOV; project rollup; `-covermin` per-dimension |
| P5 | `#[fuzz]` fuzz entry points; `mock` contracts; differential testing vs. reference Solidity |
| P6 | AI-assisted test generation from contract source (equivalent to JTest auto-generation) |
