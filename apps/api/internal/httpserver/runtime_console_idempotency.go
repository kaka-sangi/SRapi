package httpserver

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"

	idempotencycontract "github.com/srapi/srapi/apps/api/internal/modules/idempotency/contract"
	idempotencyservice "github.com/srapi/srapi/apps/api/internal/modules/idempotency/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// withConsoleIdempotency replays successful non-streaming console writes when
// the caller supplies Idempotency-Key. It scopes keys to the authenticated user
// and the route, so repeated form submits cannot create duplicate orders or API
// keys.
func (s *Server) withConsoleIdempotency(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimSpace(r.Header.Get(idempotencyKeyHeader))
		if key == "" || s.runtime.idempotency == nil {
			next(w, r)
			return
		}
		requestID := requestIDFromContext(r.Context())
		if len(key) > maxIdempotencyKeyLength {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "Idempotency-Key header is too long", requestID)
			return
		}
		body, tooLarge, err := s.readGatewayIdempotencyBody(r)
		if err != nil {
			if tooLarge {
				writeStandardError(w, http.StatusRequestEntityTooLarge, apiopenapi.INVALIDREQUEST, "request body too large", requestID)
				return
			}
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid request body", requestID)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		scope := s.consoleIdempotencyScope(r)
		storedKey := scope + ":" + key
		begin, err := s.runtime.idempotency.Begin(r.Context(), storedKey, r.Method, r.URL.Path, idempotencyRequestHash(r.Method, r.URL.Path, body))
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("console idempotency begin failed", "error", err)
			}
			next(w, r)
			return
		}
		switch begin.Outcome {
		case idempotencyservice.OutcomeReplay:
			writeConsoleIdempotencySnapshot(w, begin.Record.Snapshot, requestID)
			return
		case idempotencyservice.OutcomeInFlight:
			writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "a request with this Idempotency-Key is already being processed", requestID)
			return
		case idempotencyservice.OutcomeMismatch:
			writeStandardError(w, http.StatusUnprocessableEntity, apiopenapi.INVALIDREQUEST, "Idempotency-Key was already used with a different request body", requestID)
			return
		}

		recorder := newGatewayIdempotencyRecorder(w)
		next(recorder, r)
		if err := s.runtime.idempotency.Complete(r.Context(), storedKey, r.Method, r.URL.Path, recorder.snapshot()); err != nil && s.logger != nil {
			s.logger.Warn("console idempotency complete failed", "error", err)
		}
	}
}

func writeConsoleIdempotencySnapshot(w http.ResponseWriter, snapshot *idempotencycontract.Snapshot, requestID string) {
	if snapshot == nil {
		writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "a request with this Idempotency-Key has already completed", requestID)
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

func (s *Server) consoleIdempotencyScope(r *http.Request) string {
	session, err := s.requireConsoleSession(r)
	if err != nil {
		return "console:anonymous"
	}
	return "console:user:" + strconv.Itoa(session.User.ID)
}
