package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

type Session struct {
	ID        string            `json:"id"`
	Messages  []Message         `json:"messages"`
	Config    map[string]string `json:"config"`
	CreatedAt string            `json:"created_at"`
	UpdatedAt string            `json:"updated_at"`
}

type Message struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

func SessionsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".herald", "sessions")
}

func CurrentSessionDir() string {
	return filepath.Join(SessionsDir(), "current")
}

func EnsureSessionsDir() (string, error) {
	dir := SessionsDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

func CreateSession() (*Session, error) {
	dir, err := EnsureSessionsDir()
	if err != nil {
		return nil, err
	}

	sessionID := uuid.New().String()
	now := time.Now().Format(time.RFC3339)
	session := &Session{
		ID:        sessionID,
		Messages:  []Message{},
		Config:    make(map[string]string),
		CreatedAt: now,
		UpdatedAt: now,
	}

	sessionDir := filepath.Join(dir, sessionID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return nil, err
	}

	currentLink := filepath.Join(dir, "current")
	os.Remove(currentLink)
	if err := os.Symlink(sessionDir, currentLink); err != nil {
		return nil, err
	}

	if err := SaveSession(session); err != nil {
		return nil, err
	}

	return session, nil
}

func LoadSession(id string) (*Session, error) {
	dir := SessionsDir()
	data, err := os.ReadFile(filepath.Join(dir, id, "session.json"))
	if err != nil {
		return nil, err
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}

	return &session, nil
}

func LoadCurrentSession() (*Session, error) {
	currentLink := filepath.Join(SessionsDir(), "current")
	target, err := os.Readlink(currentLink)
	if err != nil {
		return CreateSession()
	}

	dir := filepath.Base(target)
	return LoadSession(dir)
}

func LoadCurrentSessionWithDir() (*Session, string, error) {
	currentLink := filepath.Join(SessionsDir(), "current")
	target, err := os.Readlink(currentLink)
	if err != nil {
		s, err := CreateSession()
		if err != nil {
			return nil, "", err
		}
		return s, filepath.Join(SessionsDir(), s.ID), nil
	}

	dir := filepath.Base(target)
	s, err := LoadSession(dir)
	if err != nil {
		return nil, "", err
	}
	return s, target, nil
}

func SaveSession(s *Session) error {
	dir := filepath.Join(SessionsDir(), s.ID)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "session.json"), data, 0644)
}

func GetAllSessions() ([]Session, error) {
	dir := SessionsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var sessions []Session
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "current" {
			s, err := LoadSession(entry.Name())
			if err == nil {
				sessions = append(sessions, *s)
			}
		}
	}
	return sessions, nil
}

func DeleteSession(id string) error {
	dir := filepath.Join(SessionsDir(), id)
	return os.RemoveAll(dir)
}
