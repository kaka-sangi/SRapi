package service

import (
	"bytes"
	"encoding/json"
	"strings"
)

// VERBATIM PORT FROM sub2api openai_gateway_service.go:4007
// handlePassthroughSSEToJSON + extractCodexFinalResponse +
// reconstructResponseOutputFromSSE + apicompat.BufferedResponseAccumulator
// (responses_to_chatcompletions.go:432-519).
//
// The strategy, verbatim from sub2api: when the upstream returns
// text/event-stream for a non-streaming request (codex /compact and
// some /responses paths do this regardless of the request's
// stream=false), extract the terminal `response.completed` event's
// `response` object as the JSON body. If that body's `output` array
// is empty, walk every delta event in the SSE stream and rebuild the
// output[] from accumulated text / reasoning / function-call deltas.
// Emit the resulting body with Content-Type: application/json.
//
// Why this finally works after multiple wrong fixes: the upstream
// Codex backend on /responses/compact ships the summary text via
// response.output_text.delta events and leaves the terminal event's
// `response.output` array empty. Without the output[] reconstruction,
// the JSON body emitted to codex CLI contains no `text` anywhere —
// the Rust serde parser flags this as "missing field `text` at line
// 1 column N", with N being wherever its struct member walker
// happened to fail.

// codexBodyLooksLikeSSE detects raw text/event-stream payloads.
func codexBodyLooksLikeSSE(body []byte) bool {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return false
	}
	return bytes.HasPrefix(trimmed, []byte("data:")) || bytes.Contains(trimmed, []byte("\ndata:"))
}

// codexExtractSSEDataLine mirrors sub2api extractOpenAISSEDataLine
// (openai_gateway_service.go:4941): strip the "data:" prefix and one
// optional space, return the remaining payload string.
func codexExtractSSEDataLine(line string) (string, bool) {
	if !strings.HasPrefix(line, "data:") {
		return "", false
	}
	start := len("data:")
	for start < len(line) {
		if line[start] != ' ' && line[start] != '\t' {
			break
		}
		start++
	}
	return line[start:], true
}

// codexForEachSSEDataPayload iterates over every payload (one event's
// `data:` lines joined) in an SSE body and calls fn with the bytes.
// Mirrors sub2api forEachOpenAISSEDataPayload + emitOpenAISSEDataPayload
// (openai_sse_data.go:35-70). Handles multi-line data: blocks by
// joining consecutive data: lines until a blank line.
func codexForEachSSEDataPayload(body string, fn func([]byte)) {
	if fn == nil || strings.TrimSpace(body) == "" {
		return
	}
	var lines []string
	flush := func() {
		if len(lines) == 0 {
			return
		}
		var data string
		if len(lines) == 1 {
			data = lines[0]
		} else {
			joined := strings.Join(lines, "\n")
			if json.Valid([]byte(joined)) {
				data = joined
			} else {
				// fall back to emitting each line individually
				for _, line := range lines {
					trimmed := strings.TrimSpace(line)
					if trimmed != "" && trimmed != "[DONE]" {
						fn([]byte(trimmed))
					}
				}
				lines = lines[:0]
				return
			}
		}
		lines = lines[:0]
		trimmed := strings.TrimSpace(data)
		if trimmed == "" || trimmed == "[DONE]" {
			return
		}
		fn([]byte(trimmed))
	}
	for _, raw := range strings.Split(body, "\n") {
		trimmedLine := strings.TrimRight(raw, "\r\n")
		if payload, ok := codexExtractSSEDataLine(trimmedLine); ok {
			lines = append(lines, payload)
			continue
		}
		if strings.TrimSpace(trimmedLine) == "" {
			flush()
		}
	}
	flush()
}

// codexExtractFinalResponse walks an SSE body and returns the `response`
// field from the first response.completed or response.done event it
// finds. Mirrors sub2api extractCodexFinalResponse
// (openai_gateway_service.go:5329).
func codexExtractFinalResponse(body string) ([]byte, bool) {
	var final []byte
	codexForEachSSEDataPayload(body, func(data []byte) {
		if final != nil {
			return
		}
		var event struct {
			Type     string          `json:"type"`
			Response json.RawMessage `json:"response"`
		}
		if err := json.Unmarshal(data, &event); err != nil {
			return
		}
		if event.Type != "response.completed" && event.Type != "response.done" {
			return
		}
		response := bytes.TrimSpace(event.Response)
		if len(response) == 0 || bytes.Equal(response, []byte("null")) {
			return
		}
		final = append([]byte(nil), response...)
	})
	if final != nil {
		return final, true
	}
	return nil, false
}

// codexResponsesStreamEvent is the wire shape sub2api uses to
// deserialize each SSE event for the accumulator. Mirrors the subset
// of apicompat.ResponsesStreamEvent (types.go:382) the accumulator
// reads.
type codexResponsesStreamEvent struct {
	Type        string                    `json:"type"`
	Delta       string                    `json:"delta"`
	OutputIndex int                       `json:"output_index"`
	Item        *codexResponsesStreamItem `json:"item,omitempty"`
}

type codexResponsesStreamItem struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
	Name   string `json:"name"`
}

// codexBufferedFuncCall holds an in-flight function-call accumulation.
type codexBufferedFuncCall struct {
	CallID string
	Name   string
	Args   strings.Builder
}

// codexBufferedResponseAccumulator mirrors sub2api
// apicompat.BufferedResponseAccumulator (responses_to_chatcompletions.go
// :432-519). Collects deltas from SSE events so the non-streaming path
// can rebuild the output array when the terminal event delivered an
// empty one.
type codexBufferedResponseAccumulator struct {
	text                 strings.Builder
	reasoning            strings.Builder
	funcCalls            []codexBufferedFuncCall
	outputIndexToFuncIdx map[int]int
}

func newCodexBufferedResponseAccumulator() *codexBufferedResponseAccumulator {
	return &codexBufferedResponseAccumulator{
		outputIndexToFuncIdx: make(map[int]int),
	}
}

// ProcessEvent mirrors BufferedResponseAccumulator.ProcessEvent
// (responses_to_chatcompletions.go:449). Only the delta event types
// that contribute to output are handled; everything else is silently
// ignored.
func (a *codexBufferedResponseAccumulator) ProcessEvent(event codexResponsesStreamEvent) {
	switch event.Type {
	case "response.output_text.delta":
		if event.Delta != "" {
			_, _ = a.text.WriteString(event.Delta)
		}
	case "response.output_item.added":
		if event.Item != nil && (event.Item.Type == "function_call" || event.Item.Type == "custom_tool_call") {
			idx := len(a.funcCalls)
			a.outputIndexToFuncIdx[event.OutputIndex] = idx
			a.funcCalls = append(a.funcCalls, codexBufferedFuncCall{
				CallID: event.Item.CallID,
				Name:   event.Item.Name,
			})
		}
	case "response.function_call_arguments.delta", "response.custom_tool_call_input.delta":
		if event.Delta != "" {
			if idx, ok := a.outputIndexToFuncIdx[event.OutputIndex]; ok {
				_, _ = a.funcCalls[idx].Args.WriteString(event.Delta)
			}
		}
	case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
		if event.Delta != "" {
			_, _ = a.reasoning.WriteString(event.Delta)
		}
	}
}

func (a *codexBufferedResponseAccumulator) HasContent() bool {
	return a.text.Len() > 0 || len(a.funcCalls) > 0 || a.reasoning.Len() > 0
}

// BuildOutput mirrors BufferedResponseAccumulator.BuildOutput
// (responses_to_chatcompletions.go:485). Order matches sub2api exactly:
// reasoning → message → function_calls.
func (a *codexBufferedResponseAccumulator) BuildOutput() []map[string]any {
	var out []map[string]any
	if a.reasoning.Len() > 0 {
		out = append(out, map[string]any{
			"type": "reasoning",
			"summary": []map[string]any{
				{"type": "summary_text", "text": a.reasoning.String()},
			},
		})
	}
	if a.text.Len() > 0 {
		out = append(out, map[string]any{
			"type": "message",
			"role": "assistant",
			"content": []map[string]any{
				{"type": "output_text", "text": a.text.String()},
			},
		})
	}
	for i := range a.funcCalls {
		out = append(out, map[string]any{
			"type":      "function_call",
			"call_id":   a.funcCalls[i].CallID,
			"name":      a.funcCalls[i].Name,
			"arguments": a.funcCalls[i].Args.String(),
		})
	}
	return out
}

// codexResponsesStreamEventMayContributeToOutput mirrors sub2api
// responsesStreamEventMayContributeToOutput (openai_gateway_service.go
// :5375).
func codexResponsesStreamEventMayContributeToOutput(eventType string) bool {
	switch eventType {
	case "response.output_text.delta",
		"response.output_item.added",
		"response.function_call_arguments.delta",
		"response.custom_tool_call_input.delta",
		"response.reasoning_summary_text.delta",
		"response.reasoning_text.delta":
		return true
	}
	return false
}

// codexReconstructResponseOutputFromSSE mirrors sub2api
// reconstructResponseOutputFromSSE (openai_gateway_service.go:5390).
// Walks the SSE body, processes every relevant delta event, and
// returns the rebuilt output[] as JSON.
func codexReconstructResponseOutputFromSSE(body string) ([]byte, bool) {
	acc := newCodexBufferedResponseAccumulator()
	codexForEachSSEDataPayload(body, func(data []byte) {
		var typed struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &typed); err != nil {
			return
		}
		if !codexResponsesStreamEventMayContributeToOutput(typed.Type) {
			return
		}
		var event codexResponsesStreamEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return
		}
		acc.ProcessEvent(event)
	})
	if !acc.HasContent() {
		return nil, false
	}
	out := acc.BuildOutput()
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, false
	}
	return raw, true
}

// codexConvertCompactSSEBodyToJSON applies sub2api's
// handlePassthroughSSEToJSON (openai_gateway_service.go:4007) verbatim:
// extract the terminal response.completed event, reconstruct output[]
// from delta events when empty, return as JSON bytes. On any failure
// (no terminal event, malformed SSE), returns (nil, false) so the
// caller can choose to passthrough the raw SSE instead.
func codexConvertCompactSSEBodyToJSON(body []byte) ([]byte, bool) {
	if !codexBodyLooksLikeSSE(body) {
		return nil, false
	}
	bodyText := string(body)
	finalResponse, ok := codexExtractFinalResponse(bodyText)
	if !ok {
		return nil, false
	}
	// Reconstruct output[] if the terminal response.completed event
	// shipped an empty array (the compact path normally does — the
	// summary text lives in response.output_text.delta events).
	var inspect struct {
		Output []json.RawMessage `json:"output"`
	}
	if err := json.Unmarshal(finalResponse, &inspect); err == nil && len(inspect.Output) == 0 {
		if rebuilt, ok := codexReconstructResponseOutputFromSSE(bodyText); ok {
			patched, err := codexSetRawJSONField(finalResponse, "output", rebuilt)
			if err == nil {
				finalResponse = patched
			}
		}
	}
	return finalResponse, true
}

// codexSetRawJSONField sets a top-level JSON field to a raw JSON value
// (analogous to sjson.SetRawBytes from sub2api). Uses encoding/json
// since the project doesn't depend on tidwall/sjson.
func codexSetRawJSONField(body []byte, field string, rawValue []byte) ([]byte, error) {
	var current map[string]json.RawMessage
	if err := json.Unmarshal(body, &current); err != nil {
		return nil, err
	}
	current[field] = append(json.RawMessage(nil), rawValue...)
	return json.Marshal(current)
}
