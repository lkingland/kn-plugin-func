package oci

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/term"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	progress "github.com/schollz/progressbar/v3"
	fn "knative.dev/func/pkg/functions"
)

// Pusher of OCI multi-arch layout directories.
type Pusher struct {
	verbose bool
	updates chan v1.Update
	done    chan bool
}

func NewPusher(verbose bool) *Pusher {
	return &Pusher{
		verbose: verbose,
		updates: make(chan v1.Update, 10),
		done:    make(chan bool, 1),
	}
}

func (p *Pusher) Push(ctx context.Context, f fn.Function) (digest string, err error) {
	go p.handleUpdates(ctx)
	defer func() { p.done <- true }()
	// TODO: GitOps Tagging: tag :latest by default, :[branch] for pinned
	// environments and :[user]-[branch] for development/testing feature branches.
	// has been enabled, where branch is tag-encoded.
	ref, err := name.ParseReference(f.Image)
	if err != nil {
		return
	}
	buildDir, err := getLastBuildDir(f)
	if err != nil {
		return
	}
	ii, err := layout.ImageIndexFromPath(filepath.Join(buildDir, "oci"))
	if err != nil {
		return
	}
	err = remote.WriteIndex(ref, ii, p.remoteOptions(ctx)...)
	if err != nil {
		return
	}
	h, err := ii.Digest()
	if err != nil {
		return
	}
	digest = h.String()
	if p.verbose {
		fmt.Printf("\ndigest: %s\n", h)
	}
	return
}

// The last build directory is symlinked upon successful build.
func getLastBuildDir(f fn.Function) (string, error) {
	dir := filepath.Join(f.Root, fn.RunDataDir, "builds", "last")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return dir, fmt.Errorf("last build directory not found '%v'. Has it been built?", dir)
	}
	return dir, nil
}

func (p *Pusher) remoteOptions(ctx context.Context) []remote.Option {
	return []remote.Option{
		remote.WithContext(ctx),
		remote.WithProgress(p.updates),
		// remote.WithTransport
		// remote.WithAuth
		// remote.WithAuthFromKeychain
	}
}

func (p *Pusher) handleUpdates(ctx context.Context) {
	var bar *progress.ProgressBar
	for {
		select {
		case update := <-p.updates:
			if bar == nil {
				bar = progress.NewOptions64(update.Total,
					progress.OptionSetVisibility(term.IsTerminal(int(os.Stdin.Fd()))),
					progress.OptionSetDescription("pushing"),
					progress.OptionShowCount(),
					progress.OptionShowBytes(true),
					progress.OptionShowElapsedTimeOnFinish())
			}
			bar.Set64(update.Complete)
			continue
		case <-p.done:
			if bar != nil {
				_ = bar.Finish()
			}
			return
		case <-ctx.Done():
			if bar != nil {
				_ = bar.Finish()
			}
			return
		}
	}

}
