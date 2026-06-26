package httpserver

import "strings"

const defaultImageGenerationBaseModel = "gpt-5.4-mini"

// gatewayImageModelRedirect checks if a resolved model is an image-only model
// (e.g. gpt-image-2) being requested through a conversational endpoint
// (chat/completions, responses, messages). Image-only models can't do chat;
// the request should be redirected to a capable chat model that has the
// image_generation tool. Returns the redirect model name, or empty if no
// redirect is needed.
func gatewayImageModelRedirect(canonicalModel string, sourceEndpoint string) string {
	model := strings.ToLower(strings.TrimSpace(canonicalModel))
	if !strings.HasPrefix(model, "gpt-image") && !strings.HasPrefix(model, "dall-e") {
		return ""
	}
	endpoint := strings.ToLower(strings.TrimSpace(sourceEndpoint))
	if strings.Contains(endpoint, "/images/") {
		return "" // already on the correct image endpoint
	}
	return defaultImageGenerationBaseModel
}
