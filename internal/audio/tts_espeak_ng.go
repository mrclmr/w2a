package audio

import "context"

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
