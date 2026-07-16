// Package index contains Index engines used to store values and their corresponding IDs
package index

import "context"

// Index maps encoded field values to encoded record IDs.
type Index interface {
	// Add associates value with targetID.
	Add(ctx context.Context, value []byte, targetID []byte) error
	// Remove deletes all index entries for value.
	Remove(ctx context.Context, value []byte) error
	// RemoveID deletes index entries that reference id.
	RemoveID(ctx context.Context, id []byte) error
	// Get returns the first ID associated with value.
	Get(ctx context.Context, value []byte) ([]byte, error)
	// All returns IDs associated with value using opts.
	All(ctx context.Context, value []byte, opts *Options) ([][]byte, error)
	// AllRecords returns all indexed IDs using opts.
	AllRecords(ctx context.Context, opts *Options) ([][]byte, error)
	// Range returns IDs whose values fall within the inclusive range.
	Range(ctx context.Context, min []byte, max []byte, opts *Options) ([][]byte, error)
	// Prefix returns IDs whose values begin with prefix.
	Prefix(ctx context.Context, prefix []byte, opts *Options) ([][]byte, error)
}
