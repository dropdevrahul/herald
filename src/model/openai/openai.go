package openai

import (
	"context"
	"dropdevrahul/herald/src/model"
	"os"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

func newOpenAIClient() *openai.Client {
	key, found := os.LookupEnv("API_KEY")
	if !found {
		panic("API KEY not found")
	}
	client := openai.NewClient(
		option.WithBaseURL("https://api.groq.com/openai/v1"),
		option.WithAPIKey(key),
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

func (m *OpenAIModel) Stream(ctx context.Context, messages []model.Message, opts *model.ModelOptions) (<-chan string, <-chan error) {
	if opts == nil {
		opts = &m.options
	}

	contentChan := make(chan string)
	errChan := make(chan error, 1)

	params := openai.ChatCompletionNewParams{
		Model:    opts.Model,
		Messages: toOpenAIMessages(messages),
	}

	stream := m.client.Chat.Completions.NewStreaming(ctx, params)

	go func() {
		defer close(contentChan)
		defer close(errChan)

		for stream.Next() {
			chunk := stream.Current()
			for _, choice := range chunk.Choices {
				contentChan <- choice.Delta.Content
			}
		}

		if err := stream.Err(); err != nil {
			errChan <- err
		}
	}()

	return contentChan, errChan
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
		}
	}
	return openAIMessages
}

func NewOpenAIModel(options model.ModelOptions) model.Model {
	return &OpenAIModel{options: options, client: newOpenAIClient()}
}
