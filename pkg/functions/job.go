package functions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Job represents a running function job (presumably started by this process'
// Runner instance.
type Job struct {
	Function Function
	Port     string
	Errors   chan error
	onStop   func() error
	verbose  bool
}

// Create a new Job which represents a running function task by providing
// the port on which it was started, a channel on which runtime errors can
// be received, and a stop function.
func NewJob(f Function, port string, errs chan error, onStop func() error, verbose bool) (*Job, error) {
	j := &Job{
		Function: f,
		Port:     port,
		Errors:   errs,
		onStop:   onStop,
		verbose:  verbose,
	}
	if j.Port == "" {
		return j, errors.New("port required to create job")
	}
	// mkdir -p ${f.Root}/.func/runs/${j.Port}
	if verbose {
		fmt.Printf("mkdir -p %v\n", j.Dir())
	}
	return j, os.MkdirAll(j.Dir(), os.ModePerm)
}

// Stop the Job, running the provided stop delegate and removing runtime
// metadata from disk.
func (j *Job) Stop() error {
	if j.verbose {
		fmt.Printf("rm %v\n", j.Dir())
	}
	err := j.remove() // Remove representation on disk
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: error removing run directory. %v", err)
	}
	return j.onStop()
}

// Dir is ${f.Root}/.func/runs/${j.Port}
func (j *Job) Dir() string {
	return filepath.Join(j.Function.Root, RunDataDir, "runs", j.Port)
}

func (j *Job) remove() error {
	return os.RemoveAll(j.Dir())
}

// jobPorts returns all the ports on which an instance of the given function is
// running.  len is 0 when not running.
// Improperly initialized or nonexistent (zero value) functions are considered
// to not be running.
func jobPorts(f Function) []string {
	if f.Root == "" || !f.Initialized() {
		return []string{}
	}
	runsDir := filepath.Join(f.Root, RunDataDir, "runs")
	if _, err := os.Stat(runsDir); err != nil {
		return []string{} // never started, so path does not exist
	}

	fis, err := os.ReadDir(runsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading %v", runsDir)
		return []string{}
	}
	ports := []string{}
	for _, f := range fis {
		ports = append(ports, f.Name())
	}
	// TODO: validate it's a directory whose name parses as an integer?
	return ports
}
