package index

import "errors"

var (
	// ErrNotFound is returned when the specified record is not saved in the bucket.
	ErrNotFound = errors.New("not found")

	// ErrAlreadyExists is returned when trying to set an existing value on a field that has a unique index.
	ErrAlreadyExists = errors.New("already exists")

	// ErrNilParam is returned when the specified param is expected to be not nil.
	ErrNilParam = errors.New("param must not be nil")

	// ErrNilContext is returned when a nil context.Context is passed to an operation that requires a context.
	ErrNilContext = errors.New("context must not be nil")
)
