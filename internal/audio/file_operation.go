package audio

//go:generate go run golang.org/x/tools/cmd/stringer@latest -type fileOperation -output file_operation_string.go
type fileOperation int

const (
	opErr fileOperation = iota
	noop

	// File was created or needs to be created.
	created

	// File already exists and was skipped or copied.
	skipped
	copied
)
