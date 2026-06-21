package memory

import (
	"testing"

	"github.com/dropdevrahul/herald/src/model"
)

func TestBufferMemoryAddMessages(t *testing.T) {
	m := NewBufferMemory()
	m.Add(model.Message{Role: model.RoleUser, Content: "one"})
	m.Add(model.Message{Role: model.RoleAssistant, Content: "two"})
	m.Add(model.Message{Role: model.RoleUser, Content: "three"})

	got := m.Messages()
	if len(got) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(got))
	}
	want := []string{"one", "two", "three"}
	for i, w := range want {
		if got[i].Content != w {
			t.Errorf("message %d: expected %q, got %q", i, w, got[i].Content)
		}
	}

	// Mutating the returned slice must not affect subsequent calls.
	got[0].Content = "mutated"
	again := m.Messages()
	if again[0].Content != "one" {
		t.Errorf("mutating returned slice affected internal state: got %q", again[0].Content)
	}
}

func TestBufferMemoryClear(t *testing.T) {
	m := NewBufferMemory()
	m.Add(model.Message{Role: model.RoleUser, Content: "one"})
	m.Add(model.Message{Role: model.RoleUser, Content: "two"})
	m.Clear()
	if len(m.Messages()) != 0 {
		t.Errorf("expected empty after Clear, got %d", len(m.Messages()))
	}
}

func TestWindowMemoryWindowsNonSystem(t *testing.T) {
	m := NewWindowMemory(2)
	m.Add(model.Message{Role: model.RoleSystem, Content: "sys"})
	m.Add(model.Message{Role: model.RoleUser, Content: "u1"})
	m.Add(model.Message{Role: model.RoleUser, Content: "u2"})
	m.Add(model.Message{Role: model.RoleUser, Content: "u3"})
	m.Add(model.Message{Role: model.RoleUser, Content: "u4"})

	got := m.Messages()
	if len(got) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(got))
	}
	want := []string{"sys", "u3", "u4"}
	for i, w := range want {
		if got[i].Content != w {
			t.Errorf("message %d: expected %q, got %q", i, w, got[i].Content)
		}
	}
}

func TestWindowMemoryUnlimited(t *testing.T) {
	m := NewWindowMemory(0)
	for _, c := range []string{"a", "b", "c", "d", "e"} {
		m.Add(model.Message{Role: model.RoleUser, Content: c})
	}
	got := m.Messages()
	if len(got) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(got))
	}
}
