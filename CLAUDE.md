# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Herald is a general-purpose Go framework for composing LLM workflows and building LLM agents — a lightweight alternative to LangChain/LangGraph. It is both an importable library (`src/`) and a terminal app (`cmd/herald`), a Bubble Tea TUI coding agent built on the library.

Module path: `github.com/dropdevrahul/herald`. Go 1.24.

## Commands

```bash
go build ./...                       # build everything
go build -o herald ./cmd/herald      # build the TUI binary
go test ./...                        # run all tests
go test ./src/agents/                # test one package
go test -run ExampleReActCodingAgent ./src/agents/   # run a single test/example
go vet ./...
```

The TUI (`./herald`) requires an API key in the environment: `GROQ_API_KEY`, `OPENAI_API_KEY`, or `ANTHROPIC_API_KEY`. The active provider defaults to `groq` (see `internal/config`).

## Architecture

Three layers, bottom to top:

**Model layer (`src/model/`)** — `model.go` defines the provider-agnostic `Model` interface (`Generate` + `Stream`) plus all shared types: `Message`, `ModelOptions`, `Response`, `ToolCall`, and `StreamResult`. Streaming uses the idiomatic single-channel-of-result-struct pattern (`<-chan StreamResult`), not separate error/data channels. Provider implementations live in subpackages: `openai/` (also serves Groq, Azure, and any OpenAI-compatible endpoint via `NewClient(apiKey, baseURL)`), `gemini/`, and `anthropic/` (placeholder — not fully implemented). When adding a provider, implement `model.Model` and convert to/from the shared types.

**Workflow layer (`src/worklows/`)** — note the directory is misspelled `worklows`; the package is `workflows`. Two files:
- `workflows.go` — node-based workflows. A `Node` is `{Name, Prompt}`. Workflow constructors: `NewChainingWorkflow` (sequential, supports tools), `NewOrchestratorWorkflow` (multi-node, set `.Parallel = true` for concurrency), and parallel execution. The `Tool` interface (`Name`/`Description`/`Call`) is defined here and is the canonical tool type across the codebase.
- `graph.go` — directed-graph workflows. Build with a fluent API (`AddNode`/`AddEdge`/`AddConditionalNode`/`SetStart`), then `Compile()` to get a runnable with `MaxIterations` for loop support. Conditional nodes route by returning the next node's name (empty string ends).

**Agent layer (`src/agents/`)** — `agent.go` implements a ReAct-style coding agent. `CodingAgent` wraps `ReActCodingAgent`, which loops up to `maxIters` times: stream model output, collect `ToolCalls`, dispatch each to a matching tool by name, append results as `RoleTool` messages, repeat until no tool calls remain. `tools.go` provides concrete tools (`FileTool`, `ShellTool`, `GrepTool`, `GlobTool`, `WorkspaceTool`), all scoped to a session workspace directory. `agents.Tool` is a type alias for `workflows.Tool`.

**App layer (`cmd/herald/`)** — Bubble Tea TUI. `main.go` loads config (`internal/config`), resumes or creates a session (`internal/session`), wires up an agent with the workspace-scoped tools, and renders agent output by parsing it into expandable sections. `internal/config` defines providers and reads API keys from env. `internal/session` persists sessions as JSON under `~/.herald/sessions/`, with agent file operations confined to a per-session `workspace/` subdirectory.

## Notes

- README.md and SPEC.md show import paths as `dropdevrahul/herald/...`; the real module path is `github.com/dropdevrahul/herald/...`. SPEC.md is the most detailed design doc but predates the agent/TUI/config/session code.
- Do not "fix" the `worklows` directory spelling casually — it is the import path used throughout.
