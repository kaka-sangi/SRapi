package service

import (
	"net/http"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

func (s *Service) codexPrepareResponsesPayload(req contract.ConversationRequest, payload map[string]any) (codexReasoningReplayScope, error) {
	if !imageGenerationDisabledForConversation(req) {
		if codexImageGenerationBridgeEnabled(req) || codexPayloadInputUsesImageGenerationCall(payload) {
			ensureCodexImageGenerationTool(payload, contract.NormalizeCodexUpstreamModelName(codexStringValue(payload["model"])), req.Account)
			// codexApplyImageGenerationInstructions only added the bridge
			// marker when the tool was already in the array; re-run it now
			// so the freshly-injected tool gets the matching instructions.
			codexApplyImageGenerationInstructions(payload)
		}
	}
	// Global config modes ported from CLIProxyAPI. The alias swap runs before
	// marshal so upstream receives the resolved model; the disable gate runs
	// after auto-injection so deployment policy remains authoritative.
	if aliased := ResolveCodexModelAlias(s.cfg, "openai", codexStringValue(payload["model"])); aliased != "" {
		payload["model"] = aliased
	}
	if ShouldDisableCodexImageGeneration(s.cfg, codexUserAgent(req)) && codexPayloadHasImageGenerationTool(payload) {
		return codexReasoningReplayScope{}, contract.ProviderError{
			Class:      "image_generation_disabled",
			StatusCode: http.StatusBadRequest,
			Message:    "image_generation tool is disabled for this gateway deployment",
		}
	}
	return codexApplyReasoningReplay(req, payload), nil
}
