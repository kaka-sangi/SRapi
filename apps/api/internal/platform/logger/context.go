package logger

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

type contextKey string

const (
	requestIDKey contextKey = "request_id"
	userIDKey    contextKey = "user_id"
	apiKeyIDKey  contextKey = "api_key_id"
	traceIDKey   contextKey = "trace_id"
)

// WithRequestID adds the stable request identifier used by HTTP responses and audit evidence.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey, requestID)
}

// RequestID returns the request identifier attached to ctx.
func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

// WithUserID adds the authenticated user identifier to ctx.
func WithUserID(ctx context.Context, userID int) context.Context {
	if userID <= 0 {
		return ctx
	}
	return context.WithValue(ctx, userIDKey, userID)
}

// WithAPIKeyID adds the authenticated Gateway API key identifier to ctx.
func WithAPIKeyID(ctx context.Context, apiKeyID int) context.Context {
	if apiKeyID <= 0 {
		return ctx
	}
	return context.WithValue(ctx, apiKeyIDKey, apiKeyID)
}

// WithTraceID adds an externally known trace identifier to ctx.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	if traceID == "" {
		return ctx
	}
	return context.WithValue(ctx, traceIDKey, traceID)
}

// NewContextHandler wraps a slog handler and injects safe context fields into each record.
func NewContextHandler(next slog.Handler) slog.Handler {
	if next == nil {
		next = slog.Default().Handler()
	}
	return contextHandler{next: next}
}

type contextHandler struct {
	next slog.Handler
}

func (h contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h contextHandler) Handle(ctx context.Context, record slog.Record) error {
	attrs := contextAttrs(ctx)
	if len(attrs) > 0 {
		record.AddAttrs(attrs...)
	}
	return h.next.Handle(ctx, record)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{next: h.next.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{next: h.next.WithGroup(name)}
}

func contextAttrs(ctx context.Context) []slog.Attr {
	attrs := make([]slog.Attr, 0, 4)
	if requestID := RequestID(ctx); requestID != "" {
		attrs = append(attrs, slog.String("request_id", requestID))
	}
	if userID, ok := ctx.Value(userIDKey).(int); ok && userID > 0 {
		attrs = append(attrs, slog.Int("user_id", userID))
	}
	if apiKeyID, ok := ctx.Value(apiKeyIDKey).(int); ok && apiKeyID > 0 {
		attrs = append(attrs, slog.Int("api_key_id", apiKeyID))
	}
	if traceID := traceIDFromContext(ctx); traceID != "" {
		attrs = append(attrs, slog.String("trace_id", traceID))
	}
	return attrs
}

func traceIDFromContext(ctx context.Context) string {
	if traceID, _ := ctx.Value(traceIDKey).(string); traceID != "" {
		return traceID
	}
	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.HasTraceID() {
		return spanCtx.TraceID().String()
	}
	return ""
}
