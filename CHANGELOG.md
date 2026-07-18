# Changelog

## [6.0.0] — 2026-07-17

### Added

- **Context-aware persistence APIs**: `Open`, CRUD, finder, query-terminal, scanner, KV, and
  managed-transaction operations accept `context.Context`. `Select`, `From`, `Bucket`,
  `Codec`, `WithCodec`, `Close`, and `NativeDB` remain context-free.
- **`ErrNilContext`**: New sentinel returned when a nil context is passed to a context-aware
  operation (distinct from `ErrNilParam`).
- **Managed transactions**: `ReadTransaction` and `WriteTransaction` provide callback-managed
  read and write transactions. Callback receives a transaction-bound `Node`; descendants
  from `tx.From(...)` and `tx.WithCodec(...)` remain transaction-bound.
- **`NativeDB()` method**: Replaces the removed `DB.Bolt` public field. Returns the
  underlying `*bbolt.DB` pointer for advanced interoperability.
- **Owned/borrowed lifecycle distinction**: `Open` without `UseDB` creates and owns the
  native database; `Open` with `UseDB` borrows an existing one. Ownership determines Close
  behavior.
- **Operation wrapping**: Public operations add a `rainstorm <operation>: <cause>` prefix while preserving wrapped causes for `errors.Is` classification.
- **Destination/output safety**: Read operations decode into a temporary buffer before
  publishing to the caller's destination, preventing partial writes on error.
- **v5.3.0 compatibility fixtures**: Checked-in baseline, Gob, MsgPack, Sereal, and
  AES-JSON fixtures with full read/mutate/reopen behavioural tests.
- **v5→v6→v5→v6 roundtrip**: Same-process cross-major module roundtrip test proving
  bidirectional interoperability.
- **v5/v6 benchmarks**: Paired benchmark comparison (five samples, manual medians) with
  v6-only ReadTransaction, WriteTransaction, and CanceledContext benchmarks.
- **CI and quality gates**: GitHub Actions workflow with formatting, tidy, vet, build, Staticcheck 2026.1, test matrix, race detector, compatibility verification, benchmark-module compile checks, and coverage threshold (≥80.0%).
- **Dependency automation**: Dependabot configured for Go modules and GitHub Actions with
  codec/storage dependencies intentionally frozen for v6.

### Changed

- **Module path** to `github.com/AndersonBargas/rainstorm/v6`.
- **Go minimum version** raised to 1.24.
- **All context-aware operation signatures** require `context.Context` as first parameter:
  `Open`, `Save`, `Update`, `UpdateField`, `DeleteStruct`, `Drop`, `Init`, `ReIndex`,
  `One`, `Find`, `AllByIndex`, `All`, `Range`, `Prefix`, `Count`, `Find`, `First`,
  `Delete`, `Count`, `Raw`, `RawEach`, `Each`, `PrefixScan`, `RangeScan`, `Get`, `Set`,
  `Delete`, `GetBytes`, `SetBytes`, `KeyExists`, `ReadTransaction`, `WriteTransaction`.
- **Error classification**: All operations use `errors.Is`-compatible wrapping. Callers
  must use `errors.Is` for sentinel matching, never `==` or string comparison.
- **Borrowed lifecycle**: `UseDB` no longer closes the borrowed database; `DB.Close()`
  returns nil for borrowed databases.
- **Scanner return signatures**: `PrefixScan` and `RangeScan` return `([]Node, error)`
  accepting context.
- **Codec mismatch classification**: `checkVersion` classifies decode/decryption failures
  during `Open` as `ErrDifferentCodec` preserving the underlying cause via `errors.Join`.
  Metadata marker mismatch at write time returns bare `ErrDifferentCodec`.
- **Callback/cancellation semantics**: `WriteTransaction` callback error takes precedence
  over a cancelled context; cancellation before commit rolls back; no retroactive
  cancellation after successful commit.
- **Package documentation and examples**: All package-level GoDoc and runnable examples
  updated for v6 API.
- **Version constant**: The legacy metadata marker remains `"5.3.0"`. `Open` accepts any non-empty decodable marker; compatibility is established by behavioral fixtures, not by this value.

### Removed

- `Begin` — replaced by `ReadTransaction` / `WriteTransaction`.
- `Commit` — commit is automatic in managed transactions.
- `Rollback` — rollback on error is automatic in `WriteTransaction`.
- Public `Tx` — transaction is managed internally.
- `WithTransaction` — removed with no equivalent for binding Rainstorm to a caller-owned `*bbolt.Tx`; managed transactions create their own transaction.
- `GetBucket` and `CreateBucketIfNotExists` — raw bucket helpers are no longer public; use `NativeDB()` for fully native bbolt work, outside Rainstorm guarantees.
- `Batch` and `WithBatch` — removed. Group writes explicitly in `WriteTransaction`; this changes bbolt Batch coalescing/retry and atomicity semantics.
- `ErrNotInTransaction` — removed; managed transactions avoid this state.
- Exported `DB.Bolt` field — replaced by `NativeDB()` method.

### Fixed

- `newListSink` custom bucket resolution for named `BucketNamer` types (preserving v5.3.0
  behaviour for list-producing operations).
- `checkVersion` now classifies failure to decode the stored version marker as `ErrDifferentCodec` and preserves the decode/decryption cause. This classification may also cover corruption or decoder failure, not only a true codec-name mismatch.
- `isInteger(nil)` handled defensively in `extract.go`.
- Generated Protobuf `SimpleUser` fixture moved to test-only scope so it is no longer an accidental exported production API; codec wire tests continue using the same generated descriptor.

### Compatibility

- v5.3.0 JSON, Gob, MsgPack, Sereal, and AES-JSON fixtures verified: read, mutate, and
  reopen work without migration.
- IDs, ordinary indexes, unique indexes, nested buckets, KV, `BucketNamer`, and
  `reflect.StructOf` all tested.
- Same-process v5→v6→v5→v6 roundtrip verified.
- Protobuf fixtures excluded — existing generated type cannot carry required indexed
  fixture schema.
- No process-isolated roundtrip claim; both versions share a single dependency graph
  through MVS.

### Performance

- Paired v5.3.0/v6 benchmarks (Apple M4, darwin/arm64, Go 1.26.5, default JSON, `NoSync`,
  five samples, manual medians):
  - Point reads: approximately 2–3% higher ns/op.
  - Bulk reads: approximately 0.4–1.3% higher ns/op.
  - Save: approximately 18% higher ns/op (largest observed difference).
  - KVGet: approximately 12% higher ns/op.
- No paired benchmark crossed the predefined 25% investigation threshold.
- No confidence interval, statistical-significance analysis, or causal attribution.
- Same-process shared-MVS comparison; no process isolation.

### Security/dependency policy

- Codec and storage dependencies intentionally frozen for v6: Sereal, protobuf, MsgPack, and bbolt. Revisit in a future v7.
- `govulncheck` was not available during the dependency audit; no vulnerability-free or security-audited claim is made.
- Dependabot configured for root Go module and GitHub Actions; nested compatibility
  modules excluded.
- CI enforces coverage ≥80.0%, `gofmt`, `go vet`, `go build`, and Staticcheck 2026.1.

### Migration

See [`MIGRATION_V6.md`](MIGRATION_V6.md) for the complete v5.3.0 → v6 migration guide.

## [5.3.0] - 2026-07-04

### Added

- **`BucketNamer` interface**: New public interface `rainstorm.BucketNamer` with method
  `RainstormBucketName() string`. Structs implementing this interface can define a custom
  bucket name, overriding the default type-name-based bucket resolution. This is essential
  for runtime-generated types (`reflect.StructOf`) that have no static type name.
  ([#1](#))

- **Runtime struct support**: Anonymous structs created via `reflect.StructOf` can now be
  stored and queried when used with `db.From("bucketName")`. The innermost root bucket is
  used as the data bucket when the struct type has no name and no `BucketNamer`.

- **Root bucket as data bucket**: `CreateBucketIfNotExists` and `GetBucket` now filter out
  empty bucket name segments, allowing `db.From("my_bucket")` to serve as the data bucket
  directly for anonymous types.

- **Tests for dynamic structs**: Comprehensive test suite in `dynamic_struct_test.go`
  covering save, read, find, multitenancy, combined queries, delete, init, update, and
  drop operations with both runtime-generated structs and `BucketNamer` implementations.

### Changed

- **`extract()` function**: Now resolves the bucket name via `BucketNamer` if the struct
  implements it, falling back to the static type name. No longer returns `ErrNoName`
  for empty names (the caller is responsible for handling this).

- **Sink `bucketName()` methods**: `firstSink`, `deleteSink`, `countSink`, and `eachSink`
  now check `BucketNamer` before falling back to the static type name.

- **`newListSink()`**: No longer rejects anonymous types with `ErrNoName`. Empty bucket
  names are allowed when the caller provides context (e.g., via `db.From()` or
  `Query.Bucket()`).

- **Finder methods** (`One`, `Find`, `Range`, `Prefix`): Use `resolveBucketName()` which
  falls back to the root bucket for anonymous types.

- **`query.query()`**: Added root bucket fallback and `ErrNoName` check for empty bucket
  names when neither the query nor the sink provides a name.

### Fixed

- Fixed a bug where bucket name resolution via `BucketNamer` created double-nested bucket
  paths when using `db.From(name)` with a custom bucket name.
