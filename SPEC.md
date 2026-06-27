# Herald - LLM Workflow Framework Specification

## Project Overview

**Herald** is a general-purpose Go framework for composing LLM workflows and building LLM agents. It provides a lightweight alternative to Python-based frameworks like LangChain/LangGraph.

## Status

- **Active Development**
- **Go 1.24+** required

## Core Architecture

### 1. Model Layer (`src/model/`)

Abstract `Model` interface for LLM providers:

```go
type Model interface {
    Generate(ctx context.Context, messages []Message, opts *ModelOptions) (*Response, error)
    Stream(ctx context.Context, messages []Message, opts *ModelOptions) <-chan StreamResult
}
```

**Types:**
- `Message` - `{Role, Content, ToolCallID, Name}` (system, user, assistant, tool)
- `ModelOptions` - `{Model, Temperature, MaxTokens, Tools, ToolChoice}`
- `Response` - `{Content, Usage, ToolCalls}`
- `StreamResult` - idiomatic single-channel streaming with `{Content, Delta, Usage, Err, ToolCalls}`
- `ToolCall` - function call request from model

**Helpers:**
- `GenerateJSON(ctx, m, messages, opts, out any) error` — calls `m.Generate`, extracts the first JSON object or array from the response (tolerating Markdown fences and prose), and unmarshals into `out`. Returns an error when no JSON is found or unmarshalling fails.
- `GenerateJSONStream(ctx, m, messages, opts, out any, onDelta func(string)) error` — streaming counterpart to `GenerateJSON`; streams the response to `onDelta` for live display, then extracts and unmarshals the full JSON once the stream completes (partial JSON cannot be decoded mid-stream).
- `NewRetryModel(m Model, maxRetries int) *RetryModel` — wraps any `Model` to retry failed calls with exponential backoff (`100ms * 2^attempt`); `maxRetries` is the number of extra attempts after the first. `Generate` retries on any error; `Stream` only restarts before the first delta/content is emitted (once output is forwarded, errors propagate unchanged — no mid-stream resume).

**Implementations:**
- `src/model/openai/openai.go` - OpenAI-compatible (Groq, Azure, custom endpoints)
- `src/model/anthropic/anthropic.go` - Anthropic API (net/http)
- `src/model/gemini/gemini.go` - Google Gemini API

### 2. Workflow Layer (`src/worklows/`)

#### Simple Workflows (`workflows.go`)

**Node** - Basic unit with system prompt:
```go
type Node struct {
    Name   string
    Prompt string
}
```

**Tool** - Executable functions (the canonical tool type across the codebase):
```go
type Tool interface {
    Name() string
    Description() string
    Parameters() map[string]any  // JSON-schema of the tool's arguments
    Call(ctx context.Context, args string) (string, error)
}
```

**Workflow Types:**

1. **ChainingWorkflow** - Sequential execution with tool support
   ```go
   func NewChainingWorkflow(m model.Model, nodes []Node, tools ...Tool) StreamingWorkflowI
   ```

2. **OrchestratorWorkflow** - Multi-node coordination with parallelism
   ```go
   func NewOrchestratorWorkflow(m model.Model, nodes []Node, aggregator AggregatorFunc) WorkflowI
   // Set .Parallel = true for concurrent execution
   ```

3. **ParallelWorkflow** - Run all nodes concurrently

4. **StreamingWorkflowI** - Streaming interface for all workflows
   ```go
   type StreamingWorkflowI interface {
       Run(ctx, input) (string, error)
       RunStream(ctx, input, handler) error
   }
   ```

#### Graph Workflows (`graph.go`)

```go
type Graph struct {
    Nodes       map[string]*GraphNode
    Edges       []Edge
    Conditional map[string]*ConditionalGraphNode
    Start       string
}
```

**Features:**
- Fluent API for building graphs
- Compile and execute model
- Conditional routing with LLM
- Loop support with max iterations
- Streaming execution
- Durable thread checkpointing and resume (see below)

**Durable checkpointing:** A `CompiledGraph` can be given a `Checkpointer`
(via `WithCheckpointer`) that saves a `Checkpoint{ThreadID, Node, Iteration,
State}` after every node completes. Calling `RunThread(ctx, threadID, input)`
instead of `Run` serialises the graph state to the checkpointer after each
node; a subsequent call with the same `threadID` deserialises the stored state
and resumes from the last completed node rather than restarting. Two
implementations are provided: `NewMemoryCheckpointer()` for in-process use
and `NewFileCheckpointer(dir)` for persistence across process restarts.
The `Checkpointer` interface can be implemented against any backing store.

### 3. Memory Layer (`src/memory/`)

Provider-agnostic conversation store seeded into agent runs and persisted back
across separate runs:

```go
type Memory interface {
    Add(msg model.Message)
    Messages() []model.Message
    Clear()
}
```

- `BufferMemory` - retains every message (unbounded)
- `WindowMemory` - keeps the last N non-system messages, always preserving system messages
- `FileMemory` (`NewFileMemory(path string) (*FileMemory, error)`) - disk-backed memory that persists messages as a JSON array and reloads them on construction, enabling conversation state to survive restarts

### 4. Agent Layer (`src/agents/`)

`Agent` is a generic, provider-agnostic runtime that drives a multi-turn
tool-calling loop against any `model.Model` until the model stops requesting
tools, a stop condition fires, or the turn budget is exhausted.

```go
type AgentConfig struct {
    SystemPrompt string
    Tools        []workflows.Tool
    MaxTurns     int          // default 5
    Temperature  float64      // default 0.7
    Stop         StopFunc     // end early: func(turn int, lastContent string) bool
    Memory       memory.Memory
    Approver     ToolApprover // human-in-the-loop gate
    Hooks        []Hook       // lifecycle observability
}

func NewAgent(m model.Model, cfg AgentConfig) *Agent
```

**Capabilities:**
- **Human-in-the-loop** — `ToolApprover` returns an `ApprovalDecision{Approved, Reason, Args}`
  before each tool call: approve, deny (reason fed back to the model), or rewrite arguments.
- **Sub-agents** — `NewAgentTool(name, desc, *Agent)` adapts an `Agent` as a `workflows.Tool`,
  letting a parent agent delegate to sub-agents.
- **Observability** — `Hook func(ctx, Event)` receives `turn_start`, `model_response`,
  `tool_start`, `tool_end`, and `finish` events.

The coding agents (`NewCodingAgentWithTools`, `ReActCodingAgent`) and the concrete
filesystem/shell tools (`FileTool`, `ShellTool`, `GrepTool`, `GlobTool`, `WorkspaceTool`)
are thin presets over this runtime.

**Token usage accounting:** `Agent.RunResult(ctx, input) (AgentResult, error)`
returns an `AgentResult{Content string, Usage model.Usage, Turns int}` in which
`Usage` accumulates `PromptTokens`, `CompletionTokens`, and `TotalTokens` across
every streaming turn so callers can track cost without instrumenting individual
turns. `Run` and `RunStream` retain their existing signatures unchanged.

**Functional tool adapter:**
`NewFuncTool(name, description string, parameters map[string]any, fn func(ctx context.Context, args string) (string, error)) *FuncTool`
adapts a plain function to `workflows.Tool` so tools can be defined without writing a four-method struct. Passing `nil` for parameters yields a minimal valid JSON-schema object.

**Tool resilience:** The agent runtime always recovers from tool panics — if a tool's `Call` method panics the panic value is caught and the model receives `Error: tool panicked: <value>` as the tool result, keeping the run alive. An optional `AgentConfig.ToolTimeout time.Duration` field (zero means no timeout) applies a per-call `context.WithTimeout` around each tool invocation; when the deadline expires the model receives `Error: context deadline exceeded` and the loop continues to the next turn. Together these two mechanisms prevent a single misbehaving or slow tool from crashing or deadlocking an agent run.

**Durable interrupt/resume (HITL):** `AgentConfig.Checkpointer workflows.Checkpointer` and `AgentConfig.InterruptBefore func(call model.ToolCall) bool` enable durable human-in-the-loop approval. When `InterruptBefore` returns `true` for a pending tool call, `Agent.RunThread(ctx, threadID, input) (AgentResult, error)` persists the full run state via the Checkpointer and returns an `AgentResult` with a non-nil `Interrupt *Interrupt` field (carrying `ThreadID` and the pending `ToolCall`) — the tool is not executed yet. `Agent.Resume(ctx, threadID, decision ApprovalDecision) (AgentResult, error)` reloads the persisted state for `threadID` and continues: `Approved: false` denies the call (reason fed back to the model), while `Args != ""` rewrites the call arguments before executing. Because all state is stored by the Checkpointer, `Resume` can run in a completely separate process after the original `RunThread` call exits, making the pattern suitable for asynchronous, out-of-band approval workflows.

## Key Features

- Idiomatic Go channel patterns (single channel with result struct)
- Sequential chaining of prompt-based nodes
- Real-time streaming token support
- Tool calling with automatic execution
- Multi-provider support (OpenAI, Anthropic, Gemini)
- Graph-based workflows with conditional routing

## Project Structure

```
herald/
├── cmd/herald/          # Bubble Tea TUI coding agent
├── internal/
│   ├── config/          # Provider config + API keys
│   └── session/         # Persistent sessions (~/.herald)
├── src/
│   ├── model/           # Model interfaces & implementations
│   │   ├── model.go     # Core interfaces
│   │   ├── openai/      # OpenAI/Groq provider
│   │   ├── anthropic/   # Anthropic provider
│   │   └── gemini/      # Google Gemini provider
│   ├── memory/          # Conversation memory (buffer / window)
│   ├── agents/          # Generic agent runtime + tools
│   └── worklows/        # Workflow implementations
│       ├── workflows.go # Simple workflows
│       └── graph.go     # Graph-based workflows
├── go.mod
├── README.md
└── LICENSE
```

## Usage Examples

### Simple Workflow with Tools

```go
type CalculatorTool struct{}

func (t *CalculatorTool) Name() string        { return "calculator" }
func (t *CalculatorTool) Description() string { return "Performs arithmetic" }
func (t *CalculatorTool) Call(ctx, args) (string, error) { ... }

client := openai.NewClient(apiKey, "https://api.groq.com/openai/v1")
m := openai.NewOpenAIModel(model.ModelOptions{Model: "llama-3.3-70b-versatile"}, client)

node := workflows.Node{Prompt: "You are a helpful assistant. Use tools when needed."}
wf := workflows.NewChainingWorkflow(m, []workflows.Node{node}, &CalculatorTool{})
output, _ := wf.Run(ctx, "What is 15 + 27?")
```

### Graph Workflow with Loop

```go
g := workflows.NewGraph(m).
    AddNode("chat", "You are a helpful assistant.", func(ctx, state) (any, error) { ... }).
    AddEdge("chat", "continue").
    AddConditionalNode("continue", func(ctx, state) string {
        if strings.Contains(state.(string), "bye") { return "" }
        return "chat"  // loop back
    }).
    SetStart("chat")

compiled, _ := g.Compile()
compiled.MaxIterations = 5
result, _ := compiled.Run(ctx, "Hello!")
```

### Streaming

```go
handler := func(result model.StreamResult) error {
    fmt.Print(result.Delta)
    return nil
}
wf.RunStream(ctx, "Hello!", handler)
```

## Roadmap

- [x] Tool calling support
- [x] Graph-based workflows
- [x] Streaming support
- [x] Generic agent runtime (multi-turn tool loop, stop conditions)
- [x] Memory/state management (buffer / window)
- [x] Human-in-the-loop (approve / deny / edit tool calls)
- [x] Sub-agents (agent-as-tool composition)
- [x] Observability hooks
- [x] Proper Anthropic client implementation
- [x] File-backed persistent memory (`NewFileMemory`)
- [x] Retry/resilience wrapper on `model.Model` (`NewRetryModel`)
- [ ] Subgraphs
- [ ] More examples

## Dependencies

- `github.com/openai/openai-go/v3` - OpenAI client library