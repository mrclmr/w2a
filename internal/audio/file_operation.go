package audio

//go:generate go run golang.org/x/tools/cmd/stringer@latest -type fileOperation -output file_operation_string.go
type fileOperation int

const (
	opErr fileOperation = iota
	noop
	created
	skipped
	copied
)
