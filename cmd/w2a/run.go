package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/mrclmr/w2a/internal/audio"
	"github.com/mrclmr/w2a/internal/config"
)

const (
	outputDir            = "output-w2a"
	intermediateFilesDir = "w2a-intermediate-files"
)

var (
	underscoreReg      = regexp.MustCompile(`__+`)
	filenameNormalizer = strings.NewReplacer(
		" ", "_",
		"<", "_",
		">", "_",
		":", "_",
		"\"", "_",
		"\\", "_",
		"/", "_",
		"|", "_",
		"?", "_",
		"*", "_",
	)
)

func run(ctx context.Context, cfg *config.Workout) error {
	creator, err := audio.NewFileCreator(
		audio.ToExecCmdCtx(exec.CommandContext),
		cfg.TTS.TTS(),
		cfg.AudioFormat,
		filepath.Join(tempDir(), intermediateFilesDir),
		outputDir,
	)
	if err != nil {
		return err
	}

	i18n := cfg.I18n

	workoutDur, workoutDurWithoutPauses := workoutDurations(cfg)

	tmplValues := audio.TextTmplValues{
		WorkoutExercisesCount:        len(cfg.Exercises),
		WorkoutDuration:              workoutDur,
		WorkoutDurationWithoutPauses: workoutDurWithoutPauses,
	}

	exerciseStartSoundDur := 2 * time.Second
	exerciseNameDur := 4 * time.Second

	start := 5
	countdownDur := time.Duration(start) * time.Second
	countdown := make([]audio.Segment, 0)
	for i := start; 0 < i; i-- {
		countdown = append(countdown,
			&audio.Text{Value: fmt.Sprintf("%d", i), Length: 1 * time.Second},
		)
	}

	pauseDurRemainder := cfg.Pause.Duration - (2*time.Second + countdownDur)

	for i, e := range cfg.Exercises {
		// Pause
		tmplValues.ExerciseDuration = i18n.DurToText(e.Duration)
		tmplValues.ExerciseName = e.Name

		err := creator.TextToAudioFile(ctx,
			slices.Concat(
				[]audio.Segment{
					&audio.Sound{Filename: "start-2929965.wav", Length: 2 * time.Second},
					&audio.Text{Value: cfg.Pause.Text.Replace(tmplValues), Length: pauseDurRemainder},
				},
				countdown,
			),
			fmt.Sprintf("%02d-0-Pause", i+1),
		)
		if err != nil {
			return err
		}

		// Exercise

		startAndName := []audio.Segment{
			&audio.Sound{Filename: "start-2929965.wav", Length: exerciseStartSoundDur},
			&audio.Text{Value: cfg.ExerciseBeginning.Replace(tmplValues), Length: exerciseNameDur},
		}

		var texts []audio.Segment
		for _, text := range e.Texts {
			texts = append(texts,
				&audio.Text{Value: text + ", "},
				&audio.Silence{Length: 1 * time.Second},
			)
		}

		var textsOptHalfTime []audio.Segment
		if e.HalfTime {
			textsOptHalfTime = []audio.Segment{
				&audio.Group{
					Segments: texts,
					Length:   e.Duration/2 - (exerciseStartSoundDur + exerciseNameDur),
				},
				&audio.Text{
					Value:  cfg.HalfTime.Text.Replace(tmplValues),
					Length: cfg.HalfTime.Duration,
				},
				&audio.Sound{Filename: "start-2929965.wav", Length: 1 * time.Second},
				&audio.Silence{Length: e.Duration/2 - (1*time.Second + countdownDur)},
			}
		} else {
			textsOptHalfTime = []audio.Segment{
				&audio.Group{
					Segments: texts,
					Length:   e.Duration - (exerciseStartSoundDur + exerciseNameDur + countdownDur),
				},
			}
		}

		err = creator.TextToAudioFile(ctx,
			slices.Concat(
				startAndName,
				textsOptHalfTime,
				countdown,
			),
			fmt.Sprintf("%02d-1-%s", i+1, sanitizeFilename(e.Name)),
		)
		if err != nil {
			return err
		}
	}

	if cfg.BeforeWorkoutText != nil {
		err := creator.TextToAudioFile(ctx,
			[]audio.Segment{
				&audio.Text{Value: cfg.BeforeWorkoutText.Replace(tmplValues)},
			},
			"00-Before_Workout",
		)
		if err != nil {
			return err
		}
	}

	if cfg.AfterWorkoutText != nil {
		err := creator.TextToAudioFile(ctx,
			[]audio.Segment{
				&audio.Sound{Filename: "success-a1a69bc.wav"},
				&audio.Text{Value: cfg.AfterWorkoutText.Replace(tmplValues)},
			},
			fmt.Sprintf("%02d-After_Workout", len(cfg.Exercises)+1),
		)
		if err != nil {
			return err
		}
	}

	return creator.RemoveOtherFiles()
}

func workoutDurations(cfg *config.Workout) (string, string) {
	var workoutDur time.Duration
	var workoutDurWithoutPauses time.Duration
	for _, e := range cfg.Exercises {
		workoutDur += e.Duration + cfg.Pause.Duration
		workoutDurWithoutPauses += e.Duration
	}
	return cfg.I18n.DurToText(workoutDur), cfg.I18n.DurToText(workoutDurWithoutPauses)
}

func sanitizeFilename(filename string) string {
	return underscoreReg.ReplaceAllString(
		strings.Trim(
			filenameNormalizer.Replace(filename),
			"_"),
		"_")
}

func tempDir() string {
	switch runtime.GOOS {
	case "linux", "darwin":
		return "/tmp"
	default:
		return os.TempDir()
	}
}
