package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/ory/viper"
	"github.com/spf13/cobra"
	"knative.dev/client/pkg/util"

	fn "knative.dev/kn-plugin-func"
	"knative.dev/kn-plugin-func/buildpacks"
	"knative.dev/kn-plugin-func/docker"
	"knative.dev/kn-plugin-func/docker/creds"
	"knative.dev/kn-plugin-func/knative"
	"knative.dev/kn-plugin-func/s2i"
)

func NewDeployCmd(newClient ClientFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy a Function",
		Long: `
NAME
	{{.Name}} deploy - Deploy a Function

SYNOPSIS
	{{.Name}} deploy [-R|--remote] [-r|--registry] [-i|--image] [-n|--namespace]
	             [-e|env] [-g|--git-url] [-t|git-branch] [-d|--git-dir]
	             [-b|--build] [--builder] [--builder-image] [-p|--push]
	             [-c|--confirm] [-v|--verbose]

DESCRIPTION
	
	Deploys a function to the currently configured Knative-enabled cluster.

	By default the function in the current working directory is deployed, or at
	the path defined by --path.

	A function which was previously deployed will be updated when re-deployed.

	The function is built into a container for transport to the destination
	cluster by way of a registry.  Therefore --registry must be provided or have
	previously been configured for the function. This registry is also used to
	determine the final built image tag for the function.  This final image name
	can be provided explicitly using --image, in which case it is used in place
	of --registry.
	
	To run deploy using an interactive mode, use the --confirm (-c) option.
	This mode is useful for the first deployment in particular, since subsdequent
	deployments remember most of the settings provided.

	Building
	  By default the function will be built if it has not yet been built, or if
	  changes are detected in the function's source.  The --build flag can be
	  used to override this behavior and force building either on or off.

	Remote
	  Building and pushing (deploying) is by default run on localhost.  This
	  process can also be triggered to run remotely in a Tekton-enabled cluster.
	  The --remote flag indicates that a build and deploy pipeline should be
	  invoked in the remote.  Functions deployed in this manner must have their
	  source code kept in a git repository, and the URL to this source provided
	  via --git-url.  A specific branch can be specified with --git-branch.

EXAMPLES

	o Deploy the function using interactive prompts. This is useful for the first
	  deployment, since most settings will be remembered for future deployments.
	  $ {{.Name}} deploy -c

	o Deploy the function in the current working directory.
	  The function image will be pushed to "quay.io/alice/<Function Name>"
	  $ {{.Name}} deploy --registry quay.io/alice

	o Deploy the function in the current working directory, manually specifying
	  the final image name and target cluster namespace.
	  $ {{.Name}} deploy --image quay.io/alice/myfunc --namespace myns

	o Deploy the function, rebuilding the image even if no changes have been
	  detected in the local filesystem (source).
	  $ {{.Name}} deploy --build

	o Deploy without rebuilding, even if changes have been detected in the
	  local filesystem.
	  $ {{.Name}} deploy --build=false

	o Trigger a remote deploy, which instructs the cluster to build and deploy
	  the function in the specified git repository.
	  $ {{.Name}} deploy --remote --git-url=https://example.com/alice/myfunc.git
`,
		SuggestFor: []string{"delpoy", "deplyo"},
		PreRunE:    bindEnv("confirm", "env", "git-url", "git-branch", "git-dir", "remote", "build", "builder", "builder-image", "image", "registry", "push", "platform", "path", "namespace"),
	}

	cmd.Flags().BoolP("confirm", "c", false, "Prompt to confirm all configuration options (Env: $FUNC_CONFIRM)")
	cmd.Flags().StringArrayP("env", "e", []string{}, "Environment variable to set in the form NAME=VALUE. "+
		"You may provide this flag multiple times for setting multiple environment variables. "+
		"To unset, specify the environment variable name followed by a \"-\" (e.g., NAME-).")
	cmd.Flags().StringP("git-url", "g", "", "Repo url to push the code to be built (Env: $FUNC_GIT_URL)")
	cmd.Flags().StringP("git-branch", "t", "", "Git branch to be used for remote builds (Env: $FUNC_GIT_BRANCH)")
	cmd.Flags().StringP("git-dir", "d", "", "Directory in the repo where the function is located (Env: $FUNC_GIT_DIR)")
	cmd.Flags().BoolP("remote", "", false, "Trigger a remote deployment.  Default is to deploy and build from the local system: $FUNC_REMOTE)")

	// Flags shared with Build (specifically related to the build step):
	cmd.Flags().StringP("build", "b", "auto", "Build the function. [auto|true|false]. [Env: $FUNC_BUILD]")
	cmd.Flags().Lookup("build").NoOptDefVal = "true" // --build is equivalient to --build=true
	cmd.Flags().StringP("builder", "", DefaultBuilder, "build strategy to use when creating the underlying image. Currently supported build strategies are 'pack' and 's2i'. [Env: $FUNC_BUILDER]")
	cmd.Flags().StringP("builder-image", "", "", "Builder image, either an as a an image name or a mapping name.\nSpecified value is stored in func.yaml (as 'builder' field) for subsequent builds. ($FUNC_BUILDER_IMAGE)")
	cmd.Flags().StringP("image", "i", "", "Full image name in the form [registry]/[namespace]/[name]:[tag]@[digest]. This option takes precedence over --registry. Specifying digest is optional, but if it is given, 'build' and 'push' phases are disabled. (Env: $FUNC_IMAGE)")
	cmd.Flags().StringP("registry", "r", GetDefaultRegistry(), "Registry + namespace part of the image to build, ex 'quay.io/myuser'.  The full image name is automatically determined based on the local directory name. If not provided the registry will be taken from func.yaml (Env: $FUNC_REGISTRY)")
	cmd.Flags().BoolP("push", "u", true, "Push the function image to registry before deploying (Env: $FUNC_PUSH)")
	cmd.Flags().StringP("platform", "", "", "Target platform to build (e.g. linux/amd64).")
	cmd.Flags().StringP("namespace", "n", "", "Deploy into a specific namespace. (Env: $FUNC_NAMESPACE)")
	setPathFlag(cmd)

	if err := cmd.RegisterFlagCompletionFunc("build", CompleteBuildList); err != nil {
		fmt.Println("internal: error while calling RegisterFlagCompletionFunc: ", err)
	}

	if err := cmd.RegisterFlagCompletionFunc("builder", CompleteBuilderList); err != nil {
		fmt.Println("internal: error while calling RegisterFlagCompletionFunc: ", err)
	}

	if err := cmd.RegisterFlagCompletionFunc("builder-image", CompleteBuilderImageList); err != nil {
		fmt.Println("internal: error while calling RegisterFlagCompletionFunc: ", err)
	}

	cmd.SetHelpFunc(defaultTemplatedHelp)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runDeploy(cmd, args, newClient)
	}

	return cmd
}

// runDeploy gathers configuration from environment, flags and the user,
// merges these into the function requested, and triggers either a remote or
// local build-and-deploy.
func runDeploy(cmd *cobra.Command, _ []string, newClient ClientFactory) (err error) {
	// Create a deploy config from environment variables and flags
	config, err := newDeployConfig(cmd)
	if err != nil {
		return
	}

	// Prompt the user to potentially change config interactively.
	config, err = config.Prompt()
	if err != nil {
		if err == terminal.InterruptErr {
			return nil
		}
		return
	}

	//if --image contains '@', validate image digest and disable build and push if not set, otherwise return an error
	imageSplit := strings.Split(config.Image, "@")
	imageDigestProvided := false

	if len(imageSplit) == 2 {
		if config, err = parseImageDigest(imageSplit, config, cmd); err != nil {
			return
		}
		imageDigestProvided = true
	}

	// Load the function, and if it exists (path initialized as a function), merge
	// in any updates from flags/env vars (namespace, explicit image name, envs).
	f, err := fn.NewFunction(config.Path)
	if err != nil {
		return
	}
	if !f.Initialized() {
		return fmt.Errorf("the given path '%v' does not contain an initialized function.", config.Path)
	}
	if config.Registry != "" {
		f.Registry = config.Registry
	}
	if config.Image != "" {
		f.Image = config.Image
	}
	if config.Builder != "" {
		f.Builder = config.Builder
	}
	f.Namespace = checkNamespaceDeploy(f, config, cmd.ErrOrStderr())
	if err != nil {
		return
	}
	f.Envs, _, err = mergeEnvs(f.Envs, config.EnvToUpdate, config.EnvToRemove)
	if err != nil {
		return
	}
	if imageDigestProvided {
		// TODO(lkingland):  This could instead be part of the config, relying on
		// zero values rather than a flag indicating "Image Digest was Provided"
		f.ImageDigest = imageSplit[1] // save image digest if provided in --image
	}

	// Validate that a builder short-name was obtained, whether that be from
	// the funciton's prior state, or the value of flags/environment.
	if err = ValidateBuilder(f.Builder); err != nil {
		return
	}

	// Choose a builder based on the value of the --builder flag and a possible
	// override for the build image for that builder to use from the optional
	// builder-image flag.
	var builder fn.Builder
	if config.Builder == "pack" {
		if config.Platform != "" {
			err = errors.New("the --platform flag works only with s2i builds")
			return
		}
		builder = buildpacks.NewBuilder(buildpacks.WithVerbose(config.Verbose))
	} else if config.Builder == "s2i" {
		builder = s2i.NewBuilder(s2i.WithVerbose(config.Verbose), s2i.WithPlatform(config.Platform))
	}
	// Note ValidateBuilder enforces validity of f.Builder

	// Use the user-provided builder image, if supplied
	if config.BuilderImage != "" {
		f.BuilderImages[config.Builder] = config.BuilderImage
	}
	client, done := newClient(ClientConfig{Namespace: f.Namespace, Verbose: config.Verbose},
		fn.WithRegistry(config.Registry),
		fn.WithBuilder(builder))
	defer done()

	// Default Client Registry, Function Registry or explicit Image required
	//
	// An attempt to build without either an image or registry
	// will fail with an error.  For interactive prompting, use the -c flag.
	// Since the error message generated by the client library in this case is a
	// bit terse, let's check here and offer the user a very descriptive message
	// which includes cli-specific recommendations
	if client.Registry() == "" && f.Registry == "" && f.Image == "" {
		return ErrRegistryRequired
	}

	// NOTE: curently need to preemptively write out function state until
	// the API is updated to use instances.
	//
	// Discussion:  The need for this is proof that the Client API should work on
	// function instances as opposed to paths.  In order for the following Build
	// call to complete using flag values from above, the function hast to be
	// first serialized out to disk.  This interferes with the ability to have a
	// --save option which only writes mutated values to disk on a successful
	// completion.. i.e. we have to save to communicate the Functin state to the
	// builder, potentially prematurely.  Therefore, in the forthcoming PR which
	// persists all flags to func.yaml by default (option to --save=false), the
	// Client API will need to change to accepting function instances rather than
	// paths on disk.  This has other knock-on benefits as well.
	if err = f.Write(); err != nil {
		return
	}

	// Perform the deployment either remote or local.
	// TODO: Extract
	if config.Remote {
		// Remote
		// ------
		// If a remote deploy was requested, trigger the remote to deploy (and build
		// by default) using the configured git repository URL

		// Populate f.Git from config
		// TODO: extract
		if config.GitURL != "" {
			if strings.Contains(config.GitURL, "#") {
				parts := strings.Split(config.GitURL, "#")
				if len(parts) == 2 {
					f.Git.URL = parts[0]
					f.Git.Revision = parts[1]
				} else {
					return fmt.Errorf("invalid --git-url '%v'", config.GitURL)
				}
			} else {
				f.Git.URL = config.GitURL
			}
		}
		if config.GitBranch != "" {
			f.Git.Revision = config.GitBranch
		}
		if config.GitDir != "" {
			f.Git.ContextDir = config.GitDir
		}

		// Validate the Function contains a URL.
		// TODO: This should be a check performed by the Pipeline Runner as well,
		// but by checking here, we can provide a verbose error message with cli-
		// specific recommendations.  We could refactor such that the Pipeline
		// runner returns a typed error and wrap here with the detailed message.
		if f.Git.URL == "" {
			return ErrURLRequired
		}

		// Invoke a remote build/push/deploy pipeline
		if err = client.RunPipeline(cmd.Context(), f); err != nil {
			return err
		}

	} else {
		// Local
		// -----
		// build unbuilt, filesystem changed since last build or --build forced
		// after validating no --git-x flags.
		if config.GitURL != "" || config.GitDir != "" || config.GitBranch != "" {
			return fmt.Errorf("Git settings (--git-url --git-dir and --git-branch) are currently only available when triggering remote deployments using --remote.")
		}
		if config.Build == "auto" {
			if !client.Built(f.Root) {
				if err = client.Build(cmd.Context(), config.Path); err != nil {
					return
				}
			} else {
				fmt.Println("function already built.  Use --build to force a rebuild.")
			}
		} else {
			var build bool
			if build, err = strconv.ParseBool(config.Build); err != nil {
				return fmt.Errorf("unrecognized value for --build '%v'.  accepts 'auto', 'true' or 'false' (or similarly truthy value)", build)
			}
			if build {
				if err = client.Build(cmd.Context(), config.Path); err != nil {
					return
				}
			} else {
				fmt.Println("function build disabled.")
			}
		}
		// Push built image for the function at path to registry
		if config.Push {
			if err = client.Push(cmd.Context(), config.Path); err != nil {
				return
			}
		}
		// Deploy pushed image for function at path to current platform
		if err = client.Deploy(cmd.Context(), config.Path); err != nil {
			return
		}
	}

	// Config has been gathered from the environment, from the user and merged
	// into the in-memory function.  It has potentially also been built, and
	// the remote or local deploy succeeded with those settings.  All of
	// which result in a function object which is now out of sync with its
	// on-disk representation.
	return f.Write()
}

func newPromptForCredentials(in io.Reader, out, errOut io.Writer) func(registry string) (docker.Credentials, error) {
	firstTime := true
	return func(registry string) (docker.Credentials, error) {
		var result docker.Credentials

		if firstTime {
			firstTime = false
			fmt.Fprintf(out, "Please provide credentials for image registry (%s).\n", registry)
		} else {
			fmt.Fprintln(out, "Incorrect credentials, please try again.")
		}

		var qs = []*survey.Question{
			{
				Name: "username",
				Prompt: &survey.Input{
					Message: "Username:",
				},
				Validate: survey.Required,
			},
			{
				Name: "password",
				Prompt: &survey.Password{
					Message: "Password:",
				},
				Validate: survey.Required,
			},
		}

		var (
			fr terminal.FileReader
			ok bool
		)

		isTerm := false
		if fr, ok = in.(terminal.FileReader); ok {
			isTerm = term.IsTerminal(int(fr.Fd()))
		}

		if isTerm {
			err := survey.Ask(qs, &result, survey.WithStdio(fr, out.(terminal.FileWriter), errOut))
			if err != nil {
				return docker.Credentials{}, err
			}
		} else {
			reader := bufio.NewReader(in)

			fmt.Fprintf(out, "Username: ")
			u, err := reader.ReadString('\n')
			if err != nil {
				return docker.Credentials{}, err
			}
			u = strings.Trim(u, "\r\n")

			fmt.Fprintf(out, "Password: ")
			p, err := reader.ReadString('\n')
			if err != nil {
				return docker.Credentials{}, err
			}
			p = strings.Trim(p, "\r\n")

			result = docker.Credentials{Username: u, Password: p}
		}

		return result, nil
	}
}

func newPromptForCredentialStore() creds.ChooseCredentialHelperCallback {
	return func(availableHelpers []string) (string, error) {
		if len(availableHelpers) < 1 {
			fmt.Fprintf(os.Stderr, `Credentials will not be saved.
If you would like to save your credentials in the future,
you can install docker credential helper https://github.com/docker/docker-credential-helpers.
`)
			return "", nil
		}

		isTerm := term.IsTerminal(int(os.Stdin.Fd()))

		var resp string

		if isTerm {
			err := survey.AskOne(&survey.Select{
				Message: "Choose credentials helper",
				Options: append(availableHelpers, "None"),
			}, &resp, survey.WithValidator(survey.Required))
			if err != nil {
				return "", err
			}
			if resp == "None" {
				fmt.Fprintf(os.Stderr, "No helper selected. Credentials will not be saved.\n")
				return "", nil
			}
		} else {
			fmt.Fprintf(os.Stderr, "Available credential helpers:\n")
			for _, helper := range availableHelpers {
				fmt.Fprintf(os.Stderr, "%s\n", helper)
			}
			fmt.Fprintf(os.Stderr, "Choose credentials helper: ")

			reader := bufio.NewReader(os.Stdin)

			var err error
			resp, err = reader.ReadString('\n')
			if err != nil {
				return "", err
			}
			resp = strings.Trim(resp, "\r\n")
			if resp == "" {
				fmt.Fprintf(os.Stderr, "No helper selected. Credentials will not be saved.\n")
			}
		}

		return resp, nil
	}
}

type deployConfig struct {
	buildConfig

	// Perform build using the settings from the embedded buildConfig struct.
	// Acceptable values are the keyword 'auto', or a truthy value such as
	// 'true', 'false, '1' or '0'.
	Build string

	// Remote indicates the deployment (and possibly build) process are to
	// be triggered in a remote environment rather than run locally.
	Remote bool

	// Namespace override for the deployed function.  If provided, the
	// underlying platform will be instructed to deploy the function to the given
	// namespace (if such a setting is applicable; such as for Kubernetes
	// clusters).  If not provided, the currently configured namespace will be
	// used.  For instance, that which would be used by default by `kubectl`
	// (~/.kube/config) in the case of Kubernetes.
	Namespace string

	// Path of the function implementation on local disk. Defaults to current
	// working directory of the process.
	Path string

	// Verbose logging.
	Verbose bool

	// Confirm: confirm values arrived upon from environment plus flags plus defaults,
	// with interactive prompting (only applicable when attached to a TTY).
	Confirm bool

	// Push function image to the registry before deploying.
	Push bool

	// Envs passed via cmd to be added/updated
	EnvToUpdate *util.OrderedMap

	// Envs passed via cmd to removed
	EnvToRemove []string

	// Git repo url for remote builds
	GitURL string

	// Git branch for remote builds
	GitBranch string

	// Directory in the git repo where the function is located
	GitDir string
}

// newDeployConfig creates a buildConfig populated from command flags and
// environment variables; in that precedence.
func newDeployConfig(cmd *cobra.Command) (deployConfig, error) {
	envToUpdate, envToRemove, err := envFromCmd(cmd)
	if err != nil {
		return deployConfig{}, err
	}

	return deployConfig{
		buildConfig: newBuildConfig(),
		Build:       viper.GetString("build"),
		Remote:      viper.GetBool("remote"),
		Namespace:   viper.GetString("namespace"),
		Path:        viper.GetString("path"),
		Verbose:     viper.GetBool("verbose"), // defined on root
		Confirm:     viper.GetBool("confirm"),
		Push:        viper.GetBool("push"),
		EnvToUpdate: envToUpdate,
		EnvToRemove: envToRemove,
		GitURL:      viper.GetString("git-url"),
		GitBranch:   viper.GetString("git-branch"),
		GitDir:      viper.GetString("git-dir"),
	}, nil
}

// Prompt the user with value of config members, allowing for interaractive changes.
// Skipped if not in an interactive terminal (non-TTY), or if --yes (agree to
// all prompts) was explicitly set.
func (c deployConfig) Prompt() (deployConfig, error) {
	if !interactiveTerminal() || !c.Confirm {
		return c, nil
	}

	var qs = []*survey.Question{
		{
			Name: "registry",
			Prompt: &survey.Input{
				Message: "Registry for function images:",
				Default: c.buildConfig.Registry,
			},
			Validate: survey.Required,
		},
		{
			Name: "namespace",
			Prompt: &survey.Input{
				Message: "Destination namespace:",
				Default: c.Namespace,
			},
		},
		{
			Name: "path",
			Prompt: &survey.Input{
				Message: "Function source path:",
				Default: c.Path,
			},
			Validate: survey.Required,
		},
	}
	answers := struct {
		Registry  string
		Namespace string
		Path      string
	}{}
	err := survey.Ask(qs, &answers)
	if err != nil {
		return deployConfig{}, err
	}

	dc := deployConfig{
		buildConfig: buildConfig{
			Registry: answers.Registry,
		},
		Namespace: answers.Namespace,
		Path:      answers.Path,
		Verbose:   c.Verbose,
	}

	return dc, nil
}

func parseImageDigest(imageSplit []string, config deployConfig, cmd *cobra.Command) (deployConfig, error) {

	if !strings.HasPrefix(imageSplit[1], "sha256:") {
		return config, fmt.Errorf("value '%s' in --image has invalid prefix syntax for digest (should be 'sha256:')", config.Image)
	}

	if len(imageSplit[1][7:]) != 64 {
		return config, fmt.Errorf("sha256 hash in '%s' from --image has the wrong length (%d), should be 64", imageSplit[1], len(imageSplit[1][7:]))
	}

	// if the --push flag was set by a user to 'true', return an error
	if cmd.Flags().Changed("push") && config.Push {
		return config, fmt.Errorf("the --push flag '%v' is not valid when using --image with digest", config.Push)
	}

	fmt.Printf("Deploying existing image with digest %s. Build and push are disabled.\n", imageSplit[1])

	config.Push = false
	config.Image = imageSplit[0]

	return config, nil
}

// checkNamespaceDeploy returns the namespace that should be
// effective given the current value and a given requested.
func checkNamespaceDeploy(f fn.Function, config deployConfig, out io.Writer) string {
	// --namespace provided
	if config.Namespace != "" {
		// If updating a function which is alredy presumably deployed (has the
		// namespace member populated), that if we are requesting it be deployed
		// to a different namespace, this results in an warning because it may
		// result in an orphaned instance in another namespace.
		if f.Namespace != "" && config.Namespace != f.Namespace {
			fmt.Fprintf(out, "Warning: Function is in namespace '%v', but requested namespace is '%v'. Continuing with deployment to '%v'.\n", f.Namespace, config.Namespace, config.Namespace)
		}
		return config.Namespace
	}

	// Default when no --namespace provided and function undeployed
	if f.Namespace == "" && knative.DefaultNamespace() != "" {
		// printing info only if default returned is nonempty
		fmt.Fprintf(out, "Using default namespace %v\n", knative.DefaultNamespace())
		return knative.DefaultNamespace()
	}

	// Warn if the function does include a destination namespace already, and
	// this namespace differs from the user's current namespace (the k8s
	// default namespace), this also results in a warning. becaus it ma appear
	// to the user nothing happened.
	if f.Namespace != "" && f.Namespace != knative.DefaultNamespace() {
		fmt.Fprintf(out, "Warning: Function is in namespace '%v', but currently active namespace is '%v'. Continuing with redeployment to '%v'.\n", f.Namespace, knative.DefaultNamespace(), f.Namespace)

	}
	return f.Namespace // None of the aboive conditions apply.  Leave it unchanged.
}

var ErrRegistryRequired = errors.New(`A container registry is required.  For example:

--registry docker.io/myusername

For more advanced usage, it is also possible to specify the exact image to use. For example:

--image docker.io/myusername/myfunc:latest

To run the deploy command in an interactive mode, use --confirm (-c)`)

var ErrURLRequired = errors.New(`The function is not associated with a Git repository, and needs one in order to perform a remote deployment.  For example:

--git-url = https://git.example.com/namespace/myFunction

To run the deploy command in an interactive mode, use --confirm (-c)`)
