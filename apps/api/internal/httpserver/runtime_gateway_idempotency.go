package httpserver

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	idempotencycontract "github.com/srapi/srapi/apps/api/internal/modules/idempotency/contract"
	idempotencyservice "github.com/srapi/srapi/apps/api/internal/modules/idempotency/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

const (
	idempotencyKeyHeader        = "Idempotency-Key"
	idempotencyReplayedHeader   = "Idempotency-Replayed"
	maxIdempotencyKeyLength     = 255
	maxIdempotencySnapshotBytes = 1 << 20
)

// withGatewayIdempotency makes a mutating gateway handler safe to retry: a client
// that supplies an Idempotency-Key gets the first response replayed on retry
// instead of re-executing (and re-billing). v1 covers non-streaming requests only;
// streaming requests and key-less requests pass through unchanged.
func (s *Server) withGatewayIdempotency(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimSpace(r.Header.Get(idempotencyKeyHeader))
		if key == "" || s.runtime.idempotency == nil {
			next(w, r)
			return
		}
		if len(key) > maxIdempotencyKeyLength {
			writeGatewayError(w, http.StatusBadRequest, apiopenapi.InvalidRequestError, "Idempotency-Key header is too long", "invalid_request")
			return
		}
		body, tooLarge, err := s.readGatewayIdempotencyBody(r)
		if err != nil {
			if tooLarge {
				writeGatewayError(w, http.StatusRequestEntityTooLarge, apiopenapi.InvalidRequestError, "request body too large", "invalid_request")
				return
			}
			writeGatewayError(w, http.StatusBadRequest, apiopenapi.InvalidRequestError, "invalid request body", "invalid_request")
			return
		}
		// Restore the consumed body so the wrapped handler can read it normally.
		r.Body = io.NopCloser(bytes.NewReader(body))
		if gatewayRequestIsStreaming(r, body) {
			// v1: streaming responses are not snapshotted/replayed.
			next(w, r)
			return
		}

		storedKey := idempotencyBearerScope(r) + ":" + key
		requestHash := idempotencyRequestHash(r.Method, r.URL.Path, body)
		begin, err := s.runtime.idempotency.Begin(r.Context(), storedKey, r.Method, r.URL.Path, requestHash)
		if err != nil {
			// Fail open: never let an idempotency-store hiccup break the gateway.
			if s.logger != nil {
				s.logger.Warn("gateway idempotency begin failed", "error", err)
			}
			next(w, r)
			return
		}
		switch begin.Outcome {
		case idempotencyservice.OutcomeReplay:
			writeGatewayIdempotencySnapshot(w, begin.Record.Snapshot)
			return
		case idempotencyservice.OutcomeInFlight:
			writeGatewayError(w, http.StatusConflict, apiopenapi.InvalidRequestError, "a request with this Idempotency-Key is already being processed", "idempotency_conflict")
			return
		case idempotencyservice.OutcomeMismatch:
			writeGatewayError(w, http.StatusUnprocessableEntity, apiopenapi.InvalidRequestError, "Idempotency-Key was already used with a different request body", "idempotency_key_reused")
			return
		}

		recorder := newGatewayIdempotencyRecorder(w)
		next(recorder, r)
		if err := s.runtime.idempotency.Complete(r.Context(), storedKey, r.Method, r.URL.Path, recorder.snapshot()); err != nil && s.logger != nil {
			s.logger.Warn("gateway idempotency complete failed", "error", err)
		}
	}
}

func (s *Server) readGatewayIdempotencyBody(r *http.Request) ([]byte, bool, error) {
	if r.Body == nil {
		return nil, false, nil
	}
	limit := s.cfg.Gateway.MaxBodySize
	body, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(body)) > limit {
		return nil, true, errRequestTooLarge
	}
	return body, false, nil
}

func gatewayRequestIsStreaming(r *http.Request, body []byte) bool {
	if gatewaySourceEndpointIsResponsesCompact(gatewaySourceEndpoint(r.Context(), r.URL.Path)) {
		return false
	}
	var probe struct {
		Stream *bool `json:"stream"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return false
	}
	return probe.Stream != nil && *probe.Stream
}

// idempotencyBearerScope namespaces idempotency keys per bearer token so two
// tenants reusing the same key never collide or replay each other's responses.
// The token is hashed, never stored in plaintext.
func idempotencyBearerScope(r *http.Request) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(r.Header.Get("Authorization"))))
	return hex.EncodeToString(sum[:])
}

func idempotencyRequestHash(method, path string, body []byte) string {
	hash := sha256.New()
	hash.Write([]byte(method))
	hash.Write([]byte("\n"))
	hash.Write([]byte(path))
	hash.Write([]byte("\n"))
	hash.Write(body)
	return hex.EncodeToString(hash.Sum(nil))
}

func writeGatewayIdempotencySnapshot(w http.ResponseWriter, snapshot *idempotencycontract.Snapshot) {
	if snapshot == nil {
		writeGatewayError(w, http.StatusConflict, apiopenapi.InvalidRequestError, "a request with this Idempotency-Key has already completed", "idempotency_conflict")
		return
	}
	for key, values := range snapshot.Headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.Header().Set(idempotencyReplayedHeader, "true")
	status := snapshot.StatusCode
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_, _ = w.Write(snapshot.Body)
}

// gatewayIdempotencyRecorder forwards the handler's response to the client while
// buffering it (up to a cap) so a successful non-streaming response can be stored
// for replay. Oversize, non-2xx, or SSE responses are forwarded but not snapshotted.
type gatewayIdempotencyRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	body        bytes.Buffer
	capturing   bool
	overflow    bool
}

func newGatewayIdempotencyRecorder(w http.ResponseWriter) *gatewayIdempotencyRecorder {
	return &gatewayIdempotencyRecorder{ResponseWriter: w, status: http.StatusOK, capturing: true}
}

func (rec *gatewayIdempotencyRecorder) WriteHeader(status int) {
	if !rec.wroteHeader {
		rec.status = status
		rec.wroteHeader = true
	}
	rec.ResponseWriter.WriteHeader(status)
}

func (rec *gatewayIdempotencyRecorder) Write(p []byte) (int, error) {
	rec.wroteHeader = true
	if rec.capturing {
		if rec.body.Len()+len(p) > maxIdempotencySnapshotBytes {
			rec.capturing = false
			rec.overflow = true
			rec.body.Reset()
		} else {
			rec.body.Write(p)
		}
	}
	return rec.ResponseWriter.Write(p)
}

func (rec *gatewayIdempotencyRecorder) Flush() {
	if flusher, ok := rec.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (rec *gatewayIdempotencyRecorder) snapshot() *idempotencycontract.Snapshot {
	if rec.overflow || rec.status < http.StatusOK || rec.status >= http.StatusMultipleChoices {
		return nil
	}
	if strings.Contains(strings.ToLower(rec.Header().Get("Content-Type")), "text/event-stream") {
		return nil
	}
	headers := map[string][]string{}
	if contentType := rec.Header().Values("Content-Type"); len(contentType) > 0 {
		headers["Content-Type"] = append([]string(nil), contentType...)
	}
	return &idempotencycontract.Snapshot{
		StatusCode: rec.status,
		Headers:    headers,
		Body:       append([]byte(nil), rec.body.Bytes()...),
	}
}
