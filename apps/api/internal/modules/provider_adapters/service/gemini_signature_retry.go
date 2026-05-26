package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

const geminiSignatureDowngradeWarning = "gemini_signature_sensitive_history_downgraded"

type geminiSignatureRetryStage int

const (
	geminiSignatureRetryThinking geminiSignatureRetryStage = iota + 1
	geminiSignatureRetryTools
)

func geminiSignatureRetryRequest(req contract.ConversationRequest, statusCode int, body []byte, stage geminiSignatureRetryStage) (contract.ConversationRequest, bool) {
	if !geminiSignatureValidationHTTPError(statusCode, body) {
		return contract.ConversationRequest{}, false
	}
	retryReq := cloneConversationRequestForRetry(req)
	retryReq.RawBody = nil
	changed := false
	retryReq.Messages, changed = downgradeGeminiSignatureSensitiveMessages(req.Messages, stage)
	if stage == geminiSignatureRetryTools {
		var inputChanged bool
		retryReq.InputParts, inputChanged = downgradeGeminiSignatureSensitiveParts(req.InputParts, stage)
		changed = changed || inputChanged
	}
	if !changed {
		return contract.ConversationRequest{}, false
	}
	return retryReq, true
}

func geminiSignatureValidationHTTPError(statusCode int, body []byte) bool {
	if statusCode != http.StatusBadRequest {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(string(body)))
	if message == "" {
		return false
	}
	return strings.Contains(message, "thoughtsignature") ||
		strings.Contains(message, "thought_signature") ||
		strings.Contains(message, "thought signature") ||
		strings.Contains(message, "signature")
}

func downgradeGeminiSignatureSensitiveMessages(messages []contract.ConversationMessage, stage geminiSignatureRetryStage) ([]contract.ConversationMessage, bool) {
	if len(messages) == 0 {
		return nil, false
	}
	out := make([]contract.ConversationMessage, 0, len(messages))
	changed := false
	for _, message := range messages {
		parts, partsChanged := downgradeGeminiSignatureSensitiveParts(message.Parts, stage)
		changed = changed || partsChanged
		if len(parts) == 0 {
			changed = true
			continue
		}
		out = append(out, contract.ConversationMessage{
			Role:  message.Role,
			Parts: parts,
		})
	}
	return out, changed
}

func downgradeGeminiSignatureSensitiveParts(parts []contract.ContentPart, stage geminiSignatureRetryStage) ([]contract.ContentPart, bool) {
	if len(parts) == 0 {
		return nil, false
	}
	out := make([]contract.ContentPart, 0, len(parts))
	changed := false
	for _, part := range parts {
		switch part.Kind {
		case contract.ContentPartThinking:
			changed = true
			if geminiThinkingPartIsOpaque(part) {
				continue
			}
			if text := strings.TrimSpace(part.Text); text != "" {
				out = append(out, contract.ContentPart{Kind: contract.ContentPartText, Text: text})
			}
		case contract.ContentPartToolUse, contract.ContentPartToolResult:
			if stage == geminiSignatureRetryTools {
				changed = true
				if text := geminiToolPartDowngradeText(part); text != "" {
					out = append(out, contract.ContentPart{Kind: contract.ContentPartText, Text: text})
				}
				continue
			}
			out = append(out, part)
		default:
			if geminiPartHasSignatureSensitiveMetadata(part) {
				changed = true
				part.Metadata = geminiMetadataWithoutSignature(part.Metadata)
			}
			out = append(out, part)
		}
	}
	return out, changed
}

func geminiThinkingPartIsOpaque(part contract.ContentPart) bool {
	blockType := strings.ToLower(strings.TrimSpace(metadataString(part.Metadata, "type")))
	return blockType == "redacted_thinking"
}

func geminiPartHasSignatureSensitiveMetadata(part contract.ContentPart) bool {
	return metadataString(part.Metadata, "thoughtSignature") != "" ||
		metadataString(part.Metadata, "thought_signature") != "" ||
		metadataString(part.Metadata, "signature") != ""
}

func geminiMetadataWithoutSignature(metadata map[string]any) map[string]any {
	out := cloneMap(metadata)
	delete(out, "thoughtSignature")
	delete(out, "thought_signature")
	delete(out, "signature")
	if len(out) == 0 {
		return nil
	}
	return out
}

func geminiToolPartDowngradeText(part contract.ContentPart) string {
	switch part.Kind {
	case contract.ContentPartToolUse:
		name := strings.TrimSpace(part.ToolName)
		if name == "" {
			name = "tool"
		}
		arguments := strings.TrimSpace(part.ToolArgumentsJSON)
		if arguments == "" {
			arguments = "{}"
		}
		return fmt.Sprintf("[tool_call name=%s id=%s arguments=%s]", name, strings.TrimSpace(part.ToolCallID), arguments)
	case contract.ContentPartToolResult:
		fields := map[string]any{}
		setMapString(fields, "id", firstNonEmpty(part.ToolResultForID, part.ToolCallID))
		setMapString(fields, "name", part.ToolName)
		if part.ToolResultIsError {
			fields["is_error"] = true
		}
		if text := strings.TrimSpace(part.Text); text != "" {
			fields["result"] = text
		}
		raw, err := json.Marshal(fields)
		if err != nil || len(raw) == 0 {
			return ""
		}
		return "[tool_result " + string(raw) + "]"
	default:
		return ""
	}
}

func cloneConversationRequestForRetry(req contract.ConversationRequest) contract.ConversationRequest {
	out := req
	out.Messages = cloneConversationMessages(req.Messages)
	out.InputParts = cloneContentParts(req.InputParts)
	out.System = cloneContentParts(req.System)
	out.Stop = cloneStrings(req.Stop)
	out.Tools = cloneMapSlice(req.Tools)
	out.ResponseFormat = cloneMap(req.ResponseFormat)
	out.Reasoning = cloneMap(req.Reasoning)
	out.RawBody = append([]byte(nil), req.RawBody...)
	out.Credential = cloneMap(req.Credential)
	return out
}

func cloneConversationMessages(messages []contract.ConversationMessage) []contract.ConversationMessage {
	if messages == nil {
		return nil
	}
	out := make([]contract.ConversationMessage, len(messages))
	for idx, message := range messages {
		out[idx] = contract.ConversationMessage{
			Role:  message.Role,
			Parts: cloneContentParts(message.Parts),
		}
	}
	return out
}

func cloneContentParts(parts []contract.ContentPart) []contract.ContentPart {
	if parts == nil {
		return nil
	}
	out := make([]contract.ContentPart, len(parts))
	for idx, part := range parts {
		out[idx] = part
		out[idx].Metadata = cloneMap(part.Metadata)
		out[idx].Raw = append([]byte(nil), part.Raw...)
	}
	return out
}

func appendGeminiSignatureDowngradeWarning(resp contract.ConversationResponse) contract.ConversationResponse {
	for _, warning := range resp.Warnings {
		if warning == geminiSignatureDowngradeWarning {
			return resp
		}
	}
	resp.Warnings = append(resp.Warnings, geminiSignatureDowngradeWarning)
	return resp
}

func contentPartOriginIs(part contract.ContentPart, origin string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(part.OriginProtocol)), strings.ToLower(strings.TrimSpace(origin)))
}
