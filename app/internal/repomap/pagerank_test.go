package repomap

import (
	"math"
	"testing"
)

func rankSum(rank map[string]float64) float64 {
	s := 0.0
	for _, v := range rank {
		s += v
	}
	return s
}

func TestPageRank_SimpleLinearGraph(t *testing.T) {
	g := newGraph()
	g.addEdge("A", "B", 1)
	g.addEdge("B", "C", 1)

	rank := pageRank(g, nil)

	if rank["C"] <= rank["B"] {
		t.Errorf("expected rank[C] > rank[B]; got C=%.4f B=%.4f", rank["C"], rank["B"])
	}
	if rank["B"] <= rank["A"] {
		t.Errorf("expected rank[B] > rank[A]; got B=%.4f A=%.4f", rank["B"], rank["A"])
	}
}

func TestPageRank_CycleConverges(t *testing.T) {
	g := newGraph()
	g.addEdge("A", "B", 1)
	g.addEdge("B", "C", 1)
	g.addEdge("C", "A", 1)

	rank := pageRank(g, nil)

	// In a balanced cycle, all ranks should be approximately equal.
	if math.Abs(rank["A"]-rank["B"]) > 0.01 {
		t.Errorf("cycle ranks not equal: A=%.4f B=%.4f", rank["A"], rank["B"])
	}
	sum := rankSum(rank)
	if math.Abs(sum-1.0) > 0.01 {
		t.Errorf("rank sum = %.4f, want ≈1.0", sum)
	}
}

func TestPageRank_DanglingNode(t *testing.T) {
	g := newGraph()
	g.addNode("alone")
	g.addEdge("A", "B", 1)

	rank := pageRank(g, nil)

	if rank["alone"] <= 0 {
		t.Error("dangling node should have positive rank")
	}
	if math.IsNaN(rank["alone"]) {
		t.Error("dangling node rank is NaN")
	}
}

func TestPageRank_PersonalizationSeed(t *testing.T) {
	g := newGraph()
	g.addEdge("chatFile", "util", 1)
	g.addEdge("other", "util", 1)

	pers := map[string]float64{"chatFile": 100}
	rank := pageRank(g, pers)

	// util is referenced from a high-personalization file → should rank highly.
	if rank["util"] <= rank["other"] {
		t.Errorf("expected rank[util] > rank[other]; util=%.4f other=%.4f", rank["util"], rank["other"])
	}
}

func TestPageRank_Convergence(t *testing.T) {
	// Chain of 10 nodes.
	g := newGraph()
	nodes := []string{"n0", "n1", "n2", "n3", "n4", "n5", "n6", "n7", "n8", "n9"}
	for i := 0; i < len(nodes)-1; i++ {
		g.addEdge(nodes[i], nodes[i+1], 1)
	}

	rank := pageRank(g, nil)
	if len(rank) != len(nodes) {
		t.Errorf("expected %d rank entries, got %d", len(nodes), len(rank))
	}
	// Just verify no NaN and sum ≈ 1.
	for n, r := range rank {
		if math.IsNaN(r) {
			t.Errorf("rank[%s] is NaN", n)
		}
	}
}

func TestPageRank_EmptyGraph(t *testing.T) {
	g := newGraph()
	rank := pageRank(g, nil)
	if rank != nil && len(rank) > 0 {
		t.Errorf("expected nil/empty for empty graph, got %v", rank)
	}
}

func TestPageRank_RankSumsToOne(t *testing.T) {
	g := newGraph()
	g.addEdge("A", "B", 2)
	g.addEdge("B", "C", 1)
	g.addEdge("C", "A", 3)
	g.addNode("D") // dangling

	rank := pageRank(g, nil)
	sum := rankSum(rank)
	if math.Abs(sum-1.0) > 0.01 {
		t.Errorf("rank sum = %.6f, want ≈1.0", sum)
	}
}
