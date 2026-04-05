# Herald - LLM Workflow Framework Specification

## Project Overview

**Herald** is a general-purpose Go framework for composing LLM workflows and building LLM agents. It provides a lightweight alternative to Python-based frameworks like LangChain/LangGraph.

## Status

- **Active Development**
- **Go 1.22+** required

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
- `src/model/anthropic/anthropic.go` - Anthropic API (placeholder)
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

**Tool** - Executable functions:
```go
type Tool interface {
    Name() string
    Description() string
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
├── src/
│   ├── model/           # Model interfaces & implementations
│   │   ├── model.go    # Core interfaces
│   │   ├── openai/     # OpenAI/Groq provider
│   │   ├── anthropic/ # Anthropic provider
│   │   └── gemini/     # Google Gemini provider
│   └── worklows/       # Workflow implementations
│       ├── workflows.go  # Simple workflows
│       └── graph.go      # Graph-based workflows
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
- [ ] Memory/state management
- [ ] Proper Anthropic client implementation
- [ ] Subgraphs
- [ ] More examples

## Dependencies

- `github.com/openai/openai-go/v3` - OpenAI client library