package rainstorm

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

// TestOwnedLifecycle verifies owned database lifecycle.
func TestOwnedLifecycle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "owned.db")

	db, err := Open(context.Background(), path)
	require.NoError(t, err)

	// 1. Owned Open sets internal ownership to owned.
	require.True(t, db.boltOwned, "owned database must be marked boltOwned")
	require.NotNil(t, db.bolt, "owned database must have non-nil bolt")

	// 17. NativeDB returns exact internally opened pointer for owned DB.
	require.Same(t, db.bolt, db.NativeDB(), "NativeDB must return same pointer as internal bolt")

	// 2. Owned DB.Close closes the underlying bbolt DB.
	err = db.Close()
	require.NoError(t, err)

	// 3. A second native operation after owned Close returns a bbolt closed-database error.
	// bbolt returns ErrDatabaseNotOpen for operations on a closed DB.
	viewErr := db.bolt.View(func(tx *bolt.Tx) error { return nil })
	require.ErrorIs(t, viewErr, bolt.ErrDatabaseNotOpen,
		"native View on closed owned DB must report bbolt's closed-database sentinel")
}

// TestOwnedPostOpenCancellation verifies that owned post-open cancellation cleans up.
func TestOwnedPostOpenCancellation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "owned_cancel.db")

	// 4. Owned post-open cancellation closes the database.
	ctx, cancel := context.WithCancel(context.Background())
	cancelAfterOpen := OpenOption(func(opts *Options) error {
		opts.postOpenHook = func(_ context.Context) {
			cancel()
		}
		return nil
	})

	db, err := Open(ctx, path, cancelAfterOpen)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, db)

	// Reopen to prove the file is not locked (cleanupOwned worked).
	db2, err := Open(context.Background(), path)
	require.NoError(t, err)
	require.NoError(t, db2.Close())
}

// TestOwnedInitFailureCleanup verifies owned init failure closes the database.
func TestOwnedInitFailureCleanup(t *testing.T) {
	dir := t.TempDir()

	// 5. Owned initialization failure closes the database where deterministically testable.
	//
	// Pre-seed the database with gob codec metadata, then open with default (json)
	// codec. checkVersion will call Set which triggers newMeta which detects
	// codec mismatch, returning ErrDifferentCodec.
	bDBPath := filepath.Join(dir, "prepared.db")
	bDB, err := bolt.Open(bDBPath, 0600, &bolt.Options{Timeout: 10 * time.Second})
	require.NoError(t, err)

	err = bDB.Update(func(tx *bolt.Tx) error {
		top, err := tx.CreateBucketIfNotExists([]byte(dbinfo))
		if err != nil {
			return err
		}
		_, err = top.CreateBucket([]byte(metadataBucket))
		if err != nil {
			return err
		}
		mb := top.Bucket([]byte(metadataBucket))
		// Write a codec name that will mismatch json's name.
		return mb.Put([]byte(metaCodec), []byte("not-json"))
	})
	require.NoError(t, err)
	// Close before handing to Rainstorm so it can reopen internally.
	require.NoError(t, bDB.Close())

	db, err := Open(context.Background(), bDBPath)
	require.ErrorIs(t, err, ErrDifferentCodec)
	require.Nil(t, db)

	// Prove the bolt DB was closed by cleanupOwned: reopen the same file
	// directly with bolt. If Rainstorm left the file locked, bolt.Open would fail.
	verifyDB, err := bolt.Open(bDBPath, 0600, &bolt.Options{Timeout: 10 * time.Second})
	require.NoError(t, err, "bolt.Open must succeed because Rainstorm cleanupOwned closed the DB")
	require.NoError(t, verifyDB.Close())
}

// TestBorrowedLifecycle verifies borrowed database lifecycle.
func TestBorrowedLifecycle(t *testing.T) {
	dir := t.TempDir()
	bDBPath := filepath.Join(dir, "borrowed.db")

	// Open a bolt.DB directly.
	bDB, err := bolt.Open(bDBPath, 0600, &bolt.Options{Timeout: 10 * time.Second})
	require.NoError(t, err)

	// 6. UseDB marks the database borrowed.
	db, err := Open(context.Background(), "", UseDB(bDB))
	require.NoError(t, err)

	require.False(t, db.boltOwned, "borrowed database must have boltOwned=false")

	// 18. NativeDB returns exact same pointer supplied through UseDB.
	require.Same(t, bDB, db.NativeDB(), "NativeDB must return same pointer as supplied bDB")

	// 7. Borrowed DB.Close returns nil.
	err = db.Close()
	require.NoError(t, err, "borrowed Close must return nil")

	// 8. Borrowed DB.Close does not close the supplied bbolt DB.
	// 9. Caller can perform a bbolt transaction after Rainstorm Close.
	err = bDB.View(func(tx *bolt.Tx) error { return nil })
	require.NoError(t, err, "bbolt DB must still be usable after borrowed Rainstorm Close")

	// 10. Caller can close the borrowed DB afterward.
	err = bDB.Close()
	require.NoError(t, err, "caller must be able to close borrowed DB after Rainstorm Close")
}

// TestBorrowedInitFailureDoesNotClose verifies borrowed DB is not closed on init failure.
func TestBorrowedInitFailureDoesNotClose(t *testing.T) {
	dir := t.TempDir()
	bDBPath := filepath.Join(dir, "borrowed_no_close.db")

	bDB, err := bolt.Open(bDBPath, 0600, &bolt.Options{Timeout: 10 * time.Second})
	require.NoError(t, err)

	// Pre-seed with conflicting codec to trigger init failure.
	err = bDB.Update(func(tx *bolt.Tx) error {
		top, err := tx.CreateBucketIfNotExists([]byte(dbinfo))
		if err != nil {
			return err
		}
		_, err = top.CreateBucket([]byte(metadataBucket))
		if err != nil {
			return err
		}
		mb := top.Bucket([]byte(metadataBucket))
		return mb.Put([]byte(metaCodec), []byte("not-json"))
	})
	require.NoError(t, err)

	// 11. Initialization failure does not close borrowed DB.
	db, err := Open(context.Background(), "", UseDB(bDB))
	require.ErrorIs(t, err, ErrDifferentCodec)
	require.Nil(t, db)

	// 12. Cancellation during borrowed Open/initialization does not close borrowed DB.
	// bDB must still be open.
	err = bDB.View(func(tx *bolt.Tx) error { return nil })
	require.NoError(t, err, "borrowed bbolt must still be usable after init failure")

	// Caller closes it.
	require.NoError(t, bDB.Close())
}

// TestBorrowedCancellationDoesNotClose verifies borrowed DB survives cancellation.
func TestBorrowedCancellationDoesNotClose(t *testing.T) {
	dir := t.TempDir()
	bDBPath := filepath.Join(dir, "borrowed_cancel.db")

	bDB, err := bolt.Open(bDBPath, 0600, &bolt.Options{Timeout: 10 * time.Second})
	require.NoError(t, err)

	// Cancel after UseDB has been applied but before initialization. Open's
	// post-option context check must reject the operation without closing the
	// now-associated borrowed database.
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancelDuringOptions := OpenOption(func(*Options) error {
		cancel()
		return nil
	})

	db, err := Open(cancelCtx, "", UseDB(bDB), cancelDuringOptions)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, db)

	// Both options ran and the borrowed DB must still be usable.
	require.ErrorIs(t, cancelCtx.Err(), context.Canceled)
	err = bDB.View(func(tx *bolt.Tx) error { return nil })
	require.NoError(t, err, "borrowed bbolt must still be usable after cancelled Open")

	require.NoError(t, bDB.Close())
}

// TestUseDBNilRejected verifies UseDB(nil) returns ErrNilParam.
func TestUseDBNilRejected(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "nil.db")

	// 13. UseDB(nil) returns ErrNilParam through Open and does not panic.
	ctx := context.Background()
	db, err := Open(ctx, file, UseDB(nil))
	require.ErrorIs(t, err, ErrNilParam)
	require.Nil(t, db)

	// No file should have been created (or locked).
	_, statErr := os.Stat(file)
	require.True(t, os.IsNotExist(statErr), "database file should not exist after nil UseDB")
}

// TestDBNilClose verifies nil receiver and nil bolt Close behavior.
func TestDBNilClose(t *testing.T) {
	// 14. (*DB)(nil).Close() returns ErrNilParam.
	var nilDB *DB
	err := nilDB.Close()
	require.ErrorIs(t, err, ErrNilParam)
	require.Nil(t, nilDB.NativeDB())

	// 15. DB with nil private database returns ErrNilParam from Close.
	db := &DB{} // bolt is nil
	err = db.Close()
	require.ErrorIs(t, err, ErrNilParam)
	require.Nil(t, db.NativeDB())
}

// TestNativeKVInteroperability verifies native writes are visible to Rainstorm and vice versa.
func TestNativeKVInteroperability(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "native_interop.db")

	db, err := Open(context.Background(), path)
	require.NoError(t, err)

	ctx := context.Background()

	// 19. Native writes are visible to Rainstorm when they preserve expected format.
	// 20. Rainstorm writes are visible through NativeDB.

	// Step 1: Rainstorm write via KV API.
	err = db.Set(ctx, "interop", "key1", "rainstorm_value")
	require.NoError(t, err)

	// Step 2: Verify via NativeDB (read back the raw bytes).
	var nativeVal []byte
	err = db.NativeDB().View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("interop"))
		if b == nil {
			return ErrNotFound
		}
		raw := b.Get([]byte("key1"))
		if raw == nil {
			return ErrNotFound
		}
		nativeVal = append([]byte(nil), raw...)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, `"rainstorm_value"`, string(nativeVal), "Rainstorm value must be visible through NativeDB")

	// Step 3: Native write via NativeDB (write raw JSON-encoded string).
	err = db.NativeDB().Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("interop"))
		if b == nil {
			return ErrNotFound
		}
		return b.Put([]byte("key2"), []byte(`"native_value"`))
	})
	require.NoError(t, err)

	// Step 4: Verify via Rainstorm.
	var got string
	err = db.Get(ctx, "interop", "key2", &got)
	require.NoError(t, err)
	require.Equal(t, "native_value", got, "native write must be visible through Rainstorm")

	require.NoError(t, db.Close())
}
