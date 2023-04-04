package oci

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"

	"knative.dev/func/pkg/builders"
	fn "knative.dev/func/pkg/functions"
)

// # PUNTS
//
// Things which really should be improved.
//
// ## .funcignore
//
// there should be a default .funcignore created when a project
// is initialized from the corresponding file in the template.  This
// should by default ignore .func, .git and source code files for that language.
//
// ## Build Directory Path
//
// This implementation currently relies on the implicit existence of the
// build directory .func/builds/PID.  This is a coupling which should be
// explicit.  This could be accomplished either by creating a BuildRequest
// an BuildResponse struct which includes these things, by passing in
// as "scaffolder" to the builder's constructor, or some such.
//
// By coupling via the current process, it will cause problems if there is
// a single process running multiple builds, and has to be addressed by
// throwing an "already building" error.
//
// See other punts inline with the "todo" prefix.
//
// ## Deafult in CMD
//

const DefaultName = builders.OCI

var DefaultPlatforms = []v1.Platform{
	{OS: "linux", Architecture: "amd64"},
	{OS: "linux", Architecture: "arm64"},
	{OS: "linux", Architecture: "arm", Variant: "v6"},
	{OS: "linux", Architecture: "arm", Variant: "v7"},
	{OS: "darwin", Architecture: "amd64"},
	{OS: "darwin", Architecture: "arm64"},
}

// Builder of OCI mult-arch images.
type Builder struct {
	name    string
	verbose bool
}

// buildConfig contains various settings for a single build
type buildConfig struct {
	buildDir  string          // Path to the build directory
	imgDir    string          // Path to the OCI image directory
	blobsDir  string          // Path to the OCI image's blobs/sh256 directory
	f         fn.Function     // Function being built
	timestamp time.Time       // Timestamp for this build
	ctx       context.Context // build context
	verbose   bool            // verbose logging
}

func NewBuilder(name string, verbose bool) *Builder {
	return &Builder{
		name:    name,
		verbose: verbose,
	}
}

// Build  an OCI Mult-arch (ImageIndex) container
func (b *Builder) Build(ctx context.Context, f fn.Function) (err error) {
	cfg, err := newBuildConfig(ctx, f, b.verbose)
	if err != nil {
		return
	}

	// oci/oci-layout
	if err = writeOCILayout(cfg); err != nil {
		return
	}

	// Create a data dir layer in oci/blobs
	dataDesc, dataLayer, err := newData(cfg) // shared
	if err != nil {
		return
	}

	// Create an image for each platform
	imageDescs := []v1.Descriptor{}
	for _, p := range DefaultPlatforms { // TODO: Configurable additions.
		imageDesc, err := newImage(cfg, dataDesc, dataLayer, p, b.verbose)
		if err != nil {
			return err
		}
		imageDescs = append(imageDescs, imageDesc)
	}

	// Create the Image Index which enumerates all platform images
	_, err = newImageIndex(cfg, imageDescs)
	if err != nil {
		return
	}

	// TODO: communicating build completeness throgh returning without error
	// relies on the implicit availability of the OIC image in this process'
	// build directory.  Would be better to have a formal build result object
	// which includes a general struct which can be used by all builders to
	// communicate to the pusher where the image can be found.

	return nil
}

func newBuildConfig(ctx context.Context, f fn.Function, v bool) (cfg buildConfig, err error) {
	cfg = buildConfig{ctx: ctx, f: f, verbose: v, timestamp: time.Now()}
	if cfg.buildDir, err = getBuildDir(f); err != nil {
		return
	}
	if cfg.imgDir = filepath.Join(cfg.buildDir, "oci"); err != nil {
		return
	}
	if cfg.blobsDir = filepath.Join(cfg.imgDir, "blobs", "sha256"); err != nil {
		return
	}
	err = os.MkdirAll(cfg.blobsDir, os.ModePerm)
	return
}

// the build directory is either the currently executing build for this
// process (for operations which include building such as deployment), or
// the last build which was successful for operations which do not include
// building, such as when building is expressly disabled.  Note that in the
// latter scenario, an error will be generated if there exists no prior build.
func getBuildDir(f fn.Function) (dir string, err error) {
	// Preferentially use the build created by the current process
	current := filepath.Join(f.Root, fn.RunDataDir, "builds", "by-pid", strconv.Itoa(os.Getpid()))
	if _, err = os.Stat(current); os.IsNotExist(err) {
		// Ignore nonexistence and fall through to last build
	} else if err != nil {
		return // Do not ignore other errors such as fs problems
	} else if err == nil {
		return current, nil // Found currently in process build; use it.
	}

	// Failing to find the in-progress build, use the last build successfully
	// completed if it can be found.
	last := filepath.Join(f.Root, fn.RunDataDir, "builds", "last")
	if _, err := os.Stat(last); os.IsNotExist(err) {
		return dir, fmt.Errorf("last build not found '%v'. Has the function been built?", last)
	}
	fmt.Println("Last build found")
	return last, nil
}

func writeOCILayout(cfg buildConfig) error {
	return os.WriteFile(filepath.Join(cfg.imgDir, "oci-layout"),
		[]byte(`{ "imageLayoutVersion": "1.0.0" }`), os.ModePerm)
}

func newData(cfg buildConfig) (desc v1.Descriptor, layer v1.Layer, err error) {
	// TODO: try WithCompressedCaching?

	// Create a temporary tarfile
	tarPath := filepath.Join(cfg.buildDir, "datalayer.tar.gz")
	if cfg.verbose {
		fmt.Printf("Creating data layer: %v\n", rel(cfg.buildDir, tarPath))
	}
	tf, err := os.Create(tarPath)
	if err != nil {
		return
	}
	gz := gzip.NewWriter(tf)
	tw := tar.NewWriter(gz)

	// Create /func
	if err = tw.WriteHeader(&tar.Header{Name: "/func", Typeflag: tar.TypeDir, Mode: 0755, ModTime: cfg.timestamp}); err != nil {
		return
	}

	// Write all files to /func
	filepath.Walk(cfg.f.Root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == cfg.f.Root { // unless it is the root
			return nil
		}
		if isIgnored(info) { // or if it is explicitly ignored
			if info.IsDir() {
				return filepath.SkipDir
			} else {
				return nil
			}
		}

		header, err := tar.FileInfoHeader(info, path)
		if err != nil {
			return err
		}
		header.Name = filepath.Join("/func", filepath.ToSlash(path[len(cfg.f.Root):]))
		header.ModTime = cfg.timestamp // TODO: set the other timestamps as well?
		// header.Mode = 0555     // TODO: Would pegging mode help?
		if err = tw.WriteHeader(header); err != nil {
			return err
		}
		if cfg.verbose {
			fmt.Printf("→ %v \n", header.Name)
		}
		if info.IsDir() {
			return nil
		}
		data, err := os.Open(path)
		if err != nil {
			return err
		}
		if _, err := io.Copy(tw, data); err != nil {
			return err
		}
		return nil
	})

	// Close handles
	if err = tw.Close(); err != nil {
		return
	}
	if err = gz.Close(); err != nil {
		return
	}
	if err = tf.Close(); err != nil {
		return
	}

	// Create layer and descriptor from tarball
	if layer, err = tarball.LayerFromFile(tf.Name()); err != nil {
		return
	}
	dls, err := layer.Size()
	if err != nil {
		return
	}
	dld, err := layer.Digest()
	if err != nil {
		return
	}
	desc = v1.Descriptor{
		MediaType: types.OCILayer,
		Size:      dls,
		Digest:    dld,
	}

	// Move data into blobs
	blobPath := filepath.Join(cfg.blobsDir, dld.Hex)
	if cfg.verbose {
		fmt.Printf("mv %v %v\n", rel(cfg.buildDir, tarPath), rel(cfg.buildDir, blobPath))
	}
	err = os.Rename(tarPath, blobPath)
	return
}

func newImage(cfg buildConfig, dataDesc v1.Descriptor, dataLayer v1.Layer, p v1.Platform, verbose bool) (imageDesc v1.Descriptor, err error) {

	// Write Exec Layer as Blob -> Layer
	execDesc, execLayer, err := newExec(cfg, p, verbose)
	if err != nil {
		return
	}

	// Write Config Layer as Blob -> Layer
	configDesc, _, err := newConfig(cfg, p, dataLayer, execLayer)
	if err != nil {
		return
	}

	// Image Manifest
	image := v1.Manifest{
		SchemaVersion: 2,
		MediaType:     types.OCIManifestSchema1,
		Config:        configDesc,
		Layers:        []v1.Descriptor{dataDesc, execDesc},
	}

	// Write image manifest out as json to a tempfile
	filePath := fmt.Sprintf("image.%v.%v.json", p.OS, p.Architecture)
	file, err := os.Create(filePath)
	if err != nil {
		return
	}
	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	if err = enc.Encode(image); err != nil {
		return
	}
	file.Close()

	// Create a descriptor from hash and size
	file, err = os.Open(filePath)
	if err != nil {
		return
	}
	defer file.Close()
	hash, size, err := v1.SHA256(file)
	if err != nil {
		return
	}
	imageDesc = v1.Descriptor{
		MediaType: types.OCIManifestSchema1,
		Digest:    hash,
		Size:      size,
		Platform:  &p,
	}

	// move image into blobs
	blobPath := filepath.Join(cfg.blobsDir, hash.Hex)
	if cfg.verbose {
		fmt.Printf("mv %v %v\n", rel(cfg.buildDir, filePath), rel(cfg.buildDir, blobPath))
	}
	err = os.Rename(filePath, blobPath)
	return
}

func newExec(cfg buildConfig, p v1.Platform, verbose bool) (desc v1.Descriptor, layer v1.Layer, err error) {
	switch cfg.f.Runtime {
	case "go":
		return newExecLayerGo(cfg, p, verbose)
	case "python":
		// Likely the next to be supported after Go
		err = errors.New("Python functions are not yet supported by the host builder.")
	case "node":
		// Likely the next to be supported after Python
		err = errors.New("Node functions are not yet supported by the host builder.")
	case "rust":
		// Likely the next to be supprted after Node
		err = errors.New("Rust functions are not yet supported by the host builder.")
	default:
		// Others are not likely to be supported in the near future
		err = fmt.Errorf("The language runtime '%v' is not a recognized language by the host builder.", cfg.f.Runtime)
	}
	return
}

func newConfig(cfg buildConfig, p v1.Platform, layers ...v1.Layer) (desc v1.Descriptor, config v1.ConfigFile, err error) {
	volumes := make(map[string]struct{}) // Volumes are odd, see spec.
	for _, v := range cfg.f.Run.Volumes {
		if v.Path == nil {
			continue // TODO: remove pointers from Volume and Env struct members
		}
		volumes[*v.Path] = struct{}{}
	}

	rootfs := v1.RootFS{
		Type: "layers",
	}
	var diff v1.Hash
	for _, v := range layers {
		if diff, err = v.DiffID(); err != nil {
			return
		}
		rootfs.DiffIDs = append(rootfs.DiffIDs, diff)
	}

	config = v1.ConfigFile{
		Created:      v1.Time{cfg.timestamp},
		Architecture: p.Architecture,
		OS:           p.OS,
		OSVersion:    p.OSVersion,
		// OSFeatures:   p.OSFeatures, // TODO: need to update dep to get this
		Variant: p.Variant,
		Config: v1.Config{
			ExposedPorts: map[string]struct{}{"8080/tcp": struct{}{}},
			Env:          cfg.f.Run.Envs.Slice(),
			Cmd:          []string{"/func/f"}, // NOTE: Using Cmd because Entrypoint can not be overridden
			WorkingDir:   "/func/",
			StopSignal:   "SIGKILL",
			Volumes:      volumes,
			// Labels
			// History
		},
		RootFS: rootfs,
	}

	// Write the config out as json to a tempfile
	filePath := filepath.Join(cfg.buildDir, "config.json")
	file, err := os.Create(filePath)
	if err != nil {
		return
	}
	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	if err = enc.Encode(config); err != nil {
		return
	}
	file.Close()

	// Create a descriptor using hash and size
	file, err = os.Open(filePath)
	if err != nil {
		return
	}
	defer file.Close()
	hash, size, err := v1.SHA256(file)
	if err != nil {
		return
	}
	desc = v1.Descriptor{
		MediaType: types.OCIConfigJSON,
		Digest:    hash,
		Size:      size,
	}

	// move config into blobs
	blobPath := filepath.Join(cfg.blobsDir, hash.Hex)
	if cfg.verbose {
		fmt.Printf("mv %v %v\n", rel(cfg.buildDir, filePath), rel(cfg.buildDir, blobPath))
	}
	err = os.Rename(filePath, blobPath)
	return
}

func newImageIndex(cfg buildConfig, imageDescs []v1.Descriptor) (index v1.IndexManifest, err error) {
	index = v1.IndexManifest{
		SchemaVersion: 2,
		MediaType:     types.OCIImageIndex,
		Manifests:     imageDescs,
	}

	filePath := filepath.Join(cfg.imgDir, "index.json")
	file, err := os.Create(filePath)
	if err != nil {
		return
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	err = enc.Encode(index)
	return
}

// HELPERS
// -------

func isIgnored(info os.FileInfo) bool {
	// TODO: .funcignore
	return (info.Name() == ".git" ||
		info.Name() == ".func" ||
		info.Name() == ".funcignore" ||
		info.Name() == ".gitignore")
}

// rel is a simple prefix trim used exclusively for verbose debugging
// statements to print paths as relative to the current build directory
// rather than absolute. Returns the path relative to the current working
// build directoy.  If it is not a subpath, the full path is returned
// unchanged.
func rel(base, path string) string {
	if strings.HasPrefix(path, base) {
		return "." + strings.TrimPrefix(path, base)
	}
	return path
}
