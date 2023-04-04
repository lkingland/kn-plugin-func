package functions

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v2"
)

// Signauture defines the scaffolding code necessary to expose a function as
// a service.
type Signature int

const (
	UnknownSignature Signature = iota
	InstancedHTTP
	InstancedCloudEvent
	StaticHTTP
	StaticCloudEvent
)

func (s Signature) String() string {
	return []string{"unknown", "instanced-http", "instanced-cloudevent", "static-http", "static-cloudevent"}[s]
}

// detectSignature returns whether or not the function is of the given signature.
func detectSignature(f Function) (Signature, error) {
	if f.Root == "" {
		return UnknownSignature, errors.New("function root required to check for signature")
	}
	if f.Runtime == "" {
		return UnknownSignature, errors.New("function runtime required to check for signature")
	}
	for k, v := range detectors[f.Runtime] {
		if v(f.Root) {
			fmt.Printf("detected signature: %v\n", k)
			return k, nil
		}
	}
	return UnknownSignature, nil
}

// TODO: Please note this implementaion is a stop-gap method to detect
// a function's method signature comprised of parsing func.go for the
// invocation hint and running a regex pattern match for a constructor or
// a static Handle method.
// This latter regex step is not ideal because it can break if a correct
// signature exists in a comment block, etc.  These should be replaced with a
// language parser.
type detector func(path string) (matched bool)

var detectors = map[string]map[Signature]detector{
	"go": {
		InstancedHTTP:       func(p string) bool { return !isCE(p) && match(p, ".go", `^func New\(\)`) },
		InstancedCloudEvent: func(p string) bool { return isCE(p) && match(p, ".go", `^func New\(\)`) },
		StaticHTTP:          func(p string) bool { return !isCE(p) && match(p, ".go", `^func Handle\(`) },
		StaticCloudEvent:    func(p string) bool { return isCE(p) && match(p, ".go", `^func Handle\(`) },
	},
	"python": {
		InstancedHTTP:       func(p string) bool { return false },
		InstancedCloudEvent: func(p string) bool { return false },
		StaticHTTP:          func(p string) bool { return false },
		StaticCloudEvent:    func(p string) bool { return false },
	},
}

func isCE(path string) bool {
	f := Function{}
	bb, err := os.ReadFile(filepath.Join(path, FunctionFile))
	if err != nil {
		return false
	}
	if err = yaml.Unmarshal(bb, &f); err != nil {
		return false
	}
	return f.Invoke == "cloudevent"
}

// match by pattern checks all files in the given path (nonrecursively) of the
// given extension for the given regex pattern.
// This is a simple, blung instrument to detect whether a function's source
// code is attempting to implement a given function signature, and would of
// course be better off implemented as a proper language parser; however since
// this must be executed on-demand, it has to be quick.  Scanning files in the
// target directory is indeed pretty quick.
func match(path, extension, pattern string) bool {
	rx, err := regexp.Compile(pattern)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing '%v' detector pattern '%v'. %v\n", extension, pattern, err)
	}
	dd, err := os.ReadDir(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading path '%v'. %v\n", path, err)
	}
	for _, d := range dd {
		if !strings.HasSuffix(d.Name(), extension) {
			continue
		}
		if !d.Type().IsRegular() {
			continue
		}
		f, err := os.Open(filepath.Join(path, d.Name()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to scan file '%v', %v\n", d.Name(), err)
			continue
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			if rx.MatchString(scanner.Text()) {
				return true
			}
		}
		f.Close()
	}
	return false
}
