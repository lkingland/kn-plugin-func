module knative.dev/kn-plugin-func

go 1.16

require (
	github.com/alecthomas/jsonschema v0.0.0-20180308105923-f2c93856175a
	github.com/go-git/go-billy/v5 v5.3.1
	github.com/go-git/go-git/v5 v5.4.2
	github.com/markbates/pkger v0.17.1
	github.com/mitchellh/go-homedir v1.1.0
	github.com/ory/viper v1.7.5
	github.com/spf13/cobra v1.2.1
	golang.org/x/term v0.0.0-20210503060354-a79de5458b56 // indirect
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.21.4
	k8s.io/apimachinery v0.21.4
	k8s.io/client-go v0.21.4
	knative.dev/client v0.25.1-0.20210913155632-82a21a5773be
	knative.dev/eventing v0.25.1-0.20210909163359-316e14d7fbc2
	knative.dev/pkg v0.0.0-20210909165259-d4505c660535
	knative.dev/serving v0.25.1-0.20210913112533-33aeffc6c9e2
	sigs.k8s.io/kustomize/api v0.9.0 // indirect
)
