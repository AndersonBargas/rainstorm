# Rainstorm v6 architecture plan

Status: normative

This document defines the architecture, public API direction, cancellation semantics, migration boundaries, and implementation phases for Rainstorm v6. Implementations must follow this document unless it is amended explicitly.

## 1. Goals

Rainstorm v6 will:

1. make `context.Context` mandatory for database operations;
2. provide explicit, safe transaction boundaries;
3. support cooperative cancellation during scans and internal loops;
4. preserve the strengths of the v5 data model, indexes, codecs, nested buckets, and runtime-generated structs;
5. expose errors that work reliably with `errors.Is`;
6. modernize dependencies, CI, documentation, and examples;
7. provide a direct migration guide from v5.

The v6 module path will be:

```go
module github.com/AndersonBargas/rainstorm/v6
```

## 2. Non-goals

The first v6 release will not:

- rewrite the storage engine;
- replace bbolt;
- introduce an ORM-style relationship model;
- introduce implicit retries;
- claim hard cancellation of an in-progress syscall or bbolt lock wait;
- change the on-disk format without a separately documented reason;
- add generics merely to eliminate every use of reflection;
- retain source compatibility with v5.

Database-file compatibility and Go API compatibility are separate concerns. A v6 API break does not, by itself, require an on-disk format break.

## 3. Findings from v5

### 3.1 Public surface

The v5 root package exposes:

- `DB` and `Node`;
- `TypeStore`, `Finder`, `Query`, `KeyValueStore`, `BucketScanner`, and `Tx`;
- struct operations: `Init`, `ReIndex`, `Save`, `Update`, `UpdateField`, `Drop`, and `DeleteStruct`;
- finder operations: `One`, `Find`, `AllByIndex`, `All`, `Select`, `Range`, `Prefix`, and `Count`;
- query terminal operations: `Find`, `First`, `Delete`, `Count`, `Raw`, `RawEach`, and `Each`;
- key/value operations: `Get`, `Set`, `Delete`, `GetBytes`, `SetBytes`, and `KeyExists`;
- manual transaction operations: `Begin`, `Commit`, `Rollback`, and `WithTransaction`;
- bbolt details through `DB.Bolt`, `GetBucket`, `CreateBucketIfNotExists`, `WithTransaction`, `BoltOptions`, and `UseDB`;
- nested nodes through `From`;
- runtime type bucket naming through `BucketNamer`;
- codecs and index/query options.

All database operations are currently context-free. Cursor scans and index-processing loops cannot observe cancellation.

### 3.2 Transaction behavior

v5 operations use `bbolt.View`, `bbolt.Update`, or `bbolt.Batch` when a node does not already contain a transaction. A manually created transaction is stored in a derived node.

The main risks are:

- callers can forget rollback;
- a transaction can outlive its intended request;
- context ownership is absent;
- public bbolt access allows bypassing Rainstorm invariants;
- batch mode has retry semantics that are not obvious at an application transaction boundary.

### 3.3 Runtime-generated structs

The v5.3 functionality is intentional and must be retained:

- `BucketNamer` overrides the static type name;
- runtime-generated structs can use a node root as their data bucket;
- nested bucket paths remain supported;
- index and uniqueness enforcement remains native.

## 4. Context contract

### 4.1 General rule

Every exported operation that may access the database receives `context.Context` as its first argument.

Configuration-only methods such as `From`, `WithCodec`, option constructors, and query builders do not receive a context. `Close` remains context-free because it is a resource lifecycle operation rather than a request operation.

A nil context is invalid. Implementations must not replace it silently with `context.Background()`. The exact nil-context error will be defined during the error phase.

### 4.2 Required checks

An operation must check `ctx.Err()`:

1. before opening or entering a transaction;
2. after a transaction is acquired;
3. at cooperative checkpoints in scans and potentially long loops;
4. before committing a managed writable transaction.

Internal helpers must receive the caller context rather than construct a new one.

### 4.3 Read semantics

If cancellation is observed before a read result is returned, the operation returns an error matching `context.Canceled` or `context.DeadlineExceeded`. Partial decoded collections must not be presented as a successful complete result.

Streaming callbacks stop at the first observed cancellation.

### 4.4 Write semantics

For an automatically managed write transaction:

- cancellation before commit causes rollback;
- callback or validation failure causes rollback;
- commit occurs only after the callback succeeds and the context remains valid;
- once `Commit` succeeds, the write is successful and must not subsequently be reported as rolled back because the context was canceled afterward.

Rainstorm must not return `context.Canceled` after a confirmed successful commit merely because cancellation raced with the return path.

### 4.5 Limits inherited from bbolt

Cancellation is cooperative, not preemptive.

Rainstorm cannot honestly guarantee immediate interruption of:

- an operating-system syscall;
- a bbolt-internal mutex or lock wait;
- codec code that does not observe a context;
- user callback code that ignores its context.

Rainstorm must not start an uninterruptible bbolt operation in a goroutine and abandon that goroutine when the context is canceled. Such an operation could later acquire a transaction and leak resources or perform unexpected work.

When a context is canceled while Rainstorm is waiting inside bbolt, Rainstorm checks cancellation immediately after control returns or the transaction is acquired, rolls back if needed, and returns the context error. Documentation must describe that return may not be instantaneous.

## 5. Public API direction

The signatures below define direction and naming. Exact option type names may be refined in the API implementation phase, but context placement and transaction semantics are binding.

### 5.1 Opening and lifecycle

```go
func Open(ctx context.Context, path string, options ...OpenOption) (*DB, error)
func (db *DB) Close() error
```

Opening a database performs I/O and therefore receives a context. Rainstorm checks it before opening and after initialization. Because bbolt does not provide a context-aware `Open`, cancellation remains cooperative. `Close` remains context-free so cleanup is always possible even after cancellation.

Options become named types rather than raw function signatures:

```go
type OpenOption func(*Options) error
type FindOption func(*index.Options)
```

### 5.2 Struct storage

```go
type TypeStore interface {
    Finder

    Init(ctx context.Context, data any) error
    ReIndex(ctx context.Context, data any) error
    Save(ctx context.Context, data any) error
    Update(ctx context.Context, data any) error
    UpdateField(ctx context.Context, data any, fieldName string, value any) error
    Drop(ctx context.Context, data any) error
    DeleteStruct(ctx context.Context, data any) error
}
```

### 5.3 Finders

```go
type Finder interface {
    One(ctx context.Context, fieldName string, value any, to any) error
    Find(ctx context.Context, fieldName string, value any, to any, options ...FindOption) error
    AllByIndex(ctx context.Context, fieldName string, to any, options ...FindOption) error
    All(ctx context.Context, to any, options ...FindOption) error
    Select(matchers ...q.Matcher) Query
    Range(ctx context.Context, fieldName string, min, max, to any, options ...FindOption) error
    Prefix(ctx context.Context, fieldName, prefix string, to any, options ...FindOption) error
    Count(ctx context.Context, data any) (int, error)
}
```

`Select` remains a pure builder. Context is supplied to terminal query operations.

### 5.4 Queries

```go
type Query interface {
    Skip(int) Query
    Limit(int) Query
    OrderBy(...string) Query
    Reverse() Query
    Bucket(string) Query

    Find(ctx context.Context, to any) error
    First(ctx context.Context, to any) error
    Delete(ctx context.Context, kind any) error
    Count(ctx context.Context, kind any) (int, error)
    Raw(ctx context.Context) ([][]byte, error)
    RawEach(ctx context.Context, fn func(key, value []byte) error) error
    Each(ctx context.Context, kind any, fn func(any) error) error
}
```

Query builders must be treated as single-use values unless and until concurrency safety is explicitly guaranteed. Documentation must not imply that a mutable query can be shared by goroutines.

### 5.5 Key/value API

```go
type KeyValueStore interface {
    Get(ctx context.Context, bucketName string, key any, to any) error
    Set(ctx context.Context, bucketName string, key, value any) error
    Delete(ctx context.Context, bucketName string, key any) error
    GetBytes(ctx context.Context, bucketName string, key any) ([]byte, error)
    SetBytes(ctx context.Context, bucketName string, key any, value []byte) error
    KeyExists(ctx context.Context, bucketName string, key any) (bool, error)
}
```

Returned byte slices remain defensive copies.

### 5.6 Bucket scanning

Bucket scans perform database reads and therefore become contextual and fallible:

```go
type BucketScanner interface {
    PrefixScan(ctx context.Context, prefix string) ([]Node, error)
    RangeScan(ctx context.Context, min, max string) ([]Node, error)
}
```

The v5 signatures that return only `[]Node` cannot report cancellation and will not be retained.

### 5.7 Nodes

The primary `Node` interface will contain Rainstorm abstractions, not raw bbolt transaction or bucket operations:

```go
type Node interface {
    TypeStore
    KeyValueStore
    BucketScanner

    From(path ...string) Node
    Bucket() []string
    Codec() codec.MarshalUnmarshaler
    WithCodec(codec.MarshalUnmarshaler) Node
}
```

The following v5 members must not remain in the primary `Node` contract:

- `GetBucket`;
- `CreateBucketIfNotExists`;
- `WithTransaction(*bolt.Tx)`;
- manual `Begin` inherited as the primary transaction API.

This prevents application code from depending on bbolt merely to use Rainstorm.

## 6. Transaction model

### 6.1 Canonical API

Callback-managed transactions are the canonical transaction boundary:

```go
type TransactionManager interface {
    ReadTransaction(ctx context.Context, fn func(Node) error) error
    WriteTransaction(ctx context.Context, fn func(Node) error) error
}
```

`DB` implements `TransactionManager`. The callback receives a transaction-bound `Node`.

`ReadTransaction` guarantees a read-only transaction. `WriteTransaction` guarantees a writable transaction. The explicit names avoid colliding with `TypeStore.Update`, because Go does not support method overloading. Nested transaction behavior must be rejected explicitly unless the callback is already using the transaction-bound node; Rainstorm must not silently open a second transaction.

### 6.2 Commit algorithm

`WriteTransaction` follows this sequence:

1. reject an already canceled context;
2. acquire/open the writable bbolt transaction;
3. check the context again;
4. invoke `fn` with a transaction-bound node;
5. if `fn` returns an error, roll back and return that error;
6. check the context before commit;
7. if canceled, roll back and return `ctx.Err()`;
8. attempt commit;
9. if commit fails, return the commit error;
10. after successful commit, return success without converting a later cancellation into failure.

Rollback errors must not erase the primary callback or context error. If both errors matter, Rainstorm will combine or wrap them in a form compatible with `errors.Is`.

### 6.3 Batch mode

bbolt `Batch` may execute a callback more than once. That is unsafe for callbacks with external side effects and conflicts with ordinary Unit of Work expectations.

Therefore:

- `WriteTransaction` must execute its callback exactly once;
- v5's global `Batch()` behavior must not alter `WriteTransaction` semantics;
- v6.0 removes the `Batch()` open option, `WithBatch`, and implicit batch state;
- v6.0 does not provide a batch replacement API;
- a future explicit batch API may be considered only under a separate design that names its retry semantics and requires retry-safe callbacks.

### 6.4 Manual transactions and native escape hatch

Manual `Begin`/`Commit`/`Rollback` will not be the recommended public path. Whether a low-level manual transaction API remains will be decided only after the managed API is complete.

Raw bbolt access must be isolated from the primary interfaces. If retained for advanced interoperability, it belongs behind an explicitly named native/unsafe escape hatch and its use weakens Rainstorm's cancellation guarantees. The v5 exported `DB.Bolt` field must not remain as an accidentally universal dependency.

`UseDB` may remain as an opening adapter, but ownership must be explicit. v6 must not ambiguously close a caller-owned bbolt database. The API must distinguish ownership or document a single unambiguous rule.

## 7. Error model

Existing sentinel meanings are retained where useful:

- `ErrNoID`;
- `ErrZeroID`;
- `ErrBadType`;
- `ErrAlreadyExists`;
- `ErrNilParam`;
- `ErrUnknownTag`;
- `ErrIdxNotFound`;
- pointer/target errors;
- `ErrNoName`;
- `ErrNotFound`;
- transaction-state errors;
- `ErrIncompatibleValue`;
- `ErrDifferentCodec`.

Requirements:

1. callers use `errors.Is`, not direct string comparison;
2. wrapped errors preserve their underlying sentinel or context error;
3. context cancellation returns an error matching `context.Canceled` or `context.DeadlineExceeded`;
4. bbolt errors remain discoverable through wrapping where relevant;
5. errors include operation context without leaking record data or secrets;
6. spelling and documentation defects may be fixed in v6;
7. typed errors are introduced only when callers need structured fields beyond a sentinel.

A dedicated error audit will determine aliases, removals, and new transaction errors. No implementation phase may silently change error classification.

## 8. Compatibility decisions

### 8.1 Source compatibility

v6 intentionally breaks source compatibility:

- context is mandatory;
- `interface{}` becomes `any` in public declarations;
- bucket scans return errors;
- raw bbolt members leave primary interfaces;
- managed transactions replace manual transactions as the normal API;
- option function types become named types.

No v6 compatibility wrappers may call `context.Background()` silently.

### 8.2 On-disk compatibility

The default objective is to preserve v5 database-file compatibility. Before release, compatibility tests must:

1. create fixtures with the last v5 release;
2. open and read them with v6;
3. write/update with v6;
4. verify indexes, unique constraints, nested buckets, codecs, metadata, and runtime struct buckets.

If a format change becomes necessary, it requires its own design, version marker, migration tool, and rollback guidance. It must not be hidden inside context work.

### 8.3 v5 maintenance

Before publishing v6:

- tag the final supported v5 release;
- keep the `/v5` module immutable except for deliberate critical fixes;
- publish a migration guide and replacement table;
- link the v6 documentation from the v5 README without rewriting v5 history.

## 9. Quality and modernization baseline

The v6 pipeline must run on pull requests and pushes and include:

```sh
gofmt check
go vet ./...
go test -count=1 ./...
go test -race -count=1 ./...
go test -coverprofile=coverage.out ./...
staticcheck ./...
go mod tidy check
go build ./...
```

Additional requirements:

- current stable supported Go version documented in `go.mod` and README;
- GitHub Actions pinned to maintained major versions;
- dependency update automation;
- coverage artifact and readable summary;
- examples compiled as tests;
- no CI exclusion merely because a change only touches documentation unless documentation has an independent validation workflow;
- release notes and migration guide required for the v6 tag.

Dependency updates must be performed separately from behavioral refactors when practical, so failures remain attributable.

## 10. Test requirements

### 10.1 Context behavior

Tests must prove:

- already canceled contexts suppress database work;
- deadlines return the correct context error;
- long full-bucket scans stop cooperatively;
- index loops stop cooperatively;
- query callbacks stop after cancellation;
- canceled writes roll back before commit;
- successful commits are not reported as canceled afterward;
- the exact caller context reaches managed transaction callbacks where observable;
- no operation creates a hidden background context.

Tests must not depend on timing alone when a deterministic hook or controlled callback can prove the boundary.

### 10.2 Transaction behavior

Tests must prove:

- callback errors roll back;
- cancellation before commit rolls back;
- commit errors are returned;
- callback executes exactly once in `Update`;
- writes in one transaction are atomic;
- uncommitted writes are not visible outside the transaction;
- read-only transactions reject writes through normal bbolt behavior/error mapping;
- panic policy is explicit and tested; the preferred policy is rollback followed by re-panic.

### 10.3 Existing guarantees

All existing behavior must remain covered:

- ID and increment handling;
- unique/list indexes;
- index movement during update;
- atomic index cleanup;
- nested buckets;
- codecs;
- KV operations;
- runtime-generated structs;
- `BucketNamer`;
- root bucket as data bucket;
- concurrent access under the race detector.

## 11. Documentation deliverables

v6 requires:

- refreshed `README.md` with context-first examples;
- package documentation;
- transaction and cancellation semantics;
- `MIGRATION_V6.md` with a mechanical replacement table;
- runtime-generated struct examples;
- nested bucket examples;
- codec examples;
- error handling with `errors.Is`;
- limitations inherited from bbolt;
- changelog and release notes.

A minimal migration example:

```go
// v5
err := db.Save(&user)

// v6
err := db.Save(ctx, &user)
```

Transaction migration:

```go
// v5: manual lifecycle
node, err := db.Begin(true)
// ...
err = node.Commit()

// v6: managed lifecycle
err := db.WriteTransaction(ctx, func(tx rainstorm.Node) error {
    return tx.Save(ctx, &user)
})
```

The transaction-bound node receives the same context explicitly. This is intentional: operation APIs remain uniform, and the node verifies cancellation during internal work.

## 12. Implementation phases

### R6.0 — Architecture and inventory

Deliverables:

- this normative plan;
- reviewed v5 public API inventory;
- binding context and transaction semantics;
- implementation phases and acceptance criteria.

No production changes.

### R6.1 — Module and API skeleton

- change module path to `/v6`;
- add context-first public interfaces and signatures;
- introduce named option types;
- update imports and compile-time surface;
- update tests mechanically to pass explicit contexts;
- do not yet claim loop-level cancellation until implemented.

Acceptance: repository compiles; every I/O method requires context; no compatibility wrappers use background contexts.

### R6.2 — Managed transaction core

- implement `ReadTransaction` and `WriteTransaction`;
- implement pre-acquisition, post-acquisition, and pre-commit checks;
- define panic and rollback-error behavior;
- ensure callback executes once;
- remove `Batch()`, `WithBatch`, and all implicit batch state from v6.0;
- add deterministic transaction tests.

Acceptance: rollback, cancellation, commit, panic, visibility, and single-execution tests pass under `-race`.

### R6.3 — CRUD, finder, query, scan, and KV cancellation

- thread context through all internal helpers;
- add cooperative checkpoints to cursor scans, sorting/filtering, index loops, reindexing, and multi-record operations;
- ensure collection methods do not report partial success;
- add cancellation tests for each operation family.

Acceptance: no production `context.Background()` or `context.TODO()`; cancellation tests prove suppressed work and rollback.

### R6.4 — Encapsulation and errors

- remove bbolt details from primary interfaces;
- resolve native escape-hatch and ownership API;
- audit sentinels and wrapping;
- guarantee `errors.Is` behavior;
- document reduced guarantees for native access.

Acceptance: root API consumers need no bbolt types; error classification tests pass.

### R6.5 — Compatibility and regression

- add v5-generated database fixtures;
- verify on-disk compatibility;
- retain runtime struct and `BucketNamer` behavior;
- benchmark representative v5/v6 paths;
- investigate regressions rather than weakening tests.

Acceptance: compatibility matrix and benchmark report are recorded.

### R6.6 — Dependencies and CI

- update dependencies in reviewable groups;
- modernize CI;
- add race, vet, staticcheck, formatting, build, coverage, and module checks;
- configure dependency automation.

Acceptance: all required checks run in CI and locally.

### R6.7 — Documentation and release

- rewrite examples for context-first API;
- create `MIGRATION_V6.md`;
- update README, package docs, changelog, and release notes;
- tag final v5 if needed;
- publish v6 release candidate before stable v6.0.0.

Acceptance: a v5 user can migrate using documentation without reading implementation code.

### R6.8 — Terure integration

- update Terure to `/v6`;
- make persistence adapters pass application contexts to Rainstorm;
- run Terure's complete test and race suites;
- resume Terure Phase 9.3 with context propagated to the storage boundary.

Acceptance: request cancellation reaches Rainstorm; transactional behavior remains correct; no service uses hidden background contexts.

## 13. Delegation rules

Architecture is controlled by this document. Executor LLMs may implement one subphase at a time but must not:

- redesign signatures outside the authorized phase;
- add compatibility wrappers;
- expose bbolt types again for convenience;
- weaken tests;
- claim cancellation stronger than bbolt permits;
- combine dependency upgrades with transaction changes unless explicitly requested;
- commit or push.

Every subphase requires review of the actual diff, targeted tests, full tests, and the race detector before acceptance.
