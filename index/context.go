package index

import (
	"context"
	"fmt"
)

// checkContext validates a context before an index operation.
// A nil context returns ErrNilContext. An already canceled or expired
// context returns the corresponding context error unwrapped.
func checkContext(ctx context.Context) error {
	if ctx == nil {
		return ErrNilContext
	}
	return ctx.Err()
}

// wrapError adds operation context to an error while preserving classification.
// It returns nil when err is nil.
func wrapError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("rainstorm %s: %w", operation, err)
}
