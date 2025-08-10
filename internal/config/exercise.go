package config

import (
	"time"

	"gopkg.in/yaml.v3"
)

type Exercise struct {
	Name                  string        `yaml:"name"`
	Duration              time.Duration `yaml:"duration"`
	Texts                 []string      `yaml:"texts"`
	HalfTime              bool          `yaml:"half_time"`
	PauseDurationOverride time.Duration `yaml:"pause_duration"`
}

type exercise Exercise

func (e *Exercise) UnmarshalYAML(node *yaml.Node) error {
	var y exercise
	err := node.Decode(&y)
	if err != nil {
		return err
	}
	if y.Name == "" {
		return keyEmptyError("exercise.name")
	}
	if y.Duration == 0 {
		return keyEmptyError("exercise.duration")
	}
	if y.Texts != nil && len(y.Texts) == 0 {
		return keyEmptyError("exercise.texts")
	}

	e.Name = y.Name
	e.Duration = y.Duration
	e.Texts = y.Texts
	e.HalfTime = y.HalfTime
	e.PauseDurationOverride = y.PauseDurationOverride
	return nil
}
