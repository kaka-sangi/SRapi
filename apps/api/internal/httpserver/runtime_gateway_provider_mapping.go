package httpserver

import (
	"strings"

	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

func providerConversationRequest(req gatewaycontract.CanonicalRequest, candidate schedulercontract.Candidate) provideradaptercontract.ConversationRequest {
	return provideradaptercontract.ConversationRequest{
		RequestID:         req.RequestID,
		SourceProtocol:    string(req.SourceProtocol),
		SourceEndpoint:    req.SourceEndpoint,
		TargetProtocol:    candidate.Provider.Protocol,
		Model:             req.CanonicalModel,
		Messages:          providerConversationMessages(req),
		InputParts:        providerContentParts(req.InputItems),
		Instructions:      providerInstructions(req),
		Stream:            req.Stream,
		Temperature:       req.Temperature,
		TopP:              req.TopP,
		MaxOutputTokens:   req.MaxOutputTokens,
		Stop:              append([]string(nil), req.Stop...),
		Tools:             cloneMapSlice(req.Tools),
		ToolChoice:        cloneAnyValue(req.ToolChoice),
		ResponseFormat:    cloneAnyMap(req.ResponseFormat),
		Reasoning:         cloneAnyMap(req.Reasoning),
		ContextManagement: cloneAnyMap(req.ContextManagement),
		RawBody:           append([]byte(nil), req.RawBody...),
		Provider:          candidate.Provider,
		Account:           candidate.Account,
		Mapping:           candidate.Mapping,
		SpoofSessionID:    gatewaySpoofSessionID(candidate.Account, req),
	}
}

func providerInstructions(req gatewaycontract.CanonicalRequest) string {
	instructions := strings.TrimSpace(req.Instructions)
	if instructions == "" {
		return ""
	}
	for _, message := range req.Messages {
		if strings.TrimSpace(message.Role) != "system" {
			continue
		}
		if canonicalContentText(message.Content) == instructions {
			return ""
		}
	}
	return instructions
}

func providerTokenCountRequest(req gatewaycontract.CanonicalRequest, rawBody []byte, candidate schedulercontract.Candidate) provideradaptercontract.TokenCountRequest {
	return provideradaptercontract.TokenCountRequest{
		RequestID:      req.RequestID,
		SourceProtocol: string(req.SourceProtocol),
		SourceEndpoint: req.SourceEndpoint,
		Model:          req.CanonicalModel,
		RawBody:        append([]byte(nil), rawBody...),
		Provider:       candidate.Provider,
		Account:        candidate.Account,
		Mapping:        candidate.Mapping,
	}
}

func providerResponseInputItemsRequest(req gatewaycontract.CanonicalRequest, responseID string, query map[string][]string, candidate schedulercontract.Candidate) provideradaptercontract.ResponseInputItemsRequest {
	return provideradaptercontract.ResponseInputItemsRequest{
		RequestID:      req.RequestID,
		SourceProtocol: string(req.SourceProtocol),
		SourceEndpoint: req.SourceEndpoint,
		Model:          req.CanonicalModel,
		ResponseID:     responseID,
		Query:          query,
		Provider:       candidate.Provider,
		Account:        candidate.Account,
		Mapping:        candidate.Mapping,
	}
}

func providerEmbeddingRequest(req gatewaycontract.CanonicalRequest, candidate schedulercontract.Candidate) provideradaptercontract.EmbeddingRequest {
	return provideradaptercontract.EmbeddingRequest{
		RequestID:      req.RequestID,
		SourceProtocol: string(req.SourceProtocol),
		SourceEndpoint: req.SourceEndpoint,
		Model:          req.CanonicalModel,
		Input:          append([]string(nil), req.EmbeddingInput...),
		EncodingFormat: req.EmbeddingEncoding,
		Dimensions:     cloneIntPtr(req.EmbeddingDimensions),
		User:           req.EmbeddingUser,
		Provider:       candidate.Provider,
		Account:        candidate.Account,
		Mapping:        candidate.Mapping,
	}
}

func providerImageGenerationRequest(req gatewaycontract.CanonicalRequest, candidate schedulercontract.Candidate) provideradaptercontract.ImageGenerationRequest {
	return provideradaptercontract.ImageGenerationRequest{
		RequestID:      req.RequestID,
		SourceProtocol: string(req.SourceProtocol),
		SourceEndpoint: req.SourceEndpoint,
		Model:          req.CanonicalModel,
		Prompt:         req.ImagePrompt,
		Stream:         req.ImageStream,
		Count:          req.ImageCount,
		Size:           req.ImageSize,
		Quality:        req.ImageQuality,
		Style:          req.ImageStyle,
		ResponseFormat: req.ImageResponseFormat,
		User:           req.ImageUser,
		Extra:          cloneAnyMap(req.ImageExtra),
		Provider:       candidate.Provider,
		Account:        candidate.Account,
		Mapping:        candidate.Mapping,
	}
}

func providerImageEditRequest(req gatewaycontract.CanonicalRequest, candidate schedulercontract.Candidate) provideradaptercontract.ImageEditRequest {
	return provideradaptercontract.ImageEditRequest{
		RequestID:      req.RequestID,
		SourceProtocol: string(req.SourceProtocol),
		SourceEndpoint: req.SourceEndpoint,
		Model:          req.CanonicalModel,
		Prompt:         req.ImagePrompt,
		Images:         providerImageInputs(req.ImageInputs),
		Mask:           providerImageInputPtr(req.ImageMask),
		Count:          req.ImageCount,
		Size:           req.ImageSize,
		Quality:        req.ImageQuality,
		ResponseFormat: req.ImageResponseFormat,
		User:           req.ImageUser,
		Extra:          cloneAnyMap(req.ImageExtra),
		Provider:       candidate.Provider,
		Account:        candidate.Account,
		Mapping:        candidate.Mapping,
	}
}

func providerImageVariationRequest(req gatewaycontract.CanonicalRequest, candidate schedulercontract.Candidate) provideradaptercontract.ImageVariationRequest {
	return provideradaptercontract.ImageVariationRequest{
		RequestID:      req.RequestID,
		SourceProtocol: string(req.SourceProtocol),
		SourceEndpoint: req.SourceEndpoint,
		Model:          req.CanonicalModel,
		Image:          providerImageInputValue(req.ImageInputs),
		Count:          req.ImageCount,
		Size:           req.ImageSize,
		ResponseFormat: req.ImageResponseFormat,
		User:           req.ImageUser,
		Extra:          cloneAnyMap(req.ImageExtra),
		Provider:       candidate.Provider,
		Account:        candidate.Account,
		Mapping:        candidate.Mapping,
	}
}

func providerAudioTranscriptionRequest(req gatewaycontract.CanonicalRequest, candidate schedulercontract.Candidate) provideradaptercontract.AudioTranscriptionRequest {
	return provideradaptercontract.AudioTranscriptionRequest{
		RequestID:      req.RequestID,
		SourceProtocol: string(req.SourceProtocol),
		SourceEndpoint: req.SourceEndpoint,
		Model:          req.CanonicalModel,
		FileName:       req.AudioFileName,
		ContentType:    req.AudioContentType,
		Audio:          append([]byte(nil), req.AudioBytes...),
		Language:       req.AudioLanguage,
		Prompt:         req.AudioPrompt,
		ResponseFormat: req.AudioResponseFormat,
		Temperature:    cloneFloat32Ptr(req.AudioTemperature),
		User:           req.AudioUser,
		Extra:          cloneAnyMap(req.AudioExtra),
		Provider:       candidate.Provider,
		Account:        candidate.Account,
		Mapping:        candidate.Mapping,
	}
}

func providerAudioSpeechRequest(req gatewaycontract.CanonicalRequest, candidate schedulercontract.Candidate) provideradaptercontract.AudioSpeechRequest {
	return provideradaptercontract.AudioSpeechRequest{
		RequestID:      req.RequestID,
		SourceProtocol: string(req.SourceProtocol),
		SourceEndpoint: req.SourceEndpoint,
		Model:          req.CanonicalModel,
		Input:          req.SpeechInput,
		Voice:          req.SpeechVoice,
		ResponseFormat: req.SpeechResponseFormat,
		Speed:          cloneFloat32Ptr(req.SpeechSpeed),
		Instructions:   req.SpeechInstructions,
		User:           req.SpeechUser,
		Extra:          cloneAnyMap(req.SpeechExtra),
		Provider:       candidate.Provider,
		Account:        candidate.Account,
		Mapping:        candidate.Mapping,
	}
}

func providerModerationRequest(req gatewaycontract.CanonicalRequest, candidate schedulercontract.Candidate) provideradaptercontract.ModerationRequest {
	return provideradaptercontract.ModerationRequest{
		RequestID:      req.RequestID,
		SourceProtocol: string(req.SourceProtocol),
		SourceEndpoint: req.SourceEndpoint,
		Model:          req.CanonicalModel,
		Input:          append([]string(nil), req.ModerationInput...),
		User:           req.ModerationUser,
		Provider:       candidate.Provider,
		Account:        candidate.Account,
		Mapping:        candidate.Mapping,
	}
}

func providerRerankRequest(req gatewaycontract.CanonicalRequest, candidate schedulercontract.Candidate) provideradaptercontract.RerankRequest {
	return provideradaptercontract.RerankRequest{
		RequestID:       req.RequestID,
		SourceProtocol:  string(req.SourceProtocol),
		SourceEndpoint:  req.SourceEndpoint,
		Model:           req.CanonicalModel,
		Query:           req.RerankQuery,
		Documents:       providerRerankDocuments(req.RerankDocuments),
		TopN:            cloneIntPtr(req.RerankTopN),
		ReturnDocuments: req.RerankReturnDocuments,
		User:            req.RerankUser,
		Provider:        candidate.Provider,
		Account:         candidate.Account,
		Mapping:         candidate.Mapping,
	}
}

func providerRerankDocuments(values []gatewaycontract.RerankDocument) []provideradaptercontract.RerankDocument {
	if values == nil {
		return nil
	}
	out := make([]provideradaptercontract.RerankDocument, len(values))
	for idx, value := range values {
		out[idx] = provideradaptercontract.RerankDocument{
			Text:     value.Text,
			Fields:   cloneAnyMap(value.Fields),
			Original: cloneAnyValue(value.Original),
		}
	}
	return out
}

func providerConversationMessages(req gatewaycontract.CanonicalRequest) []provideradaptercontract.ConversationMessage {
	out := make([]provideradaptercontract.ConversationMessage, 0, len(req.Messages)+1)
	for _, message := range req.Messages {
		role := strings.TrimSpace(message.Role)
		if role == "" {
			role = "user"
		}
		parts := providerContentParts(message.Content)
		if len(parts) == 0 {
			continue
		}
		out = append(out, provideradaptercontract.ConversationMessage{Role: role, Parts: parts})
	}
	if len(out) == 0 {
		out = providerConversationMessagesFromInputItems(req.InputItems)
	}
	return out
}

func providerConversationMessagesFromInputItems(blocks []gatewaycontract.ContentBlock) []provideradaptercontract.ConversationMessage {
	out := make([]provideradaptercontract.ConversationMessage, 0, len(blocks))
	var currentRole string
	currentBlocks := make([]gatewaycontract.ContentBlock, 0)
	flush := func() {
		if len(currentBlocks) == 0 {
			return
		}
		parts := providerContentParts(currentBlocks)
		currentBlocks = nil
		if len(parts) == 0 {
			return
		}
		out = append(out, provideradaptercontract.ConversationMessage{Role: currentRole, Parts: parts})
	}
	for _, block := range blocks {
		role := providerInputItemRole(block)
		if role == "" {
			role = "user"
		}
		if currentRole != "" && role != currentRole {
			flush()
		}
		currentRole = role
		currentBlocks = append(currentBlocks, block)
	}
	flush()
	return out
}

func providerInputItemRole(block gatewaycontract.ContentBlock) string {
	role := strings.ToLower(strings.TrimSpace(block.Role))
	switch role {
	case "model":
		role = "assistant"
	case "function":
		role = "tool"
	}
	switch block.Type {
	case gatewaycontract.ContentBlockToolCall:
		if role == "" || role == "user" || role == "tool" {
			return "assistant"
		}
	case gatewaycontract.ContentBlockToolResult:
		return "tool"
	}
	switch role {
	case "system", "developer", "assistant", "tool", "user":
		return role
	default:
		return "user"
	}
}

func providerContentParts(blocks []gatewaycontract.ContentBlock) []provideradaptercontract.ContentPart {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]provideradaptercontract.ContentPart, 0, len(blocks))
	for _, block := range blocks {
		part := provideradaptercontract.ContentPart{
			Kind:              providerContentPartKind(block.Type),
			Text:              strings.TrimSpace(block.Text),
			MediaURL:          strings.TrimSpace(block.MediaURL),
			MediaBase64:       strings.TrimSpace(block.MediaBase64),
			MIMEType:          strings.TrimSpace(block.MIMEType),
			FileID:            strings.TrimSpace(block.FileID),
			ToolCallID:        strings.TrimSpace(block.ToolCallID),
			ToolName:          strings.TrimSpace(block.ToolName),
			ToolArgumentsJSON: strings.TrimSpace(block.ToolArgumentsJSON),
			ToolResultForID:   strings.TrimSpace(block.ToolResultForID),
			ToolResultIsError: block.ToolResultIsError,
			Metadata:          cloneAnyMap(block.Metadata),
			Raw:               append([]byte(nil), block.Raw...),
			OriginProtocol:    strings.TrimSpace(block.OriginProtocol),
		}
		if part.Kind == provideradaptercontract.ContentPartMetadata && len(part.Metadata) == 0 && part.Text == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func providerContentPartKind(value gatewaycontract.ContentBlockType) provideradaptercontract.ContentPartKind {
	switch value {
	case gatewaycontract.ContentBlockImage:
		return provideradaptercontract.ContentPartImage
	case gatewaycontract.ContentBlockAudio:
		return provideradaptercontract.ContentPartAudio
	case gatewaycontract.ContentBlockFile:
		return provideradaptercontract.ContentPartFile
	case gatewaycontract.ContentBlockToolCall:
		return provideradaptercontract.ContentPartToolUse
	case gatewaycontract.ContentBlockToolResult:
		return provideradaptercontract.ContentPartToolResult
	case gatewaycontract.ContentBlockReasoning:
		return provideradaptercontract.ContentPartThinking
	case gatewaycontract.ContentBlockRefusal:
		return provideradaptercontract.ContentPartRefusal
	case gatewaycontract.ContentBlockMetadata:
		return provideradaptercontract.ContentPartMetadata
	default:
		return provideradaptercontract.ContentPartText
	}
}

func canonicalContentText(blocks []gatewaycontract.ContentBlock) string {
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		text := strings.TrimSpace(block.Text)
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}
