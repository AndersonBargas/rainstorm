package rainstorm

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/AndersonBargas/rainstorm/v6/codec"
	"github.com/AndersonBargas/rainstorm/v6/codec/json"
	"github.com/AndersonBargas/rainstorm/v6/index"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// finderStepContext — deterministic local context that cancels after N calls
// to Err(). Thread-safe, no timers, no goroutines.
// ---------------------------------------------------------------------------

type finderStepContext struct {
	mu       sync.Mutex
	calls    int
	cancelAt int
}

func (c *finderStepContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *finderStepContext) Done() <-chan struct{}       { return nil }
func (c *finderStepContext) Value(key interface{}) interface{} {
	return nil
}
func (c *finderStepContext) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if c.calls >= c.cancelAt {
		return context.Canceled
	}
	return nil
}

// Calls returns how many times Err() has been invoked.
func (c *finderStepContext) Calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// ---------------------------------------------------------------------------
// codecErrorCodec — wraps a delegate, unmarshals into to, then returns an
// error. Used to prove destination is preserved on codec error.
// ---------------------------------------------------------------------------

var errCodec = errors.New("codec error sentinel")

type codecErrorCodec struct {
	delegate codec.MarshalUnmarshaler
}

func (c *codecErrorCodec) Name() string { return c.delegate.Name() }

func (c *codecErrorCodec) Marshal(v interface{}) ([]byte, error) {
	return c.delegate.Marshal(v)
}

func (c *codecErrorCodec) Unmarshal(raw []byte, to interface{}) error {
	// Unmarshal into the temporary first, then return a sentinel error.
	if err := c.delegate.Unmarshal(raw, to); err != nil {
		return err
	}
	return errCodec
}

// ---------------------------------------------------------------------------
// 12.1 One
// ---------------------------------------------------------------------------

func TestOne_IndexedCancellationDuringUnmarshalPreservesDestination(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	// Seed a record with a normal codec.
	err := db.Save(context.Background(), &UniqueNameUser{ID: 1, Name: "john", Age: 10})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cc := &cancellationCodec{delegate: json.Codec}
	cc.setOnUnmarshal(func() { cancel() })

	n := db.WithCodec(cc).(*node)
	dest := UniqueNameUser{ID: 999, Name: "SENTINEL", Age: 999}
	err = n.One(ctx, "Name", "john", &dest)
	require.True(t, errors.Is(err, context.Canceled), "expected Canceled, got %v", err)
	require.Equal(t, UniqueNameUser{ID: 999, Name: "SENTINEL", Age: 999}, dest,
		"destination must be preserved on cancellation")
}

func TestOne_IndexedCodecErrorPreservesDestination(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	// Seed a record.
	err := db.Save(context.Background(), &UniqueNameUser{ID: 1, Name: "john", Age: 10})
	require.NoError(t, err)

	// Use the error codec so Unmarshal writes then returns an error.
	n := db.WithCodec(&codecErrorCodec{delegate: json.Codec}).(*node)
	dest := UniqueNameUser{ID: 999, Name: "SENTINEL", Age: 999}
	err = n.One(context.Background(), "Name", "john", &dest)
	require.True(t, errors.Is(err, errCodec), "expected errCodec, got %v", err)
	// With codecErrorCodec, the delegate unmarshals successfully first,
	// so the temporary was filled. But since the error is returned,
	// the destination must still be the sentinel.
	require.Equal(t, UniqueNameUser{ID: 999, Name: "SENTINEL", Age: 999}, dest,
		"destination must be preserved on codec error")
}

func TestOne_ByIDCancellationDuringUnmarshalPreservesDestination(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	// Seed via ID path.
	err := db.Save(context.Background(), &UniqueNameUser{ID: 42, Name: "by-id", Age: 10})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cc := &cancellationCodec{delegate: json.Codec}
	cc.setOnUnmarshal(func() { cancel() })

	n := db.WithCodec(cc).(*node)
	dest := UniqueNameUser{ID: 999, Name: "SENTINEL", Age: 999}
	err = n.One(ctx, "ID", 42, &dest)
	require.True(t, errors.Is(err, context.Canceled), "expected Canceled, got %v", err)
	require.Equal(t, UniqueNameUser{ID: 999, Name: "SENTINEL", Age: 999}, dest,
		"destination must be preserved on cancellation (ID path)")
}

// ---------------------------------------------------------------------------
// 12.2 Find
// ---------------------------------------------------------------------------

func TestFind_IndexedCancellationDuringMaterializationPreservesDestination(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	// Seed multiple records with Name indexed.
	for i := 1; i <= 5; i++ {
		err := db.Save(context.Background(), &IndexedNameUser{
			ID: i, Name: "test-find", Score: i * 10,
		})
		require.NoError(t, err)
	}

	// Cancel on the second unmarshal during materialization.
	ctx, cancel := context.WithCancel(context.Background())
	var unmarshalCalls int
	cc := &cancellationCodec{delegate: json.Codec}
	cc.setOnUnmarshal(func() {
		unmarshalCalls++
		if unmarshalCalls >= 2 {
			cancel()
		}
	})

	n := db.WithCodec(cc).(*node)

	// Initialize destination with a sentinel non-empty slice.
	sentinel := IndexedNameUser{ID: 999, Name: "SENTINEL", Score: 999}
	dest := []IndexedNameUser{sentinel}
	err := n.Find(ctx, "Name", "test-find", &dest)
	require.True(t, errors.Is(err, context.Canceled), "expected Canceled, got %v", err)
	// The sentinel slice must be preserved — no partial results.
	require.Equal(t, []IndexedNameUser{sentinel}, dest,
		"destination must be preserved on cancellation during materialization")
	require.GreaterOrEqual(t, unmarshalCalls, 2,
		"must prove more than one checkpoint was traversed")
}

func TestFind_IndexedIndexCancellationPreservesDestination(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	// Seed records.
	for i := 1; i <= 5; i++ {
		err := db.Save(context.Background(), &IndexedNameUser{
			ID: i, Name: "idx-cancel", Score: i * 10,
		})
		require.NoError(t, err)
	}

	// Public Find and readTx consume five checks before entering the helper.
	// The helper then checks entry, bucket and getIndex (calls 6-8), so call 9
	// is the first check inside idx.All.
	fsc := &finderStepContext{cancelAt: 9}
	n := db.Node.(*node)

	sentinel := IndexedNameUser{ID: 999, Name: "SENTINEL", Score: 999}
	dest := []IndexedNameUser{sentinel}
	err := n.Find(fsc, "Name", "idx-cancel", &dest)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, []IndexedNameUser{sentinel}, dest,
		"destination must be preserved on index cancellation")
	require.GreaterOrEqual(t, fsc.Calls(), 9,
		"cancellation must occur inside idx.All")
}

func TestFind_IndexedSuccessReplacesDestination(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	// Seed records.
	for i := 1; i <= 3; i++ {
		err := db.Save(context.Background(), &IndexedNameUser{
			ID: i, Name: "success", Score: i * 10,
		})
		require.NoError(t, err)
	}

	// Initialize with a non-empty sentinel slice to prove replacement.
	sentinel := IndexedNameUser{ID: 999, Name: "SENTINEL", Score: 999}
	dest := []IndexedNameUser{sentinel}
	err := db.Find(context.Background(), "Name", "success", &dest)
	require.NoError(t, err)
	require.Len(t, dest, 3)
	require.Equal(t, 1, dest[0].ID)
	require.Equal(t, "success", dest[0].Name)
	require.Equal(t, 3, dest[2].ID)
	// Must not contain the sentinel — destination was replaced, not concatenated.
	for _, d := range dest {
		require.NotEqual(t, 999, d.ID, "sentinel must not appear in result")
	}
}

// ---------------------------------------------------------------------------
// 12.3 AllByIndex
// ---------------------------------------------------------------------------

func TestAllByIndex_CancellationDuringDecodePreservesDestination(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	// Seed records with DateOfBirth indexed (User struct has DateOfBirth index).
	for i := 1; i <= 5; i++ {
		err := db.Save(context.Background(), &User{
			ID:          i,
			Name:        "abx",
			DateOfBirth: time.Now().Add(-time.Duration(i) * time.Hour),
			Slug:        fmt.Sprintf("abx-%d", i),
		})
		require.NoError(t, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var unmarshalCalls int
	cc := &cancellationCodec{delegate: json.Codec}
	cc.setOnUnmarshal(func() {
		unmarshalCalls++
		if unmarshalCalls >= 2 {
			cancel()
		}
	})

	n := db.WithCodec(cc).(*node)

	sentinel := User{ID: 999, Name: "SENTINEL"}
	dest := []User{sentinel}
	err := n.AllByIndex(ctx, "DateOfBirth", &dest)
	require.True(t, errors.Is(err, context.Canceled), "expected Canceled, got %v", err)
	require.Equal(t, []User{sentinel}, dest,
		"destination must be preserved on cancellation during decode")
	require.GreaterOrEqual(t, unmarshalCalls, 2,
		"must prove more than one checkpoint was traversed")
}

func TestAllByIndex_IndexCancellationPreservesDestination(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	for i := 1; i <= 5; i++ {
		err := db.Save(context.Background(), &User{
			ID:          i,
			Name:        "allbyidx",
			DateOfBirth: time.Now().Add(-time.Duration(i) * time.Hour),
			Slug:        fmt.Sprintf("allbyidx-%d", i),
		})
		require.NoError(t, err)
	}

	// Public AllByIndex and readTx consume four checks. The helper consumes
	// entry, bucket, field and getIndex checks (calls 5-8), so call 9 is the
	// first check inside idx.AllRecords.
	fsc := &finderStepContext{cancelAt: 9}
	n := db.Node.(*node)

	sentinel := User{ID: 999, Name: "SENTINEL"}
	dest := []User{sentinel}
	err := n.AllByIndex(fsc, "DateOfBirth", &dest)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, []User{sentinel}, dest,
		"destination must be preserved on index cancellation")
	require.GreaterOrEqual(t, fsc.Calls(), 9,
		"cancellation must occur inside idx.AllRecords")
}

func TestAllByIndex_CodecErrorPreservesDestination(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	for i := 1; i <= 5; i++ {
		err := db.Save(context.Background(), &User{
			ID:          i,
			Name:        "codecerr",
			DateOfBirth: time.Now().Add(-time.Duration(i) * time.Hour),
			Slug:        fmt.Sprintf("codecerr-%d", i),
		})
		require.NoError(t, err)
	}

	n := db.WithCodec(&codecErrorCodec{delegate: json.Codec}).(*node)

	sentinel := User{ID: 999, Name: "SENTINEL"}
	dest := []User{sentinel}
	err := n.AllByIndex(context.Background(), "DateOfBirth", &dest)
	require.True(t, errors.Is(err, errCodec), "expected errCodec, got %v", err)
	// Destination must remain the sentinel — no partial results.
	require.Equal(t, []User{sentinel}, dest,
		"destination must be preserved on codec error")
}

// ---------------------------------------------------------------------------
// 12.4 Range
// ---------------------------------------------------------------------------

func TestRange_IndexedCancellationDuringMaterializationPreservesDestination(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	// Seed users with Slug unique indexed.
	for i := 1; i <= 5; i++ {
		err := db.Save(context.Background(), &User{
			ID:   i,
			Name: "range-test",
			Slug: "A" + string(rune('A'+i)),
		})
		require.NoError(t, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var unmarshalCalls int
	cc := &cancellationCodec{delegate: json.Codec}
	cc.setOnUnmarshal(func() {
		unmarshalCalls++
		if unmarshalCalls >= 2 {
			cancel()
		}
	})

	n := db.WithCodec(cc).(*node)

	sentinel := User{ID: 999, Name: "SENTINEL", Slug: "SENTINEL"}
	dest := []User{sentinel}
	err := n.Range(ctx, "Slug", "AB", "AF", &dest)
	require.True(t, errors.Is(err, context.Canceled), "expected Canceled, got %v", err)
	require.Equal(t, []User{sentinel}, dest,
		"destination must be preserved on cancellation during materialization")
	require.GreaterOrEqual(t, unmarshalCalls, 2,
		"must prove more than one checkpoint was traversed")
}

func TestRange_IndexedIndexCancellationPreservesDestination(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	for i := 1; i <= 5; i++ {
		err := db.Save(context.Background(), &User{
			ID:   i,
			Name: "range-idx",
			Slug: "B" + string(rune('A'+i)),
		})
		require.NoError(t, err)
	}

	// Public Range and readTx consume six checks. The helper consumes entry,
	// bucket and getIndex checks (calls 7-9), so call 10 is the first check
	// inside idx.Range.
	fsc := &finderStepContext{cancelAt: 10}
	n := db.Node.(*node)

	sentinel := User{ID: 999, Name: "SENTINEL", Slug: "SENTINEL"}
	dest := []User{sentinel}
	err := n.Range(fsc, "Slug", "BA", "BF", &dest)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, []User{sentinel}, dest,
		"destination must be preserved on index cancellation")
	require.GreaterOrEqual(t, fsc.Calls(), 10,
		"cancellation must occur inside idx.Range")
}

func TestRange_IndexedEmptyResultPublishesEmptySlice(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	// No records saved — the bucket for User will not exist.
	// Range on an indexed field with no bucket returns nil and empty slice.
	sentinel := User{ID: 999, Name: "SENTINEL", Slug: "SENTINEL"}
	dest := []User{sentinel}
	err := db.Range(context.Background(), "Slug", "A", "Z", &dest)
	require.NoError(t, err)
	require.Len(t, dest, 0,
		"empty result must publish an empty slice")
}

// ---------------------------------------------------------------------------
// 12.5 Prefix
// ---------------------------------------------------------------------------

func TestPrefix_IndexedCancellationDuringMaterializationPreservesDestination(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	// Seed users with Name indexed. Use unique Slug per record.
	for i := 1; i <= 5; i++ {
		err := db.Save(context.Background(), &User{
			ID: i, Name: "prefix-test", Slug: fmt.Sprintf("ptest-%d", i),
		})
		require.NoError(t, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var unmarshalCalls int
	cc := &cancellationCodec{delegate: json.Codec}
	cc.setOnUnmarshal(func() {
		unmarshalCalls++
		if unmarshalCalls >= 2 {
			cancel()
		}
	})

	n := db.WithCodec(cc).(*node)

	sentinel := User{ID: 999, Name: "SENTINEL", Slug: "SENTINEL"}
	dest := []User{sentinel}
	err := n.Prefix(ctx, "Name", "prefix", &dest)
	require.True(t, errors.Is(err, context.Canceled), "expected Canceled, got %v", err)
	require.Equal(t, []User{sentinel}, dest,
		"destination must be preserved on cancellation during materialization")
	require.GreaterOrEqual(t, unmarshalCalls, 2,
		"must prove more than one checkpoint was traversed")
}

func TestPrefix_IndexedIndexCancellationPreservesDestination(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	for i := 1; i <= 5; i++ {
		err := db.Save(context.Background(), &User{
			ID: i, Name: "pref-ix", Slug: fmt.Sprintf("pref-%d", i),
		})
		require.NoError(t, err)
	}

	// Public Prefix and readTx consume five checks. The helper consumes entry,
	// bucket and getIndex checks (calls 6-8), so call 9 is the first check
	// inside idx.Prefix.
	fsc := &finderStepContext{cancelAt: 9}
	n := db.Node.(*node)

	sentinel := User{ID: 999, Name: "SENTINEL", Slug: "SENTINEL"}
	dest := []User{sentinel}
	err := n.Prefix(fsc, "Name", "pref", &dest)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, []User{sentinel}, dest,
		"destination must be preserved on index cancellation")
	require.GreaterOrEqual(t, fsc.Calls(), 9,
		"cancellation must occur inside idx.Prefix")
}

func TestPrefix_IndexedSuccessPublishesCompleteResult(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	for i := 1; i <= 5; i++ {
		err := db.Save(context.Background(), &User{
			ID: i, Name: "pref-ok", Slug: fmt.Sprintf("pok-%d", i),
		})
		require.NoError(t, err)
	}

	sentinel := User{ID: 999, Name: "SENTINEL", Slug: "SENTINEL"}
	dest := []User{sentinel}
	err := db.Prefix(context.Background(), "Name", "pref", &dest)
	require.NoError(t, err)
	require.Len(t, dest, 5)
	// Must not contain sentinel.
	for _, d := range dest {
		require.NotEqual(t, 999, d.ID, "sentinel must not appear in result")
	}
}

// ---------------------------------------------------------------------------
// 12.6 Options — cancellation during option stops before IO
// ---------------------------------------------------------------------------

func TestFinders_CancellationDuringOptionStopsBeforeIO(t *testing.T) {
	t.Run("Find", func(t *testing.T) {
		db, cleanup := createDB(t)
		defer cleanup()

		err := db.Save(context.Background(), &IndexedNameUser{
			ID: 1, Name: "opt-cancel", Score: 10,
		})
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		sentinel := IndexedNameUser{ID: 999, Name: "SENTINEL", Score: 999}
		dest := []IndexedNameUser{sentinel}
		err = db.Find(ctx, "Name", "opt-cancel", &dest, func(opts *index.Options) {
			cancel()
		})
		require.True(t, errors.Is(err, context.Canceled), "expected Canceled, got %v", err)
		require.Equal(t, []IndexedNameUser{sentinel}, dest,
			"destination must be preserved after option cancellation")
	})

	t.Run("AllByIndex", func(t *testing.T) {
		db, cleanup := createDB(t)
		defer cleanup()

		for i := 1; i <= 3; i++ {
			err := db.Save(context.Background(), &User{
				ID: i, Name: "abx-opt", DateOfBirth: time.Now(),
				Slug: fmt.Sprintf("abx-opt-%d", i),
			})
			require.NoError(t, err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		sentinel := User{ID: 999, Name: "SENTINEL"}
		dest := []User{sentinel}
		err := db.AllByIndex(ctx, "DateOfBirth", &dest, func(opts *index.Options) {
			cancel()
		})
		require.True(t, errors.Is(err, context.Canceled), "expected Canceled, got %v", err)
		require.Equal(t, []User{sentinel}, dest,
			"destination must be preserved after option cancellation")
	})

	t.Run("Range", func(t *testing.T) {
		db, cleanup := createDB(t)
		defer cleanup()

		err := db.Save(context.Background(), &User{
			ID: 1, Name: "rng-opt", Slug: "ro1",
		})
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		sentinel := User{ID: 999, Name: "SENTINEL", Slug: "SENTINEL"}
		dest := []User{sentinel}
		err = db.Range(ctx, "Slug", "A", "Z", &dest, func(opts *index.Options) {
			cancel()
		})
		require.True(t, errors.Is(err, context.Canceled), "expected Canceled, got %v", err)
		require.Equal(t, []User{sentinel}, dest,
			"destination must be preserved after option cancellation")
	})

	t.Run("Prefix", func(t *testing.T) {
		db, cleanup := createDB(t)
		defer cleanup()

		err := db.Save(context.Background(), &User{
			ID: 1, Name: "popt", Slug: "popt",
		})
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		sentinel := User{ID: 999, Name: "SENTINEL", Slug: "SENTINEL"}
		dest := []User{sentinel}
		err = db.Prefix(ctx, "Name", "p", &dest, func(opts *index.Options) {
			cancel()
		})
		require.True(t, errors.Is(err, context.Canceled), "expected Canceled, got %v", err)
		require.Equal(t, []User{sentinel}, dest,
			"destination must be preserved after option cancellation")
	})

	t.Run("All", func(t *testing.T) {
		db, cleanup := createDB(t)
		defer cleanup()

		for i := 1; i <= 3; i++ {
			err := db.Save(context.Background(), &User{
				ID: i, Name: "all-opt", Slug: fmt.Sprintf("all-opt-%d", i),
			})
			require.NoError(t, err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		sentinel := User{ID: 999, Name: "SENTINEL"}
		dest := []User{sentinel}
		err := db.All(ctx, &dest, func(opts *index.Options) {
			cancel()
		})
		require.True(t, errors.Is(err, context.Canceled), "expected Canceled, got %v", err)
		require.Equal(t, []User{sentinel}, dest,
			"destination must be preserved after option cancellation")
	})
}

func TestOne_IndexedRuntimeStructStillWorks(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()
	dynamicType := reflect.StructOf([]reflect.StructField{
		{Name: "ID", Type: reflect.TypeOf(0), Tag: reflect.StructTag(`rainstorm:"id"`)},
		{Name: "Name", Type: reflect.TypeOf(""), Tag: reflect.StructTag(`rainstorm:"index"`)},
	})

	n := db.From("runtime_one_indexed")
	record := reflect.New(dynamicType)
	record.Elem().FieldByName("ID").SetInt(100)
	record.Elem().FieldByName("Name").SetString("runtime-test")
	require.NoError(t, n.Save(ctx, record.Interface()))

	destination := reflect.New(dynamicType)
	require.NoError(t, n.One(ctx, "Name", "runtime-test", destination.Interface()))
	require.Equal(t, int64(100), destination.Elem().FieldByName("ID").Int())
	require.Equal(t, "runtime-test", destination.Elem().FieldByName("Name").String())
}
