package m3u

import (
	"fmt"
	"io"
	"math"
	"path/filepath"
	"time"

	"golang.org/x/text/unicode/norm"
)

type item struct {
	absFilePath string
	dur         time.Duration
}

type Playlist struct {
	w     io.Writer
	items []item
}

func NewPlaylist(w io.Writer) *Playlist {
	return &Playlist{w: w}
}

func (p *Playlist) Add(absFilePath string, dur time.Duration) {
	p.items = append(p.items, item{absFilePath, dur})
}

func (p *Playlist) Write() error {
	_, err := io.WriteString(p.w, "#EXTM3U\n")
	if err != nil {
		return err
	}
	for _, it := range p.items {
		roundedDown := int(math.Floor(it.dur.Seconds()*100) / 100)
		_, err = io.WriteString(p.w, fmt.Sprintf("#EXTINF:%d,%s\n", roundedDown, filepath.Base(it.absFilePath)))
		if err != nil {
			return err
		}
		_, err = io.WriteString(p.w, fmt.Sprintf("file://%s\n", escape(it.absFilePath)))
		if err != nil {
			return err
		}
	}
	return nil
}

func escape(input string) string {
	s := norm.NFD.String(input)
	var escaped string
	for _, b := range []byte(s) {
		if b > 127 || b == '%' {
			escaped += fmt.Sprintf("%%%02X", b)
		} else {
			escaped += string(b)
		}
	}
	return escaped
}
