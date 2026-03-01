# Tolang Implementation Roadmap

Status: Active tracking (updated 2026-03-01)
Date: 2026-03-01
Source Spec: `45-IR/TOL_SPEC.md` (Draft v0.2)
Primary Target: Implement a production canonical pipeline in `tolang`:
`TOL source -> typed/lowered TOL -> direct IR -> bytecode`, fully covering
`conditional-tokens-market-makers/contracts/*.sol` semantics.

---

## 0. Current Task Status (2026-03-01)

Milestone progress snapshot:

1. `M0` - Completed (architecture lock + package boundaries + public API skeleton).
2. `M1` - In progress (parser/lexer subset implemented; full grammar coverage pending).
3. `M2` - In progress (early semantic checks implemented; full type system pending).
4. `M3` - Not started.
5. `M4` - In progress (selector and dispatch subset implemented; typed ABI ops pending).
6. `M5` - Not started in direct-IR path.
7. `M6` - Partially complete (existing math intrinsics in runtime; full spec set pending).
8. `M7` - In progress (direct IR bootstrap pipeline works for restricted subset).
9. `M8` - Not started.
10. `M9` - Not started.
11. `M10` - Not started.

---

## 1. Objective

Build a deterministic, testable, and maintainable implementation of TOL v0.2 in `tolang`, from text parsing to bytecode generation and runtime execution hooks.

Success means:

1. `TOL source -> typed/lowered TOL -> direct IR -> bytecode` is stable and reproducible.
2. TOL features required by CTMM contracts are implemented end to end.
3. Determinism constraints are enforced (no host nondeterminism leaks).
4. A conformance test suite proves compatibility for the targeted Solidity/OZ subset.

---

## 2. Scope

In scope:

1. TOL parser, AST, resolver, verifier, lowering, codegen, tooling.
2. Runtime host interfaces required by TOL v0.2.
3. Deterministic integer `log/exp` intrinsics (user-defined scaling) needed by LMSR.
4. ABI features and dispatch semantics required by CTMM.
5. Test harnesses: unit, integration, differential, and determinism replay.

Out of scope:

1. Full Solidity compiler compatibility.
2. GTOS economic policy design (gas pricing policy, fee economics).  
   TOL only defines execution semantics and hooks.
3. Non-deterministic host features.

---

## 3. Workstream Layout

Parallel workstreams:

1. Frontend (TOL text): lexer, parser, AST, formatter.
2. Semantics: symbol resolution, typing, verifier.
3. Midend: TOL normalization and lowering to backend IR.
4. Backend: bytecode generation and artifact emission.
5. Runtime: host builtins and deterministic boundaries.
6. Quality: conformance testing, fuzzing, replay determinism.

---

## 4. Milestones

## M0 - Baseline and Architecture Lock

Goals:

1. Freeze `TOL_SPEC.md` v0.2 interpretation for implementation.
2. Define package boundaries and APIs in `tolang`.

Deliverables:

1. Architecture note with package map:
   - `tol/lexer`
   - `tol/parser`
   - `tol/ast`
   - `tol/sema`
   - `tol/lower`
   - `tol/codegen`
2. Error model convention (error codes + source spans).
3. Feature flags for staged rollout.

Exit criteria:

1. Team sign-off on module boundaries.
2. No unresolved ambiguity for v0.2 syntax/semantics blocking coding.
3. Canonical compile route
   `TOL source -> typed/lowered TOL -> direct IR -> bytecode` is explicitly locked.

---

## M1 - Lexer and Parser

Goals:

1. Parse full v0.2 grammar into AST with exact source locations.

Deliverables:

1. Lexer for all tokens including:
   - signed/unsigned integer type keywords
   - `interface`, `library`, `modifier`, `enum`, `external`, `private`, `pure`
2. Parser for:
   - top declarations (`interface`, `library`, `contract`)
   - function signatures and modifier use
   - array, mapping, and datalocation types
3. Parse diagnostics with line/column and expected token sets.

Exit criteria:

1. Parser golden tests for valid/invalid programs pass.
2. EBNF coverage report shows all productions exercised.

---

## M2 - Type System and Core Semantics

Goals:

1. Implement primitive/composite types and conversion rules from v0.2.

Deliverables:

1. Type checker support:
   - `u8..u256`, `i8..i256`, `bytes4`, `bytes32`, `bytes`, `string`, `address`, `bool`
   - `mapping(K => V)`, `T[]`, `T[N]`, enums
2. Cast rules:
   - no implicit narrowing
   - explicit signed/unsigned crossing
3. Arithmetic rule checks:
   - checked/wrapping mode
   - signed division/mod semantics
4. Data location checks:
   - `storage`, `memory`, `calldata`
   - `new T[](n)` in memory only

Exit criteria:

1. Typed AST generated for all core samples.
2. Negative tests catch illegal casts, illegal indexing, location misuse.

---

## M3 - Advanced Semantics (Inheritance, Modifiers, Interfaces)

Goals:

1. Implement Solidity-like contract composition semantics defined in v0.2.

Deliverables:

1. Inheritance graph and C3 linearization.
2. Override compatibility checks.
3. Modifier definition and application lowering (`_` expansion).
4. Interface conformance validation.
5. `super.fn(...)` resolution.

Exit criteria:

1. Linearization tests (single and multiple inheritance) pass.
2. Modifier expansion preserves control flow and return behavior.
3. Interface mismatch produces deterministic compile errors.

---

## M4 - ABI and Dispatch

Goals:

1. Deliver full v0.2 ABI surface required by CTMM and OZ subset.

Deliverables:

1. Selector generation:
   - default selector
   - override selector
   - `selector("sig")`, `Contract.fn.selector`, `this.fn.selector`
2. ABI encoding/decoding:
   - `abi.encode`
   - `abi.decode`
   - `abi.encodePacked`
   - `abi.encodeWithSelector`
   - `abi.encodeWithSignature` (compile-time literal enforcement)
3. Tuple decode/destructure lowering.
4. Visibility and mutability checks (`public/external/internal/private`, `view/pure/payable`).

Exit criteria:

1. ABI compatibility tests against canonical vectors pass.
2. Dispatch table has unique selectors and deterministic ordering.

---

## M5 - Storage and Runtime Host Builtins

Goals:

1. Implement storage model and runtime host API bindings with deterministic behavior.

Deliverables:

1. Storage addressing:
   - named slots
   - mapping hash derivation
   - signed key encoding
2. Dynamic storage arrays:
   - `.length`
   - `arr[i]`
   - `arr.push(v)`
3. Host builtins:
   - `call`, `staticcall`, `delegatecall`
   - `create`, `create2`
   - `keccak256`, `sha256`, `ripemd160`, `ecrecover`
4. Deterministic failure handling:
   - zero-address return for failed create/create2
   - no inline assembly path required in TOL

Exit criteria:

1. Storage layout tests match reference slot expectations.
2. Host call behavior is deterministic under replay.

---

## M6 - Deterministic Integer `log/exp` Intrinsics

Goals:

1. Implement LMSR-required `log/exp` intrinsics with pure integer arithmetic.

Deliverables:

1. Intrinsics:
   - `math.binaryLog(x_scaled, scale, mode)`
   - `math.pow2(x_scaled, scale, mode)`
   - `math.max(i256[])`
2. Caller-defined scaling semantics:
   - no VM-global fixed-point scale
   - explicit `scale` parameter validation (`scale > 0`)
3. `EstimationMode` handling:
   - `LowerBound`
   - `Midpoint`
   - `UpperBound`
4. Edge handling:
   - domain checks
   - overflow checks
   - rounding determinism
   - scale semantics determinism

Exit criteria:

1. Cross-platform determinism tests pass bit-for-bit.
2. LMSR reference scenarios produce expected integer outputs.

---

## M7 - Lowering and Bytecode Emission

Goals:

1. Lower typed/lowered TOL directly to IR and emit executable bytecode artifacts.

Deliverables:

1. Lowering passes:
   - expression normalization
   - control-flow lowering
   - modifier-expanded function body lowering
2. Runtime call op selection for host builtins.
3. `.glbc` artifact emission with metadata fields from spec.

Exit criteria:

1. End-to-end compile pipeline works on representative contracts.
2. Bytecode loader executes compiled artifacts successfully.

---

## M8 - Tooling and Developer UX

Goals:

1. Make the pipeline usable and auditable for contract engineers and agents.

Deliverables:

1. CLI commands:
   - `tolang -cir in.tol -o out.glbc`
   - optional `--emit-typed-ast`, `--emit-ir`, `--abi-report`
2. TOL formatter and linter.
3. Source map and debug metadata export.

Exit criteria:

1. One-command compile path documented and reproducible.
2. Diagnostics are actionable for invalid TOL.

---

## M9 - Conformance and Hardening

Goals:

1. Prove correctness and determinism to production standards.

Deliverables:

1. CTMM conformance suite:
   - ERC20 behavior subset
   - factory and deployment flow coverage
   - event and ABI checks
2. Differential tests:
   - `TOL source -> typed/lowered TOL -> direct IR -> bytecode`
   - direct-lowering invariants and bytecode equivalence checks across lowering passes
3. Fuzzers:
   - lexer/parser fuzz
   - type checker fuzz
4. Determinism replay tests across machines/architectures.

Exit criteria:

1. CTMM coverage matrix in spec passes with no unsupported fallback.
2. Determinism replay has zero divergence.

---

## M10 - GTOS Integration Readiness

Goals:

1. Integrate compiled TOL bytecode execution path into GTOS contract runtime.

Deliverables:

1. Contract deployment and invocation flow for `.glbc`.
2. Host bridge compatibility checks with GTOS runtime.
3. Migration guide from source upload to bytecode upload.

Exit criteria:

1. GTOS can deploy and execute TOL-generated bytecode contracts end to end.
2. Existing regression suites pass with bytecode path enabled.

---

## 5. Acceptance Gates

Gate A (Language Complete):

1. M1-M4 completed.
2. All v0.2 grammar/type/ABI mandatory features implemented.

Gate B (Runtime Complete):

1. M5-M7 completed.
2. Host builtins and storage semantics stable.

Gate C (CTMM Complete):

1. M6 + M9 completed.
2. `contracts/*.sol` semantic coverage confirmed by tests.

Gate D (Integration Complete):

1. M10 completed.
2. GTOS deployment pipeline supports TOL bytecode in production mode.

---

## 6. Risks and Mitigations

Risk 1: Spec ambiguity around advanced ABI edge cases.

1. Mitigation: maintain an executable ABI vector suite and lock behavior by tests.

Risk 2: Inheritance + modifier lowering bugs causing subtle control-flow regressions.

1. Mitigation: snapshot transformed AST/IR and add targeted property tests.

Risk 3: Fixed-point math divergence across platforms.

1. Mitigation: pure integer implementation, no FPU paths, cross-arch CI replay.

Risk 4: Host runtime mismatch between `tolang` and GTOS.

1. Mitigation: define a strict host capability contract and shared integration tests.

---

## 7. Immediate Next Actions

1. Finish direct-IR backend for storage slots/mappings/arrays (remove current `TOL3002` path for storage).
2. Implement typed ABI decode/encode subset needed for external/public entry wrappers.
3. Expand verifier for control-flow and function-level checks beyond current syntax/shape validation.
4. Build CTMM conformance manifest and map each missing feature to concrete tests.
