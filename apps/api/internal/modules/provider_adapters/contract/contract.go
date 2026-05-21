package contract

import (
	"context"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
)

type TextRequest struct {
	RequestID        string
	SourceProtocol   string
	SourceEndpoint   string
	Model            string
	Prompt           string
	Messages         []TextMessage
	Instructions     string
	Stream           bool
	Temperature      *float32
	TopP             *float32
	MaxOutputTokens  *int
	Stop             []string
	Tools            []map[string]any
	ToolChoice       any
	ResponseFormat   map[string]any
	Provider         providercontract.Provider
	Account          accountcontract.ProviderAccount
	Mapping          modelcontract.ModelProviderMapping
	Credential       map[string]any
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
