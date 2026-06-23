package httpserver

import (
	"fmt"
	"net/http"
	"strconv"

	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
	"github.com/srapi/srapi/apps/api/internal/platform/circuitbreaker"
	"github.com/srapi/srapi/apps/api/internal/platform/eventsub"
)

func (s *Server) handleAdminCircuitBreakers(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}

	s.runtime.accountBreakersMu.RLock()
	breakers := make(map[int]*circuitbreaker.Breaker, len(s.runtime.accountBreakers))
	for id, b := range s.runtime.accountBreakers {
		breakers[id] = b
	}
	s.runtime.accountBreakersMu.RUnlock()

	type breakerEntry struct {
		AccountID            int     `json:"account_id"`
		State                string  `json:"state"`
		Requests             int64   `json:"requests"`
		TotalSuccesses       int64   `json:"total_successes"`
		TotalFailures        int64   `json:"total_failures"`
		ConsecutiveSuccesses int64   `json:"consecutive_successes"`
		ConsecutiveFailures  int64   `json:"consecutive_failures"`
		SuccessRate          float64 `json:"success_rate"`
	}

	entries := make([]breakerEntry, 0, len(breakers))
	for id, b := range breakers {
		counts := b.Counts()
		var rate float64
		if counts.Requests > 0 {
			rate = float64(counts.TotalSuccesses) / float64(counts.Requests)
		}
		entries = append(entries, breakerEntry{
			AccountID:            id,
			State:                b.State().String(),
			Requests:             counts.Requests,
			TotalSuccesses:       counts.TotalSuccesses,
			TotalFailures:        counts.TotalFailures,
			ConsecutiveSuccesses: counts.ConsecutiveSuccesses,
			ConsecutiveFailures:  counts.ConsecutiveFailures,
			SuccessRate:          rate,
		})
	}

	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       entries,
		"total":      len(entries),
		"request_id": requestID,
	})
}

func (s *Server) handleAdminResetCircuitBreaker(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}
	accountID, err := strconv.Atoi(r.PathValue("accountId"))
	if err != nil || accountID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid account id", requestID)
		return
	}

	s.runtime.accountBreakersMu.RLock()
	b, ok := s.runtime.accountBreakers[accountID]
	s.runtime.accountBreakersMu.RUnlock()
	if !ok {
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "no circuit breaker for this account", requestID)
		return
	}

	b.Reset()
	s.runtime.logger.Info("circuit breaker manually reset",
		"account_id", accountID,
		"admin_user_id", session.User.ID)

	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       map[string]any{"account_id": accountID, "state": "closed"},
		"request_id": requestID,
	})
}

func (s *Server) handleAdminCacheStats(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}

	type cacheStatsEntry struct {
		Name      string `json:"name"`
		Hits      int64  `json:"hits"`
		Misses    int64  `json:"misses"`
		Evictions int64  `json:"evictions"`
		Size      int    `json:"size"`
		HitRate   string `json:"hit_rate"`
	}

	var entries []cacheStatsEntry
	if s.runtime.modelResolutionCache != nil {
		stats := s.runtime.modelResolutionCache.Stats()
		total := stats.Hits + stats.Misses
		hitRate := "0%"
		if total > 0 {
			hitRate = strconv.FormatFloat(float64(stats.Hits)/float64(total)*100, 'f', 1, 64) + "%"
		}
		entries = append(entries, cacheStatsEntry{
			Name:      "model_resolution",
			Hits:      stats.Hits,
			Misses:    stats.Misses,
			Evictions: stats.Evictions,
			Size:      stats.Size,
			HitRate:   hitRate,
		})
	}

	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       entries,
		"request_id": requestID,
	})
}

func (s *Server) handleAdminClearCache(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	session, err := s.requireAdminSession(r)
	if err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	if err := validateCSRF(session.Session, r.Header.Get(csrfHeaderName)); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "invalid csrf token", requestID)
		return
	}

	cleared := 0
	if s.runtime.modelResolutionCache != nil {
		cleared = s.runtime.modelResolutionCache.Len()
		s.runtime.modelResolutionCache.Clear()
	}

	s.runtime.logger.Info("admin cache cleared",
		"admin_user_id", session.User.ID,
		"entries_cleared", cleared)

	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       map[string]any{"cleared": cleared},
		"request_id": requestID,
	})
}

func (s *Server) handleAdminEventStream(w http.ResponseWriter, r *http.Request) {
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestIDFromContext(r.Context()))
		return
	}

	requestID := requestIDFromContext(r.Context())
	hub := s.runtime.eventHub
	if hub == nil {
		writeStandardError(w, http.StatusServiceUnavailable, apiopenapi.INTERNALERROR, "event stream not available", requestID)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "streaming not supported", requestID)
		return
	}

	sub := hub.Subscribe(64)
	defer hub.Unsubscribe(sub)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Send initial ping so the client knows the connection is alive.
	fmt.Fprint(w, "event: ping\ndata: {}\n\n")
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-sub.Done():
			return
		case e := <-sub.Events():
			w.Write(eventsub.MarshalSSE(e))
			flusher.Flush()
		}
	}
}
