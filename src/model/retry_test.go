package model

import (
	"context"
	"errors"
	"testing"
)

// flakyGenModel fails its Generate the first failTimes calls, then succeeds.
type flakyGenModel struct {
	failTimes int
	calls     int
}

func (f *flakyGenModel) Generate(ctx context.Context, messages []Message, opts *ModelOptions) (*Response, error) {
	f.calls++
	if f.calls <= f.failTimes {
		return nil, errors.New("generate boom")
	}
	return &Response{Content: "ok"}, nil
}

func (f *flakyGenModel) Stream(ctx context.Context, messages []Message, opts *ModelOptions) <-chan StreamResult {
	ch := make(chan StreamResult)
	close(ch)
	return ch
}

func TestRetryGenerateSucceedsWhenRetriesSuffice(t *testing.T) {
	f := &flakyGenModel{failTimes: 2}
	r := NewRetryModel(f, 2)
	resp, err := r.Generate(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if resp == nil || resp.Content != "ok" {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if f.calls != 3 {
		t.Fatalf("expected 3 calls, got %d", f.calls)
	}
}

func TestRetryGenerateExhausts(t *testing.T) {
	f := &flakyGenModel{failTimes: 5}
	r := NewRetryModel(f, 2)
	_, err := r.Generate(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error when retries are insufficient")
	}
	if f.calls != 3 {
		t.Fatalf("expected 3 attempts, got %d", f.calls)
	}
}

// flakyStreamModel errors before any output for the first failTimes calls,
// then on a later call emits successful results.
type flakyStreamModel struct {
	failTimes int
	calls     int
}

func (f *flakyStreamModel) Generate(ctx context.Context, messages []Message, opts *ModelOptions) (*Response, error) {
	return &Response{}, nil
}

func (f *flakyStreamModel) Stream(ctx context.Context, messages []Message, opts *ModelOptions) <-chan StreamResult {
	f.calls++
	ch := make(chan StreamResult)
	failing := f.calls <= f.failTimes
	go func() {
		defer close(ch)
		if failing {
			ch <- StreamResult{Err: errors.New("stream boom")}
			return
		}
		ch <- StreamResult{Delta: "hello"}
		ch <- StreamResult{Delta: " world"}
	}()
	return ch
}

func TestRetryStreamRestartsBeforeOutput(t *testing.T) {
	f := &flakyStreamModel{failTimes: 2}
	r := NewRetryModel(f, 2)
	var deltas []string
	for res := range r.Stream(context.Background(), nil, nil) {
		if res.Err != nil {
			t.Fatalf("unexpected error: %v", res.Err)
		}
		if res.Delta != "" {
			deltas = append(deltas, res.Delta)
		}
	}
	if len(deltas) != 2 || deltas[0] != "hello" || deltas[1] != " world" {
		t.Fatalf("expected [hello, world], got %v", deltas)
	}
	if f.calls != 3 {
		t.Fatalf("expected 3 stream calls, got %d", f.calls)
	}
}

// emitThenErrModel emits one delta then errors, every call.
type emitThenErrModel struct {
	calls int
}

func (m *emitThenErrModel) Generate(ctx context.Context, messages []Message, opts *ModelOptions) (*Response, error) {
	return &Response{}, nil
}

func (m *emitThenErrModel) Stream(ctx context.Context, messages []Message, opts *ModelOptions) <-chan StreamResult {
	m.calls++
	ch := make(chan StreamResult)
	go func() {
		defer close(ch)
		ch <- StreamResult{Delta: "partial"}
		ch <- StreamResult{Err: errors.New("mid-stream boom")}
	}()
	return ch
}

func TestRetryStreamPropagatesPostEmitError(t *testing.T) {
	m := &emitThenErrModel{}
	r := NewRetryModel(m, 2)
	var deltas []string
	var gotErr error
	for res := range r.Stream(context.Background(), nil, nil) {
		if res.Err != nil {
			gotErr = res.Err
			continue
		}
		if res.Delta != "" {
			deltas = append(deltas, res.Delta)
		}
	}
	if len(deltas) != 1 || deltas[0] != "partial" {
		t.Fatalf("expected [partial], got %v", deltas)
	}
	if gotErr == nil {
		t.Fatal("expected mid-stream error to propagate")
	}
	if m.calls != 1 {
		t.Fatalf("expected no retry (1 call), got %d", m.calls)
	}
}
