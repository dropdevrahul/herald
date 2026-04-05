package workflows

import (
	"context"
	"github.com/dropdevrahul/herald/src/model"
	"strings"
)

type State[T any] interface {
	Get() T
	Set(T)
}

type MessagesState struct {
	messages []model.Message
}

func NewMessagesState(messages []model.Message) MessagesState {
	return MessagesState{messages: messages}
}

func (s MessagesState) Get() []model.Message {
	return s.messages
}

func (s *MessagesState) Set(messages []model.Message) {
	s.messages = messages
}

func (s MessagesState) AddMessage(msg model.Message) MessagesState {
	s.messages = append(s.messages, msg)
	return s
}

type GraphNodeFunc func(ctx context.Context, state any) (any, error)

type GraphNode struct {
	Name        string
	Description string
	Run         GraphNodeFunc
}

type ConditionalGraphFunc func(ctx context.Context, state any) string

type ConditionalGraphNode struct {
	Name  string
	Func  ConditionalGraphFunc
	Graph *Graph
}

type ConditionalFunc func(ctx context.Context, state any) string

type ConditionalNode struct {
	Name  string
	Func  ConditionalFunc
	Graph *Graph
}

type Edge struct {
	From string
	To   string
}

type Graph struct {
	ID          string
	Model       model.Model
	Nodes       map[string]*GraphNode
	Edges       []Edge
	Conditional map[string]*ConditionalGraphNode
	Start       string
}

func NewGraph(model model.Model) *Graph {
	return &Graph{
		ID:          "default",
		Model:       model,
		Nodes:       make(map[string]*GraphNode),
		Edges:       []Edge{},
		Conditional: make(map[string]*ConditionalGraphNode),
	}
}

func (g *Graph) AddNode(name string, description string, run GraphNodeFunc) *Graph {
	g.Nodes[name] = &GraphNode{
		Name:        name,
		Description: description,
		Run:         run,
	}
	return g
}

func (g *Graph) AddEdge(from string, to string) *Graph {
	g.Edges = append(g.Edges, Edge{From: from, To: to})
	return g
}

func (g *Graph) SetStart(node string) *Graph {
	g.Start = node
	return g
}

func (g *Graph) AddConditionalNode(name string, conditionFunc ConditionalGraphFunc) *Graph {
	g.Conditional[name] = &ConditionalGraphNode{
		Name:  name,
		Func:  conditionFunc,
		Graph: g,
	}
	return g
}

func (g *Graph) Compile() (*CompiledGraph, error) {
	if g.Start == "" {
		return nil, ErrNoStartNode
	}
	if _, ok := g.Nodes[g.Start]; !ok {
		return nil, ErrNodeNotFound
	}
	return &CompiledGraph{Graph: g}, nil
}

func (g *Graph) GetNode(name string) (*GraphNode, bool) {
	node, ok := g.Nodes[name]
	return node, ok
}

func (g *Graph) GetEdgesFrom(node string) []string {
	var edges []string
	for _, e := range g.Edges {
		if e.From == node {
			edges = append(edges, e.To)
		}
	}
	return edges
}

type CompiledGraph struct {
	*Graph
	MaxIterations int
}

func (cg *CompiledGraph) Run(ctx context.Context, input any) (any, error) {
	current := cg.Start
	iteration := 0
	maxIter := cg.MaxIterations
	if maxIter == 0 {
		maxIter = 10
	}

	for current != "" && iteration < maxIter {
		node, ok := cg.Nodes[current]
		if !ok {
			break
		}

		result, err := node.Run(ctx, input)
		if err != nil {
			return nil, err
		}

		input = result
		iteration++

		current = cg.getNextNode(ctx, current, result)
	}

	return input, nil
}

func (cg *CompiledGraph) RunStream(ctx context.Context, input any, handler func(string, any) error) (any, error) {
	current := cg.Start
	iteration := 0
	maxIter := cg.MaxIterations
	if maxIter == 0 {
		maxIter = 10
	}

	for current != "" && iteration < maxIter {
		node, ok := cg.Nodes[current]
		if !ok {
			break
		}

		result, err := node.Run(ctx, input)
		if err != nil {
			return nil, err
		}

		if handler != nil {
			if err := handler(current, result); err != nil {
				return nil, err
			}
		}

		input = result
		iteration++

		current = cg.getNextNode(ctx, current, result)
	}

	return input, nil
}

func (cg *CompiledGraph) getNextNode(ctx context.Context, current string, result any) string {
	currentEdges := cg.GetEdgesFrom(current)
	if len(currentEdges) == 0 {
		return ""
	}

	for _, nextNode := range currentEdges {
		if conditional, ok := cg.Conditional[nextNode]; ok {
			nextStep := conditional.Func(ctx, result)
			if nextStep != "" {
				return nextStep
			}
		}
	}

	return currentEdges[0]
}

type LLMNode struct {
	GraphNode
	Prompt string
	Model  model.Model
}

func NewLLMNode(name string, prompt string, m model.Model) *LLMNode {
	return &LLMNode{
		GraphNode: GraphNode{
			Name:        name,
			Description: prompt,
			Run: func(ctx context.Context, state any) (any, error) {
				return runLLM(ctx, m, prompt, state)
			},
		},
		Prompt: prompt,
		Model:  m,
	}
}

func runLLM(ctx context.Context, m model.Model, prompt string, state any) (any, error) {
	var input string
	switch s := state.(type) {
	case string:
		input = s
	case map[string]any:
		if v, ok := s["input"]; ok {
			input, _ = v.(string)
		}
	}

	messages := []model.Message{
		{Role: model.RoleSystem, Content: prompt},
		{Role: model.RoleUser, Content: input},
	}

	resultChan := m.Stream(ctx, messages, nil)

	var sb strings.Builder
	for result := range resultChan {
		if result.Err != nil {
			return nil, result.Err
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

type ToolNode struct {
	GraphNode
	Tool Tool
}

func NewToolNode(name string, tool Tool) *ToolNode {
	return &ToolNode{
		GraphNode: GraphNode{
			Name:        name,
			Description: tool.Description(),
			Run: func(ctx context.Context, state any) (any, error) {
				var input string
				switch s := state.(type) {
				case string:
					input = s
				case map[string]any:
					if v, ok := s["input"]; ok {
						input, _ = v.(string)
					}
				}
				return tool.Call(ctx, input)
			},
		},
		Tool: tool,
	}
}

type ConditionalLLMNode struct {
	ConditionalGraphNode
	Model  model.Model
	System string
}

func NewConditionalLLMNode(name string, systemPrompt string, m model.Model) *ConditionalLLMNode {
	return &ConditionalLLMNode{
		ConditionalGraphNode: ConditionalGraphNode{
			Name: name,
			Func: func(ctx context.Context, state any) string {
				var input string
				switch s := state.(type) {
				case string:
					input = s
				case map[string]any:
					if v, ok := s["input"]; ok {
						input, _ = v.(string)
					}
				}

				prompt := systemPrompt + "\n\nInput: " + input + "\n\nDetermine the next step:"

				messages := []model.Message{
					{Role: model.RoleUser, Content: prompt},
				}

				resultChan := m.Stream(ctx, messages, nil)
				var sb strings.Builder
				for result := range resultChan {
					if result.Err != nil {
						return ""
					}
					if result.Delta != "" {
						sb.WriteString(result.Delta)
					}
					if result.Content != "" && result.Delta == "" {
						sb.WriteString(result.Content)
					}
				}

				return strings.TrimSpace(sb.String())
			},
		},
		Model:  m,
		System: systemPrompt,
	}
}

var (
	ErrNoStartNode  = ErrNodeNotFound
	ErrNodeNotFound = NotFoundError{"node not found"}
	ErrNoEdges      = NotFoundError{"no outgoing edges"}
)

type NotFoundError struct {
	msg string
}

func (e NotFoundError) Error() string {
	return e.msg
}
