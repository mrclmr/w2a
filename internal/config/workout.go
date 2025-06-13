package config

import (
	"log/slog"

	"github.com/mrclmr/w2a/internal/audio"
	"gopkg.in/yaml.v3"
)

type Workout struct {
	LogLevel          slog.Level      `yaml:"log_level"`
	TTS               *TTSCmd         `yaml:"tts"`
	AudioFormat       audio.Format    `yaml:"audio_format"`
	I18n              *I18n           `yaml:"i18n"`
	BeforeWorkoutText *audio.TextTmpl `yaml:"before_workout_announce"`
	AfterWorkoutText  *audio.TextTmpl `yaml:"after_workout_announce"`
	Pause             *Announce       `yaml:"pause"`
	HalfTime          *Announce       `yaml:"half_time"`
	ExerciseBeginning *audio.TextTmpl `yaml:"exercise_beginning"`
	Exercises         []Exercise      `yaml:"exercises"`
}

type workout Workout

func (w *Workout) UnmarshalYAML(node *yaml.Node) error {
	var y workout
	err := node.Decode(&y)
	if err != nil {
		return err
	}
	if y.TTS == nil {
		return keyEmptyError("tts")
	}
	if y.Pause == nil {
		return keyEmptyError("pause")
	}
	if y.HalfTime == nil {
		return keyEmptyError("half_time")
	}
	if y.ExerciseBeginning == nil {
		return keyEmptyError("exercise_beginning")
	}
	if y.I18n == nil {
		return keyEmptyError("i18n")
	}
	if len(y.Exercises) == 0 {
		return keyEmptyError("exercises")
	}

	w.LogLevel = y.LogLevel
	w.TTS = y.TTS
	w.AudioFormat = y.AudioFormat
	w.I18n = y.I18n
	w.BeforeWorkoutText = y.BeforeWorkoutText
	w.AfterWorkoutText = y.AfterWorkoutText
	w.Pause = y.Pause
	w.HalfTime = y.HalfTime
	w.ExerciseBeginning = y.ExerciseBeginning
	w.Exercises = y.Exercises
	return nil
}
