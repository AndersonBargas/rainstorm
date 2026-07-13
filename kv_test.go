package rainstorm

import (
	"context"
	"net/mail"
	"testing"
	"time"

	"github.com/AndersonBargas/rainstorm/v6/codec/gob"
	"github.com/AndersonBargas/rainstorm/v6/codec/json"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

func TestGet(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	err := db.Set(ctx, "trash", 10, 100)
	require.NoError(t, err)

	var nb int
	err = db.Get(ctx, "trash", 10, &nb)
	require.NoError(t, err)
	require.Equal(t, 100, nb)

	tm := time.Now()
	err = db.Set(ctx, "logs", tm, "I'm hungry")
	require.NoError(t, err)

	var message string
	err = db.Get(ctx, "logs", tm, &message)
	require.NoError(t, err)
	require.Equal(t, "I'm hungry", message)

	var hand int
	err = db.Get(ctx, "wallet", "100 bucks", &hand)
	require.Equal(t, ErrNotFound, err)

	err = db.Set(ctx, "wallet", "10 bucks", 10)
	require.NoError(t, err)

	err = db.Get(ctx, "wallet", "100 bucks", &hand)
	require.Equal(t, ErrNotFound, err)

	err = db.Get(ctx, "logs", tm, nil)
	require.Equal(t, ErrPtrNeeded, err)

	err = db.Get(ctx, "", nil, nil)
	require.Equal(t, ErrPtrNeeded, err)

	err = db.Get(ctx, "", "100 bucks", &hand)
	require.Equal(t, ErrNotFound, err)
}

func TestGetBytes(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	err := db.SetBytes(ctx, "trash", "a", []byte("hi"))
	require.NoError(t, err)

	val, err := db.GetBytes(ctx, "trash", "a")
	require.NoError(t, err)
	require.Equal(t, []byte("hi"), val)

	_, err = db.GetBytes(ctx, "trash", "b")
	require.Equal(t, ErrNotFound, err)
}

func TestSet(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	err := db.Set(ctx, "b1", 10, 10)
	require.NoError(t, err)
	err = db.Set(ctx, "b1", "best friend's mail", &mail.Address{Name: "Gandalf", Address: "gandalf@lorien.ma"})
	require.NoError(t, err)
	err = db.Set(ctx, "b2", []byte("i'm already a slice of bytes"), "a value")
	require.NoError(t, err)
	err = db.Set(ctx, "b2", []byte("i'm already a slice of bytes"), nil)
	require.NoError(t, err)
	err = db.Set(ctx, "b1", 0, 100)
	require.NoError(t, err)
	err = db.Set(ctx, "b1", nil, 100)
	require.Error(t, err)

	db.NativeDB().View(func(tx *bolt.Tx) error {
		b1 := tx.Bucket([]byte("b1"))
		require.NotNil(t, b1)
		b2 := tx.Bucket([]byte("b2"))
		require.NotNil(t, b2)

		k1, err := toBytes(10, json.Codec)
		require.NoError(t, err)
		val := b1.Get(k1)
		require.NotNil(t, val)

		k2 := []byte("best friend's mail")
		val = b1.Get(k2)
		require.NotNil(t, val)

		k3, err := toBytes(0, json.Codec)
		require.NoError(t, err)
		val = b1.Get(k3)
		require.NotNil(t, val)

		return nil
	})

	err = db.Set(ctx, "", 0, 100)
	require.Error(t, err)

	err = db.Set(ctx, "b", nil, 100)
	require.Error(t, err)

	err = db.Set(ctx, "b", 10, nil)
	require.NoError(t, err)

	err = db.Set(ctx, "b", nil, nil)
	require.Error(t, err)
}

func TestSetMetadata(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	w := User{ID: 10, Name: "John"}
	err := db.Set(ctx, "User", 10, &w)
	require.NoError(t, err)
	n := db.WithCodec(gob.Codec)
	err = n.Set(ctx, "User", 10, &w)
	require.Equal(t, ErrDifferentCodec, err)
}

func TestDelete(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	err := db.Set(ctx, "files", "myfile.csv", "a,b,c,d")
	require.NoError(t, err)
	err = db.Delete(ctx, "files", "myfile.csv")
	require.NoError(t, err)
	err = db.Delete(ctx, "files", "myfile.csv")
	require.NoError(t, err)
	err = db.Delete(ctx, "i don't exist", "myfile.csv")
	require.Equal(t, ErrNotFound, err)
	err = db.Delete(ctx, "", nil)
	require.Equal(t, ErrNotFound, err)
}

func TestKeyExists(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	err := db.Set(ctx, "files", "myfile.csv", "a,b,c,d")
	require.NoError(t, err)

	exists, err := db.KeyExists(ctx, "files", "myfile.csv")
	require.NoError(t, err)
	require.True(t, exists)

	err = db.Delete(ctx, "files", "myfile.csv")
	require.NoError(t, err)

	exists, err = db.KeyExists(ctx, "files", "myfile.csv")
	require.NoError(t, err)
	require.False(t, exists)

	exists, err = db.KeyExists(ctx, "i don't exist", "myfile.csv")
	require.Equal(t, ErrNotFound, err)
	require.False(t, exists)

	exists, err = db.KeyExists(ctx, "", nil)
	require.Equal(t, ErrNotFound, err)
	require.False(t, exists)
}
