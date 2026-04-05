package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/dropdevrahul/herald/src/model"
	"io"
	"net/http"
	"strings"
)

type GeminiModel struct {
	options model.ModelOptions
	client  *http.Client
	apiKey  string
	baseURL string
}

type GeminiRequest struct {
	Contents []Content `json:"contents"`
}

type Content struct {
	Parts []Part `json:"parts"`
}

type Part struct {
	Text string `json:"text"`
}

type GeminiResponse struct {
	Candidates    []Candidate    `json:"candidates"`
	UsageMetadata *UsageMetadata `json:"usageMetadata"`
}

type Candidate struct {
	Content Content `json:"content"`
}

type UsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

func NewGeminiModel(options model.ModelOptions, apiKey string) model.Model {
	return &GeminiModel{
		options: options,
		client:  &http.Client{},
		apiKey:  apiKey,
		baseURL: "https://generativelanguage.googleapis.com/v1beta",
	}
}

func (m *GeminiModel) Generate(ctx context.Context, messages []model.Message, opts *model.ModelOptions) (*model.Response, error) {
	if opts == nil {
		opts = &m.options
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", m.baseURL, opts.Model, m.apiKey)

	body, err := json.Marshal(GeminiRequest{
		Contents: toGeminiContents(messages),
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini api error: %s", string(respBody))
	}

	var geminiResp GeminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return nil, err
	}

	content := ""
	if len(geminiResp.Candidates) > 0 && len(geminiResp.Candidates[0].Content.Parts) > 0 {
		content = geminiResp.Candidates[0].Content.Parts[0].Text
	}

	usage := model.Usage{}
	if geminiResp.UsageMetadata != nil {
		usage.PromptTokens = geminiResp.UsageMetadata.PromptTokenCount
		usage.CompletionTokens = geminiResp.UsageMetadata.CandidatesTokenCount
		usage.TotalTokens = geminiResp.UsageMetadata.TotalTokenCount
	}

	return &model.Response{
		Content: content,
		Usage:   usage,
	}, nil
}

func (m *GeminiModel) Stream(ctx context.Context, messages []model.Message, opts *model.ModelOptions) <-chan model.StreamResult {
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

func toGeminiContents(messages []model.Message) []Content {
	var contents []Content
	var currentContent Content

	for _, msg := range messages {
		part := Part{Text: msg.Content}
		currentContent.Parts = append(currentContent.Parts, part)
	}

	if len(currentContent.Parts) > 0 {
		contents = append(contents, currentContent)
	}

	if len(contents) == 0 {
		contents = append(contents, Content{Parts: []Part{}})
	}

	return contents
}
