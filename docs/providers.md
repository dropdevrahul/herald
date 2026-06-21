# Providers

The model layer (`src/model/`) defines a provider-agnostic `Model` interface and
shared types. Provider implementations live in subpackages and convert to/from those
shared types.

## The Model Interface

```go
type Model interface {
    Generate(ctx context.Context, messages []Message, opts *ModelOptions) (*Response, error)
    Stream(ctx context.Context, messages []Message, opts *ModelOptions) <-chan StreamResult
}
```

Streaming uses the idiomatic single-channel-of-result-struct pattern — `Stream`
returns a `<-chan model.StreamResult`, not separate data/error channels. Each
`StreamResult` carries `{Content, Delta, Usage, Err, ToolCalls}`.

Shared types include `Message`, `ModelOptions`, `Response`, `ToolCall`, and
`StreamResult`.

## OpenAI / Groq / Azure

The `openai` package serves OpenAI, Groq, Azure, and any OpenAI-compatible endpoint.
Construct a client with `openai.NewClient(apiKey, baseURL)`, then build a model with
`openai.NewOpenAIModel(opts, client)`.

```go
import (
    "github.com/dropdevrahul/herald/src/model"
    "github.com/dropdevrahul/herald/src/model/openai"
)

// OpenAI-compatible client pointed at Groq.
client := openai.NewClient(apiKey, "https://api.groq.com/openai/v1")
m := openai.NewOpenAIModel(model.ModelOptions{Model: "llama-3.3-70b-versatile"}, client)
```

To target OpenAI directly, pass OpenAI's base URL (or the empty string for the
default); for Azure or another OpenAI-compatible service, pass that endpoint's URL.

## Anthropic

The `anthropic` package implements the `Model` interface against the Anthropic API
over `net/http`.

```go
import "github.com/dropdevrahul/herald/src/model/anthropic"
```

## Gemini

The `gemini` package implements the `Model` interface against the Google Gemini API.

```go
import "github.com/dropdevrahul/herald/src/model/gemini"
```

## TUI Provider Selection

!!! note "Environment keys"
    The terminal app (`./herald`) reads its active provider's API key from the
    environment. It looks for one of `GROQ_API_KEY`, `OPENAI_API_KEY`, or
    `ANTHROPIC_API_KEY`. The active provider defaults to `groq`.

## Structured Output

`model.GenerateJSON` calls a model and unmarshals a JSON value from its response
into a Go value. It tolerates Markdown code fences and prose around the JSON,
extracting the outermost JSON object or array before unmarshalling.

```go
import "github.com/dropdevrahul/herald/src/model"

type Person struct {
    Name string `json:"name"`
    Age  int    `json:"age"`
}

var p Person
msgs := []model.Message{
    {Role: model.RoleUser, Content: "Give me a person as JSON."},
}
if err := model.GenerateJSON(ctx, m, msgs, nil, &p); err != nil {
    log.Fatal(err)
}
```

### Streaming

`model.GenerateJSONStream` is the streaming counterpart. It streams the response,
calling `onDelta` with each chunk as it arrives (for live display), then extracts
and unmarshals the JSON once the stream completes.

```go
var p Person
err := model.GenerateJSONStream(ctx, m, msgs, nil, &p, func(delta string) {
    fmt.Print(delta) // show progress as tokens arrive
})
```

!!! note "Partial JSON can't be decoded"
    A partial JSON value cannot be unmarshalled into a typed Go value mid-stream,
    so unmarshalling happens once at the end. `onDelta` is for display only.

!!! note "Heuristic extraction"
    JSON is located with an outermost-bracket scan, which is robust to fences and
    surrounding text but is not a full streaming parser.

## Adding a Provider

To add a provider, implement the `model.Model` interface and convert to/from the
shared types (`Message`, `ModelOptions`, `Response`, `ToolCall`, `StreamResult`).
