# v5.3.0 baseline fixture generator

This generator creates `../baseline.db` — the canonical Rainstorm v5.3.0
compatibility fixture used by the v6 compatibility test suite.

## Source

- Module: `github.com/AndersonBargas/rainstorm/v5`
- Version: `v5.3.0` (tag `b98ebe5`)

## Generation command

Run from this directory:

```sh
go run .
```

The fixture is written to `../baseline.db`.

## Determinism

Raw bbolt on-disk layout may vary across runs even when the semantic
content is identical. **Do not rely on byte-for-byte reproducibility.**
Compatibility is verified through:

- The manifest (`../manifest.json`);
- Behavioral compatibility tests in the v6 test suite.

After regeneration, review the manifest and run the full v6
compatibility test suite; do not inspect raw bytes.

## Isolation

This generator is a separate Go module. It is not part of the v6
production module. The normal `go test ./...` from the repository
root does not execute or rebuild the generator.
