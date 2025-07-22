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

	"github.com/mrclmr/w2a/internal/m3u"

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
	execCmdCtx         ExecCmdCtx
	tts                *TTS
	audioFormat        Format
	tempDir            string
	outputDir          string
	createPlaylistFunc CreatePlaylistFunc

	existingFiles     map[string][]string
	outputFilesToKeep map[string]bool
}

func NewFileCreator(
	execCmd ExecCmdCtx,
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
		execCmdCtx:         execCmd,
		tts:                tts,
		audioFormat:        audioFormat,
		tempDir:            tempDir,
		outputDir:          outputDir,
		createPlaylistFunc: createPaylistFunc,

		existingFiles:     existingFilesMap,
		outputFilesToKeep: make(map[string]bool),
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
	// TODO: Create directed acyclic graph for parallel job execution.

	playlistPath := filepath.Join(f.outputDir, "playlist.m3u")
	f.outputFilesToKeep[playlistPath] = true
	playlistFile, err := f.createPlaylistFunc(playlistPath)
	if err != nil {
		return err
	}
	playlist := m3u.NewPlaylist(playlistFile)

	for _, file := range files {
		filename, op, err := f.textToAudioFile(ctx, file.Segments, file.Name)
		if err != nil {
			return err
		}

		path := filepath.Join(f.outputDir, filename)
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		// TODO: Add correct duration.
		playlist.Add(abs, 1*time.Second)

		f.outputFilesToKeep[path] = true

		slog.Info(op.String()+"\t", "path", path)
	}

	err = playlist.Write()
	if err != nil {
		return err
	}

	return nil
}

func (f *FileCreator) textToAudioFile(ctx context.Context, segments []Segment, name string) (string, fileOperation, error) {
	concatWavFile, err := f.toWavConcatenated(ctx, segments)
	if err != nil {
		return "", 0, err
	}

	switch f.audioFormat {
	case Wav:
		return f.copyWav(concatWavFile, name)
	case M4a:
		return f.afconvert(ctx, concatWavFile, name)
	case Mp3:
		return f.ffmpeg(ctx, concatWavFile, name)
	default:
		return "", 0, errors.New("unsupported audio format")
	}
}

func (f *FileCreator) toWavConcatenated(ctx context.Context, segments []Segment) (string, error) {
	if len(segments) == 1 {
		return f.toWav(ctx, segments[0])
	}

	wavFiles := make([]string, len(segments))
	for i, s := range segments {
		wavFile, err := f.toWav(ctx, s)
		if err != nil {
			return "", err
		}
		wavFiles[i] = filepath.Join(f.tempDir, wavFile)
	}

	return f.concat(ctx, wavFiles...)
}

func (f *FileCreator) toWav(ctx context.Context, s Segment) (string, error) {
	switch v := s.(type) {
	case *Sound:
		return f.extendLength(ctx, v.value(), v.len())
	case *Text:
		return f.textToWav(ctx, v)
	case *Silence:
		return f.silence(ctx, v.len())
	case *Group:
		values := v.values()
		if len(values) == 0 {
			return f.silence(ctx, v.len())
		}
		con, err := f.toWavConcatenated(ctx, values)
		if err != nil {
			return "", err
		}
		return f.extendLength(ctx, con, v.len())
	default:
		return "", errors.New("unknown Segment type")
	}
}

func (f *FileCreator) textToWav(ctx context.Context, s *Text) (string, error) {
	if s.value() == "" {
		return f.silence(ctx, s.len())
	}
	file, _, err := f.ttsCmd(ctx, s.value())
	if err != nil {
		return "", err
	}

	return f.extendLength(ctx, file, s.len())
}

func (f *FileCreator) useExistingFile(path string) (bool, fileOperation, error) {
	for _, paths := range f.existingFiles {
		for _, p := range paths {
			if norm.NFC.String(p) == path {
				return true, skipped, nil
			}
		}
	}

	paths, ok := f.existingFiles[extractHash(path)]
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

func (f *FileCreator) ttsCmd(ctx context.Context, text string) (string, fileOperation, error) {
	switch f.tts.TTSCmd {
	case Say:
		return f.say(ctx, f.tts.Voice, text)
	case EspeakNG:
		return f.espeakNG(ctx, f.tts.Voice, text)
	case Custom:
		return f.custom(ctx, text)
	default:
		return "", 0, fmt.Errorf("unknown tts command %v", f.tts.TTSCmd)
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

func (f *FileCreator) copyWav(srcFile, name string) (string, fileOperation, error) {
	srcWavPath := filepath.Join(f.tempDir, srcFile)

	filename := name + ".wav"
	dstWavPath := filepath.Join(f.outputDir, filename)

	exists, op, err := f.useExistingFile(dstWavPath)
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
func (f *FileCreator) extendLength(ctx context.Context, file string, extendedLength time.Duration) (string, error) {
	if extendedLength == 0 {
		return file, nil
	}

	ext := filepath.Ext(file)
	nameNoExt := strings.TrimSuffix(file, ext)

	filePadded := fmt.Sprintf("%s_extended-%s-%s%s", nameNoExt, extendedLength, hashShort(file, extendedLength), ext)
	filePaddedPath := filepath.Join(f.tempDir, filePadded)

	exists, _, err := f.useExistingFile(filePaddedPath)
	if err != nil {
		return "", err
	}
	if exists {
		return filePadded, nil
	}

	length, err := f.length(ctx, file)
	if err != nil {
		return "", err
	}

	addLength := extendedLength - length
	if addLength <= 0 {
		return file, nil
	}

	arguments := []string{
		filepath.Join(f.tempDir, file),
		filePaddedPath,
		"pad", "0", fmt.Sprintf("%f", addLength.Seconds()),
	}

	slog.Debug("execute", "cmd", strings.Join(append([]string{"sox"}, arguments...), " "))

	out, err := f.execCmdCtx(
		ctx,
		"sox",
		arguments...,
	).CombinedOutput()
	if err != nil {
		return "", cmdError("sox", arguments, out)
	}
	return filePadded, nil
}

func (f *FileCreator) length(ctx context.Context, file string) (time.Duration, error) {
	arguments := []string{
		"--i",
		"-D",
		filepath.Join(f.tempDir, file),
	}

	slog.Debug("execute", "cmd", strings.Join(append([]string{"sox"}, arguments...), " "))
	out, err := f.execCmdCtx(
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

func (f *FileCreator) concat(ctx context.Context, filenames ...string) (string, error) {
	file, _, err := f.cmd(
		ctx,
		args{
			cmd:              "sox",
			argsBeforeOutput: filenames,
			outPath:          f.tempDir,
			outFilename:      "concat",
			outFileExt:       "wav",
		})
	return file, err
}

// https://billposer.org/Linguistics/Computation/SoxTutorial.html#silence
func (f *FileCreator) silence(ctx context.Context, duration time.Duration) (string, error) {
	if duration <= 0 {
		return "", errors.New("negative or zero duration for silence")
	}
	file, _, err := f.cmd(
		ctx,
		args{
			cmd:              "sox",
			argsBeforeOutput: []string{"-n", "-r", "22050"},
			outPath:          f.tempDir,
			outFilename:      fmt.Sprintf("silence_%s", duration),
			outFileExt:       "wav",
			argsAfterOutput:  []string{"trim", "0.0", fmt.Sprintf("%.2f", duration.Seconds())},
		})
	return file, err
}

func (f *FileCreator) say(ctx context.Context, voice string, text string) (string, fileOperation, error) {
	// `--data-format=LEF32@22050` is needed for wav.
	// https://stackoverflow.com/questions/9729153/error-on-say-when-output-format-is-wave
	// The comments state that a sample rate higher than 22050 is not recommended.
	dataFormat := "LEF32@22050"

	return f.cmd(
		ctx,
		args{
			cmd: "say",
			argsBeforeOutput: []string{
				"--data-format", dataFormat,
				"--voice", voice,
				"--output-file",
			},
			outPath:         f.tempDir,
			outFilename:     "say",
			outFileExt:      "wav",
			argsAfterOutput: []string{text},
		})
}

func (f *FileCreator) espeakNG(ctx context.Context, voice string, text string) (string, fileOperation, error) {
	return f.cmd(
		ctx,
		args{
			cmd:              "espeak-ng",
			argsBeforeOutput: []string{"-v", voice, "-out"},
			outPath:          f.tempDir,
			outFilename:      "espeak-ng",
			outFileExt:       "wav",
			argsAfterOutput:  []string{text},
		})
}

func (f *FileCreator) custom(ctx context.Context, text string) (string, fileOperation, error) {
	cmd := f.tts.Voice
	if err := checkFmtArg(cmd, "%[1]s"); err != nil {
		return "", 0, err
	}

	if err := checkFmtArg(cmd, "%[2]s"); err != nil {
		return "", 0, err
	}

	name := fmt.Sprintf("%s-%s.wav", strings.SplitN(cmd, " ", 2)[0], hashShort(cmd, text))
	path := filepath.Join(f.tempDir, name)

	exists, op, err := f.useExistingFile(path)
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

func (f *FileCreator) ffmpeg(ctx context.Context, wavFile string, name string) (string, fileOperation, error) {
	return f.cmd(
		ctx,
		args{
			cmd:              "ffmpeg",
			argsBeforeInput:  []string{"-i"},
			inputPath:        f.tempDir,
			inputFilename:    wavFile,
			argsBeforeOutput: []string{"-ab", "256k", "-ar", "44100", "-ac", "2"},
			outPath:          f.outputDir,
			outFilename:      name,
			outFileExt:       "mp3",
		})
}

func (f *FileCreator) afconvert(ctx context.Context, wavFile string, name string) (string, fileOperation, error) {
	return f.cmd(
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
			inputPath:     f.tempDir,
			inputFilename: wavFile,
			outPath:       f.outputDir,
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

func (f *FileCreator) cmd(ctx context.Context, args args) (string, fileOperation, error) {
	outName, err := args.outFile()
	if err != nil {
		return "", 0, err
	}

	outPath := filepath.Join(args.outPath, outName)

	exists, op, err := f.useExistingFile(outPath)
	if err != nil {
		return "", 0, err
	}
	if exists {
		return outName, op, nil
	}

	inPath := args.inFilePath()
	concatArgs := slices.Concat(args.argsBeforeInput, inPath, args.argsBeforeOutput, []string{outPath}, args.argsAfterOutput)
	slog.Debug("execute", "cmd", strings.Join(append([]string{args.cmd}, concatArgs...), " "))

	out, err := f.execCmdCtx(
		ctx,
		args.cmd,
		concatArgs...,
	).CombinedOutput()
	if err != nil {
		return "", 0, cmdError(args.cmd, concatArgs, out)
	}
	return outName, created, nil
}
