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
- [ ] File-backed persistent memory
- [ ] Retry/resilience wrapper on `model.Model`
- [ ] Subgraphs
- [ ] More examples

## Dependencies

- `github.com/openai/openai-go/v3` - OpenAI client library