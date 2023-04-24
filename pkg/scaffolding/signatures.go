package scaffolding

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

type Signature int

const (
	UnknownSignature Signature = iota
	InstancedHTTP
	InstancedCloudevent
	StaticHTTP
	StaticCloudevent
)

func (s Signature) String() string {
	return []string{
		"unknown",
		"instanced-http",
		"instanced-cloudevent",
		"static-http",
		"static-cloudevent",
	}[s]
}

var signatureMap = map[bool]map[string]Signature{
	true: {
		"http":       InstancedHTTP,
		"cloudevent": InstancedCloudevent},
	false: {
		"http":       StaticHTTP,
		"cloudevent": StaticCloudevent},
}

// toSignature converts an instanced boolean and invocation hint into
// a Signature enum.
func toSignature(instanced bool, invoke string) Signature {
	if invoke == "" {
		invoke = "http"
	}
	s, ok := signatureMap[instanced][invoke]
	if !ok {
		return UnknownSignature
	}
	return s
}

// detector checks for the existence of certain method signatures in the
// source code at the given root.
type detector interface {
	Detect(dir string) (static, instanced bool, err error)
}

// signature returns the signature implemented by the given src location
// presuming the given runtime and invocation hint (default http).
func signature(src, runtime, invoke string) (s Signature, err error) {
	d, err := runtimeDetector(runtime)
	if err != nil {
		return UnknownSignature, err
	}
	static, instanced, err := d.Detect(src)
	if err != nil {
		return
	}
	// Function must implement either a static handler or the instanced handler
	// but not both.
	if static && instanced {
		return s, fmt.Errorf("function may not implement both the static and instanced method signatures simultaneously")
	} else if !static && !instanced {
		return s, fmt.Errorf("function does not appear to implement any known method signatures")
	} else if instanced {
		return toSignature(true, invoke), nil
	} else {
		return toSignature(false, invoke), nil
	}
	return
}

// runtimeDetector runtime returns a signature detector for a given runtime
func runtimeDetector(runtime string) (detector, error) {
	switch runtime {
	case "go":
		return &goDetector{}, nil
	case "python":
		return &pythonDetector{}, nil
	case "rust":
		return nil, errors.New("The Rust signature detector is not yet available.")
	case "node":
		return nil, errors.New("The Node.js signature detector is not yet available.")
	case "quarkus":
		return nil, errors.New("The TypeScript signature detector is not yet available.")
	default:
		return nil, fmt.Errorf("Unable to detect the signature of the unrecognized runtime language %q", runtime)
	}
}

// GO

type goDetector struct{}

func (d goDetector) Detect(dir string) (static, instanced bool, err error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		err = fmt.Errorf("signature detector encountered an error when scanning the function's source code. %w", err)
		return
	}
	for _, file := range files {
		filename := filepath.Join(dir, file.Name())
		if file.IsDir() || !strings.HasSuffix(filename, ".go") {
			continue
		}
		if d.hasFunctionDeclaration(filename, "New") {
			instanced = true
		}
		if d.hasFunctionDeclaration(filename, "Handle") {
			static = true
		}
	}
	return
}

func (d goDetector) hasFunctionDeclaration(filename, function string) bool {
	astFile, err := parser.ParseFile(token.NewFileSet(), filename, nil, parser.SkipObjectResolution)
	if err != nil {
		return false
	}
	for _, decl := range astFile.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok {
			if funcDecl.Name.Name == function {
				return true
			}
		}
	}
	return false
}

// PYTHON

type pythonDetector struct{}

func (d pythonDetector) Detect(dir string) (bool, bool, error) {
	return false, false, errors.New("the Python method signature detector is not yet available.")
}
