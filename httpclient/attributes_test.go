package httpclient

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

func TestRequestAttributes(t *testing.T) {
	tests := []struct {
		name      string
		req       *http.Request
		ctxRoute  string
		wantAttrs []attribute.KeyValue
	}{
		{
			name: "given no context route, then uses unknown-route",
			req: func() *http.Request {
				r, _ := http.NewRequest(http.MethodGet, "http://example.com/api/users/123", nil)
				return r
			}(),
			wantAttrs: []attribute.KeyValue{
				semconv.HTTPRequestMethodKey.String("GET"), // Note: semconv expects string
				semconv.ServerAddressKey.String("example.com"),
				semconv.ServerPortKey.Int(80),
				semconv.HTTPRouteKey.String("unknown-route"),
			},
		},
		{
			name: "given context route, then uses injected route",
			req: func() *http.Request {
				r, _ := http.NewRequest(http.MethodPost, "https://api.example.com/users", nil)
				return r
			}(),
			ctxRoute: "/users",
			wantAttrs: []attribute.KeyValue{
				semconv.HTTPRequestMethodKey.String("POST"),
				semconv.ServerAddressKey.String("api.example.com"),
				semconv.ServerPortKey.Int(443),
				semconv.HTTPRouteKey.String("/users"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.ctxRoute != "" {
				ctx = WithRoute(ctx, tt.ctxRoute)
			}
			req := tt.req.WithContext(ctx)

			attrs := requestAttributes(req)

			// Helper to check containment since order might vary if we change implementation
			// But current impl is append order.
			// Let's check strict equality for now as order is deterministic.
			assert.ElementsMatch(t, tt.wantAttrs, attrs)
		})
	}
}
