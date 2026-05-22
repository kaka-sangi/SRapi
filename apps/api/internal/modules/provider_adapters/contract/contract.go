package contract

import (
	"context"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
)

type TextRequest struct {
	RequestID       string
	SourceProtocol  string
	SourceEndpoint  string
	Model           string
	Prompt          string
	Messages        []TextMessage
	Instructions    string
	Stream          bool
	Temperature     *float32
	TopP            *float32
	MaxOutputTokens *int
	Stop            []string
	Tools           []map[string]any
	ToolChoice      any
	ResponseFormat  map[string]any
	Provider        providercontract.Provider
	Account         accountcontract.ProviderAccount
	Mapping         modelcontract.ModelProviderMapping
	Credential      map[string]any
}

type TextMessage struct {
	Role    string
	Content string
}

type Usage struct {
	InputTokens  int
	OutputTokens int
	CachedTokens int
	Estimated    bool
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

type TextResponse struct {
	Text       string
	StatusCode int
	Usage      Usage
}

type ProviderError struct {
	Class      string
	StatusCode int
	Message    string
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

type TextAdapter interface {
	InvokeText(ctx context.Context, req TextRequest) (TextResponse, error)
}

type EmbeddingAdapter interface {
	InvokeEmbeddings(ctx context.Context, req EmbeddingRequest) (EmbeddingResponse, error)
}

type ImageGenerationAdapter interface {
	InvokeImageGeneration(ctx context.Context, req ImageGenerationRequest) (ImageGenerationResponse, error)
}

type ModerationAdapter interface {
	InvokeModerations(ctx context.Context, req ModerationRequest) (ModerationResponse, error)
}
