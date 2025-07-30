package audio

import (
	"context"
	"path/filepath"

	"github.com/mrclmr/w2a/internal/dag"
	"golang.org/x/text/unicode/norm"
)

type node interface {
	dag.Node[fileOperation]
	outputFile() string
}

type fileCacheBuilder struct {
	existingFiles map[string]map[string]bool
}

func (f *fileCacheBuilder) buildCmd(
	cmd *cmd,
) *fileCache {
	return &fileCache{
		node:          cmd,
		existingFiles: f.existingFiles,
	}
}

func (f *fileCacheBuilder) buildConvertCmd(
	execCmdCtx ExecCmdCtx,
	cmdStr string,
	args []string,
) (fileOperation, node, error) {
	n := newCmd(execCmdCtx, cmdStr, args)
	op, err := useExistingFile(f.existingFiles, n)
	if err != nil {
		return 0, nil, err
	}
	return op, n, nil
}

func (f *fileCacheBuilder) buildNoop(
	outFile string,
) *fileCache {
	return &fileCache{
		node: &noopNode{
			outFile: outFile,
		},
		existingFiles: f.existingFiles,
	}
}

func (f *fileCacheBuilder) buildCopyWav(
	srcWavPath string,
	name string,
) (fileOperation, *copyWav, error) {
	cpWav := copyWav{
		srcWavPath: srcWavPath,
		dstWavPath: name + ".wav",
	}
	op, err := useExistingFile(f.existingFiles, &cpWav)
	if err != nil {
		return 0, nil, err
	}
	return op, &cpWav, nil
}

func newFileCacheBuilder(
	existingFiles map[string]map[string]bool,
) *fileCacheBuilder {
	return &fileCacheBuilder{
		existingFiles: existingFiles,
	}
}

type fileCache struct {
	node          node
	existingFiles map[string]map[string]bool
}

func (f *fileCache) outputFile() string {
	return f.node.outputFile()
}

func (f *fileCache) Hash() string {
	return f.node.Hash()
}

func (f *fileCache) Name() string {
	return f.node.Name()
}

func (f *fileCache) Run(ctx context.Context, _ []fileOperation) (fileOperation, error) {
	op, err := useExistingFile(f.existingFiles, f.node)
	if err != nil {
		return 0, err
	}
	if op >= skipped {
		return op, nil
	}
	return f.node.Run(ctx, nil)
}

func useExistingFile(existingFiles map[string]map[string]bool, n node) (fileOperation, error) {
	for _, paths := range existingFiles {
		for p := range paths {
			if norm.NFC.String(filepath.Base(p)) == n.outputFile() {
				return skipped, nil
			}
		}
	}

	hash := extractHash(n.outputFile())
	paths, ok := existingFiles[hash]
	if ok {
		var path string
		for p := range paths {
			path = p
			break
		}
		copiedPath := filepath.Join(filepath.Dir(path), n.outputFile())
		// TODO: rename file?
		err := copyFile(path, copiedPath)
		if err != nil {
			return 0, err
		}
		return copied, nil
	}
	// created means in this context "needs to be created"
	return created, nil
}
