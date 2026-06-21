package session

import (
	"testing"
)

func TestCreateAndLoadSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	s, err := CreateSession()
	if err != nil {
		t.Fatalf("CreateSession() failed: %v", err)
	}
	if s.ID == "" {
		t.Error("expected non-empty ID")
	}
	if s.CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}
	if s.UpdatedAt == "" {
		t.Error("expected non-empty UpdatedAt")
	}

	got, err := LoadSession(s.ID)
	if err != nil {
		t.Fatalf("LoadSession(%q) failed: %v", s.ID, err)
	}
	if got.ID != s.ID {
		t.Errorf("loaded ID %q != created ID %q", got.ID, s.ID)
	}
}

func TestSaveSessionRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	s, err := CreateSession()
	if err != nil {
		t.Fatalf("CreateSession() failed: %v", err)
	}

	s.Messages = append(s.Messages, Message{Type: "user", Content: "hi"})
	if err := SaveSession(s); err != nil {
		t.Fatalf("SaveSession() failed: %v", err)
	}

	got, err := LoadSession(s.ID)
	if err != nil {
		t.Fatalf("LoadSession() failed: %v", err)
	}
	if len(got.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(got.Messages))
	}
	if got.Messages[0].Content != "hi" {
		t.Errorf("expected message content 'hi', got %q", got.Messages[0].Content)
	}
}
