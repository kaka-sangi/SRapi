package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestContextHandlerAddsSafeContextFields(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(NewContextHandler(slog.NewJSONHandler(&buffer, nil)))

	ctx := context.Background()
	ctx = WithRequestID(ctx, "req_observe")
	ctx = WithUserID(ctx, 42)
	ctx = WithAPIKeyID(ctx, 7)
	ctx = WithTraceID(ctx, "trace_observe")
	logger.InfoContext(ctx, "context fields")

	var entry map[string]any
	if err := json.Unmarshal(buffer.Bytes(), &entry); err != nil {
		t.Fatalf("decode log entry: %v", err)
	}
	if entry["request_id"] != "req_observe" ||
		entry["trace_id"] != "trace_observe" ||
		entry["user_id"] != float64(42) ||
		entry["api_key_id"] != float64(7) {
		t.Fatalf("expected context fields in log entry, got %+v", entry)
	}
}
