package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gaussli/bssh/internal/hosts"
	"github.com/gaussli/bssh/internal/sshclient"
)

const exitCommand = "exit"

type options struct {
	file string
}

func newRootCmd() *cobra.Command {
	var opts options
	cmd := &cobra.Command{
		Use:          "bssh",
		Short:        "Batch hosts command execute tool",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return run(opts)
		},
	}
	cmd.Flags().StringVarP(&opts.file, "file", "f", "", "hosts yaml file (required)")
	return cmd
}

// Execute runs the root command and exits with a non-zero status on failure.
func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func run(opts options) error {
	if opts.file == "" {
		return fmt.Errorf("-f/--file is required")
	}

	targets, err := hosts.ParseFile(opts.file)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return fmt.Errorf("no hosts found in %s", opts.file)
	}

	client, err := sshclient.New()
	if err != nil {
		return err
	}
	defer client.Close()
	return readCommands(client, targets, os.Stdin, os.Stdout, os.Stderr)
}

// readCommands reads one command per line and runs it on every host serially,
// reusing cached SSH connections, until "exit" or EOF.
func readCommands(client *sshclient.Client, targets []hosts.Target, in io.Reader, out, errOut io.Writer) error {
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		command := strings.TrimSpace(scanner.Text())
		if command == exitCommand {
			return nil
		}
		if command == "" {
			continue
		}
		for _, target := range targets {
			result, err := client.Run(target.User, target.Host, target.Port, command)
			if err != nil {
				fmt.Fprintf(errOut, "%s\tERROR: %v\n", target, err)
				continue
			}
			for _, line := range strings.Split(result, "\n") {
				fmt.Fprintf(out, "%s\t%s\n", target, line)
			}
		}
	}
	return scanner.Err()
}
