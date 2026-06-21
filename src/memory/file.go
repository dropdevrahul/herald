package memory

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/dropdevrahul/herald/src/model"
)

// FileMemory is a file-backed Memory implementation that persists messages to
// disk as a JSON array and reloads them on construction, so conversation state
// survives restarts.
type FileMemory struct {
	path     string
	messages []model.Message
}

// compile-time check
var _ Memory = (*FileMemory)(nil)

// NewFileMemory creates a FileMemory backed by path. If a file already exists
// at path its contents are loaded; a missing file is treated as an empty store.
// An error is returned only when an existing file cannot be read or parsed.
func NewFileMemory(path string) (*FileMemory, error) {
	fm := &FileMemory{path: path}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fm, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, &fm.messages); err != nil {
		return nil, err
	}
	return fm, nil
}

// Add appends msg and persists the updated slice to disk.
func (m *FileMemory) Add(msg model.Message) {
	m.messages = append(m.messages, msg)
	m.persist()
}

// Messages returns a freshly-allocated copy of the stored messages; the
// internal backing array is never exposed.
func (m *FileMemory) Messages() []model.Message {
	out := make([]model.Message, len(m.messages))
	copy(out, m.messages)
	return out
}

// Clear resets the message store and persists the empty state.
func (m *FileMemory) Clear() {
	m.messages = nil
	m.persist()
}

// persist writes the messages slice as JSON to m.path, creating parent
// directories as needed. Write errors are ignored (best-effort durability).
// ponytail: write errors are silently discarded here; upgrade to a stored
// error or returned error if durability guarantees are required.
func (m *FileMemory) persist() {
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return
	}
	data, err := json.Marshal(m.messages)
	if err != nil {
		return
	}
	_ = os.WriteFile(m.path, data, 0o644)
}
