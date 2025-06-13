package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
)

// MsgHandler just prints the values and looks like fmt.Println.
type MsgHandler struct {
	writer io.Writer
	level  slog.Level
}

func NewMsgHandler(writer io.Writer, level slog.Level) *MsgHandler {
	return &MsgHandler{writer: writer, level: level}
}

func (h *MsgHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level == h.level
}

func (h *MsgHandler) Handle(_ context.Context, record slog.Record) error {
	_, _ = fmt.Fprint(h.writer, record.Message)

	record.Attrs(func(a slog.Attr) bool {
		_, _ = fmt.Fprint(h.writer, " ", a.Value)
		return true
	})

	_, _ = fmt.Fprintln(h.writer)
	return nil
}

func (h *MsgHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return h
}

func (h *MsgHandler) WithGroup(_ string) slog.Handler {
	return h
}
