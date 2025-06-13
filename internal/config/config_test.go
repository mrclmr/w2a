package config

import (
	"strings"
	"testing"
)

func TestParseExample(t *testing.T) {
	example, err := Example()
	if err != nil {
		t.Fatalf("Example(): %v", err)
	}
	_, err = Parse(strings.NewReader(example))
	if err != nil {
		t.Fatalf("Parse(): %v", err)
	}
}
