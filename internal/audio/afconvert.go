package audio

import (
	"context"
)

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
