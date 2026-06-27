package workflows

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Checkpoint captures the durable state of a graph thread at a point in time.
// Node is the NEXT node to run on resume (empty string means the run completed).
// State is the JSON-encoded graph state passed between nodes.
type Checkpoint struct {
	ThreadID  string          `json:"thread_id"`
	Node      string          `json:"node"`
	Iteration int             `json:"iteration"`
	State     json.RawMessage `json:"state"`
}

// Checkpointer is a storage backend for graph thread checkpoints.
// Load returns ok=false when no checkpoint exists for the given threadID.
type Checkpointer interface {
	Save(ctx context.Context, cp Checkpoint) error
	Load(ctx context.Context, threadID string) (Checkpoint, bool, error)
}

// Compile-time assertions.
var _ Checkpointer = (*MemoryCheckpointer)(nil)
var _ Checkpointer = (*FileCheckpointer)(nil)

// MemoryCheckpointer stores checkpoints in memory. Safe for concurrent use.
type MemoryCheckpointer struct {
	mu sync.Mutex
	m  map[string]Checkpoint
}

// NewMemoryCheckpointer returns an initialised in-memory checkpointer.
func NewMemoryCheckpointer() *MemoryCheckpointer {
	return &MemoryCheckpointer{m: make(map[string]Checkpoint)}
}

// Save stores cp keyed by cp.ThreadID.
func (mc *MemoryCheckpointer) Save(_ context.Context, cp Checkpoint) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.m[cp.ThreadID] = cp
	return nil
}

// Load returns the checkpoint for threadID, or (zero, false, nil) if absent.
func (mc *MemoryCheckpointer) Load(_ context.Context, threadID string) (Checkpoint, bool, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	cp, ok := mc.m[threadID]
	return cp, ok, nil
}

// FileCheckpointer persists each checkpoint as a JSON file inside dir.
type FileCheckpointer struct {
	dir string
}

// NewFileCheckpointer returns a file-backed checkpointer that stores files in dir.
func NewFileCheckpointer(dir string) *FileCheckpointer {
	return &FileCheckpointer{dir: dir}
}

// Save marshals cp to JSON and writes it to <dir>/<base(threadID)>.json.
// ponytail: filepath.Base blocks traversal; full sanitization only if threadIDs become untrusted
func (fc *FileCheckpointer) Save(_ context.Context, cp Checkpoint) error {
	if err := os.MkdirAll(fc.dir, 0o755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(cp, "", "  ")
	path := filepath.Join(fc.dir, filepath.Base(cp.ThreadID)+".json")
	return os.WriteFile(path, data, 0o644)
}

// Load reads the checkpoint file for threadID.
// Returns (Checkpoint{}, false, nil) when no file exists, or an error on read/parse failure.
func (fc *FileCheckpointer) Load(_ context.Context, threadID string) (Checkpoint, bool, error) {
	path := filepath.Join(fc.dir, filepath.Base(threadID)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Checkpoint{}, false, nil
		}
		return Checkpoint{}, false, err
	}
	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return Checkpoint{}, false, err
	}
	return cp, true, nil
}
