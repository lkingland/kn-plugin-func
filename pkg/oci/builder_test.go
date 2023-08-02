package oci

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	fn "knative.dev/func/pkg/functions"
	. "knative.dev/func/pkg/testing"
)

var TestPlatforms = []fn.Platform{{OS: runtime.GOOS, Architecture: runtime.GOARCH}}

// TestBuilder_Build ensures that, when given a Go Function, an OCI-compliant
// directory structure is created on .Build in the expected path.
func TestBuilder_Build(t *testing.T) {
	root, done := Mktemp(t)
	defer done()

	client := fn.New(fn.WithVerbose(true))

	f, err := client.Init(fn.Function{Root: root, Runtime: "go"})
	if err != nil {
		t.Fatal(err)
	}

	builder := NewBuilder("", true)

	if err := builder.Build(context.Background(), f, TestPlatforms); err != nil {
		t.Fatal(err)
	}

	last := path(f.Root, fn.RunDataDir, "builds", "last", "oci")

	validateOCI(last, t)
}

// TestBuilder_Concurrency
func TestBuilder_Concurrency(t *testing.T) {
	root, done := Mktemp(t)
	defer done()

	client := fn.New()

	// Initialize a new Go Function
	f, err := client.Init(fn.Function{Root: root, Runtime: "go"})
	if err != nil {
		t.Fatal(err)
	}

	// Concurrency
	//
	// The first builder is setup to use a mock implementation of the
	// builder function which will block until released after first notifying
	// that it has been paused.
	//
	// When the test receives the message that the builder has been paused, it
	// starts a second, concurrently executing builder to ensure there is a
	// typed error returned indicating a build is in progress.
	//
	// When the second builder completes, having confirmed the error message
	// received is as expected.  It signals the first (blocked) builder that it
	// can now continue.

	// Thet test waits until the first builder notifies that it is done, and
	// has therefore ran its tests as well.

	var (
		pausedCh   = make(chan bool)
		continueCh = make(chan bool)
		doneCh     = make(chan bool)
	)

	// Build A
	builder1 := NewBuilder("builder1", true)
	builder1.buildFn = func(cfg *buildConfig, p v1.Platform) (d v1.Descriptor, l v1.Layer, err error) {
		if isFirstBuild(cfg, p) {
			pausedCh <- true // Notify of being paused
			<-continueCh     // Block until released
		}
		return
	}
	builder1.onDone = func() {
		doneCh <- true // Notify of being done
	}
	go func() {
		if err := builder1.Build(context.Background(), f, TestPlatforms); err != nil {
			fmt.Fprintf(os.Stderr, "test build error %v", err)
		}
	}()

	//  Wait until build 1 indicates it is paused
	<-pausedCh

	// Build B
	builder2 := NewBuilder("builder2", true)
	go func() {
		err = builder2.Build(context.Background(), f, TestPlatforms)
		if !errors.As(err, &ErrBuildInProgress{}) {
			fmt.Fprintf(os.Stderr, "test build error %v", err)
		}
	}()

	// Release the blocking Build A and wait until complete.
	continueCh <- true
	<-doneCh
}

// TestBuilder_StaticEnvs ensures that certain "static" environment variables
// comprising Function metadata are added to the config.
func TestBuilder_StaticEnvs(t *testing.T) {
	root, done := Mktemp(t)
	defer done()

	client := fn.New()
	f, err := client.Init(fn.Function{Root: root, Runtime: "go"})
	if err != nil {
		t.Fatal(err)
	}

	builder := NewBuilder("", true)

	if err := builder.Build(context.Background(), f, TestPlatforms); err != nil {
		t.Fatal(err)
	}

	// Assert
	// Check if the OCI container defines at least one of the static
	// variables on each of the constituent containers.
	// ---
	// Get the images list (manifest descripors) from the index
	ociPath := path(f.Root, fn.RunDataDir, "builds", "last", "oci")
	data, err := ioutil.ReadFile(filepath.Join(ociPath, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index struct {
		Manifests []struct {
			Digest string `json:"digest"`
		} `json:"manifests"`
	}
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatal(err)
	}
	for _, manifestDesc := range index.Manifests {

		// Dereference the manifest descriptor into the referenced image manifest
		manifestHash := strings.TrimPrefix(manifestDesc.Digest, "sha256:")
		data, err := ioutil.ReadFile(filepath.Join(ociPath, "blobs", "sha256", manifestHash))
		if err != nil {
			t.Fatal(err)
		}
		var manifest struct {
			Config struct {
				Digest string `json:"digest"`
			} `json:"config"`
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			t.Fatal(err)
		}

		// From the image manifest get the the image's config.json
		configHash := strings.TrimPrefix(manifest.Config.Digest, "sha256:")
		data, err = ioutil.ReadFile(filepath.Join(ociPath, "blobs", "sha256", configHash))
		if err != nil {
			t.Fatal(err)
		}
		var config struct {
			Config struct {
				Env []string `json:"Env"`
			} `json:"config"`
		}
		if err := json.Unmarshal(data, &config); err != nil {
			panic(err)
		}

		// Check if one of the static envs (FUNC_CREATED) is defined
		defined := false
		for _, env := range config.Config.Env {
			if strings.HasPrefix(env, "FUNC_CREATED=") {
				t.Logf("OK: %v", env)
				defined = true
				break
			}
		}
		if !defined {
			t.Fatalf("container did not set FUNC_CREATED on image config")
		}
	}
}

func isFirstBuild(cfg *buildConfig, current v1.Platform) bool {
	first := cfg.platforms[0]
	return current.OS == first.OS &&
		current.Architecture == first.Architecture &&
		current.Variant == first.Variant
}

// ImageIndex represents the structure of an OCI Image Index.
type ImageIndex struct {
	SchemaVersion int `json:"schemaVersion"`
	Manifests     []struct {
		MediaType string `json:"mediaType"`
		Size      int64  `json:"size"`
		Digest    string `json:"digest"`
		Platform  struct {
			Architecture string `json:"architecture"`
			OS           string `json:"os"`
		} `json:"platform"`
	} `json:"manifests"`
}

// validateOCI performs a cursory check that the given path exists and
// has the basics of an OCI compliant structure.
func validateOCI(path string, t *testing.T) {
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("unable to stat output path. %v", path)
		return
	}

	ociLayoutFile := filepath.Join(path, "oci-layout")
	indexJSONFile := filepath.Join(path, "index.json")
	blobsDir := filepath.Join(path, "blobs")

	// Check if required files and directories exist
	if _, err := os.Stat(ociLayoutFile); os.IsNotExist(err) {
		t.Fatal("missing oci-layout file")
	}
	if _, err := os.Stat(indexJSONFile); os.IsNotExist(err) {
		t.Fatal("missing index.json file")
	}
	if _, err := os.Stat(blobsDir); os.IsNotExist(err) {
		t.Fatal("missing blobs directory")
	}

	// Load and validate index.json
	indexJSONData, err := os.ReadFile(indexJSONFile)
	if err != nil {
		t.Fatalf("failed to read index.json: %v", err)
	}

	var imageIndex ImageIndex
	err = json.Unmarshal(indexJSONData, &imageIndex)
	if err != nil {
		t.Fatalf("failed to parse index.json: %v", err)
	}

	if imageIndex.SchemaVersion != 2 {
		t.Fatalf("invalid schema version, expected 2, got %d", imageIndex.SchemaVersion)
	}

	if len(imageIndex.Manifests) < 1 {
		t.Fatal("fewer manifests")
	}

	digest := strings.TrimPrefix(imageIndex.Manifests[0].Digest, "sha256:")
	manifestFile := filepath.Join(path, "blobs", "sha256", digest)
	manifestFileData, err := os.ReadFile(manifestFile)
	if err != nil {
		t.Fatal(err)
	}
	mf := struct {
		Layers []struct {
			Digest string `json:"digest"`
		} `json:"layers"`
	}{}
	err = json.Unmarshal(manifestFileData, &mf)
	if err != nil {
		t.Fatal(err)
	}

	type fileInfo struct {
		Path       string
		Type       fs.FileMode
		Executable bool
	}
	var files []fileInfo

	for _, layer := range mf.Layers {
		func() {
			digest = strings.TrimPrefix(layer.Digest, "sha256:")
			f, err := os.Open(filepath.Join(path, "blobs", "sha256", digest))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()

			gr, err := gzip.NewReader(f)
			if err != nil {
				t.Fatal(err)
			}
			defer gr.Close()

			tr := tar.NewReader(gr)
			for {
				hdr, err := tr.Next()
				if err != nil {
					if errors.Is(err, io.EOF) {
						break
					}
					t.Fatal(err)
				}
				files = append(files, fileInfo{
					Path:       hdr.Name,
					Type:       hdr.FileInfo().Mode() & fs.ModeType,
					Executable: (hdr.FileInfo().Mode()&0111 == 0111) && !hdr.FileInfo().IsDir(),
				})
			}
		}()
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	expectedFiles := []fileInfo{
		{Path: "/etc/pki/tls/certs/ca-certificates.crt"},
		{Path: "/etc/ssl/certs/ca-certificates.crt"},
		{Path: "/func", Type: fs.ModeDir},
		{Path: "/func/README.md"},
		{Path: "/func/f", Executable: true},
		{Path: "/func/func.yaml"},
		{Path: "/func/go.mod"},
		{Path: "/func/handle.go"},
		{Path: "/func/handle_test.go"},
	}

	if diff := cmp.Diff(expectedFiles, files); diff != "" {
		t.Error("files in oci differ from expectation (-want, +got):", diff)
	}
}
