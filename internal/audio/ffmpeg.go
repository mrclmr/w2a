package audio

import (
	"context"
)

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
