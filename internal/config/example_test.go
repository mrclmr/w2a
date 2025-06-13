package config

import (
	"testing"

	"github.com/mrclmr/w2a/internal/audio"
)

func TestExampleHasValidTemplatePlaceholders(t *testing.T) {
	example, err := Example()
	if err != nil {
		t.Fatalf("Example(): %v", err)
	}
	_, err = audio.NewTextTmpl(example)
	if err != nil {
		t.Fatalf("unknown template placeholder in example yaml: %v", err)
	}
}
