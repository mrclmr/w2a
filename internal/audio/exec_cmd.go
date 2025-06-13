package audio

import (
	"context"
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
