package index

import "context"

// checkContext validates a context before an index operation.
// A nil context is invalid and returns ErrNilParam.
// If the context is already canceled or expired, the corresponding
// context error is returned unwrapped.
func checkContext(ctx context.Context) error {
	if ctx == nil {
		return ErrNilParam
	}
	return ctx.Err()
}
