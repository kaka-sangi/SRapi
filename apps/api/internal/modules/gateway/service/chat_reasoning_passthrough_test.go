package service

import (
	"strings"
	"testing"

	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// TestNormalizeChatCompletionsPreservesAssistantReasoning guards multi-turn
// thinking passthrough: an inbound assistant message carrying reasoning_content
// must keep that prior chain-of-thought (wrapped as <thinking>...</thinking>) in
// the canonical request so it is re-sent upstream, rather than being silently
// dropped during Chat->canonical normalization.
func TestNormalizeChatCompletionsPreservesAssistantReasoning(t *testing.T) {
	svc, err := New()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	var userContent apiopenapi.ChatMessage_Content
	if err := userContent.FromChatMessageContent0("what is it?"); err != nil {
		t.Fatalf("build user content: %v", err)
	}
	var assistantContent apiopenapi.ChatMessage_Content
	if err := assistantContent.FromChatMessageContent0("the answer is 42"); err != nil {
		t.Fatalf("build assistant content: %v", err)
	}
	reasoning := "first I considered X then Y"
	req := apiopenapi.ChatCompletionRequest{
		Model: "gpt-5.5",
		Messages: []apiopenapi.ChatMessage{
			{Role: apiopenapi.ChatMessageRoleUser, Content: userContent},
			{Role: apiopenapi.ChatMessageRoleAssistant, Content: assistantContent, ReasoningContent: &reasoning},
		},
	}

	canonical := svc.NormalizeChatCompletions(req, RequestMeta{RequestID: "req_reason", CanonicalModel: "gpt-5.5"})

	var assistantText string
	for _, m := range canonical.Messages {
		if m.Role == "assistant" {
			for _, b := range m.Content {
				assistantText += b.Text
			}
		}
	}
	if !strings.Contains(assistantText, "<thinking>first I considered X then Y</thinking>") {
		t.Fatalf("assistant message must carry prior reasoning as <thinking>, got %q", assistantText)
	}
	if !strings.Contains(assistantText, "the answer is 42") {
		t.Fatalf("assistant answer text must survive alongside the reasoning, got %q", assistantText)
	}
}
