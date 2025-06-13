package audio

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
)

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
