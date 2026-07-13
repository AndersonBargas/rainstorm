// Package index contains Index engines used to store values and their corresponding IDs
package index

import "context"

// Index interface
type Index interface {
	Add(ctx context.Context, value []byte, targetID []byte) error
	Remove(ctx context.Context, value []byte) error
	RemoveID(ctx context.Context, id []byte) error
	Get(ctx context.Context, value []byte) ([]byte, error)
	All(ctx context.Context, value []byte, opts *Options) ([][]byte, error)
	AllRecords(ctx context.Context, opts *Options) ([][]byte, error)
	Range(ctx context.Context, min []byte, max []byte, opts *Options) ([][]byte, error)
	Prefix(ctx context.Context, prefix []byte, opts *Options) ([][]byte, error)
}
