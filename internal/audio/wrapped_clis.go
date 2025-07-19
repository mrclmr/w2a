package audio

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// extendLength does not use cmd because files would be read everytime
// to get the hash. If nothing is to do reading every file consumes time (~1s vs. instant).
func (c *FileCreator) extendLength(ctx context.Context, file string, extendedLength time.Duration) (string, fileOperation, error) {
	if extendedLength == 0 {
		return file, noop, nil
	}

	ext := filepath.Ext(file)
	nameNoExt := strings.TrimSuffix(file, ext)

	filePadded := fmt.Sprintf("%s_extended-%s-%s%s", nameNoExt, extendedLength, hashShort(file, extendedLength), ext)
	filePaddedPath := filepath.Join(c.tempDir, filePadded)

	exists, op, err := c.useExistingFile(filePaddedPath)
	if err != nil {
		return "", 0, err
	}
	if exists {
		return filePadded, op, nil
	}

	length, err := c.length(ctx, file)
	if err != nil {
		return "", 0, err
	}

	addLength := extendedLength - length
	if addLength <= 0 {
		return file, 0, nil
	}

	arguments := []string{
		filepath.Join(c.tempDir, file),
		filePaddedPath,
		"pad", "0", fmt.Sprintf("%f", addLength.Seconds()),
	}

	slog.Debug("execute", "cmd", strings.Join(append([]string{"sox"}, arguments...), " "))

	out, err := c.execCmdCtx(
		ctx,
		"sox",
		arguments...,
	).CombinedOutput()
	if err != nil {
		return "", 0, cmdError("sox", arguments, out)
	}
	return filePadded, created, nil
}

func (c *FileCreator) length(ctx context.Context, file string) (time.Duration, error) {
	arguments := []string{
		"--i",
		"-D",
		filepath.Join(c.tempDir, file),
	}

	slog.Debug("execute", "cmd", strings.Join(append([]string{"sox"}, arguments...), " "))
	out, err := c.execCmdCtx(
		ctx,
		"sox",
		arguments...,
	).CombinedOutput()
	if err != nil {
		return 0, cmdError("sox", arguments, out)
	}

	var float float64
	for l := range strings.Lines(string(out)) {
		float, err = strconv.ParseFloat(strings.TrimSuffix(l, "\n"), 64)
		if err == nil {
			break
		}
	}
	if err != nil {
		return 0, fmt.Errorf("%w: no parsable float in\n%s", err, string(out))
	}
	return time.Duration(float * float64(time.Second)), nil
}

func (c *FileCreator) concat(ctx context.Context, filenames ...string) (string, fileOperation, error) {
	return c.cmd(
		ctx,
		args{
			cmd:              "sox",
			argsBeforeOutput: filenames,
			outPath:          c.tempDir,
			outFilename:      "concat",
			outFileExt:       "wav",
		})
}

// https://billposer.org/Linguistics/Computation/SoxTutorial.html#silence
func (c *FileCreator) silence(ctx context.Context, duration time.Duration) (string, fileOperation, error) {
	if duration <= 0 {
		return "", 0, errors.New("negative or zero duration for silence")
	}
	return c.cmd(
		ctx,
		args{
			cmd:              "sox",
			argsBeforeOutput: []string{"-n", "-r", "22050"},
			outPath:          c.tempDir,
			outFilename:      fmt.Sprintf("silence_%s", duration),
			outFileExt:       "wav",
			argsAfterOutput:  []string{"trim", "0.0", fmt.Sprintf("%.2f", duration.Seconds())},
		})
}

func (c *FileCreator) say(ctx context.Context, voice string, text string) (string, fileOperation, error) {
	// `--data-format=LEF32@22050` is needed for wav.
	// https://stackoverflow.com/questions/9729153/error-on-say-when-output-format-is-wave
	// The comments state that a sample rate higher than 22050 is not recommended.
	dataFormat := "LEF32@22050"

	return c.cmd(
		ctx,
		args{
			cmd: "say",
			argsBeforeOutput: []string{
				"--data-format", dataFormat,
				"--voice", voice,
				"--output-file",
			},
			outPath:         c.tempDir,
			outFilename:     "say",
			outFileExt:      "wav",
			argsAfterOutput: []string{text},
		})
}

func (c *FileCreator) espeakNG(ctx context.Context, voice string, text string) (string, fileOperation, error) {
	return c.cmd(
		ctx,
		args{
			cmd:              "espeak-ng",
			argsBeforeOutput: []string{"-v", voice, "-out"},
			outPath:          c.tempDir,
			outFilename:      "espeak-ng",
			outFileExt:       "wav",
			argsAfterOutput:  []string{text},
		})
}

func (c *FileCreator) custom(ctx context.Context, text string) (string, fileOperation, error) {
	cmd := c.tts.Voice
	if err := checkFmtArg(cmd, "%[1]s"); err != nil {
		return "", 0, err
	}

	if err := checkFmtArg(cmd, "%[2]s"); err != nil {
		return "", 0, err
	}

	name := fmt.Sprintf("%s-%s.wav", strings.SplitN(cmd, " ", 2)[0], hashShort(cmd, text))
	path := filepath.Join(c.tempDir, name)

	exists, op, err := c.useExistingFile(path)
	if err != nil {
		return "", 0, err
	}
	if exists {
		return name, op, nil
	}

	slog.Debug("execute", "cmd", text)
	out, err := exec.CommandContext(
		ctx,
		fmt.Sprintf(cmd, path, text),
	).CombinedOutput()
	if err != nil {
		return "", 0, cmdError(cmd, nil, out)
	}
	return path, created, nil
}

func checkFmtArg(cmd, fmtArg string) error {
	if !strings.Contains(cmd, fmtArg) {
		return fmt.Errorf("%s does not contain %s", cmd, fmtArg)
	}
	return nil
}

func (c *FileCreator) ffmpeg(ctx context.Context, wavFile string, name string) (string, fileOperation, error) {
	return c.cmd(
		ctx,
		args{
			cmd:              "ffmpeg",
			argsBeforeInput:  []string{"-i"},
			inputPath:        c.tempDir,
			inputFilename:    wavFile,
			argsBeforeOutput: []string{"-ab", "256k", "-ar", "44100", "-ac", "2"},
			outPath:          c.outputDir,
			outFilename:      name,
			outFileExt:       "mp3",
		})
}

func (c *FileCreator) afconvert(ctx context.Context, wavFile string, name string) (string, fileOperation, error) {
	return c.cmd(
		ctx,
		args{
			cmd: "afconvert",
			argsBeforeInput: []string{
				// For macOS Music App (iTunes) compatibility use m4af
				// despite it is described as lossless.
				// mp4f is incompatible with macOS Music App.
				"--file", "m4af",
				"--data", "aac",
				"--quality", "127",
				"--strategy", "2",
			},
			inputPath:     c.tempDir,
			inputFilename: wavFile,
			outPath:       c.outputDir,
			outFilename:   name,
			outFileExt:    "m4a",
		})
}

type args struct {
	cmd              string
	argsBeforeInput  []string
	inputPath        string
	inputFilename    string
	argsBeforeOutput []string
	outPath          string
	outFilename      string
	outFileExt       string
	argsAfterOutput  []string
}

func (a *args) inFilePath() []string {
	if a.inputPath != "" && a.inputFilename != "" {
		return []string{filepath.Join(a.inputPath, a.inputFilename)}
	}
	return nil
}

func (a *args) outFile() (string, error) {
	if a.inputFilename == "" && a.outFilename == "" {
		return "", errors.New("no input and no output file name")
	}
	if a.inputFilename != "" && a.outFilename == "" {
		return strings.TrimSuffix(a.inputFilename, filepath.Ext(a.inputFilename)) + "." + a.outFileExt, nil
	}

	return a.outFilename + "-" + hashShort(
		a.cmd,
		a.argsBeforeInput,
		a.inputPath,
		a.inputFilename,
		a.argsBeforeOutput,
		a.outFileExt,
		a.argsAfterOutput,
	) + "." + a.outFileExt, nil
}

func (c *FileCreator) cmd(ctx context.Context, args args) (string, fileOperation, error) {
	outName, err := args.outFile()
	if err != nil {
		return "", 0, err
	}

	outPath := filepath.Join(args.outPath, outName)

	exists, op, err := c.useExistingFile(outPath)
	if err != nil {
		return "", 0, err
	}
	if exists {
		return outName, op, nil
	}

	inPath := args.inFilePath()
	concatArgs := slices.Concat(args.argsBeforeInput, inPath, args.argsBeforeOutput, []string{outPath}, args.argsAfterOutput)
	slog.Debug("execute", "cmd", strings.Join(append([]string{args.cmd}, concatArgs...), " "))

	out, err := c.execCmdCtx(
		ctx,
		args.cmd,
		concatArgs...,
	).CombinedOutput()
	if err != nil {
		return "", 0, cmdError(args.cmd, concatArgs, out)
	}
	return outName, created, nil
}
