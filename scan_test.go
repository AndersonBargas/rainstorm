package rainstorm

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrefixScan(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	node := db.From("node")

	// run prefix scan on empty data
	list, err := node.PrefixScan(ctx, "foo")
	require.NoError(t, err)
	require.Empty(t, list)

	doTestPrefixScan(t, ctx, node)
	doTestPrefixScan(t, ctx, db)

	nodeWithTransaction, _ := db.Begin(ctx, true)
	defer nodeWithTransaction.Commit(ctx)

	doTestPrefixScan(t, ctx, nodeWithTransaction)
}

func doTestPrefixScan(t *testing.T, ctx context.Context, node Node) {
	for i := 1; i < 3; i++ {
		n := node.From(fmt.Sprintf("%d%02d", 2015, i))
		err := n.Save(ctx, &SimpleUser{ID: i, Name: "John"})
		require.NoError(t, err)
	}

	for i := 1; i < 4; i++ {
		n := node.From(fmt.Sprintf("%d%02d", 2016, i))
		err := n.Save(ctx, &SimpleUser{ID: i, Name: "John"})
		require.NoError(t, err)
	}

	nodes, err := node.PrefixScan(ctx, "2015")
	require.NoError(t, err)
	require.Len(t, nodes, 2)

	nodes, err = node.PrefixScan(ctx, "20")
	require.NoError(t, err)
	require.Len(t, nodes, 5)

	buckets2016, err := node.PrefixScan(ctx, "2016")
	require.NoError(t, err)
	require.Len(t, buckets2016, 3)
	count, err := buckets2016[1].Count(ctx, &SimpleUser{})

	require.NoError(t, err)
	require.Equal(t, 1, count)

	require.NoError(t, buckets2016[1].One(ctx, "ID", 2, &SimpleUser{}))
}

func TestPrefixScanWithEmptyPrefix(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	res, err := db.PrefixScan(context.Background(), "")
	require.NoError(t, err)
	require.Len(t, res, 1)
}

func TestPrefixScanSkipValues(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	err := db.Set(ctx, "a", "2015", 1)
	require.NoError(t, err)
	err = db.From("a", "2016").Save(ctx, &SimpleUser{ID: 1, Name: "John"})
	require.NoError(t, err)

	res, err := db.From("a").PrefixScan(ctx, "20")
	require.NoError(t, err)
	require.Len(t, res, 1)
}

func TestRangeScan(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	node := db.From("node")

	doTestRangeScan(t, ctx, node)
	doTestRangeScan(t, ctx, db)

	nodeWithTransaction, _ := db.Begin(ctx, true)
	defer nodeWithTransaction.Commit(ctx)

	doTestRangeScan(t, ctx, nodeWithTransaction)
}

func doTestRangeScan(t *testing.T, ctx context.Context, node Node) {

	for y := 2012; y <= 2016; y++ {
		for m := 1; m <= 12; m++ {
			n := node.From(fmt.Sprintf("%d%02d", y, m))
			require.NoError(t, n.Save(ctx, &SimpleUser{ID: m, Name: "John"}))
		}
	}

	nodes, err := node.RangeScan(ctx, "2015", "2016")
	require.NoError(t, err)
	require.Len(t, nodes, 12)

	nodes, err = node.RangeScan(ctx, "201201", "201203")
	require.NoError(t, err)
	require.Len(t, nodes, 3)

	nodes, err = node.RangeScan(ctx, "2012", "201612")
	require.NoError(t, err)
	require.Len(t, nodes, 60)

	nodes, err = node.RangeScan(ctx, "2012", "2017")
	require.NoError(t, err)
	require.Len(t, nodes, 60)

	secondIn2015, err := node.RangeScan(ctx, "2015", "2016")
	require.NoError(t, err)
	require.NoError(t, secondIn2015[1].One(ctx, "ID", 2, &SimpleUser{}))
}

func TestRangeScanSkipValues(t *testing.T) {
	db, cleanup := createDB(t)
	defer cleanup()

	ctx := context.Background()

	err := db.Set(ctx, "a", "2015", 1)
	require.NoError(t, err)
	err = db.From("a", "2016").Save(ctx, &SimpleUser{ID: 1, Name: "John"})
	require.NoError(t, err)

	res, err := db.From("a").RangeScan(ctx, "2015", "2018")
	require.NoError(t, err)
	require.Len(t, res, 1)
}
