package contract

type Protocol string

const (
	ProtocolOpenAICompatible    Protocol = "openai-compatible"
	ProtocolAnthropicCompatible Protocol = "anthropic-compatible"
)

type SourceEndpoint string

const (
	EndpointChatCompletions SourceEndpoint = "/v1/chat/completions"
	EndpointResponses       SourceEndpoint = "/v1/responses"
	EndpointMessages        SourceEndpoint = "/v1/messages"
)

type ContentBlockType string

const (
	ContentBlockText     ContentBlockType = "text"
	ContentBlockImage    ContentBlockType = "image"
	ContentBlockToolCall ContentBlockType = "tool_call"
	ContentBlockMetadata ContentBlockType = "metadata"
)

type ContentBlock struct {
	Type     ContentBlockType
	Role     string
	Text     string
	MediaURL string
	Metadata map[string]any
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
	Prompt                string
	CompatibilityWarnings []string
	RequestCapabilities   []RequestCapability
}

type Usage struct {
	InputTokens  int
	OutputTokens int
	CachedTokens int
	Estimated    bool
}

type CanonicalResponse struct {
	ID                    string
	RequestID             string
	Model                 string
	CanonicalModel        string
	Message               string
	OutputItems           []ContentBlock
	Usage                 Usage
	CompatibilityWarnings []string
}
