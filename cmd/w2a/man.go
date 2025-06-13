package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

func newManCmd(rootCmd *cobra.Command) *cobra.Command {
	// https://unix.stackexchange.com/questions/3586/what-do-the-numbers-in-a-man-page-mean
	return &cobra.Command{
		Use:                   "man",
		Short:                 "Generate man pages",
		SilenceUsage:          true,
		Hidden:                true,
		DisableFlagsInUseLine: true,
		Example:               "w2a man . && cat w2a.1",
		Args:                  cobra.ExactArgs(1),
		ValidArgsFunction:     cobra.NoFileCompletions,
		RunE: func(_ *cobra.Command, args []string) error {
			path := args[0]
			err := doc.GenManTree(rootCmd, nil, path)
			if err != nil {
				return err
			}
			return nil
		},
	}
}
