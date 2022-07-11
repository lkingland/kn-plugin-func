//go:build !integration
// +build !integration

package function

import (
	"testing"
)

func TestFunction_ImageWithDigest(t *testing.T) {
	type fields struct {
		Image       string
		ImageDigest string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name:   "Full path with port",
			fields: fields{Image: "image-registry.openshift-image-registry.svc.cluster.local:50000/default/bar", ImageDigest: "42"},
			want:   "image-registry.openshift-image-registry.svc.cluster.local:50000/default/bar@42",
		},
		{
			name:   "Path with namespace",
			fields: fields{Image: "johndoe/bar", ImageDigest: "42"},
			want:   "johndoe/bar@42",
		},
		{
			name:   "Just image name",
			fields: fields{Image: "bar:latest", ImageDigest: "42"},
			want:   "bar@42",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Function{
				Image:       tt.fields.Image,
				ImageDigest: tt.fields.ImageDigest,
			}
			if got := f.ImageWithDigest(); got != tt.want {
				t.Errorf("ImageWithDigest() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestFunction_ImageName ensures that the full image name is
// returned for a Function, based on the Function's Registry and Name,
// including utilizing the DefaultRegistry if the Function's defined
// registry is a single token (just the namespace).
func TestFunction_ImageName(t *testing.T) {
	var (
		f   Function
		got string
		err error
	)
	tests := []struct {
		registry      string
		name          string
		expectedImage string
		expectError   bool
	}{
		{"alice", "myfunc", DefaultRegistry + "/alice/myfunc:latest", false},
		{"quay.io/alice", "myfunc", "quay.io/alice/myfunc:latest", false},
		{"docker.io/alice", "myfunc", "docker.io/alice/myfunc:latest", false},
		{"docker.io/alice/sub", "myfunc", "docker.io/alice/sub/myfunc:latest", false},
		{"alice", "", "", true},
		{"", "myfunc", "", true},
	}
	for _, test := range tests {
		f = Function{Registry: test.registry, Name: test.name}
		got, err = f.ImageName()
		if test.expectError && err == nil {
			t.Errorf("registry '%v' and name '%v' did not yield the expected error", test.registry, test.name)
		}
		if got != test.expectedImage {
			t.Errorf("expected registry '%v' and name '%v' to yield image name '%v', got '%v'", test.registry, test.name, test.expectedImage, got)
		}
	}

}

// TestDeploy_ImageName ensures that the image name deployed considers
// various permutations of the defined registry.
func Test_NewImageTag(t *testing.T) {
	/*
		tests := []struct {
			name     string // name of the test case
			fnName   string // Function's name
			image    string // A provided image value
			registry string // A provided registry value
			want     string
		}{
			{
				name:     "No change",
				fnName:   "testDerivedImage",
				image:    "docker.io/foo/testDerivedImage:latest",
				registry: "docker.io/foo",
				want:     "docker.io/foo/testDerivedImage:latest",
			},
			{
				name:     "Same registry without docker.io/, original with docker.io/",
				fnName:   "testDerivedImage0",
				image:    "docker.io/foo/testDerivedImage0:latest",
				registry: "foo",
				want:     "docker.io/foo/testDerivedImage0:latest",
			},
			{
				name:     "Same registry, original without docker.io/",
				fnName:   "testDerivedImage1",
				image:    "foo/testDerivedImage1:latest",
				registry: "foo",
				want:     "docker.io/foo/testDerivedImage1:latest",
			},
			{
				name:     "Different registry without docker.io/, original without docker.io/",
				fnName:   "testDerivedImage2",
				image:    "foo/testDerivedImage2:latest",
				registry: "bar",
				want:     "docker.io/bar/testDerivedImage2:latest",
			},
			{
				name:     "Different registry with docker.io/, original without docker.io/",
				fnName:   "testDerivedImage3",
				image:    "foo/testDerivedImage3:latest",
				registry: "docker.io/bar",
				want:     "docker.io/bar/testDerivedImage3:latest",
			},
			{
				name:     "Different registry with docker.io/, original with docker.io/",
				fnName:   "testDerivedImage4",
				image:    "docker.io/foo/testDerivedImage4:latest",
				registry: "docker.io/bar",
				want:     "docker.io/bar/testDerivedImage4:latest",
			},
			{
				name:     "Different registry with quay.io/, original without docker.io/",
				fnName:   "testDerivedImage5",
				image:    "foo/testDerivedImage5:latest",
				registry: "quay.io/foo",
				want:     "quay.io/foo/testDerivedImage5:latest",
			},
			{
				name:     "Different registry with quay.io/, original with docker.io/",
				fnName:   "testDerivedImage6",
				image:    "docker.io/foo/testDerivedImage6:latest",
				registry: "quay.io/foo",
				want:     "quay.io/foo/testDerivedImage6:latest",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {

				root, rm := Mktemp(t)
				defer rm()

				if err := New().Create(Function{Runtime: "go", Name: tt.fnName, Root: root}); err != nil {
					t.Fatal(err)
				}

				got, err := ImageName(root, tt.registry)
				if err != nil {
					t.Errorf("DerivedImage() for image %v and registry %v; got error %v", tt.image, tt.registry, err)
				}
				if got != tt.want {
					t.Errorf("DerivedImage() for image %v and registry %v; got %v, want %v", tt.image, tt.registry, got, tt.want)
				}
			})
		}
	*/
}
