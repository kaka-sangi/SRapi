package service

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
)

// service_proxies.go contains the proxy lifecycle slice of the accounts service
// — CRUD, probing, and batch operations. Extracted from service.go after that
// file outgrew the architectural file-size cap. All helpers and types stay in
// the same package, so callers can keep using `*Service` exactly as before.

func (s *Service) CreateProxy(ctx context.Context, req contract.CreateProxyRequest) (contract.ProxyDefinition, error) {
	name := strings.TrimSpace(req.Name)
	proxyType := req.Type
	rawURL := strings.TrimSpace(req.URL)
	if name == "" || rawURL == "" || !validProxyType(proxyType) {
		return contract.ProxyDefinition{}, ErrInvalidInput
	}
	if err := validateProxyURL(proxyType, rawURL); err != nil {
		return contract.ProxyDefinition{}, err
	}
	ciphertext, err := s.encryptCredential(map[string]any{"url": rawURL})
	if err != nil {
		return contract.ProxyDefinition{}, err
	}
	status := contract.ProxyStatusActive
	if req.Status != nil {
		if !validProxyStatus(*req.Status) {
			return contract.ProxyDefinition{}, ErrInvalidInput
		}
		status = *req.Status
	}
	fallbackMode := contract.ProxyFallbackModeNone
	if req.FallbackMode != nil {
		if !validProxyFallbackMode(*req.FallbackMode) {
			return contract.ProxyDefinition{}, ErrInvalidInput
		}
		fallbackMode = *req.FallbackMode
	}
	backupProxyID := cloneInt(req.BackupProxyID)
	if err := s.validateProxyFallback(ctx, 0, fallbackMode, backupProxyID); err != nil {
		return contract.ProxyDefinition{}, err
	}
	if fallbackMode != contract.ProxyFallbackModeProxy {
		backupProxyID = nil
	}
	countryCode := normalizeCountryCode(req.CountryCode)
	countryName := normalizeCountryName(req.CountryName)
	return s.store.CreateProxy(ctx, contract.CreateStoredProxy{
		Name:          name,
		Type:          proxyType,
		URLCiphertext: ciphertext,
		URLVersion:    credentialVersionV1,
		Status:        status,
		Metadata:      cloneMap(req.Metadata),
		CountryCode:   countryCode,
		CountryName:   countryName,
		ExpiresAt:     cloneTime(req.ExpiresAt),
		FallbackMode:  fallbackMode,
		BackupProxyID: backupProxyID,
	})
}

func (s *Service) UpdateProxy(ctx context.Context, id int, req contract.UpdateProxyRequest) (contract.ProxyDefinition, error) {
	if id <= 0 {
		return contract.ProxyDefinition{}, ErrInvalidInput
	}
	proxy, err := s.store.FindProxyByID(ctx, id)
	if err != nil {
		return contract.ProxyDefinition{}, err
	}
	if proxy.FallbackMode == "" {
		proxy.FallbackMode = contract.ProxyFallbackModeNone
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return contract.ProxyDefinition{}, ErrInvalidInput
		}
		proxy.Name = name
	}
	if req.Type != nil {
		if !validProxyType(*req.Type) {
			return contract.ProxyDefinition{}, ErrInvalidInput
		}
		proxy.Type = *req.Type
	}
	if req.URL != nil {
		rawURL := strings.TrimSpace(*req.URL)
		if rawURL == "" {
			return contract.ProxyDefinition{}, ErrInvalidInput
		}
		if err := validateProxyURL(proxy.Type, rawURL); err != nil {
			return contract.ProxyDefinition{}, err
		}
		ciphertext, err := s.encryptCredential(map[string]any{"url": rawURL})
		if err != nil {
			return contract.ProxyDefinition{}, err
		}
		proxy.URLCiphertext = ciphertext
		proxy.URLVersion = credentialVersionV1
	} else if req.Type != nil && proxy.URLCiphertext != "" {
		rawURL, err := s.decryptProxyURL(proxy)
		if err != nil {
			return contract.ProxyDefinition{}, err
		}
		if err := validateProxyURL(proxy.Type, rawURL); err != nil {
			return contract.ProxyDefinition{}, err
		}
	}
	if req.Status != nil {
		if !validProxyStatus(*req.Status) {
			return contract.ProxyDefinition{}, ErrInvalidInput
		}
		proxy.Status = *req.Status
	}
	if req.Metadata != nil {
		proxy.Metadata = cloneMap(*req.Metadata)
	}
	if req.CountryCode != nil {
		proxy.CountryCode = normalizeCountryCode(req.CountryCode)
	}
	if req.CountryName != nil {
		proxy.CountryName = normalizeCountryName(req.CountryName)
	}
	if req.ClearExpiresAt {
		proxy.ExpiresAt = nil
	}
	if req.ExpiresAt != nil {
		proxy.ExpiresAt = cloneTime(req.ExpiresAt)
	}
	if req.FallbackMode != nil {
		if !validProxyFallbackMode(*req.FallbackMode) {
			return contract.ProxyDefinition{}, ErrInvalidInput
		}
		proxy.FallbackMode = *req.FallbackMode
	}
	if req.ClearBackupProxyID {
		proxy.BackupProxyID = nil
	}
	if req.BackupProxyID != nil {
		proxy.BackupProxyID = cloneInt(req.BackupProxyID)
	}
	if err := s.validateProxyFallback(ctx, proxy.ID, proxy.FallbackMode, proxy.BackupProxyID); err != nil {
		return contract.ProxyDefinition{}, err
	}
	if proxy.FallbackMode != contract.ProxyFallbackModeProxy {
		proxy.BackupProxyID = nil
	}
	proxy.UpdatedAt = s.clock.Now()
	return s.store.UpdateProxy(ctx, proxy)
}

// proxyCounterResetMetadataKey records the last time the rolling success/failure
// counters were zeroed. Lives on metadata so adding a rolling window does not
// require a separate ent column — see RecordProxyProbe.
const proxyCounterResetMetadataKey = "_probe_counter_reset_at"

// proxyCounterResetWindow is how long the rolling availability window is in
// wall-clock time. After this many days of probe results the counters get
// zeroed on the next probe, giving the UI a "since-last-reset" availability
// that approximates a trailing 7-day window without a separate snapshot table.
const proxyCounterResetWindow = 7 * 24 * time.Hour

// RecordProxyProbe folds one probe outcome into the proxy's rolling counters
// and updates last_probed_at + last_probe_latency_ms. Called by the
// proxy_probe worker after each pass and by the operator-initiated "probe
// now" handler so the availability percentage stays current between worker
// ticks.
//
// The counters reset every ~7 days (see proxyCounterResetWindow) so the
// availability percentage is a rolling-window value rather than a lifetime
// success rate — fresh signals weigh as much as old ones after enough time
// passes. The reset timestamp lives in metadata under
// proxyCounterResetMetadataKey to keep the ent schema delta minimal.
func (s *Service) RecordProxyProbe(ctx context.Context, proxyID int, success bool, latencyMs int) (contract.ProxyDefinition, error) {
	if proxyID <= 0 {
		return contract.ProxyDefinition{}, ErrInvalidInput
	}
	// Serialize per-proxy so two concurrent probes (e.g. worker + admin "probe
	// now") can't both observe the same counter snapshot and both write back,
	// dropping one increment. The memory store has its own mutex; the SQL
	// backend is vulnerable to this lost-update race.
	lock := s.proxyLockFor(proxyID)
	lock.Lock()
	defer lock.Unlock()
	proxy, err := s.store.FindProxyByID(ctx, proxyID)
	if err != nil {
		return contract.ProxyDefinition{}, err
	}
	now := s.clock.Now().UTC()
	metadata := cloneMap(proxy.Metadata)
	if metadata == nil {
		metadata = map[string]any{}
	}
	lastReset := metadataOptionalTime(metadata, proxyCounterResetMetadataKey)
	if lastReset == nil || now.Sub(*lastReset) >= proxyCounterResetWindow {
		proxy.ProbeSuccessCount = 0
		proxy.ProbeFailureCount = 0
		metadata[proxyCounterResetMetadataKey] = now.Format(time.RFC3339)
	}
	if success {
		proxy.ProbeSuccessCount++
		if latencyMs > 0 {
			proxy.LastProbeLatencyMs = latencyMs
		}
	} else {
		proxy.ProbeFailureCount++
	}
	probedAt := now
	proxy.LastProbedAt = &probedAt
	proxy.Metadata = metadata
	proxy.UpdatedAt = now
	return s.store.UpdateProxy(ctx, proxy)
}

// proxyLockFor returns a per-proxy mutex used to serialize the
// FindProxyByID → mutate → UpdateProxy sequence in RecordProxyProbe.
// Loaded lazily; stale entries are left in the map (low cardinality in
// practice — one per proxy that has ever been probed).
func (s *Service) proxyLockFor(proxyID int) *sync.Mutex {
	if existing, ok := s.proxyLocks.Load(proxyID); ok {
		return existing.(*sync.Mutex)
	}
	created := &sync.Mutex{}
	actual, _ := s.proxyLocks.LoadOrStore(proxyID, created)
	return actual.(*sync.Mutex)
}

// normalizeCountryCode trims and uppercases the operator-supplied country
// code, capping at the schema's 2-char MaxLen so a stray longer string from a
// future client cannot get past the contract layer.
func normalizeCountryCode(value *string) string {
	if value == nil {
		return ""
	}
	trimmed := strings.ToUpper(strings.TrimSpace(*value))
	if len(trimmed) > 2 {
		trimmed = trimmed[:2]
	}
	return trimmed
}

func normalizeCountryName(value *string) string {
	if value == nil {
		return ""
	}
	trimmed := strings.TrimSpace(*value)
	if len(trimmed) > 128 {
		trimmed = trimmed[:128]
	}
	return trimmed
}

func (s *Service) FindProxyByID(ctx context.Context, id int) (contract.ProxyDefinition, error) {
	if id <= 0 {
		return contract.ProxyDefinition{}, ErrInvalidInput
	}
	return s.store.FindProxyByID(ctx, id)
}

// DeleteProxy soft-deletes a proxy and unbinds any accounts that route through
// it (by id), which fall back to a direct connection.
func (s *Service) DeleteProxy(ctx context.Context, id int) error {
	if id <= 0 {
		return ErrInvalidInput
	}
	if _, err := s.store.FindProxyByID(ctx, id); err != nil {
		return err
	}
	return s.store.SoftDeleteProxy(ctx, id)
}

// defaultProxyTestTarget is a small, plain-text, dependable response over
// HTTPS used by TestProxy when the caller doesn't pass a custom target.
// Cloudflare's /cdn-cgi/trace returns a few hundred bytes of key=value text
// and is widely used for exactly this kind of probe.
const defaultProxyTestTarget = "https://www.cloudflare.com/cdn-cgi/trace"

// proxyLastTestMetadataKey is the reserved metadata key that TestProxy and
// BatchTestProxies use to persist the most recent probe outcome on the proxy
// row. The leading underscore signals "internal — don't touch from the form
// editor"; the value is an object with {at, ok, latency_ms, error_class,
// status_code, target_url}. Stored on metadata to avoid an ent schema change
// for what is essentially a per-row diagnostic cache.
const proxyLastTestMetadataKey = "_last_test"

// batchTestProxyConcurrency caps how many in-flight probes BatchTestProxies
// runs at once. Each one is a real HTTPS handshake — too many simultaneous
// dials would hammer the box, too few would make a moderate selection feel
// slow. 8 lets a 50-row selection finish in well under a minute with the
// 8s per-probe cap.
const batchTestProxyConcurrency = 8

// batchTestProxyMaxRows caps how many proxies a single batch can probe. A
// runaway-loop guard, not a feature limit — typical operator selections are
// well under this.
const batchTestProxyMaxRows = 200

// proxyTestTimeout caps how long a single TestProxy call can wait — a wedged
// proxy or unresponsive target shouldn't tie up an admin browser tab.
const proxyTestTimeout = 8 * time.Second

// TestProxy issues a one-shot HTTPS GET to targetURL routed through the
// proxy. The result categorizes the failure mode for the UI: bad_proxy_url
// when the stored ciphertext won't decrypt, bad_target_url when targetURL
// isn't a usable URL, timeout when the wall-clock cap expires before any
// response, transport_error when the dial / TLS / proxy CONNECT step fails,
// and bad_status when the upstream returns non-2xx. OK is true only on a
// 2xx response.
//
// targetURL may be empty — defaultProxyTestTarget is used in that case.
func (s *Service) TestProxy(ctx context.Context, id int, targetURL string) (contract.ProxyTestResult, error) {
	if id <= 0 {
		return contract.ProxyTestResult{}, ErrInvalidInput
	}
	proxy, err := s.store.FindProxyByID(ctx, id)
	if err != nil {
		return contract.ProxyTestResult{}, err
	}
	target := strings.TrimSpace(targetURL)
	if target == "" {
		target = defaultProxyTestTarget
	}
	parsedTarget, err := url.Parse(target)
	if err != nil || parsedTarget.Scheme == "" || parsedTarget.Host == "" {
		return contract.ProxyTestResult{
			OK:         false,
			ErrorClass: "bad_target_url",
			TargetURL:  target,
		}, nil
	}

	proxyRawURL, err := s.decryptProxyURL(proxy)
	if err != nil {
		return contract.ProxyTestResult{
			OK:         false,
			ErrorClass: "bad_proxy_url",
			TargetURL:  target,
		}, nil
	}
	parsedProxyURL, err := url.Parse(proxyRawURL)
	if err != nil || parsedProxyURL.Scheme == "" || parsedProxyURL.Host == "" {
		return contract.ProxyTestResult{
			OK:         false,
			ErrorClass: "bad_proxy_url",
			TargetURL:  target,
		}, nil
	}

	transport := &http.Transport{Proxy: http.ProxyURL(parsedProxyURL)}
	client := &http.Client{Transport: transport, Timeout: proxyTestTimeout}
	defer transport.CloseIdleConnections()

	probeCtx, cancel := context.WithTimeout(ctx, proxyTestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, target, nil)
	if err != nil {
		return contract.ProxyTestResult{
			OK:         false,
			ErrorClass: "bad_target_url",
			TargetURL:  target,
		}, nil
	}
	start := time.Now()
	resp, err := client.Do(req)
	latencyMS := int(time.Since(start) / time.Millisecond)
	var result contract.ProxyTestResult
	if err != nil {
		// Categorize the error: context-deadline + URL deadline errors map to
		// timeout; anything else is a transport-level failure (dial / TLS /
		// CONNECT refused). The frontend gets a stable, narrow set of classes.
		errClass := "transport_error"
		if errors.Is(err, context.DeadlineExceeded) {
			errClass = "timeout"
		}
		result = contract.ProxyTestResult{
			OK:         false,
			LatencyMS:  latencyMS,
			ErrorClass: errClass,
			TargetURL:  target,
		}
	} else {
		defer resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			result = contract.ProxyTestResult{
				OK:         true,
				LatencyMS:  latencyMS,
				StatusCode: resp.StatusCode,
				TargetURL:  target,
			}
		} else {
			result = contract.ProxyTestResult{
				OK:         false,
				LatencyMS:  latencyMS,
				StatusCode: resp.StatusCode,
				ErrorClass: "bad_status",
				TargetURL:  target,
			}
		}
	}
	// Persist the outcome onto the proxy's metadata so the list view can show
	// a "last test" badge across page loads. A persist failure is silently
	// dropped — the probe result the caller is about to see is what matters;
	// the cache is best-effort.
	_ = s.persistProxyTestResult(ctx, proxy, result)
	return result, nil
}

// persistProxyTestResult merges a result snapshot into proxy.Metadata under
// proxyLastTestMetadataKey and writes the proxy back via the store.
//
// Loads a fresh copy from the store before mutating so we don't race with
// concurrent edits to other metadata fields — the cost is one extra round-trip
// per Test, which is negligible compared to the probe itself.
func (s *Service) persistProxyTestResult(ctx context.Context, original contract.ProxyDefinition, result contract.ProxyTestResult) error {
	fresh, err := s.store.FindProxyByID(ctx, original.ID)
	if err != nil {
		return err
	}
	if fresh.Metadata == nil {
		fresh.Metadata = map[string]any{}
	}
	snapshot := map[string]any{
		"at":          time.Now().UTC().Format(time.RFC3339),
		"ok":          result.OK,
		"latency_ms":  result.LatencyMS,
		"status_code": result.StatusCode,
		"error_class": result.ErrorClass,
		"target_url":  result.TargetURL,
	}
	fresh.Metadata[proxyLastTestMetadataKey] = snapshot
	if _, err := s.store.UpdateProxy(ctx, fresh); err != nil {
		return err
	}
	return nil
}

// BatchTestProxies runs TestProxy for each id in parallel (capped at
// batchTestProxyConcurrency), in input order, with the default target URL
// for every row. Returns one row per requested id — a row whose proxy was
// missing surfaces with ErrorClass="not_found" rather than failing the
// whole call. Caller-visible duplicates in ids are returned verbatim in
// the result (same id appears twice → two rows) so the frontend can
// align by index without re-grouping.
func (s *Service) BatchTestProxies(ctx context.Context, ids []int) ([]contract.ProxyBatchTestRow, error) {
	if len(ids) == 0 || len(ids) > batchTestProxyMaxRows {
		return nil, ErrInvalidInput
	}
	for _, id := range ids {
		if id <= 0 {
			return nil, ErrInvalidInput
		}
	}
	rows := make([]contract.ProxyBatchTestRow, len(ids))
	sem := make(chan struct{}, batchTestProxyConcurrency)
	var wg sync.WaitGroup
	for i, id := range ids {
		i, id := i, id
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			result, err := s.TestProxy(ctx, id, "")
			if err != nil {
				rows[i] = contract.ProxyBatchTestRow{
					ProxyID: id,
					Result: contract.ProxyTestResult{
						OK:         false,
						ErrorClass: "not_found",
						TargetURL:  defaultProxyTestTarget,
					},
				}
				return
			}
			rows[i] = contract.ProxyBatchTestRow{ProxyID: id, Result: result}
		}()
	}
	wg.Wait()
	return rows, nil
}

// BatchCreateProxyResult is per-row outcome from BatchCreateProxies.
// Created is the inserted row, SkippedReason is set when the row was a
// soft-skip (duplicate name), Err is set on a hard validation failure.
type BatchCreateProxyResult struct {
	Index         int
	Name          string
	Created       *contract.ProxyDefinition
	SkippedReason string
	Err           error
}

// BatchCreateProxies inserts many proxies in one call. Dedupes by name
// against existing proxies AND within the request itself — a duplicate name
// surfaces as SkippedReason="duplicate_name" rather than failing the whole
// call. Hard validation errors (bad URL, bad type) surface in Err on that
// row; other rows still succeed. Order matches the input.
func (s *Service) BatchCreateProxies(ctx context.Context, reqs []contract.CreateProxyRequest) ([]BatchCreateProxyResult, error) {
	if len(reqs) == 0 {
		return nil, ErrInvalidInput
	}
	existing, err := s.store.ListProxies(ctx)
	if err != nil {
		return nil, err
	}
	taken := make(map[string]struct{}, len(existing))
	for _, p := range existing {
		taken[strings.ToLower(strings.TrimSpace(p.Name))] = struct{}{}
	}
	out := make([]BatchCreateProxyResult, len(reqs))
	for i, req := range reqs {
		name := strings.TrimSpace(req.Name)
		row := BatchCreateProxyResult{Index: i, Name: name}
		if name == "" {
			row.Err = ErrInvalidInput
			out[i] = row
			continue
		}
		key := strings.ToLower(name)
		if _, dup := taken[key]; dup {
			row.SkippedReason = "duplicate_name"
			out[i] = row
			continue
		}
		created, createErr := s.CreateProxy(ctx, req)
		if createErr != nil {
			row.Err = createErr
		} else {
			taken[key] = struct{}{}
			row.Created = &created
		}
		out[i] = row
	}
	return out, nil
}

// BatchDeleteProxyResult is per-id outcome from BatchDeleteProxies. Err is
// set when the id couldn't be deleted (e.g. not found); successful rows have
// Err == nil. Maintains input order.
type BatchDeleteProxyResult struct {
	ID  int
	Err error
}

// BatchDeleteProxies soft-deletes the named ids. Same semantics as the
// per-id DeleteProxy — accounts routed through a deleted proxy fall back to
// a direct connection. Missing ids surface in Err on that row without
// failing the call.
func (s *Service) BatchDeleteProxies(ctx context.Context, ids []int) ([]BatchDeleteProxyResult, error) {
	if len(ids) == 0 {
		return nil, ErrInvalidInput
	}
	out := make([]BatchDeleteProxyResult, len(ids))
	for i, id := range ids {
		row := BatchDeleteProxyResult{ID: id}
		if err := s.DeleteProxy(ctx, id); err != nil {
			row.Err = err
		}
		out[i] = row
	}
	return out, nil
}
func (s *Service) ListProxies(ctx context.Context) ([]contract.ProxyDefinition, error) {
	proxies, err := s.store.ListProxies(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.ProxyDefinition, 0, len(proxies))
	for _, proxy := range proxies {
		out = append(out, proxy)
	}
	return out, nil
}
