package knative

import (
	"context"

	clientservingv1 "knative.dev/client/pkg/serving/v1"

	"knative.dev/kn-plugin-func/k8s"
)

const (
	labelKey   = "boson.dev/function"
	labelValue = "true"
)

type Lister struct {
	Verbose   bool
	Namespace string
}

func NewLister(namespaceOverride string) (l *Lister, err error) {
	l = &Lister{}

	namespace, err := k8s.GetNamespace(namespaceOverride)
	if err != nil {
		return
	}
	l.Namespace = namespace

	return
}

func (l *Lister) List(ctx context.Context) (items []string, err error) {

	client, err := NewServingClient(l.Namespace)
	if err != nil {
		return
	}

	lst, err := client.ListServices(ctx, clientservingv1.WithLabel(labelKey, labelValue))
	if err != nil {
		return
	}

	for _, service := range lst.Items {

		items = append(items, service.Name)
	}
	return
}
