package index_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/AndersonBargas/rainstorm/v6"
	"github.com/AndersonBargas/rainstorm/v6/index"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

// ---------------------------------------------------------------------------
// stepContext — deterministic thread-safe context for cancellation testing
// ---------------------------------------------------------------------------

type stepContext struct {
	mu       sync.Mutex
	calls    int
	cancelAt int
}

func newStepContext(cancelAt int) *stepContext {
	return &stepContext{cancelAt: cancelAt}
}

func (c *stepContext) Calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func (c *stepContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *stepContext) Done() <-chan struct{}       { return nil }

func (c *stepContext) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if c.calls >= c.cancelAt {
		return context.Canceled
	}
	return nil
}

func (c *stepContext) Value(key any) any { return nil }

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

type testDB struct {
	db  *rainstorm.DB
	dir string
}

func openTestDB(t *testing.T) *testDB {
	t.Helper()
	dir, err := os.MkdirTemp(os.TempDir(), "rainstorm-index")
	require.NoError(t, err)
	db, err := rainstorm.Open(context.Background(), filepath.Join(dir, "rainstorm.db"))
	require.NoError(t, err)
	return &testDB{db: db, dir: dir}
}

func (td *testDB) cleanup(t *testing.T) {
	t.Helper()
	closeErr := td.db.Close()
	removeErr := os.RemoveAll(td.dir)
	require.NoError(t, errors.Join(closeErr, removeErr))
}

func createBucket(t *testing.T, tx *bolt.Tx, name string) *bolt.Bucket {
	t.Helper()
	b, err := tx.CreateBucket([]byte(name))
	require.NoError(t, err)
	return b
}

func newIDIndex(t *testing.T, b *bolt.Bucket, name string) index.Index {
	t.Helper()
	idx, err := index.NewIDIndex(b, []byte(name))
	require.NoError(t, err)
	return idx
}

func newListIndex(t *testing.T, b *bolt.Bucket, name string) index.Index {
	t.Helper()
	idx, err := index.NewListIndex(b, []byte(name))
	require.NoError(t, err)
	return idx
}

func newUniqueIndex(t *testing.T, b *bolt.Bucket, name string) index.Index {
	t.Helper()
	idx, err := index.NewUniqueIndex(b, []byte(name))
	require.NoError(t, err)
	return idx
}

// ---------------------------------------------------------------------------
// 13.1 Cancelled context rejected — table-driven
// ---------------------------------------------------------------------------

func TestIndexes_CancelledContextRejected(t *testing.T) {
	td := openTestDB(t)
	defer td.cleanup(t)

	cancelled := newStepContext(0)

	err := td.db.NativeDB().Update(func(tx *bolt.Tx) error {
		b := createBucket(t, tx, "test")

		for _, tc := range []struct {
			name string
			newI func() index.Index
		}{
			{"IDIndex", func() index.Index {
				return newIDIndex(t, b, "idx_id")
			}},
			{"ListIndex", func() index.Index {
				return newListIndex(t, b, "idx_list")
			}},
			{"UniqueIndex", func() index.Index {
				return newUniqueIndex(t, b, "idx_uniq")
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				idx := tc.newI()

				t.Run("Add", func(t *testing.T) {
					require.ErrorIs(t, idx.Add(cancelled, []byte("v"), []byte("id")), context.Canceled)
				})
				t.Run("Remove", func(t *testing.T) {
					require.ErrorIs(t, idx.Remove(cancelled, []byte("v")), context.Canceled)
				})
				t.Run("RemoveID", func(t *testing.T) {
					require.ErrorIs(t, idx.RemoveID(cancelled, []byte("id")), context.Canceled)
				})
				t.Run("Get", func(t *testing.T) {
					r, e := idx.Get(cancelled, []byte("v"))
					require.ErrorIs(t, e, context.Canceled)
					require.Nil(t, r)
				})
				t.Run("All", func(t *testing.T) {
					r, e := idx.All(cancelled, []byte("v"), nil)
					require.ErrorIs(t, e, context.Canceled)
					require.Nil(t, r)
				})
				t.Run("AllRecords", func(t *testing.T) {
					r, e := idx.AllRecords(cancelled, nil)
					require.ErrorIs(t, e, context.Canceled)
					require.Nil(t, r)
				})
				t.Run("Range", func(t *testing.T) {
					r, e := idx.Range(cancelled, []byte("a"), []byte("z"), nil)
					require.ErrorIs(t, e, context.Canceled)
					require.Nil(t, r)
				})
				t.Run("Prefix", func(t *testing.T) {
					r, e := idx.Prefix(cancelled, []byte("p"), nil)
					require.ErrorIs(t, e, context.Canceled)
					require.Nil(t, r)
				})
				t.Run("cancelled beats invalid param", func(t *testing.T) {
					require.ErrorIs(t, idx.Add(cancelled, nil, []byte("id")), context.Canceled)
				})
			})
		}
		return nil
	})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// IDIndex: Range / Prefix cancellation
// ---------------------------------------------------------------------------

func TestIDIndex_RangeCancellationDiscardsPartialResults(t *testing.T) {
	td := openTestDB(t)
	defer td.cleanup(t)

	ctx := context.Background()
	err := td.db.NativeDB().Update(func(tx *bolt.Tx) error {
		b := createBucket(t, tx, "test")
		// IDIndex.Add is a no-op; write keys directly to the bucket.
		for i := 0; i < 20; i++ {
			val := []byte(fmt.Sprintf("a%02d", i))
			require.NoError(t, b.Put(val, val))
		}
		return nil
	})
	require.NoError(t, err)

	// cancelAt=5: entry (1) + 4 iterations (2-5) → cancelled at 5th call.
	{
		sc := newStepContext(5)
		result, err := callView(td.db, "test", "pk", func(idx index.Index) ([][]byte, error) {
			return idx.Range(sc, []byte("a00"), []byte("a99"), nil)
		})
		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, result)
		require.GreaterOrEqual(t, sc.Calls(), 3)
	}

	result, err := callView(td.db, "test", "pk", func(idx index.Index) ([][]byte, error) {
		return idx.Range(ctx, []byte("a00"), []byte("a99"), nil)
	})
	require.NoError(t, err)
	require.Len(t, result, 20)
}

func TestIDIndex_PrefixCancellationDiscardsPartialResults(t *testing.T) {
	td := openTestDB(t)
	defer td.cleanup(t)

	ctx := context.Background()
	err := td.db.NativeDB().Update(func(tx *bolt.Tx) error {
		b := createBucket(t, tx, "test")
		for i := 0; i < 20; i++ {
			val := []byte(fmt.Sprintf("a%d", i))
			require.NoError(t, b.Put(val, val))
		}
		return nil
	})
	require.NoError(t, err)

	{
		sc := newStepContext(6)
		result, err := callView(td.db, "test", "pk", func(idx index.Index) ([][]byte, error) {
			return idx.Prefix(sc, []byte("a"), nil)
		})
		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, result)
		require.GreaterOrEqual(t, sc.Calls(), 2)
	}

	result, err := callView(td.db, "test", "pk", func(idx index.Index) ([][]byte, error) {
		return idx.Prefix(ctx, []byte("a"), nil)
	})
	require.NoError(t, err)
	require.Len(t, result, 20)
}

// ---------------------------------------------------------------------------
// ListIndex: All / Remove / Range / Prefix cancellation
// ---------------------------------------------------------------------------

func TestListIndex_AllCancellationDiscardsPartialResults(t *testing.T) {
	td := openTestDB(t)
	defer td.cleanup(t)

	ctx := context.Background()
	err := td.db.NativeDB().Update(func(tx *bolt.Tx) error {
		b := createBucket(t, tx, "test")
		idx := newListIndex(t, b, "li")
		for i := 0; i < 10; i++ {
			require.NoError(t, idx.Add(ctx, []byte("hello"), []byte(fmt.Sprintf("id%d", i))))
		}
		return nil
	})
	require.NoError(t, err)

	{
		sc := newStepContext(5)
		result, err := callView(td.db, "test", "li", func(idx index.Index) ([][]byte, error) {
			return idx.All(sc, []byte("hello"), nil)
		})
		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, result)
		require.GreaterOrEqual(t, sc.Calls(), 3)
	}

	result, err := callView(td.db, "test", "li", func(idx index.Index) ([][]byte, error) {
		return idx.All(ctx, []byte("hello"), nil)
	})
	require.NoError(t, err)
	require.Len(t, result, 10)
}

func TestListIndex_RemoveCancellationReturnsError(t *testing.T) {
	td := openTestDB(t)
	defer td.cleanup(t)

	ctx := context.Background()
	require.NoError(t, td.db.NativeDB().Update(func(tx *bolt.Tx) error {
		b := createBucket(t, tx, "test")
		idx := newListIndex(t, b, "li")
		for i := 0; i < 5; i++ {
			require.NoError(t, idx.Add(ctx, []byte("hello"), []byte(fmt.Sprintf("id%d", i))))
		}
		return nil
	}))

	// cancelAt=4: entry (1), 2 cursor keys (2,3), first delete check (4) → cancel.
	sc := newStepContext(4)
	err := td.db.NativeDB().Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("test"))
		idx := newListIndex(t, b, "li")
		return idx.Remove(sc, []byte("hello"))
	})
	require.ErrorIs(t, err, context.Canceled)

	// Bolt.Update already rolled back. Verify original state.
	require.NoError(t, td.db.NativeDB().View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("test"))
		idx := newListIndex(t, b, "li")
		result, err := idx.All(ctx, []byte("hello"), nil)
		require.NoError(t, err)
		require.Len(t, result, 5)
		return nil
	}))
}

func TestListIndex_RangeCancellationDiscardsPartialResults(t *testing.T) {
	td := openTestDB(t)
	defer td.cleanup(t)

	ctx := context.Background()
	err := td.db.NativeDB().Update(func(tx *bolt.Tx) error {
		b := createBucket(t, tx, "test")
		idx := newListIndex(t, b, "li")
		for i := 0; i < 10; i++ {
			require.NoError(t, idx.Add(ctx, []byte(fmt.Sprintf("key%02d", i)), []byte(fmt.Sprintf("id%d", i))))
		}
		return nil
	})
	require.NoError(t, err)

	{
		sc := newStepContext(5)
		result, err := callView(td.db, "test", "li", func(idx index.Index) ([][]byte, error) {
			return idx.Range(sc, []byte("key00"), []byte("key99"), nil)
		})
		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, result)
		require.GreaterOrEqual(t, sc.Calls(), 3)
	}

	result, err := callView(td.db, "test", "li", func(idx index.Index) ([][]byte, error) {
		return idx.Range(ctx, []byte("key00"), []byte("key99"), nil)
	})
	require.NoError(t, err)
	require.Len(t, result, 10)
}

func TestListIndex_PrefixCancellationDiscardsPartialResults(t *testing.T) {
	td := openTestDB(t)
	defer td.cleanup(t)

	ctx := context.Background()
	err := td.db.NativeDB().Update(func(tx *bolt.Tx) error {
		b := createBucket(t, tx, "test")
		idx := newListIndex(t, b, "li")
		for i := 0; i < 10; i++ {
			require.NoError(t, idx.Add(ctx, []byte(fmt.Sprintf("pref%02d", i)), []byte(fmt.Sprintf("id%d", i))))
		}
		return nil
	})
	require.NoError(t, err)

	{
		sc := newStepContext(5)
		result, err := callView(td.db, "test", "li", func(idx index.Index) ([][]byte, error) {
			return idx.Prefix(sc, []byte("pref"), nil)
		})
		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, result)
	}

	result, err := callView(td.db, "test", "li", func(idx index.Index) ([][]byte, error) {
		return idx.Prefix(ctx, []byte("pref"), nil)
	})
	require.NoError(t, err)
	require.Len(t, result, 10)
}

// ---------------------------------------------------------------------------
// UniqueIndex: AllRecords / Range / Prefix cancellation
// ---------------------------------------------------------------------------

func TestUniqueIndex_AllRecordsCancellationDiscardsPartialResults(t *testing.T) {
	td := openTestDB(t)
	defer td.cleanup(t)

	ctx := context.Background()
	err := td.db.NativeDB().Update(func(tx *bolt.Tx) error {
		b := createBucket(t, tx, "test")
		idx := newUniqueIndex(t, b, "ui")
		for i := 0; i < 10; i++ {
			require.NoError(t, idx.Add(ctx, []byte(fmt.Sprintf("u%d", i)), []byte(fmt.Sprintf("id%d", i))))
		}
		return nil
	})
	require.NoError(t, err)

	{
		sc := newStepContext(5)
		result, err := callView(td.db, "test", "ui", func(idx index.Index) ([][]byte, error) {
			return idx.AllRecords(sc, nil)
		})
		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, result)
		require.GreaterOrEqual(t, sc.Calls(), 3)
	}

	result, err := callView(td.db, "test", "ui", func(idx index.Index) ([][]byte, error) {
		return idx.AllRecords(ctx, nil)
	})
	require.NoError(t, err)
	require.Len(t, result, 10)
}

func TestUniqueIndex_RangeCancellationDiscardsPartialResults(t *testing.T) {
	td := openTestDB(t)
	defer td.cleanup(t)

	ctx := context.Background()
	err := td.db.NativeDB().Update(func(tx *bolt.Tx) error {
		b := createBucket(t, tx, "test")
		idx := newUniqueIndex(t, b, "ui")
		for i := 0; i < 10; i++ {
			require.NoError(t, idx.Add(ctx, []byte(fmt.Sprintf("u%d", i)), []byte(fmt.Sprintf("id%d", i))))
		}
		return nil
	})
	require.NoError(t, err)

	{
		sc := newStepContext(5)
		result, err := callView(td.db, "test", "ui", func(idx index.Index) ([][]byte, error) {
			return idx.Range(sc, []byte("u0"), []byte("u9"), nil)
		})
		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, result)
	}

	result, err := callView(td.db, "test", "ui", func(idx index.Index) ([][]byte, error) {
		return idx.Range(ctx, []byte("u0"), []byte("u9"), nil)
	})
	require.NoError(t, err)
	require.Len(t, result, 10)
}

func TestUniqueIndex_PrefixCancellationDiscardsPartialResults(t *testing.T) {
	td := openTestDB(t)
	defer td.cleanup(t)

	ctx := context.Background()
	err := td.db.NativeDB().Update(func(tx *bolt.Tx) error {
		b := createBucket(t, tx, "test")
		idx := newUniqueIndex(t, b, "ui")
		for i := 0; i < 10; i++ {
			require.NoError(t, idx.Add(ctx, []byte(fmt.Sprintf("p%d", i)), []byte(fmt.Sprintf("id%d", i))))
		}
		return nil
	})
	require.NoError(t, err)

	{
		sc := newStepContext(5)
		result, err := callView(td.db, "test", "ui", func(idx index.Index) ([][]byte, error) {
			return idx.Prefix(sc, []byte("p"), nil)
		})
		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, result)
	}

	result, err := callView(td.db, "test", "ui", func(idx index.Index) ([][]byte, error) {
		return idx.Prefix(ctx, []byte("p"), nil)
	})
	require.NoError(t, err)
	require.Len(t, result, 10)
}

// ---------------------------------------------------------------------------
// Mutation cancellation + rollback tests
// ---------------------------------------------------------------------------

func TestMutation_UniqueAddCancellationRollsBack(t *testing.T) {
	td := openTestDB(t)
	defer td.cleanup(t)

	ctx := context.Background()

	// UniqueIndex.Add checks: entry (1), pre-Put (2), Put, post-Put (3).
	sc := newStepContext(3)
	err := td.db.NativeDB().Update(func(tx *bolt.Tx) error {
		b := createBucket(t, tx, "test")
		idx := newUniqueIndex(t, b, "ui")
		return idx.Add(sc, []byte("val"), []byte("id1"))
	})
	require.ErrorIs(t, err, context.Canceled)

	// Confirm nothing persisted (Bolt.Update rolled back).
	require.NoError(t, td.db.NativeDB().View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("test"))
		if b == nil {
			return nil
		}
		idx := newUniqueIndex(t, b, "ui")
		id, err := idx.Get(ctx, []byte("val"))
		require.NoError(t, err)
		require.Nil(t, id)
		return nil
	}))
}

func TestMutation_UniqueRemoveCancellationRollsBack(t *testing.T) {
	td := openTestDB(t)
	defer td.cleanup(t)

	ctx := context.Background()
	require.NoError(t, td.db.NativeDB().Update(func(tx *bolt.Tx) error {
		b := createBucket(t, tx, "test")
		idx := newUniqueIndex(t, b, "ui")
		return idx.Add(ctx, []byte("val"), []byte("id1"))
	}))

	// UniqueIndex.Remove checks: entry (1), Delete, post-Delete (2).
	sc := newStepContext(2)
	err := td.db.NativeDB().Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("test"))
		idx := newUniqueIndex(t, b, "ui")
		return idx.Remove(sc, []byte("val"))
	})
	require.ErrorIs(t, err, context.Canceled)

	require.NoError(t, td.db.NativeDB().View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("test"))
		idx := newUniqueIndex(t, b, "ui")
		id, err := idx.Get(ctx, []byte("val"))
		require.NoError(t, err)
		require.Equal(t, []byte("id1"), id)
		return nil
	}))
}

func TestMutation_UniqueRemoveIDCancellationRollsBack(t *testing.T) {
	td := openTestDB(t)
	defer td.cleanup(t)

	ctx := context.Background()
	require.NoError(t, td.db.NativeDB().Update(func(tx *bolt.Tx) error {
		b := createBucket(t, tx, "test")
		idx := newUniqueIndex(t, b, "ui")
		return idx.Add(ctx, []byte("val"), []byte("id1"))
	}))

	// UniqueIndex.RemoveID: entry (1), cursor iterations with per-iter checks.
	// Seed has 1 entry. Entry check (1), first iteration check (2).
	// cancelAt=2 cancels during first iteration before comparison.
	sc := newStepContext(2)
	err := td.db.NativeDB().Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("test"))
		idx := newUniqueIndex(t, b, "ui")
		return idx.RemoveID(sc, []byte("id1"))
	})
	require.ErrorIs(t, err, context.Canceled)

	require.NoError(t, td.db.NativeDB().View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("test"))
		idx := newUniqueIndex(t, b, "ui")
		id, err := idx.Get(ctx, []byte("val"))
		require.NoError(t, err)
		require.Equal(t, []byte("id1"), id)
		return nil
	}))
}

func TestMutation_ListAddCancellationRollsBack(t *testing.T) {
	td := openTestDB(t)
	defer td.cleanup(t)

	ctx := context.Background()

	// ListIndex.Add with no existing key: entry (1), post-IDs.Get (2), skip delete,
	// post-IDs.Add (3), post-Put (4).  cancelAt=4 cancels after Put succeeds.
	sc := newStepContext(4)
	err := td.db.NativeDB().Update(func(tx *bolt.Tx) error {
		b := createBucket(t, tx, "test")
		idx := newListIndex(t, b, "li")
		return idx.Add(sc, []byte("val"), []byte("id1"))
	})
	require.ErrorIs(t, err, context.Canceled)

	require.NoError(t, td.db.NativeDB().View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("test"))
		if b == nil {
			return nil
		}
		idx := newListIndex(t, b, "li")
		id, err := idx.Get(ctx, []byte("val"))
		require.NoError(t, err)
		require.Nil(t, id)
		return nil
	}))
}

func TestMutation_ListRemoveCancellationRollsBack(t *testing.T) {
	td := openTestDB(t)
	defer td.cleanup(t)

	ctx := context.Background()
	require.NoError(t, td.db.NativeDB().Update(func(tx *bolt.Tx) error {
		b := createBucket(t, tx, "test")
		idx := newListIndex(t, b, "li")
		for i := 0; i < 3; i++ {
			require.NoError(t, idx.Add(ctx, []byte("hello"), []byte(fmt.Sprintf("id%d", i))))
		}
		return nil
	}))

	// cancelAt=6: entry (1), 3 cursor iterations (2-4), pre-Delete (5),
	// first Delete, post-Delete (6) → cancel after one mutation.
	sc := newStepContext(6)
	err := td.db.NativeDB().Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("test"))
		idx := newListIndex(t, b, "li")
		return idx.Remove(sc, []byte("hello"))
	})
	require.ErrorIs(t, err, context.Canceled)

	require.NoError(t, td.db.NativeDB().View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("test"))
		idx := newListIndex(t, b, "li")
		result, err := idx.All(ctx, []byte("hello"), nil)
		require.NoError(t, err)
		require.Len(t, result, 3)
		return nil
	}))
}

func TestMutation_ListRemoveIDCancellationRollsBack(t *testing.T) {
	td := openTestDB(t)
	defer td.cleanup(t)

	ctx := context.Background()
	require.NoError(t, td.db.NativeDB().Update(func(tx *bolt.Tx) error {
		b := createBucket(t, tx, "test")
		idx := newListIndex(t, b, "li")
		return idx.Add(ctx, []byte("hello"), []byte("id1"))
	}))

	// ListIndex.RemoveID checks: entry (1), IDs.Get entry/post-Get (2,3),
	// pre-Delete (4), Delete, post-Delete (5) → cancel after the mutation.
	sc := newStepContext(5)
	err := td.db.NativeDB().Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("test"))
		idx := newListIndex(t, b, "li")
		return idx.RemoveID(sc, []byte("id1"))
	})
	require.ErrorIs(t, err, context.Canceled)

	require.NoError(t, td.db.NativeDB().View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("test"))
		idx := newListIndex(t, b, "li")
		result, err := idx.All(ctx, []byte("hello"), nil)
		require.NoError(t, err)
		require.Equal(t, [][]byte{[]byte("id1")}, result)
		return nil
	}))
}

// ---------------------------------------------------------------------------
// callView — runs a read-only Index operation, propagating errors correctly
// ---------------------------------------------------------------------------

func callView(
	db *rainstorm.DB,
	bucketName, indexName string,
	fn func(index.Index) ([][]byte, error),
) ([][]byte, error) {
	var result [][]byte
	var resultErr error
	err := db.NativeDB().View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		idx, err := buildIndex(b, indexName)
		if err != nil {
			return err
		}
		result, resultErr = fn(idx)
		return resultErr
	})
	if err != nil {
		return nil, err
	}
	return result, resultErr
}

func buildIndex(b *bolt.Bucket, name string) (index.Index, error) {
	// Try each type; UniqueIndex and ListIndex will work if the bucket exists.
	if b == nil {
		return nil, fmt.Errorf("bucket for index %q not found", name)
	}
	if name == "pk" {
		return index.NewIDIndex(b, []byte(name))
	}
	if name == "li" {
		return index.NewListIndex(b, []byte(name))
	}
	if name == "ui" {
		return index.NewUniqueIndex(b, []byte(name))
	}
	return nil, fmt.Errorf("unknown index %q", name)
}
