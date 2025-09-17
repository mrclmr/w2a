package config

import (
	"fmt"
	"runtime"
	"slices"

	"github.com/mrclmr/w2a/internal/audio"

	"go.yaml.in/yaml/v3"
)

type TTSCmd struct {
	SayVoice      string `yaml:"say_voice"`
	ESpeakNGVoice string `yaml:"espeak_ng_voice"`
	CustomCommand string `yaml:"custom_command"`
}

func (t *TTSCmd) TTS() *audio.TTS {
	if t.SayVoice != "" {
		return &audio.TTS{
			TTSCmd: audio.Say,
			Voice:  t.SayVoice,
		}
	}
	if t.ESpeakNGVoice != "" {
		return &audio.TTS{
			TTSCmd: audio.EspeakNG,
			Voice:  t.ESpeakNGVoice,
		}
	}
	return &audio.TTS{
		TTSCmd: audio.Custom,
		Voice:  t.CustomCommand,
	}
}

type ttsCmd TTSCmd

func (t *TTSCmd) UnmarshalYAML(node *yaml.Node) error {
	var y ttsCmd
	err := node.Decode(&y)
	if err != nil {
		return err
	}

	if runtime.GOOS != "darwin" && y.SayVoice != "" {
		return fmt.Errorf("tts.say_voice is only available on macOS")
	}

	if err := checkOneSet(y.SayVoice, y.ESpeakNGVoice, y.CustomCommand); err != nil {
		return err
	}

	t.SayVoice = y.SayVoice
	t.ESpeakNGVoice = y.ESpeakNGVoice
	t.CustomCommand = y.CustomCommand
	return nil
}

func checkOneSet(args ...string) error {
	args = slices.DeleteFunc(args, func(s string) bool {
		return s == ""
	})
	if len(args) != 1 {
		return fmt.Errorf("set only one: tts.say_voice, tts.espeak_ng_voice or tts.custom_command")
	}
	return nil
}
