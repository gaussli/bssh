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

// connector pre-dials SSH connections before any command is read.
type connector interface {
	Connect(targets []hosts.Target, onDone func(index int, err error)) []error
}

// runner executes one command on one host.
type runner interface {
	Run(user, host string, port int, command string) (string, error)
}

type sshClient interface {
	connector
	runner
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
	client, err := sshclient.New()
	if err != nil {
		return err
	}
	defer client.Close()
	return runWith(opts, client, os.Stdin, os.Stdout, os.Stderr)
}

func runWith(opts options, client sshClient, in io.Reader, out, errOut io.Writer) error {
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

	ready, err := connectHosts(client, targets, errOut)
	if err != nil {
		return err
	}
	return readCommands(client, ready, in, out, errOut)
}

// connectHosts pre-dials all targets and reports progress before any command is
// read. Hosts that fail to connect are skipped so later commands do not stall
// on an unreachable host.
func connectHosts(client connector, targets []hosts.Target, out io.Writer) ([]hosts.Target, error) {
	for _, target := range targets {
		fmt.Fprintf(out, "正在连接 %s ...\n", target)
	}
	errs := client.Connect(targets, func(i int, err error) {
		if err != nil {
			fmt.Fprintf(out, "连接失败 %s: %v\n", targets[i], err)
			return
		}
		fmt.Fprintf(out, "已连接 %s\n", targets[i])
	})

	ready := make([]hosts.Target, 0, len(targets))
	for i, err := range errs {
		if err == nil {
			ready = append(ready, targets[i])
		}
	}
	if len(ready) == 0 {
		return nil, fmt.Errorf("no hosts connected (%d targets)", len(targets))
	}
	fmt.Fprintf(out, "已连接 %d/%d 台主机，可以输入命令（exit 退出）\n", len(ready), len(targets))
	return ready, nil
}

// readCommands reads one command per line and runs it on every ready host
// serially, reusing cached SSH connections, until "exit" or EOF.
func readCommands(client runner, targets []hosts.Target, in io.Reader, out, errOut io.Writer) error {
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
