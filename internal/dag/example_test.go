package dag_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/mrclmr/w2a/internal/dag"
)

type myNode struct {
	value string
}

func (m *myNode) Run(_ context.Context, values []string) (string, error) {
	if len(values) == 0 {
		return m.value, nil
	}
	return strings.Join(values, "+++"), nil
}

func (m *myNode) Name() string {
	return m.value
}

func (m *myNode) Hash() string {
	hash := sha256.Sum256([]byte(m.value))
	return hex.EncodeToString(hash[:4])[:7]
}

func Example_graphviz() {
	d := dag.New[string]()
	source1 := &myNode{value: "source1"}
	source2 := &myNode{value: "source2"}
	concat := &myNode{value: "concat"}

	chains := [][]dag.Node[string]{
		{concat, source1},
		{concat, source2},
	}
	for _, chain := range chains {
		_ = d.AddChain(chain...)
	}

	for res := range d.RunRootNodes(context.Background()) {
		fmt.Printf("// Result: %s\n", res)
	}
	fmt.Println(d)
	// Output:
	// // Result: source1+++source2
	// digraph dag {
	//     "0" [label="name: concat\nid: 0\nresult: source1+++source2"];
	//     "1" [label="name: source1\nid: 1\nresult: source1"];
	//     "2" [label="name: source2\nid: 2\nresult: source2"];
	//     "0" -> "1";
	//     "0" -> "2";
	// }
}
