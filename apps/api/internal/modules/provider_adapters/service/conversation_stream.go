package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	reverseproxycontract "github.com/srapi/srapi/apps/api/internal/modules/reverse_proxy/contract"
)

// StreamConversation forwards an eligible same-protocol conversation request
// upstream and returns the LIVE response body for incremental streaming, rather
// than buffering the whole response with io.ReadAll as InvokeConversation does.
// It supports only same-protocol reverse-proxy passthrough variants — the
// common subscription/relay resale case (Claude Code, Codex, and reverse-proxy
// OpenAI/Anthropic/Gemini). For anything else (cross-protocol translation,
// direct-egress api_key runtimes, bedrock, generic, antigravity, ChatGPT-web,
// responses-compact, or a reverse proxy without streaming support) it returns
// contract.ErrStreamingUnsupported and the caller falls back to the buffered
// InvokeConversation path.
//
// The returned ConversationResponse carries StreamBody (which the caller MUST
// Close) and StreamParse, which recovers usage from the fully-streamed bytes
// using the exact same parser as the buffered path — so metering is identical;
// only the time-to-first-byte changes.
func (s *Service) StreamConversation(ctx context.Context, req contract.ConversationRequest) (contract.ConversationResponse, error) {
	if !req.Stream {
		return contract.ConversationResponse{}, contract.ErrStreamingUnsupported
	}
	if strings.TrimSpace(req.RequestID) == "" || strings.TrimSpace(req.Model) == "" || strings.TrimSpace(req.Mapping.UpstreamModelName) == "" {
		return contract.ConversationResponse{}, ErrInvalidInput
	}
	streamer, ok := s.reverseProxy.(reverseproxycontract.StreamRuntime)
	if !ok {
		return contract.ConversationResponse{}, contract.ErrStreamingUnsupported
	}
	baseURL := upstreamBaseURL(req)
	if baseURL == "" {
		return contract.ConversationResponse{}, contract.ErrStreamingUnsupported
	}
	// Same-protocol streaming is byte passthrough (or the codex responses->chat
	// transform). Cross-protocol streaming gets the live upstream body and signals
	// the HTTP layer to transcode it into the client's protocol on the fly, when
	// an incremental parser (for the upstream) and renderer (for the client) both
	// exist for the pair — otherwise it falls back to the buffered path.
	if sameConversationProtocol(req) {
		return s.streamByTarget(ctx, streamer, req, baseURL)
	}
	upstreamProto, ok := s.crossProtocolUpstreamProtocol(req)
	if !ok || !clientHasIncrementalRenderer(req) {
		return contract.ConversationResponse{}, contract.ErrStreamingUnsupported
	}
	resp, err := s.streamByTarget(ctx, streamer, req, baseURL)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	if resp.StreamBody != nil {
		resp.TranscodeUpstreamProtocol = upstreamProto
	}
	return resp, nil
}

// streamByTarget routes a streamable request to the live upstream body exactly
// as InvokeConversation's dispatch would: reverse-proxy runtimes (OAuth /
// session / cookie subscription resale) go through DoStream; plain api_key
// runtimes stream via the account's egress client directly.
func (s *Service) streamByTarget(ctx context.Context, streamer reverseproxycontract.StreamRuntime, req contract.ConversationRequest, baseURL string) (contract.ConversationResponse, error) {
	switch {
	case isBedrockCompatible(req), isGenericReverseProxy(req), isAntigravityReverseProxy(req):
		return contract.ConversationResponse{}, contract.ErrStreamingUnsupported
	case isCodexReverseProxy(req):
		return s.streamReverseProxyCodexResponses(ctx, streamer, req, baseURL)
	case isGeminiCompatible(req):
		if isReverseProxyRuntime(req) {
			return s.streamReverseProxyGeminiCompatible(ctx, streamer, req, baseURL)
		}
		return s.streamDirectGeminiCompatible(ctx, req, baseURL)
	case isAnthropicCompatible(req):
		if isReverseProxyRuntime(req) {
			return s.streamReverseProxyAnthropicCompatible(ctx, streamer, req, baseURL)
		}
		return s.streamDirectAnthropicCompatible(ctx, req, baseURL)
	case isReverseProxyRuntime(req):
		return s.streamReverseProxyOpenAICompatible(ctx, streamer, req, baseURL)
	default:
		return s.streamDirectOpenAICompatible(ctx, req, baseURL)
	}
}

// crossProtocolUpstreamProtocol returns the canonical upstream protocol string
// for a cross-protocol streamable request (used as the transcode signal), or
// (_, false) when the upstream has no incremental per-frame parser yet (codex's
// bespoke responses wire, gemini, bedrock/generic/antigravity/chatgpt-web) — in
// which case the request falls back to the buffered path.
func (s *Service) crossProtocolUpstreamProtocol(req contract.ConversationRequest) (string, bool) {
	if isBedrockCompatible(req) || isGenericReverseProxy(req) || isAntigravityReverseProxy(req) || isChatGPTWebReverseProxy(req) {
		return "", false
	}
	switch {
	case isCodexReverseProxy(req):
		// Codex's protocol is openai-compatible but its wire is the Responses API.
		return "responses", true
	case isAnthropicCompatible(req):
		return "anthropic-compatible", true
	case isGeminiCompatible(req):
		return "gemini-compatible", true
	default:
		return "openai-compatible", true
	}
}

// clientHasIncrementalRenderer reports whether the inbound (client) protocol has
// an incremental stream renderer. Chat Completions and Anthropic Messages do;
// the /responses and Gemini client renderers are added with later components.
func clientHasIncrementalRenderer(req contract.ConversationRequest) bool {
	switch strings.ToLower(strings.TrimSpace(req.SourceProtocol)) {
	case "anthropic-compatible", "anthropic":
		return true
	case "gemini-compatible", "gemini":
		return true
	case "openai-compatible", "openai":
		// chat (ChatStreamRenderer) and /responses (ResponsesStreamRenderer) both
		// have incremental renderers now.
		return true
	default:
		return false
	}
}

// egressStreamConversation issues req via the account's egress client (proxy +
// TLS fingerprint + SSRF guard when configured) and returns the LIVE response
// body for streaming on a 2xx status. On a non-2xx status it reads (bounded),
// closes the body, and returns a classified ProviderError so the gateway can
// fail over before writing anything downstream.
func (s *Service) egressStreamConversation(
	ctx context.Context,
	req contract.ConversationRequest,
	httpReq *http.Request,
	classify func(status int, header http.Header, body []byte) error,
	parse func([]byte, int) (contract.ConversationResponse, error),
) (contract.ConversationResponse, error) {
	resp, err := s.egressHTTPClient(req.Account, req.Credential).Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return contract.ConversationResponse{}, contract.ProviderError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "provider request timed out"}
		}
		return contract.ConversationResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "provider request failed"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		_ = resp.Body.Close()
		return contract.ConversationResponse{}, classify(resp.StatusCode, resp.Header, body)
	}
	return contract.ConversationResponse{
		StatusCode:  resp.StatusCode,
		Headers:     resp.Header,
		StreamBody:  resp.Body,
		StreamParse: parse,
	}, nil
}

func (s *Service) streamDirectOpenAICompatible(ctx context.Context, req contract.ConversationRequest, baseURL string) (contract.ConversationResponse, error) {
	apiKey := credentialString(req.Credential, "api_key")
	if apiKey == "" {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "provider api key missing"}
	}
	// Only plain chat/completions is same-protocol SSE passthrough.
	if openAIResponsesEndpoint(req) {
		return contract.ConversationResponse{}, contract.ErrStreamingUnsupported
	}
	raw, err := openAICompatibleRequestBody(req)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	return s.egressStreamConversation(ctx, req, httpReq,
		func(status int, header http.Header, body []byte) error {
			return classifyProviderHTTPErrorWithHeaders(status, header, body)
		},
		parseOpenAICompatibleStream)
}

func (s *Service) streamDirectAnthropicCompatible(ctx context.Context, req contract.ConversationRequest, baseURL string) (contract.ConversationResponse, error) {
	apiKey := anthropicAPIKey(req)
	if apiKey == "" {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "provider api key missing"}
	}
	raw, err := anthropicCompatibleRequestBody(req)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/messages"
	headers := anthropicCompatibleHeaders(req, apiKey, &endpoint)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	httpReq.Header = headers
	streamResp, err := s.egressStreamConversation(ctx, req, httpReq,
		func(status int, header http.Header, body []byte) error {
			return classifyProviderHTTPErrorWithHeaders(status, header, body)
		},
		parseAnthropicCompatibleStream)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	streamResp.StreamParse = anthropicStreamParserWithHeaders(streamResp.Headers)
	return streamResp, nil
}

func (s *Service) streamDirectGeminiCompatible(ctx context.Context, req contract.ConversationRequest, baseURL string) (contract.ConversationResponse, error) {
	apiKey := geminiAPIKey(req)
	if apiKey == "" {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "provider api key missing"}
	}
	raw, err := geminiCompatibleRequestBody(req)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	endpoint := geminiEndpoint(baseURL, req.Mapping.UpstreamModelName, req.Stream)
	headers := geminiCompatibleHeaders(req, apiKey, &endpoint)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	httpReq.Header = headers
	return s.egressStreamConversation(ctx, req, httpReq,
		func(status int, header http.Header, body []byte) error {
			return classifyGeminiProviderHTTPErrorWithHeaders(status, header, body)
		},
		parseGeminiCompatibleStream)
}

func sameConversationProtocol(req contract.ConversationRequest) bool {
	source := strings.ToLower(strings.TrimSpace(req.SourceProtocol))
	target := strings.ToLower(strings.TrimSpace(req.TargetProtocol))
	return source != "" && source == target
}

// openAIResponsesEndpoint reports whether the request targets the OpenAI
// Responses API family (/responses, /responses/compact, or the /responses/ws
// WebSocket bridge). These are never plain chat/completions SSE passthrough: a
// non-native provider re-renders them from chat completions (the
// responses-over-WebSocket bridge depends on that buffered re-render), and
// native providers need a Responses-shaped upstream body. So the
// chat/completions stream builders must never handle them — they fall back to
// the buffered path (codex /responses streams via its own dedicated branch).
// chat/completions endpoints never contain "/responses", so Contains is safe.
func openAIResponsesEndpoint(req contract.ConversationRequest) bool {
	endpoint := strings.ToLower(strings.TrimSpace(req.SourceEndpoint))
	return strings.Contains(endpoint, "/responses")
}

func reverseProxyAccountRuntime(req contract.ConversationRequest) reverseproxycontract.AccountRuntime {
	return reverseproxycontract.AccountRuntime{
		AccountID:      req.Account.ID,
		RuntimeClass:   string(req.Account.RuntimeClass),
		UpstreamClient: req.Account.UpstreamClient,
		ProxyID:        req.Account.ProxyID,
		UserAgent:      mapString(req.Account.Metadata, "user_agent"),
		Metadata:       req.Account.Metadata,
		Credential:     req.Credential,
	}
}

func streamConversationResponse(resp reverseproxycontract.StreamResponse, parse func([]byte, int) (contract.ConversationResponse, error)) contract.ConversationResponse {
	return contract.ConversationResponse{
		StatusCode:  resp.StatusCode,
		Headers:     resp.Headers,
		StreamBody:  resp.Body,
		StreamParse: parse,
	}
}

func (s *Service) streamReverseProxyOpenAICompatible(ctx context.Context, streamer reverseproxycontract.StreamRuntime, req contract.ConversationRequest, baseURL string) (contract.ConversationResponse, error) {
	// Only plain chat/completions is same-protocol SSE passthrough; the
	// responses / responses-compact / ChatGPT-web shapes use bespoke buffered
	// paths and are not streamed here.
	if openAIResponsesEndpoint(req) || isChatGPTWebReverseProxy(req) {
		return contract.ConversationResponse{}, contract.ErrStreamingUnsupported
	}
	raw, err := openAICompatibleRequestBody(req)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	resp, err := streamer.DoStream(ctx, reverseproxycontract.Request{
		Account: reverseProxyAccountRuntime(req),
		Method:  http.MethodPost,
		URL:     strings.TrimRight(baseURL, "/") + "/chat/completions",
		Headers: http.Header{
			"Content-Type": {"application/json"},
		},
		Body:         raw,
		ExpectStream: true,
	})
	if err != nil {
		return contract.ConversationResponse{}, providerErrorFromReverseProxy(err)
	}
	return streamConversationResponse(resp, parseOpenAICompatibleStream), nil
}

func (s *Service) streamReverseProxyAnthropicCompatible(ctx context.Context, streamer reverseproxycontract.StreamRuntime, req contract.ConversationRequest, baseURL string) (contract.ConversationResponse, error) {
	if isClaudeCodeReverseProxy(req) && claudeCodeReverseProxyRuntimeIsAPIKey(req) {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "Claude Code reverse proxy requires OAuth or client-token runtime"}
	}
	raw, err := anthropicCompatibleRequestBody(req)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/messages"
	headers := anthropicCompatibleHeaders(req, "", &endpoint)
	if isClaudeCodeReverseProxy(req) {
		raw, err = claudeCodeMessagesPayload(req, raw)
		if err != nil {
			return contract.ConversationResponse{}, err
		}
		endpoint = claudeCodeMessagesEndpoint(baseURL)
		headers = claudeCodeMessagesHeaders(req)
	}
	resp, err := streamer.DoStream(ctx, reverseproxycontract.Request{
		Account:      reverseProxyAccountRuntime(req),
		Method:       http.MethodPost,
		URL:          endpoint,
		Headers:      headers,
		Body:         raw,
		ExpectStream: true,
	})
	if err != nil {
		return contract.ConversationResponse{}, providerErrorFromReverseProxy(err)
	}
	return anthropicStreamConversationResponse(resp), nil
}

func (s *Service) streamReverseProxyGeminiCompatible(ctx context.Context, streamer reverseproxycontract.StreamRuntime, req contract.ConversationRequest, baseURL string) (contract.ConversationResponse, error) {
	raw, err := geminiCompatibleRequestBody(req)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	endpoint := geminiEndpoint(baseURL, req.Mapping.UpstreamModelName, req.Stream)
	headers := geminiCompatibleHeaders(req, "", &endpoint)
	resp, err := streamer.DoStream(ctx, reverseproxycontract.Request{
		Account:      reverseProxyAccountRuntime(req),
		Method:       http.MethodPost,
		URL:          endpoint,
		Headers:      headers,
		Body:         raw,
		ExpectStream: true,
	})
	if err != nil {
		return contract.ConversationResponse{}, providerErrorFromReverseProxy(err)
	}
	return streamConversationResponse(resp, parseGeminiCompatibleStream), nil
}

func (s *Service) streamReverseProxyCodexResponses(ctx context.Context, streamer reverseproxycontract.StreamRuntime, req contract.ConversationRequest, baseURL string) (contract.ConversationResponse, error) {
	if codexReverseProxyRuntimeIsAPIKey(req) {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "codex reverse proxy requires OAuth/session/client-token runtime credentials"}
	}
	payload, stream, err := codexResponsesPayload(req)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	if !stream {
		return contract.ConversationResponse{}, contract.ErrStreamingUnsupported
	}
	replayScope, err := s.codexPrepareResponsesPayload(req, payload)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	var releaseImageGenSlot func()
	if codexPayloadHasImageGenerationTool(payload) {
		release, slotErr := codexImageGenSlotAcquire(ctx, req.Account)
		if slotErr != nil {
			return contract.ConversationResponse{}, contract.ProviderError{Class: "rate_limited", StatusCode: http.StatusTooManyRequests, Message: "codex image-generation slot acquire cancelled"}
		}
		releaseImageGenSlot = release
	}
	releaseStreamResources := true
	defer func() {
		if releaseStreamResources && releaseImageGenSlot != nil {
			releaseImageGenSlot()
		}
	}()
	headers := codexResponsesHeaders(req, stream, payload)
	raw, outboundState := codexApplyOutboundWiring(req.Account, headers, raw)
	resp, err := streamer.DoStream(ctx, reverseproxycontract.Request{
		Account:      codexReverseProxyAccount(req),
		Method:       http.MethodPost,
		URL:          codexResponsesEndpoint(baseURL, req),
		Headers:      headers,
		Body:         raw,
		ExpectStream: stream,
	})
	if err != nil {
		return contract.ConversationResponse{}, providerErrorFromReverseProxy(err)
	}
	releaseStreamResources = false
	streamBody := codexExposeStreamBody(resp.Body, outboundState, releaseImageGenSlot)
	parseStreamBody := func(body []byte, statusCode int) (contract.ConversationResponse, error) {
		clientBody := codexCaptureInboundWiring(outboundState, replayScope, body)
		parsed, parseErr := parseCodexResponsesBody(clientBody, statusCode)
		if parseErr != nil {
			codexClearReasoningReplayOnInvalidSignature(replayScope, statusCode, body)
			return contract.ConversationResponse{}, parseErr
		}
		parsed = withConversationResponseHeaders(parsed, resp.Headers)
		return withCodexQuotaSignals(parsed, resp.Headers), nil
	}
	// A chat/completions client cannot parse Codex /responses SSE, so transform it
	// into chat.completion.chunk SSE on the fly (true incremental streaming). A
	// Responses-API client gets the same-shape stream piped verbatim. Any OTHER
	// client (anthropic/gemini) gets the raw Responses stream + a transcode signal
	// so the cross-protocol reader re-renders it into that client's protocol. Usage
	// is always recovered from the retained raw bytes via parseCodexResponsesBody.
	if isChatCompletionsClient(req) {
		reader := newCodexChatStreamReader(streamBody, req)
		return contract.ConversationResponse{
			StatusCode: resp.StatusCode,
			Headers:    resp.Headers,
			StreamBody: reader,
			StreamParse: func(_ []byte, statusCode int) (contract.ConversationResponse, error) {
				return parseStreamBody(reader.rawBytes(), statusCode)
			},
		}, nil
	}
	return contract.ConversationResponse{
		StatusCode:  resp.StatusCode,
		Headers:     resp.Headers,
		StreamBody:  streamBody,
		StreamParse: parseStreamBody,
	}, nil
}

type codexExposeStreamBodyReader struct {
	upstream io.ReadCloser
	scanner  *bufio.Scanner
	state    CodexIdentityConfuseState
	release  func()

	frame  bytes.Buffer
	out    bytes.Buffer
	done   bool
	closed bool
}

func codexExposeStreamBody(upstream io.ReadCloser, state CodexIdentityConfuseState, release func()) io.ReadCloser {
	if upstream == nil {
		if release != nil {
			release()
		}
		return io.NopCloser(bytes.NewReader(nil))
	}
	if !state.Enabled && release == nil {
		return upstream
	}
	if !state.Enabled {
		return &codexReleaseStreamBody{upstream: upstream, release: release}
	}
	scanner := bufio.NewScanner(upstream)
	scanner.Buffer(make([]byte, 0, 64*1024), 52_428_800)
	return &codexExposeStreamBodyReader{
		upstream: upstream,
		scanner:  scanner,
		state:    state,
		release:  release,
	}
}

type codexReleaseStreamBody struct {
	upstream io.ReadCloser
	release  func()
	closed   bool
}

func (r *codexReleaseStreamBody) Read(p []byte) (int, error) {
	if r.upstream == nil {
		return 0, io.EOF
	}
	return r.upstream.Read(p)
}

func (r *codexReleaseStreamBody) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	var err error
	if r.upstream != nil {
		err = r.upstream.Close()
		r.upstream = nil
	}
	if r.release != nil {
		r.release()
		r.release = nil
	}
	return err
}

func (r *codexExposeStreamBodyReader) Read(p []byte) (int, error) {
	for r.out.Len() == 0 && !r.done {
		r.pump()
	}
	if r.out.Len() > 0 {
		return r.out.Read(p)
	}
	return 0, io.EOF
}

func (r *codexExposeStreamBodyReader) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	var err error
	if r.upstream != nil {
		err = r.upstream.Close()
		r.upstream = nil
	}
	if r.release != nil {
		r.release()
		r.release = nil
	}
	return err
}

func (r *codexExposeStreamBodyReader) pump() {
	if r.done {
		return
	}
	for r.scanner.Scan() {
		line := strings.TrimRight(r.scanner.Text(), "\r")
		if line == "" {
			if r.frame.Len() > 0 {
				exposed := ApplyCodexIdentityExposeResponsePayload(append([]byte(nil), r.frame.Bytes()...), r.state)
				r.out.Write(exposed)
				r.out.WriteByte('\n')
				r.frame.Reset()
			} else {
				r.out.WriteByte('\n')
			}
			return
		}
		r.frame.WriteString(line)
		r.frame.WriteByte('\n')
	}
	if r.frame.Len() > 0 {
		exposed := ApplyCodexIdentityExposeResponsePayload(append([]byte(nil), r.frame.Bytes()...), r.state)
		r.out.Write(exposed)
		r.out.WriteByte('\n')
		r.frame.Reset()
	}
	r.done = true
}

// isChatCompletionsClient reports whether the inbound request is an OpenAI
// Chat Completions client (openai-compatible source on a non-/responses
// endpoint) — the only client the codex reader transforms directly to chat.
func isChatCompletionsClient(req contract.ConversationRequest) bool {
	return strings.EqualFold(strings.TrimSpace(req.SourceProtocol), "openai-compatible") && !openAIResponsesEndpoint(req)
}
