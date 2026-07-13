package rainstorm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AndersonBargas/rainstorm/v6/codec/gob"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

// canceledCtx returns a context that is already canceled.
func canceledCtx() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// timedOutCtx returns a context that has already exceeded its deadline.
func timedOutCtx() context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	<-ctx.Done()
	return ctx
}

// ============================================================================
// 16.1 Open
// ============================================================================

func TestOpen_CancelledContextDoesNotOpenDatabase(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "rainstorm")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "rainstorm.db")

	db, err := Open(canceledCtx(), path)
	require.Error(t, err)
	require.Nil(t, db)
	require.True(t, errors.Is(err, context.Canceled))

	// The database file must not have been opened.
	_, statErr := os.Stat(path)
	require.True(t, os.IsNotExist(statErr), "database file should not exist after canceled Open")
}

func TestOpen_CancelledAfterOpenClosesOwnedDatabase(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "rainstorm")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "rainstorm.db")

	// Use the deterministic post-open test seam. The hook cancels the context
	// immediately after Rainstorm has opened the owned bolt.DB but before the
	// post-open context check and version initialization. No time.Sleep.
	ctx, cancel := context.WithCancel(context.Background())
	cancelAfterOpen := OpenOption(func(opts *Options) error {
		opts.postOpenHook = func(_ context.Context) {
			cancel()
		}
		return nil
	})

	db, err := Open(ctx, path, cancelAfterOpen)
	require.Error(t, err)
	require.Nil(t, db)
	require.True(t, errors.Is(err, context.Canceled))

	// The owned bolt.DB must have been closed so that the data file is no longer
	// exclusively held. We can reopen it from scratch.
	db2, err := Open(context.Background(), path)
	require.NoError(t, err)
	require.NotNil(t, db2)
	require.NoError(t, db2.Close())
}

func TestOpen_InitializationErrorDoesNotCloseBorrowedDatabase(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "rainstorm")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	bDBPath := filepath.Join(dir, "borrowed.db")

	// Step 1: use a non-default codec (gob) to open the borrowed DB so that the
	// __rainstorm_db metadata records gob as the codec.
	bDB, err := bolt.Open(bDBPath, 0600, &bolt.Options{Timeout: 10 * time.Second})
	require.NoError(t, err)

	// Pre-seed the version bucket with the gob codec marker so the default
	// (json) codec used by the second Open will conflict during checkVersion.
	err = bDB.Update(func(tx *bolt.Tx) error {
		top, err := tx.CreateBucketIfNotExists([]byte(dbinfo))
		if err != nil {
			return err
		}
		// Mirror what newMeta writes: a nested metadata bucket carrying the
		// codec name. Set via gob's codec name to force ErrDifferentCodec.
		_, err = top.CreateBucket([]byte(metadataBucket))
		if err != nil {
			return err
		}
		mb := top.Bucket([]byte(metadataBucket))
		return mb.Put([]byte(metaCodec), []byte(gob.Codec.Name()))
	})
	require.NoError(t, err)

	// Step 2: hand the borrowed DB to Rainstorm with the default (json) codec.
	// checkVersion will call Set on __rainstorm_db, which builds the data
	// bucket via setBytes -> newMeta, and newMeta will compare codec names and
	// return ErrDifferentCodec. That is our deterministic initialization error.
	db, err := Open(context.Background(), "", UseDB(bDB))
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrDifferentCodec), "expected ErrDifferentCodec, got %v", err)
	require.Nil(t, db)

	// The borrowed bolt.DB must NOT have been closed by Rainstorm. We confirm
	// by performing a fresh close on bDB directly: if Rainstorm had closed it,
	// this would fail with "database not open".
	require.NoError(t, bDB.Close())

	// And we can reopen the file from scratch, proving it is not locked.
	bDB2, err := bolt.Open(bDBPath, 0600, &bolt.Options{Timeout: 10 * time.Second})
	require.NoError(t, err)
	require.NoError(t, bDB2.Close())
}

// ============================================================================
// 16.2 Struct operations
// ============================================================================

func TestStructOps_CancelledContextPreventsWork(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	// Seed one record so update/delete targets exist on the happy path.
	require.NoError(t, db.Save(context.Background(), &User{ID: 1, Name: "seed"}))

	tests := []struct {
		name string
		op   func(ctx context.Context) error
		// verifyNoPersist, when not nil, confirms that no record was written.
		verifyNoPersist func(t *testing.T)
	}{
		{
			name: "Init",
			op: func(ctx context.Context) error {
				return db.Init(ctx, &IndexedNameUser{})
			},
			verifyNoPersist: func(t *testing.T) {
				err := db.NativeDB().View(func(tx *bolt.Tx) error {
					require.Nil(t, tx.Bucket([]byte("IndexedNameUser")))
					return nil
				})
				require.NoError(t, err)
			},
		},
		{
			name: "ReIndex",
			op: func(ctx context.Context) error {
				return db.ReIndex(ctx, &User{})
			},
		},
		{
			name: "Save",
			op: func(ctx context.Context) error {
				return db.Save(ctx, &User{ID: 42, Name: "canceled"})
			},
			verifyNoPersist: func(t *testing.T) {
				var u User
				err := db.One(context.Background(), "ID", 42, &u)
				require.ErrorIs(t, err, ErrNotFound)
			},
		},
		{
			name: "Update",
			op: func(ctx context.Context) error {
				return db.Update(ctx, &User{ID: 1, Name: "updated"})
			},
			verifyNoPersist: func(t *testing.T) {
				var u User
				require.NoError(t, db.One(context.Background(), "ID", 1, &u))
				require.Equal(t, "seed", u.Name)
			},
		},
		{
			name: "UpdateField",
			op: func(ctx context.Context) error {
				return db.UpdateField(ctx, &User{ID: 1}, "Name", "updated")
			},
			verifyNoPersist: func(t *testing.T) {
				var u User
				require.NoError(t, db.One(context.Background(), "ID", 1, &u))
				require.Equal(t, "seed", u.Name)
			},
		},
		{
			name: "Drop",
			op: func(ctx context.Context) error {
				return db.Drop(ctx, "User")
			},
			verifyNoPersist: func(t *testing.T) {
				count, err := db.Count(context.Background(), &User{})
				require.NoError(t, err)
				require.Positive(t, count)
			},
		},
		{
			name: "DeleteStruct",
			op: func(ctx context.Context) error {
				return db.DeleteStruct(ctx, &User{ID: 1, Name: "seed"})
			},
			verifyNoPersist: func(t *testing.T) {
				count, err := db.Count(context.Background(), &User{})
				require.NoError(t, err)
				require.Positive(t, count)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.op(canceledCtx())
			require.Error(t, err)
			require.True(t, errors.Is(err, context.Canceled), "expected context.Canceled, got %v", err)
			if tc.verifyNoPersist != nil {
				tc.verifyNoPersist(t)
			}
		})
	}

	// Also verify a timed-out context returns context.DeadlineExceeded.
	err := db.Save(timedOutCtx(), &User{ID: 7, Name: "x"})
	require.Error(t, err)
	require.True(t, errors.Is(err, context.DeadlineExceeded))
}

// ============================================================================
// 16.3 Finder and Query
// ============================================================================

func TestFinder_CancelledContextPreventsReads(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()
	require.NoError(t, db.Save(ctx, &IndexedNameUser{ID: 1, Name: "John"}))

	t.Run("One", func(t *testing.T) {
		var u IndexedNameUser
		err := db.One(canceledCtx(), "Name", "John", &u)
		require.Error(t, err)
		require.True(t, errors.Is(err, context.Canceled))
	})

	t.Run("Find", func(t *testing.T) {
		var us []IndexedNameUser
		err := db.Find(canceledCtx(), "Name", "John", &us)
		require.Error(t, err)
		require.True(t, errors.Is(err, context.Canceled))
	})

	t.Run("All", func(t *testing.T) {
		var us []IndexedNameUser
		err := db.All(canceledCtx(), &us)
		require.Error(t, err)
		require.True(t, errors.Is(err, context.Canceled))
	})

	t.Run("Count", func(t *testing.T) {
		_, err := db.Count(canceledCtx(), &IndexedNameUser{})
		require.Error(t, err)
		require.True(t, errors.Is(err, context.Canceled))
	})
}

func TestQuery_CancelledContextPreventsTerminals(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()
	require.NoError(t, db.Save(ctx, &Score{Value: 1}))

	t.Run("Find", func(t *testing.T) {
		var scores []Score
		err := db.Select().Find(canceledCtx(), &scores)
		require.Error(t, err)
		require.True(t, errors.Is(err, context.Canceled))
	})

	t.Run("First", func(t *testing.T) {
		var s Score
		err := db.Select().First(canceledCtx(), &s)
		require.Error(t, err)
		require.True(t, errors.Is(err, context.Canceled))
	})

	t.Run("Delete", func(t *testing.T) {
		err := db.Select().Delete(canceledCtx(), &Score{})
		require.Error(t, err)
		require.True(t, errors.Is(err, context.Canceled))
	})

	t.Run("Count", func(t *testing.T) {
		_, err := db.Select().Count(canceledCtx(), &Score{})
		require.Error(t, err)
		require.True(t, errors.Is(err, context.Canceled))
	})

	t.Run("Raw", func(t *testing.T) {
		_, err := db.Select().Bucket("Score").Raw(canceledCtx())
		require.Error(t, err)
		require.True(t, errors.Is(err, context.Canceled))
	})

	t.Run("RawEach", func(t *testing.T) {
		called := false
		err := db.Select().Bucket("Score").RawEach(canceledCtx(), func(k, v []byte) error {
			called = true
			return nil
		})
		require.Error(t, err)
		require.True(t, errors.Is(err, context.Canceled))
		require.False(t, called, "callback must not run under a canceled context")
	})

	t.Run("Each", func(t *testing.T) {
		called := false
		err := db.Select().Each(canceledCtx(), new(Score), func(_ interface{}) error {
			called = true
			return nil
		})
		require.Error(t, err)
		require.True(t, errors.Is(err, context.Canceled))
		require.False(t, called, "callback must not run under a canceled context")
	})
}

// ============================================================================
// 16.4 KV
// ============================================================================

func TestKV_CancelledContextPreventsWork(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()
	require.NoError(t, db.Set(ctx, "trash", "k", []byte("v")))

	t.Run("Get", func(t *testing.T) {
		var b []byte
		err := db.Get(canceledCtx(), "trash", "k", &b)
		require.Error(t, err)
		require.True(t, errors.Is(err, context.Canceled))
	})

	t.Run("Set", func(t *testing.T) {
		err := db.Set(canceledCtx(), "trash", "newkey", "newvalue")
		require.Error(t, err)
		require.True(t, errors.Is(err, context.Canceled))

		var s string
		gErr := db.Get(ctx, "trash", "newkey", &s)
		require.Error(t, gErr)
		require.ErrorIs(t, gErr, ErrNotFound, "canceled Set must not persist")
	})

	t.Run("Delete", func(t *testing.T) {
		err := db.Delete(canceledCtx(), "trash", "k")
		require.Error(t, err)
		require.True(t, errors.Is(err, context.Canceled))
	})

	t.Run("GetBytes", func(t *testing.T) {
		_, err := db.GetBytes(canceledCtx(), "trash", "k")
		require.Error(t, err)
		require.True(t, errors.Is(err, context.Canceled))
	})

	t.Run("SetBytes", func(t *testing.T) {
		err := db.SetBytes(canceledCtx(), "trash", "newk", []byte("newv"))
		require.Error(t, err)
		require.True(t, errors.Is(err, context.Canceled))

		_, gErr := db.GetBytes(ctx, "trash", "newk")
		require.ErrorIs(t, gErr, ErrNotFound, "canceled SetBytes must not persist")
	})

	t.Run("KeyExists", func(t *testing.T) {
		_, err := db.KeyExists(canceledCtx(), "trash", "k")
		require.Error(t, err)
		require.True(t, errors.Is(err, context.Canceled))
	})
}

// ============================================================================
// 16.5 Scanner
// ============================================================================

func TestScanner_CancelledContextReturnsErrorAndNoResults(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()
	node := db.From("node")
	require.NoError(t, node.Save(ctx, &SimpleUser{ID: 1, Name: "John"}))

	t.Run("PrefixScan", func(t *testing.T) {
		nodes, err := node.PrefixScan(canceledCtx(), "2")
		require.Error(t, err)
		require.True(t, errors.Is(err, context.Canceled))
		require.Empty(t, nodes)
	})

	t.Run("RangeScan", func(t *testing.T) {
		nodes, err := node.RangeScan(canceledCtx(), "1", "9")
		require.Error(t, err)
		require.True(t, errors.Is(err, context.Canceled))
		require.Empty(t, nodes)
	})
}

// ============================================================================
// 16.6 Manual transactions
// ============================================================================

// Manual transactions (Begin/Commit/Rollback) were removed in R6.4A.
// All transactional behavior is now exercised via managed transactions
// (ReadTransaction / WriteTransaction). See managed_transaction_test.go
// and managed_transaction_lifecycle_test.go.
