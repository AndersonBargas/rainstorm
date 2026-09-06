package rainstorm

import (
	bolt "go.etcd.io/bbolt"
)

// Tx is a Rainstorm facade bound to a bbolt transaction owned by the caller.
//
// It exposes the complete Node operation surface — Save, Update,
// DeleteStruct, One, Find, All, AllByIndex, Range, Prefix, Count, Select
// queries, bucket scans, and key/value access — and every operation executes
// inside the supplied transaction. Uncommitted writes are visible to reads
// performed through the same Tx.
//
// Tx intentionally exposes no commit, rollback, or close operations, and it
// never opens a managed transaction of its own. The caller retains full
// ownership of the transaction and must Commit or RollBack it.
type Tx interface {
	Node
}

// Compile-time assertion: the transaction-bound node implements Tx.
var _ Tx = (*node)(nil)

// Attach returns a transaction-bound facade for an existing bbolt transaction
// owned by the caller.
//
// Guarantees:
//
//   - Rainstorm never commits, rolls back, closes, or replaces the supplied
//     transaction. No operation on the returned Tx acquires a managed
//     transaction.
//   - All reads and writes through the returned Tx — including nodes derived
//     via From, WithCodec, PrefixScan, and RangeScan — use the supplied
//     transaction.
//   - Attach accepts writable and read-only transactions. Writes through a
//     read-only transaction fail with bbolt's ErrTxNotWritable.
//
// Attach fails with ErrNilParam for a nil receiver or transaction, and with
// ErrTxFromDifferentDB when the transaction belongs to a different database.
//
// The returned Tx is valid only for the lifetime of the supplied transaction.
// Using it after the caller commits or rolls back is undefined behavior, just
// as using the bbolt transaction itself would be: reads may panic and writes
// fail with bbolt's ErrTxClosed. Rainstorm never falls back to a managed
// transaction for a dead handle.
func (s *DB) Attach(tx *bolt.Tx) (Tx, error) {
	if s == nil || tx == nil {
		return nil, wrapError("attach", ErrNilParam)
	}

	if s.bolt == nil || tx.DB() != s.bolt {
		return nil, wrapError("attach", ErrTxFromDifferentDB)
	}

	root, ok := s.Node.(*node)
	if !ok {
		return nil, wrapError("attach", ErrBadType)
	}

	return root.withTransaction(tx), nil
}
