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

// Tx is a transaction.
type Tx interface {
	// Commit writes all changes to disk.
	Commit(ctx context.Context) error

	// Rollback closes the transaction and ignores all previous updates.
	Rollback() error
}

// Begin starts a new transaction.
func (n node) Begin(ctx context.Context, writable bool) (Node, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	var err error

	n.tx, err = n.s.Bolt.Begin(writable)
	if err != nil {
		return nil, err
	}

	if err = checkContext(ctx); err != nil {
		_ = n.tx.Rollback()
		return nil, err
	}

	return &n, nil
}

// Rollback closes the transaction and ignores all previous updates.
func (n *node) Rollback() error {
	if n.tx == nil {
		return ErrNotInTransaction
	}

	err := n.tx.Rollback()
	if err == bolt.ErrTxClosed {
		return ErrNotInTransaction
	}

	return err
}

// Commit writes all changes to disk.
func (n *node) Commit(ctx context.Context) error {
	if n.tx == nil {
		return ErrNotInTransaction
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	err := n.tx.Commit()
	if err == bolt.ErrTxClosed {
		return ErrNotInTransaction
	}

	return err
}

// ReadTransaction executes fn within a read-only bbolt transaction.
// The transaction is automatically rolled back when fn returns.
// Context is checked before acquisition, after acquisition, and before
// concluding the read. If the context is cancelled after the callback
// returns successfully, the cancellation error is returned.
func (s *DB) ReadTransaction(ctx context.Context, fn func(Node) error) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	if fn == nil {
		return ErrNilParam
	}

	return s.Bolt.View(func(tx *bolt.Tx) error {
		if err := checkContext(ctx); err != nil {
			return err
		}

		txNode := s.transactionNode(tx)
		if err := fn(txNode); err != nil {
			return err
		}

		if err := checkContext(ctx); err != nil {
			return err
		}

		return nil
	})
}

// WriteTransaction executes fn within a writable bbolt transaction.
// If fn returns an error, the transaction is rolled back.
// The context is checked before commit: if cancelled, the transaction
// is rolled back and ctx.Err() is returned.
//
// Unlike legacy methods, WriteTransaction ignores batch mode entirely
// and always uses Bolt.Update, which executes the callback exactly once.
func (s *DB) WriteTransaction(ctx context.Context, fn func(Node) error) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	if fn == nil {
		return ErrNilParam
	}

	return s.Bolt.Update(func(tx *bolt.Tx) error {
		if err := checkContext(ctx); err != nil {
			return err
		}

		txNode := s.transactionNode(tx)
		if err := fn(txNode); err != nil {
			return err
		}

		if err := checkContext(ctx); err != nil {
			return err
		}

		return nil
	})
}

// transactionNode builds a transaction-bound node that preserves
// codec, root bucket, and other configuration from the root node.
// It does not modify the root node or store state on DB.
func (s *DB) transactionNode(tx *bolt.Tx) Node {
	return s.Node.WithTransaction(tx)
}
