package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

const (
	bedrockDefaultRegion           = "us-east-1"
	bedrockServiceName             = "bedrock"
	bedrockAnthropicVersion        = "bedrock-2023-05-31"
	bedrockDefaultRuntimeBaseURL   = "https://bedrock-runtime.%s.amazonaws.com"
	bedrockEventStreamContentType  = "application/vnd.amazon.eventstream"
	bedrockInvokeResponseReadLimit = 4 << 20
)

var bedrockCrossRegionPrefixes = []string{"us.", "eu.", "apac.", "jp.", "au.", "us-gov.", "global."}

func (s *Service) invokeBedrockAnthropic(ctx context.Context, req contract.ConversationRequest) (contract.ConversationResponse, error) {
	awsCredential, region, err := bedrockAWSCredential(req)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	modelID, err := bedrockModelID(req, region)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	raw, err := anthropicCompatibleRequestBody(req)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	raw, err = bedrockAnthropicRequestBody(raw, req, modelID)
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	endpoint := bedrockInvokeURL(req, region, modelID, req.Stream)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return contract.ConversationResponse{}, err
	}
	httpReq.Header.Set("Accept", bedrockAcceptHeader(req.Stream))
	httpReq.Header.Set("Content-Type", "application/json")
	if err := signBedrockRequest(ctx, httpReq, raw, awsCredential, region); err != nil {
		return contract.ConversationResponse{}, err
	}

	resp, err := s.client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return contract.ConversationResponse{}, contract.ProviderError{Class: "timeout", StatusCode: http.StatusGatewayTimeout, Message: "provider request timed out"}
		}
		return contract.ConversationResponse{}, contract.ProviderError{Class: "network_error", StatusCode: http.StatusBadGateway, Message: "provider request failed"}
	}
	defer resp.Body.Close()

	if req.Stream {
		body, err := bedrockAnthropicStreamToSSE(resp.Body)
		if err != nil {
			return contract.ConversationResponse{}, contract.ProviderError{Class: "stream_interrupted", StatusCode: http.StatusBadGateway, Message: "provider stream interrupted"}
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return contract.ConversationResponse{}, classifyAnthropicProviderHTTPError(resp.StatusCode, body)
		}
		return parseAnthropicCompatibleStream(body, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, bedrockInvokeResponseReadLimit))
	if err != nil {
		return contract.ConversationResponse{}, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider response read failed"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return contract.ConversationResponse{}, classifyAnthropicProviderHTTPError(resp.StatusCode, body)
	}
	return parseAnthropicCompatibleJSON(body, resp.StatusCode)
}

func bedrockAWSCredential(req contract.ConversationRequest) (aws.Credentials, string, error) {
	accessKeyID := firstNonEmpty(
		credentialString(req.Credential, "aws_access_key_id"),
		credentialString(req.Credential, "access_key_id"),
	)
	secretAccessKey := firstNonEmpty(
		credentialString(req.Credential, "aws_secret_access_key"),
		credentialString(req.Credential, "secret_access_key"),
	)
	if accessKeyID == "" || secretAccessKey == "" {
		return aws.Credentials{}, "", contract.ProviderError{Class: "auth_failed", StatusCode: http.StatusUnauthorized, Message: "AWS credentials missing"}
	}
	region := bedrockRegion(req)
	return aws.Credentials{
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		SessionToken: firstNonEmpty(
			credentialString(req.Credential, "aws_session_token"),
			credentialString(req.Credential, "session_token"),
		),
		Source: "srapi-provider-account",
	}, region, nil
}

func bedrockRegion(req contract.ConversationRequest) string {
	for _, values := range []map[string]any{req.Credential, req.Account.Metadata, req.Provider.ConfigSchema, req.Provider.Capabilities} {
		for _, key := range []string{"aws_region", "bedrock_region", "region"} {
			if value := mapString(values, key); value != "" {
				return value
			}
		}
	}
	return bedrockDefaultRegion
}

func bedrockModelID(req contract.ConversationRequest, region string) (string, error) {
	modelID := strings.TrimSpace(req.Mapping.UpstreamModelName)
	if modelID == "" {
		return "", contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "Bedrock upstream model id missing"}
	}
	if shouldAdjustBedrockRegion(req, modelID) {
		targetRegion := region
		if mapBool(req.Credential, "aws_force_global") || mapBool(req.Account.Metadata, "aws_force_global") || mapBool(req.Account.Metadata, "bedrock_force_global") {
			targetRegion = "global"
		}
		modelID = adjustBedrockModelRegionPrefix(modelID, targetRegion)
	}
	return modelID, nil
}

func shouldAdjustBedrockRegion(req contract.ConversationRequest, modelID string) bool {
	if mapBool(req.Account.Metadata, "bedrock_disable_region_prefix_adjustment") || mapBool(req.Provider.ConfigSchema, "bedrock_disable_region_prefix_adjustment") {
		return false
	}
	return isRegionalBedrockModelID(modelID)
}

func bedrockCrossRegionPrefix(region string) string {
	region = strings.ToLower(strings.TrimSpace(region))
	switch {
	case strings.HasPrefix(region, "us-gov"):
		return "us-gov"
	case strings.HasPrefix(region, "us-"), strings.HasPrefix(region, "ca-"), strings.HasPrefix(region, "sa-"):
		return "us"
	case strings.HasPrefix(region, "eu-"):
		return "eu"
	case region == "ap-northeast-1":
		return "jp"
	case region == "ap-southeast-2":
		return "au"
	case strings.HasPrefix(region, "ap-"):
		return "apac"
	default:
		return "us"
	}
}

func adjustBedrockModelRegionPrefix(modelID string, region string) string {
	targetPrefix := bedrockCrossRegionPrefix(region)
	if strings.EqualFold(strings.TrimSpace(region), "global") {
		targetPrefix = "global"
	}
	for _, prefix := range bedrockCrossRegionPrefixes {
		if strings.HasPrefix(modelID, prefix) {
			if prefix == targetPrefix+"." {
				return modelID
			}
			return targetPrefix + "." + strings.TrimPrefix(modelID, prefix)
		}
	}
	return modelID
}

func isRegionalBedrockModelID(modelID string) bool {
	for _, prefix := range bedrockCrossRegionPrefixes {
		if strings.HasPrefix(modelID, prefix) {
			return true
		}
	}
	return false
}

func bedrockInvokeURL(req contract.ConversationRequest, region string, modelID string, stream bool) string {
	baseURL := bedrockBaseURL(req, region)
	escapedModelID := strings.ReplaceAll(url.PathEscape(modelID), ":", "%3A")
	action := "invoke"
	if stream {
		action = "invoke-with-response-stream"
	}
	return strings.TrimRight(baseURL, "/") + "/model/" + escapedModelID + "/" + action
}

func bedrockBaseURL(req contract.ConversationRequest, region string) string {
	baseURL := upstreamBaseURL(req)
	if baseURL == "" {
		return fmt.Sprintf(bedrockDefaultRuntimeBaseURL, region)
	}
	return strings.TrimRight(baseURL, "/")
}

func bedrockAcceptHeader(stream bool) string {
	if stream {
		return bedrockEventStreamContentType
	}
	return "application/json"
}

func signBedrockRequest(ctx context.Context, req *http.Request, body []byte, credential aws.Credentials, region string) error {
	payloadHash := sha256.Sum256(body)
	hexPayloadHash := hex.EncodeToString(payloadHash[:])
	return v4.NewSigner().SignHTTP(ctx, credential, req, hexPayloadHash, bedrockServiceName, region, time.Now())
}

func bedrockAnthropicRequestBody(body []byte, req contract.ConversationRequest, modelID string) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, contract.ProviderError{Class: "invalid_request", StatusCode: http.StatusBadRequest, Message: "invalid Bedrock Anthropic request"}
	}
	payload["anthropic_version"] = bedrockAnthropicVersion
	if betaTokens := bedrockAnthropicBetaTokens(req); len(betaTokens) > 0 {
		payload["anthropic_beta"] = betaTokens
	}
	delete(payload, "model")
	delete(payload, "stream")
	delete(payload, "output_config")
	if tools, ok := payload["tools"].([]any); ok {
		for _, item := range tools {
			if tool, ok := item.(map[string]any); ok {
				delete(tool, "custom")
			}
		}
	}
	sanitizeBedrockCacheControl(payload, modelID)
	return json.Marshal(payload)
}

func bedrockAnthropicBetaTokens(req contract.ConversationRequest) []string {
	raw := requestSetting(req, "anthropic_beta", "anthropic-beta", "bedrock_beta")
	if raw == "" {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, item := range strings.Split(raw, ",") {
		token := strings.TrimSpace(item)
		if token == "" {
			continue
		}
		if _, exists := seen[token]; exists {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
	}
	return out
}

func sanitizeBedrockCacheControl(payload map[string]any, modelID string) {
	allowTTL := bedrockClaude45OrNewer(modelID)
	sanitizeBlocks := func(values any) {
		items, ok := values.([]any)
		if !ok {
			return
		}
		for _, item := range items {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if cacheControl, ok := block["cache_control"].(map[string]any); ok {
				delete(cacheControl, "scope")
				if !allowTTL || !bedrockCacheTTLAllowed(cacheControl["ttl"]) {
					delete(cacheControl, "ttl")
				}
			}
		}
	}
	sanitizeBlocks(payload["system"])
	messages, ok := payload["messages"].([]any)
	if !ok {
		return
	}
	for _, item := range messages {
		message, ok := item.(map[string]any)
		if !ok {
			continue
		}
		sanitizeBlocks(message["content"])
	}
}

func bedrockCacheTTLAllowed(value any) bool {
	ttl := strings.TrimSpace(fmt.Sprint(value))
	return ttl == "5m" || ttl == "1h"
}

func bedrockClaude45OrNewer(modelID string) bool {
	lower := strings.ToLower(modelID)
	for _, family := range []string{"claude-haiku-", "claude-sonnet-", "claude-opus-"} {
		idx := strings.Index(lower, family)
		if idx < 0 {
			continue
		}
		version := lower[idx+len(family):]
		fields := strings.FieldsFunc(version, func(r rune) bool { return r == '-' || r == '.' })
		if len(fields) < 2 {
			continue
		}
		major := atoiOrZero(fields[0])
		minor := atoiOrZero(fields[1])
		return major > 4 || (major == 4 && minor >= 5)
	}
	return false
}

func atoiOrZero(value string) int {
	out := 0
	for _, r := range strings.TrimSpace(value) {
		if r < '0' || r > '9' {
			break
		}
		out = out*10 + int(r-'0')
	}
	return out
}

func bedrockAnthropicStreamToSSE(reader io.Reader) ([]byte, error) {
	decoder := eventstream.NewDecoder()
	payloadBuf := make([]byte, 0, 64*1024)
	var out bytes.Buffer
	for {
		msg, err := decoder.Decode(reader, payloadBuf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		eventType := eventstreamHeaderString(msg.Headers, ":event-type")
		messageType := eventstreamHeaderString(msg.Headers, ":message-type")
		exceptionType := eventstreamHeaderString(msg.Headers, ":exception-type")
		if exceptionType != "" {
			return nil, fmt.Errorf("bedrock exception %s: %s", exceptionType, strings.TrimSpace(string(msg.Payload)))
		}
		if messageType == "exception" || messageType == "error" {
			return nil, fmt.Errorf("bedrock stream error: %s", strings.TrimSpace(string(msg.Payload)))
		}
		if eventType != "" && eventType != "chunk" {
			continue
		}
		chunk, ok, err := bedrockStreamChunkPayload(msg.Payload)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		chunk = normalizeBedrockStreamMetrics(chunk)
		event := strings.TrimSpace(jsonStringField(chunk, "type"))
		if event != "" {
			out.WriteString("event: ")
			out.WriteString(event)
			out.WriteString("\n")
		}
		out.WriteString("data: ")
		out.Write(bytes.TrimSpace(chunk))
		out.WriteString("\n\n")
	}
	return out.Bytes(), nil
}

func eventstreamHeaderString(headers eventstream.Headers, name string) string {
	value := headers.Get(name)
	if value == nil {
		return ""
	}
	return strings.TrimSpace(value.String())
}

func bedrockStreamChunkPayload(payload []byte) ([]byte, bool, error) {
	var frame struct {
		Bytes string `json:"bytes"`
	}
	if err := json.Unmarshal(payload, &frame); err != nil {
		return nil, false, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid Bedrock eventstream json"}
	}
	if strings.TrimSpace(frame.Bytes) == "" {
		return nil, false, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(frame.Bytes)
	if err != nil {
		return nil, false, contract.ProviderError{Class: "invalid_response", StatusCode: http.StatusBadGateway, Message: "provider returned invalid Bedrock eventstream chunk"}
	}
	return decoded, true, nil
}

func normalizeBedrockStreamMetrics(payload []byte) []byte {
	var value map[string]any
	if err := json.Unmarshal(payload, &value); err != nil {
		return payload
	}
	metrics, ok := value["amazon-bedrock-invocationMetrics"].(map[string]any)
	if !ok {
		return payload
	}
	delete(value, "amazon-bedrock-invocationMetrics")
	if _, exists := value["usage"]; !exists {
		usage := map[string]any{}
		if input, ok := metrics["inputTokenCount"]; ok {
			usage["input_tokens"] = input
		}
		if output, ok := metrics["outputTokenCount"]; ok {
			usage["output_tokens"] = output
		}
		if len(usage) > 0 {
			value["usage"] = usage
		}
	}
	out, err := json.Marshal(value)
	if err != nil {
		return payload
	}
	return out
}

func jsonStringField(payload []byte, field string) string {
	var value map[string]any
	if err := json.Unmarshal(payload, &value); err != nil {
		return ""
	}
	return mapString(value, field)
}
