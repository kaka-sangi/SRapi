package httpserver

import (
	"errors"
	"net/http"
	"strconv"

	scheduledcontract "github.com/srapi/srapi/apps/api/internal/modules/scheduled_tests/contract"
	scheduledservice "github.com/srapi/srapi/apps/api/internal/modules/scheduled_tests/service"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func toAPIScheduledTestPlan(plan scheduledcontract.Plan) apiopenapi.ScheduledTestPlan {
	return apiopenapi.ScheduledTestPlan{
		Id:              int64(plan.ID),
		Name:            plan.Name,
		Enabled:         plan.Enabled,
		ScopeType:       apiopenapi.ScheduledTestPlanScopeType(plan.ScopeType),
		ScopeId:         int64PtrFromIntPtr(plan.ScopeID),
		IntervalSeconds: int64(plan.IntervalSeconds),
		CronExpression:  plan.CronExpression,
		ProbeModel:      plan.ProbeModel,
		MaxResults:      int64(plan.MaxResults),
		AutoRecover:     plan.AutoRecover,
		LastRunAt:       plan.LastRunAt,
		LastStatus:      plan.LastStatus,
		LastSummary:     plan.LastSummary,
		CreatedAt:       plan.CreatedAt.UTC(),
		UpdatedAt:       plan.UpdatedAt.UTC(),
	}
}

func toAPIScheduledTestPlanRun(run scheduledcontract.Run) apiopenapi.ScheduledTestPlanRun {
	return apiopenapi.ScheduledTestPlanRun{
		Id:         int64(run.ID),
		PlanId:     int64(run.PlanID),
		Trigger:    apiopenapi.ScheduledTestPlanRunTrigger(run.Trigger),
		Status:     apiopenapi.ScheduledTestPlanRunStatus(run.Status),
		Selected:   int64(run.Selected),
		Probed:     int64(run.Probed),
		Skipped:    int64(run.Skipped),
		Failed:     int64(run.Failed),
		Unhealthy:  int64(run.Unhealthy),
		Recovered:  int64(run.Recovered),
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
	data := make([]apiopenapi.ScheduledTestPlan, 0, len(plans))
	for _, plan := range plans {
		data = append(data, toAPIScheduledTestPlan(plan))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.ScheduledTestPlanListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
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
	var body apiopenapi.CreateScheduledTestPlanRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid scheduled test plan request", requestID)
		return
	}
	scopeID, ok := intPtrFromInt64Ptr(body.ScopeId)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid scheduled test plan request", requestID)
		return
	}
	input := scheduledcontract.CreatePlan{
		Name:           body.Name,
		Enabled:        true,
		ScopeType:      scheduledcontract.ScopeType(body.ScopeType),
		ScopeID:        scopeID,
		CronExpression: openapiOptionalString(body.CronExpression),
		ProbeModel:     openapiOptionalString(body.ProbeModel),
	}
	if body.Enabled != nil {
		input.Enabled = *body.Enabled
	}
	if body.IntervalSeconds != nil {
		interval, ok := intPtrFromInt64Ptr(body.IntervalSeconds)
		if !ok {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid scheduled test plan request", requestID)
			return
		}
		input.IntervalSeconds = *interval
	}
	if body.MaxResults != nil {
		maxResults, ok := intPtrFromInt64Ptr(body.MaxResults)
		if !ok {
			writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid scheduled test plan request", requestID)
			return
		}
		input.MaxResults = *maxResults
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
	writeJSONAny(w, http.StatusCreated, apiopenapi.ScheduledTestPlanResponse{
		Data:      toAPIScheduledTestPlan(plan),
		RequestId: requestID,
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
	var body apiopenapi.UpdateScheduledTestPlanRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid scheduled test plan request", requestID)
		return
	}
	scopeID, ok := intPtrFromInt64Ptr(body.ScopeId)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid scheduled test plan request", requestID)
		return
	}
	intervalSeconds, ok := intPtrFromInt64Ptr(body.IntervalSeconds)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid scheduled test plan request", requestID)
		return
	}
	maxResults, ok := intPtrFromInt64Ptr(body.MaxResults)
	if !ok {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid scheduled test plan request", requestID)
		return
	}
	input := scheduledcontract.UpdatePlan{
		Name:            body.Name,
		Enabled:         body.Enabled,
		ScopeID:         scopeID,
		IntervalSeconds: intervalSeconds,
		CronExpression:  body.CronExpression,
		ProbeModel:      body.ProbeModel,
		MaxResults:      maxResults,
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
	writeJSONAny(w, http.StatusOK, apiopenapi.ScheduledTestPlanResponse{
		Data:      toAPIScheduledTestPlan(plan),
		RequestId: requestID,
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
	writeJSONAny(w, http.StatusOK, apiopenapi.DeleteResponse{
		Data: struct {
			Deleted bool `json:"deleted"`
		}{Deleted: true},
		RequestId: requestID,
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
	data := make([]apiopenapi.ScheduledTestPlanRun, 0, len(runs))
	for _, run := range runs {
		data = append(data, toAPIScheduledTestPlanRun(run))
	}
	data, pg := paginate(r, data)
	writeJSONAny(w, http.StatusOK, apiopenapi.ScheduledTestPlanRunListResponse{
		Data:       data,
		Pagination: pg,
		RequestId:  requestID,
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
	writeJSONAny(w, http.StatusOK, apiopenapi.ScheduledTestPlanRunResponse{
		Data:      toAPIScheduledTestPlanRun(run),
		RequestId: requestID,
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
