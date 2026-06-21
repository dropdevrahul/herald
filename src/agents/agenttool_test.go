package agents

import (
	"context"
	"testing"

	"github.com/dropdevrahul/herald/src/model"
)

// capturingModel records the content of the last user message it is streamed
// and always answers with a fixed string, so tests can assert what input a
// wrapped sub-agent actually received.
type capturingModel struct {
	lastInput string
}

func (m *capturingModel) Generate(ctx context.Context, msgs []model.Message, opts *model.ModelOptions) (*model.Response, error) {
	return &model.Response{}, nil
}

func (m *capturingModel) Stream(ctx context.Context, msgs []model.Message, opts *model.ModelOptions) <-chan model.StreamResult {
	for _, msg := range msgs {
		if msg.Role == model.RoleUser {
			m.lastInput = msg.Content
		}
	}
	ch := make(chan model.StreamResult, 1)
	ch <- model.StreamResult{Content: "sub answer"}
	close(ch)
	return ch
}

func TestAgentToolParsesJSONInput(t *testing.T) {
	cm := &capturingModel{}
	sub := NewAgent(cm, AgentConfig{})
	tool := NewAgentTool("researcher", "researches things", sub)

	out, err := tool.Call(context.Background(), `{"input":"please research X"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "sub answer" {
		t.Fatalf("got %q, want %q", out, "sub answer")
	}
	if cm.lastInput != "please research X" {
		t.Fatalf("lastInput = %q, want %q", cm.lastInput, "please research X")
	}
}

func TestAgentToolRawFallback(t *testing.T) {
	cm := &capturingModel{}
	sub := NewAgent(cm, AgentConfig{})
	tool := NewAgentTool("researcher", "researches things", sub)

	if _, err := tool.Call(context.Background(), "just a raw string"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cm.lastInput != "just a raw string" {
		t.Fatalf("lastInput = %q, want %q", cm.lastInput, "just a raw string")
	}
}

func TestParentAgentUsesSubAgent(t *testing.T) {
	cm := &capturingModel{}
	sub := NewAgent(cm, AgentConfig{})
	tool := NewAgentTool("researcher", "researches", sub)

	pm := &scriptedModel{script: []model.StreamResult{
		toolCallResult("researcher"),
		{Content: "final answer"},
	}}
	parent := NewAgent(pm, AgentConfig{Tools: []Tool{tool}})

	out, err := parent.Run(context.Background(), "delegate please")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "final answer" {
		t.Fatalf("got %q, want %q", out, "final answer")
	}
	if cm.lastInput == "" {
		t.Fatalf("sub-agent was never invoked; lastInput is empty")
	}
}
