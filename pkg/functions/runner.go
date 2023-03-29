package functions

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	defaultRunHost        = "127.0.0.1"
	defaultRunPort        = "8080"
	defaultRunDialTimeout = 2 * time.Second
	defaultRunStopTimeout = 10 * time.Second
)

type defaultRunner struct {
	client *Client
	out    io.Writer
	err    io.Writer
}

func newDefaultRunner(client *Client, out, err io.Writer) *defaultRunner {
	return &defaultRunner{
		client: client,
		out:    out,
		err:    err,
	}
}

func (r *defaultRunner) Run(ctx context.Context, f Function) (job *Job, err error) {
	var (
		port    = choosePort(defaultRunHost, defaultRunPort, defaultRunDialTimeout)
		doneCh  = make(chan error, 10)
		stopFn  = func() error { return nil } // Only needed for continerized runs
		verbose = r.client.verbose
	)

	// NewJob creates .func/runs/PORT,
	job, err = NewJob(f, port, doneCh, stopFn, verbose)
	if err != nil {
		return
	}

	// Write scaffolding into the build directory
	if err = r.client.Scaffold(ctx, f, job.Dir()); err != nil {
		return
	}

	// Start and report any errors or premature exits on the done channel
	// NOTE that for host builds, multiple instances of the runner are all
	// running with f.Root as their root directory which can lead to FS races
	// if the function's implementation is writing to the FS and expects to be
	// in a container.
	go func() {

		// TODO: extract the build command code from the OCI Container Builder
		// and have both the runner and OCI Container Builder use the same.
		if verbose {
			fmt.Printf("cd %v && go build -o f.bin\n", job.Dir())
		}
		args := []string{"build", "-o", "f.bin"}
		if verbose {
			args = append(args, "-v")
		}
		cmd := exec.CommandContext(ctx, "go", args...)
		cmd.Dir = job.Dir()
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			doneCh <- err
			return
		}
		if verbose {
			fmt.Println("build complete")
		}

		bin := filepath.Join(job.Dir(), "f.bin")
		if verbose {
			fmt.Printf("cd %v && PORT=%v %v\n", f.Root, port, bin)
		}
		cmd = exec.CommandContext(ctx, bin)
		cmd.Dir = f.Root
		cmd.Env = append(cmd.Environ(), "PORT="+port)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		// cmd.Cancel = stop // TODO: check this, introduced go 1.20
		doneCh <- cmd.Run()
	}()

	// TODO(lkingland): probably should just run these jobs synchronously and
	// allowed the caller to place the task in a separate goroutine should they
	// want to background, using ctx.Done() to signal interrupt.
	// This will require refactoring the docker.Runner as well, however, so
	// sticking with the pattern for now.
	return
}

/*
func getRunFunc(f Function) (runner, error) {

	notSupportedError := fmt.Errorf("the runtime '%v' is not currently available as a host runner.  Perhaps try running containerized.")

	switch f.Runtime {
	case "":
		return nil, fmt.Errorf("runner requires the function have runtime set")
	case "go":
		return runFunc(runGo(ctx, f))
	case "python":
		return runPython(ctx, f)
	case "java":
		return nil, runnerNotImplemeted(f.Runtime)
	case "node":
		return nil, runnerNotImplemeted(f.Runtime)
	case "rust":
		return nil, runnerNotImplemeted(f.Runtime)
	default:
		return nil, fmt.Errorf("runner does not recognized the %q runtime", f.Runtime)
	}
}
*/

type runnerNotImplemented struct {
	Runtime string
}

func (e runnerNotImplemented) Error() string {
	return fmt.Sprintf("the runtime %q is not supported by the host runner.  Try running containerized.", e.Runtime)
}

// choosePort returns an unused port
// Note this is not fool-proof becase of a race with any other processes
// looking for a port at the same time.  If that is important, we can implement
// a check-lock-check via the filesystem.
// Also note that TCP is presumed.
func choosePort(host string, preferredPort string, dialTimeout time.Duration) string {
	var (
		port = defaultRunPort
		c    net.Conn
		l    net.Listener
		err  error
	)
	// Try preferreed
	if c, err = net.DialTimeout("tcp", net.JoinHostPort(host, port), dialTimeout); err == nil {
		c.Close() // note err==nil
		return preferredPort
	}

	// OS-chosen
	if l, err = net.Listen("tcp", net.JoinHostPort(host, "")); err != nil {
		fmt.Fprintf(os.Stderr, "unable to check for open ports. using fallback %v. %v", defaultRunPort, err)
		return port
	}
	l.Close() // begins aforementioned race
	if _, port, err = net.SplitHostPort(l.Addr().String()); err != nil {
		fmt.Fprintf(os.Stderr, "error isolating port from '%v'. %v", l.Addr(), err)
	}
	return port
}
