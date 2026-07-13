package rainstorm

import "context"

// checkContext validates a context at the entry of a contextual operation.
//
// A nil context is invalid in this phase and returns ErrNilParam. The
// definitive nil-context error model will be revisited in R6.4.
//
// If the context is already canceled or expired, the corresponding context
// error is returned unwrapped so callers can match it with errors.Is.
//
// This helper only observes input cancellation; it does not create a
// replacement context.
func checkContext(ctx context.Context) error {
	if ctx == nil {
		return ErrNilParam
	}
	return ctx.Err()
}
