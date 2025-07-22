package audio

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func newDummyCmdExec(buf *bytes.Buffer) func(context.Context, string, ...string) dummyCmd {
	return func(_ context.Context, cmd string, args ...string) dummyCmd {
		buf.WriteString(strings.Join(append([]string{cmd}, args...), " ") + "\n")
		return dummyCmd{Buffer: buf}
	}
}

type dummyCmd struct {
	*bytes.Buffer
}

func (c dummyCmd) CombinedOutput() ([]byte, error) {
	return []byte{}, nil
}

const (
	outputDir = "file_creator_audio-dir"
	tempDir   = "file_creator_temp-dir"
)

type dummyPlaylist struct {
	*bytes.Buffer
}

func (d *dummyPlaylist) Close() error {
	return nil
}

func TestFileCreator_BatchCreate(t *testing.T) {
	tests := []struct {
		name         string
		files        []File
		wantPlaylist string
		wantLog      string
		wantErr      bool
	}{
		{
			files: []File{
				{
					Name:     "my-file",
					Segments: []Segment{&Silence{Length: 1 * time.Second}},
				},
			},
			wantPlaylist: `#EXTM3U
#EXTINF:1,my-file-03bdd2e.mp3
file:///Users/meyermarcel/projects/w2a/internal/audio/file_creator_audio-dir/my-file-03bdd2e.mp3
`,
			wantLog: `sox -n -r 22050 file_creator_temp-dir/silence_1s-92ed2b7.wav trim 0.0 1.00
ffmpeg -i file_creator_temp-dir/silence_1s-92ed2b7.wav -ab 256k -ar 44100 -ac 2 file_creator_audio-dir/my-file-03bdd2e.mp3
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			bufPlaylist := &dummyPlaylist{&bytes.Buffer{}}
			creator, err := NewFileCreator(
				ToExecCmdCtx(newDummyCmdExec(buf)),
				&TTS{
					TTSCmd: EspeakNG,
					Voice:  "en-GB",
				},
				Mp3,
				tempDir,
				outputDir,
				func(name string) (io.WriteCloser, error) {
					return bufPlaylist, nil
				},
			)
			if err != nil {
				t.Fatalf("failed to create audio creator: %v", err)
			}
			err = creator.BatchCreate(t.Context(), tt.files)
			if err != nil {
				t.Fatalf("failed to create silence: %v", err)
			}
			gotLog := buf.String()
			if gotLog != tt.wantLog {
				t.Fatalf("\ngotLog\n%s\nwantLog\n%s\n", gotLog, tt.wantLog)
			}
			gotPlaylist := bufPlaylist.String()
			if gotPlaylist != tt.wantPlaylist {
				t.Fatalf("\ngotPlaylist\n%s\nwantPlaylist\n%s\n", gotPlaylist, tt.wantPlaylist)
			}
		})
	}
}
