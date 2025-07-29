package audio

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type cmd struct {
	execCmdCtx ExecCmdCtx
	cmdStr     string
	args       []string
	outFile    string
	hash       string
}

func newCmd(
	execCmdCtx ExecCmdCtx,
	cmdStr string,
	args []string,
) *cmd {
	c := &cmd{
		execCmdCtx: execCmdCtx,
		cmdStr:     cmdStr,
		args:       args,
	}
	c.createOutputFilename()
	return c
}

func (c *cmd) createOutputFilename() {
	hash := hashShort(c.cmdStr, argsBasePath(c.args))

	// Replace <hash> with actual hash and
	// store the new filename and hash for access.
	for i, arg := range c.args {
		if strings.Contains(arg, "<hash>") {
			outputFilePath := strings.ReplaceAll(arg, "<hash>", hash)
			c.args[i] = outputFilePath
			c.outFile = filepath.Base(outputFilePath)
			c.hash = hash
			break
		}
	}
}

func argsBasePath(argsOrg []string) []string {
	argsNew := make([]string, len(argsOrg))
	copy(argsNew, argsOrg)
	for i := range argsNew {
		if strings.Contains(argsNew[i], string(filepath.Separator)) {
			argsNew[i] = filepath.Base(argsNew[i])
		}
	}
	return argsNew
}

func (c *cmd) outputFile() string {
	return c.outFile
}

func (c *cmd) Hash() string {
	return c.hash
}

func (c *cmd) Name() string {
	return c.cmdStr + " " + strings.Join(c.args, " ")
}

func (c *cmd) Run(ctx context.Context, _ []fileOperation) (fileOperation, error) {
	command := c.execCmdCtx(ctx, c.cmdStr, c.args...)
	_, err := command.CombinedOutput()
	if err != nil {
		return 0, err
	}
	return created, nil
}

type noopNode struct {
	outFile string
}

func (n *noopNode) Hash() string {
	return hashShort(n.outFile)
}

func (n *noopNode) Name() string {
	return n.outFile
}

func (n *noopNode) Run(_ context.Context, _ []fileOperation) (fileOperation, error) {
	return noop, nil
}

func (n *noopNode) outputFile() string {
	return n.outFile
}

type copyWav struct {
	srcWavPath string
	dstWavPath string
}

func (c *copyWav) Hash() string {
	return hashShort(c.srcWavPath, c.dstWavPath)
}

func (c *copyWav) Name() string {
	return strings.Join([]string{"copy", c.srcWavPath, c.dstWavPath}, " ")
}

func (c *copyWav) Run(_ context.Context, _ []fileOperation) (fileOperation, error) {
	err := copyFile(c.srcWavPath, c.dstWavPath)
	if err != nil {
		return 0, err
	}
	return created, nil
}

func (c *copyWav) outputFile() string {
	return c.dstWavPath
}

func hashShort(str string, data ...any) string {
	var buf bytes.Buffer
	buf.WriteString(str)
	enc := gob.NewEncoder(&buf)
	for _, d := range data {
		_ = enc.Encode(d)
	}
	h := sha256.Sum256(buf.Bytes())
	return hex.EncodeToString(h[:4])[:7]
}

func copyFile(src, dst string) error {
	fin, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		_ = fin.Close()
	}()

	fout, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		_ = fout.Close()
	}()

	_, err = io.Copy(fout, fin)
	return err
}
