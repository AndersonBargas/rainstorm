package bolt

import (
	"errors"
	"io"
	"os"
	"time"
)

// ErrTxClosed is returned when a transaction is committed or rolled back and the underlying transaction is already closed.
var ErrTxClosed = errors.New("tx closed")

// DB is the interface that wraps the basic BoltDB operations.
type DB interface {
	// Begin starts a new transaction.
	Begin(writable bool) (Tx, error)

	// Close releases all database resources.
	Close() error

	// Update executes a function within a managed read/write transaction.
	Update(fn func(Tx) error) error

	// View executes a function within a managed read-only transaction.
	View(fn func(Tx) error) error

	// Batch executes a function within a managed read/write transaction.
	Batch(fn func(Tx) error) error
}

// Tx is the interface that wraps the basic BoltDB transaction operations.
type Tx interface {
	// Bucket retrieves a bucket by name.
	Bucket(name []byte) Bucket

	// CreateBucket creates a new bucket.
	CreateBucket(name []byte) (Bucket, error)

	// CreateBucketIfNotExists creates a new bucket if it doesn't exist.
	CreateBucketIfNotExists(name []byte) (Bucket, error)

	// DeleteBucket deletes a bucket.
	DeleteBucket(name []byte) error

	// Commit writes all changes to disk.
	Commit() error

	// Rollback closes the transaction and ignores all previous updates.
	Rollback() error

	// Writable returns whether the transaction is writable.
	Writable() bool

	// Cursor creates a cursor associated with the root bucket.
	Cursor() Cursor
}

// Bucket is the interface that wraps the basic BoltDB bucket operations.
type Bucket interface {
	// Bucket retrieves a nested bucket by name.
	Bucket(name []byte) Bucket

	// CreateBucket creates a new nested bucket.
	CreateBucket(name []byte) (Bucket, error)

	// CreateBucketIfNotExists creates a new nested bucket if it doesn't exist.
	CreateBucketIfNotExists(name []byte) (Bucket, error)

	// DeleteBucket deletes a nested bucket.
	DeleteBucket(name []byte) error

	// Cursor creates a cursor associated with the bucket.
	Cursor() Cursor

	// Delete removes a key from the bucket.
	Delete(key []byte) error

	// Get retrieves the value for a key in the bucket.
	Get(key []byte) []byte

	// Put sets the value for a key in the bucket.
	Put(key []byte, value []byte) error

	// Tx returns the transaction associated with the bucket.
	Tx() Tx

	// Writable returns whether the bucket is writable.
	Writable() bool
}

// Cursor is the interface that wraps the basic BoltDB cursor operations.
type Cursor interface {
	// Bucket returns the bucket that this cursor was created from.
	Bucket() Bucket

	// Delete removes the current key from the bucket.
	Delete() error

	// First moves the cursor to the first item in the bucket and returns its key and value.
	First() (key []byte, value []byte)

	// Last moves the cursor to the last item in the bucket and returns its key and value.
	Last() (key []byte, value []byte)

	// Next moves the cursor to the next item in the bucket and returns its key and value.
	Next() (key []byte, value []byte)

	// Prev moves the cursor to the previous item in the bucket and returns its key and value.
	Prev() (key []byte, value []byte)

	// Seek moves the cursor to a given key and returns it.
	Seek(seek []byte) (key []byte, value []byte)
}

// Options options.
type Options struct {
	// Timeout is the amount of time to wait to obtain a file lock.
	// When set to zero it will wait indefinitely. This option is only
	// available on Darwin and Linux.
	Timeout time.Duration

	// NoGrowSync sets the DB.NoGrowSync flag before memory mapping the file.
	NoGrowSync bool

	// ReadOnly opens the database in read-only mode.
	ReadOnly bool

	// MmapFlags sets the DB.MmapFlags flag before memory mapping the file.
	MmapFlags int

	// InitialMmapSize is the initial mmap size of the database
	// in bytes. Read transactions won't block write transaction
	// if the InitialMmapSize is large enough to hold database mmap
	// size. (See DB.Begin for more information)
	//
	// If <=0, the initial map size is 0.
	// If initialMmapSize is smaller than the previous database size,
	// it takes effect in the next write transaction.
	InitialMmapSize int

	// PageSize overrides the default OS page size.
	PageSize int

	// NoSync sets the DB.NoSync flag before memory mapping the file.
	NoSync bool

	// OpenFile is a function used to open the file.
	// If nil, os.OpenFile is used.
	OpenFile func(string, int, os.FileMode) (*os.File, error)
}

// File file interface.
type File interface {
	io.Reader
	io.ReaderAt
	io.Writer
	io.WriterAt
	io.Closer
	Sync() error
	Truncate(size int64) error
	Stat() (os.FileInfo, error)
}
