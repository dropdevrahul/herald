package model

import (
	"context"
	"testing"
)

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
