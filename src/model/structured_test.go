package model

import (
	"context"
	"testing"
)

// streamModel is a test double whose Stream emits the given deltas in order.
type streamModel struct {
	deltas []string
}

func (s *streamModel) Generate(_ context.Context, _ []Message, _ *ModelOptions) (*Response, error) {
	return &Response{}, nil
}

func (s *streamModel) Stream(_ context.Context, _ []Message, _ *ModelOptions) <-chan StreamResult {
	ch := make(chan StreamResult)
	go func() {
		defer close(ch)
		for _, d := range s.deltas {
			ch <- StreamResult{Delta: d}
		}
	}()
	return ch
}

// fakeModel is a test double that returns a fixed Content string from Generate.
type fakeModel struct {
	content string
}

func (f *fakeModel) Generate(_ context.Context, _ []Message, _ *ModelOptions) (*Response, error) {
	return &Response{Content: f.content}, nil
}

func (f *fakeModel) Stream(_ context.Context, _ []Message, _ *ModelOptions) <-chan StreamResult {
	ch := make(chan StreamResult)
	close(ch)
	return ch
}

func TestGenerateJSON(t *testing.T) {
	type target struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	tests := []struct {
		name    string
		content string
		want    target
	}{
		{
			name:    "bare JSON",
			content: `{"name":"hello","value":42}`,
			want:    target{Name: "hello", Value: 42},
		},
		{
			name:    "json fence wrapped",
			content: "```json\n{\"name\":\"world\",\"value\":7}\n```",
			want:    target{Name: "world", Value: 7},
		},
		{
			name:    "prose before and after JSON",
			content: "Sure! Here is the result:\n{\"name\":\"go\",\"value\":99}\nHope that helps.",
			want:    target{Name: "go", Value: 99},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := &fakeModel{content: tc.content}
			var got target
			if err := GenerateJSON(context.Background(), m, nil, nil, &got); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestGenerateJSONStream(t *testing.T) {
	type target struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	// JSON arrives fragmented across deltas, wrapped in a fence and prose.
	deltas := []string{"Here you go:\n```json\n{\"name\":", "\"streamed\"", ",\"value\":13}", "\n```"}
	m := &streamModel{deltas: deltas}

	var streamed string
	var got target
	err := GenerateJSONStream(context.Background(), m, nil, nil, &got, func(d string) {
		streamed += d
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := (target{Name: "streamed", Value: 13}); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	// onDelta must have received the full content in order.
	wantStreamed := "Here you go:\n```json\n{\"name\":\"streamed\",\"value\":13}\n```"
	if streamed != wantStreamed {
		t.Fatalf("onDelta accumulated %q, want %q", streamed, wantStreamed)
	}
}
