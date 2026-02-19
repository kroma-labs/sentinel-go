package httpclient

import (
	"net/http"
	"strconv"

	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// requestAttributes returns common attributes for all spans and metrics.
// It extracts OTel-compliant attributes from the request.
func requestAttributes(req *http.Request) []attribute.KeyValue {
	if req == nil {
		return nil
	}

	attrs := make([]attribute.KeyValue, 0, 5)

	// HTTP Method
	if req.Method != "" {
		attrs = append(attrs, semconv.HTTPRequestMethodKey.String(req.Method))
	}

	if req.URL != nil {
		// Server Address
		host := req.URL.Hostname()
		if host != "" {
			attrs = append(attrs, semconv.ServerAddressKey.String(host))
		}

		// Server Port
		port := req.URL.Port()
		if port != "" {
			if p, err := strconv.Atoi(port); err == nil {
				attrs = append(attrs, semconv.ServerPortKey.Int(p))
			}
		} else {
			switch req.URL.Scheme {
			case "http":
				attrs = append(attrs, semconv.ServerPortKey.Int(80))
			case "https":
				attrs = append(attrs, semconv.ServerPortKey.Int(443))
			}
		}

		// HTTP Route
		// We prioritize the low-cardinality route template from context.
		// If missing, we fallback to "unknown-route" to prevent cardinality explosion.
		if route := routeFromContext(req.Context()); route != "" {
			attrs = append(attrs, semconv.HTTPRouteKey.String(route))
		} else {
			attrs = append(attrs, semconv.HTTPRouteKey.String("unknown-route"))
		}
	}

	return attrs
}
