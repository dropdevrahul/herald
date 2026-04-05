# Herald

**Herald** is a general-purpose Go framework for composing LLM workflows and building LLM agents. It provides a lightweight alternative to Python-based frameworks like LangChain/LangGraph.

## Status

**Herald is in active development.** The API surface is established and functional. Contributions and feedback welcome.

## Features

- **Simple Workflows** - Sequential chaining, orchestration, and parallel execution
- **Graph-based Workflows** - Directed graphs with nodes, edges, and conditional routing
- **Tool Calling** - Define and execute tools/functions during workflow execution
- **Streaming Support** - Real-time token streaming for all workflows
- **Multi-Provider Support** - OpenAI, Groq, Anthropic, Gemini compatible

## Requirements

- Go **1.22+**

## Installation

```bash
go get dropdevrahul/herald
```

## Quick Start

### Simple Workflows

```go
import (
    "dropdevrahul/herald/src/model"
    "dropdevrahul/herald/src/model/openai"
    "dropdevrahul/herald/src/worklows"
)

client := openai.NewClient(apiKey, "https://api.groq.com/openai/v1")
m := openai.NewOpenAIModel(model.ModelOptions{Model: "llama-3.3-70b-versatile"}, client)

node := workflows.Node{
    Name:   "assistant",
    Prompt: "You are a helpful assistant.",
}

wf := workflows.NewChainingWorkflow(m, []workflows.Node{node})
output, _ := wf.Run(ctx, "Hello!")
```

### Graph Workflows

```go
g := workflows.NewGraph(m).
    AddNode("chat", "You are a helpful assistant.", func(ctx, state) (any, error) { ... }).
    AddEdge("chat", "end").
    SetStart("chat")

compiled, _ := g.Compile()
result, _ := compiled.Run(ctx, "input")
```

### With Tools

```go
type MyTool struct{}

func (t *MyTool) Name() string        { return "my_tool" }
func (t *MyTool) Description() string { return "Does something useful" }
func (t *MyTool) Call(ctx, args) (string, error) { ... }

wf := workflows.NewChainingWorkflow(m, nodes, &MyTool{})
```

## Project Structure

```
herald/
├── src/
│   ├── model/           # Model interfaces & implementations
│   │   ├── model.go    # Core interfaces
│   │   ├── openai/     # OpenAI/Groq provider
│   │   ├── anthropic/ # Anthropic provider (placeholder)
│   │   └── gemini/     # Google Gemini provider
│   └── worklows/       # Workflow implementations
│       ├── workflows.go  # Simple workflows
│       └── graph.go      # Graph-based workflows
├── go.mod
└── README.md
```

## License

MIT License - see [LICENSE](LICENSE)