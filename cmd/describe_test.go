package cmd

import (
	"testing"

	fn "knative.dev/func/pkg/functions"
	"knative.dev/func/pkg/mock"
)

// TestDescribe_ByName ensures that describing a function by name invokes
// the describer appropriately.
func TestDescribe_ByName(t *testing.T) {
	var (
		testname  = "testname"
		describer = mock.NewDescriber()
	)

	describer.DescribeFn = func(n string) (fn.InstanceRef, error) {
		if n != testname {
			t.Fatalf("expected describe name '%v', got '%v'", testname, n)
		}
		return fn.InstanceRef{}, nil
	}

	cmd := NewDescribeCmd(NewTestClient(fn.WithDescriber(describer)))
	cmd.SetArgs([]string{testname})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if !describer.DescribeInvoked {
		t.Fatal("Describer not invoked")
	}
}

// TestDescribe_ByProject ensures that describing the currently active project
// (func created in the current working directory) invokes the describer with
// its name correctly.
func TestDescribe_ByProject(t *testing.T) {
	root := fromTempDirectory(t)

	err := fn.New().Init(fn.Function{
		Name:     "testname",
		Runtime:  "go",
		Registry: TestRegistry,
		Root:     root,
	})
	if err != nil {
		t.Fatal(err)
	}

	describer := mock.NewDescriber()
	describer.DescribeFn = func(n string) (i fn.InstanceRef, err error) {
		if n != "testname" {
			t.Fatalf("expected describer to receive name 'testname', got '%v'", n)
		}
		return
	}
	cmd := NewDescribeCmd(NewTestClient(fn.WithDescriber(describer)))
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

// TestDescribe_NameAndPathExclusivity ensures that providing both a name
// and a path will generate an error.
func TestDescribe_NameAndPathExclusivity(t *testing.T) {
	d := mock.NewDescriber()
	cmd := NewDescribeCmd(NewTestClient(fn.WithDescriber(d)))
	cmd.SetArgs([]string{"-p", "./testpath", "testname"})
	if err := cmd.Execute(); err == nil {
		// TODO(lkingland): use a typed error
		t.Fatalf("expected error on conflicting flags not received")
	}
	if d.DescribeInvoked {
		t.Fatal("describer was invoked when conflicting flags were provided")
	}
}

// TestDescribe_Namespace ensures that the namespace provided to the client
// for use when describing a function is set
//  1. Blank when not provided nor available (delegate to the describer impl to
//     choose current kube context)
//  2. The namespace of the contextually active function
//  3. The flag /env variable if provided
func TestDescribe_Namespace(t *testing.T) {
	root := fromTempDirectory(t)

	client := fn.New(fn.WithDescriber(mock.NewDescriber()))

	// Ensure that the default is "", indicating the describer should use
	// config.DefaultNamespace
	cmd := NewDescribeCmd(func(cc ClientConfig, _ ...fn.Option) (*fn.Client, func()) {
		if cc.Namespace != "" {
			t.Fatalf("expected '', got '%v'", cc.Namespace)
		}
		return client, func() {}
	})
	cmd.SetArgs([]string{"somefunc"}) // by name such that no f need be created
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Ensure the extant function's namespace is used
	f := fn.Function{
		Root:    root,
		Runtime: "go",
		Deploy: fn.DeploySpec{
			Namespace: "deployed",
		},
	}
	if err := client.Init(f); err != nil {
		t.Fatal(err)
	}
	cmd = NewDescribeCmd(func(cc ClientConfig, _ ...fn.Option) (*fn.Client, func()) {
		if cc.Namespace != "deployed" {
			t.Fatalf("expected 'deployed', got '%v'", cc.Namespace)
		}
		return client, func() {}
	})
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	// Ensure an explicit namespace is plumbed through
	cmd = NewDescribeCmd(func(cc ClientConfig, _ ...fn.Option) (*fn.Client, func()) {
		if cc.Namespace != "ns" {
			t.Fatalf("expected 'ns', got '%v'", cc.Namespace)
		}
		return client, func() {}
	})
	cmd.SetArgs([]string{"--namespace", "ns"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

}
