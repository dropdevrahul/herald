# Herald

**Herald** is a general-purpose framework in Go for composing **LLM workflows** and building **LLM agents**. It takes inspiration from ecosystems like [LangChain](https://github.com/langchain-ai/langchain) and [LangGraph](https://github.com/langchain-ai/langgraph): you define steps (nodes), wire them into graphs or chains, and run them against any **OpenAI-compatible** HTTP API.

The goal is a small, idiomatic Go library you can embed in services, CLIs, or workers—without pulling in a heavy Python stack.

## Status

**Herald is very early stage and just getting started.** The API surface is small, several ideas are still placeholders, and things will evolve quickly. Treat it as an experimental foundation rather than a stable, production-ready stack—at least for now.

## Features

- **Sequential chaining workflows** — Each node runs with its own system prompt; output becomes the next node’s input.
- **Streaming completions** — Nodes aggregate streamed tokens from the chat completions API.
- **Pluggable backends** — Uses [`openai-go`](https://github.com/openai/openai-go) with a configurable base URL, so you can point at OpenAI, Groq, Azure OpenAI, or other compatible providers.
- **Composable design** — `WorkflowI` makes it straightforward to add routing, orchestration, or custom execution strategies as the project grows.

## Requirements

- Go **1.26+**
- An API key for your chosen provider (see [Configuration](#configuration)).

## Installation

```bash
go get dropdevrahul/herald
```

Or clone this repository and use the module path in your own module:

```go
import workflows "dropdevrahul/herald/src/worklows"
import "dropdevrahul/herald/src/client"
```

## Configuration

The bundled client helper expects:

| Variable   | Purpose                                      |
|-----------|-----------------------------------------------|
| `API_KEY` | Secret for the LLM provider                   |

The default client uses the Groq OpenAI-compatible endpoint (`https://api.groq.com/openai/v1`). Swap the base URL and key handling in `src/client/client.go` (or construct `openai.Client` yourself) for other hosts.

## Quick start

Define a chain of `Node`s (each with a `Prompt`), build a `ChainingWorkflow`, and call `Run`:

```go
n1 := workflows.Node{Prompt: "You are a reasoning engine. Break the problem into steps."}
n2 := workflows.Node{Prompt: "Execute the steps and return the result."}
n3 := workflows.Node{Prompt: "From the result, suggest implementation snippets."}

wf := workflows.NewChainingWorkflow(client.NewClient(), []workflows.Node{n1, n2, n3}, "llama-3.3-70b-versatile")

out, err := wf.Run(ctx, "Your user question here")
```

The `experiment/` folder contains a sample HTTP server: a minimal handler on `/chat` (with streaming-friendly headers) that runs the workflow and writes the final output. Run it from the repo root with `go run ./experiment` (requires `API_KEY`). It is experimental scaffolding, not part of the library API.

## Project layout

```
.
├── experiment/          # Sample HTTP server (experimental)
├── src/
│   ├── client/          # OpenAI-compatible client setup
│   └── worklows/        # Workflow types and chaining implementation
├── go.mod
└── README.md
```

## Roadmap

As the project grows beyond this initial sketch, planned directions include richer **routing** and **orchestration** patterns (placeholders exist in code), clearer agent abstractions, and more examples (tools, memory, subgraphs)—aligned with how LangGraph models stateful, graph-based agents.

Contributions, issues, and design discussions are welcome.

## License

This project is licensed under the [MIT License](LICENSE).
