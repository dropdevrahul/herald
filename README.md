# Herald

**Herald** is a general-purpose Go framework for composing LLM workflows and building LLM agents. It provides a lightweight alternative to Python-based frameworks like LangChain/LangGraph.

## Status

**Herald is in active development.** The API surface is established and functional. Contributions and feedback welcome.

## Features

- **Generic Agent Runtime** - Provider-agnostic multi-turn tool-calling loop with stop conditions
- **Memory** - Pluggable conversation memory (buffer / sliding window) for cross-run continuity
- **Human-in-the-Loop** - Approve, deny, or rewrite tool calls before they execute
- **Sub-Agents** - Wrap any agent as a tool to compose multi-agent systems
- **Observability** - Lifecycle hooks for turns, model responses, and tool calls
- **Simple Workflows** - Sequential chaining, orchestration, and parallel execution
- **Graph-based Workflows** - Directed graphs with nodes, edges, and conditional routing
- **Tool Calling** - Define and execute tools/functions during workflow execution
- **Streaming Support** - Real-time token streaming for all workflows
- **Multi-Provider Support** - OpenAI, Groq, Anthropic, Gemini compatible

## Requirements

- Go **1.24+**

## Installation

```bash
go get github.com/dropdevrahul/herald
```

## Quick Start

### Simple Workflows

```go
import (
    "github.com/dropdevrahul/herald/src/model"
    "github.com/dropdevrahul/herald/src/model/openai"
    "github.com/dropdevrahul/herald/src/worklows"
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

(Workflow tools also implement `Parameters() map[string]any`, returning a JSON-schema description of their arguments.)

### Agents

The `agents` package provides a generic, provider-agnostic agent runtime. It loops
against any `model.Model`, dispatching tool calls until the model stops requesting
them, a stop condition fires, or the turn budget is exhausted.

```go
import "github.com/dropdevrahul/herald/src/agents"

agent := agents.NewAgent(m, agents.AgentConfig{
    SystemPrompt: "You are a helpful assistant.",
    Tools:        []workflows.Tool{&MyTool{}},
    MaxTurns:     5,   // default 5
    Temperature:  0.7, // default 0.7
})

answer, _ := agent.Run(ctx, "What is 15 + 27?")
```

`AgentConfig` fields are all optional beyond `Tools`/`SystemPrompt`:

- **`Memory`** — seed the run from prior messages and persist new turns back, for
  continuity across `Run` calls (see below).
- **`Approver`** — a human-in-the-loop gate consulted before each tool call.
- **`Stop`** — `func(turn int, lastContent string) bool` to end a run early.
- **`Hooks`** — observe lifecycle events.

The coding agents (`NewCodingAgentWithTools`) are thin presets over this runtime.

### Memory

```go
import "github.com/dropdevrahul/herald/src/memory"

agent := agents.NewAgent(m, agents.AgentConfig{
    SystemPrompt: "You are a helpful assistant.",
    Memory:       memory.NewBufferMemory(),     // retains every message
    // or memory.NewWindowMemory(10)            // keeps the last N non-system messages
})

agent.Run(ctx, "My name is Ada.")
agent.Run(ctx, "What is my name?")  // remembers the first turn
```

### Human-in-the-Loop

```go
agent := agents.NewAgent(m, agents.AgentConfig{
    Tools: []workflows.Tool{&ShellTool{}},
    Approver: func(ctx context.Context, call model.ToolCall) (agents.ApprovalDecision, error) {
        if call.Function.Name == "shell" {
            return agents.ApprovalDecision{Approved: false, Reason: "shell disabled"}, nil
        }
        return agents.ApprovalDecision{Approved: true}, nil
    },
})
```

Return `Approved: false` to deny (the reason is fed back to the model),
`Approved: true, Args: "..."` to rewrite the arguments, or an error to abort the run.

### Sub-Agents

Wrap an agent as a tool so a parent agent can delegate to it:

```go
researcher := agents.NewAgent(m, agents.AgentConfig{SystemPrompt: "You research topics."})
tool := agents.NewAgentTool("researcher", "Delegate research tasks", researcher)

coordinator := agents.NewAgent(m, agents.AgentConfig{
    SystemPrompt: "You coordinate work by delegating to sub-agents.",
    Tools:        []workflows.Tool{tool},
})
```

### Observability

```go
agent := agents.NewAgent(m, agents.AgentConfig{
    Tools: []workflows.Tool{&MyTool{}},
    Hooks: []agents.Hook{
        func(ctx context.Context, ev agents.Event) {
            log.Printf("[%s] turn=%d tool=%s", ev.Type, ev.Turn, ev.Tool)
        },
    },
})
```

Events are emitted with `Type` of `turn_start`, `model_response`, `tool_start`,
`tool_end`, and `finish`.

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
│   │   ├── openai/      # OpenAI/Groq/Azure provider
│   │   ├── anthropic/   # Anthropic provider
│   │   └── gemini/      # Google Gemini provider
│   ├── memory/          # Conversation memory (buffer / window)
│   ├── agents/          # Generic agent runtime + tools
│   └── worklows/        # Workflow implementations
│       ├── workflows.go # Simple workflows
│       └── graph.go     # Graph-based workflows
├── go.mod
└── README.md
```

## License

MIT License - see [LICENSE](LICENSE)