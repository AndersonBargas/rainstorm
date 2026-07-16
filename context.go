package rainstorm

import "context"

// checkContext validates a context at the entry of a contextual operation.
//
// A nil context returns ErrNilContext. An already canceled or expired
// context returns the corresponding context error unwrapped so callers can
// match it with errors.Is.
//
// This helper only observes input cancellation; it does not create a
// replacement context.
func checkContext(ctx context.Context) error {
	if ctx == nil {
		return ErrNilContext
	}
	return ctx.Err()
}
