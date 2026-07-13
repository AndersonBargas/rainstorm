package rainstorm

import (
	"context"
	"testing"

	"github.com/AndersonBargas/rainstorm/v6/codec/gob"
	"github.com/AndersonBargas/rainstorm/v6/codec/json"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

func TestNode(t *testing.T) {
	db, cleanup := createDB(t, Root("a"))
	defer cleanup()

	n1 := db.From("b", "c")
	node1, ok := n1.(*node)
	require.True(t, ok)
	require.Equal(t, db, node1.s)
	require.NotEqual(t, db.Node, n1)
	require.Equal(t, []string{"a"}, db.Node.(*node).rootBucket)
	require.Equal(t, []string{"a", "b", "c"}, node1.rootBucket)
	n2 := n1.From("d", "e")
	node2, ok := n2.(*node)
	require.True(t, ok)
	require.Equal(t, []string{"a", "b", "c", "d", "e"}, node2.rootBucket)
}

func TestNodeWithTransaction(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	var user User
	db.Bolt.Update(func(tx *bolt.Tx) error {
		dbx := db.Node.(*node).withTransaction(tx)
		err := dbx.Save(ctx, &User{ID: 10, Name: "John"})
		require.NoError(t, err)
		err = dbx.One(ctx, "ID", 10, &user)
		require.NoError(t, err)
		require.Equal(t, "John", user.Name)
		return nil
	})

	err := db.One(ctx, "ID", 10, &user)
	require.NoError(t, err)
}

func TestNodeWithCodec(t *testing.T) {
	t.Run("Inheritance", func(t *testing.T) {
		db, cleanup := createDB(t)
		defer cleanup()

		n := db.From("a").(*node)
		require.Equal(t, json.Codec, n.codec)
		n = n.From("b", "c", "d").(*node)
		require.Equal(t, json.Codec, n.codec)
		n = db.WithCodec(gob.Codec).(*node)
		n = n.From("e").(*node)
		require.Equal(t, gob.Codec, n.codec)
		o := n.From("f").WithCodec(json.Codec).(*node)
		require.Equal(t, gob.Codec, n.codec)
		require.Equal(t, json.Codec, o.codec)
	})

	t.Run("CodecCall", func(t *testing.T) {
		db, cleanup := createDB(t)
		defer cleanup()

		ctx := context.Background()

		type User struct {
			ID   int
			Name string `rainstorm:"index"`
		}

		requireBytesEqual := func(raw []byte, expected interface{}) {
			var u User
			err := gob.Codec.Unmarshal(raw, &u)
			require.NoError(t, err)
			require.Equal(t, expected, u)
		}

		n := db.From("a").WithCodec(gob.Codec)
		err := n.Set(ctx, "gobBucket", "key", &User{ID: 10, Name: "John"})
		require.NoError(t, err)
		b, err := n.GetBytes(ctx, "gobBucket", "key")
		require.NoError(t, err)
		requireBytesEqual(b, User{ID: 10, Name: "John"})

		id, err := toBytes(10, n.(*node).codec)
		require.NoError(t, err)

		err = n.Save(ctx, &User{ID: 10, Name: "John"})
		require.NoError(t, err)
		b, err = n.GetBytes(ctx, "User", id)
		require.NoError(t, err)
		requireBytesEqual(b, User{ID: 10, Name: "John"})

		err = n.Update(ctx, &User{ID: 10, Name: "Jack"})
		require.NoError(t, err)
		b, err = n.GetBytes(ctx, "User", id)
		require.NoError(t, err)
		requireBytesEqual(b, User{ID: 10, Name: "Jack"})

		err = n.UpdateField(ctx, &User{ID: 10}, "Name", "John")
		require.NoError(t, err)
		b, err = n.GetBytes(ctx, "User", id)
		require.NoError(t, err)
		requireBytesEqual(b, User{ID: 10, Name: "John"})

		var users []User
		err = n.Find(ctx, "Name", "John", &users)
		require.NoError(t, err)

		var user User
		err = n.One(ctx, "Name", "John", &user)
		require.NoError(t, err)

		err = n.AllByIndex(ctx, "Name", &users)
		require.NoError(t, err)

		err = n.All(ctx, &users)
		require.NoError(t, err)

		err = n.Range(ctx, "Name", "J", "K", &users)
		require.NoError(t, err)

		err = n.Prefix(ctx, "Name", "J", &users)
		require.NoError(t, err)

		_, err = n.Count(ctx, new(User))
		require.NoError(t, err)

		err = n.Select().Find(ctx, &users)
		require.NoError(t, err)
	})
}
