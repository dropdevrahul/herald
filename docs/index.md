# Herald

**Herald** is a general-purpose Go framework for composing LLM workflows and building LLM agents. It provides a lightweight, idiomatic alternative to Python-based frameworks like LangChain/LangGraph — small, provider-agnostic, and built around plain Go interfaces and channels.

!!! note "Requirements"
    Herald requires **Go 1.24+**.

## Installation

```bash
go get github.com/dropdevrahul/herald
```

## Quick Start

A minimal sequential (chaining) workflow: construct a model, define a node with a
system prompt, and run it.

```go
package main

import (
    "context"
    "fmt"

    "github.com/dropdevrahul/herald/src/model"
    "github.com/dropdevrahul/herald/src/model/openai"
    "github.com/dropdevrahul/herald/src/worklows"
)

func main() {
    ctx := context.Background()

    // OpenAI-compatible client (here pointed at Groq).
    client := openai.NewClient(apiKey, "https://api.groq.com/openai/v1")
    m := openai.NewOpenAIModel(model.ModelOptions{Model: "llama-3.3-70b-versatile"}, client)

    node := workflows.Node{
        Name:   "assistant",
        Prompt: "You are a helpful assistant.",
    }

    wf := workflows.NewChainingWorkflow(m, []workflows.Node{node})
    output, err := wf.Run(ctx, "Hello!")
    if err != nil {
        panic(err)
    }
    fmt.Println(output)
}
```

## Features

- **Generic Agent Runtime** — provider-agnostic multi-turn tool-calling loop with stop conditions.
- **Memory** — pluggable conversation memory (buffer / sliding window) for cross-run continuity.
- **Human-in-the-Loop** — approve, deny, or rewrite tool calls before they execute.
- **Sub-Agents** — wrap any agent as a tool to compose multi-agent systems.
- **Observability** — lifecycle hooks for turns, model responses, and tool calls.
- **Workflows** — sequential chaining, orchestration, parallel execution, and directed graphs with conditional routing.
- **Streaming Support** — real-time token streaming for all workflows.
- **Multi-Provider Support** — OpenAI, Groq, Azure, Anthropic, and Gemini compatible.

## Where to next

- [Agents](agents.md) — the generic agent runtime, memory, human-in-the-loop, sub-agents, and observability.
- [Workflows](workflows.md) — chaining, orchestrator, parallel, and graph workflows.
- [Providers](providers.md) — the model layer and supported LLM providers.
