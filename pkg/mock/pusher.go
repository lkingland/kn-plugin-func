package mock

import (
	"context"

	fn "knative.dev/func/pkg/functions"
)

const TestDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

type Pusher struct {
	PushInvoked bool
	PushFn      func(fn.Function) (string, error)
}

func NewPusher() *Pusher {
	return &Pusher{
		PushFn: func(fn.Function) (string, error) { return TestDigest, nil },
	}
}

func (i *Pusher) Push(ctx context.Context, f fn.Function) (string, error) {
	i.PushInvoked = true
	return i.PushFn(f)
}
