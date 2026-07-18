# Rainstorm v6.0.0 Release Notes

> **Status:** Final release preparation. `v6.0.0-rc.1` is published and validated; the final `v6.0.0` tag and GitHub release are pending merge and CI.
> **Release preparation date:** 2026-07-17.

## 1. Release status

Rainstorm `v6.0.0-rc.1` was published as a GitHub prerelease and passed its GitHub Actions workflow. A clean-checkout validation has since completed successfully. This document describes the completed v6 work, breaking changes, evidence, and known limitations. The final `v6.0.0` tag and GitHub release have not yet been created; they remain gated on the final preparation PR and CI.

## 2. Executive summary

Rainstorm v6 is a major release focused on **context propagation**, **managed
transactions**, **operation-wrapped errors**, and a **clarified lifecycle and
ownership model**. Checked-in fixtures verify v5.3.0 interoperability for the tested schemas and codecs; v6 also adds benchmarks, CI quality gates, and a
comprehensive migration guide.

The v6 module path is:

```
github.com/AndersonBargas/rainstorm/v6
```

The minimum supported Go version is **Go 1.24**.

## 3. Why v6 exists

Rainstorm v5 was a reliable, feature-complete persistence toolkit. As
applications adopted structured context propagation, cooperative cancellation,
and formal error classification, v5's context-free API became a friction point:

- Every database operation needed its own implicit transaction.
- There was no standard way to propagate request-scoped cancellation.
- Errors lacked operation context, making diagnosis and classification
  difficult.
- The public bbolt surface invited accidental invariant bypass.

Rainstorm v6 addresses these points without a storage-engine rewrite or an intentional persisted-layout migration. Compatibility claims remain scoped to the checked-in v5.3.0 fixtures and roundtrip evidence.

## 4. Highlights

- **Context-first API**: Open, CRUD, finder, query-terminal, scanner, KV, and
  managed-transaction operations accept `context.Context`. Lifecycle and
  configuration methods remain context-free.
- **Managed transactions**: `ReadTransaction` and `WriteTransaction` with
  callback-managed boundaries. No manual `Begin`/`Commit`/`Rollback`.
- **Cooperative cancellation**: Context checked at operation boundaries and
  loop checkpoints. No abandoned goroutines. Published reads and committed
  writes are not retroactively cancelled.
- **Operation-wrapped errors**: Public operations add a `rainstorm <operation>: <cause>` prefix and preserve wrapped causes for `errors.Is` classification.
- **Owned vs. borrowed lifecycle**: `Open` without `UseDB` creates and owns
  the native database; `Open` with `UseDB` borrows an existing one. Ownership
  determines `Close` behaviour.
- **Native escape hatch**: `NativeDB()` replaces the removed `DB.Bolt` field.
- **Destination safety**: Read operations publish to caller destinations only
  after successful decode.
- **v5.3.0 on-disk compatibility**: JSON, Gob, MsgPack, Sereal, and AES-JSON
  fixtures verified. Same-process bidirectional roundtrip.

[`MIGRATION_V6.md`](../MIGRATION_V6.md) provides a complete, step-by-step
migration guide with mechanical before/after replacements.

## 5. Breaking changes

Every breaking change is described with a replacement in
[`MIGRATION_V6.md`](../MIGRATION_V6.md). The summary:

| Change | Replacement |
|--------|-------------|
| `Open(path)` → `Open(ctx, path)` | Add `context.Context` |
| All CRUD/finder/query/KV ops require `ctx` | Add `context.Context` as first arg |
| `DB.Bolt` → `NativeDB()` | Method call |
| `Begin` / `Commit` / `Rollback` / `Tx` removed | `ReadTransaction` / `WriteTransaction` |
| `Batch` / `WithBatch` removed | Group writes explicitly in `WriteTransaction`; Batch coalescing/retry semantics are not preserved |
| `WithTransaction` removed | No equivalent for binding Rainstorm to a caller-owned `*bbolt.Tx` |
| `GetBucket` / `CreateBucketIfNotExists` unexported | Use `NativeDB()` for fully native bucket work, outside Rainstorm guarantees |
| `ErrNotInTransaction` removed | Managed transactions avoid this state |
| `UseDB` no longer closes the borrowed DB | Caller manages native lifecycle |
| `PrefixScan`/`RangeScan` return `([]Node, error)` with context | Update return handling |
| Errors must be classified with `errors.Is` | Never use `==` or string parsing |

## 6. Context-first API

The following operation groups require `context.Context`:

- **Open**: `Open(ctx, path, options...)`
- **CRUD**: `Save`, `Update`, `UpdateField`, `DeleteStruct`, `Drop`, `Init`,
  `ReIndex`
- **Finder**: `One`, `Find`, `AllByIndex`, `All`, `Range`, `Prefix`, `Count`
- **Query terminal**: `Find`, `First`, `Delete`, `Count`, `Raw`, `RawEach`,
  `Each`
- **Scanner**: `PrefixScan`, `RangeScan`
- **KV**: `Get`, `Set`, `Delete`, `GetBytes`, `SetBytes`, `KeyExists`
- **Managed transactions**: `ReadTransaction`, `WriteTransaction`

The following methods remain context-free (lifecycle/configuration):

- `Select`, `From`, `Bucket`, `Codec`, `WithCodec`, `Close`, `NativeDB`

## 7. Managed transactions

The abbreviated fragments below correspond to the executable `ExampleDB_ReadTransaction` and `ExampleDB_WriteTransaction` examples in `examples_test.go`.

```go
// Read-only transaction
err := db.ReadTransaction(ctx, func(txNode rainstorm.Node) error {
    var user User
    if err := txNode.One(ctx, "ID", 1, &user); err != nil {
        return err
    }
    // ... more reads ...
    return nil
})

// Read-write transaction
err = db.WriteTransaction(ctx, func(txNode rainstorm.Node) error {
    if err := txNode.Save(ctx, &user); err != nil {
        return err
    }
    return txNode.Set(ctx, "logs", time.Now(), "user created")
})
```

Key semantics:

- The callback receives a transaction-bound `Node`.
- Descendants from `txNode.From(...)` and `txNode.WithCodec(...)` remain
  transaction-bound.
- The callback executes exactly once (no bbolt `Batch` retry semantics).
- Callback error causes write rollback.
- Callback error remains primary if the callback also cancels context.
- Cancellation before commit causes rollback.
- No retroactive cancellation after successful commit.
- Panics propagate unchanged after bbolt unwinds.
- The callback node must not be used after the callback returns.

## 8. Cooperative cancellation

Cancellation is cooperative, not preemptive. Rainstorm checks the context at:

- Operation entry (before any work).
- After bbolt transaction acquisition.
- At every loop checkpoint and cursor iteration boundary.
- Before commit and before publishing read results.

Rainstorm cannot interrupt:

- A filesystem syscall.
- A bbolt mutex or lock wait.
- Codec code that does not observe context.
- A user callback that has not returned control.

No abandoned goroutines are used to simulate cancellation. Published read
destinations and successfully committed writes are not retroactively changed
to cancellation errors.

## 9. Error model

All operation errors follow the format:

```
rainstorm <operation>: <cause>
```

Classification uses `errors.Is`; the same pattern is executable in `Example_errorsIs`:

```go
if errors.Is(err, rainstorm.ErrNotFound) {
    // handle a missing record
}
if errors.Is(err, context.Canceled) {
    // handle cancellation
}
```

Never use `==` or string comparison.

Sentinel inventory:

| Sentinel | Meaning |
|----------|---------|
| `ErrNotFound` | Record not found |
| `ErrAlreadyExists` | Duplicate unique value |
| `ErrNilContext` | Nil context passed to a context-aware operation |
| `ErrNilParam` | Nil required non-context argument |
| `ErrNoID` | No ID field or `id` tag found |
| `ErrZeroID` | ID field is a zero value |
| `ErrBadType` | Unexpected value type |
| `ErrUnknownTag` | Unrecognised struct tag |
| `ErrIdxNotFound` | Index not found |
| `ErrSlicePtrNeeded` | Expected pointer to slice |
| `ErrStructPtrNeeded` | Expected pointer to struct |
| `ErrPtrNeeded` | Expected pointer |
| `ErrNoName` | Struct has no name and no bucket specified |
| `ErrIncompatibleValue` | Value type does not match field type |
| `ErrDifferentCodec` | Codec incompatible with existing bucket |

`ErrNotInTransaction` has been removed. Managed transactions make this state
unreachable in normal usage.

Nested wrapping is allowed — for example, `Open` error wrapping may contain an
inner `rainstorm kv get bytes: ...` chain through `checkVersion`.

`ErrDifferentCodec` paths are not uniform in cause preservation:

- Database-version decode/decryption failure during `Open`: joined cause
  preserved via `errors.Join`.
- Metadata marker mismatch at write time: may return bare `ErrDifferentCodec`.
- Read path with wrong node codec: may return codec-specific decode error
  without marker classification.

## 10. Lifecycle and bbolt ownership

### Owned

This abbreviated lifecycle fragment follows the checked-close pattern in executable `ExampleOpen`:

```go
db, err := rainstorm.Open(ctx, "my.db")
if err != nil {
    return err
}
defer func() {
    if closeErr := db.Close(); closeErr != nil {
        log.Printf("close Rainstorm database: %v", closeErr)
    }
}()
```

- `Open` creates and opens the BoltDB file.
- Initialization failure closes only the owned database.
- `DB.Close()` closes the underlying BoltDB file.

### Borrowed

This abbreviated lifecycle fragment corresponds to executable `ExampleUseDB`:

```go
bDB, err := bolt.Open("my.db", 0600, nil)
if err != nil {
    return err
}
db, err := rainstorm.Open(ctx, "", rainstorm.UseDB(bDB))
if err != nil {
    return errors.Join(err, bDB.Close())
}
// DB.Close does not close bDB; the caller closes both handles in this order.
if err := db.Close(); err != nil {
    return err
}
if err := bDB.Close(); err != nil {
    return err
}
```

- `UseDB(nil)` returns `ErrNilParam`.
- Rainstorm does not close a borrowed database.
- `DB.Close()` returns nil for borrowed databases.
- The caller retains native lifecycle ownership.

## 11. Native escape hatch

`NativeDB()` returns the underlying `*bbolt.DB` pointer (exact pointer
identity — the same `*bolt.DB` that Rainstorm uses internally).

**Warnings:**

- Native operations bypass Rainstorm context checkpoints.
- Native writes can bypass codecs, indexes, metadata, and invariants.
- Rainstorm cannot guarantee index consistency or cancellation for native
  operations.
- Callers must coordinate native and Rainstorm transactions internally.
- Do not close the native database while Rainstorm is active.

## 12. Destination/output safety

Read operations that decode into a caller-provided destination (`One`, `Find`,
`First`, `Get`, etc.) decode into a temporary buffer first and only publish
to the caller's destination after successful decode and context check. This
prevents partially-populated or invalid data from appearing in the caller's
value on error.

This behaviour has been verified for the following operation groups:

- Finder: `One`, `Find`, `First`, `All`, `AllByIndex`, `Range`, `Prefix`
- KV: `Get`

## 13. v5.3.0 compatibility

Rainstorm v6 reads and mutates databases created by v5.3.0 without migration
for all tested codecs:

| Codec | Fixture | Read | Mutate | Reopen |
|-------|---------|------|--------|--------|
| JSON (default) | baseline.db | ✓ | ✓ | ✓ |
| Gob | gob.db | ✓ | ✓ | ✓ |
| MsgPack | msgpack.db | ✓ | ✓ | ✓ |
| Sereal | sereal.db | ✓ | ✓ | ✓ |
| AES-JSON | aes.db | ✓ | ✓ | ✓ |

Tested features: IDs, ordinary indexes, unique indexes, nested buckets, KV
operations, `BucketNamer`, and `reflect.StructOf`.

See [`testdata/compatibility/README.md`](../testdata/compatibility/README.md)
for the full matrix and methodology.

## 14. Codec compatibility

Rainstorm stores an encoded database-version value used during `Open` and
codec metadata in record/KV leaf buckets. Structural parent buckets may have
no codec marker.

- A failure to decode the stored database-version marker during `Open` is classified as `ErrDifferentCodec`, with the decode/decryption cause preserved via `errors.Join`. Corruption or a decoder defect can reach the same classification.
- Metadata marker mismatch at initialization/write time returns bare
  `ErrDifferentCodec`.
- Reading with the wrong node codec may produce a codec-specific decode error
  rather than `ErrDifferentCodec`.
- AES wrong-key fixture path preserves the decryption cause.

Nodes configured with `WithCodec` must continue using their matching codec.

## 15. Performance evidence

Benchmarks compare equivalent v5.3.0 and v6 workloads (Apple M4, darwin/arm64,
Go 1.26.5, default JSON codec, `NoSync`, five samples, manual medians,
same-process shared-MVS comparison).

| Operation | v6 vs v5 ns/op (approx.) |
|-----------|--------------------------|
| Point reads (OneByID, OneByUnique) | +2–3% |
| Bulk reads (Find, All) | +0.4–1.3% |
| Save | +18.2% |
| Update | +1.9–2.8% |
| KVGet | +11.7% |
| KVSet | +2.2% |

No paired benchmark exceeded the predefined 25% investigation threshold. No
confidence intervals, statistical-significance analysis, or causal attribution
was performed. Results are environment-specific descriptive snapshots, not
universal performance guarantees.

See [`testdata/compatibility/benchmark/results/comparison.md`](../testdata/compatibility/benchmark/results/comparison.md)
for the full methodology, raw results, and limitations.

## 16. Production defects fixed during compatibility work

- **`newListSink` custom bucket resolution**: When a type implements
  `BucketNamer`, list-producing operations (`Find`, `All`, etc.) now resolve
  the bucket name from the `BucketNamer` interface rather than the static type
  name, preserving v5.3.0 behaviour for custom-named buckets.
- **`checkVersion` codec mismatch classification**: Decode/decryption failures
  during `Open` are now classified as `ErrDifferentCodec` with the underlying
  cause preserved via `errors.Join`, distinct from metadata marker mismatches.
- **Defensive `isInteger(nil)`**: `isInteger` in `extract.go` handles nil inputs safely.
- **Protobuf fixture API cleanup**: The generated `SimpleUser` fixture now compiles only in codec tests, removing an accidental exported type while retaining the same descriptor for wire roundtrip coverage.

## 17. Dependency and Go-version policy

- **Go minimum**: 1.24.
- **Storage engine**: `go.etcd.io/bbolt` v1.4.3 — intentionally frozen for v6.
- **Codecs**: All codec dependencies intentionally frozen for v6 (Sereal,
  protobuf via `golang/protobuf`, MsgPack). Revisit in v7.
- **Test framework**: `github.com/stretchr/testify` v1.11.1.
- **Staticcheck**: 2026.1.
- **CI actions**: `actions/checkout@v7`, `actions/setup-go@v6`,
  `actions/upload-artifact@v7`.

Dependabot is configured for the root Go module and GitHub Actions. Nested
compatibility modules are excluded. Major action upgrades are pinned
explicitly.

See [`docs/dependency-audit.md`](dependency-audit.md) for the complete audit.

## 18. CI and quality gates

The CI workflow (`.github/workflows/main.yml`) enforces:

| Gate | Details |
|------|---------|
| Formatting | `gofmt -l .` |
| Module tidy | `go mod tidy` + diff check |
| Module verify | `go mod verify` |
| Vet | `go vet ./...` |
| Build | `go build ./...` |
| Staticcheck | `staticcheck@2026.1 ./...` |
| Test matrix | `go test` on Go 1.24.x and stable |
| Race | `go test -race` on stable |
| Compatibility | Nested module tidy/verify, roundtrip tests, and benchmark-module normal/race compile checks (no `-bench` execution) |
| Coverage | ≥80.0% with `covermode=atomic` |

The workflow is configured and passed on the published `v6.0.0-rc.1` release-candidate commit. Local equivalents also pass. The final release-preparation commit must pass the same GitHub-hosted workflow before tagging `v6.0.0`.

## 19. Upgrade instructions

1. Read [`MIGRATION_V6.md`](../MIGRATION_V6.md).
2. Update `go.mod` to `github.com/AndersonBargas/rainstorm/v6` and Go 1.24.
3. Add `context.Context` to all `Open`, CRUD, finder, query, scanner, and KV
   call sites.
4. Replace `DB.Bolt` with `NativeDB()`.
5. Rewrite manual transaction code to `ReadTransaction` / `WriteTransaction`.
6. Replace `errors.Is(err, ErrNotInTransaction)` checks — managed transactions
   avoid this state.
7. Replace `==` sentinel comparisons with `errors.Is`.
8. Verify `UseDB` callers manage the native lifecycle (Rainstorm no longer
   closes borrowed databases).
9. Run your application test suite.

## 20. Known limitations

- **v5.3.0 is the tested compatibility baseline.** Earlier v5 versions are not
  covered by checked-in fixtures.
- **Process-isolated roundtrip is not proven.** The roundtrip test uses a
  single test binary with shared MVS dependency resolution.
- **Protobuf fixtures are excluded.** The existing generated type cannot carry
  the required indexed fixture schema; a generic struct would fall back to
  JSON.
- **Custom codecs and arbitrary historical schemas** need application testing.
- **bbolt files/ciphertext are not claimed byte-for-byte deterministic.**
- **Cancellation is cooperative**, not preemptive.
- **Native operations are outside Rainstorm guarantees.**
- **Caller-owned native transactions cannot be rebound to Rainstorm.** Removal of `WithTransaction(*bbolt.Tx)` has no equivalent v6 API; use fully native bbolt operations or let Rainstorm create a managed transaction.
- **Benchmark evidence is environment-specific** and not statistically
  conclusive. No universal performance guarantee.
- **Dependencies, codecs, and storage engine** are intentionally frozen for v6.
- **No vulnerability scan was run.** `govulncheck` was unavailable during the dependency audit, so no vulnerability-free or security-audited claim is made.

## 21. Deferred v7 work

The following items are recorded for future consideration and are documented
in [`docs/future-directions.md`](future-directions.md):

- Protobuf migration from `github.com/golang/protobuf` to
  `google.golang.org/protobuf`.
- MsgPack v4 → v5 migration.
- Sereal and transitive dependency updates/replacement evaluation.
- bbolt v1.5 evaluation.
- Broader dependency modernization.
- Performance architecture and optimization.
- Process-isolated compatibility testing.
- Neutral record engine, compiled schemas, unified mutation plans, internal
  storage abstractions, index lifecycle components, query planning/explanation,
  streaming iterators, observability hooks, schema catalog/migrations, and
  bbolt cancellation research.

No v7 API commitments have been made. Future directions are exploratory and
non-normative.

## 22. Release checklist/status

- [x] Context-first API
- [x] Managed transactions
- [x] Cooperative cancellation
- [x] Operation-wrapped errors
- [x] Lifecycle and bbolt ownership model
- [x] Native escape hatch
- [x] Destination/output safety
- [x] v5.3.0 compatibility suite
- [x] Codec compatibility matrix
- [x] v5→v6→v5→v6 roundtrip
- [x] Paired benchmarks
- [x] CI and quality gates
- [x] Dependency audit and Dependabot
- [x] README and package documentation
- [x] Executable examples
- [x] Migration guide (`MIGRATION_V6.md`)
- [x] Changelog (`CHANGELOG.md`)
- [x] Release notes (this document)
- [x] Public API audit and GoDoc review
- [x] Final release-candidate validation (R6.7D)
- [x] Published prerelease `v6.0.0-rc.1`
- [ ] Final release-preparation PR and CI
- [ ] Git tag `v6.0.0`
- [ ] GitHub release

## 23. Supporting links

- [README](../README.md)
- [Migration guide](../MIGRATION_V6.md)
- [Architecture plan](v6-architecture-plan.md)
- [Dependency audit](dependency-audit.md)
- [Future directions](future-directions.md)
- [Compatibility suite](../testdata/compatibility/README.md)
- [Benchmark comparison](../testdata/compatibility/benchmark/results/comparison.md)
- [CI workflow](../.github/workflows/main.yml)
- [Dependabot configuration](../.github/dependabot.yml)
