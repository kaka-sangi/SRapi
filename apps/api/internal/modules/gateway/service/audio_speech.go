package service

import (
	"fmt"
	"strings"

	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func (s *Service) NormalizeAudioSpeech(req apiopenapi.AudioSpeechRequest, meta RequestMeta) (gatewaycontract.CanonicalRequest, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		return gatewaycontract.CanonicalRequest{}, fmt.Errorf("audio speech model is empty")
	}
	input := strings.TrimSpace(req.Input)
	if input == "" {
		return gatewaycontract.CanonicalRequest{}, fmt.Errorf("audio speech input is empty")
	}
	voice := strings.TrimSpace(req.Voice)
	if voice == "" {
		return gatewaycontract.CanonicalRequest{}, fmt.Errorf("audio speech voice is empty")
	}
	format := enumString(req.ResponseFormat)
	if format == "" {
		format = "mp3"
	}
	if !validAudioSpeechResponseFormat(format) {
		return gatewaycontract.CanonicalRequest{}, fmt.Errorf("audio speech response_format is unsupported")
	}
	if req.Speed != nil && (*req.Speed < 0.25 || *req.Speed > 4) {
		return gatewaycontract.CanonicalRequest{}, fmt.Errorf("audio speech speed must be between 0.25 and 4")
	}
	instructions := ""
	if req.Instructions != nil {
		instructions = strings.TrimSpace(*req.Instructions)
	}
	canonical := canonical(meta, gatewaycontract.ProtocolOpenAICompatible, gatewaycontract.ProtocolOpenAICompatible, model, "", false, audioSpeechPrompt(input, instructions), nil, audioSpeechContentBlocks(input, voice), instructions, nil)
	canonical.SpeechInput = input
	canonical.SpeechVoice = voice
	canonical.SpeechResponseFormat = format
	canonical.SpeechSpeed = cloneFloat32(req.Speed)
	canonical.SpeechInstructions = instructions
	if req.User != nil {
		canonical.SpeechUser = strings.TrimSpace(*req.User)
	}
	canonical.SpeechExtra = cloneMap(req.AdditionalProperties)
	canonical.RequestCapabilities = append(canonical.RequestCapabilities, gatewaycontract.RequestCapability{Key: capabilitiescontract.KeyAudioSpeech, Version: "v1"})
	return canonical, nil
}

func (s *Service) BuildCanonicalAudioSpeechResponse(req gatewaycontract.CanonicalRequest, id string, audio []byte, contentType string, usage gatewaycontract.Usage) gatewaycontract.AudioSpeechResponse {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = req.CanonicalModel
	}
	canonicalModel := strings.TrimSpace(req.CanonicalModel)
	if canonicalModel == "" {
		canonicalModel = model
	}
	id = strings.TrimSpace(id)
	if id == "" {
		id = "speech_" + randomHexString(12)
	}
	if shouldEstimateUsage(usage) {
		usage = audioSpeechEstimatedUsage(req)
	}
	return gatewaycontract.AudioSpeechResponse{
		ID:                    id,
		RequestID:             strings.TrimSpace(req.RequestID),
		Model:                 model,
		CanonicalModel:        canonicalModel,
		ContentType:           audioSpeechContentType(contentType, req.SpeechResponseFormat),
		Audio:                 append([]byte(nil), audio...),
		Usage:                 usage,
		CompatibilityWarnings: uniqueStrings(req.CompatibilityWarnings),
	}
}

func validAudioSpeechResponseFormat(format string) bool {
	switch strings.TrimSpace(format) {
	case "mp3", "opus", "aac", "flac", "wav", "pcm":
		return true
	default:
		return false
	}
}

func audioSpeechPrompt(input string, instructions string) string {
	parts := []string{strings.TrimSpace(input)}
	if instructions = strings.TrimSpace(instructions); instructions != "" {
		parts = append(parts, "instructions: "+instructions)
	}
	return strings.Join(uniqueStrings(parts), "\n")
}

func audioSpeechContentBlocks(input string, voice string) []gatewaycontract.ContentBlock {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	metadata := map[string]any{}
	if voice = strings.TrimSpace(voice); voice != "" {
		metadata["voice"] = voice
	}
	return []gatewaycontract.ContentBlock{{Type: gatewaycontract.ContentBlockText, Role: "user", Text: input, Metadata: metadata}}
}

func audioSpeechEstimatedUsage(req gatewaycontract.CanonicalRequest) gatewaycontract.Usage {
	input := estimateTokens(req.SpeechInput)
	if req.SpeechInstructions != "" {
		input += estimateTokens(req.SpeechInstructions)
	}
	output := max(1, len(req.SpeechInput)/12)
	return gatewaycontract.Usage{InputTokens: input, OutputTokens: output, Estimated: true}
}

func audioSpeechContentType(contentType string, format string) string {
	contentType = strings.TrimSpace(contentType)
	if contentType != "" {
		return contentType
	}
	switch strings.TrimSpace(format) {
	case "opus":
		return "audio/ogg"
	case "aac":
		return "audio/aac"
	case "flac":
		return "audio/flac"
	case "wav":
		return "audio/wav"
	case "pcm":
		return "application/octet-stream"
	default:
		return "audio/mpeg"
	}
}
