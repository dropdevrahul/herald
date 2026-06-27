package openai

import (
	"context"
	"fmt"
	"strings"

	"github.com/dropdevrahul/herald/src/model"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

func newOpenAIClient(apiKey string, baseURL string) *openai.Client {
	client := openai.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey(apiKey),
	)
	return &client
}

type OpenAIModel struct {
	options model.ModelOptions
	client  *openai.Client
}

// resolveOpts returns a local copy of the effective options so callers never
// mutate the receiver's shared options struct (or the caller's opts).
func (m *OpenAIModel) resolveOpts(opts *model.ModelOptions) model.ModelOptions {
	var o model.ModelOptions
	if opts != nil {
		o = *opts
	} else {
		o = m.options
	}
	if o.Model == "" {
		o.Model = m.options.Model
	}
	return o
}

func (m *OpenAIModel) buildParams(messages []model.Message, o model.ModelOptions) openai.ChatCompletionNewParams {
	params := openai.ChatCompletionNewParams{
		Model:    o.Model,
		Messages: toOpenAIMessages(messages),
	}

	if len(o.Tools) > 0 {
		params.Tools = toOpenAITools(o.Tools)
	}
	if o.Temperature != 0 {
		params.Temperature = openai.Float(o.Temperature)
	}
	if o.MaxTokens != 0 {
		params.MaxTokens = openai.Int(int64(o.MaxTokens))
	}
	if o.ToolChoice != "" {
		// The SDK encodes all three scalar modes ("none", "auto", "required")
		// through OfAuto; the named-tool variants use the other union fields.
		params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: openai.String(string(o.ToolChoice)),
		}
	}

	return params
}

func (m *OpenAIModel) Generate(ctx context.Context, messages []model.Message, opts *model.ModelOptions) (*model.Response, error) {
	o := m.resolveOpts(opts)
	params := m.buildParams(messages, o)

	resp, err := m.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai: empty choices")
	}

	msg := resp.Choices[0].Message

	var toolCalls []model.ToolCall
	for _, tc := range msg.ToolCalls {
		toolCalls = append(toolCalls, model.ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: model.Function{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}

	return &model.Response{
		Content: msg.Content,
		Usage: model.Usage{
			PromptTokens:     int(resp.Usage.PromptTokens),
			CompletionTokens: int(resp.Usage.CompletionTokens),
			TotalTokens:      int(resp.Usage.TotalTokens),
		},
		ToolCalls: toolCalls,
	}, nil
}

func (m *OpenAIModel) Stream(ctx context.Context, messages []model.Message, opts *model.ModelOptions) <-chan model.StreamResult {
	o := m.resolveOpts(opts)
	params := m.buildParams(messages, o)
	// Request usage on the final streaming chunk. This is set here rather
	// than in buildParams so that Generate (non-streaming) is unaffected.
	params.StreamOptions = openai.ChatCompletionStreamOptionsParam{
		IncludeUsage: openai.Bool(true),
	}

	resultChan := make(chan model.StreamResult)

	stream := m.client.Chat.Completions.NewStreaming(ctx, params)

	go func() {
		defer close(resultChan)

		var sb strings.Builder

		// Tool-call deltas arrive incrementally, keyed by index. Accumulate
		// them and emit the assembled calls in the final result.
		toolAccum := map[int64]*model.ToolCall{}
		var order []int64

		// usage arrives on the final chunk (Choices is empty on that chunk).
		var usage model.Usage

		for stream.Next() {
			chunk := stream.Current()
			if chunk.Usage.TotalTokens > 0 {
				usage = model.Usage{
					PromptTokens:     int(chunk.Usage.PromptTokens),
					CompletionTokens: int(chunk.Usage.CompletionTokens),
					TotalTokens:      int(chunk.Usage.TotalTokens),
				}
			}
			for _, choice := range chunk.Choices {
				delta := choice.Delta.Content
				if delta != "" {
					sb.WriteString(delta)
					resultChan <- model.StreamResult{
						Delta: delta,
					}
				}

				for _, tcd := range choice.Delta.ToolCalls {
					tc, ok := toolAccum[tcd.Index]
					if !ok {
						tc = &model.ToolCall{}
						toolAccum[tcd.Index] = tc
						order = append(order, tcd.Index)
					}
					if tcd.ID != "" {
						tc.ID = tcd.ID
					}
					if tcd.Type != "" {
						tc.Type = tcd.Type
					}
					if tcd.Function.Name != "" {
						tc.Function.Name = tcd.Function.Name
					}
					tc.Function.Arguments += tcd.Function.Arguments
				}
			}
		}

		if err := stream.Err(); err != nil {
			resultChan <- model.StreamResult{
				Err: err,
			}
			return
		}

		var toolCalls []model.ToolCall
		for _, idx := range order {
			toolCalls = append(toolCalls, *toolAccum[idx])
		}

		resultChan <- model.StreamResult{
			Content:   sb.String(),
			ToolCalls: toolCalls,
			Usage:     usage,
		}
	}()

	return resultChan
}

func toOpenAITools(tools model.Tools) []openai.ChatCompletionToolUnionParam {
	var result []openai.ChatCompletionToolUnionParam
	for _, t := range tools {
		params := t.Parameters
		if params == nil {
			params = map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			}
		}
		fn := openai.FunctionDefinitionParam{
			Name:        t.Name,
			Description: openai.String(t.Description),
			Parameters:  openai.FunctionParameters(params),
		}
		result = append(result, openai.ChatCompletionFunctionTool(fn))
	}
	return result
}

func toOpenAIMessages(messages []model.Message) []openai.ChatCompletionMessageParamUnion {
	var openAIMessages []openai.ChatCompletionMessageParamUnion
	for _, m := range messages {
		switch m.Role {
		case model.RoleSystem:
			openAIMessages = append(openAIMessages, openai.SystemMessage(m.Content))
		case model.RoleUser:
			openAIMessages = append(openAIMessages, openai.UserMessage(m.Content))
		case model.RoleAssistant:
			if len(m.ToolCalls) > 0 {
				asst := openai.ChatCompletionAssistantMessageParam{}
				if m.Content != "" {
					asst.Content.OfString = openai.String(m.Content)
				}
				for _, tc := range m.ToolCalls {
					asst.ToolCalls = append(asst.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
						OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
							ID: tc.ID,
							Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
								Name:      tc.Function.Name,
								Arguments: tc.Function.Arguments,
							},
						},
					})
				}
				openAIMessages = append(openAIMessages, openai.ChatCompletionMessageParamUnion{OfAssistant: &asst})
			} else {
				openAIMessages = append(openAIMessages, openai.AssistantMessage(m.Content))
			}
		case model.RoleTool:
			openAIMessages = append(openAIMessages, openai.ToolMessage(m.Content, m.ToolCallID))
		}
	}
	return openAIMessages
}

func NewOpenAIModel(options model.ModelOptions, client *openai.Client) model.Model {
	return &OpenAIModel{options: options, client: client}
}

func NewClient(apiKey string, baseURL string) *openai.Client {
	return newOpenAIClient(apiKey, baseURL)
}
