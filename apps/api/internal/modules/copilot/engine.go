package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

// runawayStepGuard is a safety backstop on the agentic loop: it only stops a
// runaway (looping) model from spinning forever — it is NOT a per-turn feature
// limit, and no real task approaches it. A turn otherwise runs until the model
// stops calling tools, or the admin hits Stop (which cancels the context).
//
// Tool results and catalog schemas are fed to the model in full (no byte cap):
// completeness beats token thrift for an operator assistant.
const runawayStepGuard = 100

// LLMFunc invokes the configured model with the system prompt, conversation, and
// tools. It streams content/reasoning fragments via onDelta (kind is "content"
// or "reasoning") as they arrive, and returns the final assembled response
// (which may contain tool calls). A buffered implementation may call onDelta
// once with the full text.
type LLMFunc func(ctx context.Context, system string, messages []provideradaptercontract.ConversationMessage, tools []map[string]any, onDelta func(kind, text string)) (provideradaptercontract.ConversationResponse, error)

// DispatchFunc executes one admin API call in-process and returns the HTTP
// status and response body.
type DispatchFunc func(ctx context.Context, method, path string, body []byte) (int, []byte, error)

// Engine runs the agentic loop. It is stateless across turns; the caller
// round-trips the conversation history.
type Engine struct {
	catalog *Catalog
}

// NewEngine constructs an Engine over the admin operation catalog.
func NewEngine(catalog *Catalog) *Engine { return &Engine{catalog: catalog} }

// Run executes one copilot turn, streaming events via emit. It returns the
// updated history. When a mutation needs approval it emits a pending_action and
// returns without a done event; the caller's client resumes by re-sending the
// history plus an Approval.
func (e *Engine) Run(ctx context.Context, settings Settings, history []Message, approval *Approval, llm LLMFunc, dispatch DispatchFunc, search SearchFunc, emit func(Event)) ([]Message, error) {
	system := SystemPrompt(e.catalog, settings.AutoRunReads, search != nil)
	tools := MetaToolSchemas()
	if search != nil {
		tools = append(tools, webSearchToolSchema())
	}
	steps := 0

	for {
		if ctx.Err() != nil {
			return history, ctx.Err()
		}
		pending, hasPending := unansweredToolCalls(history)
		if !hasPending {
			if steps >= runawayStepGuard {
				emit(Event{Type: EventError, Data: ErrorData{Message: "stopped after too many steps — the model may be looping; try rephrasing the request"}})
				return history, nil
			}
			steps++
			emit(Event{Type: EventStep, Data: StepData{Step: steps}})
			// Stream content/reasoning fragments to the client as they arrive.
			onDelta := func(kind, text string) {
				if text == "" {
					return
				}
				switch kind {
				case "reasoning":
					emit(Event{Type: EventAssistantReasoning, Data: AssistantReasoningData{Text: text}})
				case "content":
					emit(Event{Type: EventAssistantDelta, Data: AssistantDeltaData{Text: text}})
				}
			}
			resp, err := llm(ctx, system, toAdapterMessages(history), tools, onDelta)
			if err != nil {
				emit(Event{Type: EventError, Data: ErrorData{Message: llmErrorMessage(err)}})
				return history, nil
			}
			if resp.Usage.InputTokens > 0 || resp.Usage.OutputTokens > 0 {
				emit(Event{Type: EventUsage, Data: UsageData{InputTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens}})
			}
			assistant := assistantFromResponse(resp)
			history = append(history, assistant)
			// content + reasoning were already streamed via onDelta above.
			for _, tc := range assistant.ToolCalls {
				emit(Event{Type: EventToolCall, Data: ToolCallData{ToolCallID: tc.ID, Name: tc.Name, Arguments: tc.ArgumentsJSON}})
			}
			if len(assistant.ToolCalls) == 0 {
				emit(Event{Type: EventDone, Data: DoneData{Messages: history}})
				return history, nil
			}
			continue
		}

		// Answer the pending tool calls in order.
		for _, tc := range pending {
			switch tc.Name {
			case toolGetOperationDetail, toolGetSchema:
				content, isErr := e.executeLocalTool(tc)
				history = appendToolResult(history, tc, 0, content, isErr)
				emit(Event{Type: EventToolResult, Data: ToolResultData{ToolCallID: tc.ID, Name: tc.Name, Content: content, IsError: isErr}})
			case toolWebSearch:
				// Read-only external lookup: runs immediately, no approval.
				content := "web search is not configured"
				isErr := true
				if search != nil {
					content, isErr = runWebSearch(ctx, search, tc.ArgumentsJSON)
				}
				history = appendToolResult(history, tc, 0, content, isErr)
				emit(Event{Type: EventToolResult, Data: ToolResultData{ToolCallID: tc.ID, Name: tc.Name, Content: content, IsError: isErr}})
			case toolCallAdminAPI:
				method, path, body, perr := parseCallAdminArgs(tc.ArgumentsJSON)
				if perr != nil {
					msg := "invalid call_admin_api arguments: " + perr.Error()
					history = appendToolResult(history, tc, 0, msg, true)
					emit(Event{Type: EventToolResult, Data: ToolResultData{ToolCallID: tc.ID, Name: tc.Name, Content: msg, IsError: true}})
					continue
				}
				needsConfirm := isMutationMethod(method) || !settings.AutoRunReads
				if needsConfirm {
					if approval != nil && approval.ToolCallID == tc.ID {
						approved := approval.Approved
						approval = nil // consume; a later pending write needs its own round-trip
						if !approved {
							msg := "The administrator declined this action."
							history = appendToolResult(history, tc, 0, msg, true)
							emit(Event{Type: EventToolResult, Data: ToolResultData{ToolCallID: tc.ID, Name: tc.Name, Content: msg, IsError: true}})
							continue
						}
						// approved: fall through to execution below
					} else {
						emit(Event{Type: EventPendingAction, Data: PendingActionData{
							ToolCallID: tc.ID,
							Name:       tc.Name,
							Method:     method,
							Path:       path,
							Body:       string(body),
							Summary:    e.summarize(method, path),
							Danger:     strings.EqualFold(method, "DELETE"),
						}})
						return history, nil // suspend; resume on approval
					}
				}
				status, content, isErr := e.execute(ctx, dispatch, method, path, body)
				history = appendToolResult(history, tc, status, content, isErr)
				emit(Event{Type: EventToolResult, Data: ToolResultData{ToolCallID: tc.ID, Name: tc.Name, Status: status, Content: content, IsError: isErr}})
			default:
				msg := "unknown tool: " + tc.Name
				history = appendToolResult(history, tc, 0, msg, true)
				emit(Event{Type: EventToolResult, Data: ToolResultData{ToolCallID: tc.ID, Name: tc.Name, Content: msg, IsError: true}})
			}
		}
		// All pending answered; loop to let the model react.
	}
}

// execute dispatches an admin call and formats its result for the model.
func (e *Engine) execute(ctx context.Context, dispatch DispatchFunc, method, path string, body []byte) (int, string, bool) {
	status, respBody, err := dispatch(ctx, method, path, body)
	if err != nil {
		return 0, "request failed: " + err.Error(), true
	}
	text := strings.TrimSpace(string(respBody))
	if text == "" {
		text = "(empty response)"
	}
	formatted := fmt.Sprintf("HTTP %d\n%s", status, text)
	return status, formatted, status >= 400
}

func (e *Engine) executeLocalTool(tc ToolCall) (string, bool) {
	switch tc.Name {
	case toolGetOperationDetail:
		var args struct {
			OperationID string `json:"operation_id"`
		}
		if err := json.Unmarshal([]byte(orEmptyJSON(tc.ArgumentsJSON)), &args); err != nil {
			return "invalid arguments: " + err.Error(), true
		}
		detail, ok := e.catalog.OperationDetail(strings.TrimSpace(args.OperationID))
		if !ok {
			return fmt.Sprintf("unknown operation_id %q", args.OperationID), true
		}
		return marshalJSON(detail), false
	case toolGetSchema:
		var args struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal([]byte(orEmptyJSON(tc.ArgumentsJSON)), &args); err != nil {
			return "invalid arguments: " + err.Error(), true
		}
		schema, ok := e.catalog.Schema(strings.TrimSpace(args.Name))
		if !ok {
			return fmt.Sprintf("unknown schema %q", args.Name), true
		}
		return marshalJSON(schema), false
	default:
		return "unknown tool", true
	}
}

func (e *Engine) summarize(method, path string) string {
	if entry, ok := e.catalog.Lookup(method, path); ok && entry.Summary != "" {
		return entry.Summary
	}
	return method + " " + path
}

// unansweredToolCalls finds the trailing assistant tool-call block (if any) and
// returns the tool calls that do not yet have a result. hasPending is false when
// the next action is to call the model.
func unansweredToolCalls(history []Message) ([]ToolCall, bool) {
	la := -1
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == RoleAssistant && len(history[i].ToolCalls) > 0 {
			la = i
			break
		}
		if history[i].Role == RoleUser {
			return nil, false // a newer user turn supersedes any prior tool block
		}
	}
	if la < 0 {
		return nil, false
	}
	answered := map[string]bool{}
	for _, m := range history[la+1:] {
		if m.Role != RoleTool {
			continue
		}
		for _, tr := range m.ToolResults {
			answered[tr.ToolCallID] = true
		}
	}
	var pending []ToolCall
	for _, tc := range history[la].ToolCalls {
		if !answered[tc.ID] {
			pending = append(pending, tc)
		}
	}
	return pending, len(pending) > 0
}

func appendToolResult(history []Message, tc ToolCall, _ int, content string, isErr bool) []Message {
	return append(history, Message{
		Role:        RoleTool,
		ToolResults: []ToolResult{{ToolCallID: tc.ID, Content: content, IsError: isErr}},
	})
}

func assistantFromResponse(resp provideradaptercontract.ConversationResponse) Message {
	msg := Message{Role: RoleAssistant}
	var text, reasoning strings.Builder
	for _, part := range resp.Parts {
		switch part.Kind {
		case provideradaptercontract.ContentPartText:
			text.WriteString(part.Text)
		case provideradaptercontract.ContentPartThinking:
			reasoning.WriteString(part.Text)
		case provideradaptercontract.ContentPartToolUse:
			msg.ToolCalls = append(msg.ToolCalls, ToolCall{
				ID:            part.ToolCallID,
				Name:          part.ToolName,
				ArgumentsJSON: part.ToolArgumentsJSON,
			})
		}
	}
	msg.Content = strings.TrimSpace(text.String())
	msg.Reasoning = strings.TrimSpace(reasoning.String())
	return msg
}

func toAdapterMessages(history []Message) []provideradaptercontract.ConversationMessage {
	out := make([]provideradaptercontract.ConversationMessage, 0, len(history))
	for _, m := range history {
		switch m.Role {
		case RoleUser:
			parts := []provideradaptercontract.ContentPart{{Kind: provideradaptercontract.ContentPartText, Text: m.Content}}
			for _, img := range m.Images {
				if strings.TrimSpace(img.Data) == "" {
					continue
				}
				parts = append(parts, provideradaptercontract.ContentPart{
					Kind:        provideradaptercontract.ContentPartImage,
					MediaBase64: img.Data,
					MIMEType:    img.MIMEType,
				})
			}
			out = append(out, provideradaptercontract.ConversationMessage{Role: "user", Parts: parts})
		case RoleAssistant:
			parts := make([]provideradaptercontract.ContentPart, 0, len(m.ToolCalls)+1)
			if strings.TrimSpace(m.Content) != "" {
				parts = append(parts, provideradaptercontract.ContentPart{Kind: provideradaptercontract.ContentPartText, Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				parts = append(parts, provideradaptercontract.ContentPart{
					Kind:              provideradaptercontract.ContentPartToolUse,
					ToolCallID:        tc.ID,
					ToolName:          tc.Name,
					ToolArgumentsJSON: orEmptyJSON(tc.ArgumentsJSON),
				})
			}
			out = append(out, provideradaptercontract.ConversationMessage{Role: "assistant", Parts: parts})
		case RoleTool:
			for _, tr := range m.ToolResults {
				out = append(out, provideradaptercontract.ConversationMessage{
					Role: "tool",
					Parts: []provideradaptercontract.ContentPart{{
						Kind:              provideradaptercontract.ContentPartToolResult,
						ToolResultForID:   tr.ToolCallID,
						Text:              tr.Content,
						ToolResultIsError: tr.IsError,
					}},
				})
			}
		}
	}
	return out
}

func parseCallAdminArgs(argsJSON string) (method, path string, body []byte, err error) {
	var args struct {
		Method string          `json:"method"`
		Path   string          `json:"path"`
		Body   json.RawMessage `json:"body"`
	}
	if err = json.Unmarshal([]byte(orEmptyJSON(argsJSON)), &args); err != nil {
		return "", "", nil, err
	}
	method = strings.ToUpper(strings.TrimSpace(args.Method))
	path = strings.TrimSpace(args.Path)
	if method == "" || path == "" {
		return "", "", nil, fmt.Errorf("method and path are required")
	}
	if !strings.HasPrefix(path, adminPathPrefix) {
		return "", "", nil, fmt.Errorf("path must begin with %s", adminPathPrefix)
	}
	if len(args.Body) > 0 && string(args.Body) != "null" {
		body = []byte(args.Body)
	}
	return method, path, body, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n…(truncated)"
}

func orEmptyJSON(s string) string {
	if strings.TrimSpace(s) == "" {
		return "{}"
	}
	return s
}

// marshalJSON renders a catalog lookup (operation detail / schema) for the model
// in full, so no required field is ever hidden by truncation.
func marshalJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("failed to encode: %v", err)
	}
	return string(b)
}

func llmErrorMessage(err error) string {
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "rate limit") || strings.Contains(lower, "429") || strings.Contains(lower, "too many requests"):
		return "Rate limited by the model provider — retry in a moment"
	case strings.Contains(lower, "context length") || (strings.Contains(lower, "maximum") && strings.Contains(lower, "token")) || strings.Contains(lower, "too long"):
		return "Conversation too long for the model — start a new conversation"
	case strings.Contains(lower, "unauthorized") || strings.Contains(lower, "401") || (strings.Contains(lower, "invalid") && strings.Contains(lower, "key")) || strings.Contains(lower, "authentication"):
		return "Model authentication failed — check the copilot API key in settings"
	case strings.Contains(lower, "model") && (strings.Contains(lower, "not found") || strings.Contains(lower, "not exist") || strings.Contains(lower, "not available")):
		return "The selected model is not available — change it in copilot settings"
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline exceeded"):
		return "Model call timed out — retry or try a smaller request"
	case strings.Contains(lower, "connection refused") || strings.Contains(lower, "no such host") || strings.Contains(lower, "dns"):
		return "Cannot reach the model provider — check network and base URL"
	default:
		return "Model call failed: " + msg
	}
}
