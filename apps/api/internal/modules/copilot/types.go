// Package copilot implements the admin AI copilot: an agentic tool-calling loop
// whose tools are the admin OpenAPI surface. The model proposes admin API calls;
// reads run automatically while writes are gated behind explicit admin approval.
// Execution is delegated to an injected dispatcher (the real admin handlers run
// in-process), so auth, validation, RBAC, and audit all apply unchanged and the
// copilot can never exceed the calling admin's own privileges.
package copilot

// Settings is the resolved copilot configuration for a turn.
type Settings struct {
	Enabled           bool
	Source            string // "account" | "dedicated"
	ProviderAccountID      int
	ProviderAccountGroupID int
	Model                  string
	Models            []string
	DedicatedProtocol string
	DedicatedBaseURL  string
	OwnerOnly         bool
	AutoRunReads      bool
	MaxOutputTokens   int

	// SystemSummary is an optional runtime snapshot (account counts, health, etc.)
	// injected into the system prompt so the model starts with situational awareness.
	SystemSummary string

	// Web search (optional): when enabled + a key is configured, the engine gains
	// a web_search tool. WebSearchAPIKeyCiphertext is decrypted by the handler.
	WebSearchEnabled          bool
	WebSearchProvider         string // "tavily" | "brave"
	WebSearchBaseURL          string
	WebSearchAPIKeyCiphertext string
}

// Role identifies the author of a conversation message.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// Message is one entry in the round-tripped conversation history. The frontend
// owns the history and resends it each turn; the backend holds no state.
type Message struct {
	Role        string         `json:"role"`
	Content     string         `json:"content,omitempty"`
	Reasoning   string         `json:"reasoning,omitempty"`
	Images      []MessageImage `json:"images,omitempty"`
	ToolCalls   []ToolCall     `json:"tool_calls,omitempty"`
	ToolResults []ToolResult   `json:"tool_results,omitempty"`
}

// MessageImage is a base64 image attachment on a user message.
type MessageImage struct {
	MIMEType string `json:"mime_type"`
	Data     string `json:"data"`
}

// ToolCall is a tool invocation requested by the model.
type ToolCall struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ArgumentsJSON string `json:"arguments"`
}

// ToolResult is the outcome of a tool call, fed back to the model next turn.
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error,omitempty"`
}

// Approval resolves a previously-emitted pending action.
type Approval struct {
	ToolCallID string `json:"tool_call_id"`
	Approved   bool   `json:"approved"`
}

// Event types streamed to the client over SSE.
const (
	EventStep               = "step"
	EventAssistantReasoning = "assistant_reasoning"
	EventAssistantDelta     = "assistant_delta"
	EventToolCall           = "tool_call"
	EventToolResult         = "tool_result"
	EventPendingAction      = "pending_action"
	EventUsage              = "usage"
	EventDone               = "done"
	EventError              = "error"
)

// Event is a single SSE frame. Data is JSON-marshaled by the handler.
type Event struct {
	Type string
	Data any
}

// PendingActionData is emitted when a mutating tool call awaits approval; the
// stream ends after it and resumes when the client re-sends the history with an
// Approval for ToolCallID.
type PendingActionData struct {
	ToolCallID string `json:"tool_call_id"`
	Name       string `json:"name"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Body       string `json:"body,omitempty"`
	Summary    string `json:"summary,omitempty"`
	// Danger marks destructive actions (DELETE) so the UI demands a stronger,
	// typed confirmation.
	Danger bool `json:"danger,omitempty"`
}

// StepData announces the start of an agent step (LLM call) so the UI can show
// live "step N / M" progress through the agentic loop.
type StepData struct {
	Step int `json:"step"`
}

// UsageData reports the token usage of one LLM step; the client accumulates it
// across a turn.
type UsageData struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// AssistantReasoningData carries the model's chain-of-thought for a step.
type AssistantReasoningData struct {
	Text string `json:"text"`
}

// AssistantDeltaData carries assistant prose for a step.
type AssistantDeltaData struct {
	Text string `json:"text"`
}

// ToolCallData announces a tool the model decided to call.
type ToolCallData struct {
	ToolCallID string `json:"tool_call_id"`
	Name       string `json:"name"`
	Arguments  string `json:"arguments"`
}

// ToolResultData reports an executed tool's result.
type ToolResultData struct {
	ToolCallID string `json:"tool_call_id"`
	Name       string `json:"name"`
	Status     int    `json:"status,omitempty"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error,omitempty"`
}

// DoneData closes a turn with the full updated history for the client to persist.
type DoneData struct {
	Messages []Message `json:"messages"`
}

// ErrorData carries a terminal error.
type ErrorData struct {
	Message string `json:"message"`
}
