package agent

import (
	"context"
	"fmt"
	"sync"
)

// NodeID is a unique identifier for a graph node.
type NodeID string

// GraphNode wraps an agent task with a node identity.
type GraphNode struct {
	ID        NodeID
	AgentName string
	Input     *Input
}

// EdgeRule defines a directed edge in AgentGraph with an optional condition.
type EdgeRule struct {
	From      NodeID
	To        NodeID
	Condition func(AgentResult) bool // nil = unconditional
}

// ConditionalRoute is a helper to build a conditional EdgeRule.
func ConditionalRoute(from, to NodeID, fn func(AgentResult) bool) EdgeRule {
	return EdgeRule{From: from, To: to, Condition: fn}
}

// AgentGraph is a directed acyclic graph of agent nodes with conditional routing.
// Each node executes once per Run (cycle guard via visited set).
type AgentGraph struct {
	nodes map[NodeID]*GraphNode
	edges []EdgeRule
	entry NodeID
}

// NewAgentGraph creates an empty AgentGraph with the given entry node.
func NewAgentGraph(entry NodeID) *AgentGraph {
	return &AgentGraph{
		nodes: make(map[NodeID]*GraphNode),
		entry: entry,
	}
}

// AddNode registers a node.
func (g *AgentGraph) AddNode(node *GraphNode) {
	g.nodes[node.ID] = node
}

// AddEdge registers an edge rule.
// Returns an error if either NodeID is unregistered.
func (g *AgentGraph) AddEdge(rule EdgeRule) error {
	if _, ok := g.nodes[rule.From]; !ok {
		return fmt.Errorf("agentgraph: unknown source node %q", rule.From)
	}
	if _, ok := g.nodes[rule.To]; !ok {
		return fmt.Errorf("agentgraph: unknown target node %q", rule.To)
	}
	g.edges = append(g.edges, rule)
	return nil
}

// Run executes the graph starting from the entry node using BFS.
// Each frontier level is executed in parallel.
// A node executes at most once per Run (cycle guard).
func (g *AgentGraph) Run(ctx context.Context, exec *Executor) ([]AgentResult, error) {
	if _, ok := g.nodes[g.entry]; !ok {
		return nil, fmt.Errorf("agentgraph: entry node %q not registered", g.entry)
	}

	visited := make(map[NodeID]bool)
	var allResults []AgentResult
	var mu sync.Mutex

	frontier := []NodeID{g.entry}

	for len(frontier) > 0 {
		if err := ctx.Err(); err != nil {
			return allResults, fmt.Errorf("agentgraph: context cancelled: %w", err)
		}

		// Deduplicate frontier.
		deduped := make([]NodeID, 0, len(frontier))
		for _, id := range frontier {
			if !visited[id] {
				visited[id] = true
				deduped = append(deduped, id)
			}
		}
		if len(deduped) == 0 {
			break
		}

		// Execute current frontier in parallel.
		results := make([]AgentResult, len(deduped))
		var wg sync.WaitGroup
		for i, id := range deduped {
			wg.Add(1)
			go func(idx int, nodeID NodeID) {
				defer wg.Done()
				node := g.nodes[nodeID]
				a, err := exec.GetAgent(node.AgentName, nil)
				if err != nil {
					results[idx] = AgentResult{AgentName: node.AgentName, Err: fmt.Errorf("getting agent: %w", err)}
					return
				}
				r, execErr := a.Execute(ctx, node.Input)
				results[idx] = AgentResult{AgentName: node.AgentName, Result: r, Err: execErr}
			}(i, id)
		}
		wg.Wait()

		mu.Lock()
		allResults = append(allResults, results...)
		mu.Unlock()

		// Build next frontier based on edge conditions.
		var nextFrontier []NodeID
		for i, id := range deduped {
			res := results[i]
			for _, edge := range g.edges {
				if edge.From != id {
					continue
				}
				if edge.Condition == nil || edge.Condition(res) {
					nextFrontier = append(nextFrontier, edge.To)
				}
			}
		}
		frontier = nextFrontier
	}

	return allResults, nil
}
