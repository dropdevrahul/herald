package model

import "context"

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Message struct {
	Role       Role
	Content    string
	ToolCallID string
	Name       string
}

type ToolCall struct {
	ID       string
	Type     string
	Function Function
}

type Function struct {
	Name      string
	Arguments string
}

type FunctionDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type Tools []FunctionDefinition

type ToolChoice string

const (
	ToolChoiceNone     ToolChoice = "none"
	ToolChoiceAuto     ToolChoice = "auto"
	ToolChoiceRequired ToolChoice = "required"
)

type ModelOptions struct {
	Model       string
	Temperature float64
	MaxTokens   int
	Tools       Tools
	ToolChoice  ToolChoice
}

type Response struct {
	Content   string
	Usage     Usage
	ToolCalls []ToolCall
}

type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type StreamResult struct {
	Content   string
	Delta     string
	Usage     Usage
	Err       error
	ToolCalls []ToolCall
}

type Model interface {
	Generate(ctx context.Context, messages []Message, opts *ModelOptions) (*Response, error)
	Stream(ctx context.Context, messages []Message, opts *ModelOptions) <-chan StreamResult
}

type Provider string

const (
	ProviderOpenAI    Provider = "openai"
	ProviderAnthropic Provider = "anthropic"
	ProviderGemini    Provider = "gemini"
)

type Config struct {
	Provider    Provider
	Model       string
	APIKey      string
	BaseURL     string
	Temperature float64
	MaxTokens   int
}
