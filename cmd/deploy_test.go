package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/ory/viper"
	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/mock"
	. "knative.dev/kn-plugin-func/testing"
)

const TestRegistry = "example.com/alice"

// TestDeploy_Default ensures that running deploy on a valid default Function
// (only required options populated; all else default) completes successfully.
func TestDeploy_Default(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()

	// A Function with the minimum required values for deployment populated.
	f := fn.Function{
		Root:     root,
		Name:     "myfunc",
		Runtime:  "go",
		Registry: "example.com/alice",
	}
	if err := fn.New().Create(f); err != nil {
		t.Fatal(err)
	}

	// Deploy using an instance of the deploy command which uses a fully default
	// (noop filled) Client.  Execution should complete without error.
	cmd := NewDeployCmd(NewClientFactory(func() *fn.Client {
		return fn.New()
	}))
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

// TestDeploy_RegistryOrImageRequired ensures that when no registry or image are
// provided, and the client has not been instantiated with a default registry,
// an ErrRegistryRequired is received.
func TestDeploy_RegistryOrImageRequired(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()

	if err := fn.New().Create(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	cmd := NewDeployCmd(NewClientFactory(func() *fn.Client {
		return fn.New()
	}))

	// If neither --registry nor --image are provided, and the client was not
	// instantiated with a default registry, a ErrRegistryRequired is expected.
	cmd.SetArgs([]string{}) // this explicit clearing of args may not be necessary
	if err := cmd.Execute(); err != nil {
		if !errors.Is(err, ErrRegistryRequired) {
			t.Fatalf("expected ErrRegistryRequired, got error: %v", err)
		}
	}

	// earlire test covers the --registry only case, test here that --image
	// also succeeds.
	cmd.SetArgs([]string{"--image=example.com/alice/myfunc"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

// TestDeploy_ImageAndRegistry ensures that both --image and --registry flags
// are persisted to the Function and visible downstream to the deployer
// (plumbed through and persisted without being exclusive)
func TestDeploy_ImageAndRegistry(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()

	if err := fn.New().Create(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	var (
		deployer = mock.NewDeployer()
		cmd      = NewDeployCmd(NewClientFactory(func() *fn.Client {
			return fn.New(fn.WithDeployer(deployer), fn.WithRegistry(TestRegistry))
		}))
	)

	// If only --registry is provided:
	// the resultant Function should have the registry populated and image
	// derived from the name.
	cmd.SetArgs([]string{"--registry=example.com/alice"})
	deployer.DeployFn = func(f fn.Function) error {
		if f.Registry != "example.com/alice" {
			t.Fatal("registry flag not provided to deployer")
		}
		return nil
	}
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// If only --image is provided:
	// the deploy should not fail, and the resultant Function should have the
	// Image member set to what was explicitly provided via the --image flag
	// (not a derived name)
	cmd.SetArgs([]string{"--image=example.com/alice/myfunc"})
	deployer.DeployFn = func(f fn.Function) error {
		if f.Image != "example.com/alice/myfunc" {
			t.Fatalf("deployer expected f.Image 'example.com/alice/myfunc', got '%v'", f.Image)
		}
		return nil
	}
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// If both --registry and --image are provided:
	// they should both be plumbed through such that downstream agents (deployer
	// in this case) see them set on the Function and can act accordingly.
	cmd.SetArgs([]string{"--registry=example.com/alice", "--image=example.com/alice/subnamespace/myfunc"})
	deployer.DeployFn = func(f fn.Function) error {
		if f.Registry != "example.com/alice" {
			t.Fatal("registry flag value not seen on the Function by the deployer")
		}
		if f.Image != "example.com/alice/subnamespace/myfunc" {
			t.Fatal("image flag value not seen on the Function by deployer")
		}
		return nil
	}
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

// TestDeploy_RemoteBuildURLPermutations ensures that the remote, build and git-url flags
// are properly respected for all permutations, including empty.
func TestDeploy_RemoteBuildURLPermutations(t *testing.T) {
	// Valid flag permutations (empty indicates flag should be omitted)
	// and a functon which will convert a permutation into flags for use
	// by the subtests.
	var (
		remoteValues = []string{"", "true", "false"}
		buildValues  = []string{"", "true", "false", "auto"}
		urlValues    = []string{"", "https://example.com/user/repo"}

		toArgs = func(remote, build, url string) []string {
			args := []string{}
			if remote != "" {
				args = append(args, fmt.Sprintf("--remote=%v", remote))
			}
			if build != "" {
				args = append(args, fmt.Sprintf("--build=%v", build))
			}
			if url != "" {
				args = append(args, fmt.Sprintf("--git-url=%v", url))
			}
			return args
		}
	)

	// returns a single test function for one possible permutation of the flags.
	newTestFn := func(remote, build, url string) func(t *testing.T) {
		return func(t *testing.T) {
			root, rm := Mktemp(t)
			defer rm()

			// Create a new Function in the temp directory
			if err := fn.New().Create(fn.Function{Runtime: "go", Root: root}); err != nil {
				t.Fatal(err)
			}

			// deploy it using the deploy commnand with flags set to the currently
			// effective flag permutation.
			var (
				deployer  = mock.NewDeployer()
				builder   = mock.NewBuilder()
				pipeliner = mock.NewPipelinesProvider()
				cmd       = NewDeployCmd(NewClientFactory(func() *fn.Client {
					return fn.New(
						fn.WithDeployer(deployer),
						fn.WithBuilder(builder),
						fn.WithPipelinesProvider(pipeliner),
						fn.WithRegistry(TestRegistry),
					)
				}))
			)
			cmd.SetArgs(toArgs(remote, build, url))
			err := cmd.Execute()

			// Assertions
			if remote != "" && remote != "false" { // default "" is == false.
				// REMOTE Assertions

				// TODO: (enhancement) allow triggering remote deploy without Git.
				// This would tar up the local filesystem and send it to the cluster
				// build and deploy. For now URL is required when triggering remote.
				if url == "" && err == nil {
					t.Fatal("error expected when --remote without a --git-url")
				} else {
					return // test successfully confirmed this error case
				}

				if !pipeliner.RunInvoked { // Remote deployer should be triggered
					t.Error("remote was not invoked")
				}
				if deployer.DeployInvoked { // Local deployer should not be triggered
					t.Error("local deployer was invoked")
				}
				if builder.BuildInvoked { // Local builder should not be triggered
					t.Error("local builder invoked")
				}

				// BUILD?
				// TODO: (enhancement) Remote deployments respect build flag values
				// of off/on/auto

				// Source Location
				// TODO: (enhancement) if git url is not provided, send local source
				// to remote deployer for use when building.

			} else {
				// LOCAL Assertions

				// TODO: (enhancement) allow --git-url when running local deployment.
				// Check that the local builder is invoked with a directive to use a
				// git repo rather than the local filesystem if building is enabled and
				// a url is provided.  For now it throws an error statign that git-url
				// is only used when --remote
				if url != "" && err == nil {
					t.Fatal("error expected when deploying from local but provided --git-url")
					return
				} else if url != "" && err != nil {
					return // test successfully confirmed this is an error case
				}

				// Remote deployer should never be triggered when deploying via local
				if pipeliner.RunInvoked {
					t.Error("remote was invoked")
				}

				// BUILD?
				if build == "" || build == "true" || build == "auto" {
					// The default case for build is auto, which is equivalent to true
					// for a newly created Function which has not yet been built.
					if !builder.BuildInvoked {
						t.Error("local builder not invoked")
					}
					if !deployer.DeployInvoked {
						t.Error("local deployer not invoked")
					}

				} else {
					// Build was explicitly disabled.
					if builder.BuildInvoked { // builder should not be invoked
						t.Error("local builder was invoked when building disabled")
					}
					if deployer.DeployInvoked { // deployer should not be invoked
						t.Error("local deployer was invoked for an unbuilt Function")
					}
					if err == nil { // Should error that it is not built
						t.Error("expected 'error: not built' not received")
					} else {
						return // test successfully confirmed this is an expected error case
					}

					// IF build was explicitly disabled, but the Function has already
					// been built, it should invoke the deployer.
					// TODO

				}

			}

			if err != nil {
				t.Fatal(err)
			}
		}
	}

	// Run all permutations
	// Run a subtest whose name is set to the args permutation tested.
	for _, remote := range remoteValues {
		for _, build := range buildValues {
			for _, url := range urlValues {
				// Run a subtest whose name is set to the args permutation tested.
				name := fmt.Sprintf("%v", toArgs(remote, build, url))
				t.Run(name, newTestFn(remote, build, url))
			}
		}
	}
}

func Test_imageWithDigest(t *testing.T) {
	// TODO(lkingland): this test relies on a side-effect of the client library
	// dependency:  the format and even existence of the func.yaml.  Since this
	// is the CLI package (cmd), This test would be more correct if refactored to
	// instantiate a command struct and invoke it using explicit argments (see
	// other tests this file).
	tests := []struct {
		name      string
		image     string
		buildType string
		pushBool  bool
		funcFile  string
		errString string
	}{
		{
			name:      "valid full name with digest, expect success",
			image:     "docker.io/4141gauron3268/static_test_digest:latest@sha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4e",
			errString: "",
			funcFile: `name: test-func
runtime: go`,
		},
		{
			name:      "valid image name, build not 'disabled', expect error",
			image:     "docker.io/4141gauron3268/static_test_digest:latest@sha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4e",
			buildType: "local",
			errString: "the --build flag 'local' is not valid when using --image with digest",
			funcFile: `name: test-func
runtime: go`,
		},
		{
			name:      "valid image name, --push specified, expect error",
			image:     "docker.io/4141gauron3268/static_test_digest:latest@sha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4e",
			pushBool:  true,
			errString: "the --push flag 'true' is not valid when using --image with digest",
			funcFile: `name: test-func
runtime: go`,
		},
		{
			name:      "invalid digest prefix, expect error",
			image:     "docker.io/4141gauron3268/static_test_digest:latest@Xsha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4e",
			errString: "value 'docker.io/4141gauron3268/static_test_digest:latest@Xsha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4e' in --image has invalid prefix syntax for digest (should be 'sha256:')",
			funcFile: `name: test-func
runtime: go`,
		},
		{
			name:      "invalid sha hash length(added X at the end), expect error",
			image:     "docker.io/4141gauron3268/static_test_digest:latest@sha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4eX",
			errString: "sha256 hash in 'sha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4eX' from --image has the wrong length (65), should be 64",
			funcFile: `name: test-func
runtime: go`,
		},
	}

	defer WithEnvVar(t, "KUBECONFIG", fmt.Sprintf("%s/testdata/kubeconfig_deploy_namespace", cwd()))()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployer := mock.NewDeployer()
			cmd := NewDeployCmd(NewClientFactory(func() *fn.Client {
				return fn.New(
					fn.WithDeployer(deployer))
			}))

			// Set flags manually & reset after.
			// Differs whether build was set via CLI (gives an error if not 'disabled')
			// or not (prints just a warning)
			if tt.buildType == "" {
				cmd.SetArgs([]string{
					fmt.Sprintf("--image=%s", tt.image),
					fmt.Sprintf("--push=%t", tt.pushBool),
				})
			} else {
				cmd.SetArgs([]string{
					fmt.Sprintf("--image=%s", tt.image),
					fmt.Sprintf("--build=%s", tt.buildType),
					fmt.Sprintf("--push=%t", tt.pushBool),
				})
			}
			defer cmd.ResetFlags()

			// set test case's func.yaml
			if err := os.WriteFile("func.yaml", []byte(tt.funcFile), os.ModePerm); err != nil {
				t.Fatal(err)
			}

			ctx := context.Background()

			_, err := cmd.ExecuteContextC(ctx)
			if err != nil {
				if err := err.Error(); tt.errString != err {
					t.Fatalf("Error expected to be (%v) but was (%v)", tt.errString, err)
				}
			}
		})
	}
}

// TestDeploy_BuilderPersistence ensures that the builder chosen is read from
// the function by default, and is able to be overridden by flags/env vars.
func TestDeploy_BuilderPersistence(t *testing.T) {
	testBuilderPersistence(t, "docker.io/tigerteam", NewDeployCmd)
}

func Test_namespaceCheck(t *testing.T) {
	// TODO(lkingland): see comment in Test_imageWithDigest. May be better to
	// refactor this test to use command struct and execute using arguments
	// (see other tests this file)
	tests := []struct {
		name      string
		registry  string
		namespace string
		funcFile  string
		expectNS  string
	}{
		{
			name:     "first deployment(no ns in func.yaml), not given via cli, expect write in func.yaml",
			registry: "docker.io/4141gauron3268",
			expectNS: "test-ns-deploy",
			funcFile: `name: test-func
runtime: go`,
		},
		{
			name:     "ns in func.yaml, not given via cli, current ns matches func.yaml",
			registry: "docker.io/4141gauron3268",
			expectNS: "test-ns-deploy",
			funcFile: `name: test-func
namespace: "test-ns-deploy"
runtime: go`,
		},
		{
			name:      "ns in func.yaml, given via cli (always override)",
			namespace: "test-ns-deploy",
			expectNS:  "test-ns-deploy",
			registry:  "docker.io/4141gauron3268",
			funcFile: `name: test-func
namespace: "non-default"
runtime: go`,
		},
		{
			name:     "ns in func.yaml, not given via cli, current ns does NOT match func.yaml",
			registry: "docker.io/4141gauron3268",
			expectNS: "non-default",
			funcFile: `name: test-func
namespace: "non-default"
runtime: go`,
		},
	}

	// create mock kubeconfig with set namespace as 'default'
	defer WithEnvVar(t, "KUBECONFIG", fmt.Sprintf("%s/testdata/kubeconfig_deploy_namespace", cwd()))()

	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {
			deployer := mock.NewDeployer()
			defer Fromtemp(t)()
			cmd := NewDeployCmd(NewClientFactory(func() *fn.Client {
				return fn.New(
					fn.WithDeployer(deployer))
			}))

			// set namespace argument if given & reset after
			cmd.SetArgs([]string{}) // Do not use test command args
			viper.SetDefault("namespace", tt.namespace)
			viper.SetDefault("registry", tt.registry)
			defer viper.Reset()

			// set test case's func.yaml
			if err := os.WriteFile("func.yaml", []byte(tt.funcFile), os.ModePerm); err != nil {
				t.Fatal(err)
			}

			ctx := context.Background()

			_, err := cmd.ExecuteContextC(ctx)
			if err != nil {
				t.Fatalf("Got error '%s' but expected success", err)
			}

			fileFunction, err := fn.NewFunction(".")
			if err != nil {
				t.Fatalf("problem creating function: %v", err)
			}

			if fileFunction.Namespace != tt.expectNS {
				t.Fatalf("Expected namespace '%s' but function has '%s' namespace", tt.expectNS, fileFunction.Namespace)
			}
		})
	}
}

// TestDeploy_GitArgsPersist ensures that the git flags, if provided, are
// persisted to the Function for subsequent deployments.
func TestDeploy_GitArgsPersist(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()

	var (
		url    = "https://example.com/user/repo"
		branch = "main"
		dir    = "function"
	)

	// Create a new Function in the temp directory
	if err := fn.New().Create(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	// Deploy the Function specifying all of the git-related flags
	cmd := NewDeployCmd(NewClientFactory(func() *fn.Client {
		return fn.New(fn.WithPipelinesProvider(mock.NewPipelinesProvider()), fn.WithRegistry(TestRegistry))
	}))
	cmd.SetArgs([]string{"--remote", "--git-url=" + url, "--git-branch=" + branch, "--git-dir=" + dir, "."})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Load the Function and ensure the flags were stored.
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Git.URL != url {
		t.Errorf("expected git URL '%v' got '%v'", url, f.Git.URL)
	}
	if f.Git.Revision != branch {
		t.Errorf("expected git branch '%v' got '%v'", branch, f.Git.Revision)
	}
	if f.Git.ContextDir != dir {
		t.Errorf("expected git dir '%v' got '%v'", dir, f.Git.ContextDir)
	}
}

// TestDeploy_GitArgsUsed ensures that any git values provided as flags are used
// when invoking a remote deployment.
func TestDeploy_GitArgsUsed(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()

	var (
		url    = "https://example.com/user/repo"
		branch = "main"
		dir    = "function"
	)
	// Create a new Function in the temp dir
	if err := fn.New().Create(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	// A Pipelines Provider which will validate the expected values were received
	pipeliner := mock.NewPipelinesProvider()
	pipeliner.RunFn = func(f fn.Function) error {
		if f.Git.URL != url {
			t.Errorf("Pipeline Provider expected git URL '%v' got '%v'", url, f.Git.URL)
		}
		if f.Git.Revision != branch {
			t.Errorf("Pipeline Provider expected git branch '%v' got '%v'", branch, f.Git.Revision)
		}
		if f.Git.ContextDir != dir {
			t.Errorf("Pipeline Provider expected git dir '%v' got '%v'", url, f.Git.ContextDir)
		}
		return nil
	}

	// Deploy the Function specifying all of the git-related flags and --remote
	// such that the mock pipelines provider is invoked.
	cmd := NewDeployCmd(NewClientFactory(func() *fn.Client {
		return fn.New(fn.WithPipelinesProvider(pipeliner), fn.WithRegistry(TestRegistry))
	}))

	cmd.SetArgs([]string{"--remote=true", "--git-url=" + url, "--git-branch=" + branch, "--git-dir=" + dir})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

// TestDeploy_GitURLBranch ensures that a --git-url which specifies the branch
// in the URL is equivalent to providing --git-branch
func TestDeploy_GitURLBranch(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()

	if err := fn.New().Create(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	var (
		url            = "https://example.com/user/repo#branch"
		expectedUrl    = "https://example.com/user/repo"
		expectedBranch = "branch"
	)
	cmd := NewDeployCmd(NewClientFactory(func() *fn.Client {
		return fn.New(fn.WithPipelinesProvider(mock.NewPipelinesProvider()), fn.WithRegistry(TestRegistry))
	}))
	cmd.SetArgs([]string{"--remote", "--git-url=" + url})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Git.URL != expectedUrl {
		t.Errorf("expected git URL '%v' got '%v'", expectedUrl, f.Git.URL)
	}
	if f.Git.Revision != expectedBranch {
		t.Errorf("expected git branch '%v' got '%v'", expectedBranch, f.Git.Revision)
	}
}

// TODO: Test that an error is generated if --git-url includes a branch and
// --git-branch is provided.

// TODO:  --save
