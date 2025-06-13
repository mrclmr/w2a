package audio

import (
	"bytes"
	"fmt"
	"io"
	goTmpl "text/template"

	"gopkg.in/yaml.v3"
)

type TextTmpl struct {
	str    string
	goTmpl *goTmpl.Template
}

type TextTmplValues struct {
	WorkoutExercisesCount        int
	WorkoutDuration              string
	WorkoutDurationWithoutPauses string
	ExerciseDuration             string
	ExerciseName                 string
}

// NewTextTmpl returns a new template. Pass a Go template string.
func NewTextTmpl(str string) (*TextTmpl, error) {
	t, err := goTmpl.New("").Parse(str)
	if err != nil {
		return nil, err
	}

	err = t.Execute(io.Discard, TextTmplValues{})
	if err != nil {
		return nil, fmt.Errorf("invalid go template syntax used in '%s': %w", str, err)
	}
	return &TextTmpl{
		goTmpl: t,
	}, nil
}

func (t *TextTmpl) Replace(values TextTmplValues) string {
	b := &bytes.Buffer{}

	// Ignore because a possible error is handled in the constructor.
	_ = t.goTmpl.Execute(b, values)
	return b.String()
}

func (t *TextTmpl) UnmarshalYAML(node *yaml.Node) error {
	var str string
	err := node.Decode(&str)
	if err != nil {
		return err
	}
	if str == "" {
		return fmt.Errorf("empty template string")
	}
	template, err := NewTextTmpl(str)
	if err != nil {
		return err
	}
	t.str = str
	t.goTmpl = template.goTmpl
	return nil
}

func (t *TextTmpl) String() string {
	return t.str
}

func Must(t *TextTmpl, err error) *TextTmpl {
	if err != nil {
		panic(err)
	}
	return t
}
