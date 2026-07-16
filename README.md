# Rainstorm

[![Go Reference](https://pkg.go.dev/badge/github.com/AndersonBargas/rainstorm/v6.svg)](https://pkg.go.dev/github.com/AndersonBargas/rainstorm/v6)
[![CI](https://github.com/AndersonBargas/rainstorm/actions/workflows/main.yml/badge.svg)](https://github.com/AndersonBargas/rainstorm/actions/workflows/main.yml)

Rainstorm is a simple and powerful toolkit for [BoltDB](https://github.com/etcd-io/bbolt), forked from [Storm](https://github.com/asdine/storm). It provides indexes, a wide range of methods to store and fetch data, an advanced query system, and much more.

## Table of Contents

- [Installation](#installation)
- [Quick start](#quick-start)
- [Declaring models](#declaring-models)
- [Model tags](#model-tags)
- [Custom bucket names](#custom-bucket-names)
- [Opening a database](#opening-a-database)
- [CRUD operations](#crud-operations)
  - [Save](#save)
  - [Fetch by ID or field](#fetch-by-id-or-field)
  - [Fetch multiple records](#fetch-multiple-records)
  - [Query API](#query-api)
  - [Update](#update)
  - [Delete](#delete)
  - [Count, Init, Drop, ReIndex](#count-init-drop-reindex)
- [Managed transactions](#managed-transactions)
- [Nodes and nested buckets](#nodes-and-nested-buckets)
- [Key/value store](#keyvalue-store)
- [Context and cancellation](#context-and-cancellation)
- [Error handling](#error-handling)
- [Database lifecycle](#database-lifecycle)
- [Native BoltDB access](#native-boltdb-access)
- [Codecs](#codecs)
- [Compatibility with v5](#compatibility-with-v5)
- [Performance](#performance)
- [Testing and CI](#testing-and-ci)
- [License](#license)
- [Credits](#credits)

## Installation

Rainstorm v6 requires **Go 1.24** or later.

```sh
go get github.com/AndersonBargas/rainstorm/v6
```

```go
import "github.com/AndersonBargas/rainstorm/v6"
```

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/AndersonBargas/rainstorm/v6"
)

type User struct {
	ID    int    `rainstorm:"id,increment"`
	Name  string `rainstorm:"index"`
	Email string `rainstorm:"unique"`
	Age   int    `rainstorm:"index"`
}

func main() {
	ctx := context.Background()

	db, err := rainstorm.Open(ctx, "my.db")
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Println("close:", err)
		}
	}()

	// Save
	u := User{Name: "Alice", Email: "alice@example.com", Age: 30}
	if err := db.Save(ctx, &u); err != nil {
		log.Fatal(err)
	}
	fmt.Println("saved ID:", u.ID)

	// Fetch by indexed field
	var found User
	if err := db.One(ctx, "Email", "alice@example.com", &found); err != nil {
		log.Fatal(err)
	}
	fmt.Println("found:", found.Name)
}
```

## Declaring models

Rainstorm maps Go structs to BoltDB buckets. The struct name becomes the bucket name unless a `BucketNamer` implementation provides a custom name.

```go
type User struct {
	ID        int       `rainstorm:"id,increment"`
	Group     string    `rainstorm:"index"`
	Email     string    `rainstorm:"unique"`
	Name      string
	Age       int       `rainstorm:"index"`
	CreatedAt time.Time `rainstorm:"index"`
}
```

The primary key is identified by the `id` tag or — if no `id` tag is present — by a field named `ID`. It must be non-zero and consistently serializable by the configured codec. Auto-increment is available only for integer fields.

### Inline structs

Use the `inline` tag to flatten embedded structs. The container's bucket name is used.

```go
type Base struct {
	Ident string `rainstorm:"id"`
}

type User struct {
	Base      `rainstorm:"inline"`
	Group     string `rainstorm:"index"`
	Email     string `rainstorm:"unique"`
	Name      string
	CreatedAt time.Time `rainstorm:"index"`
}
```

## Model tags

| Tag          | Description |
|--------------|-------------|
| `id`         | Marks the primary key field. |
| `increment`  | Auto-increments integer fields. `increment=100` sets a starting value. |
| `index`      | Creates a list index for the field. |
| `unique`     | Creates a unique index for the field. |
| `inline`     | Flattens an embedded struct into the parent. |

All tags belong to the `rainstorm` struct-tag namespace: `` `rainstorm:"id,increment"` ``.

## Custom bucket names

### `BucketNamer` interface

Structs can implement `BucketNamer` to provide a custom bucket name:

```go
type User struct {
	ID   int    `rainstorm:"id,increment"`
	Name string `rainstorm:"index"`
}

func (u User) RainstormBucketName() string {
	return "customers"
}
```

The custom bucket name is used consistently across all list and point operations.

### Runtime-generated structs

Anonymous types created via `reflect.StructOf` can be used with `db.From()` to provide an explicit bucket name:

```go
dynType := reflect.StructOf([]reflect.StructField{
	{
		Name: "ID",
		Type: reflect.TypeOf(uint64(0)),
		Tag:  reflect.StructTag(`rainstorm:"id,increment"`),
	},
	{
		Name: "Name",
		Type: reflect.TypeOf(""),
		Tag:  reflect.StructTag(`rainstorm:"index"`),
	},
})

record := reflect.New(dynType)
record.Elem().FieldByName("Name").SetString("Dynamic Alice")
if err := db.From("dynamic_users").Save(ctx, record.Interface()); err != nil {
	log.Fatal(err)
}
```

Use the same reconstructed runtime type and explicit node path for subsequent reads and mutations.

## Opening a database

### Owned database

`Open` creates and owns the BoltDB database. `Close` shuts it down:

```go
db, err := rainstorm.Open(ctx, "my.db")
if err != nil {
	log.Fatal(err)
}
defer func() {
	if err := db.Close(); err != nil {
		log.Println("close:", err)
	}
}()
```

### Borrowed database

Use `UseDB` to hand Rainstorm an already-open `*bbolt.DB`:

```go
bDB, err := bolt.Open("bolt.db", 0600, &bolt.Options{Timeout: 10 * time.Second})
if err != nil {
	log.Fatal(err)
}
defer func() {
	if err := bDB.Close(); err != nil {
		log.Println("native close:", err)
	}
}()

db, err := rainstorm.Open(ctx, "", rainstorm.UseDB(bDB))
if err != nil {
	log.Fatal(err)
}
// DB.Close is a no-op for the borrowed native database, but still check it.
defer func() {
	if err := db.Close(); err != nil {
		log.Println("rainstorm close:", err)
	}
}()
```

`UseDB(nil)` is rejected with an error matching `ErrNilParam`.

### Options

`Open` accepts any number of `OpenOption` values:

- `rainstorm.BoltOptions(mode, *bolt.Options)` — custom BoltDB file mode and options.
- `rainstorm.Codec(codec)` — custom codec (default: JSON).
- `rainstorm.Root("bucket", ...)` — set a root bucket prefix.
- `rainstorm.UseDB(db)` — borrow an existing BoltDB connection.

## CRUD operations

Database open, CRUD, finder, query, scanner, KV, and managed-transaction operations accept `context.Context`. Lifecycle and escape-hatch methods such as `Close` and `NativeDB` do not.

### Save

```go
user := User{Name: "John", Email: "john@example.com", Age: 21}
err := db.Save(ctx, &user)
// user.ID is now set if auto-increment is configured
```

`Save` creates the bucket and indexes if needed, enforces unique constraints, and persists the record. Saving a record with a duplicate unique value returns `ErrAlreadyExists`.

### Auto-increment

```go
type Product struct {
	ID                 int    `rainstorm:"id,increment"`
	Name               string
	Count              uint64 `rainstorm:"increment"`
	IndexedCount       uint32 `rainstorm:"index,increment"`
	UniqueCount        int16  `rainstorm:"unique,increment=100"`
}

p := Product{Name: "Widget"}
db.Save(ctx, &p)
// p.ID == 1, p.Count == 1, p.IndexedCount == 1, p.UniqueCount == 100
```

### Fetch by ID or field

```go
var user User

// By indexed unique field
err := db.One(ctx, "Email", "john@example.com", &user)

// By unindexed field (full scan)
err = db.One(ctx, "Name", "John", &user)

// By primary key
err = db.One(ctx, "ID", 10, &user)
```

`One` uses the index when available, otherwise performs a full scan.

### Fetch multiple records

```go
var users []User
err := db.Find(ctx, "Group", "staff", &users)

// All records
err = db.All(ctx, &users)

// All sorted by index
err = db.AllByIndex(ctx, "CreatedAt", &users)

// Range query on indexed field
err = db.Range(ctx, "Age", 21, 30, &users)

// Prefix query; indexed fields use their index, while Name falls back to a scan.
err = db.Prefix(ctx, "Name", "Jo", &users)
```

Pagination is available via `FindOption`:

```go
err := db.Find(ctx, "Group", "staff", &users, rainstorm.Limit(10), rainstorm.Skip(5))
err = db.All(ctx, &users, rainstorm.Limit(10), rainstorm.Skip(5), rainstorm.Reverse())
```

### Query API

For complex queries, use `Select` with matchers from the `q` package:

```go
import "github.com/AndersonBargas/rainstorm/v6/q"

// Matchers
q.Eq("Name", "John")
q.Gt("Age", 7)
q.Gte("Age", 21)
q.Lt("Age", 77)
q.Lte("Age", 77)
q.In("Group", []string{"staff", "admin"})
q.Re("Name", "^Jo")
q.StrictEq("Field", value)

// Combined matchers
q.And(q.Gt("Age", 7), q.Re("Name", "^J"))
q.Or(q.Eq("Group", "staff"), q.Eq("Group", "admin"))
q.Not(q.Eq("Group", "blocked"))

// Build and execute
var users []User
err := db.Select(q.Gte("ID", 10), q.Lte("ID", 100)).
	Limit(10).Skip(5).Reverse().OrderBy("Age").
	Find(ctx, &users)

var single User
err = db.Select(q.Eq("Email", "john@example.com")).First(ctx, &single)

// Delete matching records
err = db.Select(q.Lt("Age", 18)).Delete(ctx, &User{})

// Count matching records
count, err := db.Select(q.Eq("Group", "staff")).Count(ctx, &User{})

// Iterate one by one
err = db.Select(q.Gte("ID", 10)).Each(ctx, &User{}, func(record any) error {
	u := record.(*User)
	fmt.Println(u.Name)
	return nil
})
```

### Update

```go
// Update non-zero fields
err := db.Update(ctx, &User{ID: 10, Name: "Jack", Age: 45})

// Update a single field (including zero values)
err := db.UpdateField(ctx, &User{ID: 10}, "Age", 0)
```

### Delete

```go
var user User
db.One(ctx, "ID", 10, &user)
err := db.DeleteStruct(ctx, &user)
```

### Count, Init, Drop, ReIndex

```go
// Count records
count, err := db.Count(ctx, &User{})

// Pre-create buckets and indexes
err = db.Init(ctx, &User{})

// Drop a bucket
err = db.Drop(ctx, "User")
err = db.Drop(ctx, &User{})

// Rebuild indexes
err = db.ReIndex(ctx, &User{})
```

## Managed transactions

Rainstorm v6 provides callback-managed transactions only. Manual `Begin`/`Commit`/`Rollback` APIs do not exist.

### ReadTransaction

```go
err := db.ReadTransaction(ctx, func(tx rainstorm.Node) error {
	var u User
	if err := tx.One(ctx, "ID", 1, &u); err != nil {
		return err
	}
	fmt.Println(u.Name)
	return nil
})
```

### WriteTransaction

```go
err := db.WriteTransaction(ctx, func(tx rainstorm.Node) error {
	if err := tx.Save(ctx, &accountA); err != nil {
		return err
	}
	if err := tx.Save(ctx, &accountB); err != nil {
		return err
	}
	return nil
})
```

**Semantics:**

- The callback executes exactly once (`bbolt.Update` never retries).
- If the callback returns an error, the transaction rolls back and the callback error is returned.
- If the context is cancelled before commit, the transaction rolls back and `ctx.Err()` is returned.
- If the callback cancels the context and also returns an error, the callback error remains primary.
- After a successful commit, cancellation is not retroactively applied.
- Panics propagate unchanged after bbolt unwinds the transaction.

## Nodes and nested buckets

Use `From` to work with nested buckets. Every node supports the full API:

```go
notes := db.From("notes")
privateNotes := notes.From("private")
workNotes := notes.From("work")

privateNotes.Save(ctx, &Note{ID: "p1", Text: "private"})
workNotes.Save(ctx, &Note{ID: "w1", Text: "work"})

// From accepts any number of path segments
personalNotes := notes.From("private", "personal")
```

A node can be further configured with `WithCodec`:

```go
node := db.From("data").WithCodec(gob.Codec)
```

The `Bucket()` method returns the bucket path as a `[]string`. The `BucketScanner` interface provides `PrefixScan` and `RangeScan` for discovering sub-buckets.

## Key/value store

Rainstorm can also be used as a simple key/value store:

```go
// Store
err := db.Set(ctx, "logs", time.Now(), "breakfast started")
err = db.Set(ctx, "sessions", sessionID, &someUser)

// Retrieve
var user User
err = db.Get(ctx, "sessions", sessionID, &user)

// Raw bytes
raw, err := db.GetBytes(ctx, "logs", key)
err = db.SetBytes(ctx, "cache", "key", []byte("value"))

// Existence check
exists, err := db.KeyExists(ctx, "sessions", sessionID)

// Delete
err = db.Delete(ctx, "sessions", sessionID)
```

## Context and cancellation

Database open, CRUD, finder, query, scanner, KV, and managed-transaction paths accept `context.Context`; passing nil to those context-aware operations returns an error matching `ErrNilContext`. `Close` and `NativeDB` are lifecycle/escape-hatch exceptions.

Cancellation is cooperative, not preemptive. Rainstorm checks the context at operation boundaries and loop checkpoints. It cannot interrupt:

- a filesystem syscall,
- a bbolt mutex or lock wait,
- codec code that does not observe a context,
- a user callback that has not returned control.

No abandoned goroutines are used to simulate cancellation. Published read destinations and successfully committed writes are not retroactively changed to cancellation errors.

Real applications should pass request or job contexts rather than creating a new `context.Background()` at each persistence call.

## Error handling

Always use `errors.Is` for sentinel classification, never direct equality or string comparison:

```go
var user User
if err := db.One(ctx, "ID", 999, &user); err != nil {
	if errors.Is(err, rainstorm.ErrNotFound) {
		// handle missing record
	}
	if errors.Is(err, context.Canceled) {
		// handle cancellation
	}
}
```

### Operation wrapping

Errors are wrapped with an operation label:

```
rainstorm <operation>: <cause>
```

Wrapping preserves Rainstorm sentinels, `context.Canceled`, `context.DeadlineExceeded`, bbolt errors, callback errors, and codec errors in the error chain. Callers can match the innermost cause with `errors.Is` through the wrapper.

### Sentinels

| Sentinel | Meaning |
|----------|---------|
| `ErrNotFound` | Record not found |
| `ErrAlreadyExists` | Duplicate unique value |
| `ErrNilContext` | Nil context passed to an operation |
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

Do not parse error strings. Use `errors.Is` (and `errors.As` if a typed error is introduced with structured fields).

## Database lifecycle

### Owned

When `Open` is called without `UseDB`, Rainstorm owns the native database:

- `Open` creates and opens the BoltDB file.
- Initialization failure closes only the owned database.
- `DB.Close()` closes the underlying BoltDB file.

### Borrowed

When `Open` is called with `UseDB`, Rainstorm borrows the database:

- `UseDB` accepts an already-open `*bbolt.DB`.
- `UseDB(nil)` returns an error matching `ErrNilParam`.
- Rainstorm does not close a borrowed database.
- `DB.Close()` returns nil for borrowed databases.
- The caller owns the lifecycle and must keep it open while Rainstorm is active.

## Native BoltDB access

`NativeDB()` returns the underlying `*bbolt.DB` pointer. This is an **advanced interoperability escape hatch**.

**Warnings:**

- Native operations bypass Rainstorm context checkpoints.
- Native writes can bypass codecs, indexes, metadata, and invariants.
- Rainstorm cannot guarantee index consistency or cancellation for native operations.
- Callers must coordinate native and Rainstorm transactions internally.
- Callers must not close the native database while Rainstorm is active.

Normal application code should prefer the managed Rainstorm APIs.

## Codecs

Rainstorm marshals data to BoltDB using a codec. The default is JSON. Change the codec with the `Codec` option:

```go
import "github.com/AndersonBargas/rainstorm/v6/codec/gob"

db, err := rainstorm.Open(ctx, "my.db", rainstorm.Codec(gob.Codec))
```

### Built-in codecs

| Package | Import path | Codec variable |
|---------|-------------|----------------|
| JSON (default) | `rainstorm/v6/codec/json` | `json.Codec` |
| Gob | `rainstorm/v6/codec/gob` | `gob.Codec` |
| MsgPack | `rainstorm/v6/codec/msgpack` | `msgpack.Codec` |
| Sereal | `rainstorm/v6/codec/sereal` | `sereal.Codec` |
| Protobuf | `rainstorm/v6/codec/protobuf` | `protobuf.Codec` |
| AES | `rainstorm/v6/codec/aes` | `NewAES(subCodec, key)` |

AES wraps another codec with AES-GCM encryption.

### Custom codecs

Implement the `codec.MarshalUnmarshaler` interface:

```go
type MarshalUnmarshaler interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(b []byte, v any) error
	Name() string
}
```

### Codec compatibility

Rainstorm records codec metadata for record/KV leaf buckets and an encoded database-version value used during `Open`. An incompatible root codec is classified with `ErrDifferentCodec`; metadata-validating initialization and write operations also classify a mismatched node codec this way. Read paths may instead return the underlying decode error, so nodes configured with `WithCodec` must continue using their matching codec.

## Compatibility with v5

Rainstorm v6 reads and mutates databases created by Rainstorm v5.3.0 without migration for the tested codecs:

| Codec | Fixture | Read | Mutate | Reopen |
|-------|---------|------|--------|--------|
| JSON (default) | baseline.db | ✓ | ✓ | ✓ |
| Gob | gob.db | ✓ | ✓ | ✓ |
| MsgPack | msgpack.db | ✓ | ✓ | ✓ |
| Sereal | sereal.db | ✓ | ✓ | ✓ |
| AES-JSON | aes.db | ✓ | ✓ | ✓ |

IDs, ordinary indexes, unique indexes, nested buckets, KV operations, `BucketNamer`, and `reflect.StructOf` are all covered. Same-process v5→v6→v5→v6 roundtrip is verified.

**Limitations:**

- Protobuf fixtures are excluded because the existing generated type cannot carry the required indexed fixture schema, and a generic struct would fall back to JSON.
- Process-isolated roundtrip testing is deferred.
- Compatibility evidence is behavioural, not byte-level (raw bbolt bytes are not deterministic).

See [`testdata/compatibility/README.md`](testdata/compatibility/README.md) for the full matrix.

See [`MIGRATION_V6.md`](MIGRATION_V6.md) for the complete v5.3.0 → v6 migration guide.

## Performance

Rainstorm v6 adds context propagation, operation wrapping, managed transactions, and destination safety on top of v5's foundation. Measured paired workloads under equivalent configurations (Apple M4, darwin/arm64, default JSON codec, `NoSync`) show:

- Point reads: approximately 2–3% higher ns/op.
- Bulk reads: approximately 0.4–1.3% higher ns/op.
- Save: approximately 18% higher ns/op (largest observed difference).
- KVGet: approximately 12% higher ns/op.

No paired benchmark exceeded the predefined 25% investigation threshold. These are manual medians from five samples in a same-process comparison with a shared dependency graph; no confidence interval or statistical-significance analysis was performed. Performance architecture work is deferred to a future version. See [`testdata/compatibility/benchmark/results/comparison.md`](testdata/compatibility/benchmark/results/comparison.md) for the full methodology and results.

## Testing and CI

```sh
# Formatting and build
test -z "$(gofmt -l .)"
go vet ./...
go build ./...

# Tests
go test -count=1 -timeout 180s ./...
go test -race -count=1 -timeout 300s ./...

# Staticcheck
go run honnef.co/go/tools/cmd/staticcheck@2026.1 ./...

# Coverage
go test -count=1 -timeout 180s -covermode=atomic -coverprofile=coverage.out ./...
go tool cover -func=coverage.out > coverage.txt
```

Nested compatibility modules:

```sh
go -C testdata/compatibility/roundtrip test -count=1 -timeout 180s ./...
go -C testdata/compatibility/benchmark test -count=1 ./...
```

## License

MIT

## Credits

- [Asdine El Hrychy](https://github.com/asdine)
- [Bjørn Erik Pedersen](https://github.com/bep)
