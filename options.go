package rainstorm

import (
	"context"
	"os"

	"github.com/AndersonBargas/rainstorm/v6/codec"
	"github.com/AndersonBargas/rainstorm/v6/index"
	bolt "go.etcd.io/bbolt"
)

// OpenOption customizes the way Rainstorm opens a database.
type OpenOption func(*Options) error

// FindOption customizes a finder query.
type FindOption func(*index.Options)

// BoltOptions used to pass options to BoltDB.
func BoltOptions(mode os.FileMode, options *bolt.Options) OpenOption {
	return func(opts *Options) error {
		opts.boltMode = mode
		opts.boltOptions = options
		return nil
	}
}

// Codec used to set a custom encoder and decoder. The default is JSON.
func Codec(c codec.MarshalUnmarshaler) OpenOption {
	return func(opts *Options) error {
		opts.codec = c
		return nil
	}
}

// Root used to set the root bucket. See also the From method.
func Root(root ...string) OpenOption {
	path := cloneBucketPath(root)
	return func(opts *Options) error {
		opts.rootBucket = cloneBucketPath(path)
		return nil
	}
}

// UseDB allows Rainstorm to use an existing open Bolt.DB.
// Warning: rainstorm.DB.Close() will close the bolt.DB instance.
func UseDB(b *bolt.DB) OpenOption {
	return func(opts *Options) error {
		opts.path = b.Path()
		opts.bolt = b
		return nil
	}
}

// Limit sets the maximum number of records to return
func Limit(limit int) FindOption {
	return func(opts *index.Options) {
		opts.Limit = limit
	}
}

// Skip sets the number of records to skip
func Skip(offset int) FindOption {
	return func(opts *index.Options) {
		opts.Skip = offset
	}
}

// Reverse will return the results in descending order
func Reverse() FindOption {
	return func(opts *index.Options) {
		opts.Reverse = true
	}
}

// Options are used to customize the way Rainstorm opens a database.
type Options struct {
	// Handles encoding and decoding of objects
	codec codec.MarshalUnmarshaler

	// Bolt file mode
	boltMode os.FileMode

	// Bolt options
	boltOptions *bolt.Options

	// The root bucket name
	rootBucket []string

	// Path of the database file
	path string

	// Bolt is still easily accessible
	bolt *bolt.DB

	// postOpenHook is a deterministic, package-private test seam invoked after
	// Rainstorm opens an owned database and before initialization continues.
	postOpenHook func(context.Context)
}
