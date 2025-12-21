package bbolt

import (
	"errors"
	"os"
	"time"

	"github.com/AndersonBargas/rainstorm/v6/bolt"
	bbolt "go.etcd.io/bbolt"
)

// New creates a new adaptor for the given bbolt.DB instance.
func New(db *bbolt.DB) *Adaptor {
	return &Adaptor{DB: db}
}

// Adaptor is a wrapper around bbolt.DB that satisfies the DB interface.
type Adaptor struct {
	*bbolt.DB
}

// Begin starts a new transaction.
func (a *Adaptor) Begin(writable bool) (bolt.Tx, error) {
	tx, err := a.DB.Begin(writable)
	if err != nil {
		return nil, err
	}
	return &TxWrapper{tx}, nil
}

// Update executes a function within a managed read/write transaction.
func (a *Adaptor) Update(fn func(bolt.Tx) error) error {
	return a.DB.Update(func(tx *bbolt.Tx) error {
		return fn(&TxWrapper{tx})
	})
}

// View executes a function within a managed read-only transaction.
func (a *Adaptor) View(fn func(bolt.Tx) error) error {
	return a.DB.View(func(tx *bbolt.Tx) error {
		return fn(&TxWrapper{tx})
	})
}

// Batch executes a function within a managed read/write transaction.
func (a *Adaptor) Batch(fn func(bolt.Tx) error) error {
	return a.DB.Batch(func(tx *bbolt.Tx) error {
		return fn(&TxWrapper{tx})
	})
}

// TxWrapper is a wrapper around bbolt.Tx that satisfies the Tx interface.
type TxWrapper struct {
	*bbolt.Tx
}

// Commit writes all changes to disk.
func (t *TxWrapper) Commit() error {
	err := t.Tx.Commit()
	if errors.Is(err, bbolt.ErrTxClosed) {
		return bolt.ErrTxClosed
	}
	return err
}

// Rollback closes the transaction and ignores all previous updates.
func (t *TxWrapper) Rollback() error {
	err := t.Tx.Rollback()
	if errors.Is(err, bbolt.ErrTxClosed) {
		return bolt.ErrTxClosed
	}
	return err
}

// Bucket retrieves a bucket by name.
func (t *TxWrapper) Bucket(name []byte) bolt.Bucket {
	b := t.Tx.Bucket(name)
	if b == nil {
		return nil
	}
	return &BucketWrapper{B: b}
}

// CreateBucket creates a new bucket.
func (t *TxWrapper) CreateBucket(name []byte) (bolt.Bucket, error) {
	b, err := t.Tx.CreateBucket(name)
	if err != nil {
		return nil, err
	}
	return &BucketWrapper{B: b}, nil
}

// CreateBucketIfNotExists creates a new bucket if it doesn't exist.
func (t *TxWrapper) CreateBucketIfNotExists(name []byte) (bolt.Bucket, error) {
	b, err := t.Tx.CreateBucketIfNotExists(name)
	if err != nil {
		return nil, err
	}
	return &BucketWrapper{B: b}, nil
}

// Cursor creates a cursor associated with the root bucket.
func (t *TxWrapper) Cursor() bolt.Cursor {
	return &CursorWrapper{t.Tx.Cursor()}
}

// BucketWrapper is a wrapper around bbolt.Bucket that satisfies the Bucket interface.
type BucketWrapper struct {
	B *bbolt.Bucket
}

// Bucket retrieves a nested bucket by name.
func (b *BucketWrapper) Bucket(name []byte) bolt.Bucket {
	nested := b.B.Bucket(name)
	if nested == nil {
		return nil
	}
	return &BucketWrapper{B: nested}
}

// CreateBucket creates a new nested bucket.
func (b *BucketWrapper) CreateBucket(name []byte) (bolt.Bucket, error) {
	nested, err := b.B.CreateBucket(name)
	if err != nil {
		return nil, err
	}
	return &BucketWrapper{B: nested}, nil
}

// CreateBucketIfNotExists creates a new nested bucket if it doesn't exist.
func (b *BucketWrapper) CreateBucketIfNotExists(name []byte) (bolt.Bucket, error) {
	nested, err := b.B.CreateBucketIfNotExists(name)
	if err != nil {
		return nil, err
	}
	return &BucketWrapper{B: nested}, nil
}

// Cursor creates a cursor associated with the bucket.
func (b *BucketWrapper) Cursor() bolt.Cursor {
	return &CursorWrapper{b.B.Cursor()}
}

// Tx returns the transaction associated with the bucket.
func (b *BucketWrapper) Tx() bolt.Tx {
	return &TxWrapper{b.B.Tx()}
}

// DeleteBucket deletes a nested bucket.
func (b *BucketWrapper) DeleteBucket(name []byte) error {
	return b.B.DeleteBucket(name)
}

// Delete removes a key from the bucket.
func (b *BucketWrapper) Delete(key []byte) error {
	return b.B.Delete(key)
}

// Get retrieves the value for a key in the bucket.
func (b *BucketWrapper) Get(key []byte) []byte {
	return b.B.Get(key)
}

// Put sets the value for a key in the bucket.
func (b *BucketWrapper) Put(key []byte, value []byte) error {
	return b.B.Put(key, value)
}

// Writable returns whether the bucket is writable.
func (b *BucketWrapper) Writable() bool {
	return b.B.Writable()
}

// CursorWrapper is a wrapper around bbolt.Cursor that satisfies the Cursor interface.
type CursorWrapper struct {
	*bbolt.Cursor
}

// Bucket returns the bucket that this cursor was created from.
func (c *CursorWrapper) Bucket() bolt.Bucket {
	return &BucketWrapper{B: c.Cursor.Bucket()}
}

// Open opens a database at the given path.
func Open(path string, mode os.FileMode, options *bolt.Options) (*Adaptor, error) {
	var opts *bbolt.Options

	if options != nil {
		opts = &bbolt.Options{
			Timeout:         options.Timeout,
			NoGrowSync:      options.NoGrowSync,
			ReadOnly:        options.ReadOnly,
			MmapFlags:       options.MmapFlags,
			InitialMmapSize: options.InitialMmapSize,
			PageSize:        options.PageSize,
			NoSync:          options.NoSync,
			OpenFile:        options.OpenFile,
		}
	} else {
		opts = &bbolt.Options{Timeout: 1 * time.Second}
	}

	db, err := bbolt.Open(path, mode, opts)
	if err != nil {
		return nil, err
	}

	return &Adaptor{db}, nil
}
