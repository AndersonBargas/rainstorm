package rainstorm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/AndersonBargas/rainstorm/v6/codec/gob"
	"github.com/stretchr/testify/require"
	bolterrors "go.etcd.io/bbolt/errors"
)

// ---------------------------------------------------------------------------
// 13.1 Contract and argument validation
// ---------------------------------------------------------------------------

func TestReadTransaction_CancelledContextDoesNotRunCallback(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ran := false
	err := db.ReadTransaction(ctx, func(Node) error {
		ran = true
		return nil
	})

	require.True(t, errors.Is(err, context.Canceled))
	require.False(t, ran)
}

func TestWriteTransaction_CancelledContextDoesNotRunCallback(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ran := false
	err := db.WriteTransaction(ctx, func(Node) error {
		ran = true
		return nil
	})

	require.True(t, errors.Is(err, context.Canceled))
	require.False(t, ran)
}

func TestReadTransaction_NilCallbackRejected(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	err := db.ReadTransaction(context.Background(), nil)
	require.True(t, errors.Is(err, ErrNilParam))
}

func TestWriteTransaction_NilCallbackRejected(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	err := db.WriteTransaction(context.Background(), nil)
	require.True(t, errors.Is(err, ErrNilParam))
}

// ---------------------------------------------------------------------------
// 13.2 Read transaction
// ---------------------------------------------------------------------------

func TestReadTransaction_ReadsExistingData(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	// Seed data outside managed transaction.
	err := db.Save(ctx, &User{ID: 1, Name: "Alice", Slug: "a"})
	require.NoError(t, err)

	var found User
	err = db.ReadTransaction(ctx, func(txNode Node) error {
		return txNode.One(ctx, "ID", 1, &found)
	})
	require.NoError(t, err)
	require.Equal(t, "Alice", found.Name)
}

func TestReadTransaction_CallbackErrorReturned(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	errCallback := errors.New("callback failed")

	err := db.ReadTransaction(ctx, func(Node) error {
		cancel()
		return errCallback
	})

	require.ErrorIs(t, err, errCallback)
	require.NotErrorIs(t, err, context.Canceled, "callback error must take precedence")
}

func TestReadTransaction_WriteRejected(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	err := db.ReadTransaction(ctx, func(txNode Node) error {
		return txNode.Save(ctx, &User{ID: 10, Name: "test", Slug: "s"})
	})
	require.ErrorIs(t, err, bolterrors.ErrTxNotWritable)

	// Verify nothing was persisted.
	var u User
	err = db.One(ctx, "ID", 10, &u)
	require.True(t, errors.Is(err, ErrNotFound))
}

func TestReadTransaction_CancellationAfterCallbackReturned(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())

	ran := false
	err := db.ReadTransaction(ctx, func(Node) error {
		ran = true
		cancel()
		return nil
	})
	require.True(t, errors.Is(err, context.Canceled))
	require.True(t, ran)
}

// ---------------------------------------------------------------------------
// 13.3 Write transaction
// ---------------------------------------------------------------------------

func TestWriteTransaction_CommitsAllWrites(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	err := db.WriteTransaction(ctx, func(txNode Node) error {
		if err := txNode.Save(ctx, &User{ID: 1, Name: "Alice", Slug: "a"}); err != nil {
			return err
		}
		return txNode.Save(ctx, &User{ID: 2, Name: "Bob", Slug: "b"})
	})
	require.NoError(t, err)

	// Both records must be visible after commit.
	var u1, u2 User
	require.NoError(t, db.One(ctx, "ID", 1, &u1))
	require.Equal(t, "Alice", u1.Name)
	require.NoError(t, db.One(ctx, "ID", 2, &u2))
	require.Equal(t, "Bob", u2.Name)
}

func TestWriteTransaction_CallbackErrorRollsBack(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()
	errCallback := errors.New("rollback me")

	err := db.WriteTransaction(ctx, func(txNode Node) error {
		if err := txNode.Save(ctx, &UniqueNameUser{ID: 1, Name: "rollback-test", Age: 10}); err != nil {
			return err
		}
		return errCallback
	})
	require.True(t, errors.Is(err, errCallback))

	// Record must not exist after rollback.
	var u UniqueNameUser
	err = db.One(ctx, "ID", 1, &u)
	require.True(t, errors.Is(err, ErrNotFound))

	// Unique index must not have an orphaned entry.
	err = db.One(ctx, "Name", "rollback-test", &u)
	require.True(t, errors.Is(err, ErrNotFound))

	// A new save with the same unique value must succeed.
	err = db.Save(ctx, &UniqueNameUser{ID: 2, Name: "rollback-test", Age: 20})
	require.NoError(t, err)

	err = db.One(ctx, "Name", "rollback-test", &u)
	require.NoError(t, err)
	require.Equal(t, 2, u.ID)
}

func TestWriteTransaction_CancellationBeforeCommitRollsBack(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())

	err := db.WriteTransaction(ctx, func(txNode Node) error {
		if err := txNode.Save(ctx, &User{ID: 1, Name: "Alice", Slug: "cancel-test"}); err != nil {
			return err
		}
		cancel()
		return nil
	})
	require.True(t, errors.Is(err, context.Canceled))

	// Record must not exist after rollback.
	var u User
	err = db.One(context.Background(), "ID", 1, &u)
	require.True(t, errors.Is(err, ErrNotFound))
}

func TestWriteTransaction_CallbackRunsExactlyOnce(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	var counter int
	err := db.WriteTransaction(context.Background(), func(txNode Node) error {
		counter++
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, counter)
}

func TestWriteTransaction_CancellationAfterReturnDoesNotUndoCommit(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())

	err := db.WriteTransaction(ctx, func(txNode Node) error {
		return txNode.Save(ctx, &User{ID: 1, Name: "Alice", Slug: "committed"})
	})
	require.NoError(t, err)

	// Cancel after successful commit — must not affect persisted state.
	cancel()

	var u User
	err = db.One(context.Background(), "ID", 1, &u)
	require.NoError(t, err)
	require.Equal(t, "Alice", u.Name)
}

// ---------------------------------------------------------------------------
// 12. Transactional visibility
// ---------------------------------------------------------------------------

func TestWriteTransaction_UncommittedWritesAreNotVisibleOutside(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	err := db.WriteTransaction(ctx, func(txNode Node) error {
		if err := txNode.Save(ctx, &User{ID: 42, Name: "invisible", Slug: "inv"}); err != nil {
			return err
		}

		var inside User
		if err := txNode.One(ctx, "ID", 42, &inside); err != nil {
			return err
		}
		if inside.Name != "invisible" {
			return fmt.Errorf("transactional read returned name %q", inside.Name)
		}

		// bbolt permits a concurrent reader while a write transaction is open.
		// That reader receives the last committed snapshot and must not observe
		// the uncommitted record.
		outsideResult := make(chan error, 1)
		go func() {
			var outside User
			outsideResult <- db.One(ctx, "ID", 42, &outside)
		}()

		if err := <-outsideResult; !errors.Is(err, ErrNotFound) {
			return fmt.Errorf("outside read before commit: %w", err)
		}

		return nil
	})
	require.NoError(t, err)

	var committed User
	require.NoError(t, db.One(ctx, "ID", 42, &committed))
	require.Equal(t, "invisible", committed.Name)
}

func TestWriteTransaction_RollbackRemovesUncommittedWrites(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()
	errCallback := errors.New("abort")

	err := db.WriteTransaction(ctx, func(txNode Node) error {
		if err := txNode.Save(ctx, &User{ID: 99, Name: "ghost", Slug: "ghost"}); err != nil {
			return err
		}

		// Visible inside transaction.
		var u User
		require.NoError(t, txNode.One(ctx, "ID", 99, &u))
		require.Equal(t, "ghost", u.Name)

		return errCallback
	})
	require.True(t, errors.Is(err, errCallback))

	// After rollback: record must not exist.
	var u User
	err = db.One(ctx, "ID", 99, &u)
	require.True(t, errors.Is(err, ErrNotFound))
}

// ---------------------------------------------------------------------------
// 13.4 Transactional node properties
// ---------------------------------------------------------------------------

func TestManagedTransaction_PreservesRootAndCodec(t *testing.T) {
	db, cleanup := createDB(t, Root("tenant", "ns"), Codec(gob.Codec))
	defer cleanup()

	err := db.WriteTransaction(context.Background(), func(txNode Node) error {
		require.Equal(t, []string{"tenant", "ns"}, txNode.Bucket())
		require.Equal(t, gob.Codec, txNode.Codec())
		return nil
	})
	require.NoError(t, err)
}

func TestManagedTransaction_DoesNotReplaceRootNode(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	rootBefore := db.Node

	err := db.WriteTransaction(context.Background(), func(txNode Node) error {
		// txNode must be different from the root node.
		if txNode == rootBefore {
			t.Error("txNode must not be the same object as root node")
		}
		return nil
	})
	require.NoError(t, err)

	// Root node must still be the same object.
	if db.Node != rootBefore {
		t.Error("root node must not be replaced by WriteTransaction")
	}
}

func TestManagedTransaction_NodeUsesProvidedTransaction(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	errCallback := errors.New("rollback")

	err := db.WriteTransaction(context.Background(), func(txNode Node) error {
		if err := txNode.Save(context.Background(), &User{ID: 1, Name: "test", Slug: "txn"}); err != nil {
			return err
		}
		return errCallback
	})
	require.True(t, errors.Is(err, errCallback))

	// The write must have been rolled back, proving the txNode shared
	// the same transaction.
	var u User
	err = db.One(context.Background(), "ID", 1, &u)
	require.True(t, errors.Is(err, ErrNotFound))
}

// ---------------------------------------------------------------------------
// Concurrency: multiple goroutines using managed transactions
// ---------------------------------------------------------------------------

func TestWriteTransaction_ConcurrentIndependent(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()
	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)

	errs := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			err := db.WriteTransaction(ctx, func(txNode Node) error {
				return txNode.Save(ctx, &User{
					ID:   id + 1,
					Name: "concurrent",
					Slug: fmt.Sprintf("slug-%d", id),
				})
			})
			errs <- err
		}(i)
	}

	wg.Wait()
	close(errs)

	for e := range errs {
		require.NoError(t, e)
	}

	// All records must be visible.
	for i := 0; i < goroutines; i++ {
		var u User
		err := db.One(ctx, "ID", i+1, &u)
		require.NoError(t, err)
	}
}
