package rainstorm

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/AndersonBargas/rainstorm/v6/codec"
	"github.com/AndersonBargas/rainstorm/v6/codec/json"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

// ---------------------------------------------------------------------------
// cancellationCodec wraps a delegate codec and can cancel a context during
// Marshal or Unmarshal. Used only in tests.
// ---------------------------------------------------------------------------

type cancellationCodec struct {
	delegate    codec.MarshalUnmarshaler
	mu          sync.Mutex
	onMarshal   func()
	onUnmarshal func()
}

func (c *cancellationCodec) Name() string { return c.delegate.Name() }

func (c *cancellationCodec) Marshal(v interface{}) ([]byte, error) {
	raw, err := c.delegate.Marshal(v)
	c.mu.Lock()
	fn := c.onMarshal
	c.mu.Unlock()
	if fn != nil {
		fn()
	}
	return raw, err
}

func (c *cancellationCodec) Unmarshal(raw []byte, to interface{}) error {
	err := c.delegate.Unmarshal(raw, to)
	c.mu.Lock()
	fn := c.onUnmarshal
	c.mu.Unlock()
	if fn != nil {
		fn()
	}
	return err
}

func (c *cancellationCodec) setOnMarshal(fn func()) {
	c.mu.Lock()
	c.onMarshal = fn
	c.mu.Unlock()
}

func (c *cancellationCodec) setOnUnmarshal(fn func()) {
	c.mu.Lock()
	c.onUnmarshal = fn
	c.mu.Unlock()
}

// ---------------------------------------------------------------------------
// 17.1 Save cancelled during marshal
// ---------------------------------------------------------------------------

func TestSave_CancellationDuringMarshalRollsBackRecordAndIndexes(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())

	cc := &cancellationCodec{delegate: json.Codec}
	cc.setOnMarshal(func() { cancel() })

	n := db.WithCodec(cc).(*node)

	err := n.Save(ctx, &UniqueNameUser{ID: 1, Name: "unique-marshal-cancel", Age: 10})
	require.True(t, errors.Is(err, context.Canceled), "expected Canceled, got %v", err)

	// Record must not exist.
	var u UniqueNameUser
	err = db.One(context.Background(), "ID", 1, &u)
	require.True(t, errors.Is(err, ErrNotFound))

	// Unique index must have no orphan.
	err = db.One(context.Background(), "Name", "unique-marshal-cancel", &u)
	require.True(t, errors.Is(err, ErrNotFound))

	// A new Save with the same unique value must succeed.
	err = db.Save(context.Background(), &UniqueNameUser{ID: 2, Name: "unique-marshal-cancel", Age: 20})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// 17.2 Update cancelled during marshal
// ---------------------------------------------------------------------------

func TestUpdate_CancellationDuringMarshalPreservesRecordAndIndexes(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	// Seed old value.
	err := db.Save(ctx, &UniqueNameUser{ID: 1, Name: "old-name", Age: 10})
	require.NoError(t, err)

	// Update with cancellation during marshal.
	updateCtx, cancel := context.WithCancel(context.Background())
	cc := &cancellationCodec{delegate: json.Codec}
	cc.setOnMarshal(func() { cancel() })

	n := db.WithCodec(cc).(*node)

	updated := UniqueNameUser{ID: 1, Name: "new-name", Age: 20}
	err = n.Update(updateCtx, &updated)
	require.True(t, errors.Is(err, context.Canceled), "expected Canceled, got %v", err)

	// Old record must remain intact.
	var u UniqueNameUser
	err = db.One(ctx, "ID", 1, &u)
	require.NoError(t, err)
	require.Equal(t, "old-name", u.Name)
	require.Equal(t, 10, u.Age)

	// Old unique index must remain.
	err = db.One(ctx, "Name", "old-name", &u)
	require.NoError(t, err)

	// New unique index must not exist.
	err = db.One(ctx, "Name", "new-name", &u)
	require.True(t, errors.Is(err, ErrNotFound))

	// A new record can use the new unique value.
	err = db.Save(ctx, &UniqueNameUser{ID: 2, Name: "new-name", Age: 30})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// 17.3 UpdateField cancelled during marshal
// ---------------------------------------------------------------------------

func TestUpdateField_CancellationDuringMarshalPreservesStoredValue(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	err := db.Save(ctx, &UniqueNameUser{ID: 1, Name: "original", Age: 10})
	require.NoError(t, err)

	updateCtx, cancel := context.WithCancel(context.Background())
	cc := &cancellationCodec{delegate: json.Codec}
	cc.setOnMarshal(func() { cancel() })

	n := db.WithCodec(cc).(*node)

	err = n.UpdateField(updateCtx, &UniqueNameUser{ID: 1}, "Name", "changed")
	require.True(t, errors.Is(err, context.Canceled), "expected Canceled, got %v", err)

	// Original value must remain.
	var u UniqueNameUser
	err = db.One(ctx, "ID", 1, &u)
	require.NoError(t, err)
	require.Equal(t, "original", u.Name)
}

// ---------------------------------------------------------------------------
// 18.1 Set cancelled during marshal
// ---------------------------------------------------------------------------

func TestSet_CancellationDuringMarshalDoesNotPersist(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cc := &cancellationCodec{delegate: json.Codec}
	cc.setOnMarshal(func() { cancel() })

	n := db.WithCodec(cc).(*node)

	err := n.Set(ctx, "test-bucket", "key1", "some-value")
	require.True(t, errors.Is(err, context.Canceled), "expected Canceled, got %v", err)

	// Nothing persisted.
	var s string
	err = db.Get(context.Background(), "test-bucket", "key1", &s)
	require.True(t, errors.Is(err, ErrNotFound))
}

// ---------------------------------------------------------------------------
// 18.2 Get cancelled during unmarshal preserves destination
// ---------------------------------------------------------------------------

func TestGet_CancellationDuringUnmarshalPreservesDestination(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	// Seed data with a normal codec.
	err := db.Set(ctx, "bucket", "key", "preserved-value")
	require.NoError(t, err)

	// Now read with a codec that cancels during unmarshal.
	getCtx, cancel := context.WithCancel(context.Background())
	cc := &cancellationCodec{delegate: json.Codec}
	cc.setOnUnmarshal(func() { cancel() })

	n := db.WithCodec(cc).(*node)

	// Initialize destination with sentinel value.
	dest := "SENTINEL"
	err = n.Get(getCtx, "bucket", "key", &dest)
	require.True(t, errors.Is(err, context.Canceled), "expected Canceled, got %v", err)

	// Destination must be unchanged.
	require.Equal(t, "SENTINEL", dest,
		"destination must not be mutated on cancellation")
}

// ---------------------------------------------------------------------------
// 18.3 Get codec error preserves destination
// ---------------------------------------------------------------------------

func TestGet_CodecErrorPreservesDestination(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	// Seed raw bytes that cause json unmarshal to fail.
	err := db.SetBytes(ctx, "bucket", "key", []byte("not-valid-json{{{}}"))
	require.NoError(t, err)

	dest := "SENTINEL"
	err = db.Get(ctx, "bucket", "key", &dest)
	require.Error(t, err)

	// Destination must be unchanged.
	require.Equal(t, "SENTINEL", dest,
		"destination must not be mutated on codec error")
}

// ---------------------------------------------------------------------------
// 18.4 Get temporary decode supports destination shapes
// ---------------------------------------------------------------------------

func TestGet_TemporaryDecodeSupportsDestinationShapes(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	// Struct.
	err := db.Save(ctx, &SimpleUser{ID: 1, Name: "struct-test"})
	require.NoError(t, err)

	var su SimpleUser
	err = db.Get(ctx, "SimpleUser", 1, &su)
	require.NoError(t, err)
	require.Equal(t, "struct-test", su.Name)

	// Scalar.
	err = db.Set(ctx, "scalars", "age", 42)
	require.NoError(t, err)

	var age int
	err = db.Get(ctx, "scalars", "age", &age)
	require.NoError(t, err)
	require.Equal(t, 42, age)

	// Slice.
	err = db.Set(ctx, "slices", "ids", []int{1, 2, 3})
	require.NoError(t, err)

	var ids []int
	err = db.Get(ctx, "slices", "ids", &ids)
	require.NoError(t, err)
	require.Equal(t, []int{1, 2, 3}, ids)
}

// ---------------------------------------------------------------------------
// 18.6 GetBytes error does not return data
// ---------------------------------------------------------------------------

func TestGetBytes_ErrorDoesNotReturnData(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	// A missing bucket returns ErrNotFound and no partial value.
	value, err := db.GetBytes(context.Background(), "nonexistent", "key")
	require.ErrorIs(t, err, ErrNotFound)
	require.Nil(t, value, "GetBytes must return nil on error")
}

// ---------------------------------------------------------------------------
// 18.7 KeyExists
// ---------------------------------------------------------------------------

func TestKeyExists_Behavior(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	err := db.Set(ctx, "bucket", "existing", "value")
	require.NoError(t, err)

	// Existent key.
	ok, err := db.KeyExists(ctx, "bucket", "existing")
	require.NoError(t, err)
	require.True(t, ok)

	// Absent key in existing bucket.
	ok, err = db.KeyExists(ctx, "bucket", "missing")
	require.NoError(t, err)
	require.False(t, ok)

	// Missing bucket preserves the existing ErrNotFound behavior.
	ok, err = db.KeyExists(ctx, "missing-bucket", "key")
	require.ErrorIs(t, err, ErrNotFound)
	require.False(t, ok)

	// Cancelled context returns false, error.
	ok, err = db.KeyExists(canceledCtx(), "bucket", "existing")
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))
	require.False(t, ok, "KeyExists must return false on cancellation")
}

// ---------------------------------------------------------------------------
// readWriteTx pre-commit cancellation
// ---------------------------------------------------------------------------

func TestWriteOperation_RollbackOnPreCommitCancellation(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	n := db.Node.(*node)

	err := n.readWriteTx(ctx, func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte("precommit"))
		if err != nil {
			return err
		}
		if err := bucket.Put([]byte("key"), []byte("value")); err != nil {
			return err
		}

		// The callback succeeds after writing, but cancellation must be observed
		// by readWriteTx before bbolt is allowed to commit.
		cancel()
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)

	err = db.Bolt.View(func(tx *bolt.Tx) error {
		require.Nil(t, tx.Bucket([]byte("precommit")), "canceled transaction must roll back bucket creation")
		return nil
	})
	require.NoError(t, err)
}
