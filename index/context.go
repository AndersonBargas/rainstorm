package index

import "context"

// checkContext validates a context before an index operation.
// A nil context returns ErrNilContext. An already canceled or expired
// context returns the corresponding context error unwrapped.
func checkContext(ctx context.Context) error {
	if ctx == nil {
		return ErrNilContext
	}
	return ctx.Err()
}
