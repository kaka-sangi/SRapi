package httpserver

import (
	"bytes"
	"errors"
	"io"
	"strings"
)

// bootstrapStreamPeekSize is the maximum number of bytes to read from the
// upstream before committing the stream to the client. Enough to detect
// JSON error bodies and initial SSE error events without consuming the
// actual content payload.
const bootstrapStreamPeekSize = 4096

// errBootstrapStreamError is returned when the bootstrap peek detects an
// upstream error embedded in a 200 response body. The failover loop treats
// this as a retryable failure.
var errBootstrapStreamError = errors.New("upstream returned error in stream body")

// bootstrapPeekStream reads up to bootstrapStreamPeekSize bytes from the
// stream body and checks for error patterns. If an error is detected,
// returns (nil, errBootstrapStreamError) so the caller can failover. If
// the peeked data looks normal, returns a reader that replays the peeked
// bytes followed by the remainder of the stream — the caller sees the
// full original body.
//
// This implements CLIProxyAPI's "bootstrap retry" pattern: silently retry
// the upstream call before any bytes reach the client, so a transient
// failure on one credential produces a transparent failover rather than
// a client-visible error event.
func bootstrapPeekStream(body io.ReadCloser) (io.ReadCloser, error) {
	if body == nil {
		return nil, errBootstrapStreamError
	}
	buf := make([]byte, bootstrapStreamPeekSize)
	n, readErr := body.Read(buf)
	if n == 0 && readErr != nil {
		body.Close()
		return nil, errBootstrapStreamError
	}
	peeked := buf[:n]
	if bootstrapLooksLikeError(peeked) {
		body.Close()
		return nil, errBootstrapStreamError
	}
	return &prefixedReadCloser{
		prefix: bytes.NewReader(peeked),
		rest:   body,
	}, nil
}

// bootstrapLooksLikeError checks the initial bytes for patterns that
// indicate the upstream sent an error inside a 200 response:
//   - JSON error envelope: {"error": ...}
//   - SSE error event: event: error / "type":"error"
//   - OpenAI error body: "insufficient_quota", "rate_limit_exceeded"
//   - Anthropic overloaded: "overloaded_error"
func bootstrapLooksLikeError(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	s := strings.ToLower(string(data))
	// A valid SSE stream starts with "data:" or "event:" or a comment ": ".
	// A JSON error body starts with "{" and contains "error".
	// Don't flag valid streams that happen to mention "error" in content.
	trimmed := strings.TrimSpace(s)

	// JSON error envelope — upstream returned a JSON body on a 200.
	if len(trimmed) > 0 && trimmed[0] == '{' {
		if strings.Contains(trimmed, `"error"`) {
			return true
		}
	}
	// SSE with an explicit error event type before any data event.
	if strings.HasPrefix(trimmed, "event: error") ||
		strings.HasPrefix(trimmed, "event:error") {
		return true
	}
	return false
}

// prefixedReadCloser replays peeked bytes before reading from the rest.
type prefixedReadCloser struct {
	prefix *bytes.Reader
	rest   io.ReadCloser
}

func (r *prefixedReadCloser) Read(p []byte) (int, error) {
	if r.prefix.Len() > 0 {
		n, err := r.prefix.Read(p)
		if err == io.EOF {
			err = nil
		}
		if n > 0 {
			return n, err
		}
	}
	return r.rest.Read(p)
}

func (r *prefixedReadCloser) Close() error {
	return r.rest.Close()
}
