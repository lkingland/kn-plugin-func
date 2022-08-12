package cmd

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

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

// Test_ImageWithDigestErrors ensures that when an image to use is explicitly
// provided via content addressing (digest), nonsensical combinations
// of other flags (such as forcing a build or pushing being enabled), yield
// informative errors.
func Test_ImageWithDigestErrors(t *testing.T) {
	tests := []struct {
		name      string // name of the test
		image     string // value to provide as --image
		build     string // If provided, the value of the build flag
		push      bool   // if true, explicitly set argument --push=true
		errString string // the string value of an expected error
	}{
		{
			name:  "correctly formatted full image with digest yields no error (degen case)",
			image: "example.com/myNamespace/myFunction:latest@sha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4e",
		},
		{
			name:      "--build forced on yields error",
			image:     "example.com/myNamespace/myFunction:latest@sha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4e",
			build:     "true",
			errString: "when an image digest is provided, --build can not also be enabled",
		},
		{
			name:      "push flag explicitly set with digest should error",
			image:     "example.com/myNamespace/myFunction:latest@sha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4e",
			push:      true,
			errString: "the --push flag 'true' is not valid when using --image with digest",
		},
		{
			name:      "invalid digest prefix 'Xsha256', expect error",
			image:     "example.com/myNamespace/myFunction:latest@Xsha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4e",
			errString: "value 'example.com/myNamespace/myFunction:latest@Xsha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4e' in --image has invalid prefix syntax for digest (should be 'sha256:')",
		},
		{
			name:      "invalid sha hash length(added X at the end), expect error",
			image:     "example.com/myNamespace/myFunction:latest@sha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4eX",
			errString: "sha256 hash in 'sha256:7d66645b0add6de7af77ef332ecd4728649a2f03b9a2716422a054805b595c4eX' from --image has the wrong length (65), should be 64",
		},
	}

	defer WithEnvVar(t, "KUBECONFIG", fmt.Sprintf("%s/testdata/kubeconfig_deploy_namespace", cwd()))()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Move into a new temp directory
			root, rm := Mktemp(t)
			defer rm()

			// Create a new Function in the temp directory
			if err := fn.New().Create(fn.Function{Runtime: "go", Root: root}); err != nil {
				t.Fatal(err)
			}

			// Deploy it using the various combinations of flags from the test
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
			args := []string{fmt.Sprintf("--image=%s", tt.image)}
			if tt.build != "" {
				args = append(args, fmt.Sprintf("--build=%s", tt.build))
			}
			if tt.push {
				args = append(args, "--push=true")
			}

			cmd.SetArgs(args)
			err := cmd.Execute()
			if err != nil {
				if tt.errString == "" {
					t.Fatal(err) // no error was expected.  fail
				}
				if tt.errString != err.Error() {
					t.Fatalf("expected error '%v' not received. got '%v'", tt.errString, err.Error())
				}
				// There was an error, but it was expected
			}
		})
	}
}

// TestDeploy_BuilderPersistence ensures that the builder chosen is read from
// the function by default, and is able to be overridden by flags/env vars.
func TestDeploy_BuilderPersistence(t *testing.T) {
	testBuilderPersistence(t, "docker.io/tigerteam", NewDeployCmd)
}

/*
Test_namespaceCheck cases were refactored into:
			"first deployment(no ns in func.yaml), not given via cli, expect write in func.yaml"
			AKA: Undeployed Function, deploying with no ns specified: use defaults
			See TestDeploy_NamespaceDefaults

			"ns in func.yaml, not given via cli, current ns matches func.yaml",
			AKA: Function Deployed, should redeploy to the same namespace.
			See TestDeploy_NamespaceRedeployWarning

			"ns in func.yaml, given via cli (always override)",
			AKA: Function Deployed, but should deploy wherever --namespace says
			See TestDeploy_NamespaceUpdateWarning which confirms this case exists
			  and yields a warning message.

			"ns in func.yaml, not given via cli, current ns does NOT match func.yaml",
			AKA: A previously deployed function should stay in its namespace, even
			  when the user's active namespace differs.
			See TestDeploy_NamespaceRedeployWarning which confirms this case exists
			  and yields a warning message.


*/

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
		return fn.New(
			fn.WithDeployer(mock.NewDeployer()),
			fn.WithBuilder(mock.NewBuilder()),
			fn.WithPipelinesProvider(mock.NewPipelinesProvider()),
			fn.WithRegistry(TestRegistry))
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

// TestDeploy_NamespaceDefaults ensures that when not specified, a users's
// active kubernetes context is used for the namespace if available.
func TestDeploy_NamespaceDefaults(t *testing.T) {
	// Set kube context to test context
	defer WithEnvVar(t, "KUBECONFIG", filepath.Join(cwd(), "testdata", "kubeconfig_deploy_namespace"))()

	// from a temp directory
	root, rm := Mktemp(t)
	defer rm()

	// Create a new function
	if err := fn.New().Create(fn.Function{Runtime: "go", Root: root}); err != nil {
		t.Fatal(err)
	}

	// Assert it has no default namespace set
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatalf("newly created functions should not have a namespace set until deployed.  Got '%v'", f.Namespace)
	}

	// New deploy command that will not actually deploy or build (mocked)
	cmd := NewDeployCmd(NewClientFactory(func() *fn.Client {
		return fn.New(
			fn.WithDeployer(mock.NewDeployer()),
			fn.WithBuilder(mock.NewBuilder()),
			fn.WithPipelinesProvider(mock.NewPipelinesProvider()),
			fn.WithRegistry(TestRegistry))
	}))
	cmd.SetArgs([]string{})

	// Execute, capturing stderr
	stderr := strings.Builder{}
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Assert the function has been updated to be in namespace from the profile
	f, err = fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Namespace != "test-ns-deploy" { // from testdata/kubeconfig_deploy_namespace
		t.Fatalf("expected function to have active namespace 'test-ns-deploy' by default.  got '%v'", f.Namespace)
	}
	// See the knative package's tests for an example of tests which require
	// the knative or kubernetes API dependency.
}

// TestDeploy_NamespaceUpdateWarning ensures that, deploying a Function
// to a new namespace issues a warning.
// Also implicitly checks that the --namespace flag takes precidence over
// the namespace of a previously deployed Function.
func TestDeploy_NamespaceUpdateWarning(t *testing.T) {
	root, rm := Mktemp(t)
	defer rm()

	// Create a Function which appears to have been deployed to 'myns'
	f := fn.Function{
		Runtime:   "go",
		Root:      root,
		Namespace: "myns",
	}
	if err := fn.New().Create(f); err != nil {
		t.Fatal(err)
	}

	// Redeploy the function, specifying 'newns'
	cmd := NewDeployCmd(NewClientFactory(func() *fn.Client {
		return fn.New(
			fn.WithDeployer(mock.NewDeployer()),
			fn.WithBuilder(mock.NewBuilder()),
			fn.WithPipelinesProvider(mock.NewPipelinesProvider()),
			fn.WithRegistry(TestRegistry))
	}))
	cmd.SetArgs([]string{"--namespace=newns"})
	stderr := strings.Builder{}
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	expected := "Warning: Function is in namespace 'myns', but requested namespace is 'newns'. Continuing with deployment to 'newns'."

	// Ensure output contained warning if changing namespace
	if !strings.Contains(stderr.String(), expected) {
		t.Log("STDERR:\n" + stderr.String())
		t.Fatalf("Expected warning not found:\n%v", expected)
	}

	// Ensure the function was saved as having been deployed to
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Namespace != "newns" {
		t.Fatalf("expected function to be deoployed into namespace 'newns'.  got '%v'", f.Namespace)
	}

}

// TestDeploy_NamespaceRedeployWarning ensures that redeploying a function
// which is in a namespace other than the active namespace prints a warning.
// Implicitly checks that redeploying a previously deployed function
// results in the function being redeployed to its previous namespace if
// not instructed otherwise.
func TestDeploy_NamespaceRedeployWarning(t *testing.T) {
	// Change profile to one whose current profile is 'test-ns-deploy'
	defer WithEnvVar(t, "KUBECONFIG", filepath.Join(cwd(), "testdata", "kubeconfig_deploy_namespace"))()

	// From within a temp directory
	root, rm := Mktemp(t)
	defer rm()

	// Create a Function which appears to have been deployed to 'myns'
	f := fn.Function{
		Runtime:   "go",
		Root:      root,
		Namespace: "myns",
	}
	if err := fn.New().Create(f); err != nil {
		t.Fatal(err)
	}

	// Redeploy the function without specifying namespace.
	cmd := NewDeployCmd(NewClientFactory(func() *fn.Client {
		return fn.New(
			fn.WithDeployer(mock.NewDeployer()),
			fn.WithBuilder(mock.NewBuilder()),
			fn.WithPipelinesProvider(mock.NewPipelinesProvider()),
			fn.WithRegistry(TestRegistry))
	}))
	cmd.SetArgs([]string{})
	stderr := strings.Builder{}
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	expected := "Warning: Function is in namespace 'myns', but currently active namespace is 'test-ns-deploy'. Continuing with redeployment to 'myns'."

	// Ensure output contained warning if changing namespace
	if !strings.Contains(stderr.String(), expected) {
		t.Log("STDERR:\n" + stderr.String())
		t.Fatalf("Expected warning not found:\n%v", expected)
	}

	// Ensure the function was saved as having been deployed to
	f, err := fn.NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.Namespace != "myns" {
		t.Fatalf("expected function to be updated with namespace 'myns'.  got '%v'", f.Namespace)
	}
}
