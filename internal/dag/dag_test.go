package dag_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mrclmr/w2a/internal/dag"
)

type sourceExecOnce struct {
	executedTimes atomic.Int32
}

func (c *sourceExecOnce) Run(_ context.Context, _ []int) (int, error) {
	time.Sleep(1 * time.Millisecond)
	c.executedTimes.Add(1)
	return 1, nil
}

func (c *sourceExecOnce) Name() string {
	return "sourceExecOnce"
}

func (c *sourceExecOnce) Hash() string {
	return "sourceExecOnce"
}

type rootNode struct {
	id string
}

func (r *rootNode) Run(_ context.Context, results []int) (int, error) {
	return results[0], nil
}

func (r *rootNode) Name() string {
	return r.id
}

func (r *rootNode) Hash() string {
	return r.id
}

func TestDag_LeafNodesOnlyCalledOnce(t *testing.T) {
	d := dag.New[int]()
	srcInt := &sourceExecOnce{}

	rootsLen := 50

	edges := make([][2]dag.Node[int], rootsLen)

	for i := range rootsLen {
		root := &rootNode{id: fmt.Sprintf("root%02d", i)}
		edges[i] = [2]dag.Node[int]{root, srcInt}
	}

	err := d.AddEdges(edges)
	if err != nil {
		t.Fatalf("failed to add edge: %v", err)
	}

	length := 0
	for result, err := range d.RunRootNodes(t.Context()) {
		if err != nil {
			t.Fatalf("failed to run root nodes: %v", err)
		}
		if result != 1 {
			t.Fatalf("failed to run root nodes: want 1, got %d", result)
		}
		length++
	}
	if length != rootsLen {
		t.Fatalf("failed to run root nodes: want %d, got %d", rootsLen, length)
	}

	executed := srcInt.executedTimes.Load()
	if executed != 1 {
		t.Fatalf("failed to run sourceInt node: want 1, got %d", executed)
	}
}

type sumInt struct {
	value string
}

func (s *sumInt) Run(_ context.Context, values []int) (int, error) {
	sum := 0
	for i := range values {
		sum += values[i]
	}
	return sum, nil
}

func (s *sumInt) Name() string {
	return s.value
}

func (s *sumInt) Hash() string {
	return s.value
}

type sourceInt struct {
	value string
}

func (s *sourceInt) Run(_ context.Context, _ []int) (int, error) {
	return 1, nil
}

func (s *sourceInt) Name() string {
	return s.value
}

func (s *sourceInt) Hash() string {
	return s.value
}

func TestDag_CorrectValues(t *testing.T) {
	d := dag.New[int]()

	sumRoot1 := &sumInt{value: "sum1"}
	sumRoot2 := &sumInt{value: "sum1"}
	sum2 := &sumInt{value: "sum2"}
	sum3 := &sumInt{value: "sum3"}

	source1 := &sourceInt{value: "source1"}
	source2 := &sourceInt{value: "source2"}
	source3 := &sourceInt{value: "source3"}
	source4 := &sourceInt{value: "source4"}

	chains := [][]dag.Node[int]{
		{sumRoot1, sum2, source1},
		{sumRoot1, sum2, source2},
		{sumRoot1, sum3, source3},
		{sumRoot1, sum3, source4},
		{sumRoot2, sum2, source1},
		{sumRoot2, sum2, source2},
		{sumRoot2, sum3, source3},
		{sumRoot2, sum3, source4},
	}
	for _, edge := range chains {
		err := d.AddChain(edge...)
		if err != nil {
			t.Fatalf("failed to add chain: %v", err)
		}
	}

	for value, err := range d.RunRootNodes(t.Context()) {
		if err != nil {
			t.Fatalf("expected no error: %v", err)
		}
		if value != 4 {
			t.Fatalf("failed to get correct return value: expected 4, got %d", value)
		}
	}
}

func TestDag_OrphanedNode(t *testing.T) {
	d := dag.New[int]()

	sum := &sumInt{value: "sum"}
	source1 := &sourceInt{value: "source1"}
	source2 := &sourceInt{value: "source2"}

	orphaned := &sourceInt{value: "orphaned"}

	chains := [][]dag.Node[int]{
		{sum, source1},
		{sum, source2},
		{orphaned},
	}
	for _, chain := range chains {
		err := d.AddChain(chain...)
		if err != nil {
			t.Fatalf("failed to add chain: %v", err)
		}
	}

	for _, err := range d.RunRootNodes(t.Context()) {
		if err == nil {
			t.Fatal("expected orphaned node error")
		}
	}
}

func TestDag_AddChain(t *testing.T) {
	d := dag.New[int]()

	sum1 := &sumInt{value: "sum1"}
	sum2 := &sumInt{value: "sum2"}
	sum3 := &sumInt{value: "sum3"}

	source1 := &sourceInt{value: "source1"}
	source2 := &sourceInt{value: "source2"}
	source3 := &sourceInt{value: "source3"}
	source4 := &sourceInt{value: "source4"}

	chains := [][]dag.Node[int]{
		{sum3, sum1, source1},
		{sum3, sum1, source2},
		{sum3, sum2, source3},
		{sum3, sum2, source4},
		// duplicated chain is ignored
		{sum3, sum2, source4},
	}
	for _, chain := range chains {
		err := d.AddChain(chain...)
		if err != nil {
			t.Fatalf("failed to add chain: %v", err)
		}
	}

	for value, err := range d.RunRootNodes(t.Context()) {
		if err != nil {
			t.Fatalf("expected no error: %v", err)
		}
		if value != 4 {
			t.Fatalf("failed to get correct return value: expected 4, got %d", value)
		}
	}
}

func TestDag_CyclicDependency(t *testing.T) {
	d := dag.New[int]()

	source1 := &sourceInt{value: "source1"}
	source2 := &sourceInt{value: "source2"}

	edges := [][2]dag.Node[int]{
		{source1, source2},
		{source2, source1},
	}
	err := d.AddEdges(edges)
	if err == nil {
		t.Fatalf("failed to detect cyclic dependency")
	}
}
