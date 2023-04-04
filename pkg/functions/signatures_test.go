package functions

import (
	"path/filepath"
	"testing"
)

func TestSignatureDetectors(t *testing.T) {
	// TODO: why is the repository's folder structure now out-of-sync with its
	// logical structure?  It used to be clear:
	// Client
	//  -> templates
	//  -> knative/Deployer
	//  -> docker/Puser
	//  etc.
	// But now we have a 'pkg/functions/Client' which depends on '../../templates'
	// and '../knative/Deployer' etc? When listing the contents of 'pkg' there is
	// no clear root. The "pkg" subdirectory convention was even called out
	// in particular by Dave Cheney as being an unnecessary distinction which
	// somehow stuck despite the Go team having long-since moved on.
	// Since we have landed on the convention of 'fn' being the short name with
	// which we refer to the function core library, we should at a bare minimum
	// move all the code in "pkg/functions" up a directory and rename it "fn", and
	// move the things it depends on in, such as templates.
	//   import fn "knative.dev/func/pkg"
	tests := []struct {
		sig  Signature
		lang string
		tpl  string
	}{
		{InstancedHTTP, "go", "http"}, // Default http template is now InstancedHTTP
		// {InstancedCloudEvent, "go", "cloudevents"},
		{StaticHTTP, "go", ".http-static"}, // hidden templates are used for testing scaffolding and may be moved into a scaffolding subdirectory.
		// {StaticCloudEvent, "go", ".cloudevents-static"},
	}

	join := filepath.Join
	tpls := join("..", "..", "templates")
	for _, test := range tests {
		root := join(tpls, test.lang, test.tpl)
		f := Function{Root: root, Runtime: test.lang}
		s, err := detectSignature(f)
		if err != nil {
			t.Fatal(err)
		}
		if s != test.sig {
			t.Errorf("expected %v/%v to be detected as signature %v, got %v (root: %v)", test.lang, test.tpl, test.sig, s, root)
		}

	}
}
