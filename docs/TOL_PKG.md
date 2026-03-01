# TOL Package System

Status: Design Draft v0.1 (2026-03-01)
Owner: GTOS/Tolang engineering
Scope: File formats, package layout, standard library, and on-chain registry

---

## Implementation Snapshot (2026-03-01)

### Go API (landed)

- `CompileTOLToTOI(source, name)` / `CompileTOLToTOIWithOptions(...)` — emit `.toi`
- `ValidateTOIText(toiBytes)` — lightweight `.toi` structural validator
- `CompileTOLToTOC(source, name)` — emit `.toc`
- `CompileTOLToTOR(source, name, opts)` — one-shot `.tol` → minimal `.tor`
  (`opts` supports package/version and `.toi` interface-name overrides)
- `EncodeTOC(...)` / `DecodeTOC(...)` / `VerifyTOCSourceHash(toc, sourceBytes)`
- `EncodeTOR(manifest, files)` / `DecodeTOR(torBytes)` / `TORPackageHash(torBytes)`
  (`contracts[*].name` and at least one of `toc`/`toi` are required;
  contract names must be unique;
  `DecodeTOR` validates embedded `.toc`/`.toi` entries)

### CLI (landed subcommands; legacy flags still supported)

The `tol` CLI now supports subcommands in the style specified in §7:

```sh
tol compile trc20.tol
tol compile --emit toi -o ITRC20.toi trc20.tol
tol compile --emit tor -o trc20.tor trc20.tol
tol pack -o trc20-base-1.0.0.tor ./contracts/
tol inspect trc20.toc
tol inspect --json trc20-base-1.0.0.tor
tol verify trc20.toc
tol verify --source trc20.tol trc20.toc
```

Legacy flat flags (`tolang -ctoc/-dtocj/-vtor/...`) are kept for compatibility.

### Not landed yet

- `.toi` import / type-check flow in the compiler
- Registry resolution (`tor://...`, `toc://...`) and `tol install` / `tol publish`

---

## 1. File Extensions

| Extension | Full name | Description |
|-----------|-----------|-------------|
| `.tol` | TOL source | Human-written or agent-generated contract source |
| `.toc` | TOL compiled | Compiled bytecode for a single contract unit |
| `.toi` | TOL interface | Interface-only declaration; no bytecode, no implementation |
| `.tor` | TOL runtime package | Archive bundling one or more `.toc` + `.toi` + metadata |

### Analogy

| Java | TOL |
|------|-----|
| `.java` | `.tol` |
| `.class` | `.toc` |
| `.java` (interface only) | `.toi` |
| `.jar` | `.tor` |
| Maven Central | TOS on-chain registry |

---

## 2. `.toi` — Interface File

A `.toi` file declares the public surface of a contract without any implementation.
Callers import `.toi` files to call a deployed contract; they do not need the `.toc`
bytecode.

Current generated form:

```
tol compile --emit toi -o ITRC20.toi trc20.tol
```

```tol
-- ITRC20.toi
tol 0.2

interface ITRC20 {
  fn totalSupply() -> (supply: u256) public view;
  fn balanceOf(owner: address) -> (balance: u256) public view;
  fn transfer(to: address, amount: u256) -> (ok: bool) public;
  fn approve(spender: address, amount: u256) -> (ok: bool) public;
  fn transferFrom(from: address, to: address, amount: u256) -> (ok: bool) public;

  event Transfer(from: address indexed, to: address indexed, value: u256);
  event Approval(owner: address indexed, spender: address indexed, value: u256);
}
```

- `.toi` files contain only `interface` declarations.
- They carry no storage, no constructor, no function bodies.
- The compiler uses them to verify call-site type correctness.
- Importing a `.toi` does not link any bytecode into the caller's artifact.

---

## 3. `.toc` — Compiled Bytecode

A `.toc` file is the output of compiling a single `.tol` source file:

```
tol compile -o trc20.toc trc20.tol
```

Fields embedded in a `.toc`:

| Field | Content |
|-------|---------|
| `magic` | `TOC\x00` (4 bytes) |
| `version` | TOL compiler version |
| `contract_name` | string |
| `bytecode` | Lua bytecode blob |
| `abi` | JSON: function selectors, event signatures |
| `storage_layout` | JSON: slot names, types, canonical hashes |
| `source_hash` | keccak256 of the source `.tol` file |
| `bytecode_hash` | keccak256 of the bytecode blob |

The `bytecode_hash` is the canonical identity of the compiled contract and is used
for content-addressed registry lookups.

---

## 4. `.tor` — Runtime Package Archive

A `.tor` file is a ZIP-format archive containing one or more contracts,
their interfaces, source files (for verification), and metadata.

### 4.1 Archive layout

```
trc20-base-1.0.0.tor
├── manifest.json
├── bytecode/
│   ├── TRC20.toc
│   ├── TRC20Burnable.toc
│   ├── TRC20Mintable.toc
│   └── TRC20Pausable.toc
├── interfaces/
│   ├── ITRC20.toi
│   └── ITRC20Burnable.toi
├── sources/
│   ├── trc20.tol
│   ├── trc20_burnable.tol
│   └── trc20_mintable.tol
└── tests/
    └── trc20_test.tol
```

### 4.2 manifest.json

```json
{
  "name": "trc20-base",
  "version": "1.0.0",
  "tol_version": "0.2",
  "compiler": "tolang/1.0.0",
  "license": "MIT",
  "description": "Standard TRC20 fungible token implementation",
  "contracts": [
    { "name": "TRC20",         "toc": "bytecode/TRC20.toc",         "toi": "interfaces/ITRC20.toi" },
    { "name": "TRC20Burnable", "toc": "bytecode/TRC20Burnable.toc", "toi": "interfaces/ITRC20Burnable.toi" }
  ],
  "dependencies": [],
  "package_hash": "0x<keccak256-of-archive-contents>"
}
```

### 4.3 Importing from a `.tor`

```tol
-- By local path
import ITRC20 from "trc20-base.tor::ITRC20"

-- By registry name and version
import ITRC20 from "tor://trc20-base@1.0.0::ITRC20"

-- By content hash (immutable, audit-safe)
import ITRC20 from "toc://0x1a2b3c...::ITRC20"
```

Content-hash imports are the only form guaranteed to be immutable.
Name-based imports resolve through the registry (§6) and should be pinned to a hash
in production deployments.

---

## 5. Standard Library (tol-stdlib)

The official TOL standard library covers the full surface of OpenZeppelin Contracts,
adapted for the TOL type system and GTOS runtime. Modules not applicable to TOL
(proxy patterns, SafeMath, Address utilities) are omitted by design.

### 5.1 Token standards

#### `trc20-base.tor`
Full OpenZeppelin ERC20 equivalent for GTOS.

| Contract | OZ equivalent | Description |
|----------|--------------|-------------|
| `TRC20` | `ERC20` | Base fungible token |
| `TRC20Burnable` | `ERC20Burnable` | Token holders can burn |
| `TRC20Mintable` | (custom) | Owner-controlled mint |
| `TRC20Capped` | `ERC20Capped` | Supply cap enforcement |
| `TRC20Pausable` | `ERC20Pausable` | Emergency pause |
| `TRC20Permit` | `ERC20Permit` | EIP-2612 gasless approve |
| `TRC20Votes` | `ERC20Votes` | Voting weight snapshots |
| `TRC20Wrapper` | `ERC20Wrapper` | Wrap another TRC20 |

Interfaces: `ITRC20.toi`, `ITRC20Burnable.toi`, `ITRC20Permit.toi`, `ITRC20Votes.toi`

#### `trc721-base.tor`
Full OpenZeppelin ERC721 equivalent.

| Contract | OZ equivalent | Description |
|----------|--------------|-------------|
| `TRC721` | `ERC721` | Base non-fungible token |
| `TRC721Burnable` | `ERC721Burnable` | Holder can burn |
| `TRC721Enumerable` | `ERC721Enumerable` | On-chain enumeration |
| `TRC721URIStorage` | `ERC721URIStorage` | Per-token URI metadata |
| `TRC721Pausable` | `ERC721Pausable` | Emergency pause |
| `TRC721Royalty` | `ERC2981` | Royalty standard |

Interfaces: `ITRC721.toi`, `ITRC721Enumerable.toi`, `ITRC721Receiver.toi`

#### `trc1155-base.tor`
Full OpenZeppelin ERC1155 equivalent.

| Contract | OZ equivalent | Description |
|----------|--------------|-------------|
| `TRC1155` | `ERC1155` | Base multi-token |
| `TRC1155Burnable` | `ERC1155Burnable` | Holder can burn |
| `TRC1155Supply` | `ERC1155Supply` | Track total supply per id |
| `TRC1155Pausable` | `ERC1155Pausable` | Emergency pause |

Interfaces: `ITRC1155.toi`, `ITRC1155Receiver.toi`

#### `trc4626-base.tor`
Tokenized vault standard.

| Contract | OZ equivalent | Description |
|----------|--------------|-------------|
| `TRC4626` | `ERC4626` | Yield-bearing vault |

Interface: `ITRC4626.toi`

---

### 5.2 Access control (`tol-access.tor`)

| Contract | OZ equivalent | Description |
|----------|--------------|-------------|
| `Ownable` | `Ownable` | Single owner with transfer |
| `Ownable2Step` | `Ownable2Step` | Two-step ownership transfer |
| `AccessControl` | `AccessControl` | Role-based permissions |
| `AccessControlEnumerable` | `AccessControlEnumerable` | Enumerable role members |

---

### 5.3 Security (`tol-security.tor`)

| Contract | OZ equivalent | Description |
|----------|--------------|-------------|
| `Pausable` | `Pausable` | Emergency stop mechanism |

> **Note:** `ReentrancyGuard` is intentionally omitted. Reentrancy is architecturally
> impossible in TOL (no external callback mechanism). Developers do not need to
> remember to inherit it. See [TOL_AUDIT.md](TOL_AUDIT.md) §2.1.

---

### 5.4 Mathematics (`tol-math.tor`)

| Module | OZ equivalent | Description |
|--------|--------------|-------------|
| `Math` | `Math` | `min`, `max`, `avg`, `sqrt`, `log2`, `log10`, `log256` |
| `SignedMath` | `SignedMath` | Signed `min`, `max`, `abs` |
| `SafeCast` | `SafeCast` | Checked u256↔smaller integer casts |
| `FixedPoint` | (ABDKMathQuad replacement) | Fixed-point arithmetic (integer-only, user-defined scale) |
| `LMSRMath` | (custom) | `binaryLog`, `pow2`, `EstimationMode` for LMSR market makers |

> **Note:** `SafeMath` is intentionally omitted. TOL arithmetic is checked by default;
> overflow causes an immediate revert without any library call.

---

### 5.5 Collections (`tol-collections.tor`)

| Module | OZ equivalent | Description |
|--------|--------------|-------------|
| `EnumerableSet` | `EnumerableSet` | AddressSet, UintSet, Bytes32Set |
| `EnumerableMap` | `EnumerableMap` | AddressToUintMap, UintToAddressMap, etc. |
| `Arrays` | `Arrays` | `sort`, `findUpperBound`, `unsafeAccess` |
| `Counters` | `Counters` | Monotonic u256 counter |
| `BitMap` | `BitMaps` | Packed boolean array |

---

### 5.6 Cryptography (`tol-crypto.tor`)

| Module | OZ equivalent | Description |
|--------|--------------|-------------|
| `ECDSA` | `ECDSA` | Signature recovery and verification |
| `MerkleProof` | `MerkleProof` | Merkle tree inclusion proofs |
| `MessageHashUtils` | `MessageHashUtils` | Ethereum-signed message hashing |
| `EIP712` | `EIP712` | Typed structured data hashing |

---

### 5.7 Finance (`tol-finance.tor`)

| Contract | OZ equivalent | Description |
|----------|--------------|-------------|
| `VestingWallet` | `VestingWallet` | Linear token vesting |
| `PaymentSplitter` | `PaymentSplitter` | Share-based payment distribution |

---

### 5.8 Governance (`tol-governance.tor`)

| Contract | OZ equivalent | Description |
|----------|--------------|-------------|
| `Governor` | `Governor` | On-chain proposal and voting |
| `GovernorSettings` | `GovernorSettings` | Configurable governance params |
| `GovernorVotes` | `GovernorVotes` | TRC20Votes-backed governance |
| `TimelockController` | `TimelockController` | Timelock for governance actions |

---

### 5.9 Library coverage summary

| OZ module | TOL status | Notes |
|-----------|-----------|-------|
| ERC20 + extensions | `trc20-base.tor` | Full coverage |
| ERC721 + extensions | `trc721-base.tor` | Full coverage |
| ERC1155 + extensions | `trc1155-base.tor` | Full coverage |
| ERC4626 | `trc4626-base.tor` | Full coverage |
| Ownable / AccessControl | `tol-access.tor` | Full coverage |
| Pausable | `tol-security.tor` | Full coverage |
| ReentrancyGuard | **Omitted** | Impossible in TOL by design |
| SafeMath | **Omitted** | Built into arithmetic by design |
| Proxy / Upgradeable | **Omitted** | No delegatecall in TOL by design |
| Address utilities | **Omitted** | Different address model |
| Math / SignedMath / SafeCast | `tol-math.tor` | Full coverage |
| FixedPoint / ABDKMathQuad | `tol-math.tor::FixedPoint` | Integer-only equivalent |
| EnumerableSet/Map | `tol-collections.tor` | Full coverage |
| ECDSA / MerkleProof / EIP712 | `tol-crypto.tor` | Full coverage |
| VestingWallet / PaymentSplitter | `tol-finance.tor` | Full coverage |
| Governor / Timelock | `tol-governance.tor` | Full coverage |

---

## 6. On-chain Package Registry (M-registry)

### 6.1 Design principle

The registry maps human-readable names to immutable content hashes.
Once a `name@version` is registered, it **cannot be overwritten** — the binding
is permanent and content-addressed. This prevents dependency poisoning attacks
(Maven Central had multiple such incidents).

### 6.2 Registry contract

The registry is itself a TOL contract deployed on GTOS:

```tol
tol 0.2

contract TolRegistry {
  storage {
    slot records: mapping(bytes32 => bytes32);
    -- key:   keccak256(name ++ "@" ++ version)
    -- value: package_hash (keccak256 of .tor archive contents)
  }

  event Registered(name: bytes32 indexed, version: bytes32, package_hash: bytes32);

  fn register(name: bytes32, version: bytes32, package_hash: bytes32) public {
    let key: bytes32 = keccak256(name, version);
    require(records[key] == 0x00, "ALREADY_REGISTERED");
    set records[key] = package_hash;
    emit Registered(name, version, package_hash);
  }

  fn resolve(name: bytes32, version: bytes32) -> (hash: bytes32) public view {
    let key: bytes32 = keccak256(name, version);
    let h: bytes32 = records[key];
    require(h != 0x00, "NOT_FOUND");
    return h;
  }
}
```

### 6.3 Resolution flow

```
import ITRC20 from "tor://trc20-base@1.0.0::ITRC20"
        ↓
registry.resolve("trc20-base", "1.0.0") → 0xabc...
        ↓
fetch .tor archive by hash from GTOS code storage
        ↓
extract interfaces/ITRC20.toi
        ↓
compile-time type check only (no bytecode linked)
```

### 6.4 Import forms and security levels

| Import form | Example | Mutable? | Audit-safe? |
|-------------|---------|----------|-------------|
| Local path | `"./trc20.tol"` | Yes | Dev only |
| Registry name | `"tor://trc20-base@1.0.0"` | No (version locked) | Yes |
| Content hash | `"toc://0xabc123..."` | No (immutable) | **Strongest** |

Production contracts should always use content-hash imports. Registry-name imports
are acceptable for development and CI.

### 6.5 `tor.lock` — lockfile

Similar to `go.sum`, `package-lock.json`, or `Cargo.lock`:

```json
{
  "imports": [
    {
      "name": "trc20-base",
      "version": "1.0.0",
      "registry": "toc://registry.tos.network",
      "package_hash": "0x1a2b3c...",
      "resolved_at": "2026-03-01"
    }
  ]
}
```

The lockfile pins every transitive dependency to a content hash.
Builds are reproducible: given the same `tor.lock`, the exact same bytecode
is produced on any machine.

---

## 7. CLI Commands

The `tol` binary uses **subcommands**, following the same conventions as
`javac`, `solc`, `go`, and `cargo`. Each subcommand has its own flags;
global flags (`--version`, `--help`) appear before the subcommand.

```
tol <subcommand> [flags] <inputs...>
```

### 7.1 `tol compile` — compile TOL source

```sh
tol compile [--emit <format>] [-o <output>] [options] <input.tol>
```

| Flag | Default | Description |
|------|---------|-------------|
| `--emit toc` | default | Compiled bytecode artifact (`.toc`) |
| `--emit toi` | | Interface-only declaration (`.toi`) |
| `--emit tor` | | One-shot package archive (`.tor`) |
| `-o, --output <file>` | `<input>.<emit>` | Output file path |
| `--name <name>` | contract name from source | Override contract/interface name |
| `--package-name <n>` | filename stem | Package name (for `--emit tor`) |
| `--package-version <v>` | `"0.0.0"` | Package version (for `--emit tor`) |
| `--include-source` | false | Embed source file in `.tor` archive |
| `--abi` | false | Also write ABI JSON alongside `.toc` |
| `--ast` | false | Dump parsed TOL AST to stdout |

**Examples (mirroring javac/solc conventions):**

```sh
# Compile to .toc (equivalent to: javac -d . MyClass.java)
tol compile trc20.tol
tol compile -o out/trc20.toc trc20.tol

# Emit interface file (equivalent to: solc --ir contract.sol)
tol compile --emit toi -o ITRC20.toi trc20.tol
tol compile --emit toi --name ITRC20 -o ITRC20.toi trc20.tol

# One-shot .tol → .tor package (equivalent to: jar cf out.jar *.class)
tol compile --emit tor -o trc20.tor trc20.tol
tol compile --emit tor \
    --package-name trc20-base \
    --package-version 1.0.0 \
    --include-source \
    -o trc20-base-1.0.0.tor trc20.tol

# Also emit ABI JSON alongside bytecode (like: solc --abi --bin contract.sol)
tol compile --abi -o trc20.toc trc20.tol
# → writes trc20.toc and trc20.abi.json

# Dump AST (like: solc --ast-compact-json contract.sol)
tol compile --ast trc20.tol
```

### 7.2 `tol pack` — build `.tor` from a directory

```sh
tol pack -o <output.tor> <directory>
```

Reads `manifest.json` from the directory root and packages all files.

```sh
tol pack -o trc20-base-1.0.0.tor ./contracts/
```

### 7.3 `tol inspect` — print artifact metadata

```sh
tol inspect [--json] <artifact>
```

| Flag | Description |
|------|-------------|
| `--json` | Output as JSON (default: human-readable text) |

```sh
# Inspect .toc (like: javap -v MyClass.class)
tol inspect trc20.toc
tol inspect --json trc20.toc

# Inspect .tor
tol inspect trc20-base-1.0.0.tor
tol inspect --json trc20-base-1.0.0.tor
```

**Text output example:**
```
Contract:      TRC20
Compiler:      tolang/1.0.0
TOL version:   0.2
Bytecode:      1842 bytes
Source hash:   0x1a2b...
Bytecode hash: 0x3c4d...
ABI functions: totalSupply, balanceOf, transfer, approve, transferFrom
Storage slots: total_supply (u256), balances (mapping), allowances (mapping)
```

### 7.4 `tol verify` — validate artifact integrity

```sh
tol verify [--source <file>] <artifact>
```

| Flag | Description |
|------|-------------|
| `--source <file>` | Verify `.toc` `source_hash` matches this `.tol` file |

```sh
# Verify .toc internal consistency (like: jarsigner -verify)
tol verify trc20.toc

# Verify .toc against its original source
tol verify --source trc20.tol trc20.toc

# Verify .tor archive integrity
tol verify trc20-base-1.0.0.tor
```

Exit codes: `0` = ok, `1` = invalid artifact, `2` = source hash mismatch.

### 7.5 `tol install` — install from registry

```sh
tol install <name>@<version>
tol install toc://<hash>
```

Resolves the package hash via the on-chain registry, downloads the `.tor`,
and writes `tor.lock`.

```sh
tol install trc20-base@1.0.0
tol install toc://0xabc123...   # direct content-hash install
```

### 7.6 `tol publish` — publish to registry

```sh
tol publish --name <name> --version <version> <package.tor>
```

```sh
tol publish --name trc20-base --version 1.0.0 trc20-base-1.0.0.tor
```

### 7.7 `tol test` — run tests

```sh
tol test [flags] [./path/...]
```

See [TOL_TEST.md](TOL_TEST.md) for full documentation.

```sh
tol test ./...
tol test --run test_transfer ./trc20_test.tol
tol test --cover --covermin 80 ./...
tol test --fuzz fuzz_transfer --fuzz-time 30s
```

### 7.8 Flag conventions

| Convention | Example | Inspiration |
|------------|---------|-------------|
| Input files are positional | `tol compile input.tol` | javac, solc |
| Output file with `-o` / `--output` | `tol compile -o out.toc` | gcc, solc, go |
| Long flags with `--` | `--emit toi`, `--include-source` | solc, cargo |
| Short alias for frequent flags | `-o` = `--output` | javac `-d` |
| Format selection with `--emit` | `--emit toi` | solc `--abi`, `--bin`, `--ir` |
| Uniform `--json` for machine output | `tol inspect --json` | consistent across subcommands |
| Subcommand help | `tol compile --help` | go, cargo |

### 7.9 Migration from legacy flat-flag style

| Legacy (current) | New subcommand style |
|-----------------|----------------------|
| `tolang -ctoc out.toc input.tol` | `tol compile -o out.toc input.tol` |
| `tolang -ctoi out.toi input.tol` | `tol compile --emit toi -o out.toi input.tol` |
| `tolang -ctoi out.toi -ctoiname X input.tol` | `tol compile --emit toi --name X -o out.toi input.tol` |
| `tolang -dtoi interface.toi` | `tol inspect interface.toi` |
| `tolang -ctor out.tor input.tol` | `tol compile --emit tor -o out.tor input.tol` |
| `tolang -ctorpkg n -ctorver v -ctorifacename X -ctorsrc -ctor out.tor in.tol` | `tol compile --emit tor --package-name n --package-version v --name X --include-source -o out.tor in.tol` |
| `tolang -ctor out.tor ./dir/` | `tol pack -o out.tor ./dir/` |
| `tolang -dtoc artifact.toc` | `tol inspect artifact.toc` |
| `tolang -dtocj artifact.toc` | `tol inspect --json artifact.toc` |
| `tolang -vtoc artifact.toc` | `tol verify artifact.toc` |
| `tolang -vtoc -vtocsrc src.tol artifact.toc` | `tol verify --source src.tol artifact.toc` |
| `tolang -vtoi interface.toi` | `tol verify interface.toi` |
| `tolang -dtor artifact.tor` | `tol inspect artifact.tor` |
| `tolang -dtorj artifact.tor` | `tol inspect --json artifact.tor` |
| `tolang -vtor artifact.tor` | `tol verify artifact.tor` |

---

## 8. Security Properties

1. **Immutability** — `name@version` bindings in the registry are permanent. No
   supply-chain attack can replace an audited package.

2. **Content-addressing** — every `.toc` and `.tor` is identified by the keccak256
   hash of its contents. The hash is the identity; the name is only a convenience.

3. **Interface separation** — callers import `.toi` (interface only). The
   implementation bytecode is not trusted by the caller's compiler; only the ABI
   shape is verified.

4. **No dynamic loading** — packages are resolved at compile time. There is no
   runtime `require()` or dynamic class loading. The deployed bytecode is fully
   self-contained.

5. **Reproducible builds** — `tor.lock` pins all transitive dependencies to content
   hashes. Any engineer can reproduce the exact bytecode from source + lockfile.

6. **Tests shipped with packages** — every `.tor` in tol-stdlib includes its
   `*_test.tol` suite. Users can run `tol test trc20-base-1.0.0.tor` to verify
   the package behaves correctly in their local environment before trusting it.
