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

func (f *fileCacheBuilder) cmd(
	cmd *cmd,
) *fileCache {
	return &fileCache{
		node:          cmd,
		existingFiles: f.existingFiles,
	}
}

func (f *fileCacheBuilder) convert(
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

func (f *fileCacheBuilder) noop(
	outFile string,
) *fileCache {
	return &fileCache{
		node: &noopNode{
			outFile: outFile,
		},
		existingFiles: f.existingFiles,
	}
}

func (f *fileCacheBuilder) copy(
	srcPath string,
	dstPath string,
) (fileOperation, *copyNode, error) {
	cpNode := &copyNode{
		srcPath: srcPath,
		dstPath: dstPath,
	}
	op, err := useExistingFile(f.existingFiles, cpNode)
	if err != nil {
		return 0, nil, err
	}
	return op, cpNode, nil
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
	if op >= exists {
		return op, nil
	}
	return f.node.Run(ctx, nil)
}

func useExistingFile(existingFiles map[string]map[string]bool, n node) (fileOperation, error) {
	for _, paths := range existingFiles {
		for p := range paths {
			if norm.NFC.String(filepath.Base(p)) == n.outputFile() {
				return exists, nil
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
