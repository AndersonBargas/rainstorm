# Rainstorm v6.1.0 Release Notes

> **Release date:** 2026-09-05 · **Tag:** `v6.1.0` · **Module:** `github.com/AndersonBargas/rainstorm/v6`

## 1. Executive summary

Rainstorm v6.1.0 is an additive, backward-compatible release. It introduces a
transaction-bound facade so Rainstorm record/index operations can participate
in a bbolt transaction **owned by the caller**, without leaking a `*bolt.Tx`
parameter into every public operation and without moving transaction ownership
out of the caller's unit of work.

## 2. New public API

```go
// Tx is a transaction-bound facade over the full Node surface.
// It exposes no commit, rollback, or close operations.
type Tx interface {
	Node
}

// Attach binds Rainstorm to an existing, caller-owned bbolt transaction.
func (s *DB) Attach(tx *bolt.Tx) (Tx, error)

// ErrTxFromDifferentDB: Attach received a transaction from another database.
var ErrTxFromDifferentDB error
```

`rainstorm.Tx` satisfies `rainstorm.Node`, so existing code that accepts a
`Node` works unchanged with a bound `Tx`.

## 3. Guarantees

- Rainstorm never commits, rolls back, closes, or replaces the supplied
  transaction, and never opens a managed transaction for a bound `Tx`.
- All reads and writes through the bound `Tx` — including nodes derived via
  `From`, `WithCodec`, `PrefixScan`, and `RangeScan` — execute inside the
  supplied transaction. Uncommitted writes are visible to reads through the
  same `Tx` and invisible to every other transaction until the caller commits.
- Attach accepts writable and read-only transactions. Writes through a
  read-only transaction fail with bbolt's `ErrTxNotWritable`.
- `Attach` returns `ErrNilParam` for a nil receiver or transaction, and
  `ErrTxFromDifferentDB` when the transaction belongs to a different database.
- A bound `Tx` is valid only for the lifetime of its transaction. Using it
  after `Commit`/`Rollback` is undefined behavior, exactly like using the
  bbolt transaction itself (writes fail with `ErrTxClosed`; reads may panic).

## 4. Example

```go
tx, err := db.NativeDB().Begin(true)
if err != nil {
	return err
}
defer tx.Rollback()

txn, err := db.Attach(tx) // rainstorm.Tx — Rainstorm borrows, never owns
if err != nil {
	return err
}

// Rainstorm record write (record + indexes) in the caller's transaction…
if err := txn.Save(ctx, &account); err != nil {
	return err
}

// …and a raw BoltDB write in the same transaction, committed atomically.
audit, err := tx.CreateBucketIfNotExists([]byte("audit"))
if err != nil {
	return err
}
if err := audit.Put([]byte("event"), []byte("created")); err != nil {
	return err
}

return tx.Commit() // the caller decides the outcome
```

## 5. Tests

Ten new tests cover: atomic commit of Rainstorm record writes plus raw bolt
bucket writes; rollback discarding records, indexes, and raw writes;
read-your-own-uncommitted-writes through the bound handle; isolation from
managed, raw, and other attached transactions; proof that Rainstorm never
commits or rolls back the caller's transaction; read-only transaction write
rejection; index consistency under commit and rollback; Attach validation; and
handle lifetime.

## 6. CI

All seven GitHub Actions jobs (Quality, Staticcheck, Test 1.24.x/stable, Race,
Compatibility, Coverage) pass on the tag and on `master`.

## 7. Upgrade

```sh
go get github.com/AndersonBargas/rainstorm/v6@v6.1.0
go mod tidy
```

No migration is required: the release is fully additive and changes no
existing behavior.
