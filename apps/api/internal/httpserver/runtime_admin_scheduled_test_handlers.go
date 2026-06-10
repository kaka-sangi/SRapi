package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	scheduledcontract "github.com/srapi/srapi/apps/api/internal/modules/scheduled_tests/contract"
	scheduledservice "github.com/srapi/srapi/apps/api/internal/modules/scheduled_tests/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

type scheduledTestPlanPayload struct {
	ID              int        `json:"id"`
	Name            string     `json:"name"`
	Enabled         bool       `json:"enabled"`
	ScopeType       string     `json:"scope_type"`
	ScopeID         *int       `json:"scope_id"`
	IntervalSeconds int        `json:"interval_seconds"`
	CronExpression  string     `json:"cron_expression"`
	ProbeModel      string     `json:"probe_model"`
	MaxResults      int        `json:"max_results"`
	AutoRecover     bool       `json:"auto_recover"`
	LastRunAt       *time.Time `json:"last_run_at"`
	LastStatus      string     `json:"last_status"`
	LastSummary     string     `json:"last_summary"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type scheduledTestPlanRunPayload struct {
	ID         int       `json:"id"`
	PlanID     int       `json:"plan_id"`
	Trigger    string    `json:"trigger"`
	Status     string    `json:"status"`
	Selected   int       `json:"selected"`
	Probed     int       `json:"probed"`
	Skipped    int       `json:"skipped"`
	Failed     int       `json:"failed"`
	Unhealthy  int       `json:"unhealthy"`
	Recovered  int       `json:"recovered"`
	Summary    string    `json:"summary"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
}

type createScheduledTestPlanRequest struct {
	Name            string `json:"name"`
	Enabled         *bool  `json:"enabled"`
	ScopeType       string `json:"scope_type"`
	ScopeID         *int   `json:"scope_id"`
	IntervalSeconds *int   `json:"interval_seconds"`
	CronExpression  string `json:"cron_expression"`
	ProbeModel      string `json:"probe_model"`
	MaxResults      *int   `json:"max_results"`
	AutoRecover     *bool  `json:"auto_recover"`
}

type updateScheduledTestPlanRequest struct {
	Name            *string `json:"name"`
	Enabled         *bool   `json:"enabled"`
	ScopeType       *string `json:"scope_type"`
	ScopeID         *int    `json:"scope_id"`
	IntervalSeconds *int    `json:"interval_seconds"`
	CronExpression  *string `json:"cron_expression"`
	ProbeModel      *string `json:"probe_model"`
	MaxResults      *int    `json:"max_results"`
	AutoRecover     *bool   `json:"auto_recover"`
}

func toScheduledTestPlanPayload(plan scheduledcontract.Plan) scheduledTestPlanPayload {
	return scheduledTestPlanPayload{
		ID:              plan.ID,
		Name:            plan.Name,
		Enabled:         plan.Enabled,
		ScopeType:       string(plan.ScopeType),
		ScopeID:         plan.ScopeID,
		IntervalSeconds: plan.IntervalSeconds,
		CronExpression:  plan.CronExpression,
		ProbeModel:      plan.ProbeModel,
		MaxResults:      plan.MaxResults,
		AutoRecover:     plan.AutoRecover,
		LastRunAt:       plan.LastRunAt,
		LastStatus:      plan.LastStatus,
		LastSummary:     plan.LastSummary,
		CreatedAt:       plan.CreatedAt.UTC(),
		UpdatedAt:       plan.UpdatedAt.UTC(),
	}
}

func toScheduledTestPlanRunPayload(run scheduledcontract.Run) scheduledTestPlanRunPayload {
	return scheduledTestPlanRunPayload{
		ID:         run.ID,
		PlanID:     run.PlanID,
		Trigger:    run.Trigger,
		Status:     run.Status,
		Selected:   run.Selected,
		Probed:     run.Probed,
		Skipped:    run.Skipped,
		Failed:     run.Failed,
		Unhealthy:  run.Unhealthy,
		Recovered:  run.Recovered,
		Summary:    run.Summary,
		StartedAt:  run.StartedAt.UTC(),
		FinishedAt: run.FinishedAt.UTC(),
	}
}

func (s *Server) handleListAdminScheduledTestPlans(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	plans, err := s.runtime.scheduledTests.ListPlans(r.Context())
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to list scheduled test plans", requestID)
		return
	}
	data := make([]scheduledTestPlanPayload, 0, len(plans))
	for _, plan := range plans {
		data = append(data, toScheduledTestPlanPayload(plan))
	}
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       data,
		"pagination": pagination(len(data)),
		"request_id": requestID,
	})
}

func (s *Server) handleCreateAdminScheduledTestPlan(w http.ResponseWriter, r *http.Request) {
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
	var body createScheduledTestPlanRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid scheduled test plan request", requestID)
		return
	}
	input := scheduledcontract.CreatePlan{
		Name:           body.Name,
		Enabled:        true,
		ScopeType:      scheduledcontract.ScopeType(body.ScopeType),
		ScopeID:        body.ScopeID,
		CronExpression: body.CronExpression,
		ProbeModel:     body.ProbeModel,
	}
	if body.Enabled != nil {
		input.Enabled = *body.Enabled
	}
	if body.IntervalSeconds != nil {
		input.IntervalSeconds = *body.IntervalSeconds
	}
	if body.MaxResults != nil {
		input.MaxResults = *body.MaxResults
	}
	if body.AutoRecover != nil {
		input.AutoRecover = *body.AutoRecover
	}
	plan, err := s.runtime.scheduledTests.CreatePlan(r.Context(), input)
	if err != nil {
		s.writeScheduledTestPlanError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "scheduled_test_plan.create", "scheduled_test_plan", strconv.Itoa(plan.ID), nil, map[string]any{
		"name":  plan.Name,
		"scope": plan.ScopeType,
	}))
	writeJSONAny(w, http.StatusCreated, map[string]any{
		"data":       toScheduledTestPlanPayload(plan),
		"request_id": requestID,
	})
}

func (s *Server) handleUpdateAdminScheduledTestPlan(w http.ResponseWriter, r *http.Request) {
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
	planID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || planID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid scheduled test plan id", requestID)
		return
	}
	var body updateScheduledTestPlanRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid scheduled test plan request", requestID)
		return
	}
	input := scheduledcontract.UpdatePlan{
		Name:            body.Name,
		Enabled:         body.Enabled,
		ScopeID:         body.ScopeID,
		IntervalSeconds: body.IntervalSeconds,
		CronExpression:  body.CronExpression,
		ProbeModel:      body.ProbeModel,
		MaxResults:      body.MaxResults,
		AutoRecover:     body.AutoRecover,
	}
	if body.ScopeType != nil {
		scope := scheduledcontract.ScopeType(*body.ScopeType)
		input.ScopeType = &scope
	}
	plan, err := s.runtime.scheduledTests.UpdatePlan(r.Context(), planID, input)
	if err != nil {
		s.writeScheduledTestPlanError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "scheduled_test_plan.update", "scheduled_test_plan", strconv.Itoa(plan.ID), nil, map[string]any{
		"name":    plan.Name,
		"enabled": plan.Enabled,
	}))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       toScheduledTestPlanPayload(plan),
		"request_id": requestID,
	})
}

func (s *Server) handleDeleteAdminScheduledTestPlan(w http.ResponseWriter, r *http.Request) {
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
	planID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || planID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid scheduled test plan id", requestID)
		return
	}
	if err := s.runtime.scheduledTests.DeletePlan(r.Context(), planID); err != nil {
		s.writeScheduledTestPlanError(w, err, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "scheduled_test_plan.delete", "scheduled_test_plan", strconv.Itoa(planID), nil, nil))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       map[string]any{"id": planID, "deleted": true},
		"request_id": requestID,
	})
}

func (s *Server) handleListAdminScheduledTestPlanRuns(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	planID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || planID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid scheduled test plan id", requestID)
		return
	}
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	runs, err := s.runtime.scheduledTests.ListRuns(r.Context(), planID, limit)
	if err != nil {
		s.writeScheduledTestPlanError(w, err, requestID)
		return
	}
	data := make([]scheduledTestPlanRunPayload, 0, len(runs))
	for _, run := range runs {
		data = append(data, toScheduledTestPlanRunPayload(run))
	}
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       data,
		"pagination": pagination(len(data)),
		"request_id": requestID,
	})
}

func (s *Server) handleRunAdminScheduledTestPlan(w http.ResponseWriter, r *http.Request) {
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
	planID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || planID <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid scheduled test plan id", requestID)
		return
	}
	plan, err := s.runtime.scheduledTests.FindPlan(r.Context(), planID)
	if err != nil {
		s.writeScheduledTestPlanError(w, err, requestID)
		return
	}
	if s.runtime.scheduledTestRunner == nil {
		writeStandardError(w, http.StatusServiceUnavailable, apiopenapi.INTERNALERROR, "scheduled test runner unavailable", requestID)
		return
	}
	run, runErr := s.runtime.scheduledTestRunner.RunPlan(r.Context(), plan, scheduledcontract.TriggerManual)
	if run.ID == 0 && runErr != nil {
		s.writeScheduledTestPlanError(w, runErr, requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "scheduled_test_plan.run", "scheduled_test_plan", strconv.Itoa(planID), nil, map[string]any{
		"trigger": scheduledcontract.TriggerManual,
		"status":  run.Status,
	}))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       toScheduledTestPlanRunPayload(run),
		"request_id": requestID,
	})
}

func (s *Server) writeScheduledTestPlanError(w http.ResponseWriter, err error, requestID string) {
	switch {
	case errors.Is(err, scheduledcontract.ErrNotFound):
		writeStandardError(w, http.StatusNotFound, apiopenapi.RESOURCENOTFOUND, "scheduled test plan not found", requestID)
	case errors.Is(err, scheduledservice.ErrInvalidInput):
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid scheduled test plan request", requestID)
	default:
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to process scheduled test plan request", requestID)
	}
}
