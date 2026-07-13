package rainstorm

import (
	"context"

	"github.com/AndersonBargas/rainstorm/v6/codec"
	bolt "go.etcd.io/bbolt"
)

// A Node in Rainstorm represents the API to a BoltDB bucket.
type Node interface {
	TypeStore
	KeyValueStore
	BucketScanner

	// From returns a new Rainstorm node with a new bucket root below the current.
	// All DB operations on the new node will be executed relative to this bucket.
	From(addend ...string) Node

	// Bucket returns the bucket name as a slice from the root.
	// In the normal, simple case this will be empty.
	Bucket() []string

	// Codec used by this instance of Rainstorm
	Codec() codec.MarshalUnmarshaler

	// WithCodec returns a New Rainstorm Node that will use the given Codec.
	WithCodec(codec codec.MarshalUnmarshaler) Node
}

// Compile-time assertion that *node implements the final Node interface.
var _ Node = (*node)(nil)

// A Node in Rainstorm represents the API to a BoltDB bucket.
type node struct {
	s *DB

	// The root bucket. In the normal, simple case this will be empty.
	rootBucket []string

	// Transaction object. Nil if not in transaction
	tx *bolt.Tx

	// Codec of this node
	codec codec.MarshalUnmarshaler
}

// cloneBucketPath returns a defensive copy of the given path slice.
// nil is preserved as nil; []string{} is preserved as an empty non-nil slice.
func cloneBucketPath(path []string) []string {
	if path == nil {
		return nil
	}

	cloned := make([]string, len(path))
	copy(cloned, path)
	return cloned
}

// From returns a new Rainstorm Node with a new bucket root below the current.
// All DB operations on the new node will be executed relative to this bucket.
func (n node) From(addend ...string) Node {
	path := make([]string, 0, len(n.rootBucket)+len(addend))
	path = append(path, n.rootBucket...)
	path = append(path, addend...)
	n.rootBucket = path
	return &n
}

// withTransaction returns a new Rainstorm Node that will use the given transaction.
func (n node) withTransaction(tx *bolt.Tx) *node {
	n.tx = tx
	return &n
}

// WithCodec returns a new Rainstorm Node that will use the given Codec.
func (n node) WithCodec(codec codec.MarshalUnmarshaler) Node {
	n.codec = codec
	return &n
}

// Bucket returns the bucket name as a slice from the root.
// In the normal, simple case this will be empty.
func (n *node) Bucket() []string {
	return cloneBucketPath(n.rootBucket)
}

// Codec returns the EncodeDecoder used by this instance of Rainstorm
func (n *node) Codec() codec.MarshalUnmarshaler {
	return n.codec
}

// Detects if already in transaction or runs a read write transaction.
func (n *node) readWriteTx(ctx context.Context, fn func(tx *bolt.Tx) error) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	run := func(tx *bolt.Tx) error {
		if err := checkContext(ctx); err != nil {
			return err
		}

		if err := fn(tx); err != nil {
			return err
		}

		return checkContext(ctx)
	}

	if n.tx != nil {
		return run(n.tx)
	}

	return n.s.bolt.Update(run)
}

// Detects if already in transaction or runs a read transaction.
func (n *node) readTx(ctx context.Context, fn func(tx *bolt.Tx) error) error {
	if err := checkContext(ctx); err != nil {
		return err
	}

	if n.tx != nil {
		return fn(n.tx)
	}

	return n.s.bolt.View(func(tx *bolt.Tx) error {
		if err := checkContext(ctx); err != nil {
			return err
		}
		return fn(tx)
	})
}
