package audio

//go:generate go run golang.org/x/tools/cmd/stringer@latest -type TTSCmd
type TTSCmd int

const (
	Say TTSCmd = iota
	EspeakNG
	Custom
)
