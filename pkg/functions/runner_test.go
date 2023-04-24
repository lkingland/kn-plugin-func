package functions_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/oci"
	. "knative.dev/func/pkg/testing"
)

// TestRunner ensures that the default internal runner correctly executes
// a scaffolded function.
func TestRunner(t *testing.T) {
	// This integration test explicitly requires the "host" builder due to its
	// lack of a dependency on a container runtime, and the other builders not
	// taking advantage of Scaffolding (expected by this runner).
	// See E2E tests for testing of running functions built using Pack or S2I and
	// which are dependent on Podman or Docker.

	// TODO: this test likely supercedes TestClient_Run which simply uses a mock.

	root, cleanup := Mktemp(t)
	defer cleanup()

	ctx := context.Background()

	client := fn.New(fn.WithBuilder(oci.NewBuilder("", true)), fn.WithVerbose(true))
	f, err := client.Init(fn.Function{Root: root, Runtime: "go", Registry: TestRegistry})
	if f, err = client.Build(ctx, f); err != nil {
		t.Fatal(err)
	}
	job, err := client.Run(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := job.Stop(); err != nil {
			t.Fatalf("error on job stop: %v", err)
		}
	}()

	resp, err := http.Get(fmt.Sprintf("http://localhost:%s", job.Port))
	if err != nil {
		t.Fatal(err)
	}
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
	}
	defer resp.Body.Close()

	t.Logf("RUN received: %s", bodyBytes)

}
