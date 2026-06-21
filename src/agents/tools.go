package agents

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dropdevrahul/herald/src/worklows"
)

type Tool = workflows.Tool

type FileTool struct {
	sessionDir string
}

func NewFileTool(sessionDir string) *FileTool {
	return &FileTool{sessionDir: sessionDir}
}

func (t *FileTool) Name() string { return "file_tool" }
func (t *FileTool) Description() string {
	return `Perform file operations. Args JSON: {"operation": "read"|"write"|"edit"|"list"|"delete", "path": "/full/path", "content": "text", "lines": [start, end]}`
}

func (t *FileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type": "string",
				"enum": []string{"read", "write", "edit", "list", "delete", "exists"},
			},
			"path":    map[string]any{"type": "string"},
			"content": map[string]any{"type": "string"},
			"lines": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "integer"},
			},
		},
		"required": []string{"operation", "path"},
	}
}

type FileOperation struct {
	Operation string `json:"operation"`
	Path      string `json:"path"`
	Content   string `json:"content,omitempty"`
	Lines     []int  `json:"lines,omitempty"`
}

func (t *FileTool) Call(ctx context.Context, args string) (string, error) {
	var op FileOperation
	if err := json.Unmarshal([]byte(args), &op); err != nil {
		return "", err
	}

	relPath := op.Path
	if !filepath.IsAbs(relPath) {
		relPath = filepath.Join(t.sessionDir, "workspace", relPath)
	}

	switch op.Operation {
	case "read":
		return t.readFile(relPath)
	case "write":
		return t.writeFile(relPath, op.Content)
	case "edit":
		return t.editFile(relPath, op.Lines, op.Content)
	case "list":
		return t.listFiles(relPath)
	case "delete":
		return t.deleteFile(relPath)
	case "exists":
		return t.fileExists(relPath)
	default:
		return "Unknown operation: " + op.Operation, nil
	}
}

func (t *FileTool) readFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (t *FileTool) writeFile(path string, content string) (string, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}
	return "File written: " + path, nil
}

func (t *FileTool) editFile(path string, lines []int, content string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	allLines := strings.Split(string(data), "\n")
	if len(lines) != 2 {
		return "", nil
	}

	start, end := lines[0]-1, lines[1]
	if start < 0 {
		start = 0
	}
	if end > len(allLines) {
		end = len(allLines)
	}

	result := append([]string{}, allLines[:start]...)
	result = append(result, content)
	result = append(result, allLines[end:]...)

	if err := os.WriteFile(path, []byte(strings.Join(result, "\n")), 0644); err != nil {
		return "", err
	}

	return "File edited: " + path, nil
}

func (t *FileTool) listFiles(path string) (string, error) {
	if path == "" {
		path = "."
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}

	var result []string
	for _, e := range entries {
		if e.IsDir() {
			result = append(result, e.Name()+"/")
		} else {
			result = append(result, e.Name())
		}
	}

	return strings.Join(result, "\n"), nil
}

func (t *FileTool) deleteFile(path string) (string, error) {
	if err := os.Remove(path); err != nil {
		return "", err
	}
	return "File deleted: " + path, nil
}

func (t *FileTool) fileExists(path string) (string, error) {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return "false", nil
	}
	return "true", nil
}

type ShellTool struct {
	sessionDir string
}

func NewShellTool(sessionDir string) *ShellTool {
	return &ShellTool{sessionDir: sessionDir}
}

func (t *ShellTool) Name() string { return "shell" }
func (t *ShellTool) Description() string {
	return `Run shell commands. Args JSON: {"command": "ls -la", "cwd": "optional/cwd"}`
}

func (t *ShellTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{"type": "string"},
			"cwd":     map[string]any{"type": "string"},
		},
		"required": []string{"command"},
	}
}

type ShellOp struct {
	Command string `json:"command"`
	Cwd     string `json:"cwd,omitempty"`
}

func (t *ShellTool) Call(ctx context.Context, args string) (string, error) {
	var op ShellOp
	if err := json.Unmarshal([]byte(args), &op); err != nil {
		return "", err
	}

	workingDir := t.sessionDir
	if op.Cwd != "" {
		if filepath.IsAbs(op.Cwd) {
			workingDir = op.Cwd
		} else {
			workingDir = filepath.Join(t.sessionDir, "workspace", op.Cwd)
		}
	}

	cmd := exec.Command("sh", "-c", op.Command)
	cmd.Dir = workingDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	return string(output), nil
}

type GrepTool struct {
	sessionDir string
}

func NewGrepTool(sessionDir string) *GrepTool {
	return &GrepTool{sessionDir: sessionDir}
}

func (t *GrepTool) Name() string { return "grep" }
func (t *GrepTool) Description() string {
	return `Search files for pattern. Args JSON: {"pattern": "search", "path": ".", "extensions": ["go", "py"]}`
}

func (t *GrepTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{"type": "string"},
			"path":    map[string]any{"type": "string"},
			"extensions": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
		"required": []string{"pattern"},
	}
}

type GrepOp struct {
	Pattern    string   `json:"pattern"`
	Path       string   `json:"path"`
	Extensions []string `json:"extensions,omitempty"`
}

func (t *GrepTool) Call(ctx context.Context, args string) (string, error) {
	var op GrepOp
	if err := json.Unmarshal([]byte(args), &op); err != nil {
		return "", err
	}

	searchPath := op.Path
	if !filepath.IsAbs(searchPath) {
		searchPath = filepath.Join(t.sessionDir, "workspace", searchPath)
	}

	exts := op.Extensions
	if len(exts) == 0 {
		exts = []string{""}
	}

	var results []string
	for _, ext := range exts {
		pattern := op.Pattern
		if ext != "" {
			pattern = "--include=*." + ext + " " + pattern
		}
		cmd := exec.Command("grep", "-rn", pattern, searchPath)
		output, _ := cmd.CombinedOutput()
		if len(output) > 0 {
			results = append(results, string(output))
		}
	}

	if len(results) == 0 {
		return "No matches found", nil
	}
	return strings.Join(results, "\n"), nil
}

type GlobTool struct {
	sessionDir string
}

func NewGlobTool(sessionDir string) *GlobTool {
	return &GlobTool{sessionDir: sessionDir}
}

func (t *GlobTool) Name() string { return "glob" }
func (t *GlobTool) Description() string {
	return `Find files by pattern. Args JSON: {"pattern": "**/*.go", "path": "."}`
}

func (t *GlobTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{"type": "string"},
			"path":    map[string]any{"type": "string"},
		},
		"required": []string{"pattern"},
	}
}

type GlobOp struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

func (t *GlobTool) Call(ctx context.Context, args string) (string, error) {
	var op GlobOp
	if err := json.Unmarshal([]byte(args), &op); err != nil {
		return "", err
	}

	searchPath := op.Path
	if !filepath.IsAbs(searchPath) {
		searchPath = filepath.Join(t.sessionDir, "workspace", searchPath)
	}

	cmd := exec.Command("find", searchPath, "-name", op.Pattern)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var files []string
	for _, f := range lines {
		if f != "" {
			files = append(files, f)
		}
	}

	if len(files) == 0 {
		return "No files found", nil
	}
	return strings.Join(files, "\n"), nil
}

type WorkspaceTool struct {
	sessionDir string
}

func NewWorkspaceTool(sessionDir string) *WorkspaceTool {
	return &WorkspaceTool{sessionDir: sessionDir}
}

func (t *WorkspaceTool) Name() string { return "workspace" }
func (t *WorkspaceTool) Description() string {
	return `Manage workspace. Args JSON: {"operation": "cwd"|"mkdir"|"exists", "path": "file/or/dir"}`
}

func (t *WorkspaceTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type": "string",
				"enum": []string{"cwd", "mkdir", "exists"},
			},
			"path": map[string]any{"type": "string"},
		},
		"required": []string{"operation"},
	}
}

type WorkspaceOp struct {
	Operation string `json:"operation"`
	Path      string `json:"path"`
}

func (t *WorkspaceTool) Call(ctx context.Context, args string) (string, error) {
	var op WorkspaceOp
	if err := json.Unmarshal([]byte(args), &op); err != nil {
		return "", err
	}

	workspaceDir := filepath.Join(t.sessionDir, "workspace")
	os.MkdirAll(workspaceDir, 0755)

	switch op.Operation {
	case "cwd":
		return workspaceDir, nil
	case "mkdir":
		path := filepath.Join(workspaceDir, op.Path)
		os.MkdirAll(path, 0755)
		return "Directory created: " + path, nil
	case "exists":
		path := filepath.Join(workspaceDir, op.Path)
		_, err := os.Stat(path)
		if os.IsNotExist(err) {
			return "false", nil
		}
		return "true", nil
	default:
		return "", nil
	}
}
