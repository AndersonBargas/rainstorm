package rainstorm

import (
	"context"

	bolt "go.etcd.io/bbolt"
)

// TransactionManager provides callback-managed transactions.
// These are the canonical transaction boundaries for Rainstorm v6.
type TransactionManager interface {
	ReadTransaction(ctx context.Context, fn func(Node) error) error
	WriteTransaction(ctx context.Context, fn func(Node) error) error
}

// Compile-time assertion: *DB implements TransactionManager.
var _ TransactionManager = (*DB)(nil)

// ReadTransaction executes fn within a read-only bbolt transaction.
// The transaction is automatically closed when fn returns.
//
// Context is checked before acquisition, after acquisition, and before
// concluding the read. If the context is cancelled after the callback
// returns successfully, the cancellation error is returned.
//
// Panic behavior: if fn panics, bbolt closes (rolls back) the read
// transaction and the panic propagates to the caller. Rainstorm does
// not recover panics.
func (s *DB) ReadTransaction(ctx context.Context, fn func(Node) error) error {
	if err := checkContext(ctx); err != nil {
		return wrapError("read transaction", err)
	}
	if fn == nil {
		return wrapError("read transaction", ErrNilParam)
	}

	return wrapError("read transaction", s.bolt.View(func(tx *bolt.Tx) error {
		if err := checkContext(ctx); err != nil {
			return err
		}

		txNode, err := s.transactionNode(tx)
		if err != nil {
			return err
		}
		if err := fn(txNode); err != nil {
			return err
		}

		if err := checkContext(ctx); err != nil {
			return err
		}

		return nil
	}))
}

// WriteTransaction executes fn within a writable bbolt transaction.
// If fn returns an error, the transaction is rolled back and the error
// is returned.
//
// The context is checked before acquisition, after acquisition, and
// before commit. If the context is cancelled before commit, the
// transaction is rolled back and ctx.Err() is returned. After a
// successful commit, a later cancellation is not retroactively applied.
//
// WriteTransaction uses bbolt.Update, which executes the callback exactly once.
//
// Panic behavior: if fn panics, bbolt rolls back the transaction and
// the panic propagates to the caller. All writes performed before the
// panic are discarded. Rainstorm does not recover panics.
func (s *DB) WriteTransaction(ctx context.Context, fn func(Node) error) error {
	if err := checkContext(ctx); err != nil {
		return wrapError("write transaction", err)
	}
	if fn == nil {
		return wrapError("write transaction", ErrNilParam)
	}

	return wrapError("write transaction", s.bolt.Update(func(tx *bolt.Tx) error {
		if err := checkContext(ctx); err != nil {
			return err
		}

		txNode, err := s.transactionNode(tx)
		if err != nil {
			return err
		}
		if err := fn(txNode); err != nil {
			return err
		}

		if err := checkContext(ctx); err != nil {
			return err
		}

		return nil
	}))
}

// transactionNode builds a transaction-bound node that preserves
// codec, root bucket, and other configuration from the root node.
// It does not modify the root node or store state on DB.
func (s *DB) transactionNode(tx *bolt.Tx) (Node, error) {
	root, ok := s.Node.(*node)
	if !ok {
		return nil, ErrBadType
	}
	return root.withTransaction(tx), nil
}
