package agents

import (
	"context"
	"testing"

	"github.com/dropdevrahul/herald/src/memory"
	"github.com/dropdevrahul/herald/src/model"
)

// scriptedModel returns a pre-scripted StreamResult for each successive Stream
// call. If the script is exhausted it repeats the last entry, which lets a
// "always asks for a tool" script drive the max-turns test.
type scriptedModel struct {
	script []model.StreamResult
	calls  int
}

func (m *scriptedModel) Generate(ctx context.Context, msgs []model.Message, opts *model.ModelOptions) (*model.Response, error) {
	return &model.Response{}, nil
}

func (m *scriptedModel) Stream(ctx context.Context, msgs []model.Message, opts *model.ModelOptions) <-chan model.StreamResult {
	idx := m.calls
	if idx >= len(m.script) {
		idx = len(m.script) - 1
	}
	res := m.script[idx]
	m.calls++

	ch := make(chan model.StreamResult, 1)
	ch <- res
	close(ch)
	return ch
}

// recordingTool implements workflows.Tool and counts how often it is called.
type recordingTool struct {
	calls int
}

func (t *recordingTool) Name() string        { return "echo" }
func (t *recordingTool) Description() string  { return "echoes back" }
func (t *recordingTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (t *recordingTool) Call(ctx context.Context, args string) (string, error) {
	t.calls++
	return "echoed", nil
}

func toolCallResult(name string) model.StreamResult {
	return model.StreamResult{
		ToolCalls: []model.ToolCall{
			{ID: "c1", Type: "function", Function: model.Function{Name: name, Arguments: "{}"}},
		},
	}
}

func TestAgentNoTools(t *testing.T) {
	m := &scriptedModel{script: []model.StreamResult{{Content: "hi"}}}
	out, err := NewAgent(m, AgentConfig{}).Run(context.Background(), "q")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "hi" {
		t.Fatalf("got %q, want %q", out, "hi")
	}
}

func TestAgentToolLoop(t *testing.T) {
	tool := &recordingTool{}
	m := &scriptedModel{script: []model.StreamResult{
		toolCallResult("echo"),
		{Content: "final answer"},
	}}

	out, err := NewAgent(m, AgentConfig{Tools: []Tool{tool}}).Run(context.Background(), "do it")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "final answer" {
		t.Fatalf("got %q, want %q", out, "final answer")
	}
	if tool.calls != 1 {
		t.Fatalf("tool called %d times, want 1", tool.calls)
	}
}

func TestAgentMaxTurns(t *testing.T) {
	tool := &recordingTool{}
	// Always returns a tool call -> would loop forever without the cap.
	m := &scriptedModel{script: []model.StreamResult{toolCallResult("echo")}}

	out, err := NewAgent(m, AgentConfig{Tools: []Tool{tool}, MaxTurns: 2}).Run(context.Background(), "loop")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tool.calls > 2 {
		t.Fatalf("tool called %d times, want at most 2", tool.calls)
	}
	if out == "" {
		t.Fatalf("expected a non-empty result")
	}
}

func TestAgentMemoryContinuity(t *testing.T) {
	mem := memory.NewBufferMemory()
	m := &scriptedModel{script: []model.StreamResult{{Content: "a"}, {Content: "b"}}}
	agent := NewAgent(m, AgentConfig{Memory: mem})

	if _, err := agent.Run(context.Background(), "first"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := agent.Run(context.Background(), "second"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := mem.Messages()
	if len(got) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(got))
	}
	wantRoles := []model.Role{model.RoleUser, model.RoleAssistant, model.RoleUser, model.RoleAssistant}
	wantContents := []string{"first", "a", "second", "b"}
	for i := range got {
		if got[i].Role != wantRoles[i] {
			t.Errorf("message %d: expected role %q, got %q", i, wantRoles[i], got[i].Role)
		}
		if got[i].Content != wantContents[i] {
			t.Errorf("message %d: expected content %q, got %q", i, wantContents[i], got[i].Content)
		}
	}
}

func TestAgentNilMemoryUnchanged(t *testing.T) {
	m := &scriptedModel{script: []model.StreamResult{{Content: "x"}}}
	out, err := NewAgent(m, AgentConfig{}).Run(context.Background(), "q")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "x" {
		t.Fatalf("got %q, want %q", out, "x")
	}
}

func TestAgentStopFunc(t *testing.T) {
	tool := &recordingTool{}
	m := &scriptedModel{script: []model.StreamResult{toolCallResult("echo")}}

	stopped := false
	cfg := AgentConfig{
		Tools:    []Tool{tool},
		MaxTurns: 10,
		Stop: func(turn int, lastContent string) bool {
			if turn >= 1 {
				stopped = true
				return true
			}
			return false
		},
	}

	_, err := NewAgent(m, cfg).Run(context.Background(), "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !stopped {
		t.Fatalf("Stop func was never observed to fire")
	}
	if tool.calls != 1 {
		t.Fatalf("tool called %d times, want 1 before stop", tool.calls)
	}
}
