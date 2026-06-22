package model

import (
	"context"
	"time"
)

// RetryModel wraps a Model and retries failed calls with exponential backoff.
//
// maxRetries is the number of EXTRA attempts after the first, so the total
// number of attempts is 1 + maxRetries. When maxRetries <= 0 a single attempt
// is made with no retries.
type RetryModel struct {
	m          Model
	maxRetries int
}

var _ Model = (*RetryModel)(nil)

// NewRetryModel wraps m so that failed Generate/Stream calls are retried with
// exponential backoff. maxRetries is the number of extra attempts after the
// first.
func NewRetryModel(m Model, maxRetries int) *RetryModel {
	return &RetryModel{m: m, maxRetries: maxRetries}
}

// backoff returns the wait duration before the retry following the given
// (zero-based) failed attempt: 100ms * 2^attempt.
func backoff(attempt int) time.Duration {
	return 100 * time.Millisecond * time.Duration(1<<uint(attempt))
}

// sleepCtx waits for d, returning false if ctx is cancelled during the wait.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// Generate calls the wrapped model, retrying on error up to 1+maxRetries
// attempts with exponential backoff between attempts.
func (r *RetryModel) Generate(ctx context.Context, messages []Message, opts *ModelOptions) (*Response, error) {
	attempts := 1 + r.maxRetries
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		resp, err := r.m.Generate(ctx, messages, opts)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if i == attempts-1 {
			break
		}
		if !sleepCtx(ctx, backoff(i)) {
			return nil, ctx.Err()
		}
	}
	return nil, lastErr
}

// Stream calls the wrapped model and forwards results. It only retries before
// the first delta/content is emitted.
//
// ponytail: Stream's retry ceiling is the first emitted output. Once any
// delta/content has been forwarded the underlying stream is partially
// consumed and cannot be safely restarted, so errors after that point
// propagate as-is (no mid-stream resume).
func (r *RetryModel) Stream(ctx context.Context, messages []Message, opts *ModelOptions) <-chan StreamResult {
	out := make(chan StreamResult)
	attempts := 1 + r.maxRetries
	if attempts < 1 {
		attempts = 1
	}
	go func() {
		defer close(out)
		var lastErr error
		for i := 0; i < attempts; i++ {
			ch := r.m.Stream(ctx, messages, opts)
			emitted := false
			attemptErr := error(nil)
			for res := range ch {
				if res.Err != nil {
					if emitted {
						// Output already emitted; cannot restart, propagate.
						out <- res
						return
					}
					// Pre-emit error: record and consider a retry.
					attemptErr = res.Err
					break
				}
				if res.Delta != "" || res.Content != "" {
					emitted = true
				}
				out <- res
			}
			if attemptErr == nil {
				// Clean end (or fully emitted stream).
				return
			}
			// Drain remaining underlying results to avoid leaking producer.
			for range ch {
			}
			lastErr = attemptErr
			if i == attempts-1 {
				out <- StreamResult{Err: lastErr}
				return
			}
			if !sleepCtx(ctx, backoff(i)) {
				out <- StreamResult{Err: ctx.Err()}
				return
			}
		}
	}()
	return out
}
