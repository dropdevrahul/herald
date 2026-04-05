package workflows

import (
	"context"
	"dropdevrahul/herald/src/model"
	"errors"
	"strings"
	"sync"
)

type Node struct {
	Name   string
	Prompt string
}

func (n Node) String() string {
	if n.Name != "" {
		return n.Name
	}
	return n.Prompt[:min(len(n.Prompt), 50)]
}

type Tool interface {
	Name() string
	Description() string
	Call(ctx context.Context, args string) (string, error)
}

type WorkflowI interface {
	Run(ctx context.Context, input string) (string, error)
}

type StreamHandler func(result model.StreamResult) error

type StreamingWorkflowI interface {
	Run(ctx context.Context, input string) (string, error)
	RunStream(ctx context.Context, input string, handler StreamHandler) error
}

type ChainingWorkflow struct {
	model model.Model
	nodes []Node
	tools []Tool
}

func (cw *ChainingWorkflow) Run(ctx context.Context, input string) (string, error) {
	output := ""
	for _, node := range cw.nodes {
		out, err := cw.RunNode(ctx, &node, input)
		if err != nil {
			return "", err
		}

		input = out
		output = out
	}

	return output, nil
}

func (cw *ChainingWorkflow) RunStream(ctx context.Context, input string, handler StreamHandler) error {
	for _, node := range cw.nodes {
		out, err := cw.RunNodeStream(ctx, node, input, handler)
		if err != nil {
			return err
		}
		input = out
	}
	return nil
}

func (cw *ChainingWorkflow) RunNodeStream(ctx context.Context, node Node, input string, handler StreamHandler) (string, error) {
	messages := []model.Message{
		{Role: model.RoleSystem, Content: node.Prompt},
		{Role: model.RoleUser, Content: input},
	}

	opts := &model.ModelOptions{
		Tools: toolsToModelTools(cw.tools),
	}

	resultChan := cw.model.Stream(ctx, messages, opts)

	var sb strings.Builder
	var toolCalls []model.ToolCall

	for result := range resultChan {
		if result.Err != nil {
			return "", result.Err
		}

		if result.Delta != "" {
			sb.WriteString(result.Delta)
			if err := handler(result); err != nil {
				return "", err
			}
		}

		if len(result.ToolCalls) > 0 {
			toolCalls = result.ToolCalls
		}

		if result.Content != "" && result.Delta == "" {
			sb.WriteString(result.Content)
			if err := handler(result); err != nil {
				return "", err
			}
		}
	}

	if len(toolCalls) > 0 {
		toolResults := cw.executeTools(ctx, toolCalls)
		messages = append(messages, model.Message{
			Role:    model.RoleAssistant,
			Content: sb.String(),
		})
		messages = append(messages, toolResults...)

		resultChan = cw.model.Stream(ctx, messages, opts)
		for result := range resultChan {
			if result.Err != nil {
				return "", result.Err
			}
			if result.Delta != "" {
				sb.WriteString(result.Delta)
				if err := handler(result); err != nil {
					return "", err
				}
			}
			if result.Content != "" && result.Delta == "" {
				sb.WriteString(result.Content)
				if err := handler(result); err != nil {
					return "", err
				}
			}
		}
	}

	return sb.String(), nil
}

func (cw *ChainingWorkflow) RunNode(ctx context.Context, node *Node, input string) (string, error) {
	messages := []model.Message{
		{Role: model.RoleSystem, Content: node.Prompt},
		{Role: model.RoleUser, Content: input},
	}

	opts := &model.ModelOptions{
		Tools: toolsToModelTools(cw.tools),
	}

	resp, err := cw.model.Generate(ctx, messages, opts)
	if err != nil {
		return "", err
	}

	if len(resp.ToolCalls) > 0 {
		toolResults := cw.executeTools(ctx, resp.ToolCalls)
		messages = append(messages, model.Message{
			Role:    model.RoleAssistant,
			Content: resp.Content,
		})
		messages = append(messages, toolResults...)

		followUp, err := cw.model.Generate(ctx, messages, opts)
		if err != nil {
			return "", err
		}
		return followUp.Content, nil
	}

	return resp.Content, nil
}

func (cw *ChainingWorkflow) executeTools(ctx context.Context, toolCalls []model.ToolCall) []model.Message {
	var results []model.Message

	for _, tc := range toolCalls {
		for _, tool := range cw.tools {
			if tc.Function.Name == tool.Name() {
				result, err := tool.Call(ctx, tc.Function.Arguments)
				results = append(results, model.Message{
					Role:       model.RoleTool,
					Content:    result,
					ToolCallID: tc.ID,
				})
				if err != nil {
					results = append(results, model.Message{
						Role:       model.RoleTool,
						Content:    "Error: " + err.Error(),
						ToolCallID: tc.ID,
					})
				}
				break
			}
		}
	}

	return results
}

var ErrNoNodes = errors.New("no nodes provided")

type AggregatorFunc func(results []string) string

func DefaultAggregator(results []string) string {
	return strings.Join(results, "\n\n")
}

type OrchestratorWorkflow struct {
	Model       model.Model
	Nodes       []Node
	Aggregator  AggregatorFunc
	Parallel    bool
	MaxParallel int
}

func (ow *OrchestratorWorkflow) Run(ctx context.Context, input string) (string, error) {
	if ow.Parallel {
		return ow.runParallel(ctx, input)
	}
	return ow.runSequential(ctx, input)
}

func (ow *OrchestratorWorkflow) runSequential(ctx context.Context, input string) (string, error) {
	var results []string
	for _, node := range ow.Nodes {
		out, err := ow.runNode(ctx, &node, input)
		if err != nil {
			return "", err
		}
		results = append(results, out)
	}

	agg := ow.Aggregator
	if agg == nil {
		agg = DefaultAggregator
	}
	return agg(results), nil
}

func (ow *OrchestratorWorkflow) runParallel(ctx context.Context, input string) (string, error) {
	maxParallel := ow.MaxParallel
	if maxParallel == 0 || maxParallel > len(ow.Nodes) {
		maxParallel = len(ow.Nodes)
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	results := make([]string, len(ow.Nodes))
	errors := make([]error, len(ow.Nodes))

	sem := make(chan struct{}, maxParallel)

	for i, node := range ow.Nodes {
		wg.Add(1)
		sem <- struct{}{}

		go func(idx int, n Node) {
			defer wg.Done()
			defer func() { <-sem }()

			out, err := ow.runNode(ctx, &n, input)
			mu.Lock()
			results[idx] = out
			errors[idx] = err
			mu.Unlock()
		}(i, node)
	}

	wg.Wait()

	for _, err := range errors {
		if err != nil {
			return "", err
		}
	}

	agg := ow.Aggregator
	if agg == nil {
		agg = DefaultAggregator
	}
	return agg(results), nil
}

func (ow *OrchestratorWorkflow) runNode(ctx context.Context, node *Node, input string) (string, error) {
	resultChan := ow.Model.Stream(ctx, []model.Message{
		{Role: model.RoleSystem, Content: node.Prompt},
		{Role: model.RoleUser, Content: input},
	}, nil)

	var sb strings.Builder
	for result := range resultChan {
		if result.Err != nil {
			return "", result.Err
		}
		if result.Delta != "" {
			sb.WriteString(result.Delta)
		}
		if result.Content != "" && result.Delta == "" {
			sb.WriteString(result.Content)
		}
	}

	return sb.String(), nil
}

type ParallelWorkflow struct {
	model model.Model
	nodes []Node
}

func (pw *ParallelWorkflow) Run(ctx context.Context, input string) (string, error) {
	var mu sync.Mutex
	var wg sync.WaitGroup
	results := make([]string, len(pw.nodes))
	errors := make([]error, len(pw.nodes))

	for i, node := range pw.nodes {
		wg.Add(1)
		go func(idx int, n Node) {
			defer wg.Done()

			resultChan := pw.model.Stream(ctx, []model.Message{
				{Role: model.RoleSystem, Content: n.Prompt},
				{Role: model.RoleUser, Content: input},
			}, nil)

			var sb strings.Builder
			for result := range resultChan {
				if result.Err != nil {
					mu.Lock()
					errors[idx] = result.Err
					mu.Unlock()
					return
				}
				if result.Delta != "" {
					sb.WriteString(result.Delta)
				}
				if result.Content != "" && result.Delta == "" {
					sb.WriteString(result.Content)
				}
			}

			mu.Lock()
			results[idx] = sb.String()
			mu.Unlock()
		}(i, node)
	}

	wg.Wait()

	for _, err := range errors {
		if err != nil {
			return "", err
		}
	}

	return strings.Join(results, "\n---\n"), nil
}

func toolsToModelTools(tools []Tool) model.Tools {
	var result model.Tools
	for _, t := range tools {
		result = append(result, model.FunctionDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  nil,
		})
	}
	return result
}

func NewChainingWorkflow(m model.Model, nodes []Node, tools ...Tool) StreamingWorkflowI {
	if len(nodes) == 0 {
		return &errorWorkflow{err: ErrNoNodes}
	}
	return &ChainingWorkflow{
		model: m,
		nodes: nodes,
		tools: tools,
	}
}

func NewOrchestratorWorkflow(m model.Model, nodes []Node, aggregator AggregatorFunc) WorkflowI {
	if len(nodes) == 0 {
		return &errorWorkflow{err: ErrNoNodes}
	}
	return &OrchestratorWorkflow{
		Model:      m,
		Nodes:      nodes,
		Aggregator: aggregator,
	}
}

func NewParallelWorkflow(m model.Model, nodes []Node) WorkflowI {
	if len(nodes) == 0 {
		return &errorWorkflow{err: ErrNoNodes}
	}
	return &ParallelWorkflow{
		model: m,
		nodes: nodes,
	}
}

type errorWorkflow struct {
	err error
}

func (e *errorWorkflow) Run(ctx context.Context, input string) (string, error) {
	return "", e.err
}

func (e *errorWorkflow) RunStream(ctx context.Context, input string, handler StreamHandler) error {
	return e.err
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
