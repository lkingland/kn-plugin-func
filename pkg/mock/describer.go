package mock

import (
	"context"

	fn "knative.dev/func/pkg/functions"
)

type Describer struct {
	DescribeInvoked bool
	DescribeFn      func(string) (fn.InstanceRef, error)
}

func NewDescriber() *Describer {
	return &Describer{
		DescribeFn: func(string) (fn.InstanceRef, error) { return fn.InstanceRef{}, nil },
	}
}

func (l *Describer) Describe(_ context.Context, name string) (fn.InstanceRef, error) {
	l.DescribeInvoked = true
	return l.DescribeFn(name)
}
