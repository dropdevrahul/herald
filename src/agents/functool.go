package agents

import (
	"context"
	"fmt"

	"github.com/dropdevrahul/herald/src/worklows"
)

// FuncTool adapts a plain function to the workflows.Tool interface so tools can
// be defined without writing a struct with four methods.
type FuncTool struct {
	name        string
	description string
	parameters  map[string]any
	fn          func(ctx context.Context, args string) (string, error)
}

// NewFuncTool constructs a FuncTool from plain values. parameters may be nil; a
// minimal valid JSON-schema object is returned from Parameters() in that case.
func NewFuncTool(
	name string,
	description string,
	parameters map[string]any,
	fn func(ctx context.Context, args string) (string, error),
) *FuncTool {
	return &FuncTool{
		name:        name,
		description: description,
		parameters:  parameters,
		fn:          fn,
	}
}

// compile-time check
var _ workflows.Tool = (*FuncTool)(nil)

func (t *FuncTool) Name() string        { return t.name }
func (t *FuncTool) Description() string { return t.description }

// Parameters returns the stored schema, or a minimal valid JSON-schema object
// when none was provided.
func (t *FuncTool) Parameters() map[string]any {
	if t.parameters != nil {
		return t.parameters
	}
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

// Call delegates to the wrapped function. Returns an error if no function was set.
func (t *FuncTool) Call(ctx context.Context, args string) (string, error) {
	if t.fn == nil {
		return "", fmt.Errorf("FuncTool %q: fn is nil", t.name)
	}
	return t.fn(ctx, args)
}
