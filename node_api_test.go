package rainstorm_test

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/AndersonBargas/rainstorm/v6"
	"github.com/AndersonBargas/rainstorm/v6/codec/gob"
	"github.com/stretchr/testify/require"
)

// testUser is used in external-package consumer tests.
type testUser struct {
	ID   int    `rainstorm:"id"`
	Name string `rainstorm:"index"`
}

// TestNode_ConsumerCanUseWithoutBbolt verifies that an external consumer can
// declare and use a rainstorm.Node without importing bbolt.
func TestNode_ConsumerCanUseWithoutBbolt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rainstorm.db")
	db, err := rainstorm.Open(context.Background(), path, rainstorm.Codec(gob.Codec))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	ctx := context.Background()

	// Exercise Node interface: From, Bucket, Codec, WithCodec.
	var n rainstorm.Node = db.From("tenant", "ns")
	require.Equal(t, []string{"tenant", "ns"}, n.Bucket())

	n2 := n.From("child")
	require.Equal(t, []string{"tenant", "ns", "child"}, n2.Bucket())

	require.Equal(t, gob.Codec, n.Codec())

	nWithCodec := n.WithCodec(gob.Codec)
	require.Equal(t, gob.Codec, nWithCodec.Codec())

	// Perform a public operation through the Node.
	err = n.Save(ctx, &testUser{ID: 1, Name: "test"})
	require.NoError(t, err)

	// Read back using the same nested node.
	var result testUser
	err = n.One(ctx, "ID", 1, &result)
	require.NoError(t, err)
	require.Equal(t, "test", result.Name)
}

// ---------------------------------------------------------------------------
// Reflection helpers
// ---------------------------------------------------------------------------

// containsBboltType recursively checks whether typ contains a type from bboltPkg.
func containsBboltType(typ reflect.Type, bboltPkg string) bool {
	seen := make(map[reflect.Type]bool)
	var visit func(reflect.Type) bool
	visit = func(typ reflect.Type) bool {
		if typ == nil || seen[typ] {
			return false
		}
		seen[typ] = true
		if typ.PkgPath() == bboltPkg {
			return true
		}
		switch typ.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Chan:
			return visit(typ.Elem())
		case reflect.Map:
			return visit(typ.Key()) || visit(typ.Elem())
		case reflect.Func:
			for i := 0; i < typ.NumIn(); i++ {
				if visit(typ.In(i)) {
					return true
				}
			}
			for i := 0; i < typ.NumOut(); i++ {
				if visit(typ.Out(i)) {
					return true
				}
			}
		case reflect.Interface:
			for i := 0; i < typ.NumMethod(); i++ {
				if visit(typ.Method(i).Type) {
					return true
				}
			}
		case reflect.Struct:
			for i := 0; i < typ.NumField(); i++ {
				if visit(typ.Field(i).Type) {
					return true
				}
			}
		}
		return false
	}
	return visit(typ)
}

// TestNodeInterface_NoBboltExposure uses reflection on the public Node interface
// to prove that no method parameter or return type exposes a bbolt type.
func TestNodeInterface_NoBboltExposure(t *testing.T) {
	iface := reflect.TypeOf((*rainstorm.Node)(nil)).Elem()

	bboltPkg := "go.etcd.io/bbolt"

	forbiddenMethods := map[string]bool{
		"Begin":                   true,
		"Commit":                  true,
		"Rollback":                true,
		"WithTransaction":         true,
		"GetBucket":               true,
		"CreateBucketIfNotExists": true,
	}

	for i := 0; i < iface.NumMethod(); i++ {
		m := iface.Method(i)

		// Assert no forbidden method names.
		require.False(t, forbiddenMethods[m.Name],
			"forbidden method %q found in Node interface", m.Name)

		// Inspect parameters for bbolt types.
		for j := 0; j < m.Type.NumIn(); j++ {
			paramType := m.Type.In(j)
			require.False(t, containsBboltType(paramType, bboltPkg),
				"method %q parameter %d exposes bbolt type %v", m.Name, j, paramType)
		}

		// Inspect return types for bbolt types.
		for j := 0; j < m.Type.NumOut(); j++ {
			outType := m.Type.Out(j)
			require.False(t, containsBboltType(outType, bboltPkg),
				"method %q return %d exposes bbolt type %v", m.Name, j, outType)
		}
	}
}

// TestDBInterface_NoBboltExposure uses reflection on the DB type to prove:
// - DB has no exported field named Bolt;
// - DB has no exported field recursively containing a bbolt type;
// - NativeDB is the only method returning a bbolt type.
func TestDBInterface_NoBboltExposure(t *testing.T) {
	dbType := reflect.TypeOf((*rainstorm.DB)(nil)).Elem()
	dbPtrType := reflect.TypeOf((*rainstorm.DB)(nil))
	bboltPkg := "go.etcd.io/bbolt"

	// DB has no exported field named Bolt.
	for i := 0; i < dbType.NumField(); i++ {
		f := dbType.Field(i)
		// Only check exported (public) fields.
		if !f.IsExported() {
			continue
		}
		require.NotEqual(t, "Bolt", f.Name,
			"DB must not have an exported field named Bolt (found %q)", f.Name)

		// DB has no exported field whose type exposes a bbolt type.
		require.False(t, containsBboltType(f.Type, bboltPkg),
			"DB field %q exposes bbolt type %v", f.Name, f.Type)
	}

	// Verify NativeDB exists and returns the expected type.
	// NativeDB has a pointer receiver, so look it up on *DB.
	nativeDB, ok := dbPtrType.MethodByName("NativeDB")
	require.True(t, ok, "DB must have a NativeDB method")
	require.Equal(t, 1, nativeDB.Type.NumOut(), "NativeDB must return exactly one value")
	outType := nativeDB.Type.Out(0)
	require.Equal(t, reflect.Pointer, outType.Kind(), "NativeDB must return a pointer")
	require.Equal(t, bboltPkg, outType.Elem().PkgPath(), "NativeDB must return *bolt.DB")

	// Verify no other method on *DB returns a bbolt type (except NativeDB).
	for i := 0; i < dbPtrType.NumMethod(); i++ {
		m := dbPtrType.Method(i)
		if m.Name == "NativeDB" {
			continue // already validated above
		}
		for j := 0; j < m.Type.NumOut(); j++ {
			outType := m.Type.Out(j)
			require.False(t, containsBboltType(outType, bboltPkg),
				"DB method %q return %d exposes bbolt type %v", m.Name, j, outType)
		}
	}
}
