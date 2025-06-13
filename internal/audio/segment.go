package audio

import (
	"time"
)

type Segment interface {
	values() []Segment
	value() string
	len() time.Duration
}

type Silence struct {
	Length time.Duration
}

func (s *Silence) values() []Segment {
	panic("Silence has no values")
}

func (s *Silence) value() string {
	panic("Silence has no value")
}

func (s *Silence) len() time.Duration {
	return s.Length
}

type Sound struct {
	Filename string
	Length   time.Duration
}

func (s *Sound) values() []Segment {
	panic("Sound has no values")
}

func (s *Sound) value() string {
	return s.Filename
}

func (s *Sound) len() time.Duration {
	return s.Length
}

type Text struct {
	Value  string
	Length time.Duration
}

func (t *Text) values() []Segment {
	panic("Text has no values")
}

func (t *Text) value() string {
	return t.Value
}

func (t *Text) len() time.Duration {
	return t.Length
}

type Group struct {
	Segments []Segment
	Length   time.Duration
}

func (g *Group) values() []Segment {
	return g.Segments
}

func (g *Group) value() string {
	panic("Group has no value")
}

func (g *Group) len() time.Duration {
	return g.Length
}
