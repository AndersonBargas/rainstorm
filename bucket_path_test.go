package rainstorm

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFrom_DoesNotMutateParentPath verifies that deriving a child node
// via From does not mutate the parent's rootBucket.
func TestFrom_DoesNotMutateParentPath(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	parent := db.From("tenant")
	child := parent.From("users")

	require.Equal(t, []string{"tenant"}, parent.Bucket())
	require.Equal(t, []string{"tenant", "users"}, child.Bucket())

	// Derive another child and confirm the first child is unchanged.
	child2 := parent.From("orders")
	require.Equal(t, []string{"tenant", "users"}, child.Bucket())
	require.Equal(t, []string{"tenant", "orders"}, child2.Bucket())
	require.Equal(t, []string{"tenant"}, parent.Bucket())
}

// TestFrom_SiblingPathsAreIndependent verifies that sibling nodes
// derived from the same parent have independent paths.
func TestFrom_SiblingPathsAreIndependent(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	parent := db.From("tenant")
	users := parent.From("users")
	orders := parent.From("orders")

	require.Equal(t, []string{"tenant", "users"}, users.Bucket())
	require.Equal(t, []string{"tenant", "orders"}, orders.Bucket())
	require.Equal(t, []string{"tenant"}, parent.Bucket())

	// Mutate the slice returned by users.Bucket() — must not affect anyone.
	usersPath := users.Bucket()
	usersPath[0] = "CORRUPTED"
	usersPath = append(usersPath, "EXTRA")
	require.Equal(t, []string{"CORRUPTED", "users", "EXTRA"}, usersPath)

	require.Equal(t, []string{"tenant", "users"}, users.Bucket())
	require.Equal(t, []string{"tenant", "orders"}, orders.Bucket())
	require.Equal(t, []string{"tenant"}, parent.Bucket())
}

// TestBucket_ReturnsDefensiveCopy verifies that Bucket() returns
// a defensive copy which, when mutated, does not affect the node.
func TestBucket_ReturnsDefensiveCopy(t *testing.T) {
	db, cleanup := createDB(t, Root("a", "b", "c"))
	defer cleanup()

	path1 := db.Bucket()
	require.Equal(t, []string{"a", "b", "c"}, path1)

	// Mutate element.
	path1[1] = "MUTATED"

	// Append extra.
	path1 = append(path1, "EXTRA")
	require.Equal(t, []string{"a", "MUTATED", "c", "EXTRA"}, path1)

	// Get path again — must be the original.
	path2 := db.Bucket()
	require.Equal(t, []string{"a", "b", "c"}, path2)
	require.Len(t, path2, 3)
}

// TestBucket_ReturnsDefensiveCopyNil verifies that Bucket on a node
// with nil rootBucket returns nil, not an empty slice.
func TestBucket_ReturnsDefensiveCopyNil(t *testing.T) {
	n := &node{}
	require.Nil(t, n.Bucket())
}

// TestRoot_CopiesCallerPath verifies that the Root option copies
// the caller's slice so that subsequent mutations don't affect the node.
func TestRoot_CopiesCallerPath(t *testing.T) {
	parts := []string{"tenant", "context"}
	rootOption := Root(parts...)

	// Root must snapshot its arguments when the option is created, not later
	// when Open applies it.
	parts[0] = "mutated-before-open"

	db, cleanup := createDB(t, rootOption)
	defer cleanup()

	parts[1] = "mutated-after-open"
	require.Equal(t, []string{"tenant", "context"}, db.Bucket())
}

// TestRoot_DifferentCallsIndependent verifies that distinct Root calls
// produce independent paths.
func TestRoot_DifferentCallsIndependent(t *testing.T) {
	db1, cleanup1 := createDB(t, Root("a"))
	defer cleanup1()
	db2, cleanup2 := createDB(t, Root("b"))
	defer cleanup2()

	require.Equal(t, []string{"a"}, db1.Bucket())
	require.Equal(t, []string{"b"}, db2.Bucket())

	// Mutate one — must not affect the other.
	p := db1.Bucket()
	p[0] = "CORRUPTED"
	require.Equal(t, []string{"a"}, db1.Bucket())
	require.Equal(t, []string{"b"}, db2.Bucket())
}

// TestBucketHelpers_DoNotMutateNodePath exercises real operations
// that call CreateBucketIfNotExists and GetBucket and confirms
// the node path is preserved.
func TestBucketHelpers_DoNotMutateNodePath(t *testing.T) {
	db, cleanup := createDB(t, Root("root"))
	defer cleanup()

	node := db.From("tenant").(*node)
	original := []string{"root", "tenant"}
	require.Equal(t, original, node.rootBucket)

	// Use a real bbolt write transaction.
	writeTx, err := db.NativeDB().Begin(true)
	require.NoError(t, err)

	// createBucketIfNotExists
	b, err := node.createBucketIfNotExists(writeTx, "users")
	require.NoError(t, err)
	require.NotNil(t, b)
	require.Equal(t, original, node.rootBucket, "CreateBucketIfNotExists must not mutate rootBucket")

	// createBucketIfNotExists with empty bucket
	b2, err := node.createBucketIfNotExists(writeTx, "")
	require.NoError(t, err)
	require.NotNil(t, b2)
	require.Equal(t, original, node.rootBucket, "CreateBucketIfNotExists with empty must not mutate rootBucket")

	// getBucket
	b3 := node.getBucket(writeTx, "users")
	require.NotNil(t, b3)
	require.Equal(t, original, node.rootBucket, "GetBucket must not mutate rootBucket")

	// getBucket with empty children
	b4 := node.getBucket(writeTx)
	require.NotNil(t, b4)
	require.Equal(t, original, node.rootBucket, "GetBucket without children must not mutate rootBucket")

	require.NoError(t, writeTx.Commit())
}

// TestNodePath_ExcessCapacity_From verifies that From does not write
// into hidden positions of the parent's backing array when the parent
// slice has excess capacity.
func TestNodePath_ExcessCapacity_From(t *testing.T) {
	// Construct a node with a slice that has excess capacity.
	root := make([]string, 1, 8)
	root[0] = "tenant"

	parent := &node{rootBucket: root}

	// Derive a child.
	child := parent.From("users")
	childNode := child.(*node)

	// The child should have the correct path.
	require.Equal(t, []string{"tenant", "users"}, childNode.rootBucket)

	// The parent's backing array beyond len(root) must remain empty.
	full := root[:cap(root)]
	for i := len(root); i < len(full); i++ {
		require.Equal(t, "", full[i], "position %d in parent backing array was written", i)
	}

	// The parent path must be unchanged.
	require.Equal(t, []string{"tenant"}, parent.rootBucket)
}

// TestNodePath_ExcessCapacity_CreateBucketIfNotExists verifies that
// CreateBucketIfNotExists does not write into the parent's backing array.
func TestNodePath_ExcessCapacity_CreateBucketIfNotExists(t *testing.T) {
	root := make([]string, 1, 8)
	root[0] = "tenant"

	n := &node{rootBucket: root}

	db, cleanup := createDB(t)
	defer cleanup()

	writeTx, err := db.NativeDB().Begin(true)
	require.NoError(t, err)

	_, err = n.createBucketIfNotExists(writeTx, "users")
	require.NoError(t, err)

	require.NoError(t, writeTx.Commit())

	// Parent backing array beyond len(root) must remain empty.
	full := root[:cap(root)]
	for i := len(root); i < len(full); i++ {
		require.Equal(t, "", full[i], "position %d in parent backing array was written", i)
	}

	require.Equal(t, []string{"tenant"}, n.rootBucket)
}

// TestNodePath_ExcessCapacity_GetBucket verifies that
// getBucket does not write into the parent's backing array.
func TestNodePath_ExcessCapacity_GetBucket(t *testing.T) {
	root := make([]string, 1, 8)
	root[0] = "tenant"

	n := &node{rootBucket: root}

	db, cleanup := createDB(t)
	defer cleanup()

	writeTx, err := db.NativeDB().Begin(true)
	require.NoError(t, err)

	// First create the bucket so that getBucket can find it.
	_, err = n.createBucketIfNotExists(writeTx, "users")
	require.NoError(t, err)

	// Now getBucket should find it.
	b := n.getBucket(writeTx, "users")
	require.NotNil(t, b)

	require.NoError(t, writeTx.Commit())

	// Parent backing array beyond len(root) must remain empty.
	full := root[:cap(root)]
	for i := len(root); i < len(full); i++ {
		require.Equal(t, "", full[i], "position %d in parent backing array was written", i)
	}

	require.Equal(t, []string{"tenant"}, n.rootBucket)
}

// TestBucketScanner_ReturnsIndependentNodePaths verifies that scanner
// results have correct, independent paths.
func TestBucketScanner_ReturnsIndependentNodePaths(t *testing.T) {
	ctx := context.Background()
	db, cleanup := createDB(t)
	defer cleanup()

	parent := db.From("root")

	// Create child buckets via saves.
	for i := 1; i <= 3; i++ {
		child := parent.From(fmtBucketName(i))
		err := child.Save(ctx, &SimpleUser{ID: i, Name: "test"})
		require.NoError(t, err)
	}

	// Scan.
	nodes, err := parent.PrefixScan(ctx, "child")
	require.NoError(t, err)
	require.Len(t, nodes, 3)

	// Each node must have a correct, independent path.
	paths := make([][]string, len(nodes))
	for i, node := range nodes {
		paths[i] = node.Bucket()
		require.Len(t, paths[i], 2) // root, child_N
		require.Equal(t, "root", paths[i][0])
	}

	// Parent must be unchanged.
	require.Equal(t, []string{"root"}, parent.Bucket())

	// Mutating the return of Bucket() on one result must not affect others.
	paths[0][0] = "CORRUPTED"
	paths[0] = append(paths[0], "EXTRA")

	for i, node := range nodes {
		p := node.Bucket()
		if i == 0 {
			require.NotEqual(t, paths[0], p)
		}
		require.Len(t, p, 2)

		var records []SimpleUser
		require.NoError(t, node.All(ctx, &records))
		require.Len(t, records, 1, "scanned node must address its original child bucket")
		require.Equal(t, "test", records[0].Name)
	}
}

// TestBucketScanner_NestedRoot verifies scanner behavior with nested root buckets.
func TestBucketScanner_NestedRoot(t *testing.T) {
	ctx := context.Background()
	db, cleanup := createDB(t, Root("tenant", "context"))
	defer cleanup()

	// db already has rootBucket ["tenant", "context"].
	// Create child buckets under it.
	for i := 1; i <= 3; i++ {
		child := db.From(fmtBucketName(i))
		err := child.Save(ctx, &SimpleUser{ID: i, Name: "test"})
		require.NoError(t, err)
	}

	nodes, err := db.PrefixScan(ctx, "child")
	require.NoError(t, err)
	require.Len(t, nodes, 3)

	for _, node := range nodes {
		p := node.Bucket()
		// rootBucket is ["tenant", "context", "child_N"]
		require.Len(t, p, 3)
		require.Equal(t, "tenant", p[0])
		require.Equal(t, "context", p[1])

		var records []SimpleUser
		require.NoError(t, node.All(ctx, &records))
		require.Len(t, records, 1, "nested scanned node must address its original child bucket")
	}

	require.Equal(t, []string{"tenant", "context"}, db.Bucket())
}

// TestNodePath_DerivationAndInspectionAreRaceSafe verifies that
// concurrent From and Bucket calls on an immutable shared parent
// are race-free.
func TestNodePath_DerivationAndInspectionAreRaceSafe(t *testing.T) {
	parent := &node{rootBucket: []string{"tenant"}}

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()

			// Derive children.
			child := parent.From("users")
			_ = child.Bucket()

			// Inspect parent.
			p := parent.Bucket()
			_ = p

			// Mutate only the local copy returned by Bucket().
			cp := parent.Bucket()
			cp = append(cp, "extra")
			_ = cp

			// Derive more children.
			for j := 0; j < 5; j++ {
				c := parent.From("orders")
				bc := c.Bucket()
				bc = append(bc, "mutated-copy")
				_ = bc
			}
		}(i)
	}

	wg.Wait()

	// After all goroutines, parent must be intact.
	require.Equal(t, []string{"tenant"}, parent.Bucket())
}

// fmtBucketName is a helper for generating bucket names in tests.
func fmtBucketName(i int) string {
	return fmt.Sprintf("child_%d", i)
}
