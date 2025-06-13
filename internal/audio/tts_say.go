package audio

import "context"

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
