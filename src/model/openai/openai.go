package openai

import (
	"context"
	"dropdevrahul/herald/src/model"
	"strings"

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

func (m *OpenAIModel) Generate(ctx context.Context, messages []model.Message, opts *model.ModelOptions) (*model.Response, error) {
	if opts == nil {
		opts = &m.options
	}

	params := openai.ChatCompletionNewParams{
		Model:    opts.Model,
		Messages: toOpenAIMessages(messages),
	}

	resp, err := m.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, err
	}

	return &model.Response{
		Content: resp.Choices[0].Message.Content,
		Usage: model.Usage{
			PromptTokens:     int(resp.Usage.PromptTokens),
			CompletionTokens: int(resp.Usage.CompletionTokens),
			TotalTokens:      int(resp.Usage.TotalTokens),
		},
	}, nil
}

func (m *OpenAIModel) Stream(ctx context.Context, messages []model.Message, opts *model.ModelOptions) <-chan model.StreamResult {
	if opts == nil {
		opts = &m.options
	}

	if opts.Model == "" {
		opts.Model = m.options.Model
	}

	resultChan := make(chan model.StreamResult)

	params := openai.ChatCompletionNewParams{
		Model:    opts.Model,
		Messages: toOpenAIMessages(messages),
	}

	stream := m.client.Chat.Completions.NewStreaming(ctx, params)

	go func() {
		defer close(resultChan)

		var sb strings.Builder
		for stream.Next() {
			chunk := stream.Current()
			for _, choice := range chunk.Choices {
				delta := choice.Delta.Content
				if delta != "" {
					sb.WriteString(delta)
					resultChan <- model.StreamResult{
						Delta: delta,
					}
				}
			}
		}

		if err := stream.Err(); err != nil {
			resultChan <- model.StreamResult{
				Err: err,
			}
			return
		}

		resultChan <- model.StreamResult{
			Content: sb.String(),
		}
	}()

	return resultChan
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
			openAIMessages = append(openAIMessages, openai.AssistantMessage(m.Content))
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
