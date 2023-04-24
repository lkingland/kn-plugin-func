package scaffolding

import (
	"fmt"
	"os"
	"path/filepath"

	"knative.dev/func/pkg/filesystem"
)

// Write scaffolding to a given path
//
// Scaffolding is a language-level operation which first detects the method
// signature used by the function's source code and then writes the
// appropriate scaffolding.
//
// NOTE: Scaffoding is not per-template, because a template is merely an
// example starting point for a Function implementation and should have no
// bearing on the shape that function can eventually take.  The language,
// and optionally invocation hint (For cloudevents) are used for this.  For
// example, there can be multiple templates which exemplify a given method
// signature, and the implementation can be switched at any time by the author.
// Language, by contrast, is fixed at time of initialization.
//
//	out:     the path to output scaffolding
//	src:      the path to the source code to scaffold
//	runtime: the expected runtime of the target source code "go", "node" etc.
//	invoke:  the optional invocatin hint (default "http")
//	fs:      filesytem which contains scaffolding at '[runtime]/scaffolding'
//	         (exclusive with 'repo')
func Write(out, src, runtime, invoke string, fs filesystem.Filesystem) (err error) {

	// signature of the source code in the given location, presuming a runtime
	// and invocation hint (default "http")
	s, err := signature(src, runtime, invoke)
	if err != nil {
		return nil
	}

	// Path in the filesystem at which scaffolding is expected to exist
	d := filepath.Join(runtime, "scaffolding", s.String())
	if _, err := fs.Stat(d); err != nil {
		return fmt.Errorf("no scaffolding found for '%v' signature '%v'. %v.  Note that this test requires the filesystem be regenerated (try make first).", runtime, s, err)
	}

	// Copy from d -> out from the filesystem
	if err := filesystem.CopyFromFS(d, out, fs); err != nil {
		return err
	}

	// Replace the 'f' link of the scaffolding (which is now incorrect) to
	// link to the function's root.
	rel, err := filepath.Rel(out, src)
	if err != nil {
		return fmt.Errorf("error determining relative path to function source %w", err)
	}
	link := filepath.Join(out, "f")
	_ = os.Remove(link)
	if err = os.Symlink(rel, link); err != nil {
		return fmt.Errorf("error linking scaffolding to source %w", err)
	}
	return
}
