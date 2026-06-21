package agents

import (
	"context"
	"testing"

	"github.com/dropdevrahul/herald/src/model"
)

func TestAgentHooksSequence(t *testing.T) {
	var events []Event
	hook := func(ctx context.Context, ev Event) {
		events = append(events, ev)
	}
	tool := &recordingTool{}
	m := &scriptedModel{script: []model.StreamResult{toolCallResult("echo"), {Content: "final"}}}
	agent := NewAgent(m, AgentConfig{Tools: []Tool{tool}, Hooks: []Hook{hook}})

	out, err := agent.Run(context.Background(), "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "final" {
		t.Fatalf("expected out %q, got %q", "final", out)
	}

	want := []string{"turn_start", "model_response", "tool_start", "tool_end", "turn_start", "model_response", "finish"}
	got := []string{}
	for _, ev := range events {
		got = append(got, ev.Type)
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d events %v, got %d events %v", len(want), want, len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event %d: expected %q, got %q (full %v)", i, want[i], got[i], got)
		}
	}

	toolStarts := 0
	for _, ev := range events {
		if ev.Type == "tool_start" {
			toolStarts++
			if ev.Tool != "echo" {
				t.Fatalf("expected tool_start Tool %q, got %q", "echo", ev.Tool)
			}
		}
	}
	if toolStarts != 1 {
		t.Fatalf("expected exactly 1 tool_start, got %d", toolStarts)
	}

	finish := events[len(events)-1]
	if finish.Type != "finish" || finish.Content != "final" {
		t.Fatalf("expected finish event with Content %q, got %+v", "final", finish)
	}
}

func TestAgentNoHooksUnchanged(t *testing.T) {
	m := &scriptedModel{script: []model.StreamResult{{Content: "x"}}}
	out, _ := NewAgent(m, AgentConfig{}).Run(context.Background(), "q")
	if out != "x" {
		t.Fatalf("expected out %q, got %q", "x", out)
	}
}
