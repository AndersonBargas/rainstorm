package rainstorm

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"

	"github.com/AndersonBargas/rainstorm/v6/codec"
	gobcodec "github.com/AndersonBargas/rainstorm/v6/codec/gob"
	msgpackcodec "github.com/AndersonBargas/rainstorm/v6/codec/msgpack"
	serealcodec "github.com/AndersonBargas/rainstorm/v6/codec/sereal"
)

// TestCodecFixture_StructuralSanity inspects the checked-in codec fixtures
// through NativeDB/bbolt to prove:
// 1. The Rainstorm metadata codec marker for the record bucket is correct.
// 2. The raw stored value for the same logical record is non-empty.
// 3. The raw stored values are not all identical across codecs.
func TestCodecFixture_StructuralSanity(t *testing.T) {
	tests := []struct {
		filename      string
		expectedCodec string
		codec         codec.MarshalUnmarshaler
	}{
		{"gob.db", "gob", gobcodec.Codec},
		{"msgpack.db", "msgpack", msgpackcodec.Codec},
		{"sereal.db", "sereal", serealcodec.Codec},
	}

	recordBucket := []byte("CodecCompatibilityRecord")
	metaBucket := []byte("__rainstorm_metadata")
	codecKey := []byte("codec")

	var rawByCodec = make(map[string][]byte, len(tests)+1)

	for _, tc := range tests {
		t.Run(tc.filename, func(t *testing.T) {
			path := copyCodecFixture(t, tc.filename)
			db, err := Open(context.Background(), path, Codec(tc.codec))
			require.NoError(t, err, "open %s", tc.filename)
			defer assertClose(t, db)

			nativeDB := db.NativeDB()
			require.NotNil(t, nativeDB, "%s: NativeDB must not be nil", tc.filename)

			var codecName string
			err = nativeDB.View(func(tx *bolt.Tx) error {
				b := tx.Bucket(recordBucket)
				if b == nil {
					return fmt.Errorf("%s: record bucket does not exist", tc.filename)
				}

				mb := b.Bucket(metaBucket)
				if mb == nil {
					return fmt.Errorf("%s: metadata bucket does not exist", tc.filename)
				}

				raw := mb.Get(codecKey)
				if raw == nil {
					return fmt.Errorf("%s: codec key does not exist in metadata", tc.filename)
				}
				codecName = string(raw)
				return nil
			})
			require.NoError(t, err, "%s: View must succeed", tc.filename)

			assert.Equal(t, tc.expectedCodec, codecName,
				"%s: metadata codec marker", tc.filename)

			var rawValue []byte
			err = nativeDB.View(func(tx *bolt.Tx) error {
				b := tx.Bucket(recordBucket)
				if b == nil {
					return fmt.Errorf("%s: record bucket does not exist in second View", tc.filename)
				}

				// Rainstorm's toBytes for uint64 produces big-endian bytes.
				// ID 1 = 0x0000000000000001 in big-endian.
				idKey := []byte{0, 0, 0, 0, 0, 0, 0, 1}
				v := b.Get(idKey)
				if v == nil {
					return fmt.Errorf("%s: record with ID 1 does not exist", tc.filename)
				}
				rawValue = append([]byte(nil), v...)
				return nil
			})
			require.NoError(t, err, "%s: second View must succeed", tc.filename)

			assert.NotEmpty(t, rawValue, "%s: raw record bytes must not be empty", tc.filename)
			rawByCodec[tc.filename] = rawValue
		})
	}

	// AES — requires key-based codec construction.
	t.Run("aes.db", func(t *testing.T) {
		entry := makeAESEntry(t)
		path := copyCodecFixture(t, "aes.db")
		db, err := Open(context.Background(), path, Codec(entry.codec))
		require.NoError(t, err, "open aes.db")
		defer assertClose(t, db)

		nativeDB := db.NativeDB()
		var codecName string
		err = nativeDB.View(func(tx *bolt.Tx) error {
			b := tx.Bucket(recordBucket)
			if b == nil {
				return fmt.Errorf("aes: record bucket does not exist")
			}
			mb := b.Bucket(metaBucket)
			if mb == nil {
				return fmt.Errorf("aes: metadata bucket does not exist")
			}
			raw := mb.Get(codecKey)
			if raw == nil {
				return fmt.Errorf("aes: codec key does not exist in metadata")
			}
			codecName = string(raw)
			return nil
		})
		require.NoError(t, err, "aes: View must succeed")
		assert.Equal(t, "aes-json", codecName, "aes: metadata codec marker")

		var rawValue []byte
		err = nativeDB.View(func(tx *bolt.Tx) error {
			b := tx.Bucket(recordBucket)
			if b == nil {
				return fmt.Errorf("aes: record bucket does not exist in second View")
			}
			idKey := []byte{0, 0, 0, 0, 0, 0, 0, 1}
			v := b.Get(idKey)
			if v == nil {
				return fmt.Errorf("aes: record with ID 1 does not exist")
			}
			rawValue = append([]byte(nil), v...)
			return nil
		})
		require.NoError(t, err, "aes: second View must succeed")
		assert.NotEmpty(t, rawValue, "aes: raw record bytes must not be empty")
		rawByCodec["aes.db"] = rawValue
	})

	// Prove the raw values are not all identical across the four codecs.
	allSame := true
	filenames := []string{"gob.db", "msgpack.db", "sereal.db", "aes.db"}
	for i := 0; i < len(filenames); i++ {
		for j := i + 1; j < len(filenames); j++ {
			if !bytes.Equal(rawByCodec[filenames[i]], rawByCodec[filenames[j]]) {
				allSame = false
			}
		}
	}
	assert.False(t, allSame,
		"raw record bytes must not be identical across all four codecs")
}
