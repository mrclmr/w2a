package config

import (
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/mrclmr/w2a/internal/audio"
)

type Announce struct {
	Text     *audio.TextTmpl `yaml:"text"`
	Duration time.Duration   `yaml:"duration"`
}

type announce Announce

func (a *Announce) UnmarshalYAML(node *yaml.Node) error {
	var y announce
	err := node.Decode(&y)
	if err != nil {
		return err
	}
	if y.Text == nil {
		return keyEmptyError("announce.text")
	}

	a.Text = y.Text
	a.Duration = y.Duration
	return nil
}
