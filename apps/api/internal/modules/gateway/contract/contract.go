package contract

import "encoding/json"

type Protocol string

const (
	ProtocolOpenAICompatible    Protocol = "openai-compatible"
	ProtocolAnthropicCompatible Protocol = "anthropic-compatible"
	ProtocolGeminiCompatible    Protocol = "gemini-compatible"
)

type SourceEndpoint string

const (
	EndpointChatCompletions       SourceEndpoint = "/v1/chat/completions"
	EndpointResponses             SourceEndpoint = "/v1/responses"
	EndpointResponseInputItems    SourceEndpoint = "/v1/responses/{response_id}/input_items"
	EndpointResponsesCompact      SourceEndpoint = "/v1/responses/compact"
	EndpointMessages              SourceEndpoint = "/v1/messages"
	EndpointEmbeddings            SourceEndpoint = "/v1/embeddings"
	EndpointImagesGenerations     SourceEndpoint = "/v1/images/generations"
	EndpointImagesEdits           SourceEndpoint = "/v1/images/edits"
	EndpointImagesVariations      SourceEndpoint = "/v1/images/variations"
	EndpointAudioTranscriptions   SourceEndpoint = "/v1/audio/transcriptions"
	EndpointAudioSpeech           SourceEndpoint = "/v1/audio/speech"
	EndpointModerations           SourceEndpoint = "/v1/moderations"
	EndpointRerank                SourceEndpoint = "/v1/rerank"
	EndpointRealtime              SourceEndpoint = "/v1/realtime"
	EndpointGeminiGenerateContent SourceEndpoint = "/v1beta/models/{model}:generateContent"
	EndpointGeminiStreamContent   SourceEndpoint = "/v1beta/models/{model}:streamGenerateContent"
	EndpointGeminiCountTokens     SourceEndpoint = "/v1beta/models/{model}:countTokens"
)

type ContentBlockType string

const (
	ContentBlockText       ContentBlockType = "text"
	ContentBlockImage      ContentBlockType = "image"
	ContentBlockAudio      ContentBlockType = "audio"
	ContentBlockFile       ContentBlockType = "file"
	ContentBlockToolCall   ContentBlockType = "tool_call"
	ContentBlockToolResult ContentBlockType = "tool_result"
	ContentBlockReasoning  ContentBlockType = "reasoning"
	ContentBlockRefusal    ContentBlockType = "refusal"
	ContentBlockMetadata   ContentBlockType = "metadata"
)

type ContentBlock struct {
	Type              ContentBlockType
	Role              string
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

type Message struct {
	Role    string
	Content []ContentBlock
}

type RequestCapability struct {
	Key     string
	Version string
}

type CanonicalRequest struct {
	RequestID             string
	SourceProtocol        Protocol
	SourceEndpoint        string
	ResponseProtocol      Protocol
	UserID                int
	APIKeyID              int
	Model                 string
	CanonicalModel        string
	InputItems            []ContentBlock
	Messages              []Message
	Instructions          string
	Stream                bool
	Temperature           *float32
	TopP                  *float32
	MaxOutputTokens       *int
	Stop                  []string
	Tools                 []map[string]any
	ToolChoice            any
	ResponseFormat        map[string]any
	Reasoning             map[string]any
	ContextManagement     map[string]any
	RawBody               []byte
	EmbeddingInput        []string
	EmbeddingEncoding     string
	EmbeddingDimensions   *int
	EmbeddingUser         string
	ImagePrompt           string
	ImageInputs           []ImageInput
	ImageMask             *ImageInput
	ImageCount            int
	ImageSize             string
	ImageQuality          string
	ImageStyle            string
	ImageResponseFormat   string
	ImageStream           bool
	ImageUser             string
	ImageExtra            map[string]any
	AudioFileName         string
	AudioContentType      string
	AudioBytes            []byte
	AudioLanguage         string
	AudioPrompt           string
	AudioResponseFormat   string
	AudioTemperature      *float32
	AudioUser             string
	AudioExtra            map[string]any
	SpeechInput           string
	SpeechVoice           string
	SpeechResponseFormat  string
	SpeechSpeed           *float32
	SpeechInstructions    string
	SpeechUser            string
	SpeechExtra           map[string]any
	ModerationInput       []string
	ModerationUser        string
	RerankQuery           string
	RerankDocuments       []RerankDocument
	RerankTopN            *int
	RerankReturnDocuments bool
	RerankUser            string
	Prompt                string
	CompatibilityWarnings []string
	RequestCapabilities   []RequestCapability
}

type ImageInput struct {
	FileName    string
	ContentType string
	Bytes       []byte
}

type Usage struct {
	InputTokens           int
	OutputTokens          int
	ImageOutputTokens     int
	CachedTokens          int // cache-read tokens
	CacheCreationTokens   int // cache-write tokens (billed at the cache-write rate)
	CacheCreation5mTokens int
	CacheCreation1hTokens int
	Estimated             bool
}

// StreamEvent captures canonical stream deltas so protocol renderers do not
// need to synthesize every stream from the final aggregated response.
type StreamEvent struct {
	Index          int
	Type           StreamEventType
	ContentIndex   int
	Delta          ContentBlock
	Usage          Usage
	StopReason     string
	RawEventType   string
	Raw            json.RawMessage
	OriginProtocol string
	Metadata       map[string]any
}

// StreamEventType identifies the canonical meaning of a stream event.
type StreamEventType string

const (
	StreamEventContentDelta  StreamEventType = "content_delta"
	StreamEventToolCallDelta StreamEventType = "tool_call_delta"
	StreamEventToolResult    StreamEventType = "tool_result_delta"
	StreamEventReasoning     StreamEventType = "reasoning_delta"
	StreamEventMetadata      StreamEventType = "metadata"
	StreamEventUsage         StreamEventType = "usage"
	StreamEventStop          StreamEventType = "stop"
)

type CanonicalResponse struct {
	ID                    string
	RequestID             string
	Model                 string
	CanonicalModel        string
	Message               string
	OutputItems           []ContentBlock
	StreamEvents          []StreamEvent
	StopReason            string
	Usage                 Usage
	RawProviderMetadata   []byte
	CompatibilityWarnings []string
}

type Embedding struct {
	Index        int
	Vector       []float32
	Base64Vector string
}

type EmbeddingResponse struct {
	ID                    string
	RequestID             string
	Model                 string
	CanonicalModel        string
	Data                  []Embedding
	Usage                 Usage
	CompatibilityWarnings []string
}

type Image struct {
	URL           string
	Base64JSON    string
	RevisedPrompt string
	Metadata      map[string]any
}

type ImageGenerationResponse struct {
	ID                    string
	RequestID             string
	Model                 string
	CanonicalModel        string
	Created               int64
	Data                  []Image
	Usage                 Usage
	CompatibilityWarnings []string
}

type ModerationResult struct {
	Flagged                   bool
	Categories                map[string]bool
	CategoryScores            map[string]float32
	CategoryAppliedInputTypes map[string][]string
}

type ModerationResponse struct {
	ID                    string
	RequestID             string
	Model                 string
	CanonicalModel        string
	Results               []ModerationResult
	Usage                 Usage
	CompatibilityWarnings []string
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
	ID                    string
	RequestID             string
	Model                 string
	CanonicalModel        string
	Text                  string
	Task                  string
	Language              string
	Duration              *float32
	Segments              []AudioTranscriptionSegment
	Usage                 Usage
	CompatibilityWarnings []string
}

type AudioSpeechResponse struct {
	ID                    string
	RequestID             string
	Model                 string
	CanonicalModel        string
	ContentType           string
	Audio                 []byte
	Usage                 Usage
	CompatibilityWarnings []string
}

type RerankDocument struct {
	Text     string
	Fields   map[string]any
	Original any
}

type RerankResult struct {
	Index          int
	RelevanceScore float32
	Document       *RerankDocument
	Metadata       map[string]any
}

type RerankResponse struct {
	ID                    string
	RequestID             string
	Model                 string
	CanonicalModel        string
	Results               []RerankResult
	Usage                 Usage
	CompatibilityWarnings []string
}

type ModalityTokenCount struct {
	Modality   string
	TokenCount int
	Metadata   map[string]any
}

type TokenCountResponse struct {
	RequestID               string
	Model                   string
	CanonicalModel          string
	TotalTokens             int
	CachedContentTokenCount *int
	PromptTokensDetails     []ModalityTokenCount
	CacheTokensDetails      []ModalityTokenCount
	Metadata                map[string]any
	CompatibilityWarnings   []string
}
