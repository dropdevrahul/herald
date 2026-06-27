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
| `ToolTimeout`  | `time.Duration`  | Per-call deadline for tool execution. `0` means no timeout.     |

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

## Durable Human-in-the-Loop

For approvals that must survive a process restart, use `RunThread` and `Resume`
backed by a `workflows.Checkpointer`. Set `AgentConfig.Checkpointer` and
`AgentConfig.InterruptBefore`; when `InterruptBefore` returns `true` for a
pending tool call, `RunThread` persists the full run state and returns an
`AgentResult` whose `Interrupt` field is non-nil — no tool is executed yet.

```go
import (
    "github.com/dropdevrahul/herald/src/agents"
    "github.com/dropdevrahul/herald/src/worklows"
)

agent := agents.NewAgent(m, agents.AgentConfig{
    Tools: []workflows.Tool{&DeleteFileTool{}},
    Checkpointer: workflows.NewFileCheckpointer("./checkpoints"),
    InterruptBefore: func(c model.ToolCall) bool {
        return c.Function.Name == "delete_file"
    },
})

res, _ := agent.RunThread(ctx, "thread-1", "Remove the temp directory.")
if res.Interrupt != nil {
    // res.Interrupt.ThreadID and res.Interrupt.ToolCall describe what is waiting.
    // Obtain a human decision, then resume — works across process restarts.
    res, _ = agent.Resume(ctx, "thread-1", agents.ApprovalDecision{Approved: true})
}
```

### AgentConfig fields for durable HITL

| Field             | Type                               | Description                                                                           |
| ----------------- | ---------------------------------- | ------------------------------------------------------------------------------------- |
| `Checkpointer`    | `workflows.Checkpointer`           | Persists durable thread state. Required when `InterruptBefore` is used. Use `workflows.NewFileCheckpointer(dir)` for cross-process durability or `workflows.NewMemoryCheckpointer()` for in-process (e.g. tests). |
| `InterruptBefore` | `func(call model.ToolCall) bool`   | When non-nil and returns `true` for a tool call, the run pauses durably BEFORE executing that call. |

### Interrupt and Resume

- `AgentResult.Interrupt *Interrupt` — non-nil when the run paused. Contains `ThreadID string` and `ToolCall model.ToolCall` describing the pending call.
- `agent.RunThread(ctx, threadID, input) (AgentResult, error)` — starts a durable run. If it pauses, state is persisted and `AgentResult.Interrupt` is set.
- `agent.Resume(ctx, threadID, decision) (AgentResult, error)` — loads persisted state for `threadID` and continues, applying the `ApprovalDecision` to the pending tool call. `Approved: false` denies the call (reason fed back to the model); `Args != ""` rewrites the call arguments before executing.

`Resume` loads state via the Checkpointer, so it can be called in a new process
after the original `RunThread` call exits.

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

## Tool Resilience

Tool panics are always recovered: if a tool's `Call` method panics, the panic is
caught and the model receives `Error: tool panicked: <value>` as the tool result
instead of the agent run crashing.

To bound a slow tool, set `AgentConfig.ToolTimeout`. When the deadline is exceeded
the model receives `Error: context deadline exceeded` and the run continues:

```go
agent := agents.NewAgent(m, agents.AgentConfig{
    Tools:       []workflows.Tool{&MyTool{}},
    ToolTimeout: 30 * time.Second, // 0 (default) means no timeout
})
```

## Token Usage

Call `agent.RunResult` instead of `agent.Run` to receive an `AgentResult` that
includes the final content plus token counts aggregated across every turn of the
run:

```go
res, err := agent.RunResult(ctx, "Summarise this document.")
if err != nil {
    log.Fatal(err)
}
fmt.Println(res.Content)
fmt.Printf("tokens used: prompt=%d completion=%d total=%d (turns=%d)\n",
    res.Usage.PromptTokens, res.Usage.CompletionTokens, res.Usage.TotalTokens, res.Turns)
```

`AgentResult` fields:

| Field     | Type          | Description                                                      |
| --------- | ------------- | ---------------------------------------------------------------- |
| `Content` | `string`      | Final model response text.                                       |
| `Usage`   | `model.Usage` | Token counts (`PromptTokens`, `CompletionTokens`, `TotalTokens`) summed across all turns. |
| `Turns`   | `int`         | Number of turns executed (including tool-call turns).            |

`Run` and `RunStream` retain their existing signatures and behaviour; `RunResult`
is the zero-overhead addition for callers that need cost accounting.
