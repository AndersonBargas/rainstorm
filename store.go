package rainstorm

import (
	"bytes"
	"context"
	"reflect"

	"github.com/AndersonBargas/rainstorm/v6/index"
	"github.com/AndersonBargas/rainstorm/v6/q"
	bolt "go.etcd.io/bbolt"
)

// TypeStore stores user defined types in BoltDB.
type TypeStore interface {
	Finder
	// Init creates the indexes and buckets for a given structure
	Init(ctx context.Context, data any) error

	// ReIndex rebuilds all the indexes of a bucket
	ReIndex(ctx context.Context, data any) error

	// Save a structure
	Save(ctx context.Context, data any) error

	// Update a structure
	Update(ctx context.Context, data any) error

	// UpdateField updates a single field
	UpdateField(ctx context.Context, data any, fieldName string, value any) error

	// Drop a bucket
	Drop(ctx context.Context, data any) error

	// DeleteStruct deletes a structure from the associated bucket
	DeleteStruct(ctx context.Context, data any) error
}

// Init creates the indexes and buckets for a given structure
func (n *node) Init(ctx context.Context, data any) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	v := reflect.ValueOf(data)
	cfg, err := extract(&v)
	if err != nil {
		return err
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	return n.readWriteTx(ctx, func(tx *bolt.Tx) error {
		return n.init(ctx, tx, cfg)
	})
}

func (n *node) init(ctx context.Context, tx *bolt.Tx, cfg *structConfig) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	bucket, err := n.CreateBucketIfNotExists(tx, cfg.Name)
	if err != nil {
		return err
	}

	// save node configuration in the bucket
	_, err = newMeta(bucket, n)
	if err != nil {
		return err
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	for fieldName, fieldCfg := range cfg.Fields {
		if err := checkContext(ctx); err != nil {
			return err
		}
		if fieldCfg.Index == "" {
			continue
		}
		switch fieldCfg.Index {
		case tagID:
			_, err = index.NewIDIndex(bucket, []byte(indexPrefix+fieldName))
		case tagUniqueIdx:
			_, err = index.NewUniqueIndex(bucket, []byte(indexPrefix+fieldName))
		case tagIdx:
			_, err = index.NewListIndex(bucket, []byte(indexPrefix+fieldName))
		default:
			err = ErrIdxNotFound
		}

		if err != nil {
			return err
		}
		if err := checkContext(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (n *node) ReIndex(ctx context.Context, data any) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	ref := reflect.ValueOf(data)

	if !ref.IsValid() || ref.Kind() != reflect.Ptr || ref.Elem().Kind() != reflect.Struct {
		return ErrStructPtrNeeded
	}

	cfg, err := extract(&ref)
	if err != nil {
		return err
	}

	return n.readWriteTx(ctx, func(tx *bolt.Tx) error {
		return n.reIndex(ctx, tx, data, cfg)
	})
}

func (n *node) reIndex(ctx context.Context, tx *bolt.Tx, data interface{}, cfg *structConfig) error {
	root := n.WithTransaction(tx)
	nodes, err := root.From(cfg.Name).PrefixScan(ctx, indexPrefix)
	if err != nil {
		return err
	}
	bucket := root.GetBucket(tx, cfg.Name)
	if bucket == nil {
		return ErrNotFound
	}

	for _, node := range nodes {
		buckets := node.Bucket()
		name := buckets[len(buckets)-1]
		err := bucket.DeleteBucket([]byte(name))
		if err != nil {
			return err
		}
	}

	total, err := root.Count(ctx, data)
	if err != nil {
		return err
	}

	for i := 0; i < total; i++ {
		err = root.Select(q.True()).Skip(i).First(ctx, data)
		if err != nil {
			return err
		}

		err = root.Update(ctx, data)
		if err != nil {
			return err
		}
	}

	return nil
}

// Save a structure
func (n *node) Save(ctx context.Context, data any) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	ref := reflect.ValueOf(data)

	if !ref.IsValid() || ref.Kind() != reflect.Ptr || ref.Elem().Kind() != reflect.Struct {
		return ErrStructPtrNeeded
	}

	cfg, err := extract(&ref)
	if err != nil {
		return err
	}

	if cfg.ID.IsZero {
		if !cfg.ID.IsInteger || !cfg.ID.Increment {
			return ErrZeroID
		}
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	return n.readWriteTx(ctx, func(tx *bolt.Tx) error {
		return n.save(ctx, tx, cfg, data, false)
	})
}

func (n *node) save(ctx context.Context, tx *bolt.Tx, cfg *structConfig, data interface{}, update bool) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	bucket, err := n.CreateBucketIfNotExists(tx, cfg.Name)
	if err != nil {
		return err
	}

	// save node configuration in the bucket
	meta, err := newMeta(bucket, n)
	if err != nil {
		return err
	}
	if err := checkContext(ctx); err != nil {
		return err
	}

	if cfg.ID.IsZero {
		err = meta.increment(cfg.ID)
		if err != nil {
			return err
		}
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	id, err := toBytes(cfg.ID.Value.Interface(), n.codec)
	if err != nil {
		return err
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	for fieldName, fieldCfg := range cfg.Fields {
		if err := checkContext(ctx); err != nil {
			return err
		}

		if !update && !fieldCfg.IsID && fieldCfg.Increment && fieldCfg.IsInteger && fieldCfg.IsZero {
			err = meta.increment(fieldCfg)
			if err != nil {
				return err
			}
			if err := checkContext(ctx); err != nil {
				return err
			}
		}

		if fieldCfg.Index == "" {
			continue
		}

		idx, err := getIndex(bucket, fieldCfg.Index, fieldName)
		if err != nil {
			return err
		}
		if err := checkContext(ctx); err != nil {
			return err
		}

		if update && fieldCfg.IsZero && !fieldCfg.ForceUpdate {
			continue
		}

		if fieldCfg.IsZero {
			err = idx.RemoveID(id)
			if err != nil {
				return err
			}
			if err := checkContext(ctx); err != nil {
				return err
			}
			continue
		}

		value, err := toBytes(fieldCfg.Value.Interface(), n.codec)
		if err != nil {
			return err
		}

		if err := checkContext(ctx); err != nil {
			return err
		}

		var found bool
		idsSaved, err := idx.All(value, nil)
		if err != nil {
			return err
		}
		if err := checkContext(ctx); err != nil {
			return err
		}
		for _, idSaved := range idsSaved {
			if err := checkContext(ctx); err != nil {
				return err
			}
			if bytes.Equal(idSaved, id) {
				found = true
				break
			}
		}

		if found {
			continue
		}

		err = idx.RemoveID(id)
		if err != nil {
			return err
		}
		if err := checkContext(ctx); err != nil {
			return err
		}

		err = idx.Add(value, id)
		if err != nil {
			if err == index.ErrAlreadyExists {
				return ErrAlreadyExists
			}
			return err
		}
		if err := checkContext(ctx); err != nil {
			return err
		}
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	raw, err := n.codec.Marshal(data)
	if err != nil {
		return err
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	if err := bucket.Put(id, raw); err != nil {
		return err
	}
	return checkContext(ctx)
}

// Update a structure
func (n *node) Update(ctx context.Context, data any) error {
	return n.update(ctx, data, func(ref *reflect.Value, current *reflect.Value, cfg *structConfig) error {
		numfield := ref.NumField()
		for i := 0; i < numfield; i++ {
			if err := checkContext(ctx); err != nil {
				return err
			}
			f := ref.Field(i)
			if ref.Type().Field(i).PkgPath != "" {
				continue
			}
			zero := reflect.Zero(f.Type()).Interface()
			actual := f.Interface()
			if !reflect.DeepEqual(actual, zero) {
				cf := current.Field(i)
				cf.Set(f)
				idxInfo, ok := cfg.Fields[ref.Type().Field(i).Name]
				if ok {
					idxInfo.Value = &cf
				}
			}
		}
		return nil
	})
}

// UpdateField updates a single field
func (n *node) UpdateField(ctx context.Context, data any, fieldName string, value any) error {
	return n.update(ctx, data, func(ref *reflect.Value, current *reflect.Value, cfg *structConfig) error {
		f := current.FieldByName(fieldName)
		if !f.IsValid() {
			return ErrNotFound
		}
		tf, _ := current.Type().FieldByName(fieldName)
		if tf.PkgPath != "" {
			return ErrNotFound
		}
		v := reflect.ValueOf(value)
		if v.Kind() != f.Kind() {
			return ErrIncompatibleValue
		}
		if err := checkContext(ctx); err != nil {
			return err
		}
		f.Set(v)
		if err := checkContext(ctx); err != nil {
			return err
		}
		idxInfo, ok := cfg.Fields[fieldName]
		if ok {
			idxInfo.Value = &f
			idxInfo.IsZero = isZero(idxInfo.Value)
			idxInfo.ForceUpdate = true
		}
		return nil
	})
}

func (n *node) update(ctx context.Context, data interface{}, fn func(*reflect.Value, *reflect.Value, *structConfig) error) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	ref := reflect.ValueOf(data)
	if !ref.IsValid() || ref.Kind() != reflect.Ptr || ref.Elem().Kind() != reflect.Struct {
		return ErrStructPtrNeeded
	}

	cfg, err := extract(&ref)
	if err != nil {
		return err
	}

	if cfg.ID.IsZero {
		return ErrNoID
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	current := reflect.New(reflect.Indirect(ref).Type())

	return n.readWriteTx(ctx, func(tx *bolt.Tx) error {
		if err := checkContext(ctx); err != nil {
			return err
		}

		err = n.WithTransaction(tx).One(ctx, cfg.ID.Name, cfg.ID.Value.Interface(), current.Interface())
		if err != nil {
			return err
		}

		if err := checkContext(ctx); err != nil {
			return err
		}

		ref := reflect.ValueOf(data).Elem()
		cref := current.Elem()
		err = fn(&ref, &cref, cfg)
		if err != nil {
			return err
		}

		if err := checkContext(ctx); err != nil {
			return err
		}

		return n.save(ctx, tx, cfg, current.Interface(), true)
	})
}

// Drop a bucket
func (n *node) Drop(ctx context.Context, data any) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	var bucketName string

	v := reflect.ValueOf(data)
	if v.Kind() != reflect.String {
		info, err := extract(&v)
		if err != nil {
			return err
		}

		bucketName = info.Name
	} else {
		bucketName = v.Interface().(string)
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	return n.readWriteTx(ctx, func(tx *bolt.Tx) error {
		return n.drop(ctx, tx, bucketName)
	})
}

func (n *node) drop(ctx context.Context, tx *bolt.Tx, bucketName string) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	bucket := n.GetBucket(tx)
	var err error
	if bucket == nil {
		err = tx.DeleteBucket([]byte(bucketName))
	} else {
		err = bucket.DeleteBucket([]byte(bucketName))
	}
	if err != nil {
		return err
	}
	return checkContext(ctx)
}

// DeleteStruct deletes a structure from the associated bucket
func (n *node) DeleteStruct(ctx context.Context, data any) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	ref := reflect.ValueOf(data)

	if !ref.IsValid() || ref.Kind() != reflect.Ptr || ref.Elem().Kind() != reflect.Struct {
		return ErrStructPtrNeeded
	}

	cfg, err := extract(&ref)
	if err != nil {
		return err
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	id, err := toBytes(cfg.ID.Value.Interface(), n.codec)
	if err != nil {
		return err
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	return n.readWriteTx(ctx, func(tx *bolt.Tx) error {
		return n.deleteStruct(ctx, tx, cfg, id)
	})
}

func (n *node) deleteStruct(ctx context.Context, tx *bolt.Tx, cfg *structConfig, id []byte) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	bucket := n.GetBucket(tx, cfg.Name)
	if bucket == nil {
		return ErrNotFound
	}

	for fieldName, fieldCfg := range cfg.Fields {
		if err := checkContext(ctx); err != nil {
			return err
		}

		if fieldCfg.Index == "" {
			continue
		}

		idx, err := getIndex(bucket, fieldCfg.Index, fieldName)
		if err != nil {
			return err
		}
		if err := checkContext(ctx); err != nil {
			return err
		}

		err = idx.RemoveID(id)
		if err != nil {
			if err == index.ErrNotFound {
				return ErrNotFound
			}
			return err
		}
		if err := checkContext(ctx); err != nil {
			return err
		}
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	raw := bucket.Get(id)
	if raw == nil {
		return ErrNotFound
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	if err := bucket.Delete(id); err != nil {
		return err
	}
	return checkContext(ctx)
}
