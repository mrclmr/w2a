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
	"slices"
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
	argsReplaced, outFile, hash := replaceHash(cmdStr, args)
	return &cmd{
		execCmdCtx: execCmdCtx,
		cmdStr:     cmdStr,
		args:       argsReplaced,
		outFile:    outFile,
		hash:       hash,
	}
}

// replaceHash replaces <hash> with actual hash.
func replaceHash(cmdStr string, argsOrig []string) (args []string, outFile string, hash string) {
	h := hashShort(cmdStr, argsBasePath(argsOrig))

	idx := slices.IndexFunc(argsOrig, func(arg string) bool { return strings.Contains(arg, "<hash>") })

	filePathReplaced := strings.ReplaceAll(argsOrig[idx], "<hash>", h)
	argsOrig[idx] = filePathReplaced

	return argsOrig, filepath.Base(filePathReplaced), h
}

// argsBasePath removes paths from file so hashing
// is consistent independently of directory names.
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
	out, err := command.CombinedOutput()
	if err != nil {
		return 0, cmdError(c.cmdStr, c.args, out)
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
