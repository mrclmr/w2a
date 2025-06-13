package audio

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
)

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
