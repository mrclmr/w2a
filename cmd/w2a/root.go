package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/mrclmr/w2a/internal/config"
	"github.com/mrclmr/w2a/internal/log"

	"github.com/spf13/cobra"
)

func Execute(version string) {
	rootCmd, err := newRootCmd(version)
	checkErr(err)

	err = rootCmd.Execute()
	checkErr(err)
}

func newRootCmd(
	version string,
) (*cobra.Command, error) {
	rootCmd := &cobra.Command{
		Version:           version,
		Use:               "w2a",
		Short:             "Convert workout yaml to audio files",
		Long:              "Convert workout yaml to audio files.",
		SilenceUsage:      true,
		SilenceErrors:     true,
		DisableAutoGenTag: true,
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true,
		},
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: autocomplete,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("configuration not found: %w", err)
			}
			f, err := os.OpenFile(path, os.O_RDONLY, 0o600)
			if err != nil {
				return err
			}
			defer func() {
				_ = f.Close()
			}()
			cfg, err := config.Parse(f)
			if err != nil {
				return err
			}
			switch cfg.LogLevel {
			case slog.LevelInfo:
				slog.SetDefault(slog.New(log.NewMsgHandler(os.Stdout, cfg.LogLevel)))
			default:
				slog.SetLogLoggerLevel(cfg.LogLevel)
			}
			return run(cmd.Context(), cfg)
		},
	}

	// https://github.com/spf13/cobra/blob/6dec1ae26659a130bdb4c985768d1853b0e1bc06/command.go#L2064
	rootCmd.SetVersionTemplate(`{{with .DisplayName}}{{printf "%s " .}}{{end}}{{printf "version %s" .Version}}

Sound Credits
* Race Start (start-2929965.wav) by JustInvoke -- https://freesound.org/s/446142/ -- License: Attribution 4.0
* success.wav (success-a1a69bc.wav) by maxmakessounds -- https://freesound.org/s/353546/ -- License: Attribution 4.0
`)

	exampleCmd := &cobra.Command{
		Use:               "example",
		Short:             "Print example workout yaml",
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			example, err := config.Example()
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(os.Stdout, example)
			return err
		},
	}

	rootCmd.AddCommand(exampleCmd)

	rootCmd.AddCommand(newManCmd(rootCmd))

	return rootCmd, nil
}

func checkErr(err error) {
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func autocomplete(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(toComplete) > 0 && strings.HasPrefix("example", toComplete) {
		return []string{"example"}, cobra.ShellCompDirectiveNoFileComp
	}
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	return []string{"yml", "yaml"}, cobra.ShellCompDirectiveFilterFileExt
}
