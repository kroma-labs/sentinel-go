package telemetry

import (
	"context"
	"fmt"
	"net/http"

	"github.com/kroma-labs/sentinel-go/example/sql/internal/config"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// Setup initializes OpenTelemetry tracing and metrics
func Setup(
	ctx context.Context,
) (shutdownTracing func(context.Context) error, shutdownMetrics func(context.Context) error, err error) {
	// === TRACING SETUP ===
	// Configure OTLP Exporter (connects to Tempo via gRPC)
	traceExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint(config.OTLPEndpoint),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	// Configure Resource (identifies this service)
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(config.ServiceName),
			semconv.ServiceVersion(config.ServiceVersion),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Configure Tracer Provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	// Configure Propagator
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// === METRICS SETUP ===
	// Configure Prometheus Exporter
	promExporter, err := prometheus.New()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create prometheus exporter: %w", err)
	}

	// Configure Meter Provider
	meterProvider := metric.NewMeterProvider(
		metric.WithReader(promExporter),
		metric.WithResource(res),
	)
	otel.SetMeterProvider(meterProvider)

	// Register Prometheus HTTP handler
	// Note: The exporter automatically registers with the global prometheus registry
	// We use promhttp to serve the metrics
	http.Handle("/metrics", promhttp.Handler())

	return tp.Shutdown, meterProvider.Shutdown, nil
}
