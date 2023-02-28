package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	giturls "github.com/whilp/git-urls"
	"gopkg.in/yaml.v2"

	fn "knative.dev/func/pkg/functions"
)

// Local configuration settings.
// Affect how a specifc function is to be treated on a specific machine.
// These are settings specific to the intersection of a function and an
// environment; not metadata of the Function itself to be tracked in source
// control etc.
type Local struct {
	DeployRemote bool   `yaml:"deploy-remote,omitempty"`
	GitURL       string `yaml:"git-urlremote,omitempty"`
	GitRef       string `yaml:"git-ref,omitempty"`
	GitDir       string `yaml:"git-dir,omitempty"`
}

// LoadLocal Config
func LoadLocal(root string) (lcfg Local, err error) {
	path := filepath.Join(root, fn.RunDataDir, Filename)
	bb, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			err = nil // config file is not required
		}
		return
	}
	err = yaml.Unmarshal(bb, &lcfg) // cfg now has applied config.yaml
	return
}

// Validate values populated are syntacticaly correct
func (c Local) Validate() (errors []string) {
	if c.GitURL != "" {
		_, err := giturls.ParseTransport(c.GitURL)
		if err != nil {
			_, err = giturls.ParseScp(c.GitURL)
		}
		if err != nil {
			errMsg := fmt.Sprintf("specified option \"git.url=%s\" is not valid", c.GitURL)

			originalErr := err.Error()
			if !strings.HasSuffix(originalErr, "is not a valid transport") {
				errMsg = fmt.Sprintf("%s, error: %s", errMsg, originalErr)
			}
			errors = append(errors, errMsg)
		}
	}
	return
}

// Write the config to the given path
// To use the currently configured path (used by the constructor) use File()
//
//	c := config.NewDefault()
//	c.Verbose = true
//	c.Write(config.File())
func (c Local) Write(path string) (err error) {
	bb, _ := yaml.Marshal(&c) // Marshaling no longer errors; this is back compat
	return os.WriteFile(path, bb, os.ModePerm)
}

// WriteFor writes the given local config for the given function
// (into the function's .Root at .func/config.yaml)
func (c Local) WriteFor(f fn.Function) error {
	return c.Write(filepath.Join(f.Root, fn.RunDataDir, Filename))
}
