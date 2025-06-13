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
