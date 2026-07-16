# Rainstorm future directions

Status: exploratory, non-normative

This document preserves architectural ideas for possible work after Rainstorm v6 is stable and Terure is operating reliably. It is not a roadmap, release commitment, or justification for expanding the v6 scope.

The decision to pursue any item requires production evidence, benchmarks, compatibility analysis, and a separate normative design.

## Decision principle

Rainstorm v6 is an evolutionary refactor focused on context propagation, safe transaction boundaries, encapsulation, compatibility, quality, and documentation. A larger redesign is not currently required.

A future redesign should be considered only if measured limitations cannot be addressed safely through incremental changes. Relevant signals include:

- reflection complexity preventing safe evolution;
- record and index atomicity becoming difficult to prove;
- metadata or schema evolution becoming fragile;
- dynamic schemas becoming unnaturally coupled to Go structs;
- query or write performance failing real workloads;
- inability to test the core without opening a physical database;
- public API constraints preventing necessary internal improvements.

## 1. Neutral record engine

A future core could operate on a representation independent of Go structs:

```go
type Record struct {
    Bucket string
    ID     Key
    Fields map[string]Value
}
```

Go structs, runtime-generated structs, and dynamic application records would map through adapters:

```text
Go struct ─┐
runtime type ├→ mapper → neutral Record → engine
dynamic data ┘
```

Potential benefits:

- first-class dynamic schema support;
- less reflection in the storage engine;
- a clearer boundary between mapping and persistence;
- easier testing of record/index behavior;
- a better fit for platforms such as Terure.

This does not imply abandoning typed struct APIs. They could remain a primary adapter.

## 2. Compiled and cached schemas

Reflection and tag analysis could produce immutable descriptors validated once and reused:

```go
type Schema struct {
    Bucket  string
    ID      Field
    Fields  []Field
    Indexes []IndexDefinition
}
```

Possible gains:

- earlier schema errors;
- less repeated reflection;
- centralized index definitions;
- consistent behavior between named and runtime-generated structs;
- improved write and query performance.

Cache ownership, invalidation, runtime type identity, memory bounds, and concurrency would require explicit design.

## 3. Unified mutation plan

All record mutations could be compiled into one internal plan:

```text
load previous record
→ validate identity and schema
→ calculate index removals
→ calculate index additions
→ validate uniqueness
→ write record and indexes atomically
```

The same engine would support:

- save;
- replace/update;
- partial field update;
- delete;
- named structs;
- runtime-generated structs;
- neutral records.

This could reduce semantic drift between mutation paths and make atomicity easier to test and reason about.

## 4. Internal storage abstraction

The core could depend on narrow internal transaction contracts rather than directly on bbolt types:

```go
type ReadTx interface {
    // internal bucket and cursor operations
}

type WriteTx interface {
    ReadTx
    // internal mutation operations
}
```

Primary goals would be isolation and testability, not a public promise of multiple storage engines.

Supporting bbolt, Badger, Pebble, or another backend publicly would require reconciling different transaction, ordering, durability, and concurrency semantics. No multi-backend commitment should be made without a concrete use case.

## 5. Index lifecycle components

Indexes could share an explicit internal lifecycle:

```go
type Index interface {
    ValidateChange(tx ReadTx, previous, next Record) error
    ApplyChange(tx WriteTx, previous, next Record) error
    Remove(tx WriteTx, record Record) error
    Lookup(tx ReadTx, value Value, options QueryOptions) ([]Key, error)
}
```

Potential benefits:

- consistent unique/list/range behavior;
- clearer rollback and cleanup tests;
- easier addition of new index types;
- centralized index integrity verification.

The exact contract must account for allocation, ordering, duplicate keys, and bbolt cursor behavior before adoption.

## 6. Query planning and explanation

A future query layer could make execution strategy explicit:

- select a usable index where possible;
- detect and optionally reject accidental full scans;
- apply limits before unnecessary decoding;
- report records examined and returned;
- expose an explain facility.

Possible API direction:

```go
plan, err := db.Select(...).Explain(ctx)
```

An explain plan must describe actual behavior rather than act as decorative metadata.

## 7. Streaming and iterators

Large result sets could use iterators rather than mandatory slice materialization:

```go
iterator, err := query.Iter(ctx)
if err != nil {
    return err
}
defer iterator.Close()

for iterator.Next() {
    // consume current record
}
if err := iterator.Err(); err != nil {
    return err
}
```

Potential gains:

- lower peak memory;
- earlier delivery of results;
- responsive cooperative cancellation.

Risks:

- long-lived read transactions;
- explicit close requirements;
- mmap lifecycle interactions;
- backpressure and callback semantics;
- misuse across goroutines.

Iterator lifecycle must be proven safe before exposing it publicly.

## 8. Neutral observability hooks

Rainstorm could expose optional instrumentation without depending directly on a telemetry vendor:

```go
type Observer interface {
    OperationFinished(OperationStats)
}
```

Useful measurements include:

- transaction wait duration;
- operation duration;
- records examined and decoded;
- index or scan strategy;
- commit and rollback outcomes;
- cancellation reason;
- encoded and decoded byte counts.

Hooks must avoid exposing record contents or secrets and must not allow observer failures to corrupt transaction behavior.

## 9. Schema catalog, migrations, and integrity

A future metadata system could distinguish:

- library version;
- on-disk format version;
- schema version;
- index version.

Possible capabilities:

- explicit migrations;
- schema and index catalog;
- resumable or transactional index rebuilding;
- integrity verification;
- orphaned index detection;
- backup/restore guidance;
- compatibility diagnostics before opening for writes.

Any format migration requires rollback guidance and fixtures from previous supported versions.

## 10. bbolt cancellation research

Rainstorm should retain official bbolt unless measurements demonstrate a concrete limitation.

A future feasibility study may examine:

- writer lock wait duration;
- read transaction effects on commit latency;
- fsync and mmap behavior;
- whether a Rainstorm-owned writer gate improves cancellation responsiveness;
- cancellation points suitable for an upstream bbolt proposal.

A permanent bbolt fork is a last resort because it transfers responsibility for crash consistency, storage compatibility, platform behavior, and security fixes to Rainstorm maintainers.

## 11. Evaluation process

After Rainstorm v6 and Terure are stable:

1. collect production traces and benchmarks;
2. identify concrete bottlenecks or maintenance hazards;
3. test whether incremental refactoring solves them;
4. create a feasibility report for remaining structural issues;
5. decide independently for each subsystem whether to retain, refactor, or replace it;
6. avoid a full rewrite unless evidence shows that staged replacement is insufficient.

A possible future document may be titled `Rainstorm v7 feasibility report`, but its existence and conclusions are intentionally undecided.
