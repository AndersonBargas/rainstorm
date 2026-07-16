package index

// NewOptions returns Options initialized with no result limit.
func NewOptions() *Options {
	return &Options{
		Limit: -1,
	}
}

// Options controls index result pagination and ordering.
type Options struct {
	// Limit is the maximum number of results; a negative value means unlimited.
	Limit int
	// Skip is the number of matching results to omit.
	Skip int
	// Reverse requests reverse index traversal.
	Reverse bool
}
