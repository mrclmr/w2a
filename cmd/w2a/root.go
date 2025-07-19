package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	"github.com/mrclmr/w2a/internal/log"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

func Execute(version string) {
	rootCmd, err := newRootCmd(version)
	checkErr(err)

	err = rootCmd.Execute()
	checkErr(err)
}

func newRootCmd(
	version string,
) (*cobra.Command, error) {
	rootCmd := &cobra.Command{
		Version:           version,
		Use:               "w2a",
		Short:             "Convert workout yaml to audio files",
		Long:              "Convert workout yaml to audio files.",
		SilenceUsage:      true,
		SilenceErrors:     true,
		DisableAutoGenTag: true,
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true,
		},
		Args:              cobra.RangeArgs(0, 1),
		ValidArgsFunction: autoComplete,
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("example") {
				example, err := config.Example()
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(os.Stdout, example)
				return err
			}
			if len(args) != 1 {
				return errors.New("argument missing: path to yaml file")
			}
			path := args[0]
			if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("configuration not found: %w", err)
			}
			f, err := os.OpenFile(path, os.O_RDONLY, 0o600)
			if err != nil {
				return err
			}
			defer func() {
				_ = f.Close()
			}()
			cfg, err := config.Parse(f)
			if err != nil {
				return err
			}
			switch cfg.LogLevel {
			case slog.LevelInfo:
				slog.SetDefault(slog.New(log.NewMsgHandler(os.Stdout, cfg.LogLevel)))
			default:
				slog.SetLogLoggerLevel(cfg.LogLevel)
			}
			return run(cmd.Context(), cfg)
		},
	}

	// https://github.com/spf13/cobra/blob/6dec1ae26659a130bdb4c985768d1853b0e1bc06/command.go#L2064
	rootCmd.SetVersionTemplate(`{{with .DisplayName}}{{printf "%s " .}}{{end}}{{printf "version %s" .Version}}

Sound Credits
* Race Start (start-2929965.wav) by JustInvoke -- https://freesound.org/s/446142/ -- License: Attribution 4.0
* success.wav (success-a1a69bc.wav) by maxmakessounds -- https://freesound.org/s/353546/ -- License: Attribution 4.0
`)

	rootCmd.Flags().BoolP("example", "e", false, "Print example workout yaml")

	rootCmd.AddCommand(newManCmd(rootCmd))

	return rootCmd, nil
}

func checkErr(err error) {
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func autoComplete(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return []string{"yml", "yaml"}, cobra.ShellCompDirectiveFilterFileExt
}

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

func newManCmd(rootCmd *cobra.Command) *cobra.Command {
	// https://unix.stackexchange.com/questions/3586/what-do-the-numbers-in-a-man-page-mean
	return &cobra.Command{
		Use:                   "man",
		Short:                 "Generate man pages",
		SilenceUsage:          true,
		Hidden:                true,
		DisableFlagsInUseLine: true,
		Example:               "w2a man . && cat w2a.1",
		Args:                  cobra.ExactArgs(1),
		ValidArgsFunction:     cobra.NoFileCompletions,
		RunE: func(_ *cobra.Command, args []string) error {
			path := args[0]
			err := doc.GenManTree(rootCmd, nil, path)
			if err != nil {
				return err
			}
			return nil
		},
	}
}
