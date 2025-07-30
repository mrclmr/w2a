package audio

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type cmdBuilder struct {
	fileCacheBuilder *fileCacheBuilder
	execCmdCtx       ExecCmdCtx
	tempDir          string
	outputDir        string
	tts              *TTS
	audioFormat      Format
}

func newCmdBuilder(
	existingFilesMap map[string]map[string]bool,
	execCmdCtx ExecCmdCtx,
	tempDir string,
	outputDir string,
	tts *TTS,
	audioFormat Format,
) *cmdBuilder {
	return &cmdBuilder{
		fileCacheBuilder: newFileCacheBuilder(existingFilesMap),
		execCmdCtx:       execCmdCtx,
		tempDir:          tempDir,
		outputDir:        outputDir,
		tts:              tts,
		audioFormat:      audioFormat,
	}
}

func (cb *cmdBuilder) ttsCmd(text string) *fileCache {
	switch cb.tts.TTSCmd {
	case Say:
		return cb.fileCacheBuilder.buildCmd(
			cb.execCmdCtx,
			"say",
			[]string{
				// `--data-format=LEF32@22050` is needed for wav.
				// https://stackoverflow.com/questions/9729153/error-on-say-when-output-format-is-wave
				// The comments state that a sample rate higher than 22050 is not recommended.
				"--data-format", "LEF32@22050",
				"--voice", cb.tts.Voice,
				"--output-file", filepath.Join(cb.tempDir, "say-<hash>.wav"),
				text,
			},
		)
	case EspeakNG:
		return cb.fileCacheBuilder.buildCmd(
			cb.execCmdCtx,
			"espeak-ng",
			[]string{
				"-v", cb.tts.Voice,
				"-out", filepath.Join(cb.tempDir, "espeak-ng-<hash>.wav"),
				text,
			},
		)
	default:
	}
	return nil
}

type cmdErr struct {
	err error
}

func (c *cmdErr) CombinedOutput() ([]byte, error) {
	return nil, c.err
}

func (cb *cmdBuilder) soxConcat(filenames []string) *fileCache {
	for i := range filenames {
		filenames[i] = filepath.Join(cb.tempDir, filenames[i])
	}
	return cb.fileCacheBuilder.buildCmd(
		cb.execCmdCtx,
		"sox",
		append(filenames, filepath.Join(cb.tempDir, "concat-<hash>.wav")),
	)
}

func (cb *cmdBuilder) soxSilence(duration time.Duration) *fileCache {
	return cb.fileCacheBuilder.buildCmd(
		func(ctx context.Context, name string, args ...string) Cmd {
			if duration <= 0 {
				return &cmdErr{errors.New("negative or zero duration for silence")}
			}
			return cb.execCmdCtx(ctx, name, args...)
		},
		"sox",
		[]string{
			"-n",
			"-r",
			"22050",
			filepath.Join(cb.tempDir, fmt.Sprintf("silence_%s-<hash>.wav", duration)),
			"trim",
			"0.0",
			fmt.Sprintf("%.2f", duration.Seconds()),
		},
	)
}

type cmdNoop struct{}

func (c *cmdNoop) CombinedOutput() ([]byte, error) {
	return nil, nil
}

// soxExtendLength needs a specific implementation.
// If nothing is to do reading every file consumes time (~1s vs. instant).
// This code is ugly and needs refactoring.
func (cb *cmdBuilder) soxExtendLength(inputFile string, extendedLength time.Duration) *fileCache {
	if extendedLength <= 0 {
		return cb.fileCacheBuilder.buildNoop(inputFile)
	}
	ext := filepath.Ext(inputFile)
	nameNoExt := strings.TrimSuffix(inputFile, ext)
	filePadded := fmt.Sprintf("%s_extended-%s-<hash>%s", nameNoExt, extendedLength, ext)
	filePaddedPath := filepath.Join(cb.tempDir, filePadded)
	inputFilePath := filepath.Join(cb.tempDir, inputFile)
	cmdStr := "sox"
	args := []string{
		inputFilePath,
		filePaddedPath,
		// The calculated length argument is inserted as last.
		"pad", "0",
	}
	args, outFile, hash := replaceHash(cmdStr, args)

	filePaddedPath = filepath.Join(cb.tempDir, outFile)

	return cb.fileCacheBuilder.buildSoxExtended(
		func(ctx context.Context, name string, args ...string) Cmd {
			arguments := []string{"--i", "-D", inputFilePath}

			slog.Debug("execute", "cmd", strings.Join(append([]string{cmdStr}, arguments...), " "))
			out, err := cb.execCmdCtx(
				ctx,
				cmdStr,
				arguments...,
			).CombinedOutput()
			if err != nil {
				return &cmdErr{err: cmdError(cmdStr, arguments, out)}
			}

			var float float64
			for l := range strings.Lines(string(out)) {
				float, err = strconv.ParseFloat(strings.TrimSuffix(l, "\n"), 64)
				if err == nil {
					break
				}
			}
			if err != nil {
				return &cmdErr{err: fmt.Errorf("%w: no parsable float in\n%s", err, string(out))}
			}

			length := time.Duration(float * float64(time.Second))
			addLength := extendedLength - length
			if addLength <= 0 {
				err = copyFile(inputFilePath, filePaddedPath)
				if err != nil {
					return &cmdErr{err: err}
				}
				return &cmdNoop{}
			}

			args = append(args, fmt.Sprintf("%f", addLength.Seconds()))
			slog.Debug("execute", "cmd", strings.Join(append([]string{cmdStr}, args...), " "))

			return cb.execCmdCtx(
				ctx,
				name,
				args...,
			)
		},
		cmdStr,
		args,
		outFile,
		hash,
	)
}

func (cb *cmdBuilder) convert(wavFile string, name string) (fileOperation, node, error) {
	switch cb.audioFormat {
	case Wav:
		return cb.fileCacheBuilder.buildCopyWav(wavFile, name)
	case M4a:
		return cb.fileCacheBuilder.buildConvertCmd(
			cb.execCmdCtx,
			"afconvert",
			[]string{
				// For macOS Music App (iTunes) compatibility use m4af
				// despite it is described as lossless.
				// mp4f is incompatible with macOS Music App.
				"--file", "m4af",
				"--data", "aac",
				"--quality", "127",
				"--strategy", "2",
				filepath.Join(cb.tempDir, wavFile),
				filepath.Join(cb.outputDir, name+"-<hash>.m4a"),
			},
		)
	case Mp3:
		return cb.fileCacheBuilder.buildConvertCmd(
			cb.execCmdCtx,
			"ffmpeg",
			[]string{
				"-i",
				filepath.Join(cb.tempDir, wavFile),
				"-ab", "256k", "-ar", "44100", "-ac", "2",
				filepath.Join(cb.outputDir, name+"-<hash>.mp3"),
			},
		)
	default:
		return 0, nil, errors.New("unsupported audio format")
	}
}

func cmdError(cmd string, args []string, out []byte) error {
	return fmt.Errorf("err: %s %s\n%s",
		cmd,
		strings.Join(args, " "),
		strings.SplitN(string(out), "\n", 1)[0],
	)
}
