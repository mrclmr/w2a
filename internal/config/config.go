package config

import (
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

func Parse(r io.Reader) (*Workout, error) {
	decoder := yaml.NewDecoder(r)
	decoder.KnownFields(true)

	var w Workout
	err := decoder.Decode(&w)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func keyEmptyError(key string) error {
	return fmt.Errorf("key '%s' is missing or value is empty", key)
}
