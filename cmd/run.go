package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/ory/viper"
	"github.com/spf13/cobra"

	"knative.dev/func/pkg/config"
	"knative.dev/func/pkg/docker"
	fn "knative.dev/func/pkg/functions"
)

func NewRunCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a function locally",
		Long: `
NAME
	{{rootCmdUse}} run - Run a function locally

SYNOPSIS
	{{rootCmdUse}} run [-t|--container] [-v|--verbose] [-p|--path]

DESCRIPTION
	Run the function locally either on the host (default) or from within its
	container.

	Containerized Runs
	  The --container flag indicates that the function's container shuould be
	  run rather than running the source code directly.  This may require that
	  the funciton's container first be rebuilt, and is required if the function
	  has not been built yet:
	    $ func build && func run --container

	Process Scaffolding
	  When running a function, either within a container or on the host, the
	  function code is first wrapped in code which presents it as a process.
	  This "scaffolding" is transient, written for each build or run, and should
	  in most cases be transparent to a function author.  However, to customize,
	  or even completely replace this scafolding code, see the 'scaffold'
	  subcommand.

EXAMPLES

	o Run the function locally on the host with no containerization.
	  $ {{rootCmdUse}} run

	o Run the function locally from within its container.
	  {{rootCmdUse}} build && {{rootCmdUse}} run --container
`,
		SuggestFor: []string{"rnu"},
		PreRunE:    bindEnv("path", "container", "verbose"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRun(cmd, args, newClient)
		},
	}

	// Global Config
	cfg, err := config.NewDefault()
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "error loading config at '%v'. %v\n", config.File(), err)
	}

	// Function Context
	f, _ := fn.NewFunction(effectivePath())
	if f.Initialized() {
		cfg = cfg.Apply(f)
	}

	// Flags
	cmd.Flags().BoolP("container", "t", false, "Run the function in a container. ($FUNC_CONTAINER)")
	addPathFlag(cmd)
	addVerboseFlag(cmd, cfg.Verbose)

	return cmd
}

func runRun(cmd *cobra.Command, args []string, newClient ClientFactory) (err error) {
	cfg := newRunConfig(cmd)
	f, err := fn.NewFunction(cfg.Path)
	if err != nil {
		return
	}
	f = cfg.Configure(f)

	// Client
	// Containerized runs require a docker runner instead of the default
	// host-based runner, and that the functoin already has been built.
	var client *fn.Client
	var done func()
	if cfg.Container {
		if !fn.Built(f.Root) {
			return errors.New("Unable to run the function's container.  Has it been built? For example 'func build && func run'.")
		}
		client, done = newClient(ClientConfig{Verbose: cfg.Verbose},
			fn.WithRunner(docker.NewRunner(cfg.Verbose, os.Stdout, os.Stderr)))
	} else {
		client, done = newClient(ClientConfig{Verbose: cfg.Verbose})
	}
	defer done()

	// Reload due to possible changes during configure
	if f, err = fn.NewFunction(f.Root); err != nil {
		return
	}

	// Run
	job, err := client.Run(cmd.Context(), cfg.Path)
	if err != nil {
		return
	}
	defer job.Stop() // noop for host-runners (subprocesses)

	fmt.Fprintf(cmd.OutOrStderr(), "Running on port %v\n", job.Port)

	select {
	case <-cmd.Context().Done():
		if !errors.Is(cmd.Context().Err(), context.Canceled) {
			err = cmd.Context().Err()
		}
		return
	case err = <-job.Errors:
		return
	}
}

type runConfig struct {
	// Globals (verbose, etc)
	config.Global

	// Path of the function implementation on local disk. Defaults to current
	// working directory of the process.
	Path string

	// Container indicates the funciton should be run in its container
	Container bool
}

func newRunConfig(cmd *cobra.Command) (cfg runConfig) {
	return runConfig{
		Global: config.Global{
			Verbose: viper.GetBool("verbose"),
		},
		Path:      viper.GetString("path"),
		Container: viper.GetBool("container"),
	}
}

func (c runConfig) Configure(f fn.Function) fn.Function {
	return c.Global.Configure(f)
	// path and container are not part of function state, so are not mentioned
	// in Configure
}

// TODO: .Prompt()
// Add --configure flag, help text and invoke .Prompt() in runRun
// Implementation should confirm verbosity, path and container
