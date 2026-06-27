package workflows

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestGraphCompileNoStart(t *testing.T) {
	g := NewGraph(nil)
	g.AddNode("a", "node a", func(ctx context.Context, state any) (any, error) {
		return state, nil
	})
	_, err := g.Compile()
	if err == nil {
		t.Fatal("expected non-nil error when Start is unset, got nil")
	}
}

func TestGraphCompileMissingStart(t *testing.T) {
	g := NewGraph(nil)
	g.SetStart("nonexistent")
	_, err := g.Compile()
	if err == nil {
		t.Fatal("expected non-nil error when Start node not registered, got nil")
	}
}

func TestGraphRunLinear(t *testing.T) {
	g := NewGraph(nil)
	g.AddNode("a", "node a", func(ctx context.Context, state any) (any, error) {
		s, _ := state.(string)
		return s + "|markerA", nil
	})
	g.AddNode("b", "node b", func(ctx context.Context, state any) (any, error) {
		s, _ := state.(string)
		return s + "|markerB", nil
	})
	g.AddEdge("a", "b")
	g.SetStart("a")

	compiled, err := g.Compile()
	if err != nil {
		t.Fatalf("Compile() failed: %v", err)
	}

	result, err := compiled.Run(context.Background(), "start")
	if err != nil {
		t.Fatalf("Run() failed: %v", err)
	}

	s, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}
	if !strings.Contains(s, "markerA") {
		t.Errorf("result %q does not contain 'markerA'", s)
	}
	if !strings.Contains(s, "markerB") {
		t.Errorf("result %q does not contain 'markerB'", s)
	}
}

func TestGraphConditionalLoop(t *testing.T) {
	type loopState struct {
		count int
		val   string
	}

	threshold := 3

	g := NewGraph(nil)
	g.AddNode("step", "worker node", func(ctx context.Context, state any) (any, error) {
		st, _ := state.(*loopState)
		st.count++
		st.val += "x"
		return st, nil
	})
	g.AddConditionalNode("next", func(ctx context.Context, state any) string {
		st, _ := state.(*loopState)
		if st.count < threshold {
			return "step"
		}
		return ""
	})
	g.AddEdge("step", "next")
	g.SetStart("step")

	compiled, err := g.Compile()
	if err != nil {
		t.Fatalf("Compile() failed: %v", err)
	}

	initial := &loopState{}
	result, err := compiled.Run(context.Background(), initial)
	if err != nil {
		t.Fatalf("Run() failed: %v", err)
	}

	st, ok := result.(*loopState)
	if !ok {
		t.Fatalf("expected *loopState result, got %T", result)
	}
	if st.count != threshold {
		t.Errorf("expected count==%d, got %d", threshold, st.count)
	}
	if st.val != strings.Repeat("x", threshold) {
		t.Errorf("expected val==%q, got %q", strings.Repeat("x", threshold), st.val)
	}
}

func TestGraphMaxIterations(t *testing.T) {
	callCount := 0

	g := NewGraph(nil)
	g.AddNode("loop", "loop node", func(ctx context.Context, state any) (any, error) {
		callCount++
		return state, nil
	})
	g.AddConditionalNode("always", func(ctx context.Context, state any) string {
		return "loop"
	})
	g.AddEdge("loop", "always")
	g.SetStart("loop")

	compiled, err := g.Compile()
	if err != nil {
		t.Fatalf("Compile() failed: %v", err)
	}
	compiled.MaxIterations = 3

	_, err = compiled.Run(context.Background(), "input")
	if err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}

	if callCount > 3 {
		t.Errorf("node ran %d times, expected at most 3", callCount)
	}
}

func TestGraphRunThreadCheckpoints(t *testing.T) {
	g := NewGraph(nil).
		AddNode("a", "node a", func(ctx context.Context, state any) (any, error) {
			return "a-done", nil
		}).
		AddNode("b", "node b", func(ctx context.Context, state any) (any, error) {
			return "b-done", nil
		}).
		AddEdge("a", "b").
		SetStart("a")

	cp := NewMemoryCheckpointer()
	compiled, err := g.Compile()
	if err != nil {
		t.Fatalf("Compile() failed: %v", err)
	}
	compiled.WithCheckpointer(cp)

	result, err := compiled.RunThread(context.Background(), "thread1", "start")
	if err != nil {
		t.Fatalf("RunThread() failed: %v", err)
	}
	if result != "b-done" {
		t.Errorf("result: got %q, want %q", result, "b-done")
	}

	// After completion, checkpoint Node should be "" (run finished).
	saved, ok, err := cp.Load(context.Background(), "thread1")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if !ok {
		t.Fatal("Load() ok=false after completed run, want true")
	}
	if saved.Node != "" {
		t.Errorf("checkpoint Node after completion: got %q, want %q", saved.Node, "")
	}
}

func TestGraphRunThreadResumes(t *testing.T) {
	var executed []string

	g := NewGraph(nil).
		AddNode("a", "node a", func(ctx context.Context, state any) (any, error) {
			executed = append(executed, "a")
			return "a-done", nil
		}).
		AddNode("b", "node b", func(ctx context.Context, state any) (any, error) {
			executed = append(executed, "b")
			return "b-done", nil
		}).
		AddEdge("a", "b").
		SetStart("a")

	cp := NewMemoryCheckpointer()
	// Pre-seed a checkpoint that positions us at node "b", skipping "a".
	if err := cp.Save(context.Background(), Checkpoint{
		ThreadID:  "thread2",
		Node:      "b",
		Iteration: 1,
		State:     json.RawMessage(`"seed"`),
	}); err != nil {
		t.Fatalf("pre-seed Save() error: %v", err)
	}

	compiled, err := g.Compile()
	if err != nil {
		t.Fatalf("Compile() failed: %v", err)
	}
	compiled.WithCheckpointer(cp)

	result, err := compiled.RunThread(context.Background(), "thread2", "ignored")
	if err != nil {
		t.Fatalf("RunThread() failed: %v", err)
	}
	if result != "b-done" {
		t.Errorf("result: got %q, want %q", result, "b-done")
	}

	// "a" must not have run; "b" must have.
	for _, n := range executed {
		if n == "a" {
			t.Error("node 'a' was executed, expected it to be skipped on resume")
		}
	}
	foundB := false
	for _, n := range executed {
		if n == "b" {
			foundB = true
		}
	}
	if !foundB {
		t.Error("node 'b' was not executed, expected it to run on resume")
	}
}
