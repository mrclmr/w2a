package config

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

type I18n struct {
	And    string `yaml:"and"`
	Second *Word  `yaml:"second"`
	Minute *Word  `yaml:"minute"`
}

func (i *I18n) DurToText(d time.Duration) string {
	totalSeconds := int(d.Seconds())
	minutes := totalSeconds / 60
	seconds := totalSeconds % 60

	if minutes == 0 {
		return amountAndUnit(seconds, i.Second)
	}

	if seconds == 0 {
		return amountAndUnit(minutes, i.Minute)
	}

	return fmt.Sprintf("%s %s %s",
		amountAndUnit(minutes, i.Minute),
		i.And,
		amountAndUnit(seconds, i.Second),
	)
}

func amountAndUnit(amount int, word *Word) string {
	if amount == 1 {
		return fmt.Sprintf("1 %s", word.Singular)
	}
	return fmt.Sprintf("%d %s", amount, word.Plural)
}

type i18n I18n

func (i *I18n) UnmarshalYAML(node *yaml.Node) error {
	var y i18n
	err := node.Decode(&y)
	if err != nil {
		return err
	}
	if y.And == "" {
		return keyEmptyError("i18n.and")
	}
	if y.Second == nil {
		return keyEmptyError("i18n.second")
	}
	if y.Minute == nil {
		return keyEmptyError("i18n.minute")
	}

	i.And = y.And
	i.Second = y.Second
	i.Minute = y.Minute
	return nil
}

type Word struct {
	Singular string `yaml:"singular"`
	Plural   string `yaml:"plural"`
}

type word Word

func (w *Word) UnmarshalYAML(node *yaml.Node) error {
	var y word
	err := node.Decode(&y)
	if err != nil {
		return err
	}
	if y.Singular == "" {
		return keyEmptyError("i18n.word.singular")
	}
	if y.Plural == "" {
		return keyEmptyError("i18n.word.plural")
	}

	w.Singular = y.Singular
	w.Plural = y.Plural
	return nil
}
