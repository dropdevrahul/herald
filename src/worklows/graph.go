package workflows

import (
	"context"
	"strings"
)

// Edge represents a directed connection between nodes
type Edge struct {
	From string // source node ID
	To   string // destination node ID
}

// NodeWithEdges represents a node with its associated edges
type NodeWithEdges struct {
	ID        string
	Prompt    string
	Edges     []string // IDs of nodes connected from this node
	LoopBack  bool     // whether this node can loop back to itself
}

// Graph represents a directed graph of workflow nodes
type Graph struct {
	ID       string
	Model    model.Model
	nodes    map[string]*NodeWithEdges
	edges    map[string][]Edge
}

// NewGraph creates a new graph-based workflow
func NewGraph(m model.Model, initialNodes ...*NodeWithEdges) *Graph {
	g := &Graph{
		ID:      "default",
		Model:   m,
		nodes:   make(map[string]*NodeWithEdges),
		edges:   make(map[string][]Edge),
	}

	for _, node := range initialNodes {
		if g.nodes[node.ID] != nil {
			g.nodes[node.ID].Prompt = g.nodes[node.ID].Prompt + "\n" + node.Prompt
			g.nodes[node.ID].Edges = append(g.nodes[node.ID].Edges, node.Edges...)
		} else {
			g.nodes[node.ID] = node
		}
	}

	for nodeID, node := range g.nodes {
		g.edges[nodeID] = make([]Edge, 0, len(node.Edges))
		for _, edgeID := range node.Edges {
			g.edges[nodeID] = append(g.edges[nodeID], Edge{From: nodeID, To: edgeID})
		}
	}

	return g
}

// Run executes the graph workflow from a starting node
func (g *Graph) Run(ctx context.Context, userInput string, startNode string) (string, error) {
	state := &State{
		Nodes:   make(map[string]string),
		Current: startNode,
	}

	output, err := g.executeStep(ctx, state.Current, userInput, state)
	if err != nil {
		return "", err
	}

	state.Nodes[state.Current] = output

	for {
		if state.Current == "" {
			break
		}

		input := state.Nodes[state.Current]
		output, err = g.executeStep(ctx, state.Current, input, state)
		if err != nil {
			return "", err
		}

		state.Nodes[state.Current] = output

		// Find next node from edges
		nextNode := g.findNextNode(state.Current)
		if nextNode == "" {
			break
		}

		state.Current = nextNode
		state.Execution[state.Current]++

		if state.Execution[state.Current] >= 10 {
			break // safety limit
		}

		output, err = g.executeStep(ctx, state.Current, output, state)
		if err != nil {
			return "", err
		}

		state.Nodes[state.Current] = output
	}

	return output, nil
}

// findNextNode chooses the next node based on edges and current state
func (g *Graph) findNextNode(current string) string {
	edgeList := g.edges[current]
	if edgeList == nil || len(edgeList) == 0 {
		return ""
	}

	for _, edge := range edgeList {
		// Skip self-loop unless LoopBack is true
		if edge.To == current {
			if g.nodes[current].LoopBack {
				continue
			}
		}

		// Skip nodes already visited more than once
		if g.Execution[edge.To] > 0 {
			continue
		}

		return edge.To
	}

	return ""
}

// executeStep runs the model with the current node
func (g *Graph) executeStep(ctx context.Context, nodeID string, input string, state *State) (string, error) {
	node := g.nodes[nodeID]
	if node == nil {
		return "", nil
	}

	contentChan, errChan := g.Model.Stream(ctx, []model.Message{
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

// State holds the state of graph execution
type State struct {
	Nodes   map[string]string
	Current string
	Execution map[string]int
}
