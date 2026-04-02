package workflows

import (
	"context"
	"dropdevrahul/herald/src/model"
	"strings"
)

type Node struct {
	Prompt string
}

type Tool interface {
	call(input string) (string, error)
}

type WorkflowI interface {
	Run(context.Context, string) (string, error)
}

type ChainingWorkflow struct {
	model model.Model
	nodes []Node
	tools []Tool
}

func (cw *ChainingWorkflow) Run(ctx context.Context, userInput string) (string, error) {
	input := userInput
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

func (cw *ChainingWorkflow) RunNode(ctx context.Context, node *Node, input string) (string, error) {
	contentChan, errChan := cw.model.Stream(ctx, []model.Message{
		{Role: model.RoleSystem, Content: node.Prompt},
		{Role: model.RoleUser, Content: input},
	}, nil)

	var sb strings.Builder
	for content := range contentChan {
		sb.WriteString(content)
	}

	if err := <-errChan; err != nil {
		return "", err
	}

	return sb.String(), nil
}

type RoutingWorkflow struct {
	model model.Model
	nodes []Node
}

type OrchestratorWorkflow struct {
	model model.Model
	nodes []Node
}

func NewChainingWorkflow(m model.Model, nodes []Node) WorkflowI {
	return &ChainingWorkflow{
		model: m,
		nodes: nodes,
	}
}
