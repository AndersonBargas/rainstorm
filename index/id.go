package index

import (
	"bytes"
	"context"

	"github.com/AndersonBargas/rainstorm/v6/internal"
	bolt "go.etcd.io/bbolt"
)

// NewIDIndex loads a IDIndex
func NewIDIndex(parent *bolt.Bucket, indexName []byte) (*IDIndex, error) {
	if parent == nil {
		return nil, wrapError("index new id", ErrNilParam)
	}
	return &IDIndex{
		IndexBucket: parent,
	}, nil
}

// IDIndex is an index that references unique values and the corresponding ID.
type IDIndex struct {
	IndexBucket *bolt.Bucket
}

// Add a value to the unique index
func (idx *IDIndex) Add(ctx context.Context, value []byte, targetID []byte) error {
	if err := checkContext(ctx); err != nil {
		return wrapError("index add", err)
	}
	if len(value) == 0 {
		return wrapError("index add", ErrNilParam)
	}
	if len(targetID) == 0 {
		return wrapError("index add", ErrNilParam)
	}

	return nil
}

// Remove a value from the unique index
func (idx *IDIndex) Remove(ctx context.Context, value []byte) error {
	if err := checkContext(ctx); err != nil {
		return wrapError("index remove", err)
	}
	return nil
}

// RemoveID removes an ID from the unique index
func (idx *IDIndex) RemoveID(ctx context.Context, id []byte) error {
	if err := checkContext(ctx); err != nil {
		return wrapError("index remove id", err)
	}
	return nil
}

// Get the id corresponding to the given value
func (idx *IDIndex) Get(ctx context.Context, value []byte) ([]byte, error) {
	if err := checkContext(ctx); err != nil {
		return nil, wrapError("index get", err)
	}

	raw := idx.IndexBucket.Get(value)

	if err := checkContext(ctx); err != nil {
		return nil, wrapError("index get", err)
	}

	return raw, nil
}

// All returns all the ids corresponding to the given value
func (idx *IDIndex) All(ctx context.Context, value []byte, opts *Options) ([][]byte, error) {
	if err := checkContext(ctx); err != nil {
		return nil, wrapError("index all", err)
	}

	id := idx.IndexBucket.Get(value)
	if err := checkContext(ctx); err != nil {
		return nil, wrapError("index all", err)
	}
	if id != nil {
		return [][]byte{id}, nil
	}

	return nil, nil
}

// AllRecords returns all the IDs of this index
func (idx *IDIndex) AllRecords(ctx context.Context, opts *Options) ([][]byte, error) {
	if err := checkContext(ctx); err != nil {
		return nil, wrapError("index all records", err)
	}
	var list [][]byte
	return list, nil
}

// Range returns the ids corresponding to the given range of values
func (idx *IDIndex) Range(ctx context.Context, min []byte, max []byte, opts *Options) ([][]byte, error) {
	if err := checkContext(ctx); err != nil {
		return nil, wrapError("index range", err)
	}

	var list [][]byte

	c := internal.RangeCursor{
		C:       idx.IndexBucket.Cursor(),
		Reverse: opts != nil && opts.Reverse,
		Min:     min,
		Max:     max,
		CompareFn: func(val, limit []byte) int {
			return bytes.Compare(val, limit)
		},
	}

	for ident, _ := c.First(); ident != nil && c.Continue(ident); ident, _ = c.Next() {
		if err := checkContext(ctx); err != nil {
			return nil, wrapError("index range", err)
		}

		if opts != nil && opts.Skip > 0 {
			opts.Skip--
			continue
		}

		if opts != nil && opts.Limit == 0 {
			break
		}

		if opts != nil && opts.Limit > 0 {
			opts.Limit--
		}

		list = append(list, ident)
	}

	if err := checkContext(ctx); err != nil {
		return nil, wrapError("index range", err)
	}

	return list, nil
}

// Prefix returns the ids whose values have the given prefix.
func (idx *IDIndex) Prefix(ctx context.Context, prefix []byte, opts *Options) ([][]byte, error) {
	if err := checkContext(ctx); err != nil {
		return nil, wrapError("index prefix", err)
	}

	var list [][]byte

	c := internal.PrefixCursor{
		C:       idx.IndexBucket.Cursor(),
		Reverse: opts != nil && opts.Reverse,
		Prefix:  prefix,
	}

	for ident, _ := c.First(); ident != nil && c.Continue(ident); ident, _ = c.Next() {
		if err := checkContext(ctx); err != nil {
			return nil, wrapError("index prefix", err)
		}

		if opts != nil && opts.Skip > 0 {
			opts.Skip--
			continue
		}

		if opts != nil && opts.Limit == 0 {
			break
		}

		if opts != nil && opts.Limit > 0 {
			opts.Limit--
		}

		list = append(list, ident)
	}

	if err := checkContext(ctx); err != nil {
		return nil, wrapError("index prefix", err)
	}

	return list, nil
}
