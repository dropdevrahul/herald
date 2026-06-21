package workflows

import (
	"context"
	"testing"

	"github.com/dropdevrahul/herald/src/model"
)

// mockModel is an in-process mock implementing model.Model.
type mockModel struct{}

func (m *mockModel) Generate(_ context.Context, _ []model.Message, _ *model.ModelOptions) (*model.Response, error) {
	return &model.Response{Content: "ok"}, nil
}

func (m *mockModel) Stream(_ context.Context, _ []model.Message, _ *model.ModelOptions) <-chan model.StreamResult {
	ch := make(chan model.StreamResult, 1)
	ch <- model.StreamResult{Content: "ok"}
	close(ch)
	return ch
}

// --- ChainingWorkflow ---

func TestChainingWorkflowRun(t *testing.T) {
	m := &mockModel{}
	nodes := []Node{
		{Name: "a", Prompt: "step a"},
		{Name: "b", Prompt: "step b"},
	}
	wf := NewChainingWorkflow(m, nodes)
	out, err := wf.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == "" {
		t.Fatal("expected non-empty output")
	}
}

func TestChainingWorkflowRunStream(t *testing.T) {
	m := &mockModel{}
	nodes := []Node{
		{Name: "a", Prompt: "step a"},
	}
	wf := NewChainingWorkflow(m, nodes)

	var collected string
	err := wf.RunStream(context.Background(), "hello", func(result model.StreamResult) error {
		collected += result.Content + result.Delta
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if collected == "" {
		t.Fatal("expected handler to receive content")
	}
}

func TestNewChainingWorkflowNoNodes(t *testing.T) {
	m := &mockModel{}
	wf := NewChainingWorkflow(m, nil)
	_, err := wf.Run(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error for empty nodes, got nil")
	}
	if err != ErrNoNodes {
		t.Fatalf("expected ErrNoNodes, got %v", err)
	}
}

// --- OrchestratorWorkflow ---

func TestOrchestratorSequential(t *testing.T) {
	m := &mockModel{}
	nodes := []Node{
		{Name: "x", Prompt: "task x"},
		{Name: "y", Prompt: "task y"},
	}
	wf := NewOrchestratorWorkflow(m, nodes, DefaultAggregator)
	out, err := wf.Run(context.Background(), "input")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == "" {
		t.Fatal("expected non-empty aggregated output")
	}
}

func TestOrchestratorParallel(t *testing.T) {
	m := &mockModel{}
	nodes := []Node{
		{Name: "p1", Prompt: "task p1"},
		{Name: "p2", Prompt: "task p2"},
	}
	ow := &OrchestratorWorkflow{
		Model:   m,
		Nodes:   nodes,
		Parallel: true,
	}
	out, err := ow.Run(context.Background(), "input")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == "" {
		t.Fatal("expected non-empty aggregated output")
	}
}

// --- ParallelWorkflow ---

func TestParallelWorkflow(t *testing.T) {
	m := &mockModel{}
	nodes := []Node{
		{Name: "a", Prompt: "pa"},
		{Name: "b", Prompt: "pb"},
	}
	wf := NewParallelWorkflow(m, nodes)
	out, err := wf.Run(context.Background(), "input")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == "" {
		t.Fatal("expected non-empty output")
	}
}

// --- DefaultAggregator ---

func TestDefaultAggregator(t *testing.T) {
	result := DefaultAggregator([]string{"a", "b"})
	expected := "a\n\nb"
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}
