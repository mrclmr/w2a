package m3u

import (
	"bytes"
	"testing"
	"time"
)

func TestPlaylist_Write(t *testing.T) {
	tests := []struct {
		name  string
		items []item
		want  string
	}{
		{
			"one item",
			[]item{
				{absFilePath: "/test/test1.mp3", dur: time.Second * 10},
			},
			`#EXTM3U
#EXTINF:10,test1.mp3
file:///test/test1.mp3
`,
		},
		{
			"escape non ASCII characters",
			[]item{
				{absFilePath: "/über/test/testütestätestötest.mp3", dur: time.Second * 10},
			},
			`#EXTM3U
#EXTINF:10,testütestätestötest.mp3
file:///u%CC%88ber/test/testu%CC%88testa%CC%88testo%CC%88test.mp3
`,
		},
		{
			"multiple items",
			[]item{
				{absFilePath: "/test/test1.mp3", dur: time.Second * 10},
				{absFilePath: "/test/test2.mp3", dur: time.Second * 8},
				{absFilePath: "/test/test3.mp3", dur: time.Second * 123},
			},
			`#EXTM3U
#EXTINF:10,test1.mp3
file:///test/test1.mp3
#EXTINF:8,test2.mp3
file:///test/test2.mp3
#EXTINF:123,test3.mp3
file:///test/test3.mp3
`,
		},
		{
			"rount time down",
			[]item{
				{absFilePath: "/test/test1.mp3", dur: time.Millisecond * 9999},
			},
			`#EXTM3U
#EXTINF:9,test1.mp3
file:///test/test1.mp3
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buffer := &bytes.Buffer{}
			p := NewPlaylist(buffer)
			for _, it := range tt.items {
				p.Add(it.absFilePath, it.dur)
			}
			err := p.Write()
			if err != nil {
				t.Fatalf("Write() error = %v", err)
			}
			if buffer.String() != tt.want {
				t.Fatalf("Write() = %v, want %v", buffer.String(), tt.want)
			}
		})
	}
}
