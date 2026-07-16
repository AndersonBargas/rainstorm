package protobuf

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/AndersonBargas/rainstorm/v6"
	"github.com/AndersonBargas/rainstorm/v6/codec/internal"
	"github.com/stretchr/testify/require"
)

func TestProtobuf(t *testing.T) {
	u1 := SimpleUser{ID: 1, Name: "John"}
	u2 := SimpleUser{}
	internal.RoundtripTester(t, Codec, &u1, &u2)
	require.Equal(t, u1.ID, u2.ID)
}

func TestSave(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, ctx)

	u1 := SimpleUser{ID: 1, Name: "John"}
	require.NoError(t, db.Save(ctx, &u1))

	u2 := SimpleUser{}
	require.NoError(t, db.One(ctx, "ID", uint64(1), &u2))
	require.Equal(t, u1.Name, u2.Name)
}

func TestGetSet(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t, ctx)

	require.NoError(t, db.Set(ctx, "bucket", "key", "value"))

	var s string
	require.NoError(t, db.Get(ctx, "bucket", "key", &s))
	require.Equal(t, "value", s)
}

func TestCodecName(t *testing.T) {
	require.Equal(t, "protobuf", Codec.Name())
}

func openTestDB(t *testing.T, ctx context.Context) *rainstorm.DB {
	t.Helper()

	db, err := rainstorm.Open(ctx, filepath.Join(t.TempDir(), "rainstorm.db"), rainstorm.Codec(Codec))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	return db
}
