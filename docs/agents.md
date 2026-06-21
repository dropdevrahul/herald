# Agents

The `agents` package provides a generic, provider-agnostic agent runtime. It loops
against any `model.Model`, dispatching tool calls until the model stops requesting
them, a stop condition fires, or the turn budget is exhausted.

## The Agent

An `Agent` is created with `NewAgent(m, cfg)` and driven with `agent.Run(ctx, input)`.

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

### AgentConfig

`AgentConfig` holds the full configuration for an agent. Everything beyond
`SystemPrompt`/`Tools` is optional.

| Field          | Type             | Description                                                     |
| -------------- | ---------------- | --------------------------------------------------------------- |
| `SystemPrompt` | `string`         | System prompt that frames the agent.                            |
| `Tools`        | `[]workflows.Tool` | Tools the agent may call.                                      |
| `MaxTurns`     | `int`            | Turn budget for the tool-calling loop (default `5`).            |
| `Temperature`  | `float64`        | Sampling temperature (default `0.7`).                           |
| `Stop`         | `StopFunc`       | `func(turn int, lastContent string) bool` — end a run early.    |
| `Memory`       | `memory.Memory`  | Seed/persist conversation across runs.                          |
| `Approver`     | `ToolApprover`   | Human-in-the-loop gate consulted before each tool call.         |
| `Hooks`        | `[]Hook`         | Lifecycle observability callbacks.                              |

The coding agents (`NewCodingAgentWithTools`, `ReActCodingAgent`) are thin presets
over this runtime.

## Tools

A tool is the canonical `Tool` interface (defined in the `worklows` package). Note
that it includes a `Parameters() map[string]any` method, which returns a JSON-schema
description of the tool's arguments.

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() map[string]any // JSON-schema of the tool's arguments
    Call(ctx context.Context, args string) (string, error)
}
```

### Functional Tools

You don't need to declare a struct and implement four methods for every tool.
`agents.NewFuncTool` adapts a plain function into a `Tool`:

```go
tool := agents.NewFuncTool(
    "get_weather",
    "Get the current weather for a city",
    map[string]any{
        "type": "object",
        "properties": map[string]any{
            "city": map[string]any{"type": "string"},
        },
        "required": []string{"city"},
    },
    func(ctx context.Context, args string) (string, error) {
        // parse args (JSON) and return a result
        return `{"temp": 21, "sky": "clear"}`, nil
    },
)

agent := agents.NewAgent(m, agents.AgentConfig{Tools: []workflows.Tool{tool}})
```

If `parameters` is `nil`, a minimal `{"type": "object"}` schema is used.

## Memory

Memory lets an agent retain context across separate `Run` calls. Prior messages are
seeded into the run and new turns are persisted back.

```go
import "github.com/dropdevrahul/herald/src/memory"

agent := agents.NewAgent(m, agents.AgentConfig{
    SystemPrompt: "You are a helpful assistant.",
    Memory:       memory.NewBufferMemory(),  // retains every message
    // or memory.NewWindowMemory(10)         // keeps the last N non-system messages
})

agent.Run(ctx, "My name is Ada.")
agent.Run(ctx, "What is my name?") // remembers the first turn
```

- `memory.NewBufferMemory()` — retains every message (unbounded).
- `memory.NewWindowMemory(n)` — keeps the last `n` non-system messages, always preserving system messages.
- `memory.NewFileMemory(path)` — disk-backed memory that persists messages as JSON and reloads them on construction, so conversations survive process restarts.

```go
mem, err := memory.NewFileMemory("/path/to/chat.json") // loads existing history if present
if err != nil {
    log.Fatal(err)
}
agent := agents.NewAgent(m, agents.AgentConfig{Memory: mem})
```

## Human-in-the-Loop

An `Approver` is consulted before each tool call. It returns an
`agents.ApprovalDecision{Approved, Reason, Args}`:

- **Approve** — `Approved: true`.
- **Deny** — `Approved: false`; the `Reason` is fed back to the model.
- **Rewrite args** — `Approved: true, Args: "..."` to replace the tool's arguments.

Returning an error aborts the run.

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

## Sub-Agents

Wrap an agent as a tool so a parent agent can delegate to it. `NewAgentTool` adapts
an `*Agent` into a `workflows.Tool`.

```go
researcher := agents.NewAgent(m, agents.AgentConfig{SystemPrompt: "You research topics."})
tool := agents.NewAgentTool("researcher", "Delegate research tasks", researcher)

coordinator := agents.NewAgent(m, agents.AgentConfig{
    SystemPrompt: "You coordinate work by delegating to sub-agents.",
    Tools:        []workflows.Tool{tool},
})
```

## Observability

Register `Hook` callbacks to observe the agent's lifecycle. Each `Hook` receives an
`Event` describing what happened.

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

Events are emitted with an `Event.Type` of one of:

- `turn_start`
- `model_response`
- `tool_start`
- `tool_end`
- `finish`
