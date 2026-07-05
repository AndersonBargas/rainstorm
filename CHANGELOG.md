# Changelog

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
