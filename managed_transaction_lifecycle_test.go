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
