package httpserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/copilot"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

const copilotSecretVersion = "copilotv1"

// handleAdminCopilotConfig reports the copilot's runtime state so the page can
// render an enabled/disabled view without exposing settings internals.
func (s *Server) handleAdminCopilotConfig(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFromContext(r.Context())
	if _, err := s.requireAdminSession(r); err != nil {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "admin access required", requestID)
		return
	}
	settings, ciphertext, err := s.copilotSettings(r.Context())
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	models, protocol := s.copilotModelList(r.Context(), settings)
	writeJSONAny(w, http.StatusOK, apiopenapi.AdminCopilotConfigResponse{
		Data: apiopenapi.AdminCopilotConfig{
			Enabled:             settings.Enabled,
			Source:              apiopenapi.AdminCopilotConfigSource(settings.Source),
			Model:               settings.Model,
			Models:              models,
			Protocol:            protocol,
			OwnerOnly:           settings.OwnerOnly,
			Configured:          copilotConfigError(settings, ciphertext) == "",
			WebSearchConfigured: settings.WebSearchEnabled && strings.TrimSpace(settings.WebSearchAPIKeyCiphertext) != "",
			WebSearchProvider:   settings.WebSearchProvider,
		},
		RequestId: requestID,
	})
}

// handleAdminCopilotChat runs one agentic copilot turn and streams events as SSE.
func (s *Server) handleAdminCopilotChat(w http.ResponseWriter, r *http.Request) {
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
	settings, ciphertext, err := s.copilotSettings(r.Context())
	if err != nil {
		writeAdminControlError(w, err, requestID)
		return
	}
	if !settings.Enabled {
		writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, "the admin copilot is disabled", requestID)
		return
	}
	if settings.OwnerOnly && !sessionHasRole(session.User.Roles, userscontract.RoleOwner) {
		writeStandardError(w, http.StatusForbidden, apiopenapi.FORBIDDEN, "owner access required", requestID)
		return
	}
	if msg := copilotConfigError(settings, ciphertext); msg != "" {
		writeStandardError(w, http.StatusConflict, apiopenapi.RESOURCECONFLICT, msg, requestID)
		return
	}
	if s.runtime.copilotEngine == nil {
		writeStandardError(w, http.StatusInternalServerError, apiopenapi.INTERNALERROR, "copilot unavailable", requestID)
		return
	}

	settings.SystemSummary = s.buildCopilotSummary(r.Context())

	var body apiopenapi.AdminCopilotChatRequest
	if err := s.decodeJSONBody(w, r, &body); err != nil {
		writeStandardError(w, jsonDecodeStatus(err), apiopenapi.INVALIDREQUEST, "invalid copilot request", requestID)
		return
	}
	history := copilotHistoryFromAPI(body.Messages)
	var approval *copilot.Approval
	if body.Approval != nil {
		approval = &copilot.Approval{ToolCallID: body.Approval.ToolCallId, Approved: body.Approval.Approved}
	}
	overrideModel := ""
	if body.Model != nil {
		overrideModel = *body.Model
	}
	effort := ""
	if body.ReasoningEffort != nil {
		effort = string(*body.ReasoningEffort)
	}

	setSSEResponseHeaders(w)
	w.Header().Set("X-Request-ID", requestID)
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	emit := func(ev copilot.Event) {
		data, mErr := json.Marshal(ev.Data)
		if mErr != nil {
			fmt.Fprintf(w, "event: error\ndata: {\"message\":\"internal encoding error\"}\n\n")
			if flusher != nil {
				flusher.Flush()
			}
			return
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data)
		if flusher != nil {
			flusher.Flush()
		}
	}

	llm := s.buildCopilotLLM(settings, ciphertext, overrideModel, effort)
	dispatch := s.buildCopilotDispatcher(r, session.User.ID, r.Header.Get("Cookie"), r.Header.Get(csrfHeaderName))
	search := s.buildCopilotSearch(settings)
	_, _ = s.runtime.copilotEngine.Run(r.Context(), settings, history, approval, llm, dispatch, search, emit)
}

// buildCopilotSearch returns a web-search backend bound to the configured
// provider + decrypted key, or nil when web search is disabled/unconfigured (in
// which case the engine offers no web_search tool). Works with any model.
func (s *Server) buildCopilotSearch(settings copilot.Settings) copilot.SearchFunc {
	if !settings.WebSearchEnabled || strings.TrimSpace(settings.WebSearchAPIKeyCiphertext) == "" {
		return nil
	}
	key, err := s.decryptCopilotSecret(settings.WebSearchAPIKeyCiphertext)
	if err != nil || strings.TrimSpace(key) == "" {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(settings.WebSearchProvider)) {
	case "brave":
		return copilot.NewBraveSearch(key, settings.WebSearchBaseURL)
	default: // tavily is the default
		return copilot.NewTavilySearch(key, settings.WebSearchBaseURL)
	}
}

// buildCopilotLLM returns an invoker bound to the configured source (an existing
// provider account, or a standalone dedicated key), the chosen model, and the
// requested thinking effort.
func (s *Server) buildCopilotLLM(settings copilot.Settings, ciphertext, overrideModel, effort string) copilot.LLMFunc {
	model := strings.TrimSpace(overrideModel)
	if model == "" {
		model = settings.Model
	}
	maxTokens := settings.MaxOutputTokens
	if maxTokens <= 0 {
		maxTokens = 8192
	}
	if budget := copilotEffortBudget(effort); budget > 0 && budget+4096 > maxTokens {
		maxTokens = budget + 4096
	}
	return func(ctx context.Context, system string, messages []provideradaptercontract.ConversationMessage, tools []map[string]any, onDelta func(kind, text string)) (provideradaptercontract.ConversationResponse, error) {
		req := provideradaptercontract.ConversationRequest{
			RequestID:       newRequestID(),
			SourceEndpoint:  "/internal/admin/copilot",
			Model:           model,
			Mapping:         modelcontract.ModelProviderMapping{UpstreamModelName: model},
			Instructions:    system,
			Messages:        messages,
			Tools:           tools,
			ToolChoice:      "auto",
			MaxOutputTokens: &maxTokens,
		}
		if settings.Source == "dedicated" {
			key, err := s.decryptCopilotSecret(ciphertext)
			if err != nil {
				return provideradaptercontract.ConversationResponse{}, fmt.Errorf("decrypt dedicated key: %w", err)
			}
			// Default an unset protocol to OpenAI-compatible (the dominant dedicated
			// case, e.g. DeepSeek). An empty protocol would make source != target
			// fail the same-protocol check and silently drop to the buffered,
			// non-streaming path.
			protocol := strings.TrimSpace(settings.DedicatedProtocol)
			if protocol == "" {
				protocol = "openai-compatible"
			}
			req.SourceProtocol = protocol
			req.TargetProtocol = protocol
			req.Provider = providercontract.Provider{
				Name:         "copilot-dedicated",
				DisplayName:  "Copilot dedicated",
				Protocol:     protocol,
				AdapterType:  protocol,
				ConfigSchema: map[string]any{"base_url": settings.DedicatedBaseURL},
			}
			req.Credential = map[string]any{"api_key": key}
		} else {
			accountID := settings.ProviderAccountID
			if settings.ProviderAccountGroupID > 0 {
				members, merr := s.runtime.accounts.ListGroupMembers(ctx, settings.ProviderAccountGroupID)
				if merr == nil && len(members) > 0 {
					accountID = members[rand.Intn(len(members))].AccountID
				}
			}
			account, err := s.runtime.accounts.FindByID(ctx, accountID)
			if err != nil {
				return provideradaptercontract.ConversationResponse{}, fmt.Errorf("load provider account: %w", err)
			}
			provider, err := s.runtime.providers.FindByID(ctx, account.ProviderID)
			if err != nil {
				return provideradaptercontract.ConversationResponse{}, fmt.Errorf("load provider: %w", err)
			}
			credential, err := s.runtime.accounts.DecryptCredential(ctx, account.ID)
			if err != nil {
				return provideradaptercontract.ConversationResponse{}, fmt.Errorf("decrypt credential: %w", err)
			}
			req.SourceProtocol = provider.Protocol
			req.TargetProtocol = provider.Protocol
			req.Provider = provider
			req.Account = account
			req.Credential = credential
		}
		applyCopilotReasoning(&req, effort)

		// Prefer same-protocol passthrough streaming so the client sees tokens as
		// they arrive; fall back to a single buffered call when streaming isn't
		// available (cross-protocol, reverse-proxy runtimes that can't stream).
		streamReq := req
		streamReq.Stream = true
		if streamResp, err := s.runtime.adapters.StreamConversation(ctx, streamReq); err == nil && streamResp.StreamBody != nil {
			defer func() { _ = streamResp.StreamBody.Close() }()
			raw, content, reasoning, streamToolCalls := consumeCopilotStream(streamResp.StreamBody, onDelta)
			status := streamResp.StatusCode
			if status == 0 {
				status = http.StatusOK
			}
			if streamResp.StreamParse != nil {
				if final, perr := streamResp.StreamParse(raw, status); perr == nil {
					return final, nil
				}
			}
			parts := make([]provideradaptercontract.ContentPart, 0, 2+len(streamToolCalls))
			if reasoning != "" {
				parts = append(parts, provideradaptercontract.ContentPart{Kind: provideradaptercontract.ContentPartThinking, Text: reasoning})
			}
			if content != "" {
				parts = append(parts, provideradaptercontract.ContentPart{Kind: provideradaptercontract.ContentPartText, Text: content})
			}
			parts = append(parts, streamToolCalls...)
			return provideradaptercontract.ConversationResponse{Parts: parts, StopReason: provideradaptercontract.StopReasonEndTurn, StatusCode: status}, nil
		} else if err != nil && !errors.Is(err, provideradaptercontract.ErrStreamingUnsupported) {
			return provideradaptercontract.ConversationResponse{}, err
		}

		// Buffered fallback: one call, then surface its content/reasoning once.
		req.Stream = false
		resp, err := s.runtime.adapters.InvokeConversation(ctx, req)
		if err != nil {
			return resp, err
		}
		for _, part := range resp.Parts {
			switch part.Kind {
			case provideradaptercontract.ContentPartThinking:
				onDelta("reasoning", part.Text)
			case provideradaptercontract.ContentPartText:
				onDelta("content", part.Text)
			}
		}
		return resp, nil
	}
}

// consumeCopilotStream reads an OpenAI-style SSE stream, forwarding content and
// reasoning_content deltas via onDelta as they arrive, and returns the raw bytes
// (for StreamParse), the accumulated content/reasoning, and any tool calls
// parsed from delta.tool_calls chunks.
func consumeCopilotStream(body io.Reader, onDelta func(kind, text string)) (raw []byte, content, reasoning string, toolCalls []provideradaptercontract.ContentPart) {
	type streamToolCall struct {
		id   string
		name string
		args strings.Builder
	}
	var rawBuf bytes.Buffer
	var contentBuf, reasoningBuf strings.Builder
	toolCallsByIdx := map[int]*streamToolCall{}
	scanner := bufio.NewScanner(io.TeeReader(body, &rawBuf))
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
					ToolCalls        []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil || len(chunk.Choices) == 0 {
			continue
		}
		if rc := chunk.Choices[0].Delta.ReasoningContent; rc != "" {
			reasoningBuf.WriteString(rc)
			onDelta("reasoning", rc)
		}
		if cc := chunk.Choices[0].Delta.Content; cc != "" {
			contentBuf.WriteString(cc)
			onDelta("content", cc)
		}
		for _, tc := range chunk.Choices[0].Delta.ToolCalls {
			stc, ok := toolCallsByIdx[tc.Index]
			if !ok {
				stc = &streamToolCall{}
				toolCallsByIdx[tc.Index] = stc
			}
			if tc.ID != "" {
				stc.id = tc.ID
			}
			if tc.Function.Name != "" {
				stc.name = tc.Function.Name
			}
			stc.args.WriteString(tc.Function.Arguments)
		}
	}
	if len(toolCallsByIdx) > 0 {
		maxIdx := 0
		for idx := range toolCallsByIdx {
			if idx > maxIdx {
				maxIdx = idx
			}
		}
		for i := 0; i <= maxIdx; i++ {
			stc, ok := toolCallsByIdx[i]
			if !ok {
				continue
			}
			toolCalls = append(toolCalls, provideradaptercontract.ContentPart{
				Kind:              provideradaptercontract.ContentPartToolUse,
				ToolCallID:        stc.id,
				ToolName:          stc.name,
				ToolArgumentsJSON: stc.args.String(),
			})
		}
	}
	return rawBuf.Bytes(), contentBuf.String(), reasoningBuf.String(), toolCalls
}

// copilotEffortBudget maps a thinking-effort level to an Anthropic/Gemini
// thinking-token budget. Returns 0 for "off"/unset.
func copilotEffortBudget(effort string) int {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "low":
		return 2048
	case "medium":
		return 8192
	case "high":
		return 16384
	default:
		return 0
	}
}

// applyCopilotReasoning wires the thinking effort into the request using the
// mechanism each protocol understands: Anthropic reads req.Reasoning natively;
// OpenAI/Gemini take a payload-transform override (reasoning_effort /
// thinkingConfig). A no-op for "off"/unset.
func applyCopilotReasoning(req *provideradaptercontract.ConversationRequest, effort string) {
	effort = strings.ToLower(strings.TrimSpace(effort))
	budget := copilotEffortBudget(effort)
	if budget == 0 {
		return
	}
	protocol := strings.ToLower(strings.TrimSpace(req.TargetProtocol))
	switch {
	case strings.Contains(protocol, "anthropic") || strings.Contains(protocol, "bedrock") || strings.Contains(protocol, "claude"):
		req.Reasoning = map[string]any{"type": "enabled", "budget_tokens": budget}
	case strings.Contains(protocol, "gemini"):
		req.PayloadTransforms = append(req.PayloadTransforms, provideradaptercontract.PayloadTransform{
			Action: "override", Path: "generationConfig.thinkingConfig.thinkingBudget", Value: budget,
		})
	default: // openai-compatible and friends
		req.PayloadTransforms = append(req.PayloadTransforms, provideradaptercontract.PayloadTransform{
			Action: "override", Path: "reasoning_effort", Value: effort,
		})
	}
}

// buildCopilotDispatcher returns a dispatcher that replays an admin call through
// the real router, carrying the admin's own session + CSRF so auth, validation,
// and audit all apply. Each executed mutation gets an extra "via copilot" audit
// entry on top of the handler's own record.
func (s *Server) buildCopilotDispatcher(r *http.Request, actorUserID int, cookie, csrf string) copilot.DispatchFunc {
	return func(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
		if s.runtime.internalRouter == nil {
			return 0, nil, errors.New("internal router unavailable")
		}
		target := "http://copilot.internal" + path
		req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
		if err != nil {
			return 0, nil, err
		}
		if cookie != "" {
			req.Header.Set("Cookie", cookie)
		}
		if csrf != "" {
			req.Header.Set(csrfHeaderName, csrf)
		}
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		s.runtime.internalRouter.ServeHTTP(rec, req)
		if copilotIsMutation(method) {
			s.runtime.recordAudit(ctx, auditRecordFromRequest(r, actorUserID, "copilot.action", "copilot", path, nil, map[string]any{
				"method": method,
				"path":   path,
				"status": rec.Code,
				"via":    "copilot",
			}))
		}
		// Redact secret/PII-bearing fields before the result reaches the LLM (and
		// the UI). The copilot's model may be a third party; admin secrets must
		// never be sent to it.
		return rec.Code, redactSensitiveJSON(rec.Body.Bytes()), nil
	}
}

// redactSensitiveJSON parses a JSON body and replaces values under secret-like
// keys (api_key, token, password, credential, …) with a redaction marker,
// recursively. Non-JSON bodies are returned unchanged.
func redactSensitiveJSON(body []byte) []byte {
	if len(bytes.TrimSpace(body)) == 0 {
		return body
	}
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return body
	}
	redacted := redactSensitiveValue("", decoded)
	out, err := json.Marshal(redacted)
	if err != nil {
		return body
	}
	return out
}

func redactSensitiveValue(key string, value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for k, v := range typed {
			if sensitiveMetadataKey(k) && isScalarOrString(v) {
				out[k] = "***redacted***"
				continue
			}
			out[k] = redactSensitiveValue(k, v)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, v := range typed {
			out[i] = redactSensitiveValue(key, v)
		}
		return out
	default:
		return typed
	}
}

func isScalarOrString(v any) bool {
	switch v.(type) {
	case string, float64, bool, nil:
		return true
	default:
		return false
	}
}

func (s *Server) copilotSettings(ctx context.Context) (copilot.Settings, string, error) {
	settings, err := s.runtime.adminControl.GetAdminSettings(ctx)
	if err != nil {
		return copilot.Settings{}, "", err
	}
	c := settings.Copilot
	return copilot.Settings{
		Enabled:           c.Enabled,
		Source:            c.Source,
		ProviderAccountID:      c.ProviderAccountID,
		ProviderAccountGroupID: c.ProviderAccountGroupID,
		Model:                  c.Model,
		Models:            append([]string(nil), c.Models...),
		DedicatedProtocol: c.DedicatedProtocol,
		DedicatedBaseURL:  c.DedicatedBaseURL,
		OwnerOnly:         c.OwnerOnly,
		AutoRunReads:      c.AutoRunReads,
		MaxOutputTokens:   c.MaxOutputTokens,

		WebSearchEnabled:          c.WebSearchEnabled,
		WebSearchProvider:         c.WebSearchProvider,
		WebSearchBaseURL:          c.WebSearchBaseURL,
		WebSearchAPIKeyCiphertext: c.WebSearchAPIKeyCiphertext,
	}, c.DedicatedAPIKeyCiphertext, nil
}

// copilotModelList resolves the models offered in the composer picker and the
// configured source's wire protocol. Models = configured list ∪ the account's
// discovered models ∪ the default — deduped, default first.
func (s *Server) copilotModelList(ctx context.Context, settings copilot.Settings) (models []string, protocol string) {
	protocol = strings.TrimSpace(settings.DedicatedProtocol)
	seen := map[string]bool{}
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		models = append(models, name)
	}
	add(settings.Model)
	for _, m := range settings.Models {
		add(m)
	}
	if settings.Source == "account" && settings.ProviderAccountID > 0 {
		if account, err := s.runtime.accounts.FindByID(ctx, settings.ProviderAccountID); err == nil {
			for _, m := range accountSupportedModels(account.Metadata) {
				add(m)
			}
			if provider, err := s.runtime.providers.FindByID(ctx, account.ProviderID); err == nil {
				protocol = provider.Protocol
			}
		}
	}
	if settings.Source == "account" && settings.ProviderAccountGroupID > 0 {
		if members, err := s.runtime.accounts.ListGroupMembers(ctx, settings.ProviderAccountGroupID); err == nil {
			for _, member := range members {
				if account, err := s.runtime.accounts.FindByID(ctx, member.AccountID); err == nil {
					for _, m := range accountSupportedModels(account.Metadata) {
						add(m)
					}
					if protocol == "" {
						if provider, err := s.runtime.providers.FindByID(ctx, account.ProviderID); err == nil {
							protocol = provider.Protocol
						}
					}
				}
			}
		}
	}
	if models == nil {
		models = []string{}
	}
	return models, protocol
}

const copilotSummaryCacheTTL = 60 * time.Second

// buildCopilotSummary produces a compact text snapshot of the system state for
// the copilot's system prompt — counts, health, etc. so the model starts with
// situational awareness and can give more relevant first responses. Best-effort:
// any error silently yields an empty string. Results are cached for 60 seconds
// to avoid re-querying accounts/providers/models on every chat message.
func (s *Server) buildCopilotSummary(ctx context.Context) string {
	s.runtime.copilotSummaryMu.Lock()
	if time.Since(s.runtime.copilotSummaryCachedAt) < copilotSummaryCacheTTL {
		cached := s.runtime.copilotSummaryCache
		s.runtime.copilotSummaryMu.Unlock()
		return cached
	}
	s.runtime.copilotSummaryMu.Unlock()

	// Build fresh summary.
	var parts []string

	// Accounts summary
	if accounts, err := s.runtime.accounts.List(ctx); err == nil {
		total := len(accounts)
		active, errored, disabled := 0, 0, 0
		for _, a := range accounts {
			switch a.Status {
			case accountcontract.StatusActive:
				active++
			case accountcontract.StatusDead, accountcontract.StatusNeedsReauth, accountcontract.StatusSuspended:
				errored++
			case accountcontract.StatusDisabled:
				disabled++
			}
		}
		parts = append(parts, fmt.Sprintf("Accounts: %d total (%d active, %d errored, %d disabled)", total, active, errored, disabled))
	}

	// Providers
	if providers, err := s.runtime.providers.List(ctx); err == nil {
		parts = append(parts, fmt.Sprintf("Providers: %d configured", len(providers)))
	}

	// Models
	if models, err := s.runtime.models.List(ctx); err == nil {
		parts = append(parts, fmt.Sprintf("Models: %d defined", len(models)))
	}

	var result string
	if len(parts) > 0 {
		result = "Current system state:\n" + strings.Join(parts, "\n")
	}

	// Cache the result.
	s.runtime.copilotSummaryMu.Lock()
	s.runtime.copilotSummaryCache = result
	s.runtime.copilotSummaryCachedAt = time.Now()
	s.runtime.copilotSummaryMu.Unlock()
	return result
}

// accountSupportedModels reads the model ids discovered for an account.
func accountSupportedModels(metadata map[string]any) []string {
	raw, ok := metadata["supported_models"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

func copilotConfigError(settings copilot.Settings, ciphertext string) string {
	if strings.TrimSpace(settings.Model) == "" {
		return "no copilot model configured"
	}
	if settings.Source == "dedicated" {
		if strings.TrimSpace(settings.DedicatedBaseURL) == "" {
			return "dedicated base URL is not configured"
		}
		if strings.TrimSpace(ciphertext) == "" {
			return "dedicated API key is not configured"
		}
		return ""
	}
	if settings.ProviderAccountID <= 0 && settings.ProviderAccountGroupID <= 0 {
		return "no provider account selected for the copilot"
	}
	return ""
}

func copilotHistoryFromAPI(messages []apiopenapi.AdminCopilotMessage) []copilot.Message {
	out := make([]copilot.Message, 0, len(messages))
	for _, m := range messages {
		msg := copilot.Message{Role: string(m.Role)}
		if m.Content != nil {
			msg.Content = *m.Content
		}
		if m.Reasoning != nil {
			msg.Reasoning = *m.Reasoning
		}
		if m.Images != nil {
			for _, img := range *m.Images {
				msg.Images = append(msg.Images, copilot.MessageImage{MIMEType: img.MimeType, Data: img.Data})
			}
		}
		if m.ToolCalls != nil {
			for _, tc := range *m.ToolCalls {
				msg.ToolCalls = append(msg.ToolCalls, copilot.ToolCall{ID: tc.Id, Name: tc.Name, ArgumentsJSON: tc.Arguments})
			}
		}
		if m.ToolResults != nil {
			for _, tr := range *m.ToolResults {
				isErr := false
				if tr.IsError != nil {
					isErr = *tr.IsError
				}
				msg.ToolResults = append(msg.ToolResults, copilot.ToolResult{ToolCallID: tr.ToolCallId, Content: tr.Content, IsError: isErr})
			}
		}
		out = append(out, msg)
	}
	return out
}

func sessionHasRole(roles []userscontract.Role, target userscontract.Role) bool {
	for _, role := range roles {
		if role == target {
			return true
		}
	}
	return false
}

func copilotIsMutation(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func (s *Server) encryptCopilotSecret(plaintext string) (string, error) {
	return s.encryptMasterSecret(plaintext, copilotSecretVersion)
}

func (s *Server) decryptCopilotSecret(ciphertext string) (string, error) {
	return s.decryptMasterSecret(ciphertext, copilotSecretVersion)
}

const oauthSecretVersion = "oauth_v1"

func (s *Server) encryptOAuthClientSecret(plaintext string) (string, error) {
	return s.encryptMasterSecret(plaintext, oauthSecretVersion)
}

func (s *Server) decryptOAuthClientSecret(ciphertext string) (string, error) {
	return s.decryptMasterSecret(ciphertext, oauthSecretVersion)
}
