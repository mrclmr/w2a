package audio

import (
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

//go:generate go run golang.org/x/tools/cmd/stringer@latest -type Format
type Format int

const (
	M4a Format = iota
	Mp3
	Wav
	Unknown
)

func (a *Format) UnmarshalYAML(node *yaml.Node) error {
	var y string
	err := node.Decode(&y)
	if err != nil {
		return err
	}
	for i := range Unknown {
		if strings.EqualFold(i.String(), y) {
			*a = i
			return nil
		}
	}
	return fmt.Errorf("unknown audio format '%s'", y)
}
