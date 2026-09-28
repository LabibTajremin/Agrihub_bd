package domain

import (
	"slices"
	"sort"
	"strconv"
)

// Graph is a directed graph of crop-season nodes.
type Graph struct {
	nodes []string
	edges map[string][]string
}

// NewGraph returns an empty graph.
func NewGraph() *Graph { return &Graph{edges: map[string][]string{}} }

// AddNode adds a node once.
func (g *Graph) AddNode(n string) {
	if _, ok := g.edges[n]; !ok {
		g.nodes = append(g.nodes, n)
		g.edges[n] = nil
	}
}

// AddEdge adds from → to (adding both nodes).
func (g *Graph) AddEdge(from, to string) {
	g.AddNode(from)
	g.AddNode(to)
	g.edges[from] = append(g.edges[from], to)
}

// Successors returns the out-neighbours of n.
func (g *Graph) Successors(n string) []string { return g.edges[n] }

// TopoSort orders nodes with Kahn's algorithm (ties by insertion order). A
// cycle yields ErrRotationCycle instead of looping forever.
func (g *Graph) TopoSort() ([]string, error) {
	indeg := make(map[string]int, len(g.nodes))
	for _, n := range g.nodes {
		for _, m := range g.edges[n] {
			indeg[m]++
		}
	}
	var queue, order []string
	for _, n := range g.nodes {
		if indeg[n] == 0 {
			queue = append(queue, n)
		}
	}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		order = append(order, n)
		for _, m := range g.edges[n] {
			indeg[m]--
			if indeg[m] == 0 {
				queue = append(queue, m)
			}
		}
	}
	if len(order) != len(g.nodes) {
		return nil, ErrRotationCycle
	}
	return order, nil
}

// RotationStep is one season of a plan.
type RotationStep struct {
	Season         string
	Crop           string
	NitrogenBefore int
	NitrogenAfter  int
	Score          float64
}

// Rotation is a planned crop sequence.
type Rotation struct {
	Steps []RotationStep
	// Deficit is true when no sequence kept soil nitrogen non-negative.
	Deficit bool
}

// node names a layer (0–9) and crop, e.g. "2:wheat".
func node(layer int, crop string) string { return strconv.Itoa(layer%10) + ":" + crop }

func nodeCrop(n string) string { return n[2:] }

// BuildRotationGraph expands the season sequence into a layered DAG: layer 0
// is the current crop, layer i holds crops grown in seasons[i-1], and every
// crop in layer i links to every different crop in layer i+1.
func BuildRotationGraph(current string, seasons []string, crops []Crop) *Graph {
	g := NewGraph()
	prev := []string{node(0, current)}
	g.AddNode(prev[0])
	for i, season := range seasons {
		var layer []string
		for _, c := range crops {
			if slices.Contains(c.Seasons, season) {
				layer = append(layer, node(i+1, c.Code))
			}
		}
		for _, p := range prev {
			for _, n := range layer {
				if nodeCrop(p) != nodeCrop(n) {
					g.AddEdge(p, n)
				}
			}
		}
		prev = layer
	}
	return g
}

// PlanRotation walks the graph in topological order from start, greedily
// choosing at each layer the successor that keeps the nitrogen balance
// non-negative with the best score (ties: more nitrogen, then crop code). If
// no successor keeps the balance, the most nitrogen-restoring one is taken and
// the plan is flagged Deficit.
func PlanRotation(g *Graph, start string, seasons []string, byCode map[string]Crop, scores map[string]float64, nitrogen int) (Rotation, error) {
	order, err := g.TopoSort()
	if err != nil {
		return Rotation{}, err
	}
	pos := make(map[string]int, len(order))
	for i, n := range order {
		pos[n] = i
	}
	var r Rotation
	cur := start
	for i := 0; i < len(seasons); i++ {
		next := append([]string(nil), g.Successors(cur)...)
		if len(next) == 0 {
			break
		}
		sort.SliceStable(next, func(a, b int) bool { return pos[next[a]] < pos[next[b]] })
		best, bestOK := "", false
		for _, n := range next {
			c := byCode[nodeCrop(n)]
			ok := nitrogen+c.NitrogenDeltaKgHa >= 0
			if best == "" || prefer(n, ok, best, bestOK, byCode, scores) {
				best, bestOK = n, ok
			}
		}
		c := byCode[nodeCrop(best)]
		r.Deficit = r.Deficit || !bestOK
		r.Steps = append(r.Steps, RotationStep{Season: seasons[i], Crop: c.Code, NitrogenBefore: nitrogen,
			NitrogenAfter: nitrogen + c.NitrogenDeltaKgHa, Score: scores[c.Code]})
		nitrogen += c.NitrogenDeltaKgHa
		cur = best
	}
	return r, nil
}

func prefer(n string, ok bool, best string, bestOK bool, byCode map[string]Crop, scores map[string]float64) bool {
	a, b := byCode[nodeCrop(n)], byCode[nodeCrop(best)]
	switch {
	case ok != bestOK:
		return ok
	case !ok: // neither keeps the balance: restore the most nitrogen
		if a.NitrogenDeltaKgHa != b.NitrogenDeltaKgHa {
			return a.NitrogenDeltaKgHa > b.NitrogenDeltaKgHa
		}
	case scores[a.Code] != scores[b.Code]:
		return scores[a.Code] > scores[b.Code]
	case a.NitrogenDeltaKgHa != b.NitrogenDeltaKgHa:
		return a.NitrogenDeltaKgHa > b.NitrogenDeltaKgHa
	}
	return a.Code < b.Code
}

// SeasonsFrom lists n seasons starting at start in calendar order.
func SeasonsFrom(start string, n int) []string {
	order := []string{"aman", "boro", "aus"}
	i := slices.Index(order, start)
	if i < 0 {
		return nil
	}
	out := make([]string, n)
	for k := range n {
		out[k] = order[(i+k)%len(order)]
	}
	return out
}

// SeasonAt returns the season a month falls in: Jul–Oct aman, Nov–Feb boro, Mar–Jun aus.
func SeasonAt(month int) string {
	switch {
	case month >= 7 && month <= 10:
		return "aman"
	case month >= 3 && month <= 6:
		return "aus"
	default:
		return "boro"
	}
}
