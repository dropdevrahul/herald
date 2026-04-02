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
	Role    Role
	Content string
}

type ModelOptions struct {
	Model       string
	Temperature float64
	MaxTokens   int
}

type Response struct {
	Content string
	Usage   Usage
}

type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type Model interface {
	Generate(ctx context.Context, messages []Message, opts *ModelOptions) (*Response, error)
	Stream(ctx context.Context, messages []Message, opts *ModelOptions) (<-chan string, <-chan error)
}
