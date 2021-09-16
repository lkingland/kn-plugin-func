package function

import (
	"errors"
	"io"
	"os"
	"strings"
)

// Templates Manager
type Templates struct {
	Repositories *Repositories // Repository Manager
}

// Template metadata
type Template struct {
	Runtime    string
	Repository string
	Name       string
}

// Fullname is a caluclate field of [repo]/[name] used
// to uniquely reference a template which may share a name
// with one in another repository.
func (t Template) Fullname() string {
	return t.Repository + "/" + t.Name
}

// List the full name of templates available runtime.
// Full name is the optional repository prefix plus the template's repository
// local name.  Default templates grouped first sans prefix.
func (t *Templates) List(runtime string) ([]string, error) {
	// TODO: if repository override was enabled, we should just return those, flat.
	builtin, err := t.ListDefault(runtime)
	if err != nil {
		return []string{}, err
	}

	extended, err := t.ListExtended(runtime)
	if err != nil {
		return []string{}, err
	}

	// Result is an alphanumerically sorted list first grouped by
	// embedded at head.
	return append(builtin, extended...), nil
}

// ListDefault (embedded) templates by runtime
func (t *Templates) ListDefault(runtime string) ([]string, error) {
	var (
		names     = newSortedSet()
		repo, err = t.Repositories.Get(DefaultRepository)
	)
	if err != nil {
		return []string{}, err
	}
	for _, template := range repo.Templates {
		if template.Runtime != runtime {
			continue
		}
		names.Add(template.Name)
	}
	return names.Items(), nil
}

// ListExtended templates returns all template full names that
// exist in all extended (config dir) repositories for a runtime.
// Prefixed, sorted.
func (t *Templates) ListExtended(runtime string) ([]string, error) {
	var (
		names      = newSortedSet()
		repos, err = t.Repositories.All()
	)
	if err != nil {
		return []string{}, err
	}
	for _, repo := range repos {
		if repo.Name == DefaultRepository {
			continue // already added at head of names
		}
		for _, template := range repo.Templates {
			if template.Runtime != runtime {
				continue
			}
			names.Add(template.Fullname())
		}
	}
	return names.Items(), nil
}

// Template returns the named template in full form '[repo]/[name]' for the
// specified runtime.
// Templates from the default repository do not require the repo name prefix,
// though it can be provided.
func (t *Templates) Get(runtime, fullname string) (Template, error) {
	var (
		template Template
		repoName string
		tplName  string
		repo     Repository
		err      error
	)

	// Split into repo and template names.
	// Defaults when unprefixed to DefaultRepository
	cc := strings.Split(fullname, "/")
	if len(cc) == 1 {
		repoName = DefaultRepository
		tplName = fullname
	} else {
		repoName = cc[0]
		tplName = cc[1]
	}

	// Get specified repository
	repo, err = t.Repositories.Get(repoName)
	if err != nil {
		return template, err
	}

	return repo.GetTemplate(runtime, tplName)
}

// Writing ------

type filesystem interface {
	Stat(name string) (os.FileInfo, error)
	Open(path string) (file, error)
	ReadDir(path string) ([]os.FileInfo, error)
}

type file interface {
	io.Reader
	io.Closer
}

type templateWriter struct {
	// Extensible Template Repositories
	// templates on disk (extensible templates)
	// Stored on disk at path:
	//   [customTemplatesPath]/[repository]/[runtime]/[template]
	// For example
	//   ~/.config/func/boson/go/http"
	// Specified when writing templates as simply:
	//   Write([runtime], [repository], [path])
	// For example
	// w := templateWriter{templates:"/home/username/.config/func/templates")
	//   w.Write("go", "boson/http")
	// Ie. "Using the custom templates in the func configuration directory,
	//    write the Boson HTTP template for the Go runtime."
	repositories string

	// URL of a a specific network-available Git repository to use for
	// templates.  Takes precidence over both builtin and extensible
	// if defined.
	url string

	// enable verbose logging
	verbose bool
}

var (
	ErrRepositoryNotFound        = errors.New("repository not found")
	ErrRepositoriesNotDefined    = errors.New("custom template repositories location not specified")
	ErrRuntimeNotFound           = errors.New("runtime not found")
	ErrTemplateNotFound          = errors.New("template not found")
	ErrTemplateMissingRepository = errors.New("template name missing repository prefix")
)

func (t templateWriter) Write(runtime, template, dest string) error {
	return errors.New("not implemented")
}
