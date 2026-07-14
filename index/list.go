package index

import (
	"bytes"
	"context"

	"github.com/AndersonBargas/rainstorm/v6/internal"
	bolt "go.etcd.io/bbolt"
)

// NewListIndex loads a ListIndex
func NewListIndex(parent *bolt.Bucket, indexName []byte) (*ListIndex, error) {
	if parent == nil || len(indexName) == 0 {
		return nil, wrapError("index new list", ErrNilParam)
	}
	var err error
	b := parent.Bucket(indexName)
	if b == nil {
		if !parent.Writable() {
			return nil, wrapError("index new list", ErrNotFound)
		}
		b, err = parent.CreateBucket(indexName)
		if err != nil {
			return nil, wrapError("index new list", err)
		}
	}

	ids, err := NewUniqueIndex(b, []byte("rainstorm__ids"))
	if err != nil {
		return nil, wrapError("index new list", err)
	}

	return &ListIndex{
		IndexBucket: b,
		Parent:      parent,
		IDs:         ids,
	}, nil
}

// ListIndex is an index that references values and the corresponding IDs.
type ListIndex struct {
	Parent      *bolt.Bucket
	IndexBucket *bolt.Bucket
	IDs         *UniqueIndex
}

// Add a value to the list index
func (idx *ListIndex) Add(ctx context.Context, newValue []byte, targetID []byte) error {
	if err := checkContext(ctx); err != nil {
		return wrapError("index add", err)
	}
	if newValue == nil || len(newValue) == 0 {
		return wrapError("index add", ErrNilParam)
	}
	if targetID == nil || len(targetID) == 0 {
		return wrapError("index add", ErrNilParam)
	}

	key, err := idx.IDs.Get(ctx, targetID)
	if err != nil {
		return wrapError("index add", err)
	}

	if err := checkContext(ctx); err != nil {
		return wrapError("index add", err)
	}

	if key != nil {
		err := idx.IndexBucket.Delete(key)
		if err != nil {
			return wrapError("index add", err)
		}

		if err := checkContext(ctx); err != nil {
			return wrapError("index add", err)
		}

		err = idx.IDs.Remove(ctx, targetID)
		if err != nil {
			return wrapError("index add", err)
		}

		if err := checkContext(ctx); err != nil {
			return wrapError("index add", err)
		}

		key = key[:0]
	}

	key = append(key, newValue...)
	key = append(key, '_')
	key = append(key, '_')
	key = append(key, targetID...)

	err = idx.IDs.Add(ctx, targetID, key)
	if err != nil {
		return wrapError("index add", err)
	}

	if err := checkContext(ctx); err != nil {
		return wrapError("index add", err)
	}

	if err := idx.IndexBucket.Put(key, targetID); err != nil {
		return wrapError("index add", err)
	}

	return wrapError("index add", checkContext(ctx))
}

// Remove a value from the unique index
func (idx *ListIndex) Remove(ctx context.Context, value []byte) error {
	if err := checkContext(ctx); err != nil {
		return wrapError("index remove", err)
	}

	var err error
	var keys [][]byte

	c := idx.IndexBucket.Cursor()
	prefix := generatePrefix(value)

	for k, _ := c.Seek(prefix); bytes.HasPrefix(k, prefix); k, _ = c.Next() {
		if err := checkContext(ctx); err != nil {
			return wrapError("index remove", err)
		}
		// Defensive copy: cursor buffer is not stable across mutations.
		keyCopy := make([]byte, len(k))
		copy(keyCopy, k)
		keys = append(keys, keyCopy)
	}

	for _, k := range keys {
		if err := checkContext(ctx); err != nil {
			return wrapError("index remove", err)
		}
		err = idx.IndexBucket.Delete(k)
		if err != nil {
			return wrapError("index remove", err)
		}
		if err := checkContext(ctx); err != nil {
			return wrapError("index remove", err)
		}
	}

	if err := checkContext(ctx); err != nil {
		return wrapError("index remove", err)
	}

	err = idx.IDs.RemoveID(ctx, value)
	if err != nil {
		return wrapError("index remove", err)
	}

	return wrapError("index remove", checkContext(ctx))
}

// RemoveID removes an ID from the list index
func (idx *ListIndex) RemoveID(ctx context.Context, targetID []byte) error {
	if err := checkContext(ctx); err != nil {
		return wrapError("index remove id", err)
	}

	value, err := idx.IDs.Get(ctx, targetID)
	if err != nil {
		return wrapError("index remove id", err)
	}
	if value == nil {
		return nil
	}

	if err := checkContext(ctx); err != nil {
		return wrapError("index remove id", err)
	}

	err = idx.IndexBucket.Delete(value)
	if err != nil {
		return wrapError("index remove id", err)
	}

	if err := checkContext(ctx); err != nil {
		return wrapError("index remove id", err)
	}

	err = idx.IDs.Remove(ctx, targetID)
	if err != nil {
		return wrapError("index remove id", err)
	}

	return wrapError("index remove id", checkContext(ctx))
}

// Get the first ID corresponding to the given value
func (idx *ListIndex) Get(ctx context.Context, value []byte) ([]byte, error) {
	if err := checkContext(ctx); err != nil {
		return nil, wrapError("index get", err)
	}

	c := idx.IndexBucket.Cursor()
	prefix := generatePrefix(value)

	for k, id := c.Seek(prefix); bytes.HasPrefix(k, prefix); k, id = c.Next() {
		if err := checkContext(ctx); err != nil {
			return nil, wrapError("index get", err)
		}
		return id, nil
	}

	if err := checkContext(ctx); err != nil {
		return nil, wrapError("index get", err)
	}

	return nil, nil
}

// All the IDs corresponding to the given value
func (idx *ListIndex) All(ctx context.Context, value []byte, opts *Options) ([][]byte, error) {
	if err := checkContext(ctx); err != nil {
		return nil, wrapError("index all", err)
	}

	var list [][]byte
	c := idx.IndexBucket.Cursor()
	cur := internal.Cursor{C: c, Reverse: opts != nil && opts.Reverse}

	prefix := generatePrefix(value)

	k, id := c.Seek(prefix)
	if cur.Reverse {
		var count int
		kc := k
		idc := id
		for ; kc != nil && bytes.HasPrefix(kc, prefix); kc, idc = c.Next() {
			if err := checkContext(ctx); err != nil {
				return nil, wrapError("index all", err)
			}
			count++
			k, id = kc, idc
		}
		if kc != nil {
			k, id = c.Prev()
		}
		list = make([][]byte, 0, count)
	}

	for ; bytes.HasPrefix(k, prefix); k, id = cur.Next() {
		if err := checkContext(ctx); err != nil {
			return nil, wrapError("index all", err)
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

		list = append(list, id)
	}

	if err := checkContext(ctx); err != nil {
		return nil, wrapError("index all", err)
	}

	return list, nil
}

// AllRecords returns all the IDs of this index
func (idx *ListIndex) AllRecords(ctx context.Context, opts *Options) ([][]byte, error) {
	if err := checkContext(ctx); err != nil {
		return nil, wrapError("index all records", err)
	}

	var list [][]byte

	c := internal.Cursor{C: idx.IndexBucket.Cursor(), Reverse: opts != nil && opts.Reverse}

	for k, id := c.First(); k != nil; k, id = c.Next() {
		if err := checkContext(ctx); err != nil {
			return nil, wrapError("index all records", err)
		}

		if id == nil || bytes.Equal(k, []byte("rainstorm__ids")) {
			continue
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

		list = append(list, id)
	}

	if err := checkContext(ctx); err != nil {
		return nil, wrapError("index all records", err)
	}

	return list, nil
}

// Range returns the ids corresponding to the given range of values
func (idx *ListIndex) Range(ctx context.Context, min []byte, max []byte, opts *Options) ([][]byte, error) {
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
			pos := bytes.LastIndex(val, []byte("__"))
			return bytes.Compare(val[:pos], limit)
		},
	}

	for k, id := c.First(); c.Continue(k); k, id = c.Next() {
		if err := checkContext(ctx); err != nil {
			return nil, wrapError("index range", err)
		}

		if id == nil || bytes.Equal(k, []byte("rainstorm__ids")) {
			continue
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

		list = append(list, id)
	}

	if err := checkContext(ctx); err != nil {
		return nil, wrapError("index range", err)
	}

	return list, nil
}

// Prefix returns the ids whose values have the given prefix.
func (idx *ListIndex) Prefix(ctx context.Context, prefix []byte, opts *Options) ([][]byte, error) {
	if err := checkContext(ctx); err != nil {
		return nil, wrapError("index prefix", err)
	}

	var list [][]byte

	c := internal.PrefixCursor{
		C:       idx.IndexBucket.Cursor(),
		Reverse: opts != nil && opts.Reverse,
		Prefix:  prefix,
	}

	for k, id := c.First(); k != nil && c.Continue(k); k, id = c.Next() {
		if err := checkContext(ctx); err != nil {
			return nil, wrapError("index prefix", err)
		}

		if id == nil || bytes.Equal(k, []byte("rainstorm__ids")) {
			continue
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

		list = append(list, id)
	}

	if err := checkContext(ctx); err != nil {
		return nil, wrapError("index prefix", err)
	}

	return list, nil
}

func generatePrefix(value []byte) []byte {
	prefix := make([]byte, len(value)+2)
	var i int
	for i = range value {
		prefix[i] = value[i]
	}
	prefix[i+1] = '_'
	prefix[i+2] = '_'
	return prefix
}
