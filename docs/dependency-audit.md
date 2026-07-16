# R6.6A Dependency Audit

**Audit date:** 2026-07-15
**Commit:** `f5994e78b7ec312bcefc7b51f6ff5bab4ce7f6b3` (docs: consolidate v5 compatibility evidence)
**Phase:** R6.6A — Dependency inventory, risk classification, testify update
**Module:** `github.com/AndersonBargas/rainstorm/v6`

---

## 1. Local Go Environment

| Variable     | Value                     |
|-------------|---------------------------|
| `go version` | go1.26.5 darwin/arm64     |
| `GOVERSION`  | go1.26.5                  |
| `GOOS`       | darwin                    |
| `GOARCH`     | arm64                     |
| `GOPROXY`    | https://proxy.golang.org,direct |
| `GOSUMDB`    | sum.golang.org            |
| `go` directive | 1.24.0 (not changed)   |

---

## 2. Main Module Direct Dependencies

| Module | Current | Latest Available | Usage | Status | Compat Risk | R6.6 Decision |
|--------|---------|-----------------|-------|--------|-------------|---------------|
| `github.com/Sereal/Sereal` | `v0.0.0-20200820125258-a016b7cda3f3` | `v0.0.0-20260710121258-d894361fc66f` | Production codec (`codec/sereal`) | Pseudo-version only; no tagged releases | **High** — API/encoding changes possible | Defer to dedicated phase (R6.6A4) |
| `github.com/golang/protobuf` | `v1.3.2` | `v1.5.4` | Production codec (`codec/protobuf`) | **Deprecated** — use `google.golang.org/protobuf` | **High** — wire format, API, generated code | Defer to dedicated phase (R6.6A2) |
| `github.com/stretchr/testify` | `v1.10.0` | **`v1.11.1`** | Test-only (`_test.go` files, roundtrip) | Active, stable | **Low** — minor version, test-only | **Updated in R6.6A** |
| `github.com/vmihailenco/msgpack` | `v4.0.4+incompatible` | (v5 via `github.com/vmihailenco/msgpack/v5`) | Production codec (`codec/msgpack`) | `+incompatible`; maintained releases use `v5` major path | **High** — API, encoding, fixture compatibility | Defer to dedicated phase (R6.6A3) |
| `go.etcd.io/bbolt` | `v1.4.3` | `v1.5.0` | Production storage layer | Active; v1.5.0 requires Go 1.24+ | **Medium** — see bbolt analysis below | Keep current in R6.6A; evaluate in later subphase |

---

## 3. Relevant Indirect Dependencies

| Module | Current | Latest Available | Notes |
|--------|---------|-----------------|-------|
| `github.com/DataDog/zstd` | `v1.4.1` | `v1.5.7` | Transitive through Sereal; update must be evaluated with the Sereal codec |
| `github.com/golang/snappy` | `v0.0.1` | `v1.0.0` | Transitive through Sereal; update must be evaluated with the Sereal codec |
| `golang.org/x/net` | `v0.0.0-20191105084925-a882066a44e0` | `v0.57.0` | Transitive through the legacy MsgPack v4 → App Engine datastore path |
| `golang.org/x/sys` | `v0.39.0` | `v0.47.0` | Used transitively by production bbolt and by test/tool dependencies |
| `google.golang.org/appengine` | `v1.6.5` | `v1.6.8` | Required transitively by legacy MsgPack v4; removal is coupled to MsgPack migration |
| `gopkg.in/yaml.v3` | `v3.0.1` | `v3.0.1` | Current; stable; no update needed |

---

## 4. Nested Module Policy

### v5 Fixture Generator (`testdata/compatibility/v5.3.0/generator`)
- **Must remain pinned** to `github.com/AndersonBargas/rainstorm/v5 v5.3.0` and its dependency graph.
- Its `go.mod` pins historical versions (`v4.0.4+incompatible` msgpack, `v1.3.2` protobuf, `v1.4.1` zstd, etc.).
- **Do not update.** Fixture provenance must not be invalidated.

### Roundtrip (`testdata/compatibility/roundtrip`)
- Intentionally imports both `rainstorm/v5` and local `rainstorm/v6` via `replace` directive.
- Already uses `testify v1.11.1`.
- **No changes needed in R6.6A.**

### Benchmark (`testdata/compatibility/benchmark`)
- Intentionally imports both `rainstorm/v5` and local `rainstorm/v6` via `replace` directive.
- **No changes needed in R6.6A.**

### Nested module CI invocation
- Implemented in R6.6B as a dedicated compatibility job.
- Roundtrip: normal + race tests on stable Go.
- Benchmark: compile/run normal + race (no full `-bench` suite).
- v5 fixture generator not invoked in ordinary CI.
- Nested module `go.mod`/`go.sum` remain independently maintained.

---

## 5. Codec Wire/On-disk Compatibility Constraints

All codec dependency updates carry **on-disk compatibility risk** with existing v5 databases:

| Codec | Risk |
|-------|------|
| **MsgPack** | `+incompatible` semantics; v5 API uses different import path; encoding may differ |
| **Protobuf** | Old API (`github.com/golang/protobuf`) vs new API (`google.golang.org/protobuf`); generated code regeneration required; wire format must be verified |
| **Sereal** | No tagged releases; pseudo-version only; encoding stability unknown across pseudo-versions |
| **JSON** | Standard library; no risk |
| **Gob** | Standard library; no risk |
| **AES** | Standard library; no risk |

Checked-in v5 fixtures (`testdata/compatibility/v5.3.0/baseline.db` and `testdata/compatibility/v5.3.0/codecs/*.db`) must remain readable after any codec migration.

---

## 6. Vulnerability Scan Status

**Not run.** The `govulncheck` tool (`golang.org/x/vuln/cmd/govulncheck`) is not installed on this machine.

No claims are made about the security posture of any dependency based on absence of advisories.

---

## 7. Update Applied in R6.6A

### Testify: `v1.10.0` → `v1.11.1`

**Command:**
```
go get github.com/stretchr/testify@v1.11.1
go mod tidy
```

**Result:**
- `go.mod`: 1 line changed (version string only)
- `go.sum`: 2 hash lines replaced (old v1.10.0 → new v1.11.1)
- **No indirect dependencies added, removed, or upgraded**
- **No codec or storage dependencies changed**
- **No production `.go` files changed**
- **No generated or fixture files changed**
- **No nested `go.mod`/`go.sum` files changed**

**Confirmation:**
- testify is test-only in the main module (all imports in `_test.go` files only)
- No production package imports testify

---

## 8. Deferred Update Groups

### Group A: Low-risk, test-only (done)
- ✅ testify v1.11.1 — applied

### Group B: Codec migrations — dedicated subphases required
- **R6.6A2:** Protobuf API migration (v1.3.2 → `google.golang.org/protobuf`)
  - Requires generated-code regeneration
  - Requires wire/on-disk compatibility verification
  - Old module is deprecated
- **R6.6A3:** MsgPack major-version migration (v4 → v5 import path)
  - Requires fixture compatibility proof
  - API changes, encoding may differ
- **R6.6A4:** Sereal + transitive dependency decision
  - Sereal has no tagged releases; evaluate replacement/removal
  - DataDog/zstd v1.4.1 is old and only transitively needed

### Group C: bbolt evaluation
- v1.4.3 is current enough (released 2025-08-19)
- v1.5.0 available; requires Go 1.24+ (compatible with current directive)
- v5 fixtures and v6 currently share the same bbolt version
- Evaluate in a later subphase with on-disk compatibility testing

### Group D: Legacy indirect dependency graph
- `golang.org/x/net` and `google.golang.org/appengine` are pulled in by legacy MsgPack v4 and must be evaluated with the MsgPack migration.
- `github.com/golang/snappy` and `github.com/DataDog/zstd` are pulled in by Sereal and must be evaluated with the Sereal decision.
- These modules must not be independently removed or upgraded without their owning codec compatibility tests.

---

## 9. Detailed Dependency Conclusions

### A. bbolt (`go.etcd.io/bbolt` v1.4.3)

| Question | Answer |
|----------|--------|
| Is v1.4.3 current? | v1.5.0 is available (2026-06-03); v1.4.3 was released 2025-08-19 |
| Minimum Go requirement | v1.4.3 → Go 1.23; v1.5.0 → Go 1.24 |
| Would upgrading alter file compatibility? | Unlikely (bbolt maintains backwards compatibility), but must be verified |
| Do v5 fixtures and v6 share same version? | Yes — both use v1.4.3 |
| R6.6A action | **No change.** Evaluate in later subphase. |

### B. testify (`github.com/stretchr/testify` v1.11.1)

| Question | Answer |
|----------|--------|
| Is v1.11.1 a stable compatible update? | Yes — minor version bump, same major API |
| Is it test-only in main module? | Yes — all imports in `_test.go` files |
| Does any production package import it? | No |
| R6.6A action | **Updated.** |

### C. golang/protobuf (`github.com/golang/protobuf` v1.3.2)

| Question | Answer |
|----------|--------|
| Is the module deprecated? | Yes — marked deprecated; recommends `google.golang.org/protobuf` |
| Does Rainstorm use the old API? | Yes — `codec/protobuf/protobuf.go` imports `github.com/golang/protobuf/proto` |
| Would migration require source changes? | Yes — API differs; `codec/protobuf/simple_user.pb.go` is generated with old API |
| Would migration require wire verification? | Yes — protobuf wire compatibility must be confirmed |
| R6.6A action | **No change.** Needs dedicated subphase. |

### D. MsgPack (`github.com/vmihailenco/msgpack` v4.0.4+incompatible)

| Question | Answer |
|----------|--------|
| Current import semantics | `+incompatible` — module has no `go.mod` or pre-modules `go.mod` |
| Maintained releases | Use `github.com/vmihailenco/msgpack/v5` (major-version import path) |
| Would migration change API? | Yes — different package, likely API changes |
| Would migration change encoding? | Possibly — must verify |
| Must v5 fixtures remain readable? | Yes — checked-in MsgPack fixtures must still decode |
| R6.6A action | **No change.** Needs dedicated subphase. |

### E. Sereal (`github.com/Sereal/Sereal`)

| Question | Answer |
|----------|--------|
| Latest available pseudo-version | `v0.0.0-20260710121258-d894361fc66f` (2026-07-10) |
| Tagged releases? | No — only pseudo-versions |
| Maintenance activity | Recent commits visible (2026-07-10) |
| Relationship with DataDog/zstd | Sereal depends on DataDog/zstd; v1.4.1 is old |
| Safe in-place update? | Unclear — no changelog, no tagged releases, encoding stability unknown |
| Replacement/removal? | Should be considered only in a future major version |
| R6.6A action | **No change.** Defer to R6.6A4. |

### F. Go Version Policy (finalized in R6.6B)

| Item | Value |
|------|-------|
| Current `go` directive | `go 1.24.0` (not changed) |
| Minimum supported Go | 1.24.x |
| Current stable (dev/CI) | `setup-go` stable channel |
| Locally installed Go | `go1.26.5` |
| R6.6A action | Policy deferred to R6.6B |
| R6.6B action | **Policy documented here.** |

**Policy:**
- The module declares `go 1.24.0` as its minimum language/toolchain requirement.
- CI proves the module builds and tests on Go 1.24.x (minimum) and current stable Go.
- Race detection, coverage, and compatibility suites run on stable only to control CI cost.
- README will document supported Go versions in R6.7.
- Dependency updates remain frozen for runtime codecs/storage in v6.
- Nested modules are invoked explicitly in CI.

---

## 10. Validation Results

| Check | Result |
|-------|--------|
| `git diff --check` | ✅ Pass |
| `go mod tidy` (first) | ✅ Pass |
| `go mod tidy` (second) | ✅ Stable — no additional diff |
| `go mod verify` | ✅ All modules verified |
| `go vet ./...` | ✅ Pass |
| `go test -count=1 -timeout 180s ./...` | ✅ All 11 packages pass |
| `go test -race -count=1 -timeout 300s ./...` | ✅ All 11 packages pass |
| `go build ./...` | ✅ Pass |
| `go -C testdata/compatibility/roundtrip test ...` | ✅ Pass |
| `go -C testdata/compatibility/benchmark test ...` | ✅ Pass (no tests to run) |

---

## 11. Final Confirmation

- **Files changed:** `go.mod`, `go.sum`, and `docs/dependency-audit.md`
- **Production `.go` files:** unchanged
- **Test `.go` files:** unchanged
- **Generated protobuf file:** unchanged
- **Compatibility fixtures:** unchanged
- **Nested module `go.mod`/`go.sum`:** unchanged
- **No commit or push performed**
- **Working tree left uncommitted for review**

---

## 12. Proposed Next Subphases

1. **R6.6A2:** Protobuf API migration feasibility and fixture proof
2. **R6.6A3:** MsgPack major-version migration feasibility and fixture proof
3. **R6.6A4:** Sereal/transitive dependency decision (update, replace, or defer)
4. **R6.6C:** Staticcheck integration and coverage threshold
5. **R6.6D:** Dependency automation (Dependabot/Renovate)

Codec migrations that cannot demonstrate wire/on-disk compatibility with checked-in v5 fixtures should be explicitly deferred to v7.

---

## 13. R6.6B -- CI Pipeline

**Date:** 2026-07-15
**Phase:** R6.6B -- CI workflow modernization

### Workflow trigger configuration

```yaml
on:
  push:
  pull_request:
  workflow_dispatch:

permissions:
  contents: read

concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true
```

### Job inventory

| Job | Go version | Timeout | Checks |
|-----|-----------|---------|--------|
| quality | 1.24.x | 10 min | gofmt, tidy diff, mod verify, vet, build |
| test | 1.24.x + stable | 10 min | `go test -count=1 -timeout 180s ./...` |
| race | stable | 15 min | `go test -race -count=1 -timeout 300s ./...` |
| compatibility | stable | 15 min | roundtrip normal + race, benchmark normal + race |
| coverage | stable | 10 min | profile, text summary, artifact upload |

### Action versions

| Action | Version | Source evidence |
|--------|---------|----------------|
| `actions/checkout` | v7 | Latest stable release from GitHub Releases API: v7.0.0 (2026-06-18). |
| `actions/setup-go` | v6 | Latest stable major from GitHub Releases API: v6.5.0 (2026-06-24). |
| `actions/upload-artifact` | v7 | Latest stable release from GitHub Releases API: v7.0.1 (2026-04-10). |

All versions confirmed via GitHub Releases API on 2026-07-15.

### Cache configuration

Main module jobs use `cache-dependency-path: go.sum`.

The compatibility job uses:

```yaml
cache-dependency-path: |
  go.sum
  testdata/compatibility/roundtrip/go.sum
  testdata/compatibility/benchmark/go.sum
```

### Excluded from R6.6B

- v5 fixture generator (not invoked in ordinary CI)
- Full benchmark suite (only compile/test; no `-bench`)
- Staticcheck (R6.6C)
- Dependabot/Renovate (R6.6D)
- `paths-ignore` (removed; all changes trigger CI)

### Coverage behavior

- Profile: `go test -covermode=atomic -coverprofile=coverage.out`
- Summary: `go tool cover -func=coverage.out` piped to `coverage.txt`
- Artifact: `coverage-stable` containing `coverage.out` + `coverage.txt`
- Retention: 14 days
- No minimum coverage threshold enforced (R6.6C)

### R6.6B local validation (2026-07-15)

| Check | Result |
|-------|--------|
| `gofmt -l .` | Pass after formatting the pre-existing generated protobuf file |
| `go mod tidy` + `git diff --exit-code` | Pass |
| `go mod verify` | Pass |
| `go vet ./...` | Pass |
| `go test -count=1 -timeout 180s ./...` | Pass (11 packages) |
| `go test -race -count=1 -timeout 300s ./...` | Pass (11 packages) |
| `go build ./...` | Pass |
| Coverage profile + summary | Total: 80.1% |
| Roundtrip normal | Pass |
| Roundtrip race | Pass |
| Benchmark normal | Pass (no tests to run) |
| Benchmark race | Pass (no tests to run) |
| Workflow syntax | `actionlint` unavailable; parsed successfully with local Ruby/Psych YAML parser and manually reviewed |

### Workflow audit checklist

| Requirement | Status |
|-------------|--------|
| No `paths-ignore` | Yes |
| Includes `pull_request` | Yes |
| Includes `push` | Yes |
| Includes `workflow_dispatch` | Yes |
| `permissions: contents: read` | Yes |
| Concurrency configured | Yes |
| Formatting present | Yes |
| Tidy diff check present | Yes |
| Mod verify present | Yes |
| Vet present | Yes |
| Normal tests on 1.24.x and stable | Yes |
| Race present | Yes |
| Build present | Yes |
| Compatibility nested modules present | Yes |
| Coverage profile and text summary present | Yes |
| Artifact upload present | Yes |
| No staticcheck yet | Yes |
| No dependency automation yet | Yes |
| No fixture regeneration | Yes |
| No full benchmarks | Yes |

### R6.6B files changed

- `.github/workflows/main.yml` -- replaced with new CI pipeline
- `docs/dependency-audit.md` -- added Go version policy and R6.6B CI section
- `codec/protobuf/simple_user.pb.go` -- gofmt-only generated-file cleanup required by the formatting job
