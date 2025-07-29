package audio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"

	"github.com/mrclmr/w2a/internal/dag"
	"github.com/mrclmr/w2a/internal/m3u"
)

type ExecCmdCtx = func(ctx context.Context, name string, args ...string) Cmd

type Cmd interface {
	CombinedOutput() ([]byte, error)
}

// ToExecCmdCtx is needed because Go does not automatically convert return types
// to interfaces in function assignments, even if the return type does implement the interface.
// See https://stackoverflow.com/questions/57735694/duck-typing-go-functions
func ToExecCmdCtx[c Cmd](fn func(context.Context, string, ...string) c) ExecCmdCtx {
	return func(ctx context.Context, name string, arg ...string) Cmd {
		return fn(ctx, name, arg...)
	}
}

type CreatePlaylistFunc = func(string) (io.WriteCloser, error)

func ToCreatePlaylistFunc[wc io.WriteCloser](fn func(string) (wc, error)) CreatePlaylistFunc {
	return func(name string) (io.WriteCloser, error) {
		return fn(name)
	}
}

type TTS struct {
	TTSCmd TTSCmd
	Voice  string
}

type FileCreator struct {
	outputDir          string
	createPlaylistFunc CreatePlaylistFunc

	outputFilesToKeep map[string]bool
	existingFilePaths map[string]map[string]bool

	dag        *dag.Dag[fileOperation]
	cmdBuilder *cmdBuilder
}

func NewFileCreator(
	execCmdCtx ExecCmdCtx,
	tts *TTS,
	audioFormat Format,
	tempDir string,
	outputDir string,
	createPaylistFunc CreatePlaylistFunc,
) (*FileCreator, error) {
	if err := mkdirAllIfNotExists(outputDir); err != nil {
		return nil, err
	}
	if err := mkdirAllIfNotExists(tempDir); err != nil {
		return nil, err
	}

	if err := initSounds(tempDir); err != nil {
		return nil, err
	}

	existingFilePaths, err := allFilePaths(tempDir, outputDir)
	if err != nil {
		return nil, err
	}

	return &FileCreator{
		outputDir:          outputDir,
		createPlaylistFunc: createPaylistFunc,

		outputFilesToKeep: make(map[string]bool),
		existingFilePaths: existingFilePaths,

		dag:        dag.New[fileOperation](),
		cmdBuilder: newCmdBuilder(existingFilePaths, execCmdCtx, tempDir, outputDir, tts, audioFormat),
	}, nil
}

func (f *FileCreator) RemoveOtherFiles() error {
	return removeOtherFiles(f.outputDir, f.outputFilesToKeep)
}

type File struct {
	Name     string
	Segments []Segment
}

func (f *FileCreator) BatchCreate(ctx context.Context, files []File) error {
	playlistPath := filepath.Join(f.outputDir, "playlist.m3u")
	f.outputFilesToKeep[playlistPath] = true
	playlistFile, err := f.createPlaylistFunc(playlistPath)
	if err != nil {
		return err
	}
	playlist := m3u.NewPlaylist(playlistFile)

	nodesToRun := make([]dag.Node[fileOperation], 0)
	paths := make([]string, 0)

	for _, file := range files {
		op, convertCmd, err := f.textToAudioFile(file.Segments, file.Name)
		if err != nil {
			return err
		}

		path := filepath.Join(f.outputDir, convertCmd.outputFile())
		f.outputFilesToKeep[path] = true

		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		// TODO: Add correct duration.
		playlist.Add(abs, 1*time.Second)

		if op >= skipped {
			slog.Info(op.String()+"\t", "path", path)
		} else {
			nodesToRun = append(nodesToRun, convertCmd)
			paths = append(paths, path)
		}
	}

	idx := 0
	for op, err := range f.dag.RunNodes(ctx, nodesToRun) {
		if err != nil {
			return err
		}

		slog.Info(op.String()+"\t", "path", paths[idx])
		idx++
	}

	err = playlist.Write()
	if err != nil {
		return err
	}

	return nil
}

func (f *FileCreator) textToAudioFile(segments []Segment, name string) (fileOperation, node, error) {
	concatCmd, err := f.toWavConcatenated(segments)
	if err != nil {
		return 0, nil, err
	}
	op, convertCmd, err := f.cmdBuilder.convert(concatCmd.outputFile(), name)
	if err != nil {
		return 0, nil, err
	}
	if op >= skipped {
		return op, convertCmd, nil
	}
	err = f.dag.AddEdge(convertCmd, concatCmd)
	if err != nil {
		return 0, nil, err
	}
	return op, convertCmd, err
}

func (f *FileCreator) toWavConcatenated(segments []Segment) (*fileCache, error) {
	if len(segments) == 1 {
		return f.toWav(segments[0])
	}
	wavFiles := make([]string, len(segments))
	cmdWavs := make([]*fileCache, len(segments))
	for i, s := range segments {
		cmdWav, err := f.toWav(s)
		if err != nil {
			return nil, err
		}
		cmdWavs[i] = cmdWav
		wavFiles[i] = cmdWav.outputFile()
	}

	concatCmd := f.cmdBuilder.soxConcat(wavFiles)
	for _, cmdWav := range cmdWavs {
		err := f.dag.AddEdge(concatCmd, cmdWav)
		if err != nil {
			return nil, err
		}
	}
	return concatCmd, nil
}

func (f *FileCreator) toWav(s Segment) (*fileCache, error) {
	switch v := s.(type) {
	case *Sound:
		return f.cmdBuilder.soxExtendLength(v.value(), v.len()), nil
	case *Text:
		return f.textToWav(v)
	case *Silence:
		return f.cmdBuilder.soxSilence(v.len()), nil
	case *Group:
		values := v.values()
		if len(values) == 0 {
			return f.cmdBuilder.soxSilence(v.len()), nil
		}
		concatCmd, err := f.toWavConcatenated(values)
		if err != nil {
			return nil, err
		}

		extLenCmd := f.cmdBuilder.soxExtendLength(concatCmd.outputFile(), v.len())
		if v.len() == 0 {
			return extLenCmd, nil
		}
		err = f.dag.AddEdge(extLenCmd, concatCmd)
		if err != nil {
			return nil, err
		}
		return extLenCmd, nil
	default:
		return nil, errors.New("unknown Segment type")
	}
}

func (f *FileCreator) textToWav(t *Text) (*fileCache, error) {
	if t.value() == "" {
		if t.len() > 0 {
			silCmd := f.cmdBuilder.soxSilence(t.len())
			return silCmd, nil
		}
		return nil, fmt.Errorf("text is empty and length is zero")
	}

	ttsCmd := f.cmdBuilder.ttsCmd(t.value())
	if t.len() > 0 {
		extLenCmd := f.cmdBuilder.soxExtendLength(ttsCmd.outputFile(), t.len())
		err := f.dag.AddEdge(extLenCmd, ttsCmd)
		if err != nil {
			return nil, err
		}
		return extLenCmd, nil
	}
	return ttsCmd, nil
}

func mkdirAllIfNotExists(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, os.ModePerm)
	}
	return nil
}

func removeOtherFiles(dir string, excludedFiles map[string]bool) error {
	filePaths, err := listFilePaths(dir)
	if err != nil {
		return err
	}
	for path := range filePaths {
		// For filenames afconvert uses a different Unicode Normalization Form (NFC, NFD, NFKC, or NFKD).
		// The Go formed string is in the map. Actual filenames have different Unicode Normalization Form.
		normPath := norm.NFC.String(path)
		if !excludedFiles[normPath] {
			err = os.Remove(path)
			if err != nil {
				return err
			}
			slog.Info("removed\t", "path", normPath)
		}
	}
	return nil
}

func allFilePaths(tempDir string, outputDir string) (map[string]map[string]bool, error) {
	all := make(map[string]map[string]bool)
	outputFilePaths, err := filePaths(outputDir)
	if err != nil {
		return nil, err
	}
	maps.Copy(all, outputFilePaths)
	tempFilePaths, err := filePaths(tempDir)
	if err != nil {
		return nil, err
	}
	maps.Copy(all, tempFilePaths)
	return all, nil
}

func filePaths(dir string) (map[string]map[string]bool, error) {
	files, err := listFilePaths(dir)
	if err != nil {
		return nil, err
	}
	existingFilesMap := make(map[string]map[string]bool)
	for f := range files {
		hash := extractHash(f)
		m := existingFilesMap[hash]
		if m == nil {
			existingFilesMap[hash] = make(map[string]bool)
		}
		existingFilesMap[hash][f] = true
	}
	return existingFilesMap, nil
}

func listFilePaths(dir string) (map[string]bool, error) {
	files := make(map[string]bool)

	err := filepath.WalkDir(dir, func(path string, file fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		name := file.Name()

		// Ignore files beginning with '.'
		if name[:1] == "." {
			return nil
		}

		if filepath.Base(path) == filepath.Base(dir) {
			return nil
		}

		if filepath.Ext(name) == ".m3u" {
			return nil
		}

		if file.IsDir() {
			return filepath.SkipDir
		}

		files[filepath.Join(dir, name)] = true

		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func extractHash(filename string) string {
	str := strings.TrimSuffix(filename, filepath.Ext(filename))
	return str[len(str)-7:]
}
