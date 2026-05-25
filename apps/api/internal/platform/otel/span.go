package otel

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/srapi/srapi/apps/api"

// StartSpan starts a process-local SRapi span with stable service instrumentation naming.
func StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, name, trace.WithAttributes(attrs...))
}

// EndSpan records final attributes, error classification, and ends span.
func EndSpan(span trace.Span, err error, errorType string, attrs ...attribute.KeyValue) {
	if span == nil {
		return
	}
	if len(attrs) > 0 {
		span.SetAttributes(attrs...)
	}
	if err != nil {
		if errorType == "" {
			errorType = "error"
		}
		span.SetAttributes(attribute.String("error.type", errorType))
		span.RecordError(err, trace.WithAttributes(attribute.String("error.type", errorType)))
		span.SetStatus(codes.Error, errorType)
	}
	span.End()
}
