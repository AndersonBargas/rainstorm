package rainstorm

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// ============================================================================
// Test 1: PrefixScan with already-canceled context returns nil, context.Canceled.
// ============================================================================

func TestPrefixScan_AlreadyCanceled(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	// Seed some data.
	node := db.From("scan-node")
	for i := 1; i <= 5; i++ {
		child := node.From(fmt.Sprintf("2015%02d", i))
		require.NoError(t, child.Save(ctx, &SimpleUser{ID: i, Name: "John"}))
	}

	nodes, err := node.PrefixScan(canceledCtx(), "2015")
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))
	require.Nil(t, nodes, "must return nil on cancellation")
}

// ============================================================================
// Test 2: RangeScan with already-canceled context returns nil, context.Canceled.
// ============================================================================

func TestRangeScan_AlreadyCanceled(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	node := db.From("scan-node")
	for i := 1; i <= 5; i++ {
		child := node.From(fmt.Sprintf("2015%02d", i))
		require.NoError(t, child.Save(ctx, &SimpleUser{ID: i, Name: "John"}))
	}

	nodes, err := node.RangeScan(canceledCtx(), "201501", "201503")
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))
	require.Nil(t, nodes, "must return nil on cancellation")
}

// ============================================================================
// Test 3: DeadlineExceeded is preserved.
// ============================================================================

func TestPrefixScan_DeadlineExceeded(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	node := db.From("scan-node")
	require.NoError(t, node.Save(ctx, &SimpleUser{ID: 1, Name: "John"}))

	nodes, err := node.PrefixScan(timedOutCtx(), "2015")
	require.Error(t, err)
	require.True(t, errors.Is(err, context.DeadlineExceeded))
	require.Nil(t, nodes)
}

func TestRangeScan_DeadlineExceeded(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	node := db.From("scan-node")
	require.NoError(t, node.Save(ctx, &SimpleUser{ID: 1, Name: "John"}))

	nodes, err := node.RangeScan(timedOutCtx(), "201501", "201599")
	require.Error(t, err)
	require.True(t, errors.Is(err, context.DeadlineExceeded))
	require.Nil(t, nodes)
}

// ============================================================================
// Test 4: PrefixScan canceled during cursor iteration returns nil, never partial.
// ============================================================================

func TestPrefixScan_CanceledDuringIterationReturnsNil(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create enough buckets to guarantee multiple cursor iterations.
	node := db.From("scan-node")
	const numBuckets = 20
	for i := 0; i < numBuckets; i++ {
		child := node.From(fmt.Sprintf("bucket_%02d", i))
		require.NoError(t, child.Save(ctx, &SimpleUser{ID: i + 1, Name: "test"}))
	}

	// cancelAt=5 triggers during cursor iteration but well before completion.
	sctx := &stepContext{cancelAt: 5}
	nodes, err := node.PrefixScan(sctx, "bucket")
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))
	require.Nil(t, nodes, "must return nil, never partial results")

	// Verify cancellation happened during iteration (not before first key).
	require.GreaterOrEqual(t, sctx.Calls(), 5,
		"must have progressed into cursor iteration")
}

// ============================================================================
// Test 5: RangeScan canceled during cursor iteration returns nil, never partial.
// ============================================================================

func TestRangeScan_CanceledDuringIterationReturnsNil(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	node := db.From("scan-node")
	const numBuckets = 20
	for i := 0; i < numBuckets; i++ {
		child := node.From(fmt.Sprintf("bucket_%02d", i))
		require.NoError(t, child.Save(ctx, &SimpleUser{ID: i + 1, Name: "test"}))
	}

	sctx := &stepContext{cancelAt: 5}
	nodes, err := node.RangeScan(sctx, "bucket_00", "bucket_99")
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))
	require.Nil(t, nodes, "must return nil, never partial results")

	require.GreaterOrEqual(t, sctx.Calls(), 5,
		"must have progressed into cursor iteration")
}

// ============================================================================
// Test 6: Deterministic context proves multiple cursor iterations.
// ============================================================================

func TestPrefixScan_MultipleIterationsBeforeCancellation(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	node := db.From("scan-node")
	const numBuckets = 20
	for i := 0; i < numBuckets; i++ {
		child := node.From(fmt.Sprintf("bucket_%02d", i))
		require.NoError(t, child.Save(ctx, &SimpleUser{ID: i + 1, Name: "test"}))
	}

	// cancelAt=10: enough to iterate over several keys.
	sctx := &stepContext{cancelAt: 10}
	nodes, err := node.PrefixScan(sctx, "bucket")
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))
	require.Nil(t, nodes)

	// Must have checked context at least 10 times, proving multiple
	// cursor iterations occurred before cancellation.
	require.GreaterOrEqual(t, sctx.Calls(), 10,
		"multiple cursor iterations must have occurred")
}

func TestRangeScan_MultipleIterationsBeforeCancellation(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	node := db.From("scan-node")
	const numBuckets = 20
	for i := 0; i < numBuckets; i++ {
		child := node.From(fmt.Sprintf("bucket_%02d", i))
		require.NoError(t, child.Save(ctx, &SimpleUser{ID: i + 1, Name: "test"}))
	}

	sctx := &stepContext{cancelAt: 10}
	nodes, err := node.RangeScan(sctx, "bucket_00", "bucket_99")
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))
	require.Nil(t, nodes)
	require.GreaterOrEqual(t, sctx.Calls(), 10,
		"multiple cursor iterations must have occurred")
}

// ============================================================================
// Test 7: Subsequent valid PrefixScan returns complete expected result.
// ============================================================================

func TestPrefixScan_ValidAfterCanceled(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	node := db.From("scan-node")
	for i := 0; i < 10; i++ {
		child := node.From(fmt.Sprintf("bucket_%02d", i))
		require.NoError(t, child.Save(ctx, &SimpleUser{ID: i + 1, Name: "test"}))
	}

	// Cancel once.
	sctx := &stepContext{cancelAt: 5}
	_, err := node.PrefixScan(sctx, "bucket")
	require.Error(t, err)

	// Subsequent valid scan must return the full result.
	nodes, err := node.PrefixScan(ctx, "bucket")
	require.NoError(t, err)
	require.Len(t, nodes, 10, "valid scan after canceled must return all results")
}

// ============================================================================
// Test 8: Subsequent valid RangeScan returns complete expected result.
// ============================================================================

func TestRangeScan_ValidAfterCanceled(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	node := db.From("scan-node")
	for i := 0; i < 10; i++ {
		child := node.From(fmt.Sprintf("bucket_%02d", i))
		require.NoError(t, child.Save(ctx, &SimpleUser{ID: i + 1, Name: "test"}))
	}

	sctx := &stepContext{cancelAt: 5}
	_, err := node.RangeScan(sctx, "bucket_00", "bucket_99")
	require.Error(t, err)

	nodes, err := node.RangeScan(ctx, "bucket_00", "bucket_99")
	require.NoError(t, err)
	require.Len(t, nodes, 10, "valid scan after canceled must return all results")
}

// ============================================================================
// Test 9: Nested root path results are correct.
// ============================================================================

func TestPrefixScan_NestedRootPathsAreCorrect(t *testing.T) {
	ctx := context.Background()
	db, cleanup := createDB(t, Root("tenant", "context"))
	defer cleanup()

	for i := 1; i <= 3; i++ {
		child := db.From(fmt.Sprintf("child_%d", i))
		require.NoError(t, child.Save(ctx, &SimpleUser{ID: i, Name: "test"}))
	}

	nodes, err := db.PrefixScan(ctx, "child")
	require.NoError(t, err)
	require.Len(t, nodes, 3)

	for _, node := range nodes {
		p := node.Bucket()
		require.Len(t, p, 3) // tenant, context, child_N
		require.Equal(t, "tenant", p[0])
		require.Equal(t, "context", p[1])

		var records []SimpleUser
		require.NoError(t, node.All(ctx, &records))
		require.Len(t, records, 1)
	}
}

func TestRangeScan_NestedRootPathsAreCorrect(t *testing.T) {
	ctx := context.Background()
	db, cleanup := createDB(t, Root("ns", "scope"))
	defer cleanup()

	for i := 1; i <= 3; i++ {
		child := db.From(fmt.Sprintf("child_%d", i))
		require.NoError(t, child.Save(ctx, &SimpleUser{ID: i, Name: "test"}))
	}

	nodes, err := db.RangeScan(ctx, "child_1", "child_9")
	require.NoError(t, err)
	require.Len(t, nodes, 3)

	for _, node := range nodes {
		p := node.Bucket()
		require.Len(t, p, 3) // ns, scope, child_N
		require.Equal(t, "ns", p[0])
		require.Equal(t, "scope", p[1])
	}
}

// ============================================================================
// Test 10: Every returned scanner Node has an independent path.
// ============================================================================

func TestPrefixScan_EachNodeHasIndependentPath(t *testing.T) {
	ctx := context.Background()
	db, cleanup := createDB(t, Root("root"))
	defer cleanup()

	for i := 1; i <= 3; i++ {
		child := db.From(fmt.Sprintf("child_%d", i))
		require.NoError(t, child.Save(ctx, &SimpleUser{ID: i, Name: "test"}))
	}

	nodes, err := db.PrefixScan(ctx, "child")
	require.NoError(t, err)
	require.Len(t, nodes, 3)

	// Collect paths.
	paths := make([][]string, len(nodes))
	for i, node := range nodes {
		paths[i] = node.Bucket()
	}

	// Mutate path from node 0 — must not affect others.
	paths[0][0] = "CORRUPTED"
	paths[0] = append(paths[0], "EXTRA")

	for i, node := range nodes {
		p := node.Bucket()
		require.Len(t, p, 2) // root, child_N
		if i == 0 {
			require.NotEqual(t, paths[0], p, "mutated copy must not affect node's real path")
		}

		// Each node must address its own bucket.
		var records []SimpleUser
		require.NoError(t, node.All(ctx, &records))
		require.Len(t, records, 1)
		require.Equal(t, "test", records[0].Name)
	}
}

func TestRangeScan_EachNodeHasIndependentPath(t *testing.T) {
	ctx := context.Background()
	db, cleanup := createDB(t)
	defer cleanup()

	node := db.From("parent")
	for i := 1; i <= 3; i++ {
		child := node.From(fmt.Sprintf("child_%d", i))
		require.NoError(t, child.Save(ctx, &SimpleUser{ID: i, Name: "test"}))
	}

	nodes, err := node.RangeScan(ctx, "child_1", "child_9")
	require.NoError(t, err)
	require.Len(t, nodes, 3)

	paths := make([][]string, len(nodes))
	for i, n := range nodes {
		paths[i] = n.Bucket()
	}

	paths[0][0] = "CORRUPTED"
	paths[0] = append(paths[0], "EXTRA")

	for i, n := range nodes {
		p := n.Bucket()
		require.Len(t, p, 2)
		if i == 0 {
			require.NotEqual(t, paths[0], p)
		}
	}
}

// ============================================================================
// Test 11: Mutating Node.Bucket() from one result does not affect any result.
// ============================================================================

func TestPrefixScan_MutatingBucketDoesNotAffectResults(t *testing.T) {
	ctx := context.Background()
	db, cleanup := createDB(t)
	defer cleanup()

	node := db.From("parent")
	for i := 1; i <= 5; i++ {
		child := node.From(fmt.Sprintf("child_%d", i))
		require.NoError(t, child.Save(ctx, &SimpleUser{ID: i, Name: "test"}))
	}

	nodes, err := node.PrefixScan(ctx, "child")
	require.NoError(t, err)
	require.Len(t, nodes, 5)

	// Mutate the Bucket() return of each node.
	for i := range nodes {
		b := nodes[i].Bucket()
		b[0] = "MUTATED"
		b = append(b, "EXTRA")
		require.Len(t, b, 3)
	}

	// Now re-read all nodes' buckets — must be unchanged.
	for i, n := range nodes {
		b := n.Bucket()
		require.Equal(t, "parent", b[0],
			"node %d bucket must not be affected by mutation", i)
		require.Len(t, b, 2, "node %d bucket must not have extra elements", i)
	}
}

func TestRangeScan_MutatingBucketDoesNotAffectResults(t *testing.T) {
	ctx := context.Background()
	db, cleanup := createDB(t)
	defer cleanup()

	node := db.From("parent")
	for i := 1; i <= 5; i++ {
		child := node.From(fmt.Sprintf("child_%d", i))
		require.NoError(t, child.Save(ctx, &SimpleUser{ID: i, Name: "test"}))
	}

	nodes, err := node.RangeScan(ctx, "child_1", "child_9")
	require.NoError(t, err)
	require.Len(t, nodes, 5)

	for i := range nodes {
		b := nodes[i].Bucket()
		b[0] = "MUTATED"
	}

	for i, n := range nodes {
		b := n.Bucket()
		require.Equal(t, "parent", b[0],
			"node %d bucket must not be affected by mutation", i)
	}
}

// ============================================================================
// Test 12: Scanner does not mutate the receiver root path.
// ============================================================================

func TestPrefixScan_DoesNotMutateReceiverRootPath(t *testing.T) {
	ctx := context.Background()
	db, cleanup := createDB(t, Root("immutable"))
	defer cleanup()

	original := db.Bucket()

	for i := 1; i <= 5; i++ {
		child := db.From(fmt.Sprintf("child_%d", i))
		require.NoError(t, child.Save(ctx, &SimpleUser{ID: i, Name: "test"}))
	}

	_, err := db.PrefixScan(ctx, "child")
	require.NoError(t, err)

	// Receiver root path must be unchanged.
	require.Equal(t, original, db.Bucket(), "PrefixScan must not mutate receiver root path")
}

func TestRangeScan_DoesNotMutateReceiverRootPath(t *testing.T) {
	ctx := context.Background()
	db, cleanup := createDB(t, Root("immutable"))
	defer cleanup()

	original := db.Bucket()

	for i := 1; i <= 5; i++ {
		child := db.From(fmt.Sprintf("child_%d", i))
		require.NoError(t, child.Save(ctx, &SimpleUser{ID: i, Name: "test"}))
	}

	_, err := db.RangeScan(ctx, "child_1", "child_9")
	require.NoError(t, err)

	require.Equal(t, original, db.Bucket(), "RangeScan must not mutate receiver root path")
}

// ============================================================================
// Test 13: Prefix and range successful ordering remains unchanged.
// ============================================================================

func TestPrefixScan_OrderingUnchanged(t *testing.T) {
	ctx := context.Background()
	db, cleanup := createDB(t)
	defer cleanup()

	node := db.From("ordering")
	keys := []string{"a", "aa", "ab", "b", "ba", "bb"}
	for _, k := range keys {
		child := node.From(k)
		require.NoError(t, child.Save(ctx, &SimpleUser{ID: 1, Name: "test"}))
	}

	nodes, err := node.PrefixScan(ctx, "a")
	require.NoError(t, err)
	require.Len(t, nodes, 3) // "a", "aa", "ab"

	for i, expected := range []string{"a", "aa", "ab"} {
		require.Equal(t, expected, nodes[i].Bucket()[1],
			"PrefixScan ordering must be lexicographic: position %d", i)
	}
}

func TestRangeScan_OrderingUnchanged(t *testing.T) {
	ctx := context.Background()
	db, cleanup := createDB(t)
	defer cleanup()

	node := db.From("ordering")
	keys := []string{"k01", "k02", "k03", "k04", "k05"}
	for _, k := range keys {
		child := node.From(k)
		require.NoError(t, child.Save(ctx, &SimpleUser{ID: 1, Name: "test"}))
	}

	nodes, err := node.RangeScan(ctx, "k01", "k03")
	require.NoError(t, err)
	require.Len(t, nodes, 3)

	for i, expected := range []string{"k01", "k02", "k03"} {
		require.Equal(t, expected, nodes[i].Bucket()[1],
			"RangeScan ordering must be lexicographic: position %d", i)
	}
}

// ============================================================================
// Test 14: Empty/no-match successful behavior remains compatible.
// ============================================================================

func TestPrefixScan_EmptyNoMatch(t *testing.T) {
	ctx := context.Background()
	db, cleanup := createDB(t)
	defer cleanup()

	// Scan on empty database.
	nodes, err := db.PrefixScan(ctx, "nonexistent")
	require.NoError(t, err)
	require.Empty(t, nodes)

	// Seed some data but scan for non-matching prefix.
	node := db.From("test")
	require.NoError(t, node.Save(ctx, &SimpleUser{ID: 1, Name: "John"}))

	nodes, err = node.PrefixScan(ctx, "zzz")
	require.NoError(t, err)
	require.Empty(t, nodes)
}

func TestRangeScan_EmptyNoMatch(t *testing.T) {
	ctx := context.Background()
	db, cleanup := createDB(t)
	defer cleanup()

	nodes, err := db.RangeScan(ctx, "nonexistent", "zzz")
	require.NoError(t, err)
	require.Empty(t, nodes)

	node := db.From("test")
	require.NoError(t, node.Save(ctx, &SimpleUser{ID: 1, Name: "John"}))

	// Range outside existing keys.
	nodes, err = node.RangeScan(ctx, "zzz", "zzz999")
	require.NoError(t, err)
	require.Empty(t, nodes)
}
