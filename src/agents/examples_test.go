package agents

import (
	"context"
	"fmt"

	"github.com/dropdevrahul/herald/src/model"
)

func ExampleNewCodingAgentWithTools() {
	m := newMockModel()

	agent := NewCodingAgentWithTools(m, nil, 5, "/tmp/test")

	result, _ := agent.Run(context.Background(), "Write a function to add two numbers")
	fmt.Println(result)
	// Output: mock response
}

func ExampleReActCodingAgent() {
	client := newMockModel()
	agent := NewReActCodingAgent(client, nil, 5, "/tmp/test")

	result, _ := agent.Run(context.Background(), "What is 2+2?")
	fmt.Println(result)
	// Output: mock response
}

func ExampleCodingAgent_RunStream() {
	client := newMockModel()

	agent := NewReActCodingAgent(client, nil, 5, "/tmp/test")

	agent.RunStream(context.Background(), "Hello", func(node, result string) error {
		fmt.Printf("[%s] %s\n", node, result)
		return nil
	})
	// Output: [content] mock response
}

type mockModel struct{}

func newMockModel() model.Model { return &mockModel{} }

func (m *mockModel) Generate(ctx context.Context, msgs []model.Message, opts *model.ModelOptions) (*model.Response, error) {
	return &model.Response{Content: "mock response"}, nil
}

func (m *mockModel) Stream(ctx context.Context, msgs []model.Message, opts *model.ModelOptions) <-chan model.StreamResult {
	ch := make(chan model.StreamResult)
	go func() {
		ch <- model.StreamResult{Content: "mock response"}
		close(ch)
	}()
	return ch
}
