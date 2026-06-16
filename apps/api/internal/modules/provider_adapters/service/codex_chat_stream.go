package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

// codexChatStreamRawCap bounds how many raw upstream SSE bytes we retain for the
// final usage parse. Beyond this the metering parse degrades to the admission
// estimate (mirroring the gateway's own meter cap) rather than growing without
// bound on a pathologically large response.
const codexChatStreamRawCap = 16 << 20

// codexChatStreamReader adapts an upstream Codex /responses SSE body into an
// OpenAI chat.completion.chunk SSE stream, incrementally. It exists so a client
// speaking the chat/completions protocol that is routed to a Codex CLI backend
// receives correctly-shaped, token-by-token streamed chunks instead of the raw
// Responses-API events (which it cannot parse) or a buffered single response.
//
// It transforms on the fly: each upstream SSE frame is read, mapped to zero or
// more chat.completion.chunk frames, and surfaced through Read as soon as it is
// produced — so the gateway's passthrough writer flushes each delta immediately.
//
// The raw upstream bytes are also retained (capped) so the gateway's StreamParse
// hook can recover usage/id/stop-reason via the existing, tested
// parseCodexResponsesBody — keeping billing accurate without re-deriving token
// accounting here.
type codexChatStreamReader struct {
	upstream io.ReadCloser
	scanner  *bufio.Scanner

	out bytes.Buffer // pending transformed chat SSE bytes awaiting Read
	raw bytes.Buffer // accumulated raw upstream SSE (capped) for final usage parse

	id        string
	model     string
	created   int64
	rawCapped bool
	roleSent  bool
	sawTool   bool
	done      bool

	toolIndex map[string]int
	nextTool  int

	// usage holds the token usage captured from the terminal event so a trailing
	// usage chunk can be emitted before [DONE], matching OpenAI streaming.
	usage *contract.Usage
}

func newCodexChatStreamReader(upstream io.ReadCloser, req contract.ConversationRequest) *codexChatStreamReader {
	scanner := bufio.NewScanner(upstream)
	scanner.Buffer(make([]byte, 0, 64*1024), 4<<20)
	id := strings.TrimSpace(req.RequestID)
	if id == "" {
		id = "codex_stream"
	}
	return &codexChatStreamReader{
		upstream: upstream,
		scanner:  scanner,
		id:       "chatcmpl_" + id,
		model:    strings.TrimSpace(req.Model),
		created:  time.Now().Unix(),
	}
}

func (r *codexChatStreamReader) Read(p []byte) (int, error) {
	for r.out.Len() == 0 && !r.done {
		r.pump()
	}
	if r.out.Len() > 0 {
		return r.out.Read(p)
	}
	return 0, io.EOF
}

func (r *codexChatStreamReader) Close() error {
	if r.upstream != nil {
		return r.upstream.Close()
	}
	return nil
}

// rawBytes returns the retained raw upstream SSE for the metering parse.
func (r *codexChatStreamReader) rawBytes() []byte { return r.raw.Bytes() }

// pump reads exactly one upstream SSE frame and appends the chat chunk(s) it maps
// to. On upstream EOF it finalizes the chat stream.
func (r *codexChatStreamReader) pump() {
	var eventField string
	var dataLines []string
	gotFrame := false
	for r.scanner.Scan() {
		line := strings.TrimRight(r.scanner.Text(), "\r")
		r.accumRaw(line)
		if line == "" {
			gotFrame = true
			break
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, ok := strings.Cut(line, ":")
		if !ok {
			field, value = line, ""
		}
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			eventField = value
		case "data":
			dataLines = append(dataLines, value)
		}
	}
	if !gotFrame {
		// Upstream ended (EOF or read error): finalize the chat stream.
		r.finish("")
		return
	}
	data := strings.TrimSpace(strings.Join(dataLines, "\n"))
	if data == "" {
		return
	}
	if data == "[DONE]" {
		r.finish("")
		return
	}
	r.handleEvent(eventField, data)
}

func (r *codexChatStreamReader) handleEvent(eventField, data string) {
	var ev codexResponsesEvent
	if err := json.Unmarshal([]byte(data), &ev); err != nil {
		return
	}
	switch firstNonEmpty(strings.TrimSpace(ev.Type), strings.TrimSpace(eventField)) {
	case "response.output_text.delta":
		if ev.Delta != "" {
			r.emitDelta(map[string]any{"content": ev.Delta})
		}
	case "response.reasoning_text.delta", "response.reasoning_summary_text.delta":
		if ev.Delta != "" {
			r.emitDelta(map[string]any{"reasoning_content": ev.Delta})
		}
	case "response.refusal.delta":
		if ev.Delta != "" {
			r.emitDelta(map[string]any{"refusal": ev.Delta})
		}
	case "response.output_item.added":
		if ev.Item != nil && codexOutputItemIsFunctionCall(*ev.Item) {
			r.sawTool = true
			r.emitDelta(map[string]any{"tool_calls": []map[string]any{{
				"index": r.toolIndexFor(ev),
				"id":    firstNonEmpty(ev.Item.CallID, ev.Item.ID),
				"type":  "function",
				"function": map[string]any{
					"name":      ev.Item.Name,
					"arguments": "",
				},
			}}})
		}
	case "response.function_call_arguments.delta", "response.custom_tool_call_input.delta":
		// custom/freeform tool calls stream their arguments under
		// custom_tool_call_input.delta; without this alias the args were silently
		// dropped and the client saw a tool call with empty arguments.
		if ev.Delta != "" {
			r.sawTool = true
			r.emitDelta(map[string]any{"tool_calls": []map[string]any{{
				"index":    r.toolIndexFor(ev),
				"function": map[string]any{"arguments": ev.Delta},
			}}})
		}
	case "response.completed", "response.done", "response.incomplete", "response.cancelled", "response.canceled", "response.failed":
		if u := codexEventUsage(ev, ""); u.InputTokens > 0 || u.OutputTokens > 0 {
			r.usage = &u
		}
		r.finish(r.terminalFinishReason(ev))
	}
}

// emitDelta writes a chat chunk for delta, preceded once by the leading
// assistant-role chunk OpenAI clients expect as the first event.
func (r *codexChatStreamReader) emitDelta(delta map[string]any) {
	r.ensureRole()
	r.writeChunk(delta, nil)
}

func (r *codexChatStreamReader) ensureRole() {
	if r.roleSent {
		return
	}
	r.roleSent = true
	r.writeChunk(map[string]any{"role": "assistant", "content": ""}, nil)
}

func (r *codexChatStreamReader) finish(reason string) {
	if r.done {
		return
	}
	r.ensureRole()
	if reason == "" {
		if r.sawTool {
			reason = "tool_calls"
		} else {
			reason = "stop"
		}
	}
	r.writeChunk(map[string]any{}, reason)
	// Emit a trailing usage chunk (choices:[] + usage) before [DONE] so streaming
	// clients can read token usage, matching OpenAI and the buffered codex path.
	if r.usage != nil {
		r.writeUsageChunk(*r.usage)
	}
	r.out.WriteString("data: [DONE]\n\n")
	r.done = true
}

// writeUsageChunk emits a terminal chat.completion.chunk that carries only the
// token usage (empty choices), as OpenAI does when stream usage is requested.
func (r *codexChatStreamReader) writeUsageChunk(usage contract.Usage) {
	chunk := map[string]any{
		"id":      r.id,
		"object":  "chat.completion.chunk",
		"created": r.created,
		"model":   r.model,
		"choices": []map[string]any{},
		"usage": map[string]any{
			"prompt_tokens":     usage.InputTokens,
			"completion_tokens": usage.OutputTokens,
			"total_tokens":      usage.InputTokens + usage.OutputTokens,
		},
	}
	encoded, err := json.Marshal(chunk)
	if err != nil {
		return
	}
	r.out.WriteString("data: ")
	r.out.Write(encoded)
	r.out.WriteString("\n\n")
}

func (r *codexChatStreamReader) terminalFinishReason(ev codexResponsesEvent) string {
	// A token-limit truncation is the terminal cause even if the model had begun
	// tool calls: OpenAI reports finish_reason "length" for a truncated turn, so
	// check IncompleteDetails before falling back to tool_calls.
	if ev.Response != nil && ev.Response.IncompleteDetails != nil && strings.TrimSpace(ev.Response.IncompleteDetails.Reason) != "" {
		return "length"
	}
	if r.sawTool {
		return "tool_calls"
	}
	return "stop"
}

func (r *codexChatStreamReader) writeChunk(delta map[string]any, finishReason any) {
	chunk := map[string]any{
		"id":      r.id,
		"object":  "chat.completion.chunk",
		"created": r.created,
		"model":   r.model,
		"choices": []map[string]any{
			{
				"index":         0,
				"delta":         delta,
				"finish_reason": finishReason,
			},
		},
	}
	encoded, err := json.Marshal(chunk)
	if err != nil {
		return
	}
	r.out.WriteString("data: ")
	r.out.Write(encoded)
	r.out.WriteString("\n\n")
}

// toolIndexFor maps an upstream function-call (identified by output index or item
// id) to a stable, zero-based chat tool_call index.
func (r *codexChatStreamReader) toolIndexFor(ev codexResponsesEvent) int {
	key := ""
	switch {
	case ev.OutputIndex != nil:
		key = "o" + strconv.Itoa(*ev.OutputIndex)
	case strings.TrimSpace(ev.ItemID) != "":
		key = "i" + strings.TrimSpace(ev.ItemID)
	case ev.Item != nil:
		key = "i" + firstNonEmpty(ev.Item.ID, ev.Item.CallID)
	}
	if r.toolIndex == nil {
		r.toolIndex = map[string]int{}
	}
	if idx, ok := r.toolIndex[key]; ok {
		return idx
	}
	idx := r.nextTool
	r.nextTool++
	r.toolIndex[key] = idx
	return idx
}

func (r *codexChatStreamReader) accumRaw(line string) {
	if r.rawCapped {
		return
	}
	if r.raw.Len()+len(line)+1 > codexChatStreamRawCap {
		r.rawCapped = true
		return
	}
	r.raw.WriteString(line)
	r.raw.WriteByte('\n')
}
