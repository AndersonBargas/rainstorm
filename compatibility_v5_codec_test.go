package rainstorm

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AndersonBargas/rainstorm/v6/codec"
	"github.com/AndersonBargas/rainstorm/v6/codec/aes"
	gobcodec "github.com/AndersonBargas/rainstorm/v6/codec/gob"
	"github.com/AndersonBargas/rainstorm/v6/codec/json"
	msgpackcodec "github.com/AndersonBargas/rainstorm/v6/codec/msgpack"
	serealcodec "github.com/AndersonBargas/rainstorm/v6/codec/sereal"
)

// CodecCompatibilityRecord mirrors the schema used by the v5.3.0 codec
// fixture generator (package main).
type CodecCompatibilityRecord struct {
	ID       uint64 `rainstorm:"id,increment"`
	Key      string `rainstorm:"unique"`
	Category string `rainstorm:"index"`
	Name     string
	Revision int
}

// -- helpers for codec fixtures --------------------------------------------

// codecsDir returns the package-relative path to the codec fixtures directory.
func codecsDir(tb testing.TB) string {
	tb.Helper()
	return filepath.Join("testdata", "compatibility", "v5.3.0", "codecs")
}

// codecFixturePath returns the path to a codec fixture file.
func codecFixturePath(tb testing.TB, filename string) string {
	tb.Helper()
	return filepath.Join(codecsDir(tb), filename)
}

// copyCodecFixture copies a codec fixture file to a temp directory.
func copyCodecFixture(tb testing.TB, filename string) string {
	tb.Helper()
	src := codecFixturePath(tb, filename)
	data, err := os.ReadFile(src)
	require.NoError(tb, err, "read codec fixture %s", filename)

	dst := filepath.Join(tb.TempDir(), filename)
	err = os.WriteFile(dst, data, 0600)
	require.NoError(tb, err, "write copy to %s", dst)
	return dst
}

// openCodecFixture opens a copied codec fixture with v6 using the given codec.
func openCodecFixture(tb testing.TB, path string, c codec.MarshalUnmarshaler) *DB {
	tb.Helper()
	db, err := Open(context.Background(), path, Codec(c))
	require.NoError(tb, err, "open codec fixture %s", path)
	return db
}

// -- aes test key (same as generator) --------------------------------------

// testAESKeyB64 is the base64-encoded AES-128 test key used by the generator.
// It is NOT a production secret.
const testAESKeyB64 = "xkBTXc1wn0C/aL31u9SA7g=="

// -- codec table type ------------------------------------------------------

type codecTestEntry struct {
	name         string
	filename     string
	codec        codec.MarshalUnmarshaler
	expectedKV   string
	incompatible codec.MarshalUnmarshaler
}

// makeCodecTable returns the test table for non-AES codecs.
func makeCodecTable() []codecTestEntry {
	return []codecTestEntry{
		{
			name:         "gob",
			filename:     "gob.db",
			codec:        gobcodec.Codec,
			expectedKV:   "gob-fixture",
			incompatible: msgpackcodec.Codec,
		},
		{
			name:         "msgpack",
			filename:     "msgpack.db",
			codec:        msgpackcodec.Codec,
			expectedKV:   "msgpack-fixture",
			incompatible: gobcodec.Codec,
		},
		{
			name:         "sereal",
			filename:     "sereal.db",
			codec:        serealcodec.Codec,
			expectedKV:   "sereal-fixture",
			incompatible: gobcodec.Codec,
		},
	}
}

// makeAESEntry creates the AES test entry with the correct key.
func makeAESEntry(tb testing.TB) codecTestEntry {
	tb.Helper()
	key, err := base64.StdEncoding.DecodeString(testAESKeyB64)
	require.NoError(tb, err, "decode AES test key")
	aesCodec, err := aes.NewAES(json.Codec, key)
	require.NoError(tb, err, "create AES codec")
	return codecTestEntry{
		name:         "aes",
		filename:     "aes.db",
		codec:        aesCodec,
		expectedKV:   "aes-fixture",
		incompatible: gobcodec.Codec,
	}
}

// ==========================================================================
// A. Codec v5 -> v6 baseline read
// ==========================================================================

func TestCompatibility_CodecBaselineRead(t *testing.T) {
	for _, entry := range makeCodecTable() {
		t.Run(entry.name, func(t *testing.T) {
			path := copyCodecFixture(t, entry.filename)
			db := openCodecFixture(t, path, entry.codec)
			defer assertClose(t, db)

			ctx := context.Background()

			var all []CodecCompatibilityRecord
			err := db.All(ctx, &all)
			require.NoError(t, err, "All for %s", entry.name)
			require.Len(t, all, 3, "%s: expected 3 records", entry.name)

			byKey := make(map[string]CodecCompatibilityRecord, 3)
			for _, r := range all {
				byKey[r.Key] = r
			}

			require.Contains(t, byKey, "alpha", "%s: alpha", entry.name)
			assert.Equal(t, uint64(1), byKey["alpha"].ID, "%s: alpha ID", entry.name)
			assert.Equal(t, "shared", byKey["alpha"].Category, "%s: alpha Category", entry.name)
			assert.Equal(t, "Alpha", byKey["alpha"].Name, "%s: alpha Name", entry.name)
			assert.Equal(t, 1, byKey["alpha"].Revision, "%s: alpha Revision", entry.name)

			require.Contains(t, byKey, "beta", "%s: beta", entry.name)
			assert.Equal(t, uint64(2), byKey["beta"].ID, "%s: beta ID", entry.name)
			assert.Equal(t, "shared", byKey["beta"].Category, "%s: beta Category", entry.name)
			assert.Equal(t, "Beta", byKey["beta"].Name, "%s: beta Name", entry.name)
			assert.Equal(t, 2, byKey["beta"].Revision, "%s: beta Revision", entry.name)

			require.Contains(t, byKey, "gamma", "%s: gamma", entry.name)
			assert.Equal(t, uint64(3), byKey["gamma"].ID, "%s: gamma ID", entry.name)
			assert.Equal(t, "other", byKey["gamma"].Category, "%s: gamma Category", entry.name)
			assert.Equal(t, "Gamma", byKey["gamma"].Name, "%s: gamma Name", entry.name)
			assert.Equal(t, 3, byKey["gamma"].Revision, "%s: gamma Revision", entry.name)
		})
	}

	t.Run("aes", func(t *testing.T) {
		entry := makeAESEntry(t)
		path := copyCodecFixture(t, entry.filename)
		db := openCodecFixture(t, path, entry.codec)
		defer assertClose(t, db)

		ctx := context.Background()

		var all []CodecCompatibilityRecord
		err := db.All(ctx, &all)
		require.NoError(t, err, "All for aes")
		require.Len(t, all, 3, "aes: expected 3 records")

		byKey := make(map[string]CodecCompatibilityRecord, 3)
		for _, r := range all {
			byKey[r.Key] = r
		}

		require.Contains(t, byKey, "alpha")
		assert.Equal(t, uint64(1), byKey["alpha"].ID, "aes: alpha ID")
		assert.Equal(t, "Alpha", byKey["alpha"].Name, "aes: alpha Name")
		require.Contains(t, byKey, "beta")
		assert.Equal(t, uint64(2), byKey["beta"].ID, "aes: beta ID")
		assert.Equal(t, "Beta", byKey["beta"].Name, "aes: beta Name")
		require.Contains(t, byKey, "gamma")
		assert.Equal(t, uint64(3), byKey["gamma"].ID, "aes: gamma ID")
		assert.Equal(t, "Gamma", byKey["gamma"].Name, "aes: gamma Name")
	})
}

// ==========================================================================
// B. Codec ordinary index verification
// ==========================================================================

func TestCompatibility_CodecOrdinaryIndex(t *testing.T) {
	for _, entry := range makeCodecTable() {
		t.Run(entry.name, func(t *testing.T) {
			path := copyCodecFixture(t, entry.filename)
			db := openCodecFixture(t, path, entry.codec)
			defer assertClose(t, db)

			ctx := context.Background()

			// shared returns alpha and beta in exact order.
			var shared []CodecCompatibilityRecord
			err := db.Find(ctx, "Category", "shared", &shared)
			require.NoError(t, err, "%s: find shared", entry.name)
			require.Len(t, shared, 2, "%s: shared count", entry.name)
			assert.Equal(t, uint64(1), shared[0].ID, "%s: shared[0] ID", entry.name)
			assert.Equal(t, "alpha", shared[0].Key, "%s: shared[0] Key", entry.name)
			assert.Equal(t, uint64(2), shared[1].ID, "%s: shared[1] ID", entry.name)
			assert.Equal(t, "beta", shared[1].Key, "%s: shared[1] Key", entry.name)

			// other returns gamma.
			var other []CodecCompatibilityRecord
			err = db.Find(ctx, "Category", "other", &other)
			require.NoError(t, err, "%s: find other", entry.name)
			require.Len(t, other, 1, "%s: other count", entry.name)
			assert.Equal(t, uint64(3), other[0].ID, "%s: other[0] ID", entry.name)
			assert.Equal(t, "gamma", other[0].Key, "%s: other[0] Key", entry.name)

			// Missing index value returns ErrNotFound.
			var missing []CodecCompatibilityRecord
			err = db.Find(ctx, "Category", "void", &missing)
			require.ErrorIs(t, err, ErrNotFound, "%s: missing category", entry.name)
			require.Empty(t, missing)
		})
	}

	t.Run("aes", func(t *testing.T) {
		entry := makeAESEntry(t)
		path := copyCodecFixture(t, entry.filename)
		db := openCodecFixture(t, path, entry.codec)
		defer assertClose(t, db)

		ctx := context.Background()

		var shared []CodecCompatibilityRecord
		err := db.Find(ctx, "Category", "shared", &shared)
		require.NoError(t, err, "aes: find shared")
		require.Len(t, shared, 2, "aes: shared count")
		assert.Equal(t, "alpha", shared[0].Key)
		assert.Equal(t, "beta", shared[1].Key)

		var other []CodecCompatibilityRecord
		err = db.Find(ctx, "Category", "other", &other)
		require.NoError(t, err, "aes: find other")
		require.Len(t, other, 1)
		assert.Equal(t, "gamma", other[0].Key)
	})
}

// ==========================================================================
// C. Codec unique index verification
// ==========================================================================

func TestCompatibility_CodecUniqueIndex(t *testing.T) {
	for _, entry := range makeCodecTable() {
		t.Run(entry.name, func(t *testing.T) {
			path := copyCodecFixture(t, entry.filename)
			db := openCodecFixture(t, path, entry.codec)
			defer assertClose(t, db)

			ctx := context.Background()

			var alpha CodecCompatibilityRecord
			err := db.One(ctx, "Key", "alpha", &alpha)
			require.NoError(t, err, "%s: One alpha", entry.name)
			assert.Equal(t, uint64(1), alpha.ID)

			var beta CodecCompatibilityRecord
			err = db.One(ctx, "Key", "beta", &beta)
			require.NoError(t, err, "%s: One beta", entry.name)
			assert.Equal(t, uint64(2), beta.ID)

			var gamma CodecCompatibilityRecord
			err = db.One(ctx, "Key", "gamma", &gamma)
			require.NoError(t, err, "%s: One gamma", entry.name)
			assert.Equal(t, uint64(3), gamma.ID)

			// Duplicate unique key must fail.
			dup := CodecCompatibilityRecord{
				Key:      "alpha",
				Category: "dup",
				Name:     "Duplicate",
				Revision: 99,
			}
			err = db.Save(ctx, &dup)
			require.ErrorIs(t, err, ErrAlreadyExists, "%s: duplicate key", entry.name)
		})
	}

	t.Run("aes", func(t *testing.T) {
		entry := makeAESEntry(t)
		path := copyCodecFixture(t, entry.filename)
		db := openCodecFixture(t, path, entry.codec)
		defer assertClose(t, db)

		ctx := context.Background()

		var alpha CodecCompatibilityRecord
		err := db.One(ctx, "Key", "alpha", &alpha)
		require.NoError(t, err)
		assert.Equal(t, uint64(1), alpha.ID)

		var gamma CodecCompatibilityRecord
		err = db.One(ctx, "Key", "gamma", &gamma)
		require.NoError(t, err)
		assert.Equal(t, uint64(3), gamma.ID)

		dup := CodecCompatibilityRecord{
			Key:      "alpha",
			Category: "dup",
			Name:     "Duplicate",
			Revision: 99,
		}
		err = db.Save(ctx, &dup)
		require.ErrorIs(t, err, ErrAlreadyExists, "aes: duplicate key")
	})
}

// ==========================================================================
// D. Codec KV verification
// ==========================================================================

func TestCompatibility_CodecKV(t *testing.T) {
	for _, entry := range makeCodecTable() {
		t.Run(entry.name, func(t *testing.T) {
			path := copyCodecFixture(t, entry.filename)
			db := openCodecFixture(t, path, entry.codec)
			defer assertClose(t, db)

			ctx := context.Background()

			var name string
			err := db.Get(ctx, "settings", "name", &name)
			require.NoError(t, err, "%s: get settings/name", entry.name)
			assert.Equal(t, entry.expectedKV, name, "%s: settings/name", entry.name)

			var rev int
			err = db.Get(ctx, "settings", "revision", &rev)
			require.NoError(t, err, "%s: get settings/revision", entry.name)
			assert.Equal(t, 1, rev, "%s: settings/revision", entry.name)
		})
	}

	t.Run("aes", func(t *testing.T) {
		entry := makeAESEntry(t)
		path := copyCodecFixture(t, entry.filename)
		db := openCodecFixture(t, path, entry.codec)
		defer assertClose(t, db)

		ctx := context.Background()

		var name string
		err := db.Get(ctx, "settings", "name", &name)
		require.NoError(t, err)
		assert.Equal(t, "aes-fixture", name)

		var rev int
		err = db.Get(ctx, "settings", "revision", &rev)
		require.NoError(t, err)
		assert.Equal(t, 1, rev)
	})
}

// ==========================================================================
// E. Codec v6 mutation (save, update, delete) over v5 data
// ==========================================================================

func TestCompatibility_CodecV6Mutation(t *testing.T) {
	for _, entry := range makeCodecTable() {
		t.Run(entry.name, func(t *testing.T) {
			path := copyCodecFixture(t, entry.filename)
			db := openCodecFixture(t, path, entry.codec)
			defer assertClose(t, db)

			ctx := context.Background()

			// 1. Save a new record: Key delta, Category shared, next ID = 4.
			delta := CodecCompatibilityRecord{
				Key:      "delta",
				Category: "shared",
				Name:     "Delta",
				Revision: 4,
			}
			err := db.Save(ctx, &delta)
			require.NoError(t, err, "%s: save delta", entry.name)
			assert.Equal(t, uint64(4), delta.ID, "%s: delta ID", entry.name)

			// Verify delta is readable.
			var fetched CodecCompatibilityRecord
			err = db.One(ctx, "Key", "delta", &fetched)
			require.NoError(t, err, "%s: read delta", entry.name)
			assert.Equal(t, "Delta", fetched.Name)

			// 2. Update alpha: Category shared -> migrated, Name/Revision changed.
			var alpha CodecCompatibilityRecord
			err = db.One(ctx, "Key", "alpha", &alpha)
			require.NoError(t, err, "%s: read alpha", entry.name)
			assert.Equal(t, "shared", alpha.Category)

			alpha.Category = "migrated"
			alpha.Name = "Alpha Updated"
			alpha.Revision = 100
			err = db.Update(ctx, &alpha)
			require.NoError(t, err, "%s: update alpha", entry.name)

			// After moving alpha: shared must contain exactly beta (ID 2) and delta (ID 4) in order.
			var sharedAfter []CodecCompatibilityRecord
			err = db.Find(ctx, "Category", "shared", &sharedAfter)
			require.NoError(t, err, "%s: shared after alpha move", entry.name)
			require.Len(t, sharedAfter, 2, "%s: shared must have beta+delta", entry.name)
			assert.Equal(t, uint64(2), sharedAfter[0].ID, "%s: shared[0]=beta", entry.name)
			assert.Equal(t, "beta", sharedAfter[0].Key, "%s: shared[0] key", entry.name)
			assert.Equal(t, uint64(4), sharedAfter[1].ID, "%s: shared[1]=delta", entry.name)
			assert.Equal(t, "delta", sharedAfter[1].Key, "%s: shared[1] key", entry.name)

			// New index (migrated) contains alpha.
			var migrated []CodecCompatibilityRecord
			err = db.Find(ctx, "Category", "migrated", &migrated)
			require.NoError(t, err, "%s: find migrated", entry.name)
			require.Len(t, migrated, 1)
			assert.Equal(t, "alpha", migrated[0].Key)
			assert.Equal(t, "Alpha Updated", migrated[0].Name)
			assert.Equal(t, 100, migrated[0].Revision)

			// Unique Key lookup still resolves.
			err = db.One(ctx, "Key", "alpha", &alpha)
			require.NoError(t, err)
			assert.Equal(t, "migrated", alpha.Category)

			// 3. Delete beta.
			var beta CodecCompatibilityRecord
			err = db.One(ctx, "Key", "beta", &beta)
			require.NoError(t, err, "%s: read beta", entry.name)
			assert.Equal(t, uint64(2), beta.ID)

			err = db.DeleteStruct(ctx, &beta)
			require.NoError(t, err, "%s: delete beta", entry.name)

			// After deleting beta: shared must contain exactly delta.
			var sharedClean []CodecCompatibilityRecord
			err = db.Find(ctx, "Category", "shared", &sharedClean)
			require.NoError(t, err, "%s: shared after beta delete", entry.name)
			require.Len(t, sharedClean, 1, "%s: shared must have only delta", entry.name)
			assert.Equal(t, "delta", sharedClean[0].Key)

			// Reuse Key "beta" in a replacement (next ID = 5).
			replacement := CodecCompatibilityRecord{
				Key:      "beta",
				Category: "replacement",
				Name:     "Replacement Beta",
				Revision: 10,
			}
			err = db.Save(ctx, &replacement)
			require.NoError(t, err, "%s: save replacement", entry.name)
			assert.Equal(t, uint64(5), replacement.ID, "%s: replacement ID", entry.name)

			// Verify replacement is readable via Key.
			var repl CodecCompatibilityRecord
			err = db.One(ctx, "Key", "beta", &repl)
			require.NoError(t, err, "%s: unique lookup replacement", entry.name)
			assert.Equal(t, uint64(5), repl.ID, "%s: unique resolves ID 5", entry.name)
			assert.Equal(t, "Replacement Beta", repl.Name)

			// Replacement index contains exactly the replacement.
			var replIdx []CodecCompatibilityRecord
			err = db.Find(ctx, "Category", "replacement", &replIdx)
			require.NoError(t, err, "%s: replacement index", entry.name)
			require.Len(t, replIdx, 1)
			assert.Equal(t, "beta", replIdx[0].Key)
			assert.Equal(t, uint64(5), replIdx[0].ID)

			// gamma remains unchanged.
			var gamma CodecCompatibilityRecord
			err = db.One(ctx, "Key", "gamma", &gamma)
			require.NoError(t, err)
			assert.Equal(t, uint64(3), gamma.ID)
			assert.Equal(t, "other", gamma.Category)
			assert.Equal(t, "Gamma", gamma.Name)
		})
	}

	t.Run("aes", func(t *testing.T) {
		entry := makeAESEntry(t)
		path := copyCodecFixture(t, entry.filename)
		db := openCodecFixture(t, path, entry.codec)
		defer assertClose(t, db)

		ctx := context.Background()

		// Save delta (ID 4).
		delta := CodecCompatibilityRecord{
			Key:      "delta",
			Category: "shared",
			Name:     "Delta",
			Revision: 4,
		}
		err := db.Save(ctx, &delta)
		require.NoError(t, err, "aes: save delta")
		assert.Equal(t, uint64(4), delta.ID)

		// Update alpha: shared -> migrated.
		var alpha CodecCompatibilityRecord
		err = db.One(ctx, "Key", "alpha", &alpha)
		require.NoError(t, err)
		alpha.Category = "migrated"
		alpha.Name = "Alpha Updated"
		alpha.Revision = 100
		err = db.Update(ctx, &alpha)
		require.NoError(t, err)

		// After move: shared must contain exactly beta and delta in order.
		var sharedAfter []CodecCompatibilityRecord
		err = db.Find(ctx, "Category", "shared", &sharedAfter)
		require.NoError(t, err, "aes: shared after alpha move")
		require.Len(t, sharedAfter, 2, "aes: shared must have beta+delta")
		assert.Equal(t, "beta", sharedAfter[0].Key, "aes: shared[0]=beta")
		assert.Equal(t, "delta", sharedAfter[1].Key, "aes: shared[1]=delta")

		var migrated []CodecCompatibilityRecord
		err = db.Find(ctx, "Category", "migrated", &migrated)
		require.NoError(t, err)
		require.Len(t, migrated, 1)
		assert.Equal(t, "alpha", migrated[0].Key)

		// Delete beta.
		var beta CodecCompatibilityRecord
		err = db.One(ctx, "Key", "beta", &beta)
		require.NoError(t, err)
		err = db.DeleteStruct(ctx, &beta)
		require.NoError(t, err)

		// After delete: shared must contain exactly delta.
		var sharedClean []CodecCompatibilityRecord
		err = db.Find(ctx, "Category", "shared", &sharedClean)
		require.NoError(t, err, "aes: shared after beta delete")
		require.Len(t, sharedClean, 1, "aes: shared must have only delta")
		assert.Equal(t, "delta", sharedClean[0].Key)

		// Replacement (ID 5).
		replacement := CodecCompatibilityRecord{
			Key:      "beta",
			Category: "replacement",
			Name:     "Replacement Beta",
			Revision: 10,
		}
		err = db.Save(ctx, &replacement)
		require.NoError(t, err)
		assert.Equal(t, uint64(5), replacement.ID, "aes: replacement ID")

		// Unique lookup resolves ID 5.
		var repl CodecCompatibilityRecord
		err = db.One(ctx, "Key", "beta", &repl)
		require.NoError(t, err, "aes: unique lookup replacement")
		assert.Equal(t, uint64(5), repl.ID, "aes: unique resolves ID 5")

		// Replacement index contains exactly the replacement.
		var replIdx []CodecCompatibilityRecord
		err = db.Find(ctx, "Category", "replacement", &replIdx)
		require.NoError(t, err, "aes: replacement index")
		require.Len(t, replIdx, 1)
		assert.Equal(t, "beta", replIdx[0].Key)
		assert.Equal(t, uint64(5), replIdx[0].ID)
	})
}

// ==========================================================================
// F. Codec close and reopen (metadata/codec persistence)
// ==========================================================================

func TestCompatibility_CodecReopen(t *testing.T) {
	for _, entry := range makeCodecTable() {
		t.Run(entry.name, func(t *testing.T) {
			path := copyCodecFixture(t, entry.filename)
			ctx := context.Background()

			var replacementID uint64
			func() {
				db := openCodecFixture(t, path, entry.codec)
				defer assertClose(t, db)

				// Save delta (ID 4).
				delta := CodecCompatibilityRecord{
					Key: "delta", Category: "shared", Name: "Delta", Revision: 4,
				}
				err := db.Save(ctx, &delta)
				require.NoError(t, err, "%s: reopen save delta", entry.name)

				// Delete beta.
				var beta CodecCompatibilityRecord
				err = db.One(ctx, "Key", "beta", &beta)
				require.NoError(t, err)
				err = db.DeleteStruct(ctx, &beta)
				require.NoError(t, err)

				// Save replacement reusing Key "beta" (ID 5).
				replacement := CodecCompatibilityRecord{
					Key: "beta", Category: "replacement", Name: "Replaced", Revision: 9,
				}
				err = db.Save(ctx, &replacement)
				require.NoError(t, err)
				replacementID = replacement.ID
			}()

			// Reopen with the same codec.
			db2, err := Open(context.Background(), path, Codec(entry.codec))
			require.NoError(t, err, "%s: reopen", entry.name)
			defer assertClose(t, db2)

			// Key "beta" resolves to replacement ID 5.
			var repl CodecCompatibilityRecord
			err = db2.One(ctx, "Key", "beta", &repl)
			require.NoError(t, err, "%s: unique Key beta resolves replacement", entry.name)
			assert.Equal(t, replacementID, repl.ID)
			assert.Equal(t, "Replaced", repl.Name)

			// Primary ID 2 returns ErrNotFound (old beta is gone).
			err = db2.One(ctx, "ID", uint64(2), &CodecCompatibilityRecord{})
			require.ErrorIs(t, err, ErrNotFound, "%s: old beta ID 2 gone", entry.name)

			// Replacement category index contains ID 5.
			var replIdx []CodecCompatibilityRecord
			err = db2.Find(ctx, "Category", "replacement", &replIdx)
			require.NoError(t, err, "%s: replacement index", entry.name)
			require.Len(t, replIdx, 1)
			assert.Equal(t, "beta", replIdx[0].Key)
			assert.Equal(t, replacementID, replIdx[0].ID)

			// Shared index contains surviving original alpha (ID 1) and new delta (ID 4).
			var shared []CodecCompatibilityRecord
			err = db2.Find(ctx, "Category", "shared", &shared)
			require.NoError(t, err, "%s: shared after reopen", entry.name)
			require.Len(t, shared, 2, "%s: shared has alpha and delta", entry.name)
			assert.Equal(t, "alpha", shared[0].Key)
			assert.Equal(t, uint64(1), shared[0].ID)
			assert.Equal(t, "delta", shared[1].Key)
			assert.Equal(t, uint64(4), shared[1].ID)

			// Gamma unchanged.
			var gamma CodecCompatibilityRecord
			err = db2.One(ctx, "Key", "gamma", &gamma)
			require.NoError(t, err, "%s: reopen read gamma", entry.name)
			assert.Equal(t, uint64(3), gamma.ID)

			// Delta persists.
			var delta CodecCompatibilityRecord
			err = db2.One(ctx, "Key", "delta", &delta)
			require.NoError(t, err, "%s: reopen read delta", entry.name)
			assert.Equal(t, uint64(4), delta.ID)
			assert.Equal(t, "Delta", delta.Name)
		})
	}

	t.Run("aes", func(t *testing.T) {
		entry := makeAESEntry(t)
		path := copyCodecFixture(t, entry.filename)
		ctx := context.Background()

		func() {
			db := openCodecFixture(t, path, entry.codec)
			defer assertClose(t, db)

			newRec := CodecCompatibilityRecord{
				Key: "delta", Category: "shared", Name: "Delta", Revision: 4,
			}
			err := db.Save(ctx, &newRec)
			require.NoError(t, err, "aes: reopen save delta")
			assert.Equal(t, uint64(4), newRec.ID)
		}()

		db2, err := Open(context.Background(), path, Codec(entry.codec))
		require.NoError(t, err, "aes: reopen")
		defer assertClose(t, db2)

		var delta CodecCompatibilityRecord
		err = db2.One(ctx, "Key", "delta", &delta)
		require.NoError(t, err, "aes: reopen read delta")
		assert.Equal(t, uint64(4), delta.ID)
	})
}

// ==========================================================================
// G. Incompatible codec detection
// ==========================================================================

func TestCompatibility_CodecIncompatible(t *testing.T) {
	for _, entry := range makeCodecTable() {
		t.Run(entry.name, func(t *testing.T) {
			path := copyCodecFixture(t, entry.filename)

			// Attempt to open with a known incompatible codec.
			db, err := Open(context.Background(), path, Codec(entry.incompatible))
			require.ErrorIs(t, err, ErrDifferentCodec,
				"%s: expected ErrDifferentCodec when opening with %s",
				entry.name, entry.incompatible.Name())
			require.Nil(t, db, "%s: db must be nil on failed open", entry.name)

			// Reopening with the correct codec still succeeds.
			db2, err2 := Open(context.Background(), path, Codec(entry.codec))
			require.NoError(t, err2, "%s: reopen with correct codec after incompatible attempt", entry.name)
			defer assertClose(t, db2)

			// Data remains exact.
			var all []CodecCompatibilityRecord
			err = db2.All(context.Background(), &all)
			require.NoError(t, err, "%s: read after incompatible attempt", entry.name)
			require.Len(t, all, 3)

			byKey := make(map[string]CodecCompatibilityRecord)
			for _, r := range all {
				byKey[r.Key] = r
			}
			require.Contains(t, byKey, "alpha")
			require.Contains(t, byKey, "beta")
			require.Contains(t, byKey, "gamma")
			assert.Equal(t, uint64(1), byKey["alpha"].ID)
			assert.Equal(t, uint64(2), byKey["beta"].ID)
			assert.Equal(t, uint64(3), byKey["gamma"].ID)
		})
	}

	t.Run("aes", func(t *testing.T) {
		entry := makeAESEntry(t)
		path := copyCodecFixture(t, entry.filename)

		// Opening AES fixture with gob codec must return ErrDifferentCodec.
		db, err := Open(context.Background(), path, Codec(gobcodec.Codec))
		require.ErrorIs(t, err, ErrDifferentCodec, "aes: open with gob must be ErrDifferentCodec")
		require.Nil(t, db, "aes: db must be nil on failed open")

		// Correct codec still works.
		db2, err2 := Open(context.Background(), path, Codec(entry.codec))
		require.NoError(t, err2, "aes: reopen with correct codec")
		defer assertClose(t, db2)

		var all []CodecCompatibilityRecord
		err = db2.All(context.Background(), &all)
		require.NoError(t, err)
		require.Len(t, all, 3)

		byKey := make(map[string]CodecCompatibilityRecord)
		for _, r := range all {
			byKey[r.Key] = r
		}
		require.Contains(t, byKey, "alpha")
		require.Contains(t, byKey, "beta")
		require.Contains(t, byKey, "gamma")
		assert.Equal(t, uint64(1), byKey["alpha"].ID)
		assert.Equal(t, uint64(2), byKey["beta"].ID)
		assert.Equal(t, uint64(3), byKey["gamma"].ID)
	})
}

// ==========================================================================
// H. AES wrong-key safety
// ==========================================================================

func TestCompatibility_CodecAESWrongKey(t *testing.T) {
	path := copyCodecFixture(t, "aes.db")

	// Create a different key by flipping one bit of the correct key.
	correctKey, err := base64.StdEncoding.DecodeString(testAESKeyB64)
	require.NoError(t, err)
	wrongKey := make([]byte, len(correctKey))
	copy(wrongKey, correctKey)
	wrongKey[0] ^= 0x01

	wrongCodec, err := aes.NewAES(json.Codec, wrongKey)
	require.NoError(t, err, "create wrong AES codec")

	// Opening with the wrong key must fail during checkVersion. The codec name
	// still matches ("aes-json"), but the encrypted version cannot be decoded.
	db, openErr := Open(context.Background(), path, Codec(wrongCodec))
	require.Error(t, openErr, "aes: wrong key must fail Open")
	require.ErrorIs(t, openErr, ErrDifferentCodec,
		"aes: wrong key must return ErrDifferentCodec")
	require.Nil(t, db, "aes: db must be nil on wrong-key open")

	// Generic preservation of the underlying decode cause is covered by
	// TestCheckVersion_ErrorChainPreservation using a classifiable sentinel.
	// Fixture must remain uncorrupted: correct key still works.
	entry := makeAESEntry(t)
	db2, err := Open(context.Background(), path, Codec(entry.codec))
	require.NoError(t, err, "aes: correct key still works after wrong-key attempt")
	defer assertClose(t, db2)

	var all []CodecCompatibilityRecord
	err = db2.All(context.Background(), &all)
	require.NoError(t, err)
	require.Len(t, all, 3, "aes: fixture intact after wrong-key attempt")

	byKey := make(map[string]CodecCompatibilityRecord)
	for _, r := range all {
		byKey[r.Key] = r
	}
	require.Contains(t, byKey, "alpha")
	assert.Equal(t, uint64(1), byKey["alpha"].ID, "aes: alpha ID intact")
	assert.Equal(t, "Alpha", byKey["alpha"].Name, "aes: alpha Name intact")
	require.Contains(t, byKey, "beta")
	assert.Equal(t, uint64(2), byKey["beta"].ID, "aes: beta ID intact")
	assert.Equal(t, "Beta", byKey["beta"].Name, "aes: beta Name intact")
	require.Contains(t, byKey, "gamma")
	assert.Equal(t, uint64(3), byKey["gamma"].ID, "aes: gamma ID intact")
	assert.Equal(t, "Gamma", byKey["gamma"].Name, "aes: gamma Name intact")

	// Verify an index and a unique lookup still work.
	var shared []CodecCompatibilityRecord
	err = db2.Find(context.Background(), "Category", "shared", &shared)
	require.NoError(t, err)
	require.Len(t, shared, 2, "aes: shared index intact")
	assert.Equal(t, "alpha", shared[0].Key)
	assert.Equal(t, "beta", shared[1].Key)

	var gammaLookup CodecCompatibilityRecord
	err = db2.One(context.Background(), "Key", "gamma", &gammaLookup)
	require.NoError(t, err, "aes: unique lookup intact")
	assert.Equal(t, uint64(3), gammaLookup.ID)
}
