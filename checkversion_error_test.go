package rainstorm

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

// errTestDecode is a stable sentinel returned by the test codec's Unmarshal.
var errTestDecode = errors.New("test-codec: deterministic decode error")

// testDecodeCodec is a private codec used only to prove that checkVersion
// preserves the underlying codec/decode error alongside ErrDifferentCodec.
type testDecodeCodec struct{}

func (testDecodeCodec) Name() string                          { return "test-decode-codec" }
func (testDecodeCodec) Marshal(v interface{}) ([]byte, error) { return []byte("x"), nil }
func (testDecodeCodec) Unmarshal(b []byte, v interface{}) error {
	return errTestDecode
}

// TestCheckVersion_EmptyVersionIsInitialized preserves the legacy behavior of
// writing the current version when an existing version value decodes as empty.
func TestCheckVersion_EmptyVersionIsInitialized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty-version.db")
	ctx := context.Background()

	db, err := Open(ctx, path)
	require.NoError(t, err)
	require.NoError(t, db.Set(ctx, dbinfo, "version", ""))
	require.NoError(t, db.Close())

	db, err = Open(ctx, path)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()

	var version string
	require.NoError(t, db.Get(ctx, dbinfo, "version", &version))
	require.Equal(t, Version, version)
}

// TestCheckVersion_ErrorChainPreservation proves that when Open encounters a
// codec mismatch during checkVersion, the returned error matches both
// ErrDifferentCodec and the underlying codec/decode error.
func TestCheckVersion_ErrorChainPreservation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "seed.db")

	// Create a database with the default JSON codec. This writes
	// __rainstorm_db/version using the JSON codec.
	seedDB, err := Open(context.Background(), path)
	require.NoError(t, err, "create seed DB")
	require.NoError(t, seedDB.Close(), "close seed DB")

	// Open with an incompatible codec that deliberately fails Unmarshal.
	db, err := Open(context.Background(), path, Codec(testDecodeCodec{}))
	require.Error(t, err)

	// Must match ErrDifferentCodec.
	require.ErrorIs(t, err, ErrDifferentCodec,
		"checkVersion must classify codec mismatch as ErrDifferentCodec")

	// The underlying decode error must remain discoverable.
	require.ErrorIs(t, err, errTestDecode,
		"checkVersion must preserve the underlying decode error in the chain")

	// No partial DB is returned.
	assert.Nil(t, db, "returned DB must be nil on initialization failure")

	// The owned database file must be closed and its bbolt lock released after
	// failed initialization. Opening the file descriptor alone would not prove
	// this because bbolt locking is advisory.
	verifyBolt, err := bolt.Open(path, 0600, &bolt.Options{Timeout: time.Second})
	require.NoError(t, err, "seed database lock must be released by failed Open")
	require.NoError(t, verifyBolt.Close())

	// Reopening with the correct codec must succeed and data must be intact.
	db2, err := Open(context.Background(), path)
	require.NoError(t, err, "reopen with correct codec")
	require.NoError(t, db2.Close())
}
