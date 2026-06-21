package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/dropdevrahul/herald/src/model"
)

type AnthropicModel struct {
	options model.ModelOptions
	apiKey  string
	baseURL string
	client  *http.Client
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicResponse struct {
	Content []anthropicContentBlock `json:"content"`
	Usage   anthropicUsage          `json:"usage"`
}

func NewAnthropicModel(options model.ModelOptions) model.Model {
	return &AnthropicModel{
		options: options,
		apiKey:  os.Getenv("ANTHROPIC_API_KEY"),
		baseURL: "https://api.anthropic.com/v1",
		client:  &http.Client{},
	}
}

func (m *AnthropicModel) Generate(ctx context.Context, messages []model.Message, opts *model.ModelOptions) (*model.Response, error) {
	if opts == nil {
		opts = &m.options
	}

	maxTokens := opts.MaxTokens
	if maxTokens == 0 {
		maxTokens = 1024
	}

	var systemParts []string
	var turns []anthropicMessage

	for _, msg := range messages {
		switch msg.Role {
		case model.RoleSystem:
			systemParts = append(systemParts, msg.Content)
		case model.RoleUser, model.RoleTool:
			turns = append(turns, anthropicMessage{Role: "user", Content: msg.Content})
		case model.RoleAssistant:
			turns = append(turns, anthropicMessage{Role: "assistant", Content: msg.Content})
		}
	}

	reqBody := anthropicRequest{
		Model:     opts.Model,
		MaxTokens: maxTokens,
		Messages:  turns,
	}
	if len(systemParts) > 0 {
		reqBody.System = strings.Join(systemParts, "\n")
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", m.baseURL+"/messages", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", m.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic api error: %s", string(respBody))
	}

	var anthropicResp anthropicResponse
	if err := json.Unmarshal(respBody, &anthropicResp); err != nil {
		return nil, err
	}

	content := ""
	if len(anthropicResp.Content) > 0 {
		content = anthropicResp.Content[0].Text
	}

	usage := model.Usage{
		PromptTokens:     anthropicResp.Usage.InputTokens,
		CompletionTokens: anthropicResp.Usage.OutputTokens,
		TotalTokens:      anthropicResp.Usage.InputTokens + anthropicResp.Usage.OutputTokens,
	}

	return &model.Response{
		Content: content,
		Usage:   usage,
	}, nil
}

func (m *AnthropicModel) Stream(ctx context.Context, messages []model.Message, opts *model.ModelOptions) <-chan model.StreamResult {
	if opts == nil {
		opts = &m.options
	}

	resultChan := make(chan model.StreamResult)

	go func() {
		defer close(resultChan)

		resp, err := m.Generate(ctx, messages, opts)
		if err != nil {
			resultChan <- model.StreamResult{Err: err}
			return
		}

		resultChan <- model.StreamResult{Content: resp.Content, Usage: resp.Usage}
	}()

	return resultChan
}
