package rainstorm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// capturePanic calls fn and returns the value passed to panic, or nil if fn
// did not panic. It is intended for tests that use a non-nil sentinel value.
func capturePanic(fn func()) (recovered any) {
	defer func() {
		recovered = recover()
	}()
	fn()
	return
}

// ---------------------------------------------------------------------------
// Panic in WriteTransaction
// ---------------------------------------------------------------------------

func TestWriteTransaction_PanicRollsBackAndPropagates(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()
	panicValue := &struct{ Name string }{Name: "boom"}

	// Execute WriteTransaction that panics.
	recovered := capturePanic(func() {
		_ = db.WriteTransaction(ctx, func(txNode Node) error {
			// Save a record.
			err := txNode.Save(ctx, &User{ID: 1, Name: "panic-test", Slug: "pt1"})
			require.NoError(t, err)

			// Confirm it is visible inside the transaction.
			var u User
			err = txNode.One(ctx, "ID", 1, &u)
			require.NoError(t, err)
			require.Equal(t, "panic-test", u.Name)

			panic(panicValue)
		})
	})

	// Panic value must be exactly the same pointer.
	require.Same(t, panicValue, recovered,
		"panic value identity must be preserved")

	// The record must not exist — rollback occurred.
	var u User
	err := db.One(ctx, "ID", 1, &u)
	require.ErrorIs(t, err, ErrNotFound, "record must not exist after panic rollback")

	// A new WriteTransaction must succeed (no leaked lock).
	err = db.WriteTransaction(ctx, func(txNode Node) error {
		return txNode.Save(ctx, &User{ID: 2, Name: "after-panic", Slug: "ap1"})
	})
	require.NoError(t, err)

	var u2 User
	err = db.One(ctx, "ID", 2, &u2)
	require.NoError(t, err)
	require.Equal(t, "after-panic", u2.Name)
}

// ---------------------------------------------------------------------------
// Panic in ReadTransaction
// ---------------------------------------------------------------------------

func TestReadTransaction_PanicPropagatesAndClosesTransaction(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	// Seed a record.
	err := db.Save(ctx, &User{ID: 10, Name: "seed", Slug: "s1"})
	require.NoError(t, err)

	panicValue := &struct{ Name string }{Name: "read-boom"}

	recovered := capturePanic(func() {
		_ = db.ReadTransaction(ctx, func(txNode Node) error {
			var u User
			err := txNode.One(ctx, "ID", 10, &u)
			require.NoError(t, err)
			require.Equal(t, "seed", u.Name)

			panic(panicValue)
		})
	})

	require.Same(t, panicValue, recovered,
		"panic value identity must be preserved")

	// A new ReadTransaction must succeed (no leaked lock).
	err = db.ReadTransaction(ctx, func(txNode Node) error {
		var u User
		return txNode.One(ctx, "ID", 10, &u)
	})
	require.NoError(t, err)

	// A new WriteTransaction must also succeed.
	err = db.WriteTransaction(ctx, func(txNode Node) error {
		return txNode.Save(ctx, &User{ID: 11, Name: "after-read-panic", Slug: "arp1"})
	})
	require.NoError(t, err)

	var u User
	err = db.One(ctx, "ID", 11, &u)
	require.NoError(t, err)
	require.Equal(t, "after-read-panic", u.Name)
}

// ---------------------------------------------------------------------------
// Manual Commit forbidden inside WriteTransaction
// ---------------------------------------------------------------------------

func TestWriteTransaction_ManualCommitPanicsAndRollsBack(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	var continued bool

	recovered := capturePanic(func() {
		_ = db.WriteTransaction(ctx, func(txNode Node) error {
			err := txNode.Save(ctx, &User{ID: 1, Name: "manual-commit", Slug: "mc1"})
			require.NoError(t, err)

			_ = txNode.Commit(ctx) // must panic

			continued = true // unreachable
			return nil
		})
	})

	require.NotNil(t, recovered, "manual Commit inside WriteTransaction must panic")
	require.False(t, continued, "callback must not continue after manual Commit")

	// Record must not exist — manual Commit does not actually commit.
	var u User
	err := db.One(ctx, "ID", 1, &u)
	require.ErrorIs(t, err, ErrNotFound, "record must not exist after manual Commit panic")

	// A new WriteTransaction must succeed.
	err = db.WriteTransaction(ctx, func(txNode Node) error {
		return txNode.Save(ctx, &User{ID: 2, Name: "after-commit-panic", Slug: "acp1"})
	})
	require.NoError(t, err)

	var u2 User
	err = db.One(ctx, "ID", 2, &u2)
	require.NoError(t, err)
	require.Equal(t, "after-commit-panic", u2.Name)
}

// ---------------------------------------------------------------------------
// Manual Rollback forbidden inside WriteTransaction
// ---------------------------------------------------------------------------

func TestWriteTransaction_ManualRollbackPanicsAndRollsBack(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	var continued bool

	recovered := capturePanic(func() {
		_ = db.WriteTransaction(ctx, func(txNode Node) error {
			err := txNode.Save(ctx, &User{ID: 1, Name: "manual-rollback", Slug: "mr1"})
			require.NoError(t, err)

			_ = txNode.Rollback() // must panic

			continued = true // unreachable
			return nil
		})
	})

	require.NotNil(t, recovered, "manual Rollback inside WriteTransaction must panic")
	require.False(t, continued, "callback must not continue after manual Rollback")

	// Record must not exist.
	var u User
	err := db.One(ctx, "ID", 1, &u)
	require.ErrorIs(t, err, ErrNotFound, "record must not exist after manual Rollback panic")

	// A new WriteTransaction must succeed.
	err = db.WriteTransaction(ctx, func(txNode Node) error {
		return txNode.Save(ctx, &User{ID: 2, Name: "after-rollback-panic", Slug: "arp2"})
	})
	require.NoError(t, err)

	var u2 User
	err = db.One(ctx, "ID", 2, &u2)
	require.NoError(t, err)
	require.Equal(t, "after-rollback-panic", u2.Name)
}

// ---------------------------------------------------------------------------
// Manual Rollback forbidden inside ReadTransaction
// ---------------------------------------------------------------------------

func TestReadTransaction_ManualRollbackPanicsAndClosesTransaction(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	// Seed a record.
	err := db.Save(ctx, &User{ID: 10, Name: "seed", Slug: "s2"})
	require.NoError(t, err)

	var continued bool

	recovered := capturePanic(func() {
		_ = db.ReadTransaction(ctx, func(txNode Node) error {
			var u User
			err := txNode.One(ctx, "ID", 10, &u)
			require.NoError(t, err)

			_ = txNode.Rollback() // must panic inside managed read tx

			continued = true // unreachable
			return nil
		})
	})

	require.NotNil(t, recovered, "manual Rollback inside ReadTransaction must panic")
	require.False(t, continued, "callback must not continue after manual Rollback")

	// A new ReadTransaction must succeed (no leaked lock).
	err = db.ReadTransaction(ctx, func(txNode Node) error {
		var u User
		return txNode.One(ctx, "ID", 10, &u)
	})
	require.NoError(t, err)

	// A new WriteTransaction must also succeed.
	err = db.WriteTransaction(ctx, func(txNode Node) error {
		return txNode.Save(ctx, &User{ID: 11, Name: "after-read-rb", Slug: "arr1"})
	})
	require.NoError(t, err)

	var u User
	err = db.One(ctx, "ID", 11, &u)
	require.NoError(t, err)
	require.Equal(t, "after-read-rb", u.Name)
}

// ---------------------------------------------------------------------------
// Panicking callback runs exactly once
// ---------------------------------------------------------------------------

func TestWriteTransaction_PanickingCallbackRunsOnce(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	var counter int
	panicValue := &struct{ Name string }{Name: "intentional"}

	recovered := capturePanic(func() {
		_ = db.WriteTransaction(context.Background(), func(txNode Node) error {
			counter++
			panic(panicValue)
		})
	})

	require.Same(t, panicValue, recovered, "panic must propagate unchanged")
	require.Equal(t, 1, counter, "panicking callback must execute exactly once")
}
