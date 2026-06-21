package agents

import (
	"context"
	"testing"

	"github.com/dropdevrahul/herald/src/model"
)

// argsTool implements the Tool interface and records the arguments it was called with.
type argsTool struct {
	calls    int
	lastArgs string
}

func (t *argsTool) Name() string        { return "echo" }
func (t *argsTool) Description() string  { return "echoes" }
func (t *argsTool) Parameters() map[string]any {
	return map[string]any{"type": "object"}
}
func (t *argsTool) Call(ctx context.Context, args string) (string, error) {
	t.calls++
	t.lastArgs = args
	return "ran", nil
}

func TestApproverDenies(t *testing.T) {
	tool := &argsTool{}
	m := &scriptedModel{script: []model.StreamResult{
		toolCallResult("echo"),
		{Content: "done"},
	}}

	out, err := NewAgent(m, AgentConfig{
		Tools: []Tool{tool},
		Approver: func(ctx context.Context, call model.ToolCall) (ApprovalDecision, error) {
			return ApprovalDecision{Approved: false, Reason: "nope"}, nil
		},
	}).Run(context.Background(), "go")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "done" {
		t.Fatalf("got %q, want %q", out, "done")
	}
	if tool.calls != 0 {
		t.Fatalf("tool called %d times, want 0", tool.calls)
	}
}

func TestApproverApproves(t *testing.T) {
	tool := &argsTool{}
	m := &scriptedModel{script: []model.StreamResult{
		toolCallResult("echo"),
		{Content: "done"},
	}}

	out, err := NewAgent(m, AgentConfig{
		Tools: []Tool{tool},
		Approver: func(ctx context.Context, call model.ToolCall) (ApprovalDecision, error) {
			return ApprovalDecision{Approved: true}, nil
		},
	}).Run(context.Background(), "go")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "done" {
		t.Fatalf("got %q, want %q", out, "done")
	}
	if tool.calls != 1 {
		t.Fatalf("tool called %d times, want 1", tool.calls)
	}
}

func TestApproverEditsArgs(t *testing.T) {
	tool := &argsTool{}
	m := &scriptedModel{script: []model.StreamResult{
		toolCallResult("echo"),
		{Content: "done"},
	}}

	_, err := NewAgent(m, AgentConfig{
		Tools: []Tool{tool},
		Approver: func(ctx context.Context, call model.ToolCall) (ApprovalDecision, error) {
			return ApprovalDecision{Approved: true, Args: `{"edited":true}`}, nil
		},
	}).Run(context.Background(), "go")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tool.lastArgs != `{"edited":true}` {
		t.Fatalf("lastArgs = %q, want %q", tool.lastArgs, `{"edited":true}`)
	}
}
