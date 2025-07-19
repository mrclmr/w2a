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
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"
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

type TTS struct {
	TTSCmd TTSCmd
	Voice  string
}

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
			slog.Info("removed\t", "path", normPath)
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

// extendLength does not use cmd because files would be read everytime
// to get the hash. If nothing is to do reading every file consumes time (~1s vs. instant).
func (c *FileCreator) extendLength(ctx context.Context, file string, extendedLength time.Duration) (string, fileOperation, error) {
	if extendedLength == 0 {
		return file, noop, nil
	}

	ext := filepath.Ext(file)
	nameNoExt := strings.TrimSuffix(file, ext)

	filePadded := fmt.Sprintf("%s_extended-%s-%s%s", nameNoExt, extendedLength, hashShort(file, extendedLength), ext)
	filePaddedPath := filepath.Join(c.tempDir, filePadded)

	exists, op, err := c.useExistingFile(filePaddedPath)
	if err != nil {
		return "", 0, err
	}
	if exists {
		return filePadded, op, nil
	}

	length, err := c.length(ctx, file)
	if err != nil {
		return "", 0, err
	}

	addLength := extendedLength - length
	if addLength <= 0 {
		return file, 0, nil
	}

	arguments := []string{
		filepath.Join(c.tempDir, file),
		filePaddedPath,
		"pad", "0", fmt.Sprintf("%f", addLength.Seconds()),
	}

	slog.Debug("execute", "cmd", strings.Join(append([]string{"sox"}, arguments...), " "))

	out, err := c.execCmdCtx(
		ctx,
		"sox",
		arguments...,
	).CombinedOutput()
	if err != nil {
		return "", 0, cmdError("sox", arguments, out)
	}
	return filePadded, created, nil
}

func (c *FileCreator) length(ctx context.Context, file string) (time.Duration, error) {
	arguments := []string{
		"--i",
		"-D",
		filepath.Join(c.tempDir, file),
	}

	slog.Debug("execute", "cmd", strings.Join(append([]string{"sox"}, arguments...), " "))
	out, err := c.execCmdCtx(
		ctx,
		"sox",
		arguments...,
	).CombinedOutput()
	if err != nil {
		return 0, cmdError("sox", arguments, out)
	}

	var float float64
	for l := range strings.Lines(string(out)) {
		float, err = strconv.ParseFloat(strings.TrimSuffix(l, "\n"), 64)
		if err == nil {
			break
		}
	}
	if err != nil {
		return 0, fmt.Errorf("%w: no parsable float in\n%s", err, string(out))
	}
	return time.Duration(float * float64(time.Second)), nil
}

func (c *FileCreator) concat(ctx context.Context, filenames ...string) (string, fileOperation, error) {
	return c.cmd(
		ctx,
		args{
			cmd:              "sox",
			argsBeforeOutput: filenames,
			outPath:          c.tempDir,
			outFilename:      "concat",
			outFileExt:       "wav",
		})
}

// https://billposer.org/Linguistics/Computation/SoxTutorial.html#silence
func (c *FileCreator) silence(ctx context.Context, duration time.Duration) (string, fileOperation, error) {
	if duration <= 0 {
		return "", 0, errors.New("negative or zero duration for silence")
	}
	return c.cmd(
		ctx,
		args{
			cmd:              "sox",
			argsBeforeOutput: []string{"-n", "-r", "22050"},
			outPath:          c.tempDir,
			outFilename:      fmt.Sprintf("silence_%s", duration),
			outFileExt:       "wav",
			argsAfterOutput:  []string{"trim", "0.0", fmt.Sprintf("%.2f", duration.Seconds())},
		})
}

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

func (c *FileCreator) afconvert(ctx context.Context, wavFile string, name string) (string, fileOperation, error) {
	return c.cmd(
		ctx,
		args{
			cmd: "afconvert",
			argsBeforeInput: []string{
				// For macOS Music App (iTunes) compatibility use m4af
				// despite it is described as lossless.
				// mp4f is incompatible with macOS Music App.
				"--file", "m4af",
				"--data", "aac",
				"--quality", "127",
				"--strategy", "2",
			},
			inputPath:     c.tempDir,
			inputFilename: wavFile,
			outPath:       c.outputDir,
			outFilename:   name,
			outFileExt:    "m4a",
		})
}

type args struct {
	cmd              string
	argsBeforeInput  []string
	inputPath        string
	inputFilename    string
	argsBeforeOutput []string
	outPath          string
	outFilename      string
	outFileExt       string
	argsAfterOutput  []string
}

func (a *args) inFilePath() []string {
	if a.inputPath != "" && a.inputFilename != "" {
		return []string{filepath.Join(a.inputPath, a.inputFilename)}
	}
	return nil
}

func (a *args) outFile() (string, error) {
	if a.inputFilename == "" && a.outFilename == "" {
		return "", errors.New("no input and no output file name")
	}
	if a.inputFilename != "" && a.outFilename == "" {
		return strings.TrimSuffix(a.inputFilename, filepath.Ext(a.inputFilename)) + "." + a.outFileExt, nil
	}

	return a.outFilename + "-" + hashShort(
		a.cmd,
		a.argsBeforeInput,
		a.inputPath,
		a.inputFilename,
		a.argsBeforeOutput,
		a.outFileExt,
		a.argsAfterOutput,
	) + "." + a.outFileExt, nil
}

func (c *FileCreator) cmd(ctx context.Context, args args) (string, fileOperation, error) {
	outName, err := args.outFile()
	if err != nil {
		return "", 0, err
	}

	outPath := filepath.Join(args.outPath, outName)

	exists, op, err := c.useExistingFile(outPath)
	if err != nil {
		return "", 0, err
	}
	if exists {
		return outName, op, nil
	}

	inPath := args.inFilePath()
	concatArgs := slices.Concat(args.argsBeforeInput, inPath, args.argsBeforeOutput, []string{outPath}, args.argsAfterOutput)
	slog.Debug("execute", "cmd", strings.Join(append([]string{args.cmd}, concatArgs...), " "))

	out, err := c.execCmdCtx(
		ctx,
		args.cmd,
		concatArgs...,
	).CombinedOutput()
	if err != nil {
		return "", 0, cmdError(args.cmd, concatArgs, out)
	}
	return outName, created, nil
}
