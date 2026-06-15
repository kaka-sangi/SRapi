package service

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

func readCodexChatChunks(t *testing.T, body string) ([]map[string]any, bool, *codexChatStreamReader) {
	t.Helper()
	reader := newCodexChatStreamReader(io.NopCloser(strings.NewReader(body)), contract.ConversationRequest{RequestID: "test-req", Model: "gpt-5.5"})
	raw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read transformed stream: %v", err)
	}
	chunks := make([]map[string]any, 0)
	sawDone := false
	for _, block := range strings.Split(string(raw), "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		if !strings.HasPrefix(block, "data:") {
			t.Fatalf("unexpected non-data SSE block: %q", block)
		}
		payload := strings.TrimSpace(strings.TrimPrefix(block, "data:"))
		if payload == "[DONE]" {
			sawDone = true
			continue
		}
		var chunk map[string]any
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			t.Fatalf("decode chat chunk %q: %v", payload, err)
		}
		if chunk["object"] != "chat.completion.chunk" {
			t.Fatalf("chunk object = %v, want chat.completion.chunk", chunk["object"])
		}
		if chunk["id"] != "chatcmpl_test-req" {
			t.Fatalf("chunk id = %v, want chatcmpl_test-req", chunk["id"])
		}
		if chunk["model"] != "gpt-5.5" {
			t.Fatalf("chunk model = %v, want gpt-5.5", chunk["model"])
		}
		chunks = append(chunks, chunk)
	}
	return chunks, sawDone, reader
}

func chunkDelta(t *testing.T, chunk map[string]any) (map[string]any, any) {
	t.Helper()
	choices, ok := chunk["choices"].([]any)
	if !ok || len(choices) != 1 {
		t.Fatalf("chunk choices = %v, want exactly one", chunk["choices"])
	}
	choice, _ := choices[0].(map[string]any)
	delta, _ := choice["delta"].(map[string]any)
	return delta, choice["finish_reason"]
}

func TestCodexChatStreamReaderStreamsTextAndReasoning(t *testing.T) {
	body := strings.Join([]string{
		"event: response.created",
		`data: {"type":"response.created","response":{"id":"resp_1"}}`,
		"",
		"event: response.reasoning_text.delta",
		`data: {"type":"response.reasoning_text.delta","delta":"thinking"}`,
		"",
		"event: response.output_text.delta",
		`data: {"type":"response.output_text.delta","delta":"Hello"}`,
		"",
		"event: response.output_text.delta",
		`data: {"type":"response.output_text.delta","delta":" world"}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_1","status":"completed","usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}}`,
		"",
	}, "\n")

	chunks, sawDone, reader := readCodexChatChunks(t, body)
	if !sawDone {
		t.Fatal("expected a terminating data: [DONE]")
	}
	// role, reasoning, content, content, finish
	if len(chunks) != 5 {
		t.Fatalf("got %d chunks, want 5: %+v", len(chunks), chunks)
	}

	if delta, finish := chunkDelta(t, chunks[0]); delta["role"] != "assistant" || finish != nil {
		t.Fatalf("first chunk should be the assistant role chunk, got delta=%v finish=%v", delta, finish)
	}
	if delta, _ := chunkDelta(t, chunks[1]); delta["reasoning_content"] != "thinking" {
		t.Fatalf("reasoning chunk = %v, want reasoning_content=thinking", delta)
	}
	if delta, _ := chunkDelta(t, chunks[2]); delta["content"] != "Hello" {
		t.Fatalf("content chunk = %v, want content=Hello", delta)
	}
	if delta, _ := chunkDelta(t, chunks[3]); delta["content"] != " world" {
		t.Fatalf("content chunk = %v, want content=' world'", delta)
	}
	if delta, finish := chunkDelta(t, chunks[4]); finish != "stop" || len(delta) != 0 {
		t.Fatalf("final chunk = delta:%v finish:%v, want empty delta + stop", delta, finish)
	}

	// Usage/metadata must still be recoverable from the retained raw bytes via
	// the existing buffered parser — this is exactly what the gateway's
	// StreamParse hook meters. The reconstructed raw must re-parse cleanly.
	if _, err := parseCodexResponsesBody(reader.rawBytes(), 200); err != nil {
		t.Fatalf("parse retained raw codex bytes: %v", err)
	}
	if !strings.Contains(string(reader.rawBytes()), "response.completed") {
		t.Fatalf("retained raw bytes should include the terminal event: %q", reader.rawBytes())
	}
}

func TestCodexChatStreamReaderStreamsToolCalls(t *testing.T) {
	body := strings.Join([]string{
		"event: response.output_item.added",
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","call_id":"call_abc","name":"get_weather"}}`,
		"",
		"event: response.function_call_arguments.delta",
		`data: {"type":"response.function_call_arguments.delta","output_index":0,"delta":"{\"city\":"}`,
		"",
		"event: response.function_call_arguments.delta",
		`data: {"type":"response.function_call_arguments.delta","output_index":0,"delta":"\"SF\"}"}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_2","status":"completed"}}`,
		"",
	}, "\n")

	chunks, sawDone, _ := readCodexChatChunks(t, body)
	if !sawDone {
		t.Fatal("expected a terminating data: [DONE]")
	}
	// role, tool header, args delta, args delta, finish
	if len(chunks) != 5 {
		t.Fatalf("got %d chunks, want 5: %+v", len(chunks), chunks)
	}

	header, _ := chunkDelta(t, chunks[1])
	toolCalls, ok := header["tool_calls"].([]any)
	if !ok || len(toolCalls) != 1 {
		t.Fatalf("tool header delta = %v, want one tool_call", header)
	}
	call, _ := toolCalls[0].(map[string]any)
	if call["index"].(float64) != 0 || call["id"] != "call_abc" || call["type"] != "function" {
		t.Fatalf("tool header = %v, want index 0 / id call_abc / type function", call)
	}
	if fn, _ := call["function"].(map[string]any); fn["name"] != "get_weather" {
		t.Fatalf("tool header function = %v, want name get_weather", call["function"])
	}

	var args strings.Builder
	for _, idx := range []int{2, 3} {
		delta, _ := chunkDelta(t, chunks[idx])
		tc, _ := delta["tool_calls"].([]any)
		one, _ := tc[0].(map[string]any)
		fn, _ := one["function"].(map[string]any)
		args.WriteString(fn["arguments"].(string))
	}
	if args.String() != `{"city":"SF"}` {
		t.Fatalf("streamed tool args = %q, want %q", args.String(), `{"city":"SF"}`)
	}

	if _, finish := chunkDelta(t, chunks[4]); finish != "tool_calls" {
		t.Fatalf("final finish_reason = %v, want tool_calls", finish)
	}
}

func TestCodexChatStreamReaderEmptyResponseStillTerminates(t *testing.T) {
	body := strings.Join([]string{
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_3","status":"completed"}}`,
		"",
	}, "\n")

	chunks, sawDone, _ := readCodexChatChunks(t, body)
	if !sawDone {
		t.Fatal("expected a terminating data: [DONE]")
	}
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2 (role + finish): %+v", len(chunks), chunks)
	}
	if delta, _ := chunkDelta(t, chunks[0]); delta["role"] != "assistant" {
		t.Fatalf("first chunk should carry the assistant role, got %v", delta)
	}
	if _, finish := chunkDelta(t, chunks[1]); finish != "stop" {
		t.Fatalf("final finish_reason = %v, want stop", finish)
	}
}
