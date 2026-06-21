package agents

import (
	"context"
	"encoding/json"

	"github.com/dropdevrahul/herald/src/worklows"
)

// AgentTool adapts an *Agent so it can be used as a workflows.Tool, enabling
// sub-agent composition: a parent Agent can delegate a task to a wrapped
// sub-agent by calling this tool.
type AgentTool struct {
	name        string
	description string
	agent       *Agent
}

// NewAgentTool wraps an *Agent as a tool with the given name and description.
func NewAgentTool(name string, description string, agent *Agent) *AgentTool {
	return &AgentTool{name: name, description: description, agent: agent}
}

func (t *AgentTool) Name() string { return t.name }

func (t *AgentTool) Description() string { return t.description }

func (t *AgentTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input": map[string]any{
				"type":        "string",
				"description": "The task or question to delegate to this sub-agent.",
			},
		},
		"required": []string{"input"},
	}
}

func (t *AgentTool) Call(ctx context.Context, args string) (string, error) {
	input := args
	var parsed struct {
		Input string `json:"input"`
	}
	if json.Unmarshal([]byte(args), &parsed) == nil && parsed.Input != "" {
		input = parsed.Input
	}
	return t.agent.Run(ctx, input)
}

var _ workflows.Tool = (*AgentTool)(nil)
