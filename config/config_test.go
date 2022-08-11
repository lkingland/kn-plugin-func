package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"knative.dev/kn-plugin-func/config"

	. "knative.dev/kn-plugin-func/testing"
)

// TestNewDefaults ensures that the default Config
// constructor yelds a struct prepopulated with static
// defaults.
func TestNewDefaults(t *testing.T) {
	cfg := config.New()
	if cfg.Language != config.DefaultLanguage {
		t.Fatalf("expected config's language = '%v', got '%v'", config.DefaultLanguage, cfg.Language)
	}
}

// TestLoad ensures that loading a config reads values
// in from a config file at path.
func TestLoad(t *testing.T) {
	cfg, err := config.Load("testdata/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Language != "custom" {
		t.Fatalf("loaded config did not contain values from config file.  Expected \"custom\" got \"%v\"", cfg.Language)
	}
}

// TestSave ensures that saving an update config persists.
func TestSave(t *testing.T) {
	// mktmp
	root, rm := Mktemp(t)
	defer rm()

	// touch config.yaml
	filename := filepath.Join(root, "config.yaml")

	// update
	cfg := config.New()
	cfg.Language = "testSave"

	// save
	if err := cfg.Save(filename); err != nil {
		t.Fatal(err)
	}

	// reload
	cfg, err := config.Load(filename)
	if err != nil {
		t.Fatal(err)
	}

	// assert persisted
	if cfg.Language != "testSave" {
		t.Fatalf("config did not persist.  expected 'testSave', got '%v'", cfg.Language)
	}
}

// TestPath ensures that the Path returns
// XDG_CONFIG_HOME/.config/func
func TestPath(t *testing.T) {
	home := t.TempDir()                 // root of all configs
	path := filepath.Join(home, "func") // our config

	t.Setenv("XDG_CONFIG_HOME", home)
	fmt.Printf("Checking XDG_CONFIG_HOME: %v\n", os.Getenv("XDG_CONFIG_HOME"))

	if config.Path() != path {
		t.Fatalf("expected config path '%v', got '%v'", path, config.Path())
	}
}
