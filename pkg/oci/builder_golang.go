package oci

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
	fn "knative.dev/func/pkg/functions"
)

// TODO: generalize to a single newExec which takes a set of exec files for
// the given language and move this back into builder.go
func newExecLayerGo(cfg buildConfig, p v1.Platform, verbose bool) (desc v1.Descriptor, layer v1.Layer, err error) {
	// Create a temporary tarfile
	tarPath := filepath.Join(cfg.buildDir, fmt.Sprintf("execlayer.%v.%v.tar.gz", p.OS, p.Architecture))
	if cfg.verbose {
		fmt.Printf("Creating %v %v exec layer: %v\n", p.OS, p.Architecture, rel(cfg.buildDir, tarPath))
	}
	tf, err := os.Create(tarPath)
	if err != nil {
		return
	}
	gz := gzip.NewWriter(tf)
	tw := tar.NewWriter(gz)

	// Create binary
	binPath, err := goBuild(cfg, p, verbose) // Path to the binary
	if err != nil {
		return
	}
	binInfo, err := os.Stat(binPath)
	if err != nil {
		return
	}
	binFile, err := os.Open(binPath)
	if err != nil {
		return
	}
	defer binFile.Close()

	// Write all files to /func
	header, err := tar.FileInfoHeader(binInfo, binPath)
	if err != nil {
		return
	}
	header.Name = filepath.Join("/func", "f")
	header.ModTime = cfg.timestamp
	// header.Mode = 0555 // TODO: would pegging mode help?
	if err = tw.WriteHeader(header); err != nil {
		return
	}
	if cfg.verbose {
		fmt.Printf("→ %v \n", header.Name)
	}
	if _, err = io.Copy(tw, binFile); err != nil {
		return
	}

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
		Platform:  &p,
	}

	// Move the content-addressible objec to blobs and return the descriptor
	blobPath := filepath.Join(cfg.blobsDir, dld.Hex)
	err = os.Rename(tarPath, blobPath)
	if cfg.verbose {
		fmt.Printf("mv %v %v\n", rel(cfg.buildDir, tarPath), rel(cfg.buildDir, blobPath))
	}
	return
}

func goBuild(cfg buildConfig, p v1.Platform, verbose bool) (binPath string, err error) {
	gobin, args, outpath, err := goBuildCmd(p, cfg)
	if err != nil {
		return
	}
	if verbose {
		fmt.Printf(" %v\n", gobin, strings.Join(args, " "))
	} else {
		fmt.Printf("   %v\n", filepath.Base(outpath))
	}
	cmd := exec.CommandContext(cfg.ctx, gobin, args...)
	cmd.Env = goBuildEnvs(cfg.f, p)
	cmd.Dir = cfg.buildDir
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	return outpath, cmd.Run()
}

func goBuildCmd(p v1.Platform, cfg buildConfig) (gobin string, args []string, outpath string, err error) {
	// Use Build Command override from the function if provided
	if cfg.f.Build.BuildCommand != "" {
		if strings.Contains(cfg.f.Build.BuildCommand, "toolexec") {
			err = errors.New("function build command may not include 'toolexec'")
			return
		}
		pp := strings.Split(cfg.f.Build.BuildCommand, " ")
		return pp[0], pp[1:], "", nil
	}

	// Use the binary specified FUNC_GO_PATH if defined
	gobin = os.Getenv("FUNC_GO_PATH") // TODO: move to main and plumb through
	if gobin == "" {
		gobin = "go"
	}

	// Build as ./func/builds/$PID/result/f.$OS.$Architecture
	name := fmt.Sprintf("f.%v.%v", p.OS, p.Architecture)
	if p.Variant != "" {
		name = name + "." + p.Variant
	}
	outpath = filepath.Join(cfg.buildDir, "result", name)
	args = []string{"build", "-o", outpath}
	return gobin, args, outpath, nil
}

func goBuildEnvs(f fn.Function, p v1.Platform) []string {
	envs := []string{
		"HOME=" + os.Getenv("HOME"),
		"CGO_ENABLED=0",
		"GOOS=" + p.OS,
		"GOARCH=" + p.Architecture,
	}
	if p.Variant != "" && p.Architecture == "arm" {
		envs = append(envs, "GOARM="+strings.TrimPrefix(p.Variant, "v"))
	} else if p.Variant != "" && p.Architecture == "amd64" {
		envs = append(envs, "GOAMD64="+p.Variant)
	}
	for _, e := range f.Build.BuildEnvs {
		envs = append(envs, e.KeyValuePair())
	}
	return envs
}
