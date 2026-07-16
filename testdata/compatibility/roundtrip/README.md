# Roundtrip compatibility suite

This nested module imports both `v5` and `v6` of Rainstorm intentionally
to perform cross-version roundtrip compatibility testing.

## Why a nested module?

The main `v6` module must not import `v5`. This nested module isolates the
cross-version dependency so that both major versions can be used in the
same test binary without polluting the main module's dependency graph.

## What this proves

- **Cross-major public-API/on-disk interoperability**: v5 creates databases,
  v6 reads and mutates them, v5 reopens and verifies, v6 performs a final
  verification.
- **Shared transitive dependencies**: Go MVS selects each transitive
  dependency once for the test binary. The relevant `go.etcd.io/bbolt`
  version used by both v5.3.0 and v6 is the same (verify with `go list -m all`).
- This is **not** process-level or toolchain-level isolation between the
  two major versions. Both v5 and v6 compile into the same test binary and
  share their dependency graph.
- Process-isolated compatibility may be added later if dependency or
  toolchain divergence requires it.

## Running

```sh
go -C testdata/compatibility/roundtrip test -count=1 ./...
```

## Excluded from main module

This module is not traversed by the main module's `go test ./...`.
It must be run explicitly during compatibility validation.
CI integration belongs to R6.6.

## Test behavior

- All databases are created dynamically in `t.TempDir()`.
- No checked-in fixtures are modified.
- Tests are independent and must not depend on ordering.
- No package-global mutable state is shared between tests.
- Every phase uses only public APIs of the respective version.
