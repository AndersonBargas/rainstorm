package rainstorm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTransaction(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	err := db.Rollback()
	require.Error(t, err)

	err = db.Commit(ctx)
	require.Error(t, err)

	tx, err := db.Begin(ctx, true)
	require.NoError(t, err)

	ntx, ok := tx.(*node)
	require.True(t, ok)
	require.NotNil(t, ntx.tx)

	err = tx.Init(ctx, &SimpleUser{})
	require.NoError(t, err)

	err = tx.Save(ctx, &User{ID: 10, Name: "John"})
	require.NoError(t, err)

	err = tx.Save(ctx, &User{ID: 20, Name: "John"})
	require.NoError(t, err)

	err = tx.Save(ctx, &User{ID: 30, Name: "Steve"})
	require.NoError(t, err)

	var user User
	err = tx.One(ctx, "ID", 10, &user)
	require.NoError(t, err)

	var users []User
	err = tx.AllByIndex(ctx, "Name", &users)
	require.NoError(t, err)
	require.Len(t, users, 3)

	err = tx.All(ctx, &users)
	require.NoError(t, err)
	require.Len(t, users, 3)

	err = tx.Find(ctx, "Name", "Steve", &users)
	require.NoError(t, err)
	require.Len(t, users, 1)

	err = tx.DeleteStruct(ctx, &user)
	require.NoError(t, err)

	err = tx.One(ctx, "ID", 10, &user)
	require.Error(t, err)

	err = tx.Set(ctx, "b1", "best friend's mail", "mail@provider.com")
	require.NoError(t, err)

	var str string
	err = tx.Get(ctx, "b1", "best friend's mail", &str)
	require.NoError(t, err)
	require.Equal(t, "mail@provider.com", str)

	err = tx.Delete(ctx, "b1", "best friend's mail")
	require.NoError(t, err)

	err = tx.Get(ctx, "b1", "best friend's mail", &str)
	require.Error(t, err)

	err = tx.Commit(ctx)
	require.NoError(t, err)

	err = tx.Commit(ctx)
	require.Error(t, err)
	require.Equal(t, ErrNotInTransaction, err)

	err = db.One(ctx, "ID", 30, &user)
	require.NoError(t, err)
	require.Equal(t, 30, user.ID)
}

func TestTransactionRollback(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	tx, err := db.Begin(ctx, true)
	require.NoError(t, err)

	err = tx.Save(ctx, &User{ID: 10, Name: "John"})
	require.NoError(t, err)

	var user User
	err = tx.One(ctx, "ID", 10, &user)
	require.NoError(t, err)
	require.Equal(t, 10, user.ID)

	err = tx.Rollback()
	require.NoError(t, err)

	err = db.One(ctx, "ID", 10, &user)
	require.Error(t, err)
}

func TestTransactionNotWritable(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	err := db.Save(ctx, &User{ID: 10, Name: "John"})
	require.NoError(t, err)

	tx, err := db.Begin(ctx, false)
	require.NoError(t, err)

	err = tx.Save(ctx, &User{ID: 20, Name: "John"})
	require.Error(t, err)

	var user User
	err = tx.One(ctx, "ID", 10, &user)
	require.NoError(t, err)

	err = tx.Rollback()
	require.NoError(t, err)
}
