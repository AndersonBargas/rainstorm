package index_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/AndersonBargas/rainstorm/v6"
	"github.com/AndersonBargas/rainstorm/v6/codec/gob"
	"github.com/AndersonBargas/rainstorm/v6/index"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

func TestUniqueIndex(t *testing.T) {
	dir, _ := os.MkdirTemp(os.TempDir(), "rainstorm")
	defer os.RemoveAll(dir)
	db, _ := rainstorm.Open(context.Background(), filepath.Join(dir, "rainstorm.db"))
	defer db.Close()

	err := db.NativeDB().Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucket([]byte("test"))
		require.NoError(t, err)

		idx, err := index.NewUniqueIndex(b, []byte("uindex1"))
		require.NoError(t, err)

		err = idx.Add(context.Background(), []byte("hello"), []byte("id1"))
		require.NoError(t, err)

		err = idx.Add(context.Background(), []byte("hello"), []byte("id1"))
		require.NoError(t, err)

		err = idx.Add(context.Background(), []byte("hello"), []byte("id2"))
		require.Error(t, err)
		require.ErrorIs(t, err, index.ErrAlreadyExists)

		err = idx.Add(context.Background(), nil, []byte("id2"))
		require.Error(t, err)
		require.ErrorIs(t, err, index.ErrNilParam)

		err = idx.Add(context.Background(), []byte("hi"), nil)
		require.Error(t, err)
		require.ErrorIs(t, err, index.ErrNilParam)

		id, _ := idx.Get(context.Background(), []byte("hello"))
		require.Equal(t, []byte("id1"), id)

		id, _ = idx.Get(context.Background(), []byte("goodbye"))
		require.Nil(t, id)

		err = idx.Remove(context.Background(), []byte("hello"))
		require.NoError(t, err)

		err = idx.Remove(context.Background(), nil)
		require.NoError(t, err)

		id, _ = idx.Get(context.Background(), []byte("hello"))
		require.Nil(t, id)

		err = idx.Add(context.Background(), []byte("hello"), []byte("id1"))
		require.NoError(t, err)

		err = idx.Add(context.Background(), []byte("hi"), []byte("id2"))
		require.NoError(t, err)

		err = idx.Add(context.Background(), []byte("yo"), []byte("id3"))
		require.NoError(t, err)

		list, err := idx.AllRecords(context.Background(), nil)
		require.NoError(t, err)
		require.Len(t, list, 3)

		opts := index.NewOptions()
		opts.Limit = 2
		list, err = idx.AllRecords(context.Background(), opts)
		require.NoError(t, err)
		require.Len(t, list, 2)

		opts = index.NewOptions()
		opts.Skip = 2
		list, err = idx.AllRecords(context.Background(), opts)
		require.NoError(t, err)
		require.Len(t, list, 1)
		require.Equal(t, []byte("id3"), list[0])

		opts = index.NewOptions()
		opts.Skip = 2
		opts.Limit = 1
		opts.Reverse = true
		list, err = idx.AllRecords(context.Background(), opts)
		require.NoError(t, err)
		require.Len(t, list, 1)
		require.Equal(t, []byte("id1"), list[0])

		err = idx.RemoveID(context.Background(), []byte("id2"))
		require.NoError(t, err)

		id, _ = idx.Get(context.Background(), []byte("hello"))
		require.Equal(t, []byte("id1"), id)
		id, _ = idx.Get(context.Background(), []byte("hi"))
		require.Nil(t, id)
		id, _ = idx.Get(context.Background(), []byte("yo"))
		require.Equal(t, []byte("id3"), id)
		ids, err := idx.All(context.Background(), []byte("yo"), nil)
		require.NoError(t, err)
		require.Len(t, ids, 1)
		require.Equal(t, []byte("id3"), ids[0])

		err = idx.RemoveID(context.Background(), []byte("id2"))
		require.NoError(t, err)
		err = idx.RemoveID(context.Background(), []byte("id4"))
		require.NoError(t, err)
		return nil
	})

	require.NoError(t, err)
}

func TestUniqueIndexRange(t *testing.T) {
	dir, _ := os.MkdirTemp(os.TempDir(), "rainstorm")
	defer os.RemoveAll(dir)
	db, _ := rainstorm.Open(context.Background(), filepath.Join(dir, "rainstorm.db"))
	defer db.Close()

	db.NativeDB().Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucket([]byte("test"))
		require.NoError(t, err)

		idx, err := index.NewUniqueIndex(b, []byte("uindex1"))
		require.NoError(t, err)

		for i := 0; i < 10; i++ {
			val, _ := gob.Codec.Marshal(i)
			err = idx.Add(context.Background(), val, val)
			require.NoError(t, err)
		}

		min, _ := gob.Codec.Marshal(3)
		max, _ := gob.Codec.Marshal(5)
		list, err := idx.Range(context.Background(), min, max, nil)
		require.Len(t, list, 3)
		require.NoError(t, err)
		assertEncodedIntListEqual(t, []int{3, 4, 5}, list)

		min, _ = gob.Codec.Marshal(11)
		max, _ = gob.Codec.Marshal(20)
		list, err = idx.Range(context.Background(), min, max, nil)
		require.Len(t, list, 0)
		require.NoError(t, err)

		min, _ = gob.Codec.Marshal(7)
		max, _ = gob.Codec.Marshal(2)
		list, err = idx.Range(context.Background(), min, max, nil)
		require.Len(t, list, 0)
		require.NoError(t, err)

		min, _ = gob.Codec.Marshal(-5)
		max, _ = gob.Codec.Marshal(2)
		list, err = idx.Range(context.Background(), min, max, nil)
		require.Len(t, list, 0)
		require.NoError(t, err)

		min, _ = gob.Codec.Marshal(3)
		max, _ = gob.Codec.Marshal(7)
		opts := index.NewOptions()
		opts.Skip = 2
		list, err = idx.Range(context.Background(), min, max, opts)
		require.Len(t, list, 3)
		require.NoError(t, err)
		assertEncodedIntListEqual(t, []int{5, 6, 7}, list)

		opts = index.NewOptions()
		opts.Limit = 2
		list, err = idx.Range(context.Background(), min, max, opts)
		require.Len(t, list, 2)
		require.NoError(t, err)
		assertEncodedIntListEqual(t, []int{3, 4}, list)

		opts = index.NewOptions()
		opts.Reverse = true
		opts.Skip = 2
		opts.Limit = 2
		list, err = idx.Range(context.Background(), min, max, opts)
		require.Len(t, list, 2)
		require.NoError(t, err)
		assertEncodedIntListEqual(t, []int{5, 4}, list)
		return nil
	})
}

func TestUniqueIndexPrefix(t *testing.T) {
	dir, _ := os.MkdirTemp(os.TempDir(), "rainstorm")
	defer os.RemoveAll(dir)
	db, _ := rainstorm.Open(context.Background(), filepath.Join(dir, "rainstorm.db"))
	defer db.Close()

	db.NativeDB().Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucket([]byte("test"))
		require.NoError(t, err)

		idx, err := index.NewUniqueIndex(b, []byte("uindex1"))
		require.NoError(t, err)

		for i := 0; i < 10; i++ {
			val := []byte(fmt.Sprintf("a%d", i))
			err = idx.Add(context.Background(), val, val)
			require.NoError(t, err)
		}

		for i := 0; i < 10; i++ {
			val := []byte(fmt.Sprintf("b%d", i))
			err = idx.Add(context.Background(), val, val)
			require.NoError(t, err)
		}

		list, err := idx.Prefix(context.Background(), []byte("a"), nil)
		require.Len(t, list, 10)
		require.NoError(t, err)

		list, err = idx.Prefix(context.Background(), []byte("b"), nil)
		require.Len(t, list, 10)
		require.NoError(t, err)
		require.Equal(t, []byte("b0"), list[0])
		require.Equal(t, []byte("b9"), list[9])

		opts := index.NewOptions()
		opts.Reverse = true
		list, err = idx.Prefix(context.Background(), []byte("a"), opts)
		require.Len(t, list, 10)
		require.NoError(t, err)
		require.Equal(t, []byte("a9"), list[0])
		require.Equal(t, []byte("a0"), list[9])

		opts = index.NewOptions()
		opts.Reverse = true
		list, err = idx.Prefix(context.Background(), []byte("b"), opts)
		require.Len(t, list, 10)
		require.NoError(t, err)
		require.Equal(t, []byte("b9"), list[0])
		require.Equal(t, []byte("b0"), list[9])

		opts = index.NewOptions()
		opts.Skip = 9
		opts.Limit = 5
		list, err = idx.Prefix(context.Background(), []byte("a"), opts)
		require.Len(t, list, 1)
		require.NoError(t, err)
		require.Equal(t, []byte("a9"), list[0])

		opts = index.NewOptions()
		opts.Reverse = true
		opts.Skip = 9
		opts.Limit = 5
		list, err = idx.Prefix(context.Background(), []byte("a"), opts)
		require.Len(t, list, 1)
		require.NoError(t, err)
		require.Equal(t, []byte("a0"), list[0])
		return nil
	})
}

func assertEncodedIntListEqual(t *testing.T, expected []int, actual [][]byte) {
	ints := make([]int, len(actual))

	for i, e := range actual {
		err := gob.Codec.Unmarshal(e, &ints[i])
		require.NoError(t, err)
	}

	require.Equal(t, expected, ints)
}
