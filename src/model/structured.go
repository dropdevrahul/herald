package model

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// GenerateJSON calls m.Generate, extracts the first JSON object or array from
// the response content (tolerating Markdown code fences and surrounding prose),
// and unmarshals it into out.
//
// Extraction is a heuristic outermost-bracket scan: it finds the earliest { or [
// and the latest matching } or ] and treats that span as the JSON value.
// ponytail: upgrade to a real streaming JSON scanner only if the heuristic proves
// insufficient for deeply-nested or fragmented responses.
func GenerateJSON(ctx context.Context, m Model, messages []Message, opts *ModelOptions, out any) error {
	resp, err := m.Generate(ctx, messages, opts)
	if err != nil {
		return err
	}

	raw, err := extractJSON(resp.Content)
	if err != nil {
		return fmt.Errorf("GenerateJSON: %w", err)
	}

	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("GenerateJSON: unmarshal: %w", err)
	}
	return nil
}

// extractJSON strips optional Markdown fences and then returns the first
// outermost JSON object or array found in s.
func extractJSON(s string) ([]byte, error) {
	s = strings.TrimSpace(s)

	// Strip ```json ... ``` or ``` ... ``` fences.
	if idx := strings.Index(s, "```"); idx != -1 {
		inner := s[idx+3:]
		// Skip optional language tag (e.g. "json\n").
		if nl := strings.Index(inner, "\n"); nl != -1 {
			inner = inner[nl+1:]
		}
		if end := strings.LastIndex(inner, "```"); end != -1 {
			inner = inner[:end]
		}
		s = strings.TrimSpace(inner)
	}

	// Find the earliest { or [.
	firstCurly := strings.Index(s, "{")
	firstSquare := strings.Index(s, "[")

	var open, close byte
	var start int

	switch {
	case firstCurly == -1 && firstSquare == -1:
		return nil, fmt.Errorf("no JSON object or array found in response")
	case firstCurly == -1:
		start = firstSquare
		open, close = '[', ']'
	case firstSquare == -1:
		start = firstCurly
		open, close = '{', '}'
	case firstCurly < firstSquare:
		start = firstCurly
		open, close = '{', '}'
	default:
		start = firstSquare
		open, close = '[', ']'
	}

	// Find the last matching closing bracket.
	end := strings.LastIndex(s, string(close))
	if end <= start {
		return nil, fmt.Errorf("no JSON object or array found in response")
	}

	_ = open // used implicitly via start selection above
	return []byte(s[start : end+1]), nil
}
