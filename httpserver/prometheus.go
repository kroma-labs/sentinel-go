package httpserver

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// PrometheusHandler returns an http.Handler for the /metrics endpoint.
//
// This exposes Prometheus metrics in the standard text format.
//
// Example:
//
//	mux.Handle("/metrics", httpserver.PrometheusHandler())
func PrometheusHandler() http.Handler {
	return promhttp.Handler()
}

// PrometheusHandlerFor returns a Prometheus handler with custom options.
//
// Example:
//
//	mux.Handle("/metrics", httpserver.PrometheusHandlerFor(opts))
func PrometheusHandlerFor(opts promhttp.HandlerOpts) http.Handler {
	return promhttp.HandlerFor(nil, opts)
}
