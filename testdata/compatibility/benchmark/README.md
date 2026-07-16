# Rainstorm Cross-Major Benchmark Module

This nested module compares representative v5.3.0 and current local v6 operations
using equivalent data, codecs, indexes, and bbolt versions.

## Structure

```
testdata/compatibility/benchmark/
├── go.mod
├── go.sum
├── benchmark_test.go
├── README.md
└── results/
    ├── environment.txt
    ├── v5.txt
    ├── v6.txt
    ├── v6_only.txt
    └── comparison.md
```

## Running

```sh
# Smoke test (all benchmarks, short)
go test -run '^$' -bench . -benchtime=100ms -count=1

# Collect v5 samples
go test -run '^$' -bench '^BenchmarkV5/' -benchmem -benchtime=500ms -count=5 > results/v5.txt

# Collect v6 samples
go test -run '^$' -bench '^BenchmarkV6/' -benchmem -benchtime=500ms -count=5 > results/v6.txt

# Collect v6-only samples
go test -run '^$' -bench '^BenchmarkV6_' -benchmem -benchtime=500ms -count=5 > results/v6_only.txt

# Run all tests (unit)
go test -count=1 ./...

# Run tests with race detector
go test -race -count=1 ./...
```

## Methodology

See `results/comparison.md` for the full methodology, raw data, and analysis.

## Schema

```go
type BenchmarkRecord struct {
    ID       uint64 `rainstorm:"id,increment"`
    Key      string `rainstorm:"unique"`
    Category string `rainstorm:"index"`
    Name     string
    Revision int
    Payload  string
}
```

## Limitations

- Single-machine results (Apple M4, darwin/arm64).
- NoSync bbolt for faster temp-dir setup.
- No statistical significance testing.
- Raw results are environment-specific snapshots.
