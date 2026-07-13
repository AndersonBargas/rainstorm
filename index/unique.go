package index

import (
	"bytes"
	"context"

	"github.com/AndersonBargas/rainstorm/v6/internal"
	bolt "go.etcd.io/bbolt"
)

// NewUniqueIndex loads a UniqueIndex
func NewUniqueIndex(parent *bolt.Bucket, indexName []byte) (*UniqueIndex, error) {
	var err error
	b := parent.Bucket(indexName)
	if b == nil {
		if !parent.Writable() {
			return nil, ErrNotFound
		}
		b, err = parent.CreateBucket(indexName)
		if err != nil {
			return nil, err
		}
	}

	return &UniqueIndex{
		IndexBucket: b,
		Parent:      parent,
	}, nil
}

// UniqueIndex is an index that references unique values and the corresponding ID.
type UniqueIndex struct {
	Parent      *bolt.Bucket
	IndexBucket *bolt.Bucket
}

// Add a value to the unique index
func (idx *UniqueIndex) Add(ctx context.Context, value []byte, targetID []byte) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	if value == nil || len(value) == 0 {
		return ErrNilParam
	}
	if targetID == nil || len(targetID) == 0 {
		return ErrNilParam
	}

	exists := idx.IndexBucket.Get(value)
	if exists != nil {
		if bytes.Equal(exists, targetID) {
			return nil
		}
		return ErrAlreadyExists
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	if err := idx.IndexBucket.Put(value, targetID); err != nil {
		return err
	}

	return checkContext(ctx)
}

// Remove a value from the unique index
func (idx *UniqueIndex) Remove(ctx context.Context, value []byte) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	if err := idx.IndexBucket.Delete(value); err != nil {
		return err
	}

	return checkContext(ctx)
}

// RemoveID removes an ID from the unique index
func (idx *UniqueIndex) RemoveID(ctx context.Context, id []byte) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	c := idx.IndexBucket.Cursor()

	for val, ident := c.First(); val != nil; val, ident = c.Next() {
		if err := checkContext(ctx); err != nil {
			return err
		}
		if bytes.Equal(ident, id) {
			return idx.Remove(ctx, val)
		}
	}
	return checkContext(ctx)
}

// Get the id corresponding to the given value
func (idx *UniqueIndex) Get(ctx context.Context, value []byte) ([]byte, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	raw := idx.IndexBucket.Get(value)

	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	return raw, nil
}

// All returns all the ids corresponding to the given value
func (idx *UniqueIndex) All(ctx context.Context, value []byte, opts *Options) ([][]byte, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	id := idx.IndexBucket.Get(value)
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	if id != nil {
		return [][]byte{id}, nil
	}

	return nil, nil
}

// AllRecords returns all the IDs of this index
func (idx *UniqueIndex) AllRecords(ctx context.Context, opts *Options) ([][]byte, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	var list [][]byte

	c := internal.Cursor{C: idx.IndexBucket.Cursor(), Reverse: opts != nil && opts.Reverse}

	for val, ident := c.First(); val != nil; val, ident = c.Next() {
		if err := checkContext(ctx); err != nil {
			return nil, err
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
		return nil, err
	}

	return list, nil
}

// Range returns the ids corresponding to the given range of values
func (idx *UniqueIndex) Range(ctx context.Context, min []byte, max []byte, opts *Options) ([][]byte, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
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

	for val, ident := c.First(); val != nil && c.Continue(val); val, ident = c.Next() {
		if err := checkContext(ctx); err != nil {
			return nil, err
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
		return nil, err
	}

	return list, nil
}

// Prefix returns the ids whose values have the given prefix.
func (idx *UniqueIndex) Prefix(ctx context.Context, prefix []byte, opts *Options) ([][]byte, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	var list [][]byte

	c := internal.PrefixCursor{
		C:       idx.IndexBucket.Cursor(),
		Reverse: opts != nil && opts.Reverse,
		Prefix:  prefix,
	}

	for val, ident := c.First(); val != nil && c.Continue(val); val, ident = c.Next() {
		if err := checkContext(ctx); err != nil {
			return nil, err
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
		return nil, err
	}

	return list, nil
}
