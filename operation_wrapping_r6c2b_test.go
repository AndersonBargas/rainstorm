package rainstorm

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AndersonBargas/rainstorm/v6/index"
	"github.com/AndersonBargas/rainstorm/v6/q"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

// ============================================================================
// R6.4C2B — Operation wrapping tests
// ============================================================================

// errCallback is a custom error returned by callbacks in tests.
var errCallback = errors.New("callback error")

func openWrappingBolt(t *testing.T, name string) *bolt.DB {
	t.Helper()
	db, err := bolt.Open(filepath.Join(t.TempDir(), name), 0600, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	return db
}

// ---------------------------------------------------------------------------
// Finder labels and classification
// ---------------------------------------------------------------------------

func TestWrap_Finder_LabelsAndClassification(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()
	nilCtx := context.Context(nil)

	require.NoError(t, db.Save(ctx, &User{ID: 1, Name: "John", Slug: "john"}))

	type testCase struct {
		name   string
		prefix string
		op     func() error
		cause  error
	}
	tests := []testCase{
		{
			name:   "One",
			prefix: "rainstorm one:",
			op:     func() error { return db.One(nilCtx, "Name", "John", &User{}) },
			cause:  ErrNilContext,
		},
		{
			name:   "Find",
			prefix: "rainstorm find:",
			op:     func() error { return db.Find(ctx, "Name", "John", &[]UniqueNameUser{}) },
			cause:  ErrNotFound,
		},
		{
			name:   "AllByIndex",
			prefix: "rainstorm all by index:",
			op:     func() error { return db.From("nonexistent-bucket-xyz").AllByIndex(ctx, "field", &[]User{}) },
			cause:  index.ErrNotFound,
		},
		{
			name:   "All",
			prefix: "rainstorm all:",
			op:     func() error { return db.All(nilCtx, &[]User{}) },
			cause:  ErrNilContext,
		},
		{
			name:   "Range",
			prefix: "rainstorm range:",
			op:     func() error { return db.Range(ctx, "Name", "John", "John", &User{}) },
			cause:  ErrSlicePtrNeeded,
		},
		{
			name:   "Prefix",
			prefix: "rainstorm prefix:",
			op:     func() error { return db.Prefix(ctx, "Name", "Jo", &User{}) },
			cause:  ErrSlicePtrNeeded,
		},
		{
			name:   "Count",
			prefix: "rainstorm count:",
			op:     func() error { _, err := db.Count(nilCtx, &User{}); return err },
			cause:  ErrNilContext,
		},
	}
	_ = tests[1]

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.op()
			require.Error(t, err)
			require.True(t, strings.HasPrefix(err.Error(), tc.prefix),
				"%s: expected prefix %q, got %q", tc.name, tc.prefix, err.Error())
			require.ErrorIs(t, err, tc.cause)
		})
	}
}

// ---------------------------------------------------------------------------
// Finder output safety
// ---------------------------------------------------------------------------

func TestWrap_AllByIndex_DelegatedBranchesKeepOuterLabel(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	for _, fieldName := range []string{"", "ID"} {
		t.Run(fieldName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancelOption := func(*index.Options) { cancel() }
			var users []User
			err := db.AllByIndex(ctx, fieldName, &users, cancelOption)
			require.ErrorIs(t, err, context.Canceled)
			require.True(t, strings.HasPrefix(err.Error(), "rainstorm all by index:"))
		})
	}
}

func TestWrap_Finder_OutputSafety(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	require.NoError(t, db.Save(ctx, &User{ID: 1, Name: "Alice", Slug: "alice"}))

	// One cancellation preserves sentinel destination
	t.Run("OneCancellationPreservesDest", func(t *testing.T) {
		sentinel := User{ID: 999, Name: "SENTINEL", Slug: "sentinel"}
		sctx := &finderStepContext{cancelAt: 3}
		err := db.One(sctx, "Name", "Alice", &sentinel)
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "rainstorm one:"))
		require.Equal(t, "SENTINEL", sentinel.Name, "destination should be preserved on error")
	})

	// Find cancellation preserves sentinel slice
	t.Run("FindCancellationPreservesDest", func(t *testing.T) {
		require.NoError(t, db.Save(ctx, &User{ID: 2, Name: "Alice", Slug: "alice2"}))
		users := []User{{ID: 999, Name: "SENTINEL", Slug: "sentinel"}}
		sctx := &finderStepContext{cancelAt: 5}
		err := db.Find(sctx, "Name", "Alice", &users)
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "rainstorm find:"))
		require.Equal(t, "SENTINEL", users[0].Name, "slice destination should be preserved on error")
	})

	// AllByIndex cancellation preserves destination
	t.Run("AllByIndexCancellationPreservesDest", func(t *testing.T) {
		require.NoError(t, db.Save(ctx, &User{ID: 3, Name: "Bob", Slug: "bob"}))
		users := []User{{ID: 999, Name: "SENTINEL", Slug: "sentinel"}}
		sctx := &finderStepContext{cancelAt: 3}
		err := db.AllByIndex(sctx, "Name", &users)
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "rainstorm all by index:"))
		require.Equal(t, "SENTINEL", users[0].Name, "destination should be preserved on error")
	})

	// Range cancellation preserves destination
	t.Run("RangeCancellationPreservesDest", func(t *testing.T) {
		require.NoError(t, db.Save(ctx, &User{ID: 20, Name: "Alice", Slug: "alice20"}))
		users := []User{{ID: 999, Name: "SENTINEL", Slug: "sentinel"}}
		err := db.Range(canceledCtx(), "Name", "A", "Z", &users)
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "rainstorm range:"))
		require.Equal(t, "SENTINEL", users[0].Name, "destination should be preserved on error")
	})

	// Prefix cancellation preserves destination
	t.Run("PrefixCancellationPreservesDest", func(t *testing.T) {
		require.NoError(t, db.Save(ctx, &User{ID: 4, Name: "Charlie", Slug: "charlie"}))
		users := []User{{ID: 999, Name: "SENTINEL", Slug: "sentinel"}}
		sctx := &finderStepContext{cancelAt: 3}
		err := db.Prefix(sctx, "Name", "Ch", &users)
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "rainstorm prefix:"))
		require.Equal(t, "SENTINEL", users[0].Name, "destination should be preserved on error")
	})

	// Count returns zero on wrapped error
	t.Run("CountReturnsZeroOnError", func(t *testing.T) {
		count, err := db.Count(canceledCtx(), &User{})
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "rainstorm count:"))
		require.Equal(t, 0, count)
	})

	// Successful operations still publish complete results and return nil
	t.Run("SuccessfulOnePublishesAndReturnsNil", func(t *testing.T) {
		var user User
		err := db.One(ctx, "Name", "Alice", &user)
		require.NoError(t, err)
		require.Equal(t, "Alice", user.Name)
	})

	t.Run("SuccessfulFindPublishesCompleteResult", func(t *testing.T) {
		var users []User
		err := db.Find(ctx, "Name", "Alice", &users)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(users), 1)
	})
}

// ---------------------------------------------------------------------------
// Query labels and classification
// ---------------------------------------------------------------------------

func TestWrap_Query_LabelsAndClassification(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()
	nilCtx := context.Context(nil)

	require.NoError(t, db.Save(ctx, &User{ID: 1, Name: "John", Slug: "john"}))

	type testCase struct {
		name   string
		prefix string
		op     func() error
		cause  error
	}
	tests := []testCase{
		{
			name:   "QueryFind",
			prefix: "rainstorm query find:",
			op:     func() error { return db.Select(q.True()).Find(nilCtx, &[]User{}) },
			cause:  ErrNilContext,
		},
		{
			name:   "QueryFirst",
			prefix: "rainstorm query first:",
			op:     func() error { return db.Select(q.True()).First(nilCtx, &User{}) },
			cause:  ErrNilContext,
		},
		{
			name:   "QueryDelete",
			prefix: "rainstorm query delete:",
			op:     func() error { return db.Select(q.True()).Delete(nilCtx, &User{}) },
			cause:  ErrNilContext,
		},
		{
			name:   "QueryCount",
			prefix: "rainstorm query count:",
			op:     func() error { _, err := db.Select(q.True()).Count(nilCtx, &User{}); return err },
			cause:  ErrNilContext,
		},
		{
			name:   "QueryRaw",
			prefix: "rainstorm query raw:",
			op:     func() error { _, err := db.Select(q.True()).Raw(nilCtx); return err },
			cause:  ErrNilContext,
		},
		{
			name:   "QueryRawEach",
			prefix: "rainstorm query raw each:",
			op: func() error {
				return db.Select(q.True()).RawEach(ctx, nil)
			},
			cause: ErrNilParam,
		},
		{
			name:   "QueryEach",
			prefix: "rainstorm query each:",
			op: func() error {
				return db.Select(q.True()).Each(ctx, &User{}, nil)
			},
			cause: ErrNilParam,
		},
	}
	_ = tests[6]

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.op()
			require.Error(t, err)
			require.True(t, strings.HasPrefix(err.Error(), tc.prefix),
				"%s: expected prefix %q, got %q", tc.name, tc.prefix, err.Error())
			require.ErrorIs(t, err, tc.cause)
		})
	}
}

// ---------------------------------------------------------------------------
// Query output safety
// ---------------------------------------------------------------------------

func TestWrap_Query_OutputSafety(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	require.NoError(t, db.Save(ctx, &User{ID: 1, Name: "John", Slug: "john"}))

	// Query.Find destination unchanged on wrapped cancellation
	t.Run("QueryFindDestUnchangedOnCancel", func(t *testing.T) {
		users := []User{{ID: 999, Name: "SENTINEL", Slug: "sentinel"}}
		err := db.Select(q.True()).Find(canceledCtx(), &users)
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "rainstorm query find:"))
		require.Equal(t, "SENTINEL", users[0].Name)
	})

	// Query.First destination unchanged on wrapped codec error or cancellation
	t.Run("QueryFirstDestUnchangedOnCancel", func(t *testing.T) {
		user := User{ID: 999, Name: "SENTINEL", Slug: "sentinel"}
		err := db.Select(q.True()).First(canceledCtx(), &user)
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "rainstorm query first:"))
		require.Equal(t, "SENTINEL", user.Name)
	})

	// Query.Count returns zero on wrapped cancellation
	t.Run("QueryCountReturnsZeroOnCancel", func(t *testing.T) {
		count, err := db.Select(q.True()).Count(canceledCtx(), &User{})
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "rainstorm query count:"))
		require.Equal(t, 0, count)
	})

	// Query.Raw returns nil on wrapped cancellation
	t.Run("QueryRawReturnsNilOnCancel", func(t *testing.T) {
		raw, err := db.Select(q.True()).Raw(canceledCtx())
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "rainstorm query raw:"))
		require.Nil(t, raw)
	})

	// Successful ordered query publishes complete sorted results
	t.Run("SuccessfulOrderedQueryPublishesCompleteResult", func(t *testing.T) {
		require.NoError(t, db.Save(ctx, &User{ID: 2, Name: "Zach", Slug: "zach"}))
		require.NoError(t, db.Save(ctx, &User{ID: 3, Name: "Alice", Slug: "alice2"}))

		var users []User
		err := db.Select(q.True()).OrderBy("Name").Find(ctx, &users)
		require.NoError(t, err)
		require.Len(t, users, 3)
		require.Equal(t, "Alice", users[0].Name)
	})
}

// ---------------------------------------------------------------------------
// Query callbacks
// ---------------------------------------------------------------------------

func TestWrap_Query_Callbacks(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	require.NoError(t, db.Save(ctx, &User{ID: 1, Name: "John", Slug: "john"}))
	require.NoError(t, db.Save(ctx, &User{ID: 2, Name: "Jane", Slug: "jane"}))

	// RawEach nil callback -> wrapped ErrNilParam
	t.Run("RawEachNilCallback", func(t *testing.T) {
		err := db.Select(q.True()).RawEach(ctx, nil)
		require.ErrorIs(t, err, ErrNilParam)
		require.True(t, strings.HasPrefix(err.Error(), "rainstorm query raw each:"))
	})

	// Each nil callback -> wrapped ErrNilParam
	t.Run("EachNilCallback", func(t *testing.T) {
		err := db.Select(q.True()).Each(ctx, &User{}, nil)
		require.ErrorIs(t, err, ErrNilParam)
		require.True(t, strings.HasPrefix(err.Error(), "rainstorm query each:"))
	})

	// RawEach callback error remains discoverable
	t.Run("RawEachCallbackErrorDiscoverable", func(t *testing.T) {
		require.NoError(t, db.Save(ctx, &User{ID: 10, Name: "cb", Slug: "cb1"}))
		err := db.Select().Bucket("User").RawEach(ctx, func(k, v []byte) error {
			return errCallback
		})
		require.ErrorIs(t, err, errCallback)
		require.True(t, strings.HasPrefix(err.Error(), "rainstorm query raw each:"))
	})

	// Each callback error remains discoverable
	t.Run("EachCallbackErrorDiscoverable", func(t *testing.T) {
		require.NoError(t, db.Save(ctx, &User{ID: 11, Name: "cb2", Slug: "cb2"}))
		err := db.Select().Bucket("User").Each(ctx, &User{}, func(a any) error {
			return errCallback
		})
		require.ErrorIs(t, err, errCallback)
		require.True(t, strings.HasPrefix(err.Error(), "rainstorm query each:"))
	})

	// Callback error beats cancellation triggered inside callback
	t.Run("RawEachCallbackErrorBeatsCancel", func(t *testing.T) {
		require.NoError(t, db.Save(ctx, &User{ID: 12, Name: "cb3", Slug: "cb3"}))
		callbackCtx, cancel := context.WithCancel(ctx)
		err := db.Select().Bucket("User").RawEach(callbackCtx, func(k, v []byte) error {
			cancel()
			return errCallback
		})
		require.ErrorIs(t, err, errCallback)
		require.NotErrorIs(t, err, context.Canceled)
		require.True(t, strings.HasPrefix(err.Error(), "rainstorm query raw each:"))
	})

	// Cancellation after successful callback prevents later callbacks
	t.Run("RawEachCancelAfterFirstCallback", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			require.NoError(t, db.Save(ctx, &User{ID: 100 + i, Name: "multi", Slug: "multi" + string(rune('a'+i))}))
		}
		callCount := 0
		qsc := newQueryStepContext(context.Background())
		qsc.cancelAfter(2)
		err := db.Select().Bucket("User").RawEach(qsc.context(), func(k, v []byte) error {
			callCount++
			return qsc.step()
		})
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "rainstorm query raw each:"))
		require.ErrorIs(t, err, context.Canceled)
		require.GreaterOrEqual(t, callCount, 2)
	})

	// Panic propagates unchanged; Rainstorm does not recover it
	t.Run("RawEachPanicPropagates", func(t *testing.T) {
		require.NoError(t, db.Save(ctx, &User{ID: 13, Name: "panic", Slug: "panic"}))
		var recovered any
		func() {
			defer func() {
				recovered = recover()
			}()
			_ = db.Select().Bucket("User").RawEach(ctx, func(k, v []byte) error {
				panic("test panic")
			})
		}()
		require.NotNil(t, recovered)
		require.Equal(t, "test panic", recovered.(string))
	})
}

// ---------------------------------------------------------------------------
// Query Delete rollback
// ---------------------------------------------------------------------------

func TestWrap_QueryDelete_Rollback(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	// Save test records
	require.NoError(t, db.Save(ctx, &User{ID: 1, Name: "Alice", Slug: "alice"}))
	require.NoError(t, db.Save(ctx, &User{ID: 2, Name: "Bob", Slug: "bob"}))
	require.NoError(t, db.Save(ctx, &User{ID: 3, Name: "Charlie", Slug: "charlie"}))

	// Verify data saved
	var count int
	count, err := db.Count(ctx, &User{})
	require.NoError(t, err)
	require.Equal(t, 3, count)

	// The first matcher succeeds and allows one delete mutation. The second
	// matcher cancels, forcing the enclosing bbolt transaction to roll back.
	qsc := newQueryStepContext(context.Background())
	qsc.cancelAfter(2)
	deleteErr := db.Select(&stepMatcher{qsc: qsc}).Delete(qsc.context(), &User{})
	require.Error(t, deleteErr)
	require.True(t, strings.HasPrefix(deleteErr.Error(), "rainstorm query delete:"))
	require.ErrorIs(t, deleteErr, context.Canceled)

	require.GreaterOrEqual(t, qsc.count(), 2, "query must advance past the first mutated record")

	// All records and indexes must be restored.
	count, err = db.Count(ctx, &User{})
	require.NoError(t, err)
	require.Equal(t, 3, count, "all records must be restored after rollback")
	var indexed User
	require.NoError(t, db.One(ctx, "Slug", "alice", &indexed))
	require.Equal(t, 1, indexed.ID)

	// Subsequent query succeeds
	var users []User
	err = db.All(ctx, &users)
	require.NoError(t, err)
	require.Len(t, users, 3)
}

// ---------------------------------------------------------------------------
// Scanners
// ---------------------------------------------------------------------------

func TestWrap_Scanners(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()
	var nilCtx context.Context

	// Seed scan data
	node := db.From("scan-node")
	require.NoError(t, node.Save(ctx, &User{ID: 1, Name: "John", Slug: "john"}))

	// PrefixScan nil context -> exact prefix and nil nodes
	t.Run("PrefixScanNilContext", func(t *testing.T) {
		nodes, err := node.PrefixScan(nilCtx, "scan")
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "rainstorm prefix scan:"))
		require.Nil(t, nodes)
	})

	// RangeScan nil context -> exact prefix and nil nodes
	t.Run("RangeScanNilContext", func(t *testing.T) {
		nodes, err := node.RangeScan(nilCtx, "a", "z")
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "rainstorm range scan:"))
		require.Nil(t, nodes)
	})

	// PrefixScan canceled context -> nil nodes, not partial
	t.Run("PrefixScanCanceledContext", func(t *testing.T) {
		nodes, err := node.PrefixScan(canceledCtx(), "scan")
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "rainstorm prefix scan:"))
		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, nodes, "must return nil, not partial")
	})

	// RangeScan canceled context -> nil nodes, not partial
	t.Run("RangeScanCanceledContext", func(t *testing.T) {
		nodes, err := node.RangeScan(canceledCtx(), "a", "z")
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "rainstorm range scan:"))
		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, nodes, "must return nil, not partial")
	})

	// Cancellation during cursor iteration -> nil nodes, not partial
	t.Run("PrefixScanCancelDuringIterationReturnsNil", func(t *testing.T) {
		for i := 1; i <= 10; i++ {
			child := node.From("bucket_" + string(rune('A'+i-1)))
			require.NoError(t, child.Save(ctx, &User{ID: i + 10, Name: "test", Slug: "test"}))
		}
		sctx := &stepContext{cancelAt: 5}
		nodes, err := node.PrefixScan(sctx, "bucket")
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "rainstorm prefix scan:"))
		require.Nil(t, nodes, "must return nil, not partial")
	})

	// Subsequent successful scan returns complete result
	t.Run("SuccessfulPrefixScanAfterCancel", func(t *testing.T) {
		nodes, err := node.PrefixScan(ctx, "bucket")
		require.NoError(t, err)
		require.NotEmpty(t, nodes)
	})
}

// ---------------------------------------------------------------------------
// Index constructors
// ---------------------------------------------------------------------------

func TestWrap_IndexConstructors(t *testing.T) {
	bDB := openWrappingBolt(t, "constructors.db")

	_, err := index.NewIDIndex(nil, []byte("id-idx"))
	require.ErrorIs(t, err, index.ErrNilParam)
	require.True(t, strings.HasPrefix(err.Error(), "rainstorm index new id:"))
	_, err = index.NewListIndex(nil, []byte("list-idx"))
	require.ErrorIs(t, err, index.ErrNilParam)
	require.True(t, strings.HasPrefix(err.Error(), "rainstorm index new list:"))
	_, err = index.NewUniqueIndex(nil, []byte("unique-idx"))
	require.ErrorIs(t, err, index.ErrNilParam)
	require.True(t, strings.HasPrefix(err.Error(), "rainstorm index new unique:"))

	// Test constructor labels by creating in a writable transaction
	require.NoError(t, bDB.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucket([]byte("test"))
		require.NoError(t, err)

		// NewIDIndex should succeed
		_, err = index.NewIDIndex(b, []byte("id-idx"))
		require.NoError(t, err)

		// NewListIndex should succeed
		_, err = index.NewListIndex(b, []byte("list-idx"))
		require.NoError(t, err)

		// NewUniqueIndex should succeed
		_, err = index.NewUniqueIndex(b, []byte("unique-idx"))
		require.NoError(t, err)

		return nil
	}))

	// Test NewUniqueIndex on non-writable bucket -> ErrNotFound
	require.NoError(t, bDB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("test"))
		require.NotNil(t, b)
		_, err := index.NewUniqueIndex(b, []byte("non-existent-unique"))
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "rainstorm index new unique:"))
		require.ErrorIs(t, err, index.ErrNotFound)
		return nil
	}))

	// Test NewListIndex on non-writable bucket -> ErrNotFound
	require.NoError(t, bDB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("test"))
		require.NotNil(t, b)
		_, err := index.NewListIndex(b, []byte("non-existent-list"))
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "rainstorm index new list:"))
		require.ErrorIs(t, err, index.ErrNotFound)
		return nil
	}))
}

// ---------------------------------------------------------------------------
// Index methods — table-driven
// ---------------------------------------------------------------------------

func TestWrap_IndexMethods(t *testing.T) {
	var nilCtx context.Context
	bDB := openWrappingBolt(t, "methods.db")

	// Create the indexes once in a writable transaction.
	var idIdx *index.IDIndex
	var listIdx *index.ListIndex
	var uniqueIdx *index.UniqueIndex

	require.NoError(t, bDB.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucket([]byte("test"))
		require.NoError(t, err)

		idIdx, err = index.NewIDIndex(b, []byte("id-idx"))
		require.NoError(t, err)

		listIdx, err = index.NewListIndex(b, []byte("list-idx"))
		require.NoError(t, err)

		uniqueIdx, err = index.NewUniqueIndex(b, []byte("unique-idx"))
		require.NoError(t, err)

		return nil
	}))

	type idxTest struct {
		label  string
		op     func() error
		cause  error
		prefix string
	}

	// All test cases for all index methods.
	// We cover:
	// - index add (IDIndex, ListIndex, UniqueIndex)
	// - index remove (IDIndex, ListIndex, UniqueIndex)
	// - index remove id (IDIndex, ListIndex, UniqueIndex)
	// - index get (IDIndex, ListIndex, UniqueIndex)
	// - index all (IDIndex, ListIndex, UniqueIndex)
	// - index all records (IDIndex, ListIndex, UniqueIndex)
	// - index range (IDIndex, ListIndex, UniqueIndex)
	// - index prefix (IDIndex, ListIndex, UniqueIndex)

	// ---------------------------------------------------------------------------
	// index add
	// ---------------------------------------------------------------------------

	t.Run("index add", func(t *testing.T) {
		tests := []idxTest{
			{
				label:  "IDIndex.Add nil context",
				prefix: "rainstorm index add:",
				cause:  index.ErrNilContext,
				op:     func() error { return idIdx.Add(nilCtx, []byte("v"), []byte("id")) },
			},
			{
				label:  "IDIndex.Add nil param",
				prefix: "rainstorm index add:",
				cause:  index.ErrNilParam,
				op:     func() error { return idIdx.Add(context.Background(), nil, []byte("id")) },
			},
			{
				label:  "ListIndex.Add nil context",
				prefix: "rainstorm index add:",
				cause:  index.ErrNilContext,
				op:     func() error { return listIdx.Add(nilCtx, []byte("v"), []byte("id")) },
			},
			{
				label:  "ListIndex.Add nil param",
				prefix: "rainstorm index add:",
				cause:  index.ErrNilParam,
				op:     func() error { return listIdx.Add(context.Background(), nil, []byte("id")) },
			},
			{
				label:  "UniqueIndex.Add nil context",
				prefix: "rainstorm index add:",
				cause:  index.ErrNilContext,
				op:     func() error { return uniqueIdx.Add(nilCtx, []byte("v"), []byte("id")) },
			},
			{
				label:  "UniqueIndex.Add nil param",
				prefix: "rainstorm index add:",
				cause:  index.ErrNilParam,
				op:     func() error { return uniqueIdx.Add(context.Background(), nil, []byte("id")) },
			},
		}
		for _, tc := range tests {
			t.Run(tc.label, func(t *testing.T) {
				err := tc.op()
				require.Error(t, err)
				require.True(t, strings.HasPrefix(err.Error(), tc.prefix),
					"%s: expected prefix %q, got %q", tc.label, tc.prefix, err.Error())
				require.ErrorIs(t, err, tc.cause)
			})
		}
	})

	// ---------------------------------------------------------------------------
	// index remove
	// ---------------------------------------------------------------------------

	t.Run("index remove", func(t *testing.T) {
		// Seed data needed for ListIndex.Remove
		require.NoError(t, bDB.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte("test"))
			lIdx, _ := index.NewListIndex(b, []byte("list-idx"))
			return lIdx.Add(context.Background(), []byte("remove-me"), []byte("id-rm"))
		}))

		tests := []idxTest{
			{
				label:  "IDIndex.Remove nil context",
				prefix: "rainstorm index remove:",
				cause:  index.ErrNilContext,
				op:     func() error { return idIdx.Remove(nilCtx, []byte("x")) },
			},
			{
				label:  "ListIndex.Remove nil context",
				prefix: "rainstorm index remove:",
				cause:  index.ErrNilContext,
				op:     func() error { return listIdx.Remove(nilCtx, []byte("x")) },
			},
			{
				label:  "UniqueIndex.Remove nil context",
				prefix: "rainstorm index remove:",
				cause:  index.ErrNilContext,
				op:     func() error { return uniqueIdx.Remove(nilCtx, []byte("x")) },
			},
		}
		for _, tc := range tests {
			t.Run(tc.label, func(t *testing.T) {
				err := tc.op()
				require.Error(t, err)
				require.True(t, strings.HasPrefix(err.Error(), tc.prefix),
					"%s: expected prefix %q, got %q", tc.label, tc.prefix, err.Error())
				require.ErrorIs(t, err, tc.cause)
			})
		}
	})

	// ---------------------------------------------------------------------------
	// index remove id
	// ---------------------------------------------------------------------------

	t.Run("index remove id", func(t *testing.T) {
		tests := []idxTest{
			{
				label:  "IDIndex.RemoveID nil context",
				prefix: "rainstorm index remove id:",
				cause:  index.ErrNilContext,
				op:     func() error { return idIdx.RemoveID(nilCtx, []byte("x")) },
			},
			{
				label:  "ListIndex.RemoveID nil context",
				prefix: "rainstorm index remove id:",
				cause:  index.ErrNilContext,
				op:     func() error { return listIdx.RemoveID(nilCtx, []byte("x")) },
			},
			{
				label:  "UniqueIndex.RemoveID nil context",
				prefix: "rainstorm index remove id:",
				cause:  index.ErrNilContext,
				op:     func() error { return uniqueIdx.RemoveID(nilCtx, []byte("x")) },
			},
		}
		for _, tc := range tests {
			t.Run(tc.label, func(t *testing.T) {
				err := tc.op()
				require.Error(t, err)
				require.True(t, strings.HasPrefix(err.Error(), tc.prefix),
					"%s: expected prefix %q, got %q", tc.label, tc.prefix, err.Error())
				require.ErrorIs(t, err, tc.cause)
			})
		}
	})

	// ---------------------------------------------------------------------------
	// index get
	// ---------------------------------------------------------------------------

	t.Run("index get", func(t *testing.T) {
		tests := []idxTest{
			{
				label:  "IDIndex.Get nil context",
				prefix: "rainstorm index get:",
				cause:  index.ErrNilContext,
				op:     func() error { _, err := idIdx.Get(nilCtx, []byte("x")); return err },
			},
			{
				label:  "ListIndex.Get nil context",
				prefix: "rainstorm index get:",
				cause:  index.ErrNilContext,
				op:     func() error { _, err := listIdx.Get(nilCtx, []byte("x")); return err },
			},
			{
				label:  "UniqueIndex.Get nil context",
				prefix: "rainstorm index get:",
				cause:  index.ErrNilContext,
				op:     func() error { _, err := uniqueIdx.Get(nilCtx, []byte("x")); return err },
			},
			// Get returns nil on error
			{
				label:  "IDIndex.Get returns nil on error",
				prefix: "rainstorm index get:",
				cause:  index.ErrNilContext,
				op: func() error {
					val, err := idIdx.Get(nilCtx, []byte("x"))
					require.Nil(t, val)
					return err
				},
			},
		}
		for _, tc := range tests {
			t.Run(tc.label, func(t *testing.T) {
				err := tc.op()
				require.Error(t, err)
				require.True(t, strings.HasPrefix(err.Error(), tc.prefix),
					"%s: expected prefix %q, got %q", tc.label, tc.prefix, err.Error())
				require.ErrorIs(t, err, tc.cause)
			})
		}
	})

	// ---------------------------------------------------------------------------
	// index all
	// ---------------------------------------------------------------------------

	t.Run("index all", func(t *testing.T) {
		tests := []idxTest{
			{
				label:  "IDIndex.All nil context",
				prefix: "rainstorm index all:",
				cause:  index.ErrNilContext,
				op:     func() error { _, err := idIdx.All(nilCtx, []byte("x"), nil); return err },
			},
			{
				label:  "ListIndex.All nil context",
				prefix: "rainstorm index all:",
				cause:  index.ErrNilContext,
				op:     func() error { _, err := listIdx.All(nilCtx, []byte("x"), nil); return err },
			},
			{
				label:  "UniqueIndex.All nil context",
				prefix: "rainstorm index all:",
				cause:  index.ErrNilContext,
				op:     func() error { _, err := uniqueIdx.All(nilCtx, []byte("x"), nil); return err },
			},
			// All returns nil on error
			{
				label:  "IDIndex.All returns nil on error",
				prefix: "rainstorm index all:",
				cause:  index.ErrNilContext,
				op: func() error {
					val, err := idIdx.All(nilCtx, []byte("x"), nil)
					require.Nil(t, val)
					return err
				},
			},
		}
		for _, tc := range tests {
			t.Run(tc.label, func(t *testing.T) {
				err := tc.op()
				require.Error(t, err)
				require.True(t, strings.HasPrefix(err.Error(), tc.prefix),
					"%s: expected prefix %q, got %q", tc.label, tc.prefix, err.Error())
				require.ErrorIs(t, err, tc.cause)
			})
		}
	})

	// ---------------------------------------------------------------------------
	// index all records
	// ---------------------------------------------------------------------------

	t.Run("index all records", func(t *testing.T) {
		tests := []idxTest{
			{
				label:  "IDIndex.AllRecords nil context",
				prefix: "rainstorm index all records:",
				cause:  index.ErrNilContext,
				op:     func() error { _, err := idIdx.AllRecords(nilCtx, nil); return err },
			},
			{
				label:  "ListIndex.AllRecords nil context",
				prefix: "rainstorm index all records:",
				cause:  index.ErrNilContext,
				op:     func() error { _, err := listIdx.AllRecords(nilCtx, nil); return err },
			},
			{
				label:  "UniqueIndex.AllRecords nil context",
				prefix: "rainstorm index all records:",
				cause:  index.ErrNilContext,
				op:     func() error { _, err := uniqueIdx.AllRecords(nilCtx, nil); return err },
			},
			// AllRecords returns nil on error
			{
				label:  "UniqueIndex.AllRecords returns nil on error",
				prefix: "rainstorm index all records:",
				cause:  index.ErrNilContext,
				op: func() error {
					val, err := uniqueIdx.AllRecords(nilCtx, nil)
					require.Nil(t, val)
					return err
				},
			},
		}
		for _, tc := range tests {
			t.Run(tc.label, func(t *testing.T) {
				err := tc.op()
				require.Error(t, err)
				require.True(t, strings.HasPrefix(err.Error(), tc.prefix),
					"%s: expected prefix %q, got %q", tc.label, tc.prefix, err.Error())
				require.ErrorIs(t, err, tc.cause)
			})
		}
	})

	// ---------------------------------------------------------------------------
	// index range
	// ---------------------------------------------------------------------------

	t.Run("index range", func(t *testing.T) {
		tests := []idxTest{
			{
				label:  "IDIndex.Range nil context",
				prefix: "rainstorm index range:",
				cause:  index.ErrNilContext,
				op:     func() error { _, err := idIdx.Range(nilCtx, []byte("a"), []byte("z"), nil); return err },
			},
			{
				label:  "ListIndex.Range nil context",
				prefix: "rainstorm index range:",
				cause:  index.ErrNilContext,
				op:     func() error { _, err := listIdx.Range(nilCtx, []byte("a"), []byte("z"), nil); return err },
			},
			{
				label:  "UniqueIndex.Range nil context",
				prefix: "rainstorm index range:",
				cause:  index.ErrNilContext,
				op:     func() error { _, err := uniqueIdx.Range(nilCtx, []byte("a"), []byte("z"), nil); return err },
			},
			// Range returns nil on error
			{
				label:  "UniqueIndex.Range returns nil on error",
				prefix: "rainstorm index range:",
				cause:  index.ErrNilContext,
				op: func() error {
					val, err := uniqueIdx.Range(nilCtx, []byte("a"), []byte("z"), nil)
					require.Nil(t, val)
					return err
				},
			},
		}
		for _, tc := range tests {
			t.Run(tc.label, func(t *testing.T) {
				err := tc.op()
				require.Error(t, err)
				require.True(t, strings.HasPrefix(err.Error(), tc.prefix),
					"%s: expected prefix %q, got %q", tc.label, tc.prefix, err.Error())
				require.ErrorIs(t, err, tc.cause)
			})
		}
	})

	// ---------------------------------------------------------------------------
	// index prefix
	// ---------------------------------------------------------------------------

	t.Run("index prefix", func(t *testing.T) {
		tests := []idxTest{
			{
				label:  "IDIndex.Prefix nil context",
				prefix: "rainstorm index prefix:",
				cause:  index.ErrNilContext,
				op:     func() error { _, err := idIdx.Prefix(nilCtx, []byte("a"), nil); return err },
			},
			{
				label:  "ListIndex.Prefix nil context",
				prefix: "rainstorm index prefix:",
				cause:  index.ErrNilContext,
				op:     func() error { _, err := listIdx.Prefix(nilCtx, []byte("a"), nil); return err },
			},
			{
				label:  "UniqueIndex.Prefix nil context",
				prefix: "rainstorm index prefix:",
				cause:  index.ErrNilContext,
				op:     func() error { _, err := uniqueIdx.Prefix(nilCtx, []byte("a"), nil); return err },
			},
			// Prefix returns nil on error
			{
				label:  "UniqueIndex.Prefix returns nil on error",
				prefix: "rainstorm index prefix:",
				cause:  index.ErrNilContext,
				op: func() error {
					val, err := uniqueIdx.Prefix(nilCtx, []byte("a"), nil)
					require.Nil(t, val)
					return err
				},
			},
		}
		for _, tc := range tests {
			t.Run(tc.label, func(t *testing.T) {
				err := tc.op()
				require.Error(t, err)
				require.True(t, strings.HasPrefix(err.Error(), tc.prefix),
					"%s: expected prefix %q, got %q", tc.label, tc.prefix, err.Error())
				require.ErrorIs(t, err, tc.cause)
			})
		}
	})
}

// ---------------------------------------------------------------------------
// Index mutation rollback
// ---------------------------------------------------------------------------

func TestWrap_Index_MutationRollback(t *testing.T) {
	bDB := openWrappingBolt(t, "rollback.db")

	t.Run("UniqueIndexAddCancellationRollsBack", func(t *testing.T) {
		require.NoError(t, bDB.Update(func(tx *bolt.Tx) error {
			b, err := tx.CreateBucket([]byte("rollback"))
			require.NoError(t, err)
			_, cerr := index.NewUniqueIndex(b, []byte("uni-idx"))
			return cerr
		}))

		// Verify empty before mutation.
		require.NoError(t, bDB.View(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte("rollback"))
			if b == nil {
				return ErrNotFound
			}
			idx, err := index.NewUniqueIndex(b, []byte("uni-idx"))
			if err != nil {
				return err
			}
			list, err := idx.AllRecords(context.Background(), nil)
			if err != nil {
				return err
			}
			require.Empty(t, list)
			return nil
		}))

		// UniqueIndex.Add checks directly at entry, before Put, and after Put.
		// Cancel on the third direct check to prove the Put occurred.
		cancelled := &cancelAtCallerContext{
			callerSuffix: ".(*UniqueIndex).Add",
			cancelAt:     3,
		}
		updateErr := bDB.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte("rollback"))
			idx, err := index.NewUniqueIndex(b, []byte("uni-idx"))
			if err != nil {
				return err
			}
			err = idx.Add(cancelled, []byte("val"), []byte("id"))
			// Should return a wrapped context error
			require.Error(t, err)
			require.True(t, strings.HasPrefix(err.Error(), "rainstorm index add:"))
			require.ErrorIs(t, err, context.Canceled)
			// Return the error to trigger Bolt rollback
			return err
		})
		// The Update itself fails because the callback returned an error (rollback)
		require.Error(t, updateErr)
		require.True(t, strings.HasPrefix(updateErr.Error(), "rainstorm index add:"))
		require.ErrorIs(t, updateErr, context.Canceled)
		require.Equal(t, 3, cancelled.Hits(), "cancellation must occur after UniqueIndex.Put")

		// Verify index is empty after rollback.
		require.NoError(t, bDB.View(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte("rollback"))
			if b == nil {
				return ErrNotFound
			}
			idx, err := index.NewUniqueIndex(b, []byte("uni-idx"))
			if err != nil {
				return err
			}
			list, err := idx.AllRecords(context.Background(), nil)
			if err != nil {
				return err
			}
			require.Empty(t, list, "previous index state should be preserved")
			return nil
		}))
	})

	t.Run("ListIndexAddCancellationRollsBack", func(t *testing.T) {
		require.NoError(t, bDB.Update(func(tx *bolt.Tx) error {
			b, err := tx.CreateBucket([]byte("rollback-list"))
			require.NoError(t, err)
			_, cerr := index.NewListIndex(b, []byte("list-idx"))
			return cerr
		}))

		// Verify empty before mutation.
		require.NoError(t, bDB.View(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte("rollback-list"))
			if b == nil {
				return ErrNotFound
			}
			idx, err := index.NewListIndex(b, []byte("list-idx"))
			if err != nil {
				return err
			}
			list, err := idx.AllRecords(context.Background(), nil)
			if err != nil {
				return err
			}
			require.Empty(t, list)
			return nil
		}))

		// ListIndex.Add's fourth direct check occurs after IndexBucket.Put.
		cancelled := &cancelAtCallerContext{
			callerSuffix: ".(*ListIndex).Add",
			cancelAt:     4,
		}
		updateErr := bDB.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte("rollback-list"))
			idx, err := index.NewListIndex(b, []byte("list-idx"))
			if err != nil {
				return err
			}
			err = idx.Add(cancelled, []byte("val"), []byte("id"))
			require.Error(t, err)
			require.True(t, strings.HasPrefix(err.Error(), "rainstorm index add:"))
			require.ErrorIs(t, err, context.Canceled)
			return err
		})
		require.Error(t, updateErr)
		require.True(t, strings.HasPrefix(updateErr.Error(), "rainstorm index add:"))
		require.ErrorIs(t, updateErr, context.Canceled)
		require.Equal(t, 4, cancelled.Hits(), "cancellation must occur after ListIndex.Put")

		// Verify index is empty after rollback.
		require.NoError(t, bDB.View(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte("rollback-list"))
			if b == nil {
				return ErrNotFound
			}
			idx, err := index.NewListIndex(b, []byte("list-idx"))
			if err != nil {
				return err
			}
			list, err := idx.AllRecords(context.Background(), nil)
			if err != nil {
				return err
			}
			require.Empty(t, list, "previous index state should be preserved")
			return nil
		}))
	})
}

// ---------------------------------------------------------------------------
// Nested wrapping and errors.Is proof
// ---------------------------------------------------------------------------

func TestWrap_NestedWrapping(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	// Seed data with indexed fields
	require.NoError(t, db.Save(ctx, &User{ID: 1, Name: "John", Slug: "john"}))

	// All calls query.Find which generates nested wrapping
	t.Run("AllNestedWithQueryFind", func(t *testing.T) {
		err := db.All(ctx, &[]User{})
		require.NoError(t, err)
	})

	// Cancelled All -> expects wrapping on the All boundary
	t.Run("CancelledAllShowsNestedWrapping", func(t *testing.T) {
		err := db.All(canceledCtx(), &[]User{})
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "rainstorm all:"),
			"outermost prefix for All must be 'rainstorm all:', got %q", err.Error())
		require.ErrorIs(t, err, context.Canceled)
	})

	// Delete with index wrapping
	t.Run("DeleteNestedWithIndexRemove", func(t *testing.T) {
		require.NoError(t, db.Save(ctx, &UniqueNameUser{ID: 1, Name: "delete-me", Age: 10}))

		err := db.Select(q.Eq("ID", 1)).Delete(canceledCtx(), &UniqueNameUser{})
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "rainstorm query delete:"),
			"outermost prefix for Delete must be 'rainstorm query delete:'")
		require.ErrorIs(t, err, context.Canceled)
	})

	// Find with index wrapping — nested wrapping is expected
	t.Run("FindWithIndexWrapping", func(t *testing.T) {
		err := db.Find(ctx, "Name", "John", &[]UniqueNameUser{})
		require.Error(t, err)
		require.True(t, strings.HasPrefix(err.Error(), "rainstorm find:"))
		require.ErrorIs(t, err, ErrNotFound)
	})

}

// ---------------------------------------------------------------------------
// Sensitive data exclusion
// ---------------------------------------------------------------------------

func TestWrap_SensitiveDataExclusion(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	// Use distinctive secret-like values.
	const secretField = "SecretF13ld!XYZ"
	const secretValue = "S3cr3t-Val!789"
	const secretMin = "MIN-SECRET-123"
	const secretMax = "MAX-SECRET-456"
	const secretPrefix = "PRFX-SECRET-abc"
	const secretIndexVal = "IDX-SECRET-xyz"
	const secretTargetID = "TGT-SECRET-999"

	require.NoError(t, db.Save(ctx, &User{ID: 1, Name: "Alice", Slug: "alice"}))

	t.Run("FinderFieldValue_NotLeaked", func(t *testing.T) {
		// The secret value should not be leaked in error messages.
		// Field names from extractSingleField are existing behavior and allowed.
		// Use an existing field "Name" with a secret value, and trigger ErrNotFound.
		err := db.Find(ctx, "Name", secretValue, &[]UniqueNameUser{})
		t.Logf("Finder error: %v", err)
		msg := err.Error()
		// The secret value must not be in the error message
		require.NotContains(t, msg, secretValue)
	})

	t.Run("RangeMinMax_NotLeaked", func(t *testing.T) {
		// Range with secret min/max values on an existing field to avoid
		// extractSingleField injecting field names.
		err := db.Range(ctx, "Name", secretMin, secretMax, &[]User{})
		t.Logf("Range error: %v", err)
		msg := err.Error()
		require.NotContains(t, msg, secretMin)
		require.NotContains(t, msg, secretMax)
	})

	t.Run("PrefixValue_NotLeaked", func(t *testing.T) {
		err := db.Prefix(ctx, "Name", secretPrefix, &[]User{})
		t.Logf("Prefix error: %v", err)
		msg := err.Error()
		require.NotContains(t, msg, secretPrefix)
	})

	t.Run("QueryMatcher_NotLeaked", func(t *testing.T) {
		// Use Raw with secret field name as matcher value; secret value should not leak
		_, err := db.Select(q.Eq(secretField, secretValue)).Raw(canceledCtx())
		t.Logf("Query error: %v", err)
		msg := err.Error()
		require.NotContains(t, msg, secretField)
		require.NotContains(t, msg, secretValue)
	})

	t.Run("ScannerPrefixRange_NotLeaked", func(t *testing.T) {
		node := db.From("scan-node")
		require.NoError(t, node.Save(ctx, &User{ID: 100, Name: "test", Slug: "test"}))

		// Prefix scan with secret prefix
		nodes, err := node.PrefixScan(canceledCtx(), secretPrefix)
		require.Error(t, err)
		msg := err.Error()
		require.NotContains(t, msg, secretPrefix)

		// Range scan with secret bounds
		nodes, err = node.RangeScan(canceledCtx(), secretMin, secretMax)
		require.Error(t, err)
		msg = err.Error()
		require.NotContains(t, msg, secretMin)
		require.NotContains(t, msg, secretMax)
		_ = nodes
	})

	t.Run("IndexValueAndTargetID_NotLeaked", func(t *testing.T) {
		bDB := openWrappingBolt(t, "sensitive.db")

		require.NoError(t, bDB.Update(func(tx *bolt.Tx) error {
			b, err := tx.CreateBucket([]byte("test"))
			require.NoError(t, err)
			idx, err := index.NewUniqueIndex(b, []byte("secret-idx"))
			require.NoError(t, err)

			// Trigger ErrNilParam with secret values
			err = idx.Add(context.Background(), nil, []byte(secretTargetID))
			require.Error(t, err)
			msg := err.Error()
			require.NotContains(t, msg, secretTargetID)

			err = idx.Add(context.Background(), []byte(secretIndexVal), nil)
			require.Error(t, err)
			msg = err.Error()
			require.NotContains(t, msg, secretIndexVal)

			return nil
		}))
	})
}

// ---------------------------------------------------------------------------
// All operation labels covered (extended from C2A)
// ---------------------------------------------------------------------------

func TestWrap_AllLabels_C2B(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()
	nilCtx := context.Context(nil)

	require.NoError(t, db.Save(ctx, &User{ID: 1, Name: "John", Slug: "john"}))

	tests := []struct {
		label  string
		prefix string
		op     func() error
	}{
		// Finder
		{"one", "rainstorm one:", func() error { return db.One(nilCtx, "Name", "John", &User{}) }},
		{"find", "rainstorm find:", func() error { return db.Find(ctx, "Name", "John", &[]UniqueNameUser{}) }},
		{"all by index", "rainstorm all by index:", func() error { return db.From("nonexistent-b").AllByIndex(ctx, "field", &[]User{}) }},
		{"all", "rainstorm all:", func() error { return db.All(nilCtx, &[]User{}) }},
		{"range", "rainstorm range:", func() error { return db.Range(ctx, "Name", "John", "John", &User{}) }},
		{"prefix", "rainstorm prefix:", func() error { return db.Prefix(ctx, "Name", "Jo", &User{}) }},
		{"count", "rainstorm count:", func() error { _, err := db.Count(nilCtx, &User{}); return err }},

		// Query
		{"query find", "rainstorm query find:", func() error { return db.Select(q.True()).Find(nilCtx, &[]User{}) }},
		{"query first", "rainstorm query first:", func() error { return db.Select(q.True()).First(nilCtx, &User{}) }},
		{"query delete", "rainstorm query delete:", func() error { return db.Select(q.True()).Delete(nilCtx, &User{}) }},
		{"query count", "rainstorm query count:", func() error { _, err := db.Select(q.True()).Count(nilCtx, &User{}); return err }},
		{"query raw", "rainstorm query raw:", func() error { _, err := db.Select(q.True()).Raw(nilCtx); return err }},
		{"query raw each", "rainstorm query raw each:", func() error { return db.Select(q.True()).RawEach(ctx, nil) }},
		{"query each", "rainstorm query each:", func() error { return db.Select(q.True()).Each(ctx, &User{}, nil) }},

		// Scanners
		{"prefix scan", "rainstorm prefix scan:", func() error { _, err := db.From("x").PrefixScan(nilCtx, "p"); return err }},
		{"range scan", "rainstorm range scan:", func() error { _, err := db.From("x").RangeScan(nilCtx, "a", "z"); return err }},
	}

	for _, tc := range tests {
		t.Run(tc.label, func(t *testing.T) {
			err := tc.op()
			require.Error(t, err)
			require.True(t, strings.HasPrefix(err.Error(), tc.prefix),
				"label %q: expected prefix %q, got %q", tc.label, tc.prefix, err.Error())
		})
	}
}
