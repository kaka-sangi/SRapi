package httpserver

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"

	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	gatewayservice "github.com/srapi/srapi/apps/api/internal/modules/gateway/service"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
)

// crossProtocolStreamRawCap bounds the retained raw upstream bytes used for the
// StreamParse usage/billing pass (mirrors the codex chat reader's cap).
const crossProtocolStreamRawCap = 16 << 20

// crossProtocolStreamReader transcodes a live upstream SSE stream into the
// client's protocol on the fly: it scans one upstream frame, drives the upstream
// per-frame parser to canonical provider events, lifts them to gateway stream
// events, and feeds them to a stateful client renderer that serializes the
// client's SSE. The raw upstream bytes are retained (capped) so the unchanged
// upstream StreamParse can recover usage for metering.
type crossProtocolStreamReader struct {
	upstream   io.ReadCloser
	scanner    *bufio.Scanner
	parser     provideradaptercontract.UpstreamStreamParser
	transcoder clientStreamTranscoder

	out       bytes.Buffer
	raw       bytes.Buffer
	rawCapped bool
	finalized bool
	done      bool
}

func newCrossProtocolStreamReader(upstream io.ReadCloser, parser provideradaptercontract.UpstreamStreamParser, transcoder clientStreamTranscoder) *crossProtocolStreamReader {
	scanner := bufio.NewScanner(upstream)
	scanner.Buffer(make([]byte, 0, 64*1024), 4<<20)
	return &crossProtocolStreamReader{upstream: upstream, scanner: scanner, parser: parser, transcoder: transcoder}
}

func (r *crossProtocolStreamReader) Read(p []byte) (int, error) {
	for r.out.Len() == 0 && !r.done {
		r.pump()
	}
	if r.out.Len() > 0 {
		return r.out.Read(p)
	}
	return 0, io.EOF
}

func (r *crossProtocolStreamReader) Close() error {
	if r.upstream != nil {
		return r.upstream.Close()
	}
	return nil
}

// rawBytes returns the retained raw upstream SSE for the metering parse.
func (r *crossProtocolStreamReader) rawBytes() []byte { return r.raw.Bytes() }

func (r *crossProtocolStreamReader) accumRaw(line string) {
	if r.rawCapped {
		return
	}
	if r.raw.Len()+len(line)+1 > crossProtocolStreamRawCap {
		r.rawCapped = true
		return
	}
	r.raw.WriteString(line)
	r.raw.WriteByte('\n')
}

// pump scans one upstream SSE frame and appends the transcoded client SSE it maps
// to. On upstream EOF it finalizes the client stream.
func (r *crossProtocolStreamReader) pump() {
	var eventType string
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
			eventType = value
		case "data":
			dataLines = append(dataLines, value)
		}
	}
	if !gotFrame {
		r.finish()
		return
	}
	data := strings.TrimSpace(strings.Join(dataLines, "\n"))
	if data == "" {
		return
	}
	events, done, err := r.parser.FeedFrame(eventType, data)
	if err != nil {
		// A mid-stream upstream error frame: stop transcoding and finalize the
		// client stream rather than leaking a broken frame.
		r.finish()
		return
	}
	if len(events) > 0 {
		r.out.Write(r.transcoder.feed(gatewayStreamEventsFromProvider(events)))
	}
	if done {
		r.finish()
	}
}

func (r *crossProtocolStreamReader) finish() {
	if r.finalized {
		r.done = true
		return
	}
	r.finalized = true
	if tail := r.parser.Finalize(); len(tail) > 0 {
		r.out.Write(r.transcoder.feed(gatewayStreamEventsFromProvider(tail)))
	}
	r.out.Write(r.transcoder.finalize())
	r.done = true
}

// clientStreamTranscoder feeds canonical gateway stream events to a stateful
// client renderer and serializes the resulting client SSE bytes.
type clientStreamTranscoder interface {
	feed(events []gatewaycontract.StreamEvent) []byte
	finalize() []byte
}

// chatTranscoder serializes Chat Completions chunks ("data: {json}\n\n",
// terminated by data: [DONE]).
type chatTranscoder struct{ renderer *gatewayservice.ChatStreamRenderer }

func (t *chatTranscoder) feed(events []gatewaycontract.StreamEvent) []byte {
	var b bytes.Buffer
	for _, event := range events {
		for _, chunk := range t.renderer.FeedEvent(event) {
			writeChatChunkSSE(&b, chunk)
		}
	}
	return b.Bytes()
}

func (t *chatTranscoder) finalize() []byte {
	var b bytes.Buffer
	for _, chunk := range t.renderer.Finalize() {
		writeChatChunkSSE(&b, chunk)
	}
	b.WriteString("data: [DONE]\n\n")
	return b.Bytes()
}

func writeChatChunkSSE(b *bytes.Buffer, chunk map[string]any) {
	raw, err := json.Marshal(chunk)
	if err != nil {
		return
	}
	b.WriteString("data: ")
	b.Write(raw)
	b.WriteString("\n\n")
}

// anthropicTranscoder serializes Anthropic Messages named events
// ("event: X\ndata: {json}\n\n"); there is no [DONE] sentinel.
type anthropicTranscoder struct{ renderer *gatewayservice.AnthropicStreamRenderer }

func (t *anthropicTranscoder) feed(events []gatewaycontract.StreamEvent) []byte {
	var b bytes.Buffer
	for _, event := range events {
		for _, out := range t.renderer.FeedEvent(event) {
			writeAnthropicEventSSE(&b, out)
		}
	}
	return b.Bytes()
}

func (t *anthropicTranscoder) finalize() []byte {
	var b bytes.Buffer
	for _, out := range t.renderer.Finalize() {
		writeAnthropicEventSSE(&b, out)
	}
	return b.Bytes()
}

func writeAnthropicEventSSE(b *bytes.Buffer, event gatewayservice.StreamEvent) {
	if name := strings.TrimSpace(event.Event); name != "" {
		b.WriteString("event: ")
		b.WriteString(name)
		b.WriteByte('\n')
	}
	raw, err := json.Marshal(event.Data)
	if err != nil {
		return
	}
	b.WriteString("data: ")
	b.Write(raw)
	b.WriteString("\n\n")
}

// geminiTranscoder serializes Gemini streamGenerateContent chunks
// ("data: {json}\n\n"); there is no event name or [DONE] sentinel.
type geminiTranscoder struct{ renderer *gatewayservice.GeminiStreamRenderer }

func (t *geminiTranscoder) feed(events []gatewaycontract.StreamEvent) []byte {
	var b bytes.Buffer
	for _, event := range events {
		for _, out := range t.renderer.FeedEvent(event) {
			writeGeminiChunkSSE(&b, out)
		}
	}
	return b.Bytes()
}

func (t *geminiTranscoder) finalize() []byte {
	var b bytes.Buffer
	for _, out := range t.renderer.Finalize() {
		writeGeminiChunkSSE(&b, out)
	}
	// Match SRapi's existing gemini stream path, which terminates with the
	// [DONE] sentinel (the buffered writeSSEEvents path appends it).
	b.WriteString("data: [DONE]\n\n")
	return b.Bytes()
}

func writeGeminiChunkSSE(b *bytes.Buffer, event gatewayservice.StreamEvent) {
	raw, err := json.Marshal(event.Data)
	if err != nil {
		return
	}
	b.WriteString("data: ")
	b.Write(raw)
	b.WriteString("\n\n")
}

// responsesTranscoder serializes OpenAI Responses named events
// ("event: X\ndata: {json}\n\n"), terminated by [DONE] to match SRapi's existing
// /v1/responses stream path.
type responsesTranscoder struct{ renderer *gatewayservice.ResponsesStreamRenderer }

func (t *responsesTranscoder) feed(events []gatewaycontract.StreamEvent) []byte {
	var b bytes.Buffer
	for _, event := range events {
		for _, out := range t.renderer.FeedEvent(event) {
			writeAnthropicEventSSE(&b, out)
		}
	}
	return b.Bytes()
}

func (t *responsesTranscoder) finalize() []byte {
	var b bytes.Buffer
	for _, out := range t.renderer.Finalize() {
		writeAnthropicEventSSE(&b, out)
	}
	b.WriteString("data: [DONE]\n\n")
	return b.Bytes()
}

// newClientStreamTranscoder selects the client-protocol transcoder for a
// cross-protocol stream. seed carries the chunk id/model.
func newClientStreamTranscoder(gateway *gatewayservice.Service, req provideradaptercontract.ConversationRequest) (clientStreamTranscoder, bool) {
	seed := gatewaycontract.CanonicalResponse{ID: req.RequestID, Model: req.Model}
	switch strings.ToLower(strings.TrimSpace(req.SourceProtocol)) {
	case "anthropic-compatible", "anthropic":
		return &anthropicTranscoder{renderer: gateway.NewAnthropicStreamRenderer(seed)}, true
	case "gemini-compatible", "gemini":
		return &geminiTranscoder{renderer: gateway.NewGeminiStreamRenderer(seed)}, true
	case "openai-compatible", "openai":
		if strings.Contains(strings.ToLower(req.SourceEndpoint), "/responses") {
			return &responsesTranscoder{renderer: gateway.NewResponsesStreamRenderer(seed)}, true
		}
		return &chatTranscoder{renderer: gateway.NewChatStreamRenderer(seed)}, true
	default:
		return nil, false
	}
}
