// Package dag provides a directed acyclic graph to run tasks in parallel.
package dag

import (
	"context"
	"fmt"
	"iter"
	"slices"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
)

// Node must be implemented to add a node to the graph.
type Node[T comparable] interface {
	Hash() string
	Name() string
	Run(ctx context.Context, values []T) (T, error)
}

// Dag is a directed acyclic graph.
type Dag[T comparable] struct {
	hashToIdx map[string]int
	nodes     []*node[T]
}

// New return a new Dag.
func New[T comparable]() *Dag[T] {
	return &Dag[T]{
		hashToIdx: make(map[string]int),
	}
}

// RunRootNodes starts execution by running the root nodes.
func (d *Dag[T]) RunRootNodes(ctx context.Context) iter.Seq2[T, error] {
	nodes, err := d.rootNodes()
	if err != nil {
		return func(yield func(T, error) bool) {
			var zero T
			yield(zero, err)
		}
	}
	return runNodes(ctx, nodes)
}

// RunNodes starts execution by running passed nodes.
func (d *Dag[T]) RunNodes(ctx context.Context, nodes []Node[T]) iter.Seq2[T, error] {
	nodesToRun := make([]*node[T], 0)
	for _, n := range nodes {
		idx, ok := d.hashToIdx[n.Hash()]
		if ok {
			nodesToRun = append(nodesToRun, d.nodes[idx])
		}
	}
	return runNodes(ctx, nodesToRun)
}

// AddChain adds a slice of connected nodes. The first node is the root.
func (d *Dag[T]) AddChain(nodes ...Node[T]) error {
	if len(nodes) == 1 {
		d.addNode(nodes[0])
		return nil
	}
	parent := nodes[0]
	for i := 1; i < len(nodes); i++ {
		d.addNode(parent)
		d.addNode(nodes[i])
		err := d.AddEdge(parent, nodes[i])
		if err != nil {
			return err
		}
		parent = nodes[i]
	}
	return nil
}

// AddEdges adds multiple edges between nodes to the directed acyclic graph (Dag).
func (d *Dag[T]) AddEdges(edges [][2]Node[T]) error {
	for _, edge := range edges {
		err := d.AddEdge(edge[0], edge[1])
		if err != nil {
			return err
		}
	}
	return nil
}

// AddEdge adds an edge between nodes to the directed acyclic graph (Dag).
func (d *Dag[T]) AddEdge(parent Node[T], child Node[T]) error {
	parentNodeID := d.addNode(parent)
	childNodeID := d.addNode(child)

	parentNode := d.nodes[parentNodeID]
	childNode := d.nodes[childNodeID]

	// edge already exists
	if d.hasChild(parentNode, childNode.id) {
		return nil
	}
	if d.hasPath(childNode, parentNode) {
		return fmt.Errorf("cyclic dependency: %s to %s", parentNode.name, childNode.name)
	}

	parentNode.children = append(parentNode.children, childNode)
	return nil
}

// addNode adds a node to the directed acyclic graph (Dag).
func (d *Dag[T]) addNode(n Node[T]) (id int) {
	hash := n.Hash()
	id, exists := d.hashToIdx[hash]
	if !exists {
		id = len(d.nodes)
		d.hashToIdx[hash] = id
		d.nodes = append(d.nodes, &node[T]{
			id:      id,
			name:    n.Name(),
			runFunc: n.Run,
		})
	}
	return id
}

// hasPath checks if there is already a path from start to target node (DFS).
func (d *Dag[T]) hasPath(src, dst *node[T]) bool {
	visited := make(map[int]bool)
	return d.dfs(src, dst, visited)
}

func (d *Dag[T]) dfs(src, dst *node[T], visited map[int]bool) bool {
	if src.id == dst.id {
		return true
	}
	visited[src.id] = true
	for _, child := range src.children {
		if !visited[child.id] && d.dfs(child, dst, visited) {
			return true
		}
	}
	return false
}

func (d *Dag[T]) hasChild(parent *node[T], childID int) bool {
	return slices.ContainsFunc(parent.children, func(child *node[T]) bool {
		return child.id == childID
	})
}

func (d *Dag[T]) rootNodes() ([]*node[T], error) {
	childSet := make(map[int]bool)

	for _, n := range d.nodes {
		for _, c := range n.children {
			childSet[c.id] = true
		}
	}

	var roots []*node[T]
	for id, n := range d.nodes {
		if !childSet[id] {
			if len(d.nodes) > 1 && n.children == nil {
				return nil, fmt.Errorf("node %v is orphaned", n)
			}
			roots = append(roots, n)
		}
	}

	return roots, nil
}

func (d *Dag[T]) String() string {
	b := strings.Builder{}

	b.WriteString("digraph dag {\n")

	for _, n := range d.nodes {
		b.WriteString(fmt.Sprintf("    \"%d\" [label=\"", n.id))
		b.WriteString(fmt.Sprintf("name: %s", n.name))
		b.WriteString("\\n")
		b.WriteString(fmt.Sprintf("id: %d", n.id))
		b.WriteString("\\n")
		b.WriteString(fmt.Sprintf("result: %v", n.result))
		b.WriteString("\"];\n")
	}

	for id, n := range d.nodes {
		for _, c := range n.children {
			b.WriteString(fmt.Sprintf("    \"%d\" -> \"%d\";\n", id, c.id))
		}
	}

	b.WriteString("}")
	return b.String()
}

type node[T comparable] struct {
	id       int
	name     string
	children []*node[T]

	lock            sync.Mutex
	runFunc         func(ctx context.Context, values []T) (result T, err error)
	runFuncExecuted bool
	result          T
}

func (n *node[T]) run(ctx context.Context) (T, error) {
	n.lock.Lock()
	defer n.lock.Unlock()

	if n.runFuncExecuted {
		return n.result, nil
	}

	var zeroVal T
	var results []T
	if len(n.children) > 0 {
		rs, err := runChildren(ctx, n.children)
		if err != nil {
			return zeroVal, err
		}
		results = rs
	}

	result, err := n.runFunc(ctx, results)
	if err != nil {
		return zeroVal, err
	}
	n.result = result
	n.runFuncExecuted = true
	return result, nil
}

func runChildren[T comparable](ctx context.Context, children []*node[T]) ([]T, error) {
	results := make([]T, len(children))
	errg, ctx := errgroup.WithContext(ctx)
	for i, n := range children {
		errg.Go(func() error {
			val, err := n.run(ctx)
			if err != nil {
				return err
			}
			results[i] = val
			return nil
		})
	}
	err := errg.Wait()
	if err != nil {
		return nil, err
	}
	return results, nil
}

// runNodes returns an iterator that iterates results as soon as
// contiguous parts from the start are complete.
func runNodes[T comparable](ctx context.Context, nodes []*node[T]) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		lenNodes := len(nodes)
		results := make([]T, lenNodes)
		errs := make([]error, lenNodes)
		done := make([]bool, lenNodes)

		var mu sync.Mutex
		cond := sync.NewCond(&mu)

		for i, n := range nodes {
			go func() {
				val, err := n.run(ctx)
				mu.Lock()
				results[i] = val
				errs[i] = err
				done[i] = true
				cond.Broadcast()
				mu.Unlock()
			}()
		}

		for i := range lenNodes {
			mu.Lock()
			for !done[i] {
				mu.Unlock()
				select {
				case <-ctx.Done():
					return
				default:
				}
				mu.Lock()
				cond.Wait()
			}
			val, err := results[i], errs[i]
			mu.Unlock()

			if !yield(val, err) || err != nil || ctx.Err() != nil {
				return
			}
		}
	}
}
