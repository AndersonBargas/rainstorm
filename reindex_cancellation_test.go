package rainstorm

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AndersonBargas/rainstorm/v6/codec"
	"github.com/AndersonBargas/rainstorm/v6/codec/json"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

// ---------------------------------------------------------------------------
// stepContext — a deterministic context that cancels after N calls to Err().
// ---------------------------------------------------------------------------

type stepContext struct {
	mu       sync.Mutex
	calls    int
	cancelAt int
}

func (c *stepContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *stepContext) Done() <-chan struct{}       { return nil }
func (c *stepContext) Value(key interface{}) interface{} {
	return nil
}
func (c *stepContext) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if c.calls >= c.cancelAt {
		return context.Canceled
	}
	return nil
}

// Calls returns how many times Err() has been invoked.
func (c *stepContext) Calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// cancelAtCallerContext cancels on the Nth checkContext call made directly
// by a function whose fully-qualified name ends with callerSuffix.
type cancelAtCallerContext struct {
	mu           sync.Mutex
	callerSuffix string
	cancelAt     int
	hits         int
}

func (c *cancelAtCallerContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *cancelAtCallerContext) Done() <-chan struct{}       { return nil }
func (c *cancelAtCallerContext) Value(key interface{}) interface{} {
	return nil
}
func (c *cancelAtCallerContext) Err() error {
	pcs := make([]uintptr, 16)
	n := runtime.Callers(2, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		if !strings.HasSuffix(frame.Function, ".checkContext") {
			if strings.HasSuffix(frame.Function, c.callerSuffix) {
				c.mu.Lock()
				c.hits++
				cancel := c.hits >= c.cancelAt
				c.mu.Unlock()
				if cancel {
					return context.Canceled
				}
			}
			break
		}
		if !more {
			break
		}
	}
	return nil
}
func (c *cancelAtCallerContext) Hits() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits
}

// ---------------------------------------------------------------------------
// marshalErrorCancelingCodec — a codec that cancels the context and returns an
// error during Marshal. Used to prove non-context error precedence.
// ---------------------------------------------------------------------------

var errMarshalForced = errors.New("forced marshal error")

type marshalErrorCancelingCodec struct {
	delegate codec.MarshalUnmarshaler
	cancel   context.CancelFunc
}

func (c *marshalErrorCancelingCodec) Name() string { return c.delegate.Name() }
func (c *marshalErrorCancelingCodec) Marshal(v interface{}) ([]byte, error) {
	c.cancel()
	return nil, errMarshalForced
}
func (c *marshalErrorCancelingCodec) Unmarshal(raw []byte, to interface{}) error {
	return c.delegate.Unmarshal(raw, to)
}

// ---------------------------------------------------------------------------
// reindexUser — a struct with unique, list/index, and id fields for testing.
// ---------------------------------------------------------------------------

type reindexUser struct {
	ID    int    `rainstorm:"id"`
	Name  string `rainstorm:"index"`
	Group string `rainstorm:"unique"`
	Score int    `rainstorm:"index"`
}

// ============================================================================
// Test 1: Already-canceled ReIndex does not modify indexes.
// ============================================================================

func TestReIndex_AlreadyCanceledDoesNotModifyIndexes(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	// Seed indexed data.
	for i := 1; i <= 5; i++ {
		u := reindexUser{
			ID: i, Name: "user-" + string(rune('a'+i-1)),
			Group: "group-" + string(rune('A'+i-1)), Score: i * 10,
		}
		require.NoError(t, db.Save(ctx, &u))
	}

	// Verify indexes exist before the canceled ReIndex.
	var found []reindexUser
	require.NoError(t, db.Find(ctx, "Name", "user-a", &found))
	require.Len(t, found, 1)
	found = nil
	require.NoError(t, db.Find(ctx, "Score", 20, &found))
	require.Len(t, found, 1)
	found = nil
	require.NoError(t, db.Find(ctx, "Group", "group-A", &found))
	require.Len(t, found, 1)

	// ReIndex with canceled context.
	err := db.ReIndex(canceledCtx(), &reindexUser{})
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))

	// Indexes must still work.
	found = nil
	require.NoError(t, db.Find(ctx, "Name", "user-a", &found))
	require.Len(t, found, 1)
	found = nil
	require.NoError(t, db.Find(ctx, "Score", 20, &found))
	require.Len(t, found, 1)
	found = nil
	require.NoError(t, db.Find(ctx, "Group", "group-A", &found))
	require.Len(t, found, 1)
}

// ============================================================================
// Test 2: Deadline-exceeded ReIndex does not modify indexes.
// ============================================================================

func TestReIndex_DeadlineExceededDoesNotModifyIndexes(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	// Seed indexed data.
	for i := 1; i <= 5; i++ {
		u := reindexUser{
			ID: i, Name: "user-" + string(rune('a'+i-1)),
			Group: "group-" + string(rune('A'+i-1)), Score: i * 10,
		}
		require.NoError(t, db.Save(ctx, &u))
	}

	// ReIndex with deadline-exceeded context.
	err := db.ReIndex(timedOutCtx(), &reindexUser{})
	require.Error(t, err)
	require.True(t, errors.Is(err, context.DeadlineExceeded))

	// Indexes must still work.
	var found []reindexUser
	require.NoError(t, db.Find(ctx, "Name", "user-a", &found))
	require.Len(t, found, 1)
}

// ============================================================================
// Test 3: Cancellation during the record cursor loop returns context.Canceled.
// ============================================================================

func TestReIndex_CancellationDuringRecordLoop(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	// Seed 5 indexed records.
	for i := 1; i <= 5; i++ {
		u := reindexUser{
			ID: i, Name: "user-" + string(rune('a'+i-1)),
			Group: "group-" + string(rune('A'+i-1)), Score: i * 10,
		}
		require.NoError(t, db.Save(ctx, &u))
	}

	// Cancel at Update's entry. ReIndex only calls Update after First has read
	// a record, so this deterministically proves the record loop was reached.
	sctx := &cancelAtCallerContext{
		callerSuffix: ".(*node).update",
		cancelAt:     1,
	}
	err := db.ReIndex(sctx, &reindexUser{})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, sctx.Hits())
}

// ============================================================================
// Test 4: Cancellation after at least one index mutation rolls back all mutations.
// ============================================================================

func TestReIndex_CancellationAfterMutationRollsBackAll(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	// Seed indexed data.
	for i := 1; i <= 5; i++ {
		u := reindexUser{
			ID: i, Name: "user-" + string(rune('a'+i-1)),
			Group: "group-" + string(rune('A'+i-1)), Score: i * 10,
		}
		require.NoError(t, db.Save(ctx, &u))
	}

	// Verify indexes exist pre-ReIndex via bbolt.
	require.NoError(t, db.Bolt.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("reindexUser"))
		require.NotNil(t, b)
		require.NotNil(t, b.Bucket([]byte(indexPrefix+"Name")))
		require.NotNil(t, b.Bucket([]byte(indexPrefix+"Group")))
		require.NotNil(t, b.Bucket([]byte(indexPrefix+"Score")))
		return nil
	}))

	// The direct reIndex checks are: entry, post-scan, pre-delete, post-delete.
	// Canceling on the fourth direct check therefore happens after the first
	// DeleteBucket mutation and forces bbolt to roll it back.
	sctx := &cancelAtCallerContext{
		callerSuffix: ".(*node).reIndex",
		cancelAt:     4,
	}
	err := db.ReIndex(sctx, &reindexUser{})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 4, sctx.Hits())

	// All index buckets must be back after rollback.
	require.NoError(t, db.Bolt.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("reindexUser"))
		require.NotNil(t, b)
		require.NotNil(t, b.Bucket([]byte(indexPrefix+"Name")))
		require.NotNil(t, b.Bucket([]byte(indexPrefix+"Group")))
		require.NotNil(t, b.Bucket([]byte(indexPrefix+"Score")))
		return nil
	}))

	// Indexes must be fully functional.
	var found []reindexUser
	require.NoError(t, db.Find(ctx, "Name", "user-a", &found))
	require.Len(t, found, 1)
}

// ============================================================================
// Test 5: Cancellation while rebuilding multiple indexed fields rolls back all.
// ============================================================================

func TestReIndex_CancellationDuringMultiFieldRebuildRollsBack(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	// Seed data with all three index types filled.
	for i := 1; i <= 5; i++ {
		u := reindexUser{
			ID: i, Name: "multi-" + string(rune('a'+i-1)),
			Group: "grp-" + string(rune('A'+i-1)), Score: i * 100,
		}
		require.NoError(t, db.Save(ctx, &u))
	}

	// UniqueIndex.Add checks context at entry, before Put, and after Put.
	// Canceling on its third direct check proves a rebuilt unique-index entry
	// was written before cancellation; the enclosing transaction must undo it
	// together with all prior index reconstruction work.
	sctx := &cancelAtCallerContext{
		callerSuffix: ".(*UniqueIndex).Add",
		cancelAt:     3,
	}
	err := db.ReIndex(sctx, &reindexUser{})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 3, sctx.Hits())

	// All index buckets must be back.
	require.NoError(t, db.Bolt.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("reindexUser"))
		require.NotNil(t, b)
		require.NotNil(t, b.Bucket([]byte(indexPrefix+"Name")))
		require.NotNil(t, b.Bucket([]byte(indexPrefix+"Group")))
		require.NotNil(t, b.Bucket([]byte(indexPrefix+"Score")))
		return nil
	}))

	// All three index types must work.
	var found []reindexUser
	require.NoError(t, db.Find(ctx, "Name", "multi-a", &found))
	require.Len(t, found, 1)
	found = nil
	require.NoError(t, db.Find(ctx, "Score", 100, &found))
	require.Len(t, found, 1)
	found = nil
	require.NoError(t, db.Find(ctx, "Group", "grp-A", &found))
	require.Len(t, found, 1)
}

// ============================================================================
// Test 6: Unique indexes retain their previous committed state after rollback.
// ============================================================================

func TestReIndex_UniqueIndexPreservedAfterRollback(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	// Seed with unique group values.
	for i := 1; i <= 5; i++ {
		u := reindexUser{
			ID: i, Name: "unique-test-" + string(rune('a'+i-1)),
			Group: "unique-group-" + string(rune('A'+i-1)), Score: i * 10,
		}
		require.NoError(t, db.Save(ctx, &u))
	}

	// Cancel during the record loop.
	sctx := &stepContext{cancelAt: 35}
	err := db.ReIndex(sctx, &reindexUser{})
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))

	// Unique index must still work.
	var found []reindexUser
	require.NoError(t, db.Find(ctx, "Group", "unique-group-A", &found))
	require.Len(t, found, 1)
	require.Equal(t, 1, found[0].ID)

	// A new save with a different unique value must succeed.
	err = db.Save(ctx, &reindexUser{ID: 10, Name: "new", Group: "unique-group-Z", Score: 99})
	require.NoError(t, err)

	// But a duplicate unique value must fail.
	err = db.Save(ctx, &reindexUser{ID: 11, Name: "dup", Group: "unique-group-A", Score: 55})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrAlreadyExists))
}

// ============================================================================
// Test 7: List/non-unique indexes retain their previous committed state after rollback.
// ============================================================================

func TestReIndex_ListIndexPreservedAfterRollback(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	// Seed with list-indexed scores where some share the same score.
	// All scores must be non-zero to ensure they are indexed (zero values
	// are treated as "not set" and are not added to list indexes).
	for i := 1; i <= 6; i++ {
		u := reindexUser{
			ID: i, Name: "list-" + string(rune('a'+i-1)),
			Group: "group-" + string(rune('A'+i-1)), Score: (i%2 + 1) * 10,
		}
		require.NoError(t, db.Save(ctx, &u))
	}

	sctx := &stepContext{cancelAt: 35}
	err := db.ReIndex(sctx, &reindexUser{})
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))

	// List index must still work: find all with Score=20 (should be 3 records).
	var found []reindexUser
	require.NoError(t, db.Find(ctx, "Score", 20, &found))
	require.Len(t, found, 3)

	// Find by Score=10 should also still work (3 records).
	found = nil
	require.NoError(t, db.Find(ctx, "Score", 10, &found))
	require.Len(t, found, 3)
}

// ============================================================================
// Test 8: ID lookup remains correct after rollback.
// ============================================================================

func TestReIndex_IDLookupCorrectAfterRollback(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	// Seed data.
	for i := 1; i <= 5; i++ {
		u := reindexUser{
			ID: i, Name: "idtest-" + string(rune('a'+i-1)),
			Group: "group-" + string(rune('A'+i-1)), Score: i * 10,
		}
		require.NoError(t, db.Save(ctx, &u))
	}

	sctx := &stepContext{cancelAt: 35}
	err := db.ReIndex(sctx, &reindexUser{})
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))

	// ID lookup must work for every record.
	for i := 1; i <= 5; i++ {
		var u reindexUser
		require.NoError(t, db.One(ctx, "ID", i, &u))
		require.Equal(t, i, u.ID)
	}
}

// ============================================================================
// Test 9: A later successful ReIndex works after a canceled ReIndex.
// ============================================================================

func TestReIndex_SuccessfulAfterCanceled(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	// Seed data.
	for i := 1; i <= 5; i++ {
		u := reindexUser{
			ID: i, Name: "retry-" + string(rune('a'+i-1)),
			Group: "group-" + string(rune('A'+i-1)), Score: i * 10,
		}
		require.NoError(t, db.Save(ctx, &u))
	}

	// First ReIndex: cancel.
	sctx := &stepContext{cancelAt: 35}
	err := db.ReIndex(sctx, &reindexUser{})
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))

	// Second ReIndex: succeed.
	require.NoError(t, db.ReIndex(ctx, &reindexUser{}))

	// Indexes must work after successful ReIndex.
	var found []reindexUser
	require.NoError(t, db.Find(ctx, "Name", "retry-a", &found))
	require.Len(t, found, 1)
	found = nil
	require.NoError(t, db.Find(ctx, "Group", "group-A", &found))
	require.Len(t, found, 1)
}

// ============================================================================
// Test 10: Successful ReIndex repairs deliberately stale index state.
// ============================================================================

func TestReIndex_SuccessfulRepairsStaleIndexes(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	// Seed data with one index configuration.
	for i := 1; i <= 5; i++ {
		u := reindexUser{
			ID: i, Name: "stale-" + string(rune('a'+i-1)),
			Group: "group-" + string(rune('A'+i-1)), Score: i * 10,
		}
		require.NoError(t, db.Save(ctx, &u))
	}

	// Deliberately corrupt the index state via direct bbolt access.
	// Delete the Score index bucket, simulating stale index state.
	require.NoError(t, db.Bolt.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("reindexUser"))
		if b == nil {
			return ErrNotFound
		}
		return b.DeleteBucket([]byte(indexPrefix + "Score"))
	}))

	// Verify Score index is now gone.
	require.NoError(t, db.Bolt.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("reindexUser"))
		require.NotNil(t, b)
		require.Nil(t, b.Bucket([]byte(indexPrefix+"Score")),
			"Score index should be missing after deliberate corruption")
		return nil
	}))

	// Score lookup should fail.
	var found []reindexUser
	err := db.Find(ctx, "Score", 10, &found)
	require.Error(t, err)

	// Run successful ReIndex to repair.
	require.NoError(t, db.ReIndex(ctx, &reindexUser{}))

	// Score index must be recreated and functional.
	require.NoError(t, db.Bolt.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("reindexUser"))
		require.NotNil(t, b)
		require.NotNil(t, b.Bucket([]byte(indexPrefix+"Score")),
			"Score index should be recreated after ReIndex")
		return nil
	}))

	// Score lookup must now work.
	found = nil
	require.NoError(t, db.Find(ctx, "Score", 10, &found))
	require.Len(t, found, 1)
	require.Equal(t, "stale-a", found[0].Name)

	// Other indexes must still work.
	found = nil
	require.NoError(t, db.Find(ctx, "Name", "stale-b", &found))
	require.Len(t, found, 1)
}

// ============================================================================
// Test 11: ReIndex inside a managed WriteTransaction rolls back all writes.
// ============================================================================

func TestReIndex_InsideWriteTransactionCancellationRollsBackAll(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())

	// Seed some User data and a separate reindexUser data.
	require.NoError(t, db.Save(context.Background(), &User{ID: 1, Name: "tx-write", Slug: "txw"}))

	for i := 1; i <= 5; i++ {
		u := reindexUser{
			ID: i, Name: "wtx-" + string(rune('a'+i-1)),
			Group: "group-" + string(rune('A'+i-1)), Score: i * 10,
		}
		require.NoError(t, db.Save(context.Background(), &u))
	}

	err := db.WriteTransaction(ctx, func(txNode Node) error {
		// Perform a separate write.
		if err := txNode.Save(context.Background(), &User{ID: 2, Name: "tx-write2", Slug: "txw2"}); err != nil {
			return err
		}

		// Run ReIndex inside the same transaction.
		if err := txNode.ReIndex(context.Background(), &reindexUser{}); err != nil {
			return err
		}

		// Cancel before commit.
		cancel()
		return nil
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))

	// The User save inside the transaction must be rolled back.
	var u User
	err = db.One(context.Background(), "ID", 2, &u)
	require.True(t, errors.Is(err, ErrNotFound),
		"User saved inside transaction must be rolled back")

	// The ReIndex changes must be rolled back. Original indexes must work.
	var found []reindexUser
	require.NoError(t, db.Find(context.Background(), "Name", "wtx-a", &found))
	require.Len(t, found, 1)
}

// ============================================================================
// Test 12: Non-context error precedence over cancellation.
// ============================================================================

func TestReIndex_NonContextErrorPrecedence(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	// Seed data.
	for i := 1; i <= 5; i++ {
		u := reindexUser{
			ID: i, Name: "errprec-" + string(rune('a'+i-1)),
			Group: "group-" + string(rune('A'+i-1)), Score: i * 10,
		}
		require.NoError(t, db.Save(ctx, &u))
	}

	// Use a codec that cancels the context AND returns a non-context error
	// during Marshal.
	opCtx, cancel := context.WithCancel(context.Background())
	cc := &marshalErrorCancelingCodec{delegate: json.Codec, cancel: cancel}
	n := db.WithCodec(cc).(*node)

	err := n.ReIndex(opCtx, &reindexUser{})
	require.Error(t, err)
	require.True(t, errors.Is(err, errMarshalForced),
		"expected codec error, got %v", err)
	require.False(t, errors.Is(err, context.Canceled),
		"non-context error must take precedence over cancellation")
}
