# Rainstorm v6.1.1 Release Notes

> **Release date:** 2026-09-06 · **Tag:** `v6.1.1` · **Module:** `github.com/AndersonBargas/rainstorm/v6`

## 1. Summary

Rainstorm v6.1.1 is a patch release with no public API changes. It updates the
test-only dependency `github.com/stretchr/testify` from `v1.11.1` to `v1.12.1`
(Dependabot go-deps group) and keeps the nested compatibility fixtures under
`testdata/compatibility` in sync so the tidy/diff CI gates stay green.

## 2. Changes

- `github.com/stretchr/testify` `v1.11.1` → `v1.12.1`
- `testdata/compatibility/roundtrip` and `testdata/compatibility/benchmark`
  manifests re-tidied against the new dependency graph.

## 3. Upgrade

```sh
go get github.com/AndersonBargas/rainstorm/v6@v6.1.1
go mod tidy
```

No migration is required.
