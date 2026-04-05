package anthropic

import (
	"context"
	"dropdevrahul/herald/src/model"
)

type AnthropicModel struct {
	options model.ModelOptions
}

func (m *AnthropicModel) Generate(ctx context.Context, messages []model.Message, opts *model.ModelOptions) (*model.Response, error) {
	if opts == nil {
		opts = &m.options
	}

	return &model.Response{
		Content: "Anthropic response placeholder",
		Usage:   model.Usage{},
	}, nil
}

func (m *AnthropicModel) Stream(ctx context.Context, messages []model.Message, opts *model.ModelOptions) <-chan model.StreamResult {
	if opts == nil {
		opts = &m.options
	}

	resultChan := make(chan model.StreamResult)

	go func() {
		defer close(resultChan)
		resultChan <- model.StreamResult{
			Delta: "Anthropic ",
		}
		resultChan <- model.StreamResult{
			Delta: "streaming ",
		}
		resultChan <- model.StreamResult{
			Content: "placeholder",
		}
	}()

	return resultChan
}

func NewAnthropicModel(options model.ModelOptions) model.Model {
	return &AnthropicModel{options: options}
}
