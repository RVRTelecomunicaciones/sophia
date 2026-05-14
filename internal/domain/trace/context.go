package trace

import "context"

// ctxKey is an unexported type for the trace context key.
// Using a typed key prevents collisions with other packages.
type ctxKey struct{}

// NewContext returns a new context carrying t. Outbound adapters retrieve it
// with FromContext when wiring the Traceparent header onto each request.
func NewContext(ctx context.Context, t Trace) context.Context {
	return context.WithValue(ctx, ctxKey{}, t)
}

// FromContext retrieves the Trace stored by NewContext.
// Returns the zero Trace and false if none is present.
func FromContext(ctx context.Context) (Trace, bool) {
	t, ok := ctx.Value(ctxKey{}).(Trace)
	return t, ok
}
