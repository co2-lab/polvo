package repomap

import "math"

const (
	prDamping    = 0.85
	prMaxIter    = 20
	prTolerance  = 1e-6
)

// pageRank runs power iteration on g and returns a rank per node.
// pers is the personalisation vector (unnormalised; nil/empty = uniform).
// The returned map sums to approximately 1.0.
func pageRank(g *graph, pers map[string]float64) map[string]float64 {
	N := len(g.nodes)
	if N == 0 {
		return nil
	}

	// Collect ordered node list for stable iteration.
	nodes := make([]string, 0, N)
	for n := range g.nodes {
		nodes = append(nodes, n)
	}

	// Normalise personalisation vector.
	persNorm := make(map[string]float64, N)
	persSum := 0.0
	for _, n := range nodes {
		persNorm[n] = pers[n]
		persSum += pers[n]
	}
	if persSum == 0 {
		uniform := 1.0 / float64(N)
		for _, n := range nodes {
			persNorm[n] = uniform
		}
	} else {
		for _, n := range nodes {
			persNorm[n] /= persSum
		}
	}

	// Initialise ranks uniformly.
	rank := make(map[string]float64, N)
	for _, n := range nodes {
		rank[n] = 1.0 / float64(N)
	}

	next := make(map[string]float64, N)

	for iter := 0; iter < prMaxIter; iter++ {
		// Dangling sum: rank mass from nodes with no outgoing edges.
		danglingSum := 0.0
		for _, n := range nodes {
			if g.outWeight[n] == 0 {
				danglingSum += rank[n]
			}
		}

		// Compute next ranks.
		for _, n := range nodes {
			next[n] = (1-prDamping)*persNorm[n] + prDamping*danglingSum*persNorm[n]
		}
		for src, dsts := range g.edges {
			srcRank := rank[src]
			outW := g.outWeight[src]
			if outW == 0 {
				continue
			}
			for dst, w := range dsts {
				next[dst] += prDamping * srcRank * w / outW
			}
		}

		// Check convergence (L1 norm of delta).
		delta := 0.0
		for _, n := range nodes {
			delta += math.Abs(next[n] - rank[n])
		}

		// Swap.
		rank, next = next, rank

		if delta < prTolerance {
			break
		}
	}

	return rank
}
