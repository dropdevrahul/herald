package workflows

import (
	"context"
	"encoding/json"
	"testing"
)

func TestMemoryCheckpointerRoundTrip(t *testing.T) {
	mc := NewMemoryCheckpointer()
	ctx := context.Background()

	cp := Checkpoint{
		ThreadID:  "t1",
		Node:      "b",
		Iteration: 2,
		State:     json.RawMessage(`"x"`),
	}

	if err := mc.Save(ctx, cp); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	got, ok, err := mc.Load(ctx, "t1")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if !ok {
		t.Fatal("Load() ok=false, want true")
	}
	if got.ThreadID != cp.ThreadID {
		t.Errorf("ThreadID: got %q, want %q", got.ThreadID, cp.ThreadID)
	}
	if got.Node != cp.Node {
		t.Errorf("Node: got %q, want %q", got.Node, cp.Node)
	}
	if got.Iteration != cp.Iteration {
		t.Errorf("Iteration: got %d, want %d", got.Iteration, cp.Iteration)
	}
	if string(got.State) != string(cp.State) {
		t.Errorf("State: got %s, want %s", got.State, cp.State)
	}

	_, ok, err = mc.Load(ctx, "missing")
	if err != nil {
		t.Fatalf("Load(missing) error: %v", err)
	}
	if ok {
		t.Fatal("Load(missing) ok=true, want false")
	}
}

func TestFileCheckpointerRoundTrip(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	cp := Checkpoint{
		ThreadID:  "thread-file",
		Node:      "nodeC",
		Iteration: 7,
		State:     json.RawMessage(`{"key":"value"}`),
	}

	// Save via one instance.
	if err := NewFileCheckpointer(dir).Save(ctx, cp); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Load via a NEW instance to prove durability.
	got, ok, err := NewFileCheckpointer(dir).Load(ctx, "thread-file")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if !ok {
		t.Fatal("Load() ok=false, want true")
	}
	if got.Node != cp.Node {
		t.Errorf("Node: got %q, want %q", got.Node, cp.Node)
	}
	if got.Iteration != cp.Iteration {
		t.Errorf("Iteration: got %d, want %d", got.Iteration, cp.Iteration)
	}

	// Missing thread returns ok=false, nil error.
	_, ok, err = NewFileCheckpointer(dir).Load(ctx, "no-such-thread")
	if err != nil {
		t.Fatalf("Load(missing) error: %v", err)
	}
	if ok {
		t.Fatal("Load(missing) ok=true, want false")
	}
}
