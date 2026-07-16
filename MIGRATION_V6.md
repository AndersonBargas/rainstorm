# Rainstorm v5.3.0 → v6 Migration Guide

## Table of Contents

1. [Overview](#1-overview)
2. [Who should migrate](#2-who-should-migrate)
3. [Supported starting point](#3-supported-starting-point)
4. [Executive summary of breaking changes](#4-executive-summary-of-breaking-changes)
5. [Prerequisites](#5-prerequisites)
6. [Module and import-path migration](#6-module-and-import-path-migration)
7. [Context propagation](#7-context-propagation)
8. [Open and lifecycle changes](#8-open-and-lifecycle-changes)
9. [Owned versus borrowed databases](#9-owned-versus-borrowed-databases)
10. [CRUD signature migration](#10-crud-signature-migration)
11. [KV signature migration](#11-kv-signature-migration)
12. [Finder and query migration](#12-finder-and-query-migration)
13. [Managed transaction migration](#13-managed-transaction-migration)
14. [Removal of manual transaction APIs](#14-removal-of-manual-transaction-apis)
15. [Removal of batch APIs](#15-removal-of-batch-apis)
16. [Node API changes](#16-node-api-changes)
17. [Native database access changes](#17-native-database-access-changes)
18. [Error-classification migration](#18-error-classification-migration)
19. [Nil-context versus nil-parameter behavior](#19-nil-context-versus-nil-parameter-behavior)
20. [Cancellation and callback precedence](#20-cancellation-and-callback-precedence)
21. [Destination and output safety](#21-destination-and-output-safety)
22. [Codec behavior](#22-codec-behavior)
23. [On-disk v5 compatibility](#23-on-disk-v5-compatibility)
24. [BucketNamer and dynamic runtime types](#24-bucketnamer-and-dynamic-runtime-types)
25. [Performance expectations](#25-performance-expectations)
26. [Recommended migration sequence](#26-recommended-migration-sequence)
27. [Mechanical replacement checklist](#27-mechanical-replacement-checklist)
28. [Validation checklist](#28-validation-checklist)
29. [Troubleshooting](#29-troubleshooting)
30. [Known limitations](#30-known-limitations)
31. [Rollback strategy](#31-rollback-strategy)
32. [Links to supporting evidence](#32-links-to-supporting-evidence)

---

## 1. Overview

Rainstorm v6 is a major release that introduces context propagation, managed
transactions, operation-wrapped errors, and a clarified lifecycle model. This
guide documents every known breaking API change from **v5.3.0** to **v6** and
provides mechanical before/after replacements, semantic explanations, and a
practical migration checklist.

The v6 module path is:

```
github.com/AndersonBargas/rainstorm/v6
```

The minimum supported Go version is **Go 1.24**.

---

## 2. Who should migrate

- Maintainers of applications that currently depend on
  `github.com/AndersonBargas/rainstorm/v5` (any patch release).
- Anyone who needs context propagation, managed transactions, or error
  classification in their persistence layer.
- Anyone who must stay current with the supported Rainstorm major version.

If you are pinned to v5.3.0 and have no need for the v6 features, you may
continue using v5.3.0. v5.3.0 remains published and usable.

---

## 3. Supported starting point

This guide targets migration from **Rainstorm v5.3.0**.

Earlier v5 versions (v5.0.x – v5.2.x) are not covered by the checked-in
compatibility fixtures. If you are on an earlier v5 release, upgrade to v5.3.0
first and run your application test suite before starting the v6 migration.

---

## 4. Executive summary of breaking changes

| Change | Impact |
|--------|--------|
| Database open, CRUD, finder, query-terminal, scanner, KV, and managed-transaction operations require `context.Context` | Update those context-aware call sites; lifecycle/configuration methods remain context-free |
| `Open` signature: `(path)` → `(ctx, path)` | All `Open` call sites must update |
| `DB.Bolt` public field replaced by `NativeDB()` method | All direct field accesses must change |
| Manual transaction APIs (`Begin`, `Commit`, `Rollback`, `Tx`) removed | Rewrite to `ReadTransaction` / `WriteTransaction` |
| Batch APIs (`Batch()`, `WithBatch`, `batchMode`) removed | Rewrite to `WriteTransaction` |
| `PrefixScan`/`RangeScan` now return `([]Node, error)` with context | Return value and error handling must change |
| Query terminal methods require `context.Context` | Every query chain must update |
| All errors are operation-wrapped (`rainstorm <op>: <cause>`) | Applications must use `errors.Is`, never `==` |
| `ErrNotInTransaction` removed | No direct replacement; managed transactions avoid this state |
| `UseDB` no longer closes the borrowed database | Caller must manage the native DB lifecycle |
| `Node.GetBucket`, `Node.CreateBucketIfNotExists`, `Node.WithTransaction` unexported | Use Node directly in transactions or managed APIs |
| `Node.WithBatch` removed | No direct replacement |

---

## 5. Prerequisites

1. **Go 1.24 or later** — the v6 module directive is `go 1.24.0`.
2. **Back up all production databases** before any v6 write.
3. **Verify backups** are readable by your current v5.3.0 application.
4. **Upgrade to v5.3.0** if you are on an earlier v5 release.

---

## 6. Module and import-path migration

### 6.1 Main import path

Before:

```go
import "github.com/AndersonBargas/rainstorm/v5"
```

After:

```go
import "github.com/AndersonBargas/rainstorm/v6"
```

### 6.2 Sub-package imports

The codec and query packages follow the same pattern:

Before:

```go
import (
    "github.com/AndersonBargas/rainstorm/v5/codec/gob"
    "github.com/AndersonBargas/rainstorm/v5/q"
)
```

After:

```go
import (
    "github.com/AndersonBargas/rainstorm/v6/codec/gob"
    "github.com/AndersonBargas/rainstorm/v6/q"
)
```

### 6.3 Module coexistence

v5 and v6 are different Go modules and **can** coexist in the same test binary
for compatibility verification (this is how the roundtrip suite works). However,
normal application code should migrate consistently to a single major version.
Mixing major versions in application logic is not recommended.

Automated import rewrites (e.g. `sed` on `/v5` → `/v6`) should always be
followed by compilation, as public signatures also changed.

---

## 7. Context propagation

Context propagation is the central v6 migration. Database open, CRUD, finder, query-terminal, scanner, KV, and managed-transaction operations now take `context.Context`. Configuration and lifecycle methods such as `Select`, `From`, `Bucket`, `Codec`, `WithCodec`, `Close`, and `NativeDB` remain context-free.

### 7.1 Where contexts come from

- HTTP request: `r.Context()`
- RPC: the context from the incoming request handler
- Worker/job: the context provided by the job framework
- CLI: `signal.NotifyContext(context.Background(), os.Interrupt)` or
  `context.Background()`
- Service shutdown: a context derived from the shutdown signal

Do **not** create `context.Background()` at every persistence call. Prefer
explicit context plumbing from the application boundary. A root
`context.Background()` or `context.TODO()` may be used temporarily during
migration only if labeled as transitional.

### 7.2 Representative before/after

Before (v5.3.0):

```go
db, err := rainstorm.Open("/path/to/db")
err = db.Save(&user)
err = db.One("ID", id, &user)
```

After (v6):

```go
ctx := context.Background() // or application context
db, err := rainstorm.Open(ctx, "/path/to/db")
err = db.Save(ctx, &user)
err = db.One(ctx, "ID", id, &user)
```

### 7.3 Nil context behavior

- A `nil` context returns an error matching `ErrNilContext`.
- Rainstorm does **not** silently replace `nil` with `context.Background()`.
- A `nil` required non-context argument returns an error matching `ErrNilParam`.
- Use `errors.Is` to classify:

```go
if errors.Is(err, rainstorm.ErrNilContext) {
    // caller passed nil context
}
```

---

## 8. Open and lifecycle changes

### 8.1 Open signature

Before (v5.3.0):

```go
db, err := rainstorm.Open(path, options...)
```

After (v6):

```go
db, err := rainstorm.Open(ctx, path, options...)
```

### 8.2 Open options

Open options remain structurally similar but use the defined `OpenOption` function type instead of the unnamed raw function type:

Before (v5.3.0):

```go
// Option type: func(*Options) error
db, err := rainstorm.Open(path, rainstorm.Codec(gob.Codec))
```

After (v6):

```go
// Option type: rainstorm.OpenOption
db, err := rainstorm.Open(ctx, path, rainstorm.Codec(gob.Codec))
```

### 8.3 Close behavior

Before (v5.3.0):
- `Close()` always closes the underlying `*bolt.DB`.
- `UseDB` docs warned that `Close()` would close the provided database.

After (v6):
- For an **owned** database (opened without `UseDB`): `Close()` closes the
  underlying bbolt database and returns its error.
- For a **borrowed** database (opened with `UseDB`): `Close()` returns `nil`
  and does **not** close the native database.
- If the receiver is `nil` or the underlying database is `nil`, `Close()`
  returns an error matching `ErrNilParam`.

Before (v5.3.0):
```go
db, _ := rainstorm.Open(path)
defer db.Close()
```

After (v6):
```go
db, err := rainstorm.Open(ctx, path)
if err != nil {
    // handle error; do NOT defer Close on a nil db
    return err
}
defer func() {
    if cerr := db.Close(); cerr != nil {
        // log or handle close error
    }
}()
```

---

## 9. Owned versus borrowed databases

### 9.1 Owned database

When `Open` is called **without** `UseDB`:

```go
db, err := rainstorm.Open(ctx, "/path/to/db")
```

- Rainstorm opens the bbolt database.
- `DB.Close()` closes the native database.
- If initialization fails or the context is canceled after the bbolt open,
  Rainstorm closes the owned database before returning the error.

### 9.2 Borrowed database

When `Open` is called **with** `UseDB`:

```go
bDB, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 1 * time.Second})
if err != nil {
    return err
}

db, err := rainstorm.Open(ctx, "", rainstorm.UseDB(bDB))
if err != nil {
    if closeErr := bDB.Close(); closeErr != nil {
        return errors.Join(err, closeErr)
    }
    return err
}
defer func() {
    if closeErr := db.Close(); closeErr != nil {
        log.Printf("close Rainstorm handle: %v", closeErr)
    }
    if closeErr := bDB.Close(); closeErr != nil {
        log.Printf("close borrowed bbolt database: %v", closeErr)
    }
}()
```

Rules for borrowed databases:
- `UseDB(nil)` returns an error matching `ErrNilParam`.
- Rainstorm does **not** close the borrowed database.
- `DB.Close()` returns `nil` and does not close the native database.
- The caller must keep the native database alive while Rainstorm uses it.
- The caller remains responsible for closing the native database.

---

## 10. CRUD signature migration

Every CRUD method gains `context.Context` as its first argument. The remaining
parameters are unchanged.

| v5.3.0 | v6 |
|--------|-----|
| `Init(data any) error` | `Init(ctx context.Context, data any) error` |
| `ReIndex(data any) error` | `ReIndex(ctx context.Context, data any) error` |
| `Save(data any) error` | `Save(ctx context.Context, data any) error` |
| `Update(data any) error` | `Update(ctx context.Context, data any) error` |
| `UpdateField(data any, fieldName string, value any) error` | `UpdateField(ctx context.Context, data any, fieldName string, value any) error` |
| `Drop(data any) error` | `Drop(ctx context.Context, data any) error` |
| `DeleteStruct(data any) error` | `DeleteStruct(ctx context.Context, data any) error` |

Before:

```go
err := db.Save(&user)
err = db.Update(&user)
err = db.DeleteStruct(&user)
err = db.Drop(&User{})
err = db.Init(&User{})
err = db.ReIndex(&User{})
```

After:

```go
err := db.Save(ctx, &user)
err = db.Update(ctx, &user)
err = db.DeleteStruct(ctx, &user)
err = db.Drop(ctx, &User{})
err = db.Init(ctx, &User{})
err = db.ReIndex(ctx, &User{})
```

---

## 11. KV signature migration

Every KV method gains `context.Context` as its first argument.

| v5.3.0 | v6 |
|--------|-----|
| `Get(bucket string, key, to any) error` | `Get(ctx context.Context, bucket string, key, to any) error` |
| `Set(bucket string, key, value any) error` | `Set(ctx context.Context, bucket string, key, value any) error` |
| `Delete(bucket string, key any) error` | `Delete(ctx context.Context, bucket string, key any) error` |
| `GetBytes(bucket string, key any) ([]byte, error)` | `GetBytes(ctx context.Context, bucket string, key any) ([]byte, error)` |
| `SetBytes(bucket string, key any, value []byte) error` | `SetBytes(ctx context.Context, bucket string, key any, value []byte) error` |
| `KeyExists(bucket string, key any) (bool, error)` | `KeyExists(ctx context.Context, bucket string, key any) (bool, error)` |

Before:

```go
err := db.Set("users", 1, &user)
err = db.Get("users", 1, &user)
exists, err := db.KeyExists("users", 1)
```

After:

```go
err := db.Set(ctx, "users", 1, &user)
err = db.Get(ctx, "users", 1, &user)
exists, err := db.KeyExists(ctx, "users", 1)
```

---

## 12. Finder and query migration

### 12.1 Finder methods

| v5.3.0 | v6 |
|--------|-----|
| `One(fieldName string, value, to any) error` | `One(ctx context.Context, fieldName string, value, to any) error` |
| `Find(fieldName string, value, to any, opts ...func(*index.Options)) error` | `Find(ctx context.Context, fieldName string, value, to any, opts ...FindOption) error` |
| `AllByIndex(fieldName string, to any, opts ...func(*index.Options)) error` | `AllByIndex(ctx context.Context, fieldName string, to any, opts ...FindOption) error` |
| `All(to any, opts ...func(*index.Options)) error` | `All(ctx context.Context, to any, opts ...FindOption) error` |
| `Range(fieldName string, min, max, to any, opts ...func(*index.Options)) error` | `Range(ctx context.Context, fieldName string, min, max, to any, opts ...FindOption) error` |
| `Prefix(fieldName string, prefix string, to any, opts ...func(*index.Options)) error` | `Prefix(ctx context.Context, fieldName string, prefix string, to any, opts ...FindOption) error` |
| `Count(data any) (int, error)` | `Count(ctx context.Context, data any) (int, error)` |
| `Select(matchers ...q.Matcher) Query` | `Select(matchers ...q.Matcher) Query` (unchanged) |

Before:

```go
var user User
err := db.One("ID", 1, &user)

var users []User
err = db.Find("Group", "staff", &users)
err = db.All(&users)
err = db.Range("Age", 20, 30, &users)
err = db.Prefix("Name", "J", &users)
count, err := db.Count(&User{})
```

After:

```go
var user User
err := db.One(ctx, "ID", 1, &user)

var users []User
err = db.Find(ctx, "Group", "staff", &users)
err = db.All(ctx, &users)
err = db.Range(ctx, "Age", 20, 30, &users)
err = db.Prefix(ctx, "Name", "J", &users)
count, err := db.Count(ctx, &User{})
```

### 12.2 Query terminal methods

`Select` itself does not change, but every terminal method gains
`context.Context`:

| v5.3.0 | v6 |
|--------|-----|
| `Query.Find(to any) error` | `Query.Find(ctx context.Context, to any) error` |
| `Query.First(to any) error` | `Query.First(ctx context.Context, to any) error` |
| `Query.Delete(kind any) error` | `Query.Delete(ctx context.Context, kind any) error` |
| `Query.Count(kind any) (int, error)` | `Query.Count(ctx context.Context, kind any) (int, error)` |
| `Query.Raw() ([][]byte, error)` | `Query.Raw(ctx context.Context) ([][]byte, error)` |
| `Query.RawEach(fn func([]byte, []byte) error) error` | `Query.RawEach(ctx context.Context, fn func(key, value []byte) error) error` |
| `Query.Each(kind any, fn func(any) error) error` | `Query.Each(ctx context.Context, kind any, fn func(any) error) error` |

Before:

```go
err := db.Select(q.Eq("Group", "staff")).Find(&users)
err = db.Select(q.True()).First(&user)
count, err := db.Select(q.Gt("Age", 18)).Count(&User{})
```

After:

```go
err := db.Select(q.Eq("Group", "staff")).Find(ctx, &users)
err = db.Select(q.True()).First(ctx, &user)
count, err := db.Select(q.Gt("Age", 18)).Count(ctx, &User{})
```

### 12.3 Scanner/node APIs

`PrefixScan` and `RangeScan` gain context and an error return:

| v5.3.0 | v6 |
|--------|-----|
| `PrefixScan(prefix string) []Node` | `PrefixScan(ctx context.Context, prefix string) ([]Node, error)` |
| `RangeScan(min, max string) []Node` | `RangeScan(ctx context.Context, min, max string) ([]Node, error)` |

Before:

```go
nodes := db.PrefixScan("2016")
```

After:

```go
nodes, err := db.PrefixScan(ctx, "2016")
if err != nil {
    // handle error
}
```

---

## 13. Managed transaction migration

### 13.1 Overview

v6 replaces manual `Begin`/`Commit`/`Rollback` transactions with callback-based
managed transactions. The canonical v6 forms are:

```go
err := db.ReadTransaction(ctx, func(tx rainstorm.Node) error {
    // reads through tx
    return nil
})

err = db.WriteTransaction(ctx, func(tx rainstorm.Node) error {
    // writes through tx
    return nil
})
```

### 13.2 Before/after: read transaction

Before (v5.3.0):

```go
// v5 manual read transaction
tx, err := db.Begin(false) // writable=false = read-only
if err != nil {
    return err
}
defer tx.Rollback()

if err := tx.One("ID", 1, &user); err != nil {
    return err
}

// The deferred Rollback closes the v5 read-only transaction; do not Commit it.
```

After (v6):

```go
// v6 managed read transaction
err := db.ReadTransaction(ctx, func(tx rainstorm.Node) error {
    return tx.One(ctx, "ID", 1, &user)
})
```

### 13.3 Before/after: write transaction

Before (v5.3.0):

```go
// v5 manual write transaction
tx, err := db.Begin(true)
if err != nil {
    return err
}
defer tx.Rollback()

account1.Amount -= 1000
account2.Amount += 1000

err = tx.Save(&account1)
if err != nil {
    return err
}
err = tx.Save(&account2)
if err != nil {
    return err
}

err = tx.Commit()
```

After (v6):

```go
// v6 managed write transaction
err := db.WriteTransaction(ctx, func(tx rainstorm.Node) error {
    account1.Amount -= 1000
    account2.Amount += 1000

    if err := tx.Save(ctx, &account1); err != nil {
        return err
    }
    if err := tx.Save(ctx, &account2); err != nil {
        return err
    }
    return nil
})
```

### 13.4 Managed transaction semantics

- The callback receives a transaction-bound `Node`. This node carries the
  transaction context.
- The callback runs exactly once (bbolt `Update` semantics, not bbolt `Batch`).
- All transaction work must use the callback `Node` or descendants derived from it, such as `tx.From(...)` and `tx.WithCodec(...)`; those descendants remain bound to the same transaction. Do not use the outer `DB` or a node derived from it for transactional work inside the callback, because that can attempt a nested transaction and may deadlock.
- If the callback returns an error, the write transaction is rolled back.
- If the context is canceled **before** commit, the write transaction is rolled
  back and the cancellation error is returned.
- If the context is canceled **after** a successful commit, the cancellation is
  not retroactively applied. The commit stands.
- If the callback cancels its own context and also returns a separate error, the
  callback's error is primary (the context is checked first, but a non-nil
  callback return takes precedence since bbolt already sees the error).
- Panics inside the callback propagate unchanged; bbolt rolls back the
  transaction. Rainstorm does not recover panics.
- Do **not** store or use the callback `Node` after the callback returns.
  Its transaction is already closed.

### 13.5 Porting multi-step transactions

Before (v5.3.0, manual transaction with error handling):

```go
tx, err := db.Begin(true)
if err != nil {
    return fmt.Errorf("begin: %w", err)
}
defer tx.Rollback()

if err := tx.Save(&order); err != nil {
    return fmt.Errorf("save order: %w", err)
}
if err := tx.Save(&auditLog); err != nil {
    return fmt.Errorf("save audit: %w", err)
}

return tx.Commit()
```

After (v6, managed transaction):

```go
return db.WriteTransaction(ctx, func(tx rainstorm.Node) error {
    if err := tx.Save(ctx, &order); err != nil {
        return fmt.Errorf("save order: %w", err)
    }
    if err := tx.Save(ctx, &auditLog); err != nil {
        return fmt.Errorf("save audit: %w", err)
    }
    return nil
})
```

Note: v6 operation wrapping already prefixes errors with
`rainstorm <op>:`, so additional wrapping by the caller is optional.

---

## 14. Removal of manual transaction APIs

The following v5.3.0 APIs are **removed** in v6 and have no direct replacement:

| Removed API | v5.3.0 signature | v6 replacement |
|-------------|------------------|----------------|
| `Node.Begin` | `Begin(writable bool) (Node, error)` | `ReadTransaction` / `WriteTransaction` |
| `Node.Commit` | `Commit() error` | Managed transaction commits on successful callback return |
| `Node.Rollback` | `Rollback() error` | Managed transaction rolls back on callback error or cancellation |
| `Tx` interface | `type Tx interface { Commit() error; Rollback() error }` | `TransactionManager` interface |
| `ErrNotInTransaction` | `errors.New("not in transaction")` | Removed; managed transactions eliminate this state |

Before (v5.3.0 — **will not compile in v6**):

```go
tx, err := db.Begin(true)
// ...
tx.Commit()
tx.Rollback()
```

After (v6):

```go
err := db.WriteTransaction(ctx, func(tx rainstorm.Node) error {
    // ...
    return nil
})
```

---

## 15. Removal of batch APIs

The following v5.3.0 batch-related APIs are **removed** in v6:

| Removed API | v5.3.0 usage | v6 replacement |
|-------------|-------------|----------------|
| `Batch()` option | `rainstorm.Open(path, rainstorm.Batch())` | `WriteTransaction` |
| `Node.WithBatch(bool)` | `n.WithBatch(true)` | Managed write transaction |
| `batchMode` field | (internal) | No direct equivalent |

### 15.1 Migrating grouped writes

Before (v5.3.0, batch mode):

```go
db, err := rainstorm.Open(path, rainstorm.Batch())
// All writes use bbolt Batch under the hood:
db.Save(&record1)
db.Save(&record2)
db.Save(&record3)
```

After (v6, managed write transaction):

```go
db, err := rainstorm.Open(ctx, path)

err = db.WriteTransaction(ctx, func(tx rainstorm.Node) error {
    if err := tx.Save(ctx, &record1); err != nil {
        return err
    }
    if err := tx.Save(ctx, &record2); err != nil {
        return err
    }
    if err := tx.Save(ctx, &record3); err != nil {
        return err
    }
    return nil
})
```

Managed write transactions provide explicit atomicity and error propagation.
They do **not** preserve bbolt `Batch` callback semantics. bbolt may combine concurrent `Batch` calls and may invoke an individual batch callback again when a combined batch fails and is retried separately. In v5, enabling batch mode did not make several sequential `Save` calls one atomic group; the v6 `WriteTransaction` example deliberately does. Atomicity, callback execution, concurrency, and performance characteristics therefore change and require application review.

---

## 16. Node API changes

### 16.1 Node interface

The `Node` interface changed:

| v5.3.0 `Node` member | v6 status |
|-----------------------|-----------|
| `Tx` (embedded) | Removed (manual transaction interface gone) |
| `TypeStore` | Unchanged except context |
| `KeyValueStore` | Unchanged except context |
| `BucketScanner` | Changed: methods now return `error` |
| `From(...string) Node` | Unchanged |
| `Bucket() []string` | Unchanged |
| `GetBucket(tx *bolt.Tx, children ...string) *bolt.Bucket` | **Unexported** in v6 |
| `CreateBucketIfNotExists(tx *bolt.Tx, bucket string) (*bolt.Bucket, error)` | **Unexported** in v6 |
| `WithTransaction(tx *bolt.Tx) Node` | **Unexported** in v6 |
| `Begin(bool) (Node, error)` | **Removed** |
| `Codec() codec.MarshalUnmarshaler` | Unchanged |
| `WithCodec(codec.MarshalUnmarshaler) Node` | Unchanged |
| `WithBatch(bool) Node` | **Removed** |

### 16.2 From, Bucket, Codec, WithCodec

These remain unchanged (except that operations on the resulting `Node` now
require context):

```go
notes := db.From("notes", "private")
notes.Save(ctx, &note)

name := notes.Bucket() // e.g. ["notes", "private"]
c := notes.Codec()
gobNotes := notes.WithCodec(gob.Codec)
```

### 16.3 GetBucket, CreateBucketIfNotExists, WithTransaction

These are no longer exported. In v6:

- Use the callback `Node` provided by `ReadTransaction`/`WriteTransaction` for
  transaction-scoped operations. The node carries the transaction internally.
- Use the `DB` node directly for auto-transactional single operations.
- If you need access to the raw `*bolt.Tx` for advanced use cases, use
  `NativeDB()` to obtain the `*bolt.DB` and manage transactions yourself.

---

## 17. Native database access changes

### 17.1 DB.Bolt → NativeDB()

The v5.3.0 public field `DB.Bolt` is removed in v6. It is replaced by the
`NativeDB()` method:

Before (v5.3.0):

```go
nativeDB := db.Bolt
// use nativeDB directly for advanced operations
```

After (v6):

```go
nativeDB := db.NativeDB()
// use nativeDB directly for advanced operations
```

`NativeDB()` returns the identical `*bolt.DB` pointer. If the receiver is
`nil`, `NativeDB()` returns `nil`.

### 17.2 Warnings

Native operations are an **advanced interoperability escape hatch**:

- Native operations bypass Rainstorm context checkpoints. Cancellation is
  not checked during native bbolt operations.
- Native operations are outside Rainstorm's operation wrapping. Errors from
  native operations are not wrapped with `rainstorm <op>:`.
- Native writes may bypass codecs, indexes, metadata, and invariants.
- Native writes can make Rainstorm indexes inconsistent with stored data.
- Native and Rainstorm transactions must be coordinated by the caller. bbolt
  allows only one read-write transaction at a time.
- Do **not** close the native `*bolt.DB` while Rainstorm is in use.
- Do **not** close a native database managed by Rainstorm (owned); use
  `DB.Close()` instead.
- Prefer Rainstorm managed APIs (`ReadTransaction`, `WriteTransaction`,
  individual CRUD/KV methods) for application code.

---

## 18. Error-classification migration

### 18.1 Direct equality → errors.Is

v6 wraps every public error with operation context:

```
rainstorm <operation>: <cause>
```

This means direct sentinel equality (`==`) no longer works. Applications must
use `errors.Is`:

Incorrect (v5 style — **does not work in v6**):

```go
if err == rainstorm.ErrNotFound {
    // will NOT match in v6
}
```

Correct (v6):

```go
if errors.Is(err, rainstorm.ErrNotFound) {
    // correctly matches wrapped errors
}
```

### 18.2 Error wrapping

Operation wrapping preserves all underlying sentinels:

```
rainstorm save: already exists
rainstorm one: not found
rainstorm open: context canceled
```

Nested operation wrapping is allowed. The outermost public operation label
remains visible.

Applications **must not** parse error strings. Error text is not a stable API.
Applications **should not** depend on exact full error text.

### 18.3 Sentinels

| Sentinel | v5.3.0 | v6 status |
|----------|--------|-----------|
| `ErrNotFound` | Local to `rainstorm` | Shared with `index` package; `errors.Is` works across both |
| `ErrAlreadyExists` | Local to `rainstorm` | Shared with `index` package; `errors.Is` works across both |
| `ErrNilParam` | Local to `rainstorm` | Shared with `index` package |
| `ErrNilContext` | **Did not exist** | New in v6; returned when `ctx` is `nil` |
| `ErrNotInTransaction` | `errors.New("not in transaction")` | **Removed** (no transaction state to be outside of) |
| `ErrNoID` | Present | Unchanged |
| `ErrZeroID` | Present | Unchanged |
| `ErrBadType` | Present | Unchanged |
| `ErrUnknownTag` | Present | Unchanged |
| `ErrIdxNotFound` | Present | Unchanged |
| `ErrSlicePtrNeeded` | Present | Unchanged |
| `ErrStructPtrNeeded` | Present | Unchanged |
| `ErrPtrNeeded` | Present | Unchanged |
| `ErrNoName` | Present | Unchanged |
| `ErrIncompatibleValue` | Present | Unchanged |
| `ErrDifferentCodec` | Present | Classifiable with `errors.Is`; version decode/decryption failures preserve their underlying cause |

### 18.4 Error-migration table

| v5 pattern | v6 replacement |
|------------|---------------|
| `err == rainstorm.ErrNotFound` | `errors.Is(err, rainstorm.ErrNotFound)` |
| `err == rainstorm.ErrAlreadyExists` | `errors.Is(err, rainstorm.ErrAlreadyExists)` |
| `err == rainstorm.ErrNotInTransaction` | Remove; managed transactions eliminate this state |
| `err.Error() == "not found"` | Never use string comparison; use `errors.Is` |
| `strings.Contains(err.Error(), "not found")` | Never use string matching; use `errors.Is` |
| `err == context.Canceled` | `errors.Is(err, context.Canceled)` |

---

## 19. Nil-context versus nil-parameter behavior

| Condition | v6 behavior | Sentinel |
|-----------|------------|----------|
| `ctx` is `nil` | Returns error, operation not attempted | `ErrNilContext` |
| Required non-context argument is `nil` | Returns error, operation not attempted | `ErrNilParam` |
| `fn` is `nil` in managed transactions | Returns error | `ErrNilParam` |
| `UseDB(nil)` | Returns error during `Open` | `ErrNilParam` |
| `Close()` on nil receiver | Returns error, no panic | `ErrNilParam` |

Rainstorm does **not** silently substitute `context.Background()` for a nil
context. Callers must always provide a valid context.

---

## 20. Cancellation and callback precedence

### 20.1 Cooperative cancellation

Rainstorm makes best-effort cancellation checks at explicit boundaries:

- Before a bbolt transaction is acquired.
- After a bbolt transaction is acquired (before the callback runs).
- During loops (index scans, query iterations, reindex loops).
- Before commit (writes).
- After a successful read (reads do not retroactively change published data).

Rainstorm **cannot**:

- Interrupt an OS syscall.
- Interrupt a bbolt lock wait.
- Preempt codec code (marshal/unmarshal).
- Preempt a user callback that has not returned control.
- Use abandoned goroutines or timers to simulate preemption.

Callbacks remain responsible for observing context during long-running
application work.

### 20.2 Cancellation timing

- A **successful commit** is not retroactively reported as canceled.
- A **fully published read destination** is not retroactively changed.
- Immediate cancellation latency is not guaranteed.

### 20.3 Callback error vs context cancellation

In a `WriteTransaction`:

1. Rainstorm checks context before the bbolt callback.
2. If the context is already canceled, the callback is not called.
3. If the callback runs and returns an error, that error is returned.
4. If the callback succeeds but the context was canceled during the callback,
   the transaction is rolled back and the cancellation error is returned.
5. If commit succeeds, later cancellation does not undo it.

---

## 21. Destination and output safety

v6 provides stronger safety guarantees for destination values on error:

| Operation | Error behavior |
|-----------|---------------|
| Point reads (`One`, `First`, `Get`) | Destination is **unchanged** on error |
| List reads (`Find`, `All`, `AllByIndex`, `Range`, `Prefix`, `Query.Find`) | Destination is **not partially published** on error |
| `Count`, `Query.Count` | Returns `(0, error)` on failure |
| `KeyExists` | Returns `(false, error)` on failure |
| `GetBytes` | Returns `(nil, error)` on failure |
| Byte/scanner/raw result operations | Returns `(nil, error)` on failure where documented |

Do **not** consume output values when the returned error is non-nil, unless an
API explicitly documents otherwise.

---

## 22. Codec behavior

### 22.1 Default codec

Default is JSON (unchanged from v5).

### 22.2 Setting a codec

Before (v5.3.0):

```go
db, err := rainstorm.Open(path, rainstorm.Codec(gob.Codec))
```

After (v6):

```go
db, err := rainstorm.Open(ctx, path, rainstorm.Codec(gob.Codec))
```

### 22.3 Codec metadata and mismatch classification

Rainstorm records codec metadata in record and KV leaf buckets. Structural parent buckets need not carry a marker, and `WithCodec` allows different nodes to select different codecs. Rainstorm also stores an encoded database-version value used during `Open`.

Opening a database whose version value cannot be decoded by the selected root codec returns an error matching `ErrDifferentCodec`. Operations that create or validate leaf metadata, such as initialization and writes, also return `ErrDifferentCodec` when the selected node codec disagrees with the stored marker:

```go
if errors.Is(err, rainstorm.ErrDifferentCodec) {
    // verify the root Open codec and any node-specific WithCodec selection
}
```

When `Open` fails because decoding the database-version value fails, v6 joins `ErrDifferentCodec` with the underlying decode/decryption error, so both remain reachable through `errors.Is`. A leaf-bucket marker mismatch may return `ErrDifferentCodec` without an additional codec cause because no decode was attempted.

For the tested AES fixtures, opening with the wrong key returns `ErrDifferentCodec` while the underlying decryption error is preserved.

### 22.4 Codec compatibility

The following codecs are tested compatible with v5.3.0 databases:

- JSON (default)
- Gob
- MsgPack
- Sereal
- AES-JSON

Protobuf compatibility is **excluded** — the existing generated type cannot
carry the required indexed fixture schema, and a generic struct would fall back
to JSON, which would not prove protobuf wire compatibility.

Custom codecs are not universally guaranteed compatible. Test with your
specific codec against a copy of production data before rollout.

---

## 23. On-disk v5 compatibility

### 23.1 Evidence-backed conclusions

Based on the checked-in compatibility fixtures and roundtrip tests:

- **No migration is required** for databases created by v5.3.0 with the tested
  codecs.
- v6 can **read** and **mutate** tested v5.3.0 data.
- Auto-increment IDs continue from v5 state (the next ID is the same as v5
  would have assigned).
- Ordinary and unique indexes remain usable.
- Updates and deletes clean indexes correctly (v6 removes old index entries
  and adds new ones, same as v5).
- Nested buckets and KV operations remain compatible.
- `BucketNamer` custom buckets remain compatible.
- The tested `reflect.StructOf` compatibility pattern reconstructs the same field names, types, order, and tags and uses the same explicit `From(...)` path.
- Close/reopen after v6 mutations remains readable.
- Same-process v5→v6→v5→v6 bidirectional roundtrip is verified.

### 23.2 Limitations

- Process-isolated roundtrip is not currently proven (the roundtrip test uses
  a single test binary with shared MVS dependency resolution).
- Compatibility evidence targets v5.3.0 specifically.
- Custom codecs and arbitrary historical schemas require application-specific
  testing.
- Protobuf fixture compatibility is excluded.
- Byte-for-byte deterministic files are **not** guaranteed (raw bbolt bytes
  are not deterministic across runs).
- **Backups are still required** before production rollout.

---

## 24. BucketNamer and dynamic runtime types

### 24.1 BucketNamer interface

The `BucketNamer` interface is unchanged in v6:

```go
type BucketNamer interface {
    RainstormBucketName() string
}
```

Custom bucket resolution is consistent. The v6 list-sink defect (which
affected `BucketNamer` records in list-producing operations) was fixed before
release.

Application migration does **not** require renaming existing v5 custom buckets.
Named types should continue implementing `RainstormBucketName()` consistently.

### 24.2 Runtime-generated structs (reflect.StructOf)

The compatibility suite proves the following conservative pattern for reading records created by v5.3.0 with `reflect.StructOf`:

1. Reconstruct the struct with the same field names, types, order, and struct tags used by the tested schema.
2. Use the same explicit `From(...)` path that was used to write them.
3. Do **not** wrap the anonymous record inside an arbitrary interface field.
4. Verify reads succeed before enabling writes.

Example pattern for reconstructing a runtime type:

```go
// Reconstruct the same shape used during v5 writes.
fields := []reflect.StructField{
    {
        Name: "ID",
        Type: reflect.TypeOf(int(0)),
        Tag:  reflect.StructTag(`rainstorm:"id,increment"`),
    },
    {
        Name: "Value",
        Type: reflect.TypeOf(""),
        Tag:  reflect.StructTag(`rainstorm:"unique"`),
    },
}
rt := reflect.StructOf(fields)

// Build a pointer to a slice of the reconstructed runtime type.
records := reflect.New(reflect.SliceOf(rt)).Interface()

// Use the same explicit node path as v5 and verify reads before writing.
node := db.From("custom", "path")
err := node.All(ctx, records)
```

Identical `reflect.StructOf` shapes may resolve to the same canonical Go type
when created in the same process. This is a Go runtime property, not a
Rainstorm behavior.

---

## 25. Performance expectations

Performance data is based on checked-in benchmark evidence only. Summary:

| Metric | Observation (v6 vs v5.3.0) |
|--------|---------------------------|
| Point reads (OneByID) | +2.1% to +2.5% ns/op |
| Point reads (OneByUnique) | +2.5% to +2.7% ns/op |
| Bulk reads (Find, All) | +0.4% to +1.3% ns/op |
| Save | +18.2% ns/op (largest observed difference) |
| Update | +1.9% to +2.8% ns/op |
| KVGet | +11.7% ns/op |
| KVSet | +2.2% ns/op |

**Methodology and limitations:**

- Environment: Apple M4, darwin/arm64, Go 1.26, default JSON codec, `NoSync`.
- Same-process, shared-MVS comparison (both v5 and v6 share
  `go.etcd.io/bbolt v1.4.3`).
- Five samples per benchmark, manual medians.
- No confidence interval or statistical-significance analysis.
- No causal attribution (these are end-to-end benchmarks; the cost of
  individual v6 features was not isolated).

**No paired benchmark exceeded the predefined 25% investigation threshold.**
This does not mean "no regression"; it means the threshold was not crossed in
these measurements.

Application-specific benchmarking is strongly recommended. Performance
architecture work is deferred to v7.

See [`testdata/compatibility/benchmark/results/comparison.md`](testdata/compatibility/benchmark/results/comparison.md)
for the full methodology and results.

---

## 26. Recommended migration sequence

1. **Upgrade toolchain** to Go 1.24 or later.
2. **Back up** all production databases. Verify backups.
3. **Add application-level context plumbing.** Identify where contexts enter
   your application (HTTP handler, RPC handler, worker main loop, CLI main)
   and thread them through to persistence calls.
4. **Update module and import paths**: `go.mod` require v6, all imports from
   `v5` to `v6`.
5. **Update `Open` calls**: add `ctx` as the first argument.
6. **Update CRUD, KV, query, and scanner call signatures**: add `ctx` as the
   first argument to every I/O call.
7. **Replace manual transactions and batch APIs** with
   `ReadTransaction`/`WriteTransaction`.
8. **Replace `db.Bolt` field access** with `db.NativeDB()` or prefer managed
   Rainstorm APIs.
9. **Migrate error checks** to `errors.Is`.
10. **Review lifecycle ownership**: ensure borrowed databases are not closed
    by `DB.Close()` and that the caller closes them at the right point.
11. **Run compatibility tests** against a copy of production data.
12. **Run race tests** (`go test -race`) and your application's benchmarks.
13. **Deploy to a staging environment** with real workload patterns.
14. **Roll out gradually** (canary, then progressively).
15. **Preserve rollback backups** until the rollout is confirmed stable.

---

## 27. Mechanical replacement checklist

| Search pattern | Replacement / Action | Risk |
|---------------|---------------------|------|
| `/rainstorm/v5` | `/rainstorm/v6` | Low: mechanical, but verify compilation |
| `rainstorm.Open(path` | `rainstorm.Open(ctx, path` | Low: adds param |
| `.Save(` | `.Save(ctx, ` | **High**: may affect non-Rainstorm `Save` methods; compile after each family |
| `.Update(` | `.Update(ctx, ` | **High**: same risk |
| `.One(` | `.One(ctx, ` | **High**: same risk |
| `.Find(` | `.Find(ctx, ` | **High**: same risk |
| `.All(` | `.All(ctx, ` | **High**: same risk |
| `.AllByIndex(` | `.AllByIndex(ctx, ` | **High**: same risk |
| `.Count(` | `.Count(ctx, ` | **High**: same risk |
| `.Range(` | `.Range(ctx, ` | **High**: same risk |
| `.Prefix(` | `.Prefix(ctx, ` | **High**: same risk |
| `.Get(` | `.Get(ctx, ` | **High**: same risk |
| `.Set(` | `.Set(ctx, ` | **High**: same risk |
| `.Delete(` | `.Delete(ctx, ` | **High**: same risk |
| `.GetBytes(` | `.GetBytes(ctx, ` | **High**: same risk |
| `.SetBytes(` | `.SetBytes(ctx, ` | **High**: same risk |
| `.KeyExists(` | `.KeyExists(ctx, ` | **High**: same risk |
| `.Drop(` | `.Drop(ctx, ` | **High**: same risk |
| `.Init(` | `.Init(ctx, ` | **High**: same risk |
| `.ReIndex(` | `.ReIndex(ctx, ` | **High**: same risk |
| `.DeleteStruct(` | `.DeleteStruct(ctx, ` | **High**: same risk |
| `.UpdateField(` | `.UpdateField(ctx, ` | **High**: same risk |
| `.PrefixScan(` | `.PrefixScan(ctx, ` | **High**: also add error handling |
| `.RangeScan(` | `.RangeScan(ctx, ` | **High**: also add error handling |
| `db.Bolt` | `db.NativeDB()` | Medium: method call vs field access |
| `== rainstorm.Err` (any sentinel) | `errors.Is(err, rainstorm.Err...)` | Low: mechanical, but must import `errors` |
| `db.Begin(false)` | `db.ReadTransaction(ctx, func(tx rainstorm.Node) error { ... })` | **High**: requires structural rewrite |
| `db.Begin(true)` | `db.WriteTransaction(ctx, func(tx rainstorm.Node) error { ... })` | **High**: requires structural rewrite |
| `tx.Commit()` | (remove; callback return nil) | **High**: part of transaction rewrite |
| `tx.Rollback()` | (remove; callback return error) | **High**: part of transaction rewrite |
| `rainstorm.Batch()` | (remove; use `WriteTransaction`) | **High**: requires structural rewrite |
| `.WithBatch(` | (remove; use `WriteTransaction`) | **High**: requires structural rewrite |
| Query terminal calls such as `query.Find(...)` and `db.Select(...).First(...)` | Add `ctx` to the terminal method only | **High**: chained calls and generic method names require AST/compiler-guided review |

**Warning:** Naive text replacement can affect callbacks, helper wrappers,
tests, and unrelated methods with the same names. Require compilation after
each family of changes.

---

## 28. Validation checklist

After migration, verify:

- [ ] `go build ./...` succeeds.
- [ ] `go vet ./...` passes.
- [ ] `go test -race -count=1 ./...` passes.
- [ ] No direct sentinel equality (`== rainstorm.Err...`) remains.
- [ ] No `db.Bolt` field access remains.
- [ ] No manual `Begin`/`Commit`/`Rollback` calls remain.
- [ ] No `rainstorm.Batch()` or `.WithBatch()` calls remain.
- [ ] No `db.Begin(false)` used as a read transaction (use `ReadTransaction`).
- [ ] All `Open` calls include context.
- [ ] All I/O calls include context from the appropriate application boundary.
- [ ] Borrowed database lifecycle: caller closes the native `*bolt.DB`.
- [ ] `DB.Close()` return value is checked for owned databases.
- [ ] All error classification uses `errors.Is`.
- [ ] Production database backup is verified readable.

---

## 29. Troubleshooting

### 29.1 Compile errors after adding context

**Symptom:** `too many arguments in call to db.Save`.

**Cause:** The context is being passed but other non-Rainstorm methods share the
same name (e.g. a local `Save` helper).

**Fix:** Use qualified calls or rename local helpers. Compile after each method
family.

### 29.2 Nil context returns ErrNilContext

**Symptom:** `rainstorm save: nil context` or `errors.Is(err, ErrNilContext)`
is true.

**Cause:** A `nil` context was passed. Rainstorm does not replace `nil` with
`context.Background()`.

**Fix:** Pass `context.Background()` (temporarily) or, preferably, plumb the
application context through.

### 29.3 Operation error text changed

**Symptom:** Error messages now include `rainstorm <op>:` prefix.

**Cause:** v6 wraps all errors with operation context.

**Fix:** Use `errors.Is` for classification. Remove any string-based error
matching.

### 29.4 Direct sentinel comparison no longer works

**Symptom:** `if err == rainstorm.ErrNotFound` no longer matches.

**Cause:** Errors are now wrapped: `rainstorm one: not found`.

**Fix:** Use `errors.Is(err, rainstorm.ErrNotFound)`.

### 29.5 Manual transaction symbols missing

**Symptom:** `undefined: rainstorm.Tx`, `db.Begin undefined`.

**Cause:** Manual transaction APIs were removed in v6.

**Fix:** Rewrite to `db.ReadTransaction` / `db.WriteTransaction`.

### 29.6 `db.Bolt` missing

**Symptom:** `db.Bolt undefined (type *rainstorm.DB has no field or method Bolt)`.

**Cause:** The public field was replaced by `NativeDB()`.

**Fix:** Use `db.NativeDB()` or, preferably, use managed Rainstorm APIs.

### 29.7 Borrowed DB unexpectedly remains open after DB.Close

**Symptom:** The native `*bolt.DB` is still open after calling `db.Close()`.

**Cause:** `DB.Close()` returns `nil` for borrowed databases and does not close
the native database.

**Fix:** The caller must close the native `*bolt.DB` explicitly. See
[Section 9.2](#92-borrowed-database).

### 29.8 Wrong codec returns ErrDifferentCodec

**Symptom:** `errors.Is(err, rainstorm.ErrDifferentCodec)` is true.

**Cause:** The selected root codec may be unable to decode the stored database-version value, or a metadata-validating initialization/write may detect that a node's selected codec disagrees with the record/KV leaf marker. Read paths do not uniformly validate leaf markers; a wrong node codec may instead produce a codec-specific decode error.

**Fix:** Open with the correct root codec and apply the matching `WithCodec` selection to nodes that historically used a different codec. Re-create the database with a new codec only through an explicit, backed-up data migration.

### 29.9 AES wrong key

**Symptom:** Opening an AES-encrypted v5.3.0 database with v6 using the wrong
key returns `ErrDifferentCodec`.

**Cause:** The AES decryption failed, and Rainstorm classifies this as a
codec incompatibility.

**Fix:** Use the correct AES key. The underlying decryption error is preserved
inside the error chain.

### 29.10 Runtime-generated type reads return no records

**Symptom:** `All`, `Find`, or `One` returns no records for a
`reflect.StructOf` type that has data in v5.3.0.

**Cause:** Common causes include using a different `From(...)` path or reconstructing a runtime shape that the selected codec cannot decode compatibly. Rainstorm's evidence covers the canonical same-shape reconstruction pattern; it does not prove that every shape difference must fail.

**Fix:** Start with the tested pattern: reconstruct identical fields, types, order, and tags, use the same `From(...)` path, and verify reads before enabling writes.

### 29.11 BucketNamer records not found

**Symptom:** Custom bucket records written by v5.3.0 are not found in v6.

**Cause:** The `RainstormBucketName()` implementation may return a different
value, or the Node path may differ.

**Fix:** Ensure `RainstormBucketName()` returns the same string as in v5.
Ensure the same Node path is used.

### 29.12 Cancellation is not immediate

**Symptom:** After canceling a context, the operation continues for a
noticeable duration.

**Cause:** Cooperation-based cancellation checks occur at Rainstorm operation
boundaries, not preemptively. bbolt lock waits, OS syscalls, codec operations,
and user callbacks are not preempted.

**Fix:** This is expected behavior. Ensure callbacks observe context during
long-running application work (e.g. within `Each` / `RawEach` callbacks).

### 29.13 Callback error versus context cancellation

**Symptom:** In a `WriteTransaction`, if the callback returns an error and
the context is also canceled, only one error is returned.

**Cause:** Once the callback runs and returns a non-nil error, Rainstorm preserves that callback error as the primary cause and rolls the transaction back, even if the callback also canceled the context.

**Fix:** If you need both errors, handle cancellation explicitly inside the
callback.

### 29.14 Performance differences

**Symptom:** v6 operation latency is higher than v5.3.0.

**Cause:** v6 adds context checks, operation wrapping, and destination safety,
which add some per-operation overhead. Measured differences are documented in
[Section 25](#25-performance-expectations).

**Fix:** Profile your specific workload. The predefined 25% investigation
threshold was not crossed by any paired benchmark, but application-specific
patterns may differ.

### 29.15 v5 database backup/recovery

**Symptom:** Need to revert from v6 to v5.

**Cause:** Rollback is required.

**Fix:** See [Section 31](#31-rollback-strategy). Restore the backup taken
before the first v6 write and deploy the v5 binary.

---

## 30. Known limitations

1. **Protobuf compatibility** is excluded from the compatibility suite.
2. **Process-isolated roundtrip** is not currently proven (the roundtrip suite
   uses a single test binary with shared MVS dependency graph).
3. **Byte-for-byte deterministic files** are not guaranteed by bbolt.
4. **Custom codecs** are not universally guaranteed compatible — test with
   your specific codec and schema.
5. **Performance benchmarks** are single-machine, NoSync, five-sample
   snapshots. They are not permanent performance guarantees.
6. **v5.3.0 is the only supported starting version** — earlier v5 releases
   are not covered by compatibility fixtures.
7. **Go 1.24 is the minimum** — Go versions below 1.24 are not supported.

---

## 31. Rollback strategy

A production rollback plan must include:

1. **Back up the database before the first v6 write.** This is the rollback
   baseline.
2. **Validate backups** by opening them with v5.3.0 and running application
   smoke tests.
3. **Test rollback in staging** before production rollout. Perform a v6
   write, restore the backup, and verify v5.3.0 can read it.
4. **Preserve the v5 binary and configuration** for rapid redeployment.
5. **Avoid concurrent v5 and v6 writers** against the same database file.
   Both processes may open the file read-only concurrently, but only one
   writer should be active at a time.
6. **Close one process before opening the other** when switching versions
   on the same database file.
7. **Same-process compatibility tests** (the roundtrip suite) are evidence
   of on-disk format compatibility, not a recommendation to run both
   application versions concurrently in production.
8. **After v6 writes**, tested v5.3.0 roundtrip compatibility exists, but
   production rollback still requires backups and application-level
   verification.
9. **Native writes outside Rainstorm guarantees** complicate rollback because
   indexes may not reflect native changes. If you used `NativeDB()` to perform
   writes, verify index consistency before relying on Rainstorm finders after
   rollback.

Rollback is **not risk-free**. Always test on a staging copy first.

---

## 32. Links to supporting evidence

- [`README.md`](README.md) — v6 usage, API reference, and examples
- [`docs/v6-architecture-plan.md`](docs/v6-architecture-plan.md) — design
  decisions, context contract, transaction model
- [`docs/dependency-audit.md`](docs/dependency-audit.md) — dependency policy,
  CI, coverage enforcement
- [`testdata/compatibility/README.md`](testdata/compatibility/README.md) —
  compatibility suite, fixture matrix, roundtrip details
- [`testdata/compatibility/benchmark/results/comparison.md`](testdata/compatibility/benchmark/results/comparison.md) —
  full benchmark methodology and results
