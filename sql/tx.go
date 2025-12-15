package sql

import (
	"context"
	"database/sql/driver"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Compile-time interface check.
var _ driver.Tx = (*otelTx)(nil)

// otelTx wraps a driver.Tx with OpenTelemetry instrumentation.
type otelTx struct {
	tx  driver.Tx
	cfg *config
}

// newOtelTx creates a new instrumented transaction.
func newOtelTx(tx driver.Tx, cfg *config) *otelTx {
	return &otelTx{
		tx:  tx,
		cfg: cfg,
	}
}

// Commit implements driver.Tx.
func (t *otelTx) Commit() error {
	_, span := t.cfg.Tracer.Start(context.Background(), "COMMIT",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(t.cfg.baseAttributes()...),
	)
	defer span.End()

	err := t.tx.Commit()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	return nil
}

// Rollback implements driver.Tx.
func (t *otelTx) Rollback() error {
	_, span := t.cfg.Tracer.Start(context.Background(), "ROLLBACK",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(t.cfg.baseAttributes()...),
	)
	defer span.End()

	err := t.tx.Rollback()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	return nil
}
