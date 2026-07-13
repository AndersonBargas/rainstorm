package rainstorm

import (
	"context"

	"github.com/AndersonBargas/rainstorm/v6/internal"
	"github.com/AndersonBargas/rainstorm/v6/q"
	bolt "go.etcd.io/bbolt"
)

// Select a list of records that match a list of matchers. Doesn't use indexes.
func (n *node) Select(matchers ...q.Matcher) Query {
	tree := q.And(matchers...)
	return newQuery(n, tree)
}

// Query is the low level query engine used by Rainstorm. It allows to operate searches through an entire bucket.
type Query interface {
	// Skip matching records by the given number
	Skip(int) Query

	// Limit the results by the given number
	Limit(int) Query

	// Order by the given fields, in descending precedence, left-to-right.
	OrderBy(...string) Query

	// Reverse the order of the results
	Reverse() Query

	// Bucket specifies the bucket name
	Bucket(string) Query

	// Find a list of matching records
	Find(ctx context.Context, to any) error

	// First gets the first matching record
	First(ctx context.Context, to any) error

	// Delete all matching records
	Delete(ctx context.Context, kind any) error

	// Count all the matching records
	Count(ctx context.Context, kind any) (int, error)

	// Returns all the records without decoding them
	Raw(ctx context.Context) ([][]byte, error)

	// Execute the given function for each raw element
	RawEach(ctx context.Context, fn func(key, value []byte) error) error

	// Execute the given function for each element
	Each(ctx context.Context, kind any, fn func(any) error) error
}

func newQuery(n *node, tree q.Matcher) *query {
	return &query{
		skip:  0,
		limit: -1,
		node:  n,
		tree:  tree,
	}
}

type query struct {
	limit   int
	skip    int
	reverse bool
	tree    q.Matcher
	node    *node
	bucket  string
	orderBy []string
}

func (q *query) Skip(nb int) Query {
	q.skip = nb
	return q
}

func (q *query) Limit(nb int) Query {
	q.limit = nb
	return q
}

func (q *query) OrderBy(field ...string) Query {
	q.orderBy = field
	return q
}

func (q *query) Reverse() Query {
	q.reverse = true
	return q
}

func (q *query) Bucket(bucketName string) Query {
	q.bucket = bucketName
	return q
}

func (q *query) Find(ctx context.Context, to any) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	sink, err := newListSink(q.node, to)
	if err != nil {
		return err
	}

	return q.runQuery(ctx, sink)
}

func (q *query) First(ctx context.Context, to any) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	sink, err := newFirstSink(q.node, to)
	if err != nil {
		return err
	}

	q.limit = 1
	return q.runQuery(ctx, sink)
}

func (q *query) Delete(ctx context.Context, kind any) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	sink, err := newDeleteSink(q.node, kind)
	if err != nil {
		return err
	}

	return q.runQuery(ctx, sink)
}

func (q *query) Count(ctx context.Context, kind any) (int, error) {
	if err := checkContext(ctx); err != nil {
		return 0, err
	}

	sink, err := newCountSink(q.node, kind)
	if err != nil {
		return 0, err
	}

	err = q.runQuery(ctx, sink)
	if err != nil {
		return 0, err
	}

	return sink.counter, nil
}

func (q *query) Raw(ctx context.Context) ([][]byte, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	sink := newRawSink()

	err := q.runQuery(ctx, sink)
	if err != nil {
		return nil, err
	}

	return sink.results, nil
}

func (q *query) RawEach(ctx context.Context, fn func(key, value []byte) error) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	sink := newRawSink()

	sink.execFn = fn

	return q.runQuery(ctx, sink)
}

func (q *query) Each(ctx context.Context, kind any, fn func(any) error) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	sink, err := newEachSink(kind)
	if err != nil {
		return err
	}

	sink.execFn = fn

	return q.runQuery(ctx, sink)
}

func (q *query) runQuery(ctx context.Context, sink sink) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	if q.node.tx != nil {
		return q.query(q.node.tx, sink)
	}
	if sink.readOnly() {
		return q.node.s.Bolt.View(func(tx *bolt.Tx) error {
			if err := checkContext(ctx); err != nil {
				return err
			}
			return q.query(tx, sink)
		})
	}
	return q.node.s.Bolt.Update(func(tx *bolt.Tx) error {
		if err := checkContext(ctx); err != nil {
			return err
		}
		return q.query(tx, sink)
	})
}

func (q *query) query(tx *bolt.Tx, sink sink) error {
	bucketName := q.bucket
	if bucketName == "" {
		bucketName = sink.bucketName()
	}
	// If still empty, try rootBucket fallback (keep as empty for root-bucket-as-data pattern)
	if bucketName == "" && len(q.node.rootBucket) == 0 {
		return ErrNoName
	}
	bucket := q.node.GetBucket(tx, bucketName)

	if q.limit == 0 {
		return sink.flush()
	}

	sorter := newSorter(q.node, sink)
	sorter.orderBy = q.orderBy
	sorter.reverse = q.reverse
	sorter.skip = q.skip
	sorter.limit = q.limit
	if bucket != nil {
		c := internal.Cursor{C: bucket.Cursor(), Reverse: q.reverse}
		for k, v := c.First(); k != nil; k, v = c.Next() {
			if v == nil {
				continue
			}

			stop, err := sorter.filter(q.tree, bucket, k, v)
			if err != nil {
				return err
			}

			if stop {
				break
			}
		}
	}

	return sorter.flush()
}
