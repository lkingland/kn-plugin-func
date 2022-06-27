package cmd

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/ory/viper"
	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/mock"
	. "knative.dev/kn-plugin-func/testing"
)

// TestDeploy_RemoteBuildURLPermutations ensures that the remote, build and git-url flags
// are properly respected.
func TestDeploy_RemoteBuildURLPermutations(t *testing.T) {
	// Valid flag permutations (empty indicates flag should be omitted)
	// and a functon which will convert a permutation into flags for use
	// by the subtests.
	var (
		remoteValues = []string{"", "true", "false"}
		buildValues  = []string{"", "true", "false", "auto"}
		urlValues    = []string{"", "https://example.com/user/repo"}

		toArgs = func(remote string, build string, url string) []string {
			args := []string{}
			if remote != "" {
				args = append(args, fmt.Sprintf("--remote=%v", remote))
			}
			if build != "" {
				args = append(args, fmt.Sprintf("--build=%v", build))
			}
			if url != "" {
				args = append(args, fmt.Sprintf("--git-url=%v", build))
			}
			return args
		}
	)

	// returns a single test function for one possible permutation of the flags.
	newTestFn := func(remote string, build string, url string) func(t *testing.T) {
		return func(t *testing.T) {
			root, rm := Mktemp(t)
			defer rm()

			// Create a new Funciton in the temp directory
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
					return fn.New(fn.WithDeployer(deployer), fn.WithBuilder(builder), fn.WithPipelinesProvider(pipeliner))
				}))
			)
			cmd.SetArgs(toArgs(remote, build, url))
			err := cmd.Execute()

			// Assertions

			// LOCAL/REMOTE?
			if remote == "" || remote == "false" { // default is remote false.
				if !deployer.DeployInvoked {
					t.Error("local deployer not invoked")
				}
				if pipeliner.RunInvoked {
					t.Error("remote was invoked")
				}
			} else {
				// Remote was enabled.
				if !pipeliner.RunInvoked {
					t.Error("remote was not invoked")
				}
				if deployer.DeployInvoked {
					t.Error("local deployer was invoked")
				}
			}

			// BUILD?
			if build == "" || build == "true" || build == "auto" {
				// The default case for build is auto, which is equivalent to true
				// for a newly created Function which has not yet been built.
				if !builder.BuildInvoked && remote != "true" {
					t.Error("local builder not invoked")
				}

				// TODO: (enhancement) Check if the remote was invoked with building
				// enabled (forced or auto) when --remote.
			} else {
				// Build was explicitly disabled.
				if builder.BuildInvoked {
					t.Error("local builder was invoked")
				}

				// TODO: (enhancement) Check that the remote was invoked with building
				// expressly disabled (a redeploy).
			}

			// GIT OR TARBALL?
			if url == "" {
				// TODO: (enhancement) Check that the remote is invoked with a directive
				// to send a tarball of the local filesystem when remote is enabled but
				// no git URL was provided insted of the current test which:

				// Check an error is generated when attempting to run a remote build
				// without providing this value.
				if err == nil {
					t.Fatal("error expected when --remote without a --git-url")
				}

			} else {
				// TODO: (enhancement) Check that the local builder is invoked with a
				// directive to use a git repo rather than the local filesystem if
				// building is enabled.
			}
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	// Run all permutations
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
		return fn.New(fn.WithPipelinesProvider(mock.NewPipelinesProvider()))
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
		return fn.New(fn.WithPipelinesProvider(pipeliner))
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
		return fn.New(fn.WithPipelinesProvider(mock.NewPipelinesProvider()))
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
