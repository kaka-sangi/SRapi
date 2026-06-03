package contract

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
)

// ErrStreamingUnsupported indicates the request is not eligible for
// same-protocol passthrough streaming (e.g. cross-protocol translation, a
// non-streaming request, or an adapter/runtime without a streaming transport).
// Callers fall back to the buffered InvokeConversation path.
var ErrStreamingUnsupported = errors.New("provider adapter: streaming passthrough unsupported")

type ConversationRequest struct {
	RequestID         string
	SourceProtocol    string
	SourceEndpoint    string
	TargetProtocol    string
	Model             string
	Messages          []ConversationMessage
	InputParts        []ContentPart
	System            []ContentPart
	Instructions      string
	Stream            bool
	Temperature       *float32
	TopP              *float32
	MaxOutputTokens   *int
	Stop              []string
	Tools             []map[string]any
	ToolChoice        any
	ResponseFormat    map[string]any
	Reasoning         map[string]any
	ContextManagement map[string]any
	RawBody           []byte
	Provider          providercontract.Provider
	Account           accountcontract.ProviderAccount
	Mapping           modelcontract.ModelProviderMapping
	Credential        map[string]any
	PayloadTransforms []PayloadTransform
}

// PayloadTransform is a single operator-configured mutation applied to the
// marshaled upstream request body just before dispatch. Action is "default"
// (set the path only when absent), "override" (always set), or "filter" (remove
// the path). Path is a dotted JSON path, e.g. "reasoning.effort" or
// "generationConfig.thinkingConfig.thinkingBudget".
type PayloadTransform struct {
	Action string
	Path   string
	Value  any
}

type TokenCountRequest struct {
	RequestID      string
	SourceProtocol string
	SourceEndpoint string
	Model          string
	RawBody        []byte
	Provider       providercontract.Provider
	Account        accountcontract.ProviderAccount
	Mapping        modelcontract.ModelProviderMapping
	Credential     map[string]any
}

type ResponseInputItemsRequest struct {
	RequestID      string
	SourceProtocol string
	SourceEndpoint string
	Model          string
	ResponseID     string
	Query          url.Values
	Provider       providercontract.Provider
	Account        accountcontract.ProviderAccount
	Mapping        modelcontract.ModelProviderMapping
	Credential     map[string]any
}

type ConversationMessage struct {
	Role  string
	Parts []ContentPart
}

type ContentPartKind string

const (
	ContentPartText       ContentPartKind = "text"
	ContentPartImage      ContentPartKind = "image"
	ContentPartAudio      ContentPartKind = "audio"
	ContentPartFile       ContentPartKind = "file"
	ContentPartToolUse    ContentPartKind = "tool_use"
	ContentPartToolResult ContentPartKind = "tool_result"
	ContentPartThinking   ContentPartKind = "thinking"
	ContentPartRefusal    ContentPartKind = "refusal"
	ContentPartMetadata   ContentPartKind = "metadata"
)

type ContentPart struct {
	Kind              ContentPartKind
	Text              string
	MediaURL          string
	MediaBase64       string
	MIMEType          string
	FileID            string
	ToolCallID        string
	ToolName          string
	ToolArgumentsJSON string
	ToolResultForID   string
	ToolResultIsError bool
	Metadata          map[string]any
	Raw               json.RawMessage
	OriginProtocol    string
}

type StopReason string

const (
	StopReasonEndTurn       StopReason = "end_turn"
	StopReasonMaxTokens     StopReason = "max_tokens"
	StopReasonToolUse       StopReason = "tool_use"
	StopReasonContentFilter StopReason = "content_filter"
	StopReasonRefusal       StopReason = "refusal"
)

type Usage struct {
	InputTokens  int
	OutputTokens int
	// CachedTokens is cache-READ tokens (a prompt-cache hit), billed at the
	// cache-read rate. For OpenAI/Gemini this is the only cache class; for
	// Anthropic it is reported separately from input_tokens.
	CachedTokens int
	// CacheCreationTokens is cache-WRITE tokens (prompt cache being populated),
	// billed at the cache-write rate (which exceeds the input rate). Currently
	// populated for Anthropic; zero for providers without cache writes.
	CacheCreationTokens int
	Estimated           bool
}

type EmbeddingRequest struct {
	RequestID      string
	SourceProtocol string
	SourceEndpoint string
	Model          string
	Input          []string
	EncodingFormat string
	Dimensions     *int
	User           string
	Provider       providercontract.Provider
	Account        accountcontract.ProviderAccount
	Mapping        modelcontract.ModelProviderMapping
	Credential     map[string]any
}

type ImageGenerationRequest struct {
	RequestID      string
	SourceProtocol string
	SourceEndpoint string
	Model          string
	Prompt         string
	Count          int
	Size           string
	Quality        string
	Style          string
	ResponseFormat string
	User           string
	Extra          map[string]any
	Provider       providercontract.Provider
	Account        accountcontract.ProviderAccount
	Mapping        modelcontract.ModelProviderMapping
	Credential     map[string]any
}

type ImageInput struct {
	FileName    string
	ContentType string
	Bytes       []byte
}

type ImageEditRequest struct {
	RequestID      string
	SourceProtocol string
	SourceEndpoint string
	Model          string
	Prompt         string
	Images         []ImageInput
	Mask           *ImageInput
	Count          int
	Size           string
	Quality        string
	ResponseFormat string
	User           string
	Extra          map[string]any
	Provider       providercontract.Provider
	Account        accountcontract.ProviderAccount
	Mapping        modelcontract.ModelProviderMapping
	Credential     map[string]any
}

type ImageVariationRequest struct {
	RequestID      string
	SourceProtocol string
	SourceEndpoint string
	Model          string
	Image          ImageInput
	Count          int
	Size           string
	ResponseFormat string
	User           string
	Extra          map[string]any
	Provider       providercontract.Provider
	Account        accountcontract.ProviderAccount
	Mapping        modelcontract.ModelProviderMapping
	Credential     map[string]any
}

type AudioTranscriptionRequest struct {
	RequestID      string
	SourceProtocol string
	SourceEndpoint string
	Model          string
	FileName       string
	ContentType    string
	Audio          []byte
	Language       string
	Prompt         string
	ResponseFormat string
	Temperature    *float32
	User           string
	Extra          map[string]any
	Provider       providercontract.Provider
	Account        accountcontract.ProviderAccount
	Mapping        modelcontract.ModelProviderMapping
	Credential     map[string]any
}

type AudioSpeechRequest struct {
	RequestID      string
	SourceProtocol string
	SourceEndpoint string
	Model          string
	Input          string
	Voice          string
	ResponseFormat string
	Speed          *float32
	Instructions   string
	User           string
	Extra          map[string]any
	Provider       providercontract.Provider
	Account        accountcontract.ProviderAccount
	Mapping        modelcontract.ModelProviderMapping
	Credential     map[string]any
}

type ModerationRequest struct {
	RequestID      string
	SourceProtocol string
	SourceEndpoint string
	Model          string
	Input          []string
	User           string
	Provider       providercontract.Provider
	Account        accountcontract.ProviderAccount
	Mapping        modelcontract.ModelProviderMapping
	Credential     map[string]any
}

type RerankDocument struct {
	Text     string
	Fields   map[string]any
	Original any
}

type RerankRequest struct {
	RequestID       string
	SourceProtocol  string
	SourceEndpoint  string
	Model           string
	Query           string
	Documents       []RerankDocument
	TopN            *int
	ReturnDocuments bool
	User            string
	Provider        providercontract.Provider
	Account         accountcontract.ProviderAccount
	Mapping         modelcontract.ModelProviderMapping
	Credential      map[string]any
}

type Embedding struct {
	Index        int
	Vector       []float32
	Base64Vector string
}

type EmbeddingResponse struct {
	Data       []Embedding
	Model      string
	StatusCode int
	Usage      Usage
}

type Image struct {
	URL           string
	Base64JSON    string
	RevisedPrompt string
	Metadata      map[string]any
}

type ImageGenerationResponse struct {
	Created    int64
	Data       []Image
	Model      string
	StatusCode int
	Usage      Usage
}

type ModerationResult struct {
	Flagged                   bool
	Categories                map[string]bool
	CategoryScores            map[string]float32
	CategoryAppliedInputTypes map[string][]string
}

type ModerationResponse struct {
	ID         string
	Results    []ModerationResult
	Model      string
	StatusCode int
	Usage      Usage
}

type AudioTranscriptionSegment struct {
	ID               *int
	Seek             *int
	Start            *float32
	End              *float32
	Text             string
	Tokens           []int
	Temperature      *float32
	AvgLogprob       *float32
	CompressionRatio *float32
	NoSpeechProb     *float32
	Metadata         map[string]any
}

type AudioTranscriptionResponse struct {
	ID         string
	Text       string
	Task       string
	Language   string
	Duration   *float32
	Segments   []AudioTranscriptionSegment
	Model      string
	StatusCode int
	Usage      Usage
}

type AudioSpeechResponse struct {
	ID          string
	Audio       []byte
	ContentType string
	Model       string
	StatusCode  int
	Usage       Usage
}

type RerankResult struct {
	Index          int
	RelevanceScore float32
	Document       *RerankDocument
	Metadata       map[string]any
}

type RerankResponse struct {
	ID         string
	Results    []RerankResult
	Model      string
	StatusCode int
	Usage      Usage
}

type TokenCountResponse struct {
	TotalTokens             int
	CachedContentTokenCount *int
	PromptTokensDetails     []ModalityTokenCount
	CacheTokensDetails      []ModalityTokenCount
	StatusCode              int
	Metadata                map[string]any
}

// QuotaSignal carries sanitized provider quota observations from upstream response headers.
type QuotaSignal struct {
	QuotaType      string
	Remaining      string
	Used           string
	QuotaLimit     string
	RemainingRatio float32
	ResetAt        *time.Time
	SnapshotAt     time.Time
}

// QuotaReport is a normalized, per-provider view of an account's subscription
// and credit standing, produced by an active out-of-band quota/subscription
// fetch. Supported is false when the provider exposes no quota endpoint.
type QuotaReport struct {
	Provider         string
	Supported        bool
	Source           string // "endpoint" | "headers" | "none"
	Plan             string
	CreditsRemaining string
	CreditsUsed      string
	CreditsLimit     string
	Currency         string
	QuotaSignals     []QuotaSignal
	StatusCode       int
	FetchedAt        time.Time
}

// AccountQuotaFetcher performs an active per-account quota/subscription fetch.
type AccountQuotaFetcher interface {
	FetchAccountQuota(ctx context.Context, req ProbeRequest) (QuotaReport, error)
	// QuotaConfigured reports whether the account/provider exposes a quota
	// endpoint, using only provider config + account metadata (no credential),
	// so callers can skip credential decryption for accounts without quota.
	QuotaConfigured(req ProbeRequest) bool
}

type ResponseInputItemsResponse struct {
	Raw          []byte
	StatusCode   int
	QuotaSignals []QuotaSignal
}

type ModalityTokenCount struct {
	Modality   string
	TokenCount int
	Metadata   map[string]any
}

// ConversationStreamEvent captures provider stream semantics after protocol parsing.
type ConversationStreamEvent struct {
	Index          int
	Type           ConversationStreamEventType
	ContentIndex   int
	Delta          ContentPart
	Usage          Usage
	StopReason     StopReason
	RawEventType   string
	Raw            json.RawMessage
	OriginProtocol string
	Metadata       map[string]any
}

// ConversationStreamEventType identifies the canonical meaning of a provider stream event.
type ConversationStreamEventType string

const (
	ConversationStreamEventContentDelta  ConversationStreamEventType = "content_delta"
	ConversationStreamEventToolCallDelta ConversationStreamEventType = "tool_call_delta"
	ConversationStreamEventToolResult    ConversationStreamEventType = "tool_result_delta"
	ConversationStreamEventReasoning     ConversationStreamEventType = "reasoning_delta"
	ConversationStreamEventMetadata      ConversationStreamEventType = "metadata"
	ConversationStreamEventUsage         ConversationStreamEventType = "usage"
	ConversationStreamEventStop          ConversationStreamEventType = "stop"
)

type ConversationResponse struct {
	ID           string
	Parts        []ContentPart
	StopReason   StopReason
	StatusCode   int
	Usage        Usage
	Raw          json.RawMessage
	Warnings     []string
	StreamEvents []ConversationStreamEvent
	QuotaSignals []QuotaSignal

	// StreamBody, when non-nil, carries the live upstream response body for
	// same-protocol passthrough streaming. The caller MUST Close it after
	// streaming to the client (Close also releases the request's concurrency
	// lease). When set, Parts/Raw/Usage are not yet populated; the caller copies
	// the body to the client incrementally and then calls StreamParse on the
	// fully-streamed bytes to recover usage for metering.
	StreamBody io.ReadCloser
	// StreamParse extracts the canonical response (usage, raw, quota) from the
	// fully-streamed body using the same parser as the buffered path. Only set
	// when StreamBody is non-nil.
	StreamParse func(body []byte, statusCode int) (ConversationResponse, error)
}

// ProbeRequest contains the provider, account, and credential used for a health probe.
type ProbeRequest struct {
	Provider   providercontract.Provider
	Account    accountcontract.ProviderAccount
	Credential map[string]any
}

// ProbeResponse captures a provider adapter health probe outcome.
type ProbeResponse struct {
	OK         bool
	ErrorClass string
	StatusCode int
	LatencyMS  int
	Metadata   map[string]any
}

type RealtimeRequest struct {
	RequestID      string
	SourceProtocol string
	SourceEndpoint string
	Model          string
	RequestPayload []byte
	Headers        http.Header
	Provider       providercontract.Provider
	Account        accountcontract.ProviderAccount
	Mapping        modelcontract.ModelProviderMapping
	Credential     map[string]any
}

type RealtimeSession struct {
	URL          string
	Headers      http.Header
	InitialFrame []byte
}

type ProviderError struct {
	Class      string
	StatusCode int
	Message    string
	RetryAfter *time.Time
}

func (e ProviderError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Class != "" {
		return e.Class
	}
	return "provider adapter error"
}

// ConversationAdapter invokes a provider-backed text or multimodal conversation request.
type ConversationAdapter interface {
	InvokeConversation(ctx context.Context, req ConversationRequest) (ConversationResponse, error)
}

// ProbeAdapter checks whether a provider account can reach its upstream.
type ProbeAdapter interface {
	ProbeAccount(ctx context.Context, req ProbeRequest) (ProbeResponse, error)
}

// RealtimeAdapter prepares a provider realtime session for WebSocket relay.
type RealtimeAdapter interface {
	PrepareRealtime(ctx context.Context, req RealtimeRequest) (RealtimeSession, error)
}

// EmbeddingAdapter invokes provider embedding generation.
type EmbeddingAdapter interface {
	InvokeEmbeddings(ctx context.Context, req EmbeddingRequest) (EmbeddingResponse, error)
}

// ImageGenerationAdapter invokes provider image generation.
type ImageGenerationAdapter interface {
	InvokeImageGeneration(ctx context.Context, req ImageGenerationRequest) (ImageGenerationResponse, error)
}

// ImageEditAdapter invokes provider image editing.
type ImageEditAdapter interface {
	InvokeImageEdit(ctx context.Context, req ImageEditRequest) (ImageGenerationResponse, error)
}

// ImageVariationAdapter invokes provider image variation generation.
type ImageVariationAdapter interface {
	InvokeImageVariation(ctx context.Context, req ImageVariationRequest) (ImageGenerationResponse, error)
}

// AudioTranscriptionAdapter invokes provider audio transcription.
type AudioTranscriptionAdapter interface {
	InvokeAudioTranscription(ctx context.Context, req AudioTranscriptionRequest) (AudioTranscriptionResponse, error)
}

// AudioSpeechAdapter invokes provider text-to-speech generation.
type AudioSpeechAdapter interface {
	InvokeAudioSpeech(ctx context.Context, req AudioSpeechRequest) (AudioSpeechResponse, error)
}

// ModerationAdapter invokes provider moderation checks.
type ModerationAdapter interface {
	InvokeModerations(ctx context.Context, req ModerationRequest) (ModerationResponse, error)
}

// RerankAdapter invokes provider reranking.
type RerankAdapter interface {
	InvokeRerank(ctx context.Context, req RerankRequest) (RerankResponse, error)
}

// ResponseInputItemsAdapter fetches provider response input items.
type ResponseInputItemsAdapter interface {
	InvokeResponseInputItems(ctx context.Context, req ResponseInputItemsRequest) (ResponseInputItemsResponse, error)
}
