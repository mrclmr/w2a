package audio

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/text/unicode/norm"
)

type FileCreator struct {
	execCmdCtx  ExecCmdCtx
	tts         *TTS
	audioFormat Format
	tempDir     string
	outputDir   string

	existingFiles     map[string][]string
	outputFilesToKeep map[string]bool
}

func NewFileCreator(
	execCmd ExecCmdCtx,
	tts *TTS,
	audioFormat Format,
	tempDir string,
	outputDir string,
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

	existingFilesMap := make(map[string][]string)
	existingOutputFilesMap, err := existingFiles(outputDir)
	if err != nil {
		return nil, err
	}
	maps.Copy(existingFilesMap, existingOutputFilesMap)
	existingTempFilesMap, err := existingFiles(tempDir)
	if err != nil {
		return nil, err
	}
	maps.Copy(existingFilesMap, existingTempFilesMap)

	return &FileCreator{
		execCmdCtx:  execCmd,
		tts:         tts,
		audioFormat: audioFormat,
		tempDir:     tempDir,
		outputDir:   outputDir,

		existingFiles:     existingFilesMap,
		outputFilesToKeep: make(map[string]bool),
	}, nil
}

func (c *FileCreator) RemoveOtherFiles() error {
	return removeOtherFiles(c.outputDir, c.outputFilesToKeep)
}

func (c *FileCreator) TextToAudioFile(ctx context.Context, segments []Segment, name string) error {
	file, op, err := c.textToAudioFile(ctx, segments, name)
	if err != nil {
		return err
	}
	path := filepath.Join(c.outputDir, file)

	c.outputFilesToKeep[path] = true

	slog.Info(op.String()+"\t", "path", path)

	return nil
}

func (c *FileCreator) textToAudioFile(ctx context.Context, segments []Segment, name string) (string, fileOperation, error) {
	concatWavFile, _, err := c.toWavConcatenated(ctx, segments)
	if err != nil {
		return "", 0, err
	}

	switch c.audioFormat {
	case Wav:
		return c.copyWav(concatWavFile, name)
	case M4a:
		return c.afconvert(ctx, concatWavFile, name)
	case Mp3:
		return c.ffmpeg(ctx, concatWavFile, name)
	default:
		return "", 0, errors.New("unsupported audio format")
	}
}

func (c *FileCreator) toWavConcatenated(ctx context.Context, segments []Segment) (string, fileOperation, error) {
	if len(segments) == 1 {
		return c.toWav(ctx, segments[0])
	}

	wavFiles := make([]string, len(segments))
	for i, s := range segments {
		wavFile, _, err := c.toWav(ctx, s)
		if err != nil {
			return "", 0, err
		}
		wavFiles[i] = filepath.Join(c.tempDir, wavFile)
	}

	return c.concat(ctx, wavFiles...)
}

func (c *FileCreator) toWav(ctx context.Context, s Segment) (string, fileOperation, error) {
	switch v := s.(type) {
	case *Sound:
		return c.extendLength(ctx, v.value(), v.len())
	case *Text:
		return c.textToWav(ctx, v)
	case *Silence:
		return c.silence(ctx, v.len())
	case *Group:
		values := v.values()
		if len(values) == 0 {
			return c.silence(ctx, v.len())
		}
		con, _, err := c.toWavConcatenated(ctx, values)
		if err != nil {
			return "", 0, err
		}
		return c.extendLength(ctx, con, v.len())
	default:
		return "", 0, errors.New("unknown Segment type")
	}
}

func (c *FileCreator) textToWav(ctx context.Context, s *Text) (string, fileOperation, error) {
	if s.value() == "" {
		return c.silence(ctx, s.len())
	}
	file, _, err := c.ttsCmd(ctx, s.value())
	if err != nil {
		return "", 0, err
	}

	return c.extendLength(ctx, file, s.len())
}

func (c *FileCreator) useExistingFile(path string) (bool, fileOperation, error) {
	for _, paths := range c.existingFiles {
		for _, p := range paths {
			if norm.NFC.String(p) == path {
				return true, skipped, nil
			}
		}
	}

	paths, ok := c.existingFiles[extractHash(path)]
	if ok {
		// If the hash exists the is at least one path in the list.
		err := copyFile(paths[0], path)
		if err != nil {
			return false, 0, err
		}
		return true, copied, nil
	}
	return false, noop, nil
}

func (c *FileCreator) ttsCmd(ctx context.Context, text string) (string, fileOperation, error) {
	switch c.tts.TTSCmd {
	case Say:
		return c.say(ctx, c.tts.Voice, text)
	case EspeakNG:
		return c.espeakNG(ctx, c.tts.Voice, text)
	case Custom:
		return c.custom(ctx, text)
	default:
		return "", 0, fmt.Errorf("unknown tts command %v", c.tts.TTSCmd)
	}
}

func existingFiles(outputDir string) (map[string][]string, error) {
	files, err := listFiles(outputDir)
	if err != nil {
		return nil, err
	}
	existingFilesMap := make(map[string][]string)
	for f := range files {
		hash := extractHash(f)
		existingFilesMap[hash] = append(existingFilesMap[hash], f)
	}
	return existingFilesMap, nil
}

func hashShort(str string, data ...any) string {
	var buf bytes.Buffer
	buf.WriteString(str)
	for _, d := range data {
		enc := gob.NewEncoder(&buf)
		_ = enc.Encode(d)
	}
	h := sha256.Sum256(buf.Bytes())
	return hex.EncodeToString(h[:])[:7]
}

func cmdError(cmd string, args []string, out []byte) error {
	return fmt.Errorf("err: %s %s\n%s",
		cmd,
		strings.Join(args, " "),
		strings.SplitN(string(out), "\n", 1)[0],
	)
}

func mkdirAllIfNotExists(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, os.ModePerm)
	}
	return nil
}

func removeOtherFiles(dir string, excludedFiles map[string]bool) error {
	files, err := listFiles(dir)
	if err != nil {
		return err
	}
	for path := range files {
		// For filenames afconvert uses a different Unicode Normalization Form (NFC, NFD, NFKC, or NFKD).
		// The Go formed string is in the map. Actual filenames have different Unicode Normalization Form.
		normPath := norm.NFC.String(path)
		if !excludedFiles[normPath] {
			err = os.Remove(path)
			if err != nil {
				return err
			}
			slog.Info("removed", "path", normPath)
		}
	}
	return nil
}

func listFiles(dir string) (map[string]bool, error) {
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

func (c *FileCreator) copyWav(srcFile, name string) (string, fileOperation, error) {
	srcWavPath := filepath.Join(c.tempDir, srcFile)

	filename := name + ".wav"
	dstWavPath := filepath.Join(c.outputDir, filename)

	exists, op, err := c.useExistingFile(dstWavPath)
	if err != nil {
		return "", 0, err
	}
	if exists {
		return filename, op, nil
	}

	err = copyFile(srcWavPath, dstWavPath)
	if err != nil {
		return "", 0, err
	}
	return filename, created, nil
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

func extractHash(filename string) string {
	str := strings.TrimSuffix(filename, filepath.Ext(filename))
	return str[len(str)-7:]
}
