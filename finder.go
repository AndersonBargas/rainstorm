package rainstorm

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/AndersonBargas/rainstorm/v6/index"
	"github.com/AndersonBargas/rainstorm/v6/q"
	bolt "go.etcd.io/bbolt"
)

// Finder retrieves records from Rainstorm buckets.
type Finder interface {
	// One returns one record by the specified index
	One(ctx context.Context, fieldName string, value any, to any) error

	// Find returns one or more records by the specified index
	Find(ctx context.Context, fieldName string, value any, to any, options ...FindOption) error

	// AllByIndex gets all the records of a bucket that are indexed in the specified index
	AllByIndex(ctx context.Context, fieldName string, to any, options ...FindOption) error

	// All gets all the records of a bucket.
	// If there are no records it returns no error and the 'to' parameter is set to an empty slice.
	All(ctx context.Context, to any, options ...FindOption) error

	// Select a list of records that match a list of matchers. Doesn't use indexes.
	Select(matchers ...q.Matcher) Query

	// Range returns one or more records by the specified index within the specified range
	Range(ctx context.Context, fieldName string, min any, max any, to any, options ...FindOption) error

	// Prefix returns one or more records whose given field starts with the specified prefix.
	Prefix(ctx context.Context, fieldName string, prefix string, to any, options ...FindOption) error

	// Count counts all the records of a bucket
	Count(ctx context.Context, data any) (int, error)
}

// resolveBucketName returns the bucket name, falling back to the
// innermost rootBucket if the sink has no name (anonymous type).
func (n *node) resolveBucketName(sink sink) string {
	name := sink.bucketName()
	if name == "" && len(n.rootBucket) > 0 {
		// Return empty to signal that the root bucket itself is the data bucket.
		// GetBucket/CreateBucketIfNotExists will use only the rootBucket path.
		return ""
	}
	return name
}

// hasBucketName returns true if a usable bucket name can be determined.
func (n *node) hasBucketName(sink sink) bool {
	if sink.bucketName() != "" {
		return true
	}
	return len(n.rootBucket) > 0
}

// One returns one record by the specified index
func (n *node) One(ctx context.Context, fieldName string, value any, to any) error {
	if err := checkContext(ctx); err != nil {
		return wrapError("one", err)
	}

	sink, err := newFirstSink(n, to)
	if err != nil {
		return wrapError("one", err)
	}

	if !n.hasBucketName(sink) {
		return wrapError("one", ErrNoName)
	}

	bucketName := n.resolveBucketName(sink)

	if fieldName == "" {
		return wrapError("one", ErrNotFound)
	}

	ref := reflect.Indirect(sink.ref)
	cfg, err := extractSingleField(&ref, fieldName)
	if err != nil {
		return wrapError("one", err)
	}

	field, ok := cfg.Fields[fieldName]
	if !ok || (!field.IsID && field.Index == "") {
		query := newQuery(n, q.StrictEq(fieldName, value))
		query.Limit(1)

		err = n.readTx(ctx, func(tx *bolt.Tx) error {
			return query.query(ctx, tx, sink)
		})

		if err != nil {
			return wrapError("one", err)
		}

		return wrapError("one", sink.flush(ctx))
	}

	val, err := toBytes(value, n.codec)
	if err != nil {
		return wrapError("one", err)
	}

	if err := checkContext(ctx); err != nil {
		return wrapError("one", err)
	}

	return wrapError("one", n.readTx(ctx, func(tx *bolt.Tx) error {
		return n.one(ctx, tx, bucketName, fieldName, cfg, to, val, field.IsID)
	}))
}

func (n *node) one(ctx context.Context, tx *bolt.Tx, bucketName, fieldName string, cfg *structConfig, to interface{}, val []byte, skipIndex bool) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	bucket := n.getBucket(tx, bucketName)
	if bucket == nil {
		return ErrNotFound
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	var id []byte
	if !skipIndex {
		idx, err := getIndex(bucket, cfg.Fields[fieldName].Index, fieldName)
		if err != nil {
			if errors.Is(err, index.ErrNotFound) {
				return ErrNotFound
			}
			return err
		}

		if err := checkContext(ctx); err != nil {
			return err
		}

		var gErr error
		id, gErr = idx.Get(ctx, val)
		if gErr != nil {
			return gErr
		}

		if err := checkContext(ctx); err != nil {
			return err
		}
	} else {
		id = val
		if err := checkContext(ctx); err != nil {
			return err
		}
	}

	if id == nil {
		return ErrNotFound
	}

	raw := bucket.Get(id)
	if raw == nil {
		return ErrNotFound
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	// Decode into a temporary so the caller's destination is preserved on
	// codec error or cancellation.
	destination := reflect.ValueOf(to)
	temporary := reflect.New(destination.Elem().Type())

	if err := n.codec.Unmarshal(raw, temporary.Interface()); err != nil {
		return err
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	destination.Elem().Set(temporary.Elem())
	return nil
}

// Find returns one or more records by the specified index
func (n *node) Find(ctx context.Context, fieldName string, value any, to any, options ...FindOption) error {
	if err := checkContext(ctx); err != nil {
		return wrapError("find", err)
	}

	sink, err := newListSink(n, to)
	if err != nil {
		return wrapError("find", err)
	}
	if !n.hasBucketName(sink) {
		return wrapError("find", ErrNoName)
	}
	bucketName := n.resolveBucketName(sink)

	ref := reflect.Indirect(reflect.New(sink.elemType))
	cfg, err := extractSingleField(&ref, fieldName)
	if err != nil {
		return wrapError("find", err)
	}

	opts := index.NewOptions()
	for _, fn := range options {
		fn(opts)
	}

	if err := checkContext(ctx); err != nil {
		return wrapError("find", err)
	}

	field, ok := cfg.Fields[fieldName]
	if !ok || (!field.IsID && (field.Index == "" || value == nil)) {
		query := newQuery(n, q.Eq(fieldName, value))
		query.Skip(opts.Skip).Limit(opts.Limit)

		if opts.Reverse {
			query.Reverse()
		}

		err = n.readTx(ctx, func(tx *bolt.Tx) error {
			return query.query(ctx, tx, sink)
		})

		if err != nil {
			return wrapError("find", err)
		}

		return wrapError("find", sink.flush(ctx))
	}

	val, err := toBytes(value, n.codec)
	if err != nil {
		return wrapError("find", err)
	}

	if err := checkContext(ctx); err != nil {
		return wrapError("find", err)
	}

	return wrapError("find", n.readTx(ctx, func(tx *bolt.Tx) error {
		return n.find(ctx, tx, bucketName, fieldName, cfg, sink, val, opts)
	}))
}

func (n *node) find(ctx context.Context, tx *bolt.Tx, bucketName, fieldName string, cfg *structConfig, sink *listSink, val []byte, opts *index.Options) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	bucket := n.getBucket(tx, bucketName)
	if bucket == nil {
		return ErrNotFound
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	idx, err := getIndex(bucket, cfg.Fields[fieldName].Index, fieldName)
	if err != nil {
		return err
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	list, err := idx.All(ctx, val, opts)
	if err != nil {
		if errors.Is(err, index.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	sink.results = reflect.MakeSlice(reflect.Indirect(sink.ref).Type(), len(list), len(list))

	sorter := newSorter(n, sink)
	for i := range list {
		if err := checkContext(ctx); err != nil {
			return err
		}

		raw := bucket.Get(list[i])
		if raw == nil {
			return ErrNotFound
		}

		if err := checkContext(ctx); err != nil {
			return err
		}

		if _, err := sorter.filter(ctx, nil, bucket, list[i], raw); err != nil {
			return err
		}

		if err := checkContext(ctx); err != nil {
			return err
		}
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	return sorter.flush(ctx)
}

// AllByIndex gets all the records of a bucket that are indexed in the specified index
func (n *node) AllByIndex(ctx context.Context, fieldName string, to any, options ...FindOption) error {
	if err := checkContext(ctx); err != nil {
		return wrapError("all by index", err)
	}

	if fieldName == "" {
		return wrapError("all by index", n.All(ctx, to, options...))
	}

	ref := reflect.ValueOf(to)

	if ref.Kind() != reflect.Ptr || ref.Elem().Kind() != reflect.Slice {
		return wrapError("all by index", ErrSlicePtrNeeded)
	}

	typ := reflect.Indirect(ref).Type().Elem()

	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	newElem := reflect.New(typ)

	cfg, err := extract(&newElem)
	if err != nil {
		return wrapError("all by index", err)
	}

	if cfg.ID.Name == fieldName {
		return wrapError("all by index", n.All(ctx, to, options...))
	}

	opts := index.NewOptions()
	for _, fn := range options {
		fn(opts)
	}

	if err := checkContext(ctx); err != nil {
		return wrapError("all by index", err)
	}

	return wrapError("all by index", n.readTx(ctx, func(tx *bolt.Tx) error {
		return n.allByIndex(ctx, tx, fieldName, cfg, &ref, opts)
	}))
}

func (n *node) allByIndex(ctx context.Context, tx *bolt.Tx, fieldName string, cfg *structConfig, ref *reflect.Value, opts *index.Options) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	bucket := n.getBucket(tx, cfg.Name)
	if bucket == nil {
		return ErrNotFound
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	fieldCfg, ok := cfg.Fields[fieldName]
	if !ok {
		return ErrNotFound
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	idx, err := getIndex(bucket, fieldCfg.Index, fieldName)
	if err != nil {
		return err
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	list, err := idx.AllRecords(ctx, opts)
	if err != nil {
		if errors.Is(err, index.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	results := reflect.MakeSlice(reflect.Indirect(*ref).Type(), len(list), len(list))

	for i := range list {
		if err := checkContext(ctx); err != nil {
			return err
		}

		raw := bucket.Get(list[i])
		if raw == nil {
			return ErrNotFound
		}

		if err := checkContext(ctx); err != nil {
			return err
		}

		err = n.codec.Unmarshal(raw, results.Index(i).Addr().Interface())
		if err != nil {
			return err
		}

		if err := checkContext(ctx); err != nil {
			return err
		}
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	reflect.Indirect(*ref).Set(results)
	return nil
}

// All gets all the records of a bucket.
// If there are no records it returns no error and the 'to' parameter is set to an empty slice.
func (n *node) All(ctx context.Context, to any, options ...FindOption) error {
	if err := checkContext(ctx); err != nil {
		return wrapError("all", err)
	}

	opts := index.NewOptions()
	for _, fn := range options {
		fn(opts)
	}

	if err := checkContext(ctx); err != nil {
		return wrapError("all", err)
	}

	query := newQuery(n, nil).Limit(opts.Limit).Skip(opts.Skip)
	if opts.Reverse {
		query.Reverse()
	}

	err := query.Find(ctx, to)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return wrapError("all", err)
	}

	if errors.Is(err, ErrNotFound) {
		ref := reflect.ValueOf(to)
		results := reflect.MakeSlice(reflect.Indirect(ref).Type(), 0, 0)
		reflect.Indirect(ref).Set(results)
	}
	return nil
}

// Range returns one or more records by the specified index within the specified range
func (n *node) Range(ctx context.Context, fieldName string, min any, max any, to any, options ...FindOption) error {
	if err := checkContext(ctx); err != nil {
		return wrapError("range", err)
	}

	sink, err := newListSink(n, to)
	if err != nil {
		return wrapError("range", err)
	}

	if !n.hasBucketName(sink) {
		return wrapError("range", ErrNoName)
	}

	bucketName := n.resolveBucketName(sink)

	ref := reflect.Indirect(reflect.New(sink.elemType))
	cfg, err := extractSingleField(&ref, fieldName)
	if err != nil {
		return wrapError("range", err)
	}

	opts := index.NewOptions()
	for _, fn := range options {
		fn(opts)
	}

	if err := checkContext(ctx); err != nil {
		return wrapError("range", err)
	}

	field, ok := cfg.Fields[fieldName]
	if !ok || (!field.IsID && field.Index == "") {
		query := newQuery(n, q.And(q.Gte(fieldName, min), q.Lte(fieldName, max)))
		query.Skip(opts.Skip).Limit(opts.Limit)

		if opts.Reverse {
			query.Reverse()
		}

		err = n.readTx(ctx, func(tx *bolt.Tx) error {
			return query.query(ctx, tx, sink)
		})

		if err != nil {
			return wrapError("range", err)
		}

		return wrapError("range", sink.flush(ctx))
	}

	mn, err := toBytes(min, n.codec)
	if err != nil {
		return wrapError("range", err)
	}

	if err := checkContext(ctx); err != nil {
		return wrapError("range", err)
	}

	mx, err := toBytes(max, n.codec)
	if err != nil {
		return wrapError("range", err)
	}

	if err := checkContext(ctx); err != nil {
		return wrapError("range", err)
	}

	return wrapError("range", n.readTx(ctx, func(tx *bolt.Tx) error {
		return n.rnge(ctx, tx, bucketName, fieldName, cfg, sink, mn, mx, opts)
	}))
}

func (n *node) rnge(ctx context.Context, tx *bolt.Tx, bucketName, fieldName string, cfg *structConfig, sink *listSink, min, max []byte, opts *index.Options) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	bucket := n.getBucket(tx, bucketName)
	if bucket == nil {
		if err := checkContext(ctx); err != nil {
			return err
		}
		reflect.Indirect(sink.ref).SetLen(0)
		return nil
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	idx, err := getIndex(bucket, cfg.Fields[fieldName].Index, fieldName)
	if err != nil {
		return err
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	list, err := idx.Range(ctx, min, max, opts)
	if err != nil {
		return err
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	sink.results = reflect.MakeSlice(reflect.Indirect(sink.ref).Type(), len(list), len(list))
	sorter := newSorter(n, sink)
	for i := range list {
		if err := checkContext(ctx); err != nil {
			return err
		}

		raw := bucket.Get(list[i])
		if raw == nil {
			return ErrNotFound
		}

		if err := checkContext(ctx); err != nil {
			return err
		}

		if _, err := sorter.filter(ctx, nil, bucket, list[i], raw); err != nil {
			return err
		}

		if err := checkContext(ctx); err != nil {
			return err
		}
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	return sorter.flush(ctx)
}

// Prefix returns one or more records whose given field starts with the specified prefix.
func (n *node) Prefix(ctx context.Context, fieldName string, prefix string, to any, options ...FindOption) error {
	if err := checkContext(ctx); err != nil {
		return wrapError("prefix", err)
	}

	sink, err := newListSink(n, to)
	if err != nil {
		return wrapError("prefix", err)
	}

	if !n.hasBucketName(sink) {
		return wrapError("prefix", ErrNoName)
	}

	bucketName := n.resolveBucketName(sink)

	ref := reflect.Indirect(reflect.New(sink.elemType))
	cfg, err := extractSingleField(&ref, fieldName)
	if err != nil {
		return wrapError("prefix", err)
	}

	opts := index.NewOptions()
	for _, fn := range options {
		fn(opts)
	}

	if err := checkContext(ctx); err != nil {
		return wrapError("prefix", err)
	}

	field, ok := cfg.Fields[fieldName]
	if !ok || (!field.IsID && field.Index == "") {
		query := newQuery(n, q.Re(fieldName, fmt.Sprintf("^%s", prefix)))
		query.Skip(opts.Skip).Limit(opts.Limit)

		if opts.Reverse {
			query.Reverse()
		}

		err = n.readTx(ctx, func(tx *bolt.Tx) error {
			return query.query(ctx, tx, sink)
		})

		if err != nil {
			return wrapError("prefix", err)
		}

		return wrapError("prefix", sink.flush(ctx))
	}

	prfx, err := toBytes(prefix, n.codec)
	if err != nil {
		return wrapError("prefix", err)
	}

	if err := checkContext(ctx); err != nil {
		return wrapError("prefix", err)
	}

	return wrapError("prefix", n.readTx(ctx, func(tx *bolt.Tx) error {
		return n.prefix(ctx, tx, bucketName, fieldName, cfg, sink, prfx, opts)
	}))
}

func (n *node) prefix(ctx context.Context, tx *bolt.Tx, bucketName, fieldName string, cfg *structConfig, sink *listSink, prefix []byte, opts *index.Options) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	bucket := n.getBucket(tx, bucketName)
	if bucket == nil {
		if err := checkContext(ctx); err != nil {
			return err
		}
		reflect.Indirect(sink.ref).SetLen(0)
		return nil
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	idx, err := getIndex(bucket, cfg.Fields[fieldName].Index, fieldName)
	if err != nil {
		return err
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	list, err := idx.Prefix(ctx, prefix, opts)
	if err != nil {
		return err
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	sink.results = reflect.MakeSlice(reflect.Indirect(sink.ref).Type(), len(list), len(list))
	sorter := newSorter(n, sink)
	for i := range list {
		if err := checkContext(ctx); err != nil {
			return err
		}

		raw := bucket.Get(list[i])
		if raw == nil {
			return ErrNotFound
		}

		if err := checkContext(ctx); err != nil {
			return err
		}

		if _, err := sorter.filter(ctx, nil, bucket, list[i], raw); err != nil {
			return err
		}

		if err := checkContext(ctx); err != nil {
			return err
		}
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	return sorter.flush(ctx)
}

// Count counts all the records of a bucket
func (n *node) Count(ctx context.Context, data any) (int, error) {
	count, err := n.Select().Count(ctx, data)
	if err != nil {
		return 0, wrapError("count", err)
	}
	return count, nil
}
