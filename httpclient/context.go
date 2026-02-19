package httpclient

import "context"

type routeKey struct{}

// WithRoute adds the matched route template to the request context.
// This is used for low-cardinality metrics (e.g. "/users/{id}").
func WithRoute(ctx context.Context, route string) context.Context {
	return context.WithValue(ctx, routeKey{}, route)
}

// routeFromContext retrieves the route template from the context.
func routeFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(routeKey{}).(string); ok {
		return v
	}
	return ""
}
