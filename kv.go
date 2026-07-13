package rainstorm

import (
	"context"
	"reflect"

	bolt "go.etcd.io/bbolt"
)

// KeyValueStore can store and fetch values by key
type KeyValueStore interface {
	// Get a value from a bucket
	Get(ctx context.Context, bucketName string, key any, to any) error
	// Set a key/value pair into a bucket
	Set(ctx context.Context, bucketName string, key any, value any) error
	// Delete deletes a key from a bucket
	Delete(ctx context.Context, bucketName string, key any) error
	// GetBytes gets a raw value from a bucket.
	GetBytes(ctx context.Context, bucketName string, key any) ([]byte, error)
	// SetBytes sets a raw value into a bucket.
	SetBytes(ctx context.Context, bucketName string, key any, value []byte) error
	// KeyExists reports the presence of a key in a bucket.
	KeyExists(ctx context.Context, bucketName string, key any) (bool, error)
}

// GetBytes gets a raw value from a bucket.
func (n *node) GetBytes(ctx context.Context, bucketName string, key any) ([]byte, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	id, err := toBytes(key, n.codec)
	if err != nil {
		return nil, err
	}

	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	var val []byte
	err = n.readTx(ctx, func(tx *bolt.Tx) error {
		if err := checkContext(ctx); err != nil {
			return err
		}

		raw, err := n.getBytes(ctx, tx, bucketName, id)
		if err != nil {
			return err
		}

		if err := checkContext(ctx); err != nil {
			return err
		}

		val = make([]byte, len(raw))
		copy(val, raw)
		return checkContext(ctx)
	})
	if err != nil {
		return nil, err
	}

	return val, nil
}

func (n *node) getBytes(ctx context.Context, tx *bolt.Tx, bucketName string, id []byte) ([]byte, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	bucket := n.GetBucket(tx, bucketName)
	if bucket == nil {
		return nil, ErrNotFound
	}

	raw := bucket.Get(id)
	if raw == nil {
		return nil, ErrNotFound
	}
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	return raw, nil
}

// SetBytes sets a raw value into a bucket.
func (n *node) SetBytes(ctx context.Context, bucketName string, key any, value []byte) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	if key == nil {
		return ErrNilParam
	}

	id, err := toBytes(key, n.codec)
	if err != nil {
		return err
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	return n.readWriteTx(ctx, func(tx *bolt.Tx) error {
		return n.setBytes(ctx, tx, bucketName, id, value)
	})
}

func (n *node) setBytes(ctx context.Context, tx *bolt.Tx, bucketName string, id, data []byte) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	bucket, err := n.CreateBucketIfNotExists(tx, bucketName)
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

	if err := bucket.Put(id, data); err != nil {
		return err
	}
	return checkContext(ctx)
}

// Get a value from a bucket
func (n *node) Get(ctx context.Context, bucketName string, key any, to any) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	ref := reflect.ValueOf(to)

	if !ref.IsValid() || ref.Kind() != reflect.Ptr {
		return ErrPtrNeeded
	}

	id, err := toBytes(key, n.codec)
	if err != nil {
		return err
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	return n.readTx(ctx, func(tx *bolt.Tx) error {
		if err := checkContext(ctx); err != nil {
			return err
		}

		raw, err := n.getBytes(ctx, tx, bucketName, id)
		if err != nil {
			return err
		}

		if err := checkContext(ctx); err != nil {
			return err
		}

		// Decode into a temporary to preserve destination on error.
		temporary := reflect.New(ref.Elem().Type())
		if err := n.codec.Unmarshal(raw, temporary.Interface()); err != nil {
			return err
		}

		if err := checkContext(ctx); err != nil {
			return err
		}

		ref.Elem().Set(temporary.Elem())
		return nil
	})
}

// Set a key/value pair into a bucket
func (n *node) Set(ctx context.Context, bucketName string, key any, value any) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	var data []byte
	var err error
	if value != nil {
		if err := checkContext(ctx); err != nil {
			return err
		}
		data, err = n.codec.Marshal(value)
		if err != nil {
			return err
		}
		if err := checkContext(ctx); err != nil {
			return err
		}
	}

	return n.SetBytes(ctx, bucketName, key, data)
}

// Delete deletes a key from a bucket
func (n *node) Delete(ctx context.Context, bucketName string, key any) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	id, err := toBytes(key, n.codec)
	if err != nil {
		return err
	}

	if err := checkContext(ctx); err != nil {
		return err
	}

	return n.readWriteTx(ctx, func(tx *bolt.Tx) error {
		return n.delete(ctx, tx, bucketName, id)
	})
}

func (n *node) delete(ctx context.Context, tx *bolt.Tx, bucketName string, id []byte) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	bucket := n.GetBucket(tx, bucketName)
	if bucket == nil {
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

// KeyExists reports the presence of a key in a bucket.
func (n *node) KeyExists(ctx context.Context, bucketName string, key any) (bool, error) {
	if err := checkContext(ctx); err != nil {
		return false, err
	}

	id, err := toBytes(key, n.codec)
	if err != nil {
		return false, err
	}

	if err := checkContext(ctx); err != nil {
		return false, err
	}

	var exists bool
	err = n.readTx(ctx, func(tx *bolt.Tx) error {
		if err := checkContext(ctx); err != nil {
			return err
		}

		bucket := n.GetBucket(tx, bucketName)
		if bucket == nil {
			return ErrNotFound
		}

		v := bucket.Get(id)
		exists = v != nil

		if err := checkContext(ctx); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return false, err
	}

	return exists, nil
}
