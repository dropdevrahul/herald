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

## Adding a Provider

To add a provider, implement the `model.Model` interface and convert to/from the
shared types (`Message`, `ModelOptions`, `Response`, `ToolCall`, `StreamResult`).
