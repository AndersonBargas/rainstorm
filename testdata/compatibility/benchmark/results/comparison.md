# Rainstorm v5.3.0 vs v6 Cross-Major Benchmark Comparison

## 1. Commits Compared

| Version   | Identifier                                                                 |
|-----------|----------------------------------------------------------------------------|
| v5.3.0    | `github.com/AndersonBargas/rainstorm/v5` v5.3.0                            |
| v6 (local)| `github.com/AndersonBargas/rainstorm/v6` HEAD `4860c35` (ancestry of v5.3.0) |

## 2. Environment

| Property   | Value                      |
|------------|----------------------------|
| Go version | go1.26.5                   |
| GOOS       | darwin                     |
| GOARCH     | arm64                      |
| CPU        | Apple M4                   |
| bbolt      | go.etcd.io/bbolt v1.4.3    |
| benchstat  | not installed (manual median) |
| benchtime  | 500ms per sample           |
| count      | 5 samples per benchmark    |
| codec      | default JSON               |
| sync       | NoSync (explicit benchmark configuration) |

Both v5 and v6 share `go.etcd.io/bbolt v1.4.3` through MVS. Both major versions compile
into the same benchmark binary, proving relative API implementation behaviour under one
selected dependency graph. This is not process / toolchain isolation.

## 3. Command Lines

```sh
# v5 collection
go -C testdata/compatibility/benchmark test -run '^$' -bench '^BenchmarkV5/' \
  -benchmem -benchtime=500ms -count=5 -timeout 600s > testdata/compatibility/benchmark/results/v5.txt

# v6 collection
go -C testdata/compatibility/benchmark test -run '^$' -bench '^BenchmarkV6/' \
  -benchmem -benchtime=500ms -count=5 -timeout 600s > testdata/compatibility/benchmark/results/v6.txt

# v6-only collection
go -C testdata/compatibility/benchmark test -run '^$' -bench '^BenchmarkV6_' \
  -benchmem -benchtime=500ms -count=5 -timeout 300s > testdata/compatibility/benchmark/results/v6_only.txt
```

## 4. Workload Definitions

All benchmarks use `BenchmarkRecord` with the default JSON codec:

```go
type BenchmarkRecord struct {
    ID       uint64 `rainstorm:"id,increment"`
    Key      string `rainstorm:"unique"`
    Category string `rainstorm:"index"`
    Name     string
    Revision int
    Payload  string  // 100 deterministic bytes
}
```

| Benchmark          | Description                                                  |
|--------------------|--------------------------------------------------------------|
| OneByID            | Single record lookup by primary key (`ID` field)             |
| OneByUnique        | Single record lookup by unique index (`Key` field)           |
| FindIndexed        | Multi-record find on indexed field (`Category`), returns N/2 |
| FindUnindexed      | Multi-record find on unindexed field (`Name`), returns N/2   |
| All                | Retrieve all records via full scan                           |
| Save               | Insert new distinct record (key formatted inside timed loop) |
| Update             | Update existing record, alternating Revision between 1001 and 1002 |
| KVGet              | Key-value Get on a known key                                 |
| KVSet              | Key-value Set overwriting the same key                       |

### Dataset Sizes

- **100 records**: `Category` split evenly cat-a / cat-b (50 each). `Name` split evenly name-a / name-b (50 each).
- **1000 records**: Same proportional distribution (500 each).

### Setup & Population Methodology

1. Create database in `b.TempDir()` with `NoSync` bbolt option.
2. Save N deterministic records using `populateRecordsV5` / `populateRecordsV6`.
3. Run correctness preflight outside the timed region:
   - `All` count matches N.
   - `One("ID", N/2, &dst)` resolves correctly.
   - `One("Key", "key-0000", &dst)` resolves correctly.
   - `Find("Category", "cat-a", &dst)` returns expected count.
   - `Find("Name", "name-a", &dst)` returns same count.
   - KV `Set`/`Get` roundtrip.
4. `b.ReportAllocs()` then `b.ResetTimer()`.
5. Benchmark only the target operation.
6. Stop the timer and verify the final Save/Update persisted as intended.

For Save, a fresh struct with a unique key (`fmt.Sprintf("save-%d", i)`) is created
inside the timed loop. The `fmt.Sprintf` overhead is identically included in both v5
and v6. Each iteration resets `ID` to zero so Rainstorm assigns a new increment ID.

For Update, the target record's `Revision` alternates between the non-zero values 1001
and 1002 so every iteration performs a real persisted update. A postflight lookup verifies
the final value after the timer stops.

For KVSet, the value alternates between `"hello"` and `"world"`, overwriting the same key.

Every benchmark/sub-benchmark creates and owns its own database. No mutable state is
shared across benchmark functions.

## 5. Correctness Preflight Inventory

Every read benchmark (OneByID, OneByUnique, FindIndexed, FindUnindexed, All, KVGet)
runs the following preflight checks outside the timed region before `b.ResetTimer()`:

| Check              | Verification                                   |
|--------------------|------------------------------------------------|
| `All` count        | `len(all) == N`                                |
| `One("ID", ...)`   | returned record has expected ID                |
| `One("Key", ...)`  | returned record has expected Key               |
| `Find("Category")` | returns N/2 records (all with cat-a)           |
| `Find("Name")`     | returns N/2 records (all with name-a)          |
| `KV Get`/`Set`     | roundtrip returns expected value               |

The codec (default JSON) is implicitly verified by these checks. All preflight
assertions pass for both v5 and v6.

## 6. Paired Comparison Tables

All values are approximate medians from 5 samples. Percentage differences are computed
as `(v6 - v5) / v5 * 100`. No confidence interval or statistical significance test
was performed.

### 6.1 OneByID

| Size | v5 ns/op | v6 ns/op | Δ ns/op | v5 B/op | v6 B/op | Δ B/op |
|------|----------|----------|---------|---------|---------|--------|
| 100  | 1,773    | 1,818    | +2.5%   | 1,688   | 1,784   | +5.7%  |
| 1000 | 1,816    | 1,855    | +2.1%   | 1,696   | 1,792   | +5.7%  |

### 6.2 OneByUnique

| Size | v5 ns/op | v6 ns/op | Δ ns/op | v5 B/op | v6 B/op | Δ B/op |
|------|----------|----------|---------|---------|---------|--------|
| 100  | 1,941    | 1,993    | +2.7%   | 1,768   | 1,864   | +5.4%  |
| 1000 | 2,024    | 2,074    | +2.5%   | 1,832   | 1,928   | +5.2%  |

### 6.3 FindIndexed

| Size | v5 ns/op  | v6 ns/op  | Δ ns/op | v5 B/op  | v6 B/op  | Δ B/op |
|------|-----------|-----------|---------|----------|----------|--------|
| 100  | 62,663    | 63,210    | +0.9%   | 39,056   | 39,008   | -0.1%  |
| 1000 | 614,855   | 618,032   | +0.5%   | 331,010  | 330,961  | -0.0%  |

### 6.4 FindUnindexed

| Size | v5 ns/op   | v6 ns/op   | Δ ns/op | v5 B/op  | v6 B/op  | Δ B/op |
|------|------------|------------|---------|----------|----------|--------|
| 100  | 124,097    | 124,612    | +0.4%   | 77,544   | 77,496   | -0.1%  |
| 1000 | 1,232,378  | 1,237,873  | +0.4%   | 743,164  | 743,116  | -0.0%  |

### 6.5 All

| Size | v5 ns/op   | v6 ns/op   | Δ ns/op | v5 B/op  | v6 B/op  | Δ B/op |
|------|------------|------------|---------|----------|----------|--------|
| 100  | 116,146    | 117,669    | +1.3%   | 79,592   | 79,544   | -0.1%  |
| 1000 | 1,151,938  | 1,162,986  | +1.0%   | 724,845  | 724,795  | -0.0%  |

### 6.6 Save

| v5 ns/op | v6 ns/op | Δ ns/op | v5 B/op*  | v6 B/op*  |
|----------|----------|---------|-----------|-----------|
| 127,355  | 150,592  | +18.2%  | 504,447   | 580,636   |

*B/op for Save shows high variance across samples because the iteration count varies,
changing database growth and bbolt internal state. Medians computed independently for
ns/op and B/op.

### 6.7 Update

| Size | v5 ns/op | v6 ns/op | Δ ns/op | v5 B/op  | v6 B/op  | Δ B/op |
|------|----------|----------|---------|----------|----------|--------|
| 100  | 17,978   | 18,320   | +1.9%   | 34,567   | 34,568   | +0.0%  |
| 1000 | 24,185   | 24,871   | +2.8%   | 61,001   | 61,021   | +0.0%  |

### 6.8 KVGet

| v5 ns/op | v6 ns/op | Δ ns/op | v5 B/op | v6 B/op | Δ B/op |
|----------|----------|---------|---------|---------|--------|
| 331.8    | 370.7    | +11.7%  | 608     | 624     | +2.6%  |

### 6.9 KVSet

| v5 ns/op | v6 ns/op | Δ ns/op | v5 B/op  | v6 B/op  | Δ B/op |
|----------|----------|---------|----------|----------|--------|
| 9,272    | 9,472    | +2.2%   | 19,473   | 19,399   | -0.4%  |

## 7. V6-Only Benchmarks

These benchmarks exercise v6-specific APIs and have no direct v5 equivalent.

| Benchmark               | Median ns/op | Median B/op | Median allocs/op | Description                          |
|-------------------------|-------------|-------------|------------------|--------------------------------------|
| ReadTransaction         | 181,706     | 120,112     | 1,902            | Grouped One+Find+All+Get in one tx   |
| WriteTransaction        | 9,512       | 19,679      | 74               | Two Set calls in one write tx        |
| CanceledContext         | 102.9       | 112         | 4                | Already-canceled context rejection   |

## 8. Regression Trigger Analysis

Using the investigation thresholds (>25% slower ns/op or >25% higher B/op):

**No benchmark triggers** either the >25% ns/op regression threshold or the >25% B/op
regression threshold.

The largest observed differences:
- Save: +18.2% ns/op, +15.1% B/op — below the investigation threshold
- KVGet: +11.7% ns/op, +2.6% B/op — below the investigation threshold

All other ns/op differences are less than 3%. Some point-read B/op differences are
between 5% and 6%.

### Summary of Observed Differences

| Category          | Typical ns/op Δ | Typical B/op Δ | Observation |
|-------------------|----------------|----------------|-------------|
| Point reads (ID)  | +2.1–2.5%      | +5.7%          | v6 uses about 96 additional bytes/op |
| Point reads (idx) | +2.5–2.7%      | +5.2–5.4%      | v6 uses about 96 additional bytes/op |
| Multi reads       | +0.4–1.3%      | ~0%            | close results for these datasets |
| Save              | +18.2%         | +15.1%         | largest observed paired difference |
| Update/KVSet      | +1.9–2.8%      | ~0%            | close results under this NoSync workload |
| KVGet             | +11.7%         | +2.6%          | second-largest relative latency difference |

These end-to-end benchmarks do not isolate the cost of context checks, destination
publication, transaction changes, or any other individual implementation difference.
The 96-byte point-read difference and latency differences are observations; assigning
them to a specific mechanism would require profiling or ablation benchmarks.

## 9. Interpretation

1. **Point reads show a small end-to-end difference**: v6 is approximately 2–3%
   slower in these samples and uses approximately 96 additional bytes per operation.
   This suite does not isolate the source of that difference.

2. **Bulk-operation allocations are close**: Find, All, and Update show negligible
   B/op differences (<0.1%) for these datasets.

3. **Save has the largest paired difference** (+18.2% ns/op and +15.1% B/op) under
   NoSync. It remains below the predefined investigation trigger, and no profiling was
   performed to assign the difference to a particular implementation change.

4. **KVGet is the second-largest relative latency difference** (+11.7% ns/op), while
   its absolute difference is approximately 39 ns/op. No profiling was performed to
   assign this difference to a particular v6 feature.

5. **V6 transaction numbers are standalone workload baselines**. Because there is no
   equivalent composite non-transaction benchmark, they do not quantify transaction
   overhead or savings.

6. **CanceledContext measures an already-canceled rejection path**. Its preflight and
   postflight verify that the returned error matches `context.Canceled`.

7. **No regression triggers**: all paired metrics remain below the predefined 25%
   investigation threshold.

## 10. Limitations

1. **Single machine**: Results reflect one Apple M4 (darwin/arm64) configuration.
   Different hardware, OS, or filesystem characteristics may produce different
   absolute and relative numbers. Comparisons across machines are not treated as
   equivalent evidence.

2. **NoSync databases**: Benchmarks use `bolt.Options{NoSync: true}` for both versions.
   These results compare the versions under that same configuration, but they do not
   establish relative write performance with production synchronization enabled.

3. **No statistical testing**: Five samples allow approximate median computation
   but no confidence intervals or significance tests. The reported percentage
   differences are approximate.

4. **Homogeneous dataset**: Records use a single schema with uniform payload size.
   Performance characteristics may differ with varied payloads, many indexes,
   or deeply nested buckets.

5. **Shared bbolt version**: Both v5 and v6 share `go.etcd.io/bbolt v1.4.3`
   through MVS. Any bbolt-level performance characteristics are identical.

6. **Save benchmark key formatting**: `fmt.Sprintf("save-%d", i)` is included in
   the timed loop for both v5 and v6. It is part of both measured workloads, but
   its effect is not isolated.

7. **Implementation attribution**: v6 includes context propagation, operation wrapping,
   managed transactions, and destination-safety changes. This end-to-end suite does not
   isolate the individual performance cost of those changes.

## 11. Regressions Investigated

No workload exceeded the 25% investigation threshold. Therefore, no rerun with
`benchtime=1s count=10` was performed. The observed differences were recorded but not
attributed to individual implementation mechanisms because no profiling or ablation
measurement was performed.

## 12. Conclusion

Under this environment and NoSync configuration, v6 measured:

- **Point reads**: approximately 2–3% higher ns/op and 96 additional B/op;
- **Bulk reads**: approximately 0.4–1.3% higher ns/op with negligible B/op difference;
- **Save**: approximately 18% higher ns/op and 15% higher B/op;
- **Update**: approximately 2–3% higher ns/op with negligible B/op difference;
- **KVGet**: approximately 12% higher ns/op and 16 additional B/op;
- **KVSet**: approximately 2% higher ns/op with negligible B/op difference.

No paired benchmark exceeds the 25% investigation threshold. These are descriptive
end-to-end results, not causal measurements of individual v6 features.

## 13. Raw Result Files

- `results/v5.txt` — 5 samples, 15 v5 benchmark families (81 lines)
- `results/v6.txt` — 5 samples, 15 v6 benchmark families (81 lines)
- `results/v6_only.txt` — 5 samples, 3 v6-only benchmark families (21 lines)
- `results/environment.txt` — Go version, GOOS/GOARCH, CPU, `go list -m all`

Raw results are environment-specific snapshots and not permanent performance
guarantees. Rerunning may produce different numbers. Future comparisons should
use the same machine and commands.

## 14. Existing bench_test.go

Not modified. The existing `bench_test.go` in the v6 package root was inspected but
left unchanged. It serves as a package-local v6 benchmark covering different workloads
(User struct, indexed/unindexed Find, OneByID, Save). No correctness flaws requiring
correction were identified in the existing benchmarks.

## 15. Production Changes

**None.** No production code was modified. All benchmark code is contained in the
nested module `testdata/compatibility/benchmark/`.

## 16. Reproducibility Notes

- Rerun with identical commands on the same machine for comparable results.
- CI hardware should not directly compare with these local numbers.
- `NoSync` eliminates fsync variability but means absolute write latencies are lower
  than production.
- Future comparisons should pin the same Go version, bbolt version, and benchtime.
