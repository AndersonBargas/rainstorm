# Rainstorm v6 Compatibility Suite

## Purpose

This directory contains the R6.5 compatibility suite that proves Rainstorm v6
can read, mutate, and coexist with databases created by Rainstorm v5.3.0. The
suite verifies on-disk format, codec, index, BucketNamer, runtime-struct, and
roundtrip compatibility across major versions.

Normative source: `docs/v6-architecture-plan.md` sections 8.2 and 10.3.

## Directory Layout

```
testdata/compatibility/
├── README.md                          ← this file
├── v5.3.0/
│   ├── baseline.db                    ← canonical v5.3.0 fixture (JSON codec)
│   ├── manifest.json                  ← semantic fixture manifest
│   ├── codecs/
│   │   ├── manifest.json              ← codec fixture manifest
│   │   ├── gob.db
│   │   ├── msgpack.db
│   │   ├── sereal.db
│   │   └── aes.db
│   └── generator/
│       ├── go.mod                     ← isolated module: requires v5.3.0 only
│       ├── go.sum
│       ├── main.go
│       └── README.md                  ← generation instructions
├── roundtrip/
│   ├── go.mod                         ← imports both v5 and v6 (nested module)
│   ├── go.sum
│   ├── roundtrip_test.go
│   └── README.md
└── benchmark/
    ├── go.mod                         ← imports both v5 and v6 (nested module)
    ├── go.sum
    ├── benchmark_test.go
    ├── README.md
    └── results/
        ├── comparison.md              ← detailed analysis
        ├── environment.txt
        ├── v5.txt                     ← 5 raw samples
        ├── v6.txt                     ← 5 raw samples
        └── v6_only.txt                ← v6-only benchmarks
```

## Source Fixture Version

- Module: `github.com/AndersonBargas/rainstorm/v5`
- Tag: `v5.3.0`
- Generator directory: `v5.3.0/generator/`
- The main v6 module does **not** import v5.
- All v5 imports are isolated in the nested `generator/`, `roundtrip/`, and
  `benchmark/` modules.

## Fixture Regeneration

```sh
cd testdata/compatibility/v5.3.0/generator
go run .
```

The generator writes `../baseline.db` and `../codecs/*.db`. After regeneration,
review the manifests and run the full compatibility test suite. Behavioral tests
are the primary proof; the structural test inspects only codec markers and raw
record presence, not exact byte equality.

**Raw bbolt on-disk bytes are not deterministic across runs.** Compatibility is
verified through the manifest and behavioral tests, not byte-level comparison.

**Fixtures must be copied before mutation.** All main-module compatibility tests
copy fixture files to `t.TempDir()` before opening them. The checked-in fixtures
are never directly mutated.

## Main Compatibility Test Commands

```sh
# All main-module tests (includes compatibility suites)
go test -count=1 -timeout 180s ./...
go test -race -count=1 -timeout 300s ./...
```

The root `go test ./...` excludes nested modules. CI invocation belongs to R6.6.

## Roundtrip Command

```sh
go -C testdata/compatibility/roundtrip test -count=1 -timeout 180s ./...
go -C testdata/compatibility/roundtrip test -race -count=1 -timeout 300s ./...
```

## Benchmark Commands

```sh
# Smoke test
go -C testdata/compatibility/benchmark test -run '^$' -bench . -benchtime=100ms -count=1

# Full collection (v5, v6, v6-only)
go -C testdata/compatibility/benchmark test -run '^$' -bench '^BenchmarkV5/' -benchmem -benchtime=500ms -count=5 -timeout 600s > testdata/compatibility/benchmark/results/v5.txt
go -C testdata/compatibility/benchmark test -run '^$' -bench '^BenchmarkV6/' -benchmem -benchtime=500ms -count=5 -timeout 600s > testdata/compatibility/benchmark/results/v6.txt
go -C testdata/compatibility/benchmark test -run '^$' -bench '^BenchmarkV6_' -benchmem -benchtime=500ms -count=5 -timeout 300s > testdata/compatibility/benchmark/results/v6_only.txt
```

See `benchmark/results/comparison.md` for the detailed report.

## Compatibility Matrix

| Scenario                                      | Status                                        |
|-----------------------------------------------|-----------------------------------------------|
| default JSON: v5 fixture → v6 read            | Verified                                       |
| named records                                 | Verified                                       |
| named records: v5 fixture → v6 mutate/reopen  | Verified                                       |
| increment IDs                                 | Verified                                       |
| ordinary indexes                              | Verified                                       |
| unique indexes                                | Verified                                       |
| duplicate rejection and atomicity             | Verified                                       |
| update and index movement                     | Verified                                       |
| delete and index cleanup                      | Verified                                       |
| unique-value reuse                            | Verified                                       |
| nested buckets                                | Verified                                       |
| root/nested isolation                         | Verified                                       |
| root and nested KV                            | Verified                                       |
| metadata/version acceptance                   | Verified                                       |
| close/reopen after v6 writes                  | Verified                                       |
| BucketNamer: custom bucket used               | Verified                                       |
| BucketNamer: list-producing operations        | Verified (includes newListSink fix)            |
| reflect.StructOf: explicit node               | Verified                                       |
| reflect.StructOf: node root as data           | Verified                                       |
| reflect.StructOf: IDs, indexes, unique        | Verified (via explicit + root-data mutation)   |
| reflect.StructOf: update/delete/index cleanup | Verified                                       |
| reflect.StructOf: unique reuse                | Verified                                       |
| reflect.StructOf: close/reopen persistence    | Verified (runtime root-data mutation)          |
| Gob: read, IDs, indexes, KV, struct evidence  | Verified                                       |
| Gob: v6 mutate/index movement/reopen          | Verified                                       |
| MsgPack: read, IDs, indexes, KV               | Verified                                       |
| MsgPack: v6 mutate/index movement/reopen      | Verified                                       |
| Sereal: read, IDs, indexes, KV                | Verified                                       |
| Sereal: v6 mutate/index movement/reopen       | Verified                                       |
| AES-JSON: read, IDs, indexes, KV              | Verified                                       |
| AES-JSON: v6 mutate/index movement/reopen     | Verified                                       |
| AES: wrong key → ErrDifferentCodec            | Verified                                       |
| AES: correct key still reads after wrong key  | Verified                                       |
| AES: fixed key is test-only                   | Verified                                       |
| Incompatible codec classification             | Verified                                       |
| Codec raw record bytes for ID 1              | Structurally verified as non-empty and not all identical |
| Codec metadata markers (gob/msgpack/sereal/aes-json) | Structurally verified                  |
| Protobuf                                      | Excluded: existing generated type cannot carry required indexed fixture schema; generic struct would fall back to JSON, which would not prove protobuf wire compatibility |
| v5 → v6 → v5 → v6 bidirectional roundtrip     | Verified in same-process cross-major module    |
| v5 → v6 benchmarks                            | Verified: same-process, equivalent workloads   |
| Process-isolated roundtrip                    | Deferred; revisit if dependency/toolchain divergence requires it |

## Codec Matrix

| Codec     | Fixture | Metadata Marker | Read | Indexes | KV | Mutate | Reopen | Incompatible Detection | Wrong Key | Raw Evidence |
|-----------|---------|-----------------|------|---------|-----|--------|--------|----------------------|-----------|-------------|
| Gob       | gob.db  | `gob`           | ✓    | ✓       | ✓   | ✓      | ✓      | ✓                    | N/A       | Structurally verified |
| MsgPack   | msgpack.db | `msgpack`    | ✓    | ✓       | ✓   | ✓      | ✓      | ✓                    | N/A       | Structurally verified |
| Sereal    | sereal.db | `sereal`     | ✓    | ✓       | ✓   | ✓      | ✓      | ✓                    | N/A       | Structurally verified |
| AES-JSON  | aes.db  | `aes-json`      | ✓    | ✓       | ✓   | ✓      | ✓      | ✓                    | ✓         | Structurally verified |
| JSON      | baseline.db | N/A        | ✓    | ✓       | ✓   | ✓      | ✓      | N/A (default codec)   | N/A       | Implicit        |
| Protobuf  | N/A     | N/A             | Excluded with documented reason (see codec manifest) |     |    |        |         |                      |           |                 |

## Bidirectional Roundtrip Scope

The roundtrip test (`roundtrip/roundtrip_test.go`) exercises:

1. v5 creates a database with records, indexes, KV, and nested records.
2. v6 reopens, reads, updates, deletes, and writes.
3. v5 reopens and verifies all v6 mutations (exact IDs, record sets, update
   field persistence, index movement, delete cleanup, unique reuse, increment
   continuation, KV update, nested record update, unaffected records).
4. v6 performs a final reopen and verify of the complete state.

**Limitation:** v5 and v6 compile into the same test binary via Go MVS. Both
share `go.etcd.io/bbolt v1.4.3`. This proves cross-major API/on-disk
interoperability under a single dependency graph. It is not process or
toolchain isolation. Process-isolated roundtrip testing may be added if
dependencies diverge.

## Benchmark Summary

Benchmarks compare equivalent v5.3.0 and v6 workloads using the default JSON
codec with `NoSync` on `Apple M4 darwin/arm64`. Methodology:

- Setup (populate + preflight) outside the timed region.
- Five raw samples per benchmark with median comparison.
- Save postflight verifies the final record persisted.
- Update alternates between non-zero values (1001/1002).
- Canceled-context uses preflight and postflight assertions.
- Same `go.etcd.io/bbolt v1.4.3` for both versions via MVS.

**Results:** No paired benchmark exceeds the 25% investigation threshold. The
largest observed differences: Save +18.2% ns/op, KVGet +11.7% ns/op. All other
differences are <3% ns/op. The report does not causally attribute differences
without profiling and explicitly states the NoSync limitation.

See `benchmark/results/comparison.md` for the full report.

## Known Limitations and Exclusions

- **Protobuf:** Excluded because the existing generated type cannot carry the
  required indexed fixture schema, and a generic struct would fall back to JSON.
- **Process-isolated roundtrip:** Deferred; current roundtrip uses a single test
  binary with shared MVS dependency resolution.
- **Benchmarks are single-machine, NoSync, 5-sample:** Results are descriptive
  snapshots, not permanent performance guarantees. No statistical significance
  testing was performed.
- **Raw bbolt bytes are not deterministic:** Compatibility is verified through
  manifests and behavioral tests, not byte-level comparison.
- `ErrNoName` is an API precondition (anonymous types require explicit nodes),
  not proof of storage isolation. Identical `reflect.StructOf` shapes may
  resolve to the same canonical type.

## CI Note (R6.6)

The root `go test ./...` excludes nested compatibility modules. R6.6 must
decide how CI invokes:

- `go -C testdata/compatibility/roundtrip test ...`
- `go -C testdata/compatibility/benchmark test -run '^$' -bench ...`

Do not add CI workflows during R6.5.
