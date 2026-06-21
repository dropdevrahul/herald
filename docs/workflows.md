# Workflows

The `worklows` package provides node-based and graph-based workflows.

!!! note "Directory vs. package name"
    The directory is spelled `worklows` (the import path), but the Go package is
    named `workflows`. Import it as
    `github.com/dropdevrahul/herald/src/worklows` and reference it as `workflows`.

## Nodes

A `Node` is the basic unit of a workflow — a name plus a system prompt.

```go
type Node struct {
    Name   string
    Prompt string
}
```

## Chaining Workflow

`NewChainingWorkflow` runs nodes sequentially, threading each node's output into the
next. It also supports tools, which the model may call during execution.

```go
import (
    "github.com/dropdevrahul/herald/src/model"
    "github.com/dropdevrahul/herald/src/model/openai"
    "github.com/dropdevrahul/herald/src/worklows"
)

type CalculatorTool struct{}

func (t *CalculatorTool) Name() string                  { return "calculator" }
func (t *CalculatorTool) Description() string            { return "Performs arithmetic" }
func (t *CalculatorTool) Parameters() map[string]any     { return map[string]any{} }
func (t *CalculatorTool) Call(ctx context.Context, args string) (string, error) {
    // ...
    return "42", nil
}

client := openai.NewClient(apiKey, "https://api.groq.com/openai/v1")
m := openai.NewOpenAIModel(model.ModelOptions{Model: "llama-3.3-70b-versatile"}, client)

node := workflows.Node{Prompt: "You are a helpful assistant. Use tools when needed."}
wf := workflows.NewChainingWorkflow(m, []workflows.Node{node}, &CalculatorTool{})
output, _ := wf.Run(ctx, "What is 15 + 27?")
```

The signature is:

```go
func NewChainingWorkflow(m model.Model, nodes []Node, tools ...Tool) StreamingWorkflowI
```

## Orchestrator Workflow

`NewOrchestratorWorkflow` coordinates multiple nodes and aggregates their results.
Set `.Parallel = true` to run the nodes concurrently.

```go
wf := workflows.NewOrchestratorWorkflow(m, nodes, aggregatorFunc)
wf.Parallel = true // run nodes concurrently
output, _ := wf.Run(ctx, "input")
```

## Parallel Execution

Workflows can run all of their nodes concurrently and combine the results — useful
for fan-out tasks where each node works on the same input independently.

## Graph Workflows

Graph workflows are directed graphs of nodes, edges, and conditional routes. Build a
graph with the fluent API, then `Compile()` it into a runnable. Set `MaxIterations`
to bound loops.

```go
import "strings"

g := workflows.NewGraph(m).
    AddNode("chat", "You are a helpful assistant.", func(ctx context.Context, state any) (any, error) {
        // ...
        return state, nil
    }).
    AddEdge("chat", "continue").
    AddConditionalNode("continue", func(ctx context.Context, state any) string {
        if strings.Contains(state.(string), "bye") {
            return "" // empty string ends the graph
        }
        return "chat" // loop back
    }).
    SetStart("chat")

compiled, _ := g.Compile()
compiled.MaxIterations = 5
result, _ := compiled.Run(ctx, "Hello!")
```

- `AddNode(name, prompt, fn)` — add a node with a handler.
- `AddEdge(from, to)` — connect two nodes.
- `AddConditionalNode(name, router)` — route by returning the next node's name (empty string ends the graph).
- `SetStart(name)` — set the entry node.
- `Compile()` — produce a runnable; set `MaxIterations` for loop support.

## Streaming

Every workflow supports streaming via `RunStream`. The handler receives each
`model.StreamResult` as tokens arrive.

```go
handler := func(result model.StreamResult) error {
    fmt.Print(result.Delta)
    return nil
}
wf.RunStream(ctx, "Hello!", handler)
```
