package audio

import (
	"embed"
	"os"
	"path/filepath"
)

// Allow standalone executable (go build) by embedding and calling initSounds().
//
// Use one channel:        sox old.wav -c 1 new.wav
// Use 22050 sample rate:  sox old.wav -r 22050 new.wav
//
// A Sha256 hash is needed in the file name. Best is to use Sha256 of the file content.
//
//	my-sound-name-<SHA256SHORT>.wav
//
//go:embed sounds
var sounds embed.FS

func initSounds(dstDir string) error {
	entries, err := sounds.ReadDir("sounds")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		data, err := sounds.ReadFile(filepath.Join("sounds", entry.Name()))
		if err != nil {
			return err
		}

		err = os.WriteFile(filepath.Join(dstDir, entry.Name()), data, 0o600)
		if err != nil {
			return err
		}
	}
	return nil
}
