package memory

import "github.com/dropdevrahul/herald/src/model"

// Memory is a provider-agnostic store of conversation messages that an agent
// can seed from and persist turns back into across separate runs.
type Memory interface {
	Add(msg model.Message)
	Messages() []model.Message
	Clear()
}

// BufferMemory is an unbounded memory that retains every message added.
type BufferMemory struct {
	messages []model.Message
}

func NewBufferMemory() *BufferMemory {
	return &BufferMemory{}
}

func (m *BufferMemory) Add(msg model.Message) {
	m.messages = append(m.messages, msg)
}

// Messages returns a freshly-allocated copy of the retained messages; the
// internal backing array is never exposed.
func (m *BufferMemory) Messages() []model.Message {
	out := make([]model.Message, len(m.messages))
	copy(out, m.messages)
	return out
}

func (m *BufferMemory) Clear() {
	m.messages = nil
}

// WindowMemory retains only the most recent max non-system messages while
// always preserving every system message. If max <= 0, no windowing is applied
// and all non-system messages are retained.
type WindowMemory struct {
	max      int
	messages []model.Message
}

func NewWindowMemory(max int) *WindowMemory {
	return &WindowMemory{max: max}
}

func (m *WindowMemory) Add(msg model.Message) {
	m.messages = append(m.messages, msg)
}

// Messages returns a freshly-allocated slice containing all system messages in
// insertion order followed by the most recent max non-system messages in
// insertion order.
func (m *WindowMemory) Messages() []model.Message {
	var system []model.Message
	var nonSystem []model.Message
	for _, msg := range m.messages {
		if msg.Role == model.RoleSystem {
			system = append(system, msg)
		} else {
			nonSystem = append(nonSystem, msg)
		}
	}

	if m.max > 0 && len(nonSystem) > m.max {
		nonSystem = nonSystem[len(nonSystem)-m.max:]
	}

	out := make([]model.Message, 0, len(system)+len(nonSystem))
	out = append(out, system...)
	out = append(out, nonSystem...)
	return out
}

func (m *WindowMemory) Clear() {
	m.messages = nil
}
