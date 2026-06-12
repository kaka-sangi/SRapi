package copilot

import (
	"context"
	"strings"
	"testing"

	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

func mustCatalog(t *testing.T) *Catalog {
	t.Helper()
	cat, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	return cat
}

func TestLoadCatalogFindsAdminOpsAndExcludesCopilot(t *testing.T) {
	cat := mustCatalog(t)
	if len(cat.Entries()) < 50 {
		t.Fatalf("expected many admin operations, got %d", len(cat.Entries()))
	}
	if _, ok := cat.byID["listAdminUsers"]; !ok {
		t.Fatalf("expected listAdminUsers in catalog")
	}
	for _, e := range cat.Entries() {
		if e.Path == copilotChatPath || e.Path == copilotConfigPath {
			t.Fatalf("copilot routes must be excluded from the catalog, found %s", e.Path)
		}
		if !strings.HasPrefix(e.Path, adminPathPrefix) {
			t.Fatalf("non-admin path leaked into catalog: %s", e.Path)
		}
	}
	if _, ok := cat.Lookup("GET", "/api/v1/admin/users/42"); !ok {
		t.Fatalf("Lookup should match a path with a concrete {id}")
	}
}

func TestLoadCatalogReusesParsedCatalog(t *testing.T) {
	first := mustCatalog(t)
	second := mustCatalog(t)
	if first != second {
		t.Fatal("expected LoadCatalog to reuse the parsed embedded OpenAPI catalog")
	}
}

func textResponse(text string) provideradaptercontract.ConversationResponse {
	return provideradaptercontract.ConversationResponse{
		Parts:      []provideradaptercontract.ContentPart{{Kind: provideradaptercontract.ContentPartText, Text: text}},
		StopReason: provideradaptercontract.StopReasonEndTurn,
	}
}

func toolUseResponse(id, name, args string) provideradaptercontract.ConversationResponse {
	return provideradaptercontract.ConversationResponse{
		Parts: []provideradaptercontract.ContentPart{{
			Kind:              provideradaptercontract.ContentPartToolUse,
			ToolCallID:        id,
			ToolName:          name,
			ToolArgumentsJSON: args,
		}},
		StopReason: provideradaptercontract.StopReasonToolUse,
	}
}

func TestSystemPromptIncludesOperationalGuidance(t *testing.T) {
	cat := mustCatalog(t)
	prompt := SystemPrompt(cat, true, false)
	for _, want := range []string{
		"get_operation_detail",
		"Never invent IDs",
		"GET the resource",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("system prompt missing operational guidance %q", want)
		}
	}
	if strings.Contains(prompt, "web_search") {
		t.Fatalf("web_search guidance must be absent when search is disabled")
	}
	if !strings.Contains(SystemPrompt(cat, true, true), "web_search") {
		t.Fatalf("web_search guidance must appear when search is enabled")
	}
}

func collectTypes(events []Event) []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		out = append(out, e.Type)
	}
	return out
}

func TestEngineReadFlowAutoRuns(t *testing.T) {
	eng := NewEngine(mustCatalog(t))
	calls := 0
	llm := func(_ context.Context, _ string, _ []provideradaptercontract.ConversationMessage, _ []map[string]any, _ func(string, string)) (provideradaptercontract.ConversationResponse, error) {
		calls++
		if calls == 1 {
			return toolUseResponse("c1", toolCallAdminAPI, `{"method":"GET","path":"/api/v1/admin/users"}`), nil
		}
		return textResponse("Here are the users."), nil
	}
	dispatched := false
	dispatch := func(_ context.Context, method, path string, _ []byte) (int, []byte, error) {
		dispatched = true
		if method != "GET" || path != "/api/v1/admin/users" {
			t.Fatalf("unexpected dispatch %s %s", method, path)
		}
		return 200, []byte(`{"data":[]}`), nil
	}
	var events []Event
	emit := func(e Event) { events = append(events, e) }

	_, err := eng.Run(context.Background(), Settings{Enabled: true, AutoRunReads: true, MaxSteps: 8}, []Message{{Role: RoleUser, Content: "list users"}}, nil, llm, dispatch, nil, emit)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !dispatched {
		t.Fatalf("expected the read call to auto-dispatch")
	}
	types := collectTypes(events)
	wantContains(t, types, EventStep)
	wantContains(t, types, EventToolCall)
	wantContains(t, types, EventToolResult)
	wantContains(t, types, EventDone)
	for _, ty := range types {
		if ty == EventPendingAction {
			t.Fatalf("read flow must not emit pending_action")
		}
	}
}

func TestEngineWriteFlowRequiresApproval(t *testing.T) {
	eng := NewEngine(mustCatalog(t))
	llm := func(_ context.Context, _ string, _ []provideradaptercontract.ConversationMessage, _ []map[string]any, _ func(string, string)) (provideradaptercontract.ConversationResponse, error) {
		return toolUseResponse("w1", toolCallAdminAPI, `{"method":"POST","path":"/api/v1/admin/users","body":{"email":"a@b.co"}}`), nil
	}
	dispatch := func(_ context.Context, _ string, _ string, _ []byte) (int, []byte, error) {
		t.Fatalf("mutation must not dispatch before approval")
		return 0, nil, nil
	}
	var events []Event
	emit := func(e Event) { events = append(events, e) }

	settings := Settings{Enabled: true, AutoRunReads: true, MaxSteps: 8}
	history, err := eng.Run(context.Background(), settings, []Message{{Role: RoleUser, Content: "create user"}}, nil, llm, dispatch, nil, emit)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	types := collectTypes(events)
	wantContains(t, types, EventPendingAction)
	for _, ty := range types {
		if ty == EventDone {
			t.Fatalf("pending mutation turn must not emit done")
		}
	}
	// The assistant tool-use must remain in history (unanswered) for resume.
	if pending, ok := unansweredToolCalls(history); !ok || pending[0].ID != "w1" {
		t.Fatalf("expected w1 pending after suspend, got %+v ok=%v", pending, ok)
	}

	// Resume with approval: now it executes, then the model wraps up.
	executed := false
	llm2 := func(_ context.Context, _ string, _ []provideradaptercontract.ConversationMessage, _ []map[string]any, _ func(string, string)) (provideradaptercontract.ConversationResponse, error) {
		return textResponse("Created."), nil
	}
	dispatch2 := func(_ context.Context, method, path string, _ []byte) (int, []byte, error) {
		executed = true
		if method != "POST" || path != "/api/v1/admin/users" {
			t.Fatalf("unexpected dispatch %s %s", method, path)
		}
		return 201, []byte(`{"data":{"id":7}}`), nil
	}
	var events2 []Event
	emit2 := func(e Event) { events2 = append(events2, e) }
	_, err = eng.Run(context.Background(), settings, history, &Approval{ToolCallID: "w1", Approved: true}, llm2, dispatch2, nil, emit2)
	if err != nil {
		t.Fatalf("resume Run: %v", err)
	}
	if !executed {
		t.Fatalf("approved mutation must dispatch on resume")
	}
	wantContains(t, collectTypes(events2), EventDone)
}

func TestEngineWriteFlowDenied(t *testing.T) {
	eng := NewEngine(mustCatalog(t))
	llm := func(_ context.Context, _ string, _ []provideradaptercontract.ConversationMessage, _ []map[string]any, _ func(string, string)) (provideradaptercontract.ConversationResponse, error) {
		return toolUseResponse("d1", toolCallAdminAPI, `{"method":"DELETE","path":"/api/v1/admin/announcements/3"}`), nil
	}
	// First turn: suspend.
	pendHistory, _ := eng.Run(context.Background(), Settings{Enabled: true, AutoRunReads: true, MaxSteps: 8}, []Message{{Role: RoleUser, Content: "delete it"}}, nil, llm,
		func(_ context.Context, _, _ string, _ []byte) (int, []byte, error) {
			t.Fatal("must not dispatch")
			return 0, nil, nil
		}, nil, func(Event) {})

	denyDispatched := false
	llm2 := func(_ context.Context, _ string, _ []provideradaptercontract.ConversationMessage, _ []map[string]any, _ func(string, string)) (provideradaptercontract.ConversationResponse, error) {
		return textResponse("Okay, leaving it."), nil
	}
	var events []Event
	_, err := eng.Run(context.Background(), Settings{Enabled: true, AutoRunReads: true, MaxSteps: 8}, pendHistory, &Approval{ToolCallID: "d1", Approved: false}, llm2,
		func(_ context.Context, _, _ string, _ []byte) (int, []byte, error) {
			denyDispatched = true
			return 0, nil, nil
		},
		nil,
		func(e Event) { events = append(events, e) })
	if err != nil {
		t.Fatalf("resume Run: %v", err)
	}
	if denyDispatched {
		t.Fatalf("denied mutation must not dispatch")
	}
	wantContains(t, collectTypes(events), EventDone)
}

func TestToAdapterMessagesMapsImages(t *testing.T) {
	history := []Message{{
		Role:    RoleUser,
		Content: "what is in this image?",
		Images:  []MessageImage{{MIMEType: "image/png", Data: "QUJD"}},
	}}
	msgs := toAdapterMessages(history)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 adapter message, got %d", len(msgs))
	}
	var image *provideradaptercontract.ContentPart
	for i := range msgs[0].Parts {
		if msgs[0].Parts[i].Kind == provideradaptercontract.ContentPartImage {
			image = &msgs[0].Parts[i]
		}
	}
	if image == nil {
		t.Fatalf("expected an image content part, got parts %+v", msgs[0].Parts)
	}
	if image.MediaBase64 != "QUJD" || image.MIMEType != "image/png" {
		t.Fatalf("image part not mapped correctly: %+v", image)
	}
}

func wantContains(t *testing.T, haystack []string, needle string) {
	t.Helper()
	for _, h := range haystack {
		if h == needle {
			return
		}
	}
	t.Fatalf("expected events to contain %q, got %v", needle, haystack)
}
