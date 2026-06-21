package memory

import (
	"path/filepath"
	"testing"

	"github.com/dropdevrahul/herald/src/model"
)

func TestFileMemory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mem.json")

	// Create a FileMemory, add two messages, and close it.
	fm, err := NewFileMemory(path)
	if err != nil {
		t.Fatalf("NewFileMemory: %v", err)
	}
	fm.Add(model.Message{Role: model.RoleUser, Content: "hello"})
	fm.Add(model.Message{Role: model.RoleAssistant, Content: "world"})

	// Reload from the same path — simulates a restart.
	fm2, err := NewFileMemory(path)
	if err != nil {
		t.Fatalf("NewFileMemory reload: %v", err)
	}

	msgs := fm2.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages after reload, got %d", len(msgs))
	}
	if msgs[0].Content != "hello" || msgs[1].Content != "world" {
		t.Fatalf("unexpected messages: %+v", msgs)
	}

	// Clear and reload — messages must be empty.
	fm2.Clear()

	fm3, err := NewFileMemory(path)
	if err != nil {
		t.Fatalf("NewFileMemory after clear: %v", err)
	}
	if len(fm3.Messages()) != 0 {
		t.Fatalf("expected 0 messages after Clear, got %d", len(fm3.Messages()))
	}
}
