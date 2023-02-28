package openshift

const (
	annotationOpenShiftVcsUri = "app.openshift.io/vcs-uri"
	annotationOpenShiftVcsRef = "app.openshift.io/vcs-ref"

	labelAppK8sInstance   = "app.kubernetes.io/instance"
	labelOpenShiftRuntime = "app.openshift.io/runtime"
)

var iconValuesForRuntimes = map[string]string{
	"go":         "golang",
	"node":       "nodejs",
	"python":     "python",
	"quarkus":    "quarkus",
	"springboot": "spring-boot",
}

type OpenshiftMetadataDecorator struct{}

func (o OpenshiftMetadataDecorator) UpdateGitAnnotations(url, ref string, annotations map[string]string) map[string]string {
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[annotationOpenShiftVcsUri] = url
	annotations[annotationOpenShiftVcsRef] = ref

	return annotations
}

func (o OpenshiftMetadataDecorator) UpdateLabels(name, runtime string, labels map[string]string) map[string]string {
	if labels == nil {
		labels = map[string]string{}
	}

	// this label is used for referencing a Tekton Pipeline and deployed KService
	labels[labelAppK8sInstance] = name

	// if supported, set the label representing a runtime icon in Developer Console
	iconValue, ok := iconValuesForRuntimes[runtime]
	if ok {
		labels[labelOpenShiftRuntime] = iconValue
	}

	return labels
}
