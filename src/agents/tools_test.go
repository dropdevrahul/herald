package agents

import (
	"context"
	"strings"
	"testing"
)

func TestFileToolWriteRead(t *testing.T) {
	sessionDir := t.TempDir()
	ft := NewFileTool(sessionDir)
	ctx := context.Background()

	_, err := ft.Call(ctx, `{"operation":"write","path":"a.txt","content":"hello"}`)
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	result, err := ft.Call(ctx, `{"operation":"read","path":"a.txt"}`)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestFileToolListAndDelete(t *testing.T) {
	sessionDir := t.TempDir()
	ft := NewFileTool(sessionDir)
	ctx := context.Background()

	_, err := ft.Call(ctx, `{"operation":"write","path":"a.txt","content":"data"}`)
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	listResult, err := ft.Call(ctx, `{"operation":"list","path":"."}`)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(listResult, "a.txt") {
		t.Errorf("list result %q does not contain 'a.txt'", listResult)
	}

	_, err = ft.Call(ctx, `{"operation":"delete","path":"a.txt"}`)
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	_, readErr := ft.Call(ctx, `{"operation":"read","path":"a.txt"}`)
	if readErr == nil {
		t.Error("expected error reading deleted file, got nil")
	}
}

func TestWorkspaceToolCwd(t *testing.T) {
	sessionDir := t.TempDir()
	wt := NewWorkspaceTool(sessionDir)
	ctx := context.Background()

	result, err := wt.Call(ctx, `{"operation":"cwd"}`)
	if err != nil {
		t.Fatalf("cwd failed: %v", err)
	}
	if !strings.HasSuffix(result, "workspace") {
		t.Errorf("expected path ending in 'workspace', got %q", result)
	}
}

func TestShellToolEcho(t *testing.T) {
	sessionDir := t.TempDir()
	st := NewShellTool(sessionDir)
	ctx := context.Background()

	result, err := st.Call(ctx, `{"command":"echo hi"}`)
	if err != nil {
		t.Fatalf("shell echo failed: %v", err)
	}
	if !strings.Contains(result, "hi") {
		t.Errorf("expected output to contain 'hi', got %q", result)
	}
}

func TestGrepToolMatch(t *testing.T) {
	sessionDir := t.TempDir()
	ft := NewFileTool(sessionDir)
	gt := NewGrepTool(sessionDir)
	ctx := context.Background()

	_, err := ft.Call(ctx, `{"operation":"write","path":"search.txt","content":"this contains needle in it"}`)
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	result, err := gt.Call(ctx, `{"pattern":"needle","path":"."}`)
	if err != nil {
		t.Fatalf("grep failed: %v", err)
	}
	if !strings.Contains(result, "needle") {
		t.Errorf("grep result %q does not contain 'needle'", result)
	}
}

func TestToolParametersSchema(t *testing.T) {
	dir := t.TempDir()
	tools := []Tool{
		NewFileTool(dir),
		NewShellTool(dir),
		NewGrepTool(dir),
		NewGlobTool(dir),
		NewWorkspaceTool(dir),
	}

	for _, tool := range tools {
		params := tool.Parameters()
		if params["type"] != "object" {
			t.Errorf("%s: expected type 'object', got %v", tool.Name(), params["type"])
		}
		if params["properties"] == nil {
			t.Errorf("%s: expected non-nil properties", tool.Name())
		}
	}
}

func TestGlobToolFind(t *testing.T) {
	sessionDir := t.TempDir()
	ft := NewFileTool(sessionDir)
	gt := NewGlobTool(sessionDir)
	ctx := context.Background()

	_, err := ft.Call(ctx, `{"operation":"write","path":"foo.go","content":"package main"}`)
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	result, err := gt.Call(ctx, `{"pattern":"*.go","path":"."}`)
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}
	if !strings.Contains(result, "foo.go") {
		t.Errorf("glob result %q does not contain 'foo.go'", result)
	}
}
