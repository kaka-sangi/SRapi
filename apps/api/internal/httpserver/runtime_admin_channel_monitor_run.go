package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	channelmonitorscontract "github.com/srapi/srapi/apps/api/internal/modules/channel_monitors/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func toChannelMonitorRunPayload(run channelmonitorscontract.RunResult) map[string]any {
	results := make([]map[string]any, 0, len(run.Results))
	for _, result := range run.Results {
		entry := map[string]any{
			"account_id":   result.AccountID,
			"account_name": result.AccountName,
			"provider_id":  result.ProviderID,
			"model":        result.Model,
			"ok":           result.OK,
			"status_code":  result.StatusCode,
			"latency_ms":   result.LatencyMS,
		}
		if result.ErrorClass != "" {
			entry["error_class"] = result.ErrorClass
		}
		if result.Metadata != nil {
			entry["metadata"] = result.Metadata
		}
		results = append(results, entry)
	}
	return map[string]any{
		"id":            run.ID,
		"monitor_id":    run.MonitorID,
		"run_id":        run.RunID,
		"ok":            run.OK,
		"checked_count": run.CheckedCount,
		"ok_count":      run.OKCount,
		"latency_ms":    run.LatencyMS,
		"trigger":       run.Trigger,
		"results":       results,
		"created_at":    run.CreatedAt.UTC(),
	}
}

func (s *Server) handleRunAdminChannelMonitor(w http.ResponseWriter, r *http.Request) {
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
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		writeStandardError(w, http.StatusBadRequest, apiopenapi.INVALIDREQUEST, "invalid channel monitor id", requestID)
		return
	}
	def, err := s.runtime.channelMonitors.GetDefinition(r.Context(), id)
	if err != nil {
		s.writeChannelMonitorError(w, err, requestID)
		return
	}

	startedAt := time.Now()
	accounts, err := s.resolveChannelMonitorAccounts(r.Context(), def)
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to resolve monitor targets", requestID)
		return
	}
	results := make([]channelmonitorscontract.CheckResult, 0, len(accounts))
	okCount := 0
	for _, account := range accounts {
		result := s.runChannelMonitorProbe(r.Context(), def, account)
		if result.OK {
			okCount++
		}
		results = append(results, result)
	}

	runID := fmt.Sprintf("monitor_%d_%d", def.ID, time.Now().UnixNano())
	overallOK := len(results) > 0 && okCount == len(results)
	run, err := s.runtime.channelMonitors.RecordRun(r.Context(), channelmonitorscontract.RecordRun{
		MonitorID:    def.ID,
		RunID:        runID,
		OK:           overallOK,
		CheckedCount: len(results),
		OKCount:      okCount,
		LatencyMS:    elapsedMillis(startedAt),
		Trigger:      "manual",
		Results:      results,
	})
	if err != nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "failed to record monitor run", requestID)
		return
	}
	s.runtime.recordAudit(r.Context(), auditRecordFromRequest(r, session.User.ID, "channel_monitor.run", "channel_monitor", strconv.Itoa(def.ID), nil, map[string]any{
		"checked": run.CheckedCount,
		"ok":      run.OKCount,
	}))
	writeJSONAny(w, http.StatusOK, map[string]any{
		"data":       toChannelMonitorRunPayload(run),
		"request_id": requestID,
	})
}

// resolveChannelMonitorAccounts returns the provider accounts a monitor targets,
// based on its scope and scope_ref.
func (s *Server) resolveChannelMonitorAccounts(ctx context.Context, def channelmonitorscontract.Definition) ([]accountcontract.ProviderAccount, error) {
	all, err := s.runtime.accounts.List(ctx)
	if err != nil {
		return nil, err
	}
	switch def.Scope {
	case channelmonitorscontract.ScopeAccount:
		accountID, _ := strconv.Atoi(strings.TrimSpace(def.ScopeRef))
		for _, account := range all {
			if account.ID == accountID {
				return []accountcontract.ProviderAccount{account}, nil
			}
		}
		return nil, nil
	case channelmonitorscontract.ScopeProvider:
		providerID, _ := strconv.Atoi(strings.TrimSpace(def.ScopeRef))
		out := make([]accountcontract.ProviderAccount, 0)
		for _, account := range all {
			if account.ProviderID == providerID {
				out = append(out, account)
			}
		}
		return out, nil
	case channelmonitorscontract.ScopeGroup:
		groupID, _ := strconv.Atoi(strings.TrimSpace(def.ScopeRef))
		members, err := s.runtime.accounts.ListGroupMembers(ctx, groupID)
		if err != nil {
			return nil, err
		}
		memberIDs := make(map[int]struct{}, len(members))
		for _, member := range members {
			memberIDs[member.AccountID] = struct{}{}
		}
		out := make([]accountcontract.ProviderAccount, 0, len(members))
		for _, account := range all {
			if _, ok := memberIDs[account.ID]; ok {
				out = append(out, account)
			}
		}
		return out, nil
	case channelmonitorscontract.ScopeModel:
		return s.resolveChannelMonitorModelAccounts(ctx, def, all)
	default:
		return nil, nil
	}
}

func (s *Server) resolveChannelMonitorModelAccounts(ctx context.Context, def channelmonitorscontract.Definition, all []accountcontract.ProviderAccount) ([]accountcontract.ProviderAccount, error) {
	pattern := strings.TrimSpace(def.ScopeRef)
	if pattern == "" {
		return nil, nil
	}
	models, err := s.runtime.models.List(ctx)
	if err != nil {
		return nil, err
	}
	providerIDs := make(map[int]struct{})
	for _, model := range models {
		if !channelMonitorGlobMatch(pattern, model.CanonicalName) {
			continue
		}
		mappings, err := s.runtime.models.ListMappingsByModel(ctx, model.ID)
		if err != nil {
			return nil, err
		}
		for _, mapping := range mappings {
			providerIDs[mapping.ProviderID] = struct{}{}
		}
	}
	out := make([]accountcontract.ProviderAccount, 0)
	for _, account := range all {
		if _, ok := providerIDs[account.ProviderID]; ok {
			out = append(out, account)
		}
	}
	return out, nil
}

// runChannelMonitorProbe overlays the monitor's custom request onto the account
// metadata using the config-map probe keys the probe service reads, decrypts the
// credential, and runs the existing ProbeAccount path, returning a per-account
// CheckResult.
func (s *Server) runChannelMonitorProbe(ctx context.Context, def channelmonitorscontract.Definition, account accountcontract.ProviderAccount) channelmonitorscontract.CheckResult {
	model := strings.TrimSpace(def.Model)
	result := channelmonitorscontract.CheckResult{
		AccountID:   account.ID,
		AccountName: account.Name,
		ProviderID:  account.ProviderID,
		Model:       model,
	}
	provider, err := s.runtime.providers.FindByID(ctx, account.ProviderID)
	if err != nil {
		result.ErrorClass = "provider_not_found"
		result.StatusCode = http.StatusBadRequest
		return result
	}
	credential, err := s.runtime.accounts.DecryptCredential(ctx, account.ID)
	if err != nil {
		result.ErrorClass = "credential_decrypt_failed"
		result.StatusCode = http.StatusInternalServerError
		return result
	}
	overlay := channelMonitorProbeMetadata(def.Request)
	metadata := mergeAccountMetadata(account.Metadata, &overlay)
	probeAccount := account
	probeAccount.Metadata = metadata

	resp, err := s.runtime.adapters.ProbeAccount(ctx, provideradaptercontract.ProbeRequest{
		Provider:   provider,
		Account:    probeAccount,
		Credential: credential,
	})
	if err != nil {
		result.ErrorClass = "probe_failed"
		result.StatusCode = http.StatusBadGateway
		return result
	}
	result.OK = resp.OK
	result.StatusCode = resp.StatusCode
	result.LatencyMS = resp.LatencyMS
	result.ErrorClass = resp.ErrorClass
	if resp.Metadata != nil {
		result.Metadata = resp.Metadata
	}
	return result
}

// channelMonitorProbeMetadata maps a monitor's custom request onto the
// health_probe_* metadata keys the probe service consumes. Empty fields are
// omitted so the account/provider config defaults still apply.
func channelMonitorProbeMetadata(req channelmonitorscontract.CustomRequest) map[string]any {
	overlay := map[string]any{}
	if req.Method != "" {
		overlay["health_probe_method"] = req.Method
	}
	if req.URL != "" {
		overlay["health_probe_url"] = req.URL
	}
	if len(req.Headers) > 0 {
		overlay["health_probe_headers"] = req.Headers
	}
	if req.Body != "" {
		overlay["health_probe_body"] = req.Body
	}
	if len(req.ExpectedStatusCodes) > 0 {
		overlay["health_probe_expected_status_codes"] = req.ExpectedStatusCodes
	}
	if req.ResponseJSONPath != "" {
		overlay["health_probe_response_path"] = req.ResponseJSONPath
	}
	if req.ResponseContains != "" {
		overlay["health_probe_response_contains"] = req.ResponseContains
	}
	return overlay
}

func channelMonitorGlobMatch(pattern, value string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern == "*" {
		return true
	}
	if matched, err := path.Match(pattern, value); err == nil && matched {
		return true
	}
	return strings.EqualFold(pattern, value)
}
