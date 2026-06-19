package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/channel_monitors/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

// ErrInvalidInput is returned for malformed monitor definitions or templates.
var ErrInvalidInput = errors.New("invalid channel monitor")

// ErrDisabled is returned when a disabled monitor is asked to run.
var ErrDisabled = errors.New("channel monitor disabled")

// Service provides admin-managed channel-monitor CRUD, run history, and template apply.
type Service struct {
	store contract.Store
}

func New(store contract.Store) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	return &Service{store: store}, nil
}

func (s *Service) ListDefinitions(ctx context.Context) ([]contract.Definition, error) {
	return s.store.ListDefinitions(ctx)
}

// uptimeWindowDays bounds the rolling-uptime window. Seven days matches the
// horizon admins typically reason about ("did this channel work over the last
// week?"). uptimeWindowRunCap caps how many recent runs we look at per monitor
// so a chatty monitor (interval=30s -> ~20k runs/week) doesn't drag the list
// page. The cap reflects "enough samples to make the percentage meaningful"
// rather than "every run in the window".
const (
	uptimeWindowDays   = 7
	uptimeWindowRunCap = 200
)

// ListDefinitionsWithSummary returns every monitor definition paired with a
// thin summary of its most recent run (or nil when no runs exist), plus a
// rolling uptime aggregate computed from the last uptimeWindowRunCap runs that
// fell inside the rolling window.
//
// The implementation is N+1 — one ListRuns call per monitor — which is fine for
// the admin list page (monitor counts are typically small) and keeps the store
// interface unchanged. A batched store query is the right swap if monitor counts
// grow problematic.
func (s *Service) ListDefinitionsWithSummary(ctx context.Context) ([]contract.DefinitionWithSummary, error) {
	defs, err := s.store.ListDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	since := time.Now().UTC().Add(-uptimeWindowDays * 24 * time.Hour)
	out := make([]contract.DefinitionWithSummary, 0, len(defs))
	for _, def := range defs {
		entry := contract.DefinitionWithSummary{Definition: def}
		// One query pulls both the freshest run (for LastRun) and the recent
		// window for the uptime aggregate — runs are returned newest-first by
		// the store contract.
		runs, err := s.store.ListRuns(ctx, def.ID, uptimeWindowRunCap)
		if err != nil {
			return nil, err
		}
		if len(runs) > 0 {
			r := runs[0]
			entry.LastRun = &contract.RunSummary{
				At:        r.CreatedAt,
				OK:        r.OK,
				LatencyMS: r.LatencyMS,
			}
		}
		var sampleCount, successes int
		for _, r := range runs {
			if r.CreatedAt.Before(since) {
				break
			}
			sampleCount++
			if r.OK {
				successes++
			}
		}
		if sampleCount > 0 {
			entry.Recent = &contract.RecentUptime{
				SampleCount: sampleCount,
				Successes:   successes,
				WindowDays:  uptimeWindowDays,
			}
		}
		out = append(out, entry)
	}
	return out, nil
}

func (s *Service) GetDefinition(ctx context.Context, id int) (contract.Definition, error) {
	if id <= 0 {
		return contract.Definition{}, ErrInvalidInput
	}
	return s.store.GetDefinition(ctx, id)
}

func (s *Service) CreateDefinition(ctx context.Context, input contract.CreateDefinition) (contract.Definition, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return contract.Definition{}, ErrInvalidInput
	}
	scope, ok := normalizeScope(input.Scope)
	if !ok {
		return contract.Definition{}, ErrInvalidInput
	}
	input.Scope = scope
	input.ScopeRef = strings.TrimSpace(input.ScopeRef)
	input.Interval = normalizeInterval(input.Interval)
	input.Model = strings.TrimSpace(input.Model)
	req, err := normalizeRequest(input.Request)
	if err != nil {
		return contract.Definition{}, ErrInvalidInput
	}
	input.Request = req
	return s.store.CreateDefinition(ctx, input)
}

func (s *Service) UpdateDefinition(ctx context.Context, id int, input contract.UpdateDefinition) (contract.Definition, error) {
	if id <= 0 {
		return contract.Definition{}, ErrInvalidInput
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return contract.Definition{}, ErrInvalidInput
		}
		input.Name = &name
	}
	if input.Scope != nil {
		scope, ok := normalizeScope(*input.Scope)
		if !ok {
			return contract.Definition{}, ErrInvalidInput
		}
		input.Scope = &scope
	}
	if input.ScopeRef != nil {
		ref := strings.TrimSpace(*input.ScopeRef)
		input.ScopeRef = &ref
	}
	if input.Interval != nil {
		interval := normalizeInterval(*input.Interval)
		input.Interval = &interval
	}
	if input.Model != nil {
		model := strings.TrimSpace(*input.Model)
		input.Model = &model
	}
	if input.Request != nil {
		req, err := normalizeRequest(*input.Request)
		if err != nil {
			return contract.Definition{}, ErrInvalidInput
		}
		input.Request = &req
	}
	return s.store.UpdateDefinition(ctx, id, input)
}

func (s *Service) DeleteDefinition(ctx context.Context, id int) error {
	if id <= 0 {
		return ErrInvalidInput
	}
	return s.store.DeleteDefinition(ctx, id)
}

func (s *Service) ListTemplates(ctx context.Context) ([]contract.Template, error) {
	return s.store.ListTemplates(ctx)
}

func (s *Service) GetTemplate(ctx context.Context, id int) (contract.Template, error) {
	if id <= 0 {
		return contract.Template{}, ErrInvalidInput
	}
	return s.store.GetTemplate(ctx, id)
}

func (s *Service) CreateTemplate(ctx context.Context, input contract.CreateTemplate) (contract.Template, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return contract.Template{}, ErrInvalidInput
	}
	input.Description = strings.TrimSpace(input.Description)
	req, err := normalizeRequest(input.Request)
	if err != nil {
		return contract.Template{}, ErrInvalidInput
	}
	input.Request = req
	return s.store.CreateTemplate(ctx, input)
}

func (s *Service) UpdateTemplate(ctx context.Context, id int, input contract.UpdateTemplate) (contract.Template, error) {
	if id <= 0 {
		return contract.Template{}, ErrInvalidInput
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return contract.Template{}, ErrInvalidInput
		}
		input.Name = &name
	}
	if input.Description != nil {
		desc := strings.TrimSpace(*input.Description)
		input.Description = &desc
	}
	if input.Request != nil {
		req, err := normalizeRequest(*input.Request)
		if err != nil {
			return contract.Template{}, ErrInvalidInput
		}
		input.Request = &req
	}
	return s.store.UpdateTemplate(ctx, id, input)
}

func (s *Service) DeleteTemplate(ctx context.Context, id int) error {
	if id <= 0 {
		return ErrInvalidInput
	}
	return s.store.DeleteTemplate(ctx, id)
}

// ApplyTemplate copies a template's custom request onto every supplied monitor
// definition and returns the updated definitions. monitorIDs that do not exist
// are skipped; the count of applied monitors is returned.
func (s *Service) ApplyTemplate(ctx context.Context, templateID int, monitorIDs []int) ([]contract.Definition, error) {
	if templateID <= 0 || len(monitorIDs) == 0 {
		return nil, ErrInvalidInput
	}
	template, err := s.store.GetTemplate(ctx, templateID)
	if err != nil {
		return nil, err
	}
	request := template.Request
	applied := make([]contract.Definition, 0, len(monitorIDs))
	for _, id := range monitorIDs {
		if id <= 0 {
			continue
		}
		def, err := s.store.UpdateDefinition(ctx, id, contract.UpdateDefinition{Request: &request})
		if err != nil {
			if errors.Is(err, contract.ErrNotFound) {
				continue
			}
			return nil, err
		}
		applied = append(applied, def)
	}
	return applied, nil
}

func (s *Service) RecordRun(ctx context.Context, input contract.RecordRun) (contract.RunResult, error) {
	if input.MonitorID <= 0 || strings.TrimSpace(input.RunID) == "" {
		return contract.RunResult{}, ErrInvalidInput
	}
	if strings.TrimSpace(input.Trigger) == "" {
		input.Trigger = contract.TriggerManual
	}
	return s.store.RecordRun(ctx, input)
}

func (s *Service) ListRuns(ctx context.Context, monitorID int, limit int) ([]contract.RunResult, error) {
	if monitorID <= 0 {
		return nil, ErrInvalidInput
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.store.ListRuns(ctx, monitorID, limit)
}

// RunDefinition executes one enabled monitor definition and records the run.
func (s *Service) RunDefinition(ctx context.Context, id int, deps contract.RunnerDependencies, trigger string) (contract.RunResult, error) {
	if id <= 0 || !validRunnerDependencies(deps) {
		return contract.RunResult{}, ErrInvalidInput
	}
	def, err := s.store.GetDefinition(ctx, id)
	if err != nil {
		return contract.RunResult{}, err
	}
	if !def.Enabled {
		return contract.RunResult{}, ErrDisabled
	}
	return s.runDefinition(ctx, def, deps, trigger)
}

// RunDue executes enabled monitor definitions whose interval has elapsed.
func (s *Service) RunDue(ctx context.Context, deps contract.RunnerDependencies, now time.Time, limit int) ([]contract.RunResult, error) {
	if !validRunnerDependencies(deps) {
		return nil, ErrInvalidInput
	}
	defs, err := s.store.ListDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	runs := make([]contract.RunResult, 0)
	var firstErr error
	for _, def := range defs {
		if limit > 0 && len(runs) >= limit {
			break
		}
		if !def.Enabled {
			continue
		}
		lastRuns, err := s.store.ListRuns(ctx, def.ID, 1)
		if err != nil {
			firstErr = errors.Join(firstErr, err)
			continue
		}
		if !definitionDue(def, lastRuns, now) {
			continue
		}
		run, err := s.runDefinition(ctx, def, deps, contract.TriggerScheduled)
		if err != nil {
			firstErr = errors.Join(firstErr, err)
			continue
		}
		runs = append(runs, run)
	}
	return runs, firstErr
}

func validRunnerDependencies(deps contract.RunnerDependencies) bool {
	return deps.Accounts != nil && deps.Providers != nil && deps.Models != nil && deps.Adapter != nil
}

func definitionDue(def contract.Definition, runs []contract.RunResult, now time.Time) bool {
	if !def.Enabled {
		return false
	}
	interval := time.Duration(normalizeInterval(def.Interval)) * time.Second
	if len(runs) == 0 {
		return true
	}
	return !runs[0].CreatedAt.Add(interval).After(now.UTC())
}

func (s *Service) runDefinition(ctx context.Context, def contract.Definition, deps contract.RunnerDependencies, trigger string) (contract.RunResult, error) {
	startedAt := time.Now().UTC()
	accounts, err := resolveMonitorAccounts(ctx, deps, def)
	if err != nil {
		return contract.RunResult{}, err
	}
	results := make([]contract.CheckResult, 0, len(accounts))
	okCount := 0
	for _, account := range accounts {
		result := runMonitorProbe(ctx, deps, def, account)
		if result.OK {
			okCount++
		}
		results = append(results, result)
	}
	runID := fmt.Sprintf("monitor_%d_%d", def.ID, time.Now().UnixNano())
	run, err := s.RecordRun(ctx, contract.RecordRun{
		MonitorID:    def.ID,
		RunID:        runID,
		OK:           len(results) > 0 && okCount == len(results),
		CheckedCount: len(results),
		OKCount:      okCount,
		LatencyMS:    elapsedMillis(startedAt),
		Trigger:      trigger,
		Results:      results,
	})
	if err != nil {
		return contract.RunResult{}, err
	}
	return run, nil
}

func resolveMonitorAccounts(ctx context.Context, deps contract.RunnerDependencies, def contract.Definition) ([]accountcontract.ProviderAccount, error) {
	all, err := deps.Accounts.List(ctx)
	if err != nil {
		return nil, err
	}
	switch def.Scope {
	case contract.ScopeAccount:
		accountID, _ := strconv.Atoi(strings.TrimSpace(def.ScopeRef))
		for _, account := range all {
			if account.ID == accountID {
				return []accountcontract.ProviderAccount{account}, nil
			}
		}
		return nil, nil
	case contract.ScopeProvider:
		providerID, _ := strconv.Atoi(strings.TrimSpace(def.ScopeRef))
		out := make([]accountcontract.ProviderAccount, 0)
		for _, account := range all {
			if account.ProviderID == providerID {
				out = append(out, account)
			}
		}
		return out, nil
	case contract.ScopeGroup:
		groupID, _ := strconv.Atoi(strings.TrimSpace(def.ScopeRef))
		members, err := deps.Accounts.ListGroupMembers(ctx, groupID)
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
	case contract.ScopeModel:
		return resolveMonitorModelAccounts(ctx, deps, def, all)
	default:
		return nil, nil
	}
}

func resolveMonitorModelAccounts(ctx context.Context, deps contract.RunnerDependencies, def contract.Definition, all []accountcontract.ProviderAccount) ([]accountcontract.ProviderAccount, error) {
	pattern := strings.TrimSpace(def.ScopeRef)
	if pattern == "" {
		return nil, nil
	}
	models, err := deps.Models.List(ctx)
	if err != nil {
		return nil, err
	}
	providerIDs := make(map[int]struct{})
	for _, model := range models {
		if !globMatch(pattern, model.CanonicalName) {
			continue
		}
		mappings, err := deps.Models.ListMappingsByModel(ctx, model.ID)
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

func runMonitorProbe(ctx context.Context, deps contract.RunnerDependencies, def contract.Definition, account accountcontract.ProviderAccount) contract.CheckResult {
	model := strings.TrimSpace(def.Model)
	result := contract.CheckResult{
		AccountID:   account.ID,
		AccountName: account.Name,
		ProviderID:  account.ProviderID,
		Model:       model,
	}
	provider, err := deps.Providers.FindByID(ctx, account.ProviderID)
	if err != nil {
		result.ErrorClass = "provider_not_found"
		result.StatusCode = http.StatusBadRequest
		return result
	}
	credential, err := deps.Accounts.DecryptCredential(ctx, account.ID)
	if err != nil {
		result.ErrorClass = "credential_decrypt_failed"
		result.StatusCode = http.StatusInternalServerError
		return result
	}
	overlay := probeMetadata(def.Request)
	probeAccount := account
	probeAccount.Metadata = mergeMetadata(account.Metadata, overlay)
	resp, err := deps.Adapter.ProbeAccount(ctx, provideradaptercontract.ProbeRequest{
		Provider:   provider,
		Account:    probeAccount,
		Model:      model,
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

func probeMetadata(req contract.CustomRequest) map[string]any {
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

func mergeMetadata(base map[string]any, overlay map[string]any) map[string]any {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	out := make(map[string]any, len(base)+len(overlay))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range overlay {
		out[key] = value
	}
	return out
}

func globMatch(pattern, value string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern == "*" {
		return true
	}
	if matched, err := path.Match(pattern, value); err == nil && matched {
		return true
	}
	return strings.EqualFold(pattern, value)
}

func elapsedMillis(startedAt time.Time) int {
	if startedAt.IsZero() {
		return 0
	}
	return int(time.Since(startedAt) / time.Millisecond)
}

func normalizeScope(scope contract.Scope) (contract.Scope, bool) {
	switch contract.Scope(strings.ToLower(strings.TrimSpace(string(scope)))) {
	case contract.ScopeAccount, "":
		return contract.ScopeAccount, true
	case contract.ScopeGroup:
		return contract.ScopeGroup, true
	case contract.ScopeProvider:
		return contract.ScopeProvider, true
	case contract.ScopeModel:
		return contract.ScopeModel, true
	default:
		return "", false
	}
}

func normalizeInterval(interval int) int {
	if interval < 30 {
		return 30
	}
	if interval > 86400 {
		return 86400
	}
	return interval
}

func normalizeRequest(req contract.CustomRequest) (contract.CustomRequest, error) {
	out := contract.CustomRequest{
		Method:           strings.ToUpper(strings.TrimSpace(req.Method)),
		URL:              strings.TrimSpace(req.URL),
		Body:             strings.TrimSpace(req.Body),
		ResponseJSONPath: strings.TrimSpace(req.ResponseJSONPath),
		ResponseContains: strings.TrimSpace(req.ResponseContains),
	}
	switch out.Method {
	case "", "GET", "HEAD", "POST":
	default:
		return contract.CustomRequest{}, errors.New("unsupported probe method")
	}
	if out.Body != "" && !json.Valid([]byte(out.Body)) {
		return contract.CustomRequest{}, errors.New("probe body must be valid json")
	}
	out.Headers = cleanHeaders(req.Headers)
	out.ExpectedStatusCodes = cleanStatusCodes(req.ExpectedStatusCodes)
	return out, nil
}

func cleanHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cleanStatusCodes(codes []int) []int {
	if len(codes) == 0 {
		return nil
	}
	out := make([]int, 0, len(codes))
	seen := map[int]struct{}{}
	for _, code := range codes {
		if code < 100 || code > 599 {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		out = append(out, code)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
