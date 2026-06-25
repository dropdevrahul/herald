package agents

import (
	"context"
	"testing"
)

func TestFuncTool(t *testing.T) {
	// Build a FuncTool whose fn echoes its args back.
	echo := NewFuncTool(
		"echo",
		"Echoes the input",
		map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}}},
		func(ctx context.Context, args string) (string, error) {
			return args, nil
		},
	)

	result, err := echo.Call(context.Background(), "hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hi" {
		t.Fatalf("expected %q, got %q", "hi", result)
	}

	// Parameters() with a provided map must be non-nil.
	params := echo.Parameters()
	if params == nil {
		t.Fatal("Parameters() returned nil for provided map")
	}

	// Parameters() with a nil map must also return non-nil (the default schema).
	noParams := NewFuncTool("noop", "No params", nil, func(ctx context.Context, args string) (string, error) {
		return "", nil
	})
	if noParams.Parameters() == nil {
		t.Fatal("Parameters() returned nil for nil map")
	}
}
