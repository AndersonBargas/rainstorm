package rainstorm

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/AndersonBargas/rainstorm/v6/index"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

// ============================================================================
// R6.4C1 — Error taxonomy and sentinel classification tests
// ============================================================================

// ---------------------------------------------------------------------------
// 1. Shared sentinel identity
// ---------------------------------------------------------------------------

func TestSharedSentinelIdentity(t *testing.T) {
	t.Run("ErrNotFound", func(t *testing.T) {
		require.Equal(t, index.ErrNotFound, ErrNotFound,
			"rainstorm.ErrNotFound must be identical to index.ErrNotFound")
	})
	t.Run("ErrAlreadyExists", func(t *testing.T) {
		require.Equal(t, index.ErrAlreadyExists, ErrAlreadyExists,
			"rainstorm.ErrAlreadyExists must be identical to index.ErrAlreadyExists")
	})
	t.Run("ErrNilParam", func(t *testing.T) {
		require.Equal(t, index.ErrNilParam, ErrNilParam,
			"rainstorm.ErrNilParam must be identical to index.ErrNilParam")
	})
	t.Run("ErrNilContext", func(t *testing.T) {
		require.Equal(t, index.ErrNilContext, ErrNilContext,
			"rainstorm.ErrNilContext must be identical to index.ErrNilContext")
	})
}

func TestSharedSentinelErrorsIs(t *testing.T) {
	// Prove errors.Is works in both directions for direct values.
	require.ErrorIs(t, index.ErrNotFound, ErrNotFound)
	require.ErrorIs(t, ErrNotFound, index.ErrNotFound)

	require.ErrorIs(t, index.ErrAlreadyExists, ErrAlreadyExists)
	require.ErrorIs(t, ErrAlreadyExists, index.ErrAlreadyExists)

	require.ErrorIs(t, index.ErrNilParam, ErrNilParam)
	require.ErrorIs(t, ErrNilParam, index.ErrNilParam)

	require.ErrorIs(t, index.ErrNilContext, ErrNilContext)
	require.ErrorIs(t, ErrNilContext, index.ErrNilContext)
}

func TestSharedSentinelWrappedErrorsIs(t *testing.T) {
	// Prove errors.Is works through wrapping in both directions.
	checks := []struct {
		name string
		err  error
	}{
		{"ErrNotFound", index.ErrNotFound},
		{"ErrAlreadyExists", index.ErrAlreadyExists},
		{"ErrNilParam", index.ErrNilParam},
		{"ErrNilContext", index.ErrNilContext},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			wrapped := fmt.Errorf("test: %w", c.err)
			require.ErrorIs(t, wrapped, c.err)
			// Also prove root alias works.
			switch c.name {
			case "ErrNotFound":
				require.ErrorIs(t, wrapped, ErrNotFound)
			case "ErrAlreadyExists":
				require.ErrorIs(t, wrapped, ErrAlreadyExists)
			case "ErrNilParam":
				require.ErrorIs(t, wrapped, ErrNilParam)
			case "ErrNilContext":
				require.ErrorIs(t, wrapped, ErrNilContext)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 2. Nil context — root operations
// ---------------------------------------------------------------------------

func TestNilContext_RootOperations(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	// Seed data so operations that need existing records are testable.
	ctx := context.Background()
	var nilCtx context.Context
	require.NoError(t, db.Save(ctx, &User{ID: 1, Name: "test", Slug: "t1"}))

	t.Run("Open", func(t *testing.T) {
		db2, err := Open(nilCtx, filepath.Join(t.TempDir(), "test.db"))
		require.ErrorIs(t, err, ErrNilContext)
		require.NotErrorIs(t, err, ErrNilParam)
		require.Nil(t, db2)
	})

	t.Run("Save", func(t *testing.T) {
		err := db.Save(nilCtx, &User{ID: 2, Name: "x", Slug: "x"})
		require.ErrorIs(t, err, ErrNilContext)
		require.NotErrorIs(t, err, ErrNilParam)
	})

	t.Run("One", func(t *testing.T) {
		var u User
		err := db.One(nilCtx, "ID", 1, &u)
		require.ErrorIs(t, err, ErrNilContext)
		require.NotErrorIs(t, err, ErrNilParam)
	})

	t.Run("Find", func(t *testing.T) {
		var us []User
		err := db.Find(nilCtx, "ID", 1, &us)
		require.ErrorIs(t, err, ErrNilContext)
		require.NotErrorIs(t, err, ErrNilParam)
	})

	t.Run("QueryFind", func(t *testing.T) {
		var scores []Score
		err := db.Select().Find(nilCtx, &scores)
		require.ErrorIs(t, err, ErrNilContext)
		require.NotErrorIs(t, err, ErrNilParam)
	})

	t.Run("QueryFirst", func(t *testing.T) {
		var s Score
		err := db.Select().First(nilCtx, &s)
		require.ErrorIs(t, err, ErrNilContext)
		require.NotErrorIs(t, err, ErrNilParam)
	})

	t.Run("QueryCount", func(t *testing.T) {
		_, err := db.Select().Count(nilCtx, &Score{})
		require.ErrorIs(t, err, ErrNilContext)
		require.NotErrorIs(t, err, ErrNilParam)
	})

	t.Run("QueryDelete", func(t *testing.T) {
		err := db.Select().Delete(nilCtx, &Score{})
		require.ErrorIs(t, err, ErrNilContext)
		require.NotErrorIs(t, err, ErrNilParam)
	})

	t.Run("QueryRaw", func(t *testing.T) {
		raw, err := db.Select().Bucket("User").Raw(nilCtx)
		require.ErrorIs(t, err, ErrNilContext)
		require.NotErrorIs(t, err, ErrNilParam)
		require.Nil(t, raw, "Raw must return nil result on nil context")
	})

	t.Run("KVGet", func(t *testing.T) {
		var s string
		err := db.Get(nilCtx, "bucket", "key", &s)
		require.ErrorIs(t, err, ErrNilContext)
		require.NotErrorIs(t, err, ErrNilParam)
	})

	t.Run("KVSet", func(t *testing.T) {
		err := db.Set(nilCtx, "bucket", "key", "val")
		require.ErrorIs(t, err, ErrNilContext)
		require.NotErrorIs(t, err, ErrNilParam)
	})

	t.Run("KVDelete", func(t *testing.T) {
		err := db.Delete(nilCtx, "bucket", "key")
		require.ErrorIs(t, err, ErrNilContext)
		require.NotErrorIs(t, err, ErrNilParam)
	})

	t.Run("KVGetBytes", func(t *testing.T) {
		b, err := db.GetBytes(nilCtx, "bucket", "key")
		require.ErrorIs(t, err, ErrNilContext)
		require.NotErrorIs(t, err, ErrNilParam)
		require.Nil(t, b, "GetBytes must return nil result on nil context")
	})

	t.Run("KVSetBytes", func(t *testing.T) {
		err := db.SetBytes(nilCtx, "bucket", "key", []byte("val"))
		require.ErrorIs(t, err, ErrNilContext)
		require.NotErrorIs(t, err, ErrNilParam)
	})

	t.Run("PrefixScan", func(t *testing.T) {
		nodes, err := db.PrefixScan(nilCtx, "p")
		require.ErrorIs(t, err, ErrNilContext)
		require.NotErrorIs(t, err, ErrNilParam)
		require.Nil(t, nodes, "PrefixScan must return nil result on nil context")
	})

	t.Run("RangeScan", func(t *testing.T) {
		nodes, err := db.RangeScan(nilCtx, "a", "z")
		require.ErrorIs(t, err, ErrNilContext)
		require.NotErrorIs(t, err, ErrNilParam)
		require.Nil(t, nodes, "RangeScan must return nil result on nil context")
	})

	t.Run("ReadTransaction", func(t *testing.T) {
		ran := false
		err := db.ReadTransaction(nilCtx, func(Node) error {
			ran = true
			return nil
		})
		require.ErrorIs(t, err, ErrNilContext)
		require.NotErrorIs(t, err, ErrNilParam)
		require.False(t, ran, "callback must not execute under nil context")
	})

	t.Run("WriteTransaction", func(t *testing.T) {
		ran := false
		err := db.WriteTransaction(nilCtx, func(Node) error {
			ran = true
			return nil
		})
		require.ErrorIs(t, err, ErrNilContext)
		require.NotErrorIs(t, err, ErrNilParam)
		require.False(t, ran, "callback must not execute under nil context")
	})
}

// ---------------------------------------------------------------------------
// 3. Nil context — index operations
// ---------------------------------------------------------------------------

func TestNilContext_IndexOperations(t *testing.T) {
	bDB, err := bolt.Open(filepath.Join(t.TempDir(), "test.db"), 0600, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, bDB.Close())
	})

	// Create a UniqueIndex inside a writable transaction.
	var idx *index.UniqueIndex
	err = bDB.Update(func(tx *bolt.Tx) error {
		b, cerr := tx.CreateBucketIfNotExists([]byte("test"))
		if cerr != nil {
			return cerr
		}
		idx, err = index.NewUniqueIndex(b, []byte("idx"))
		return err
	})
	require.NoError(t, err)
	require.NotNil(t, idx)
	var nilCtx context.Context

	t.Run("UniqueIndexAdd", func(t *testing.T) {
		err := idx.Add(nilCtx, []byte("val"), []byte{1})
		require.ErrorIs(t, err, index.ErrNilContext)
		require.NotErrorIs(t, err, index.ErrNilParam)
	})

	t.Run("UniqueIndexGet", func(t *testing.T) {
		_, err := idx.Get(nilCtx, []byte("val"))
		require.ErrorIs(t, err, index.ErrNilContext)
		require.NotErrorIs(t, err, index.ErrNilParam)
	})
}

// ---------------------------------------------------------------------------
// 4. Nil parameter distinction (valid context + nil param → ErrNilParam)
// ---------------------------------------------------------------------------

func TestNilParam_DistinctFromNilContext(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("WriteTransactionNilCallback", func(t *testing.T) {
		err := db.WriteTransaction(ctx, nil)
		require.ErrorIs(t, err, ErrNilParam)
		require.NotErrorIs(t, err, ErrNilContext)
	})

	t.Run("ReadTransactionNilCallback", func(t *testing.T) {
		err := db.ReadTransaction(ctx, nil)
		require.ErrorIs(t, err, ErrNilParam)
		require.NotErrorIs(t, err, ErrNilContext)
	})

	t.Run("QueryRawEachNilCallback", func(t *testing.T) {
		err := db.Select().Bucket("User").RawEach(ctx, nil)
		require.ErrorIs(t, err, ErrNilParam)
		require.NotErrorIs(t, err, ErrNilContext)
	})

	t.Run("QueryEachNilCallback", func(t *testing.T) {
		err := db.Select().Each(ctx, new(User), nil)
		require.ErrorIs(t, err, ErrNilParam)
		require.NotErrorIs(t, err, ErrNilContext)
	})

	t.Run("IndexUniqueAddNilValue", func(t *testing.T) {
		bDB, err := bolt.Open(filepath.Join(t.TempDir(), "test.db"), 0600, nil)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, bDB.Close())
		})

		var idx *index.UniqueIndex
		err = bDB.Update(func(tx *bolt.Tx) error {
			b, cerr := tx.CreateBucketIfNotExists([]byte("test"))
			if cerr != nil {
				return cerr
			}
			idx, err = index.NewUniqueIndex(b, []byte("idx"))
			return err
		})
		require.NoError(t, err)

		// Nil value with valid context and valid targetID → ErrNilParam
		err = idx.Add(ctx, nil, []byte{1})
		require.ErrorIs(t, err, index.ErrNilParam)
		require.NotErrorIs(t, err, index.ErrNilContext)
	})
}

// ---------------------------------------------------------------------------
// 5. Existing classification proofs
// ---------------------------------------------------------------------------

func TestClassification_DuplicateUniqueValue(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()
	require.NoError(t, db.Save(ctx, &UniqueNameUser{ID: 1, Name: "alice", Age: 30}))
	err := db.Save(ctx, &UniqueNameUser{ID: 2, Name: "alice", Age: 25})
	require.ErrorIs(t, err, ErrAlreadyExists)
}

func TestClassification_MissingRecord(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()
	var u User
	err := db.One(ctx, "ID", 999, &u)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestClassification_MissingIndex(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()
	require.NoError(t, db.Save(ctx, &User{ID: 1, Name: "test", Slug: "idx"}))

	err := db.NativeDB().View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("User"))
		if bucket == nil {
			return ErrNotFound
		}
		_, err := getIndex(bucket, "unsupported-index-kind", "Name")
		return err
	})
	require.ErrorIs(t, err, ErrIdxNotFound)
}

func TestClassification_IncompatibleValue(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()
	require.NoError(t, db.Save(ctx, &User{ID: 1, Name: "test", Slug: "s"}))

	// Setting a value with a different type than the field should return ErrIncompatibleValue.
	err := db.UpdateField(ctx, &User{ID: 1}, "Name", 42) // Name is string, 42 is int
	require.ErrorIs(t, err, ErrIncompatibleValue)
}

func TestClassification_InvalidDestinationShape(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("FindWithNonSlicePtr", func(t *testing.T) {
		err := db.Find(ctx, "Name", "x", &User{})
		require.ErrorIs(t, err, ErrSlicePtrNeeded)
	})

	t.Run("SaveWithNonStructPtr", func(t *testing.T) {
		err := db.Save(ctx, User{ID: 1})
		require.True(t, errors.Is(err, ErrStructPtrNeeded) || errors.Is(err, ErrBadType))
	})

	t.Run("GetWithNonPtr", func(t *testing.T) {
		var s string
		err := db.Get(ctx, "bucket", "key", s) // not a pointer
		require.ErrorIs(t, err, ErrPtrNeeded)
	})
}

func TestClassification_CanceledContextNotErrNilContext(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := db.Save(ctx, &User{ID: 1, Name: "x", Slug: "x"})
	require.ErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, ErrNilContext)
}

func TestClassification_ExpiredContextNotErrNilContext(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	err := db.Save(timedOutCtx(), &User{ID: 1, Name: "x", Slug: "x"})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NotErrorIs(t, err, ErrNilContext)
}

func TestClassification_ClosedOwnedNativeDB(t *testing.T) {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "closed.db"))
	require.NoError(t, err)

	// Close the underlying bolt.DB.
	require.NoError(t, db.NativeDB().Close())

	ctx := context.Background()
	err = db.Save(ctx, &User{ID: 1, Name: "x", Slug: "x"})
	require.ErrorIs(t, err, bolt.ErrDatabaseNotOpen)
}
