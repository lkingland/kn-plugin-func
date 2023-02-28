package functions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v2"
	. "knative.dev/func/pkg/testing"
)

// migration 0 is the genesis migration
// It represents functions prior to having a version.
func migration0(f any, m migration) (any, error) {
	return v0_0_0{}, nil
}

// migration 1 is the simplest migration other than the above degenerate case:
// adding a field of primitive type.
func migration1(f any, m migration) (any, error) {
	// No mapping is necessary because only new fields were added.
	return v1{SpecVersion: "0.1.0"}, // handled automatically by actual Functions
		nil
}

// migration2 is the next simplest migration, renaming a field.
func migration2(f any, m migration) (any, error) {
	in := f.(v1)
	// NOTE: These early migration examples map each field individually and
	// manually set SpecVersion.
	//
	// In an an actual function migration (and later more advanced tests), we
	// write out 'in; to disk, then read it back in as an 'out' because a
	// function N should always be parseable as a function N+1, even when that
	// means introducing an interstital migration to. Also, SpecVersion need not
	// be manually carried through as this is handled by Function.Write.
	//
	// This process allows each migration to only define changes.
	// For example, changing the type of a field would require introducing a
	// migration which renames the field, followed by a migration which adds
	// it back as the new type, removing the old field and copying in the value.
	return v2{SpecVersion: "0.3.0", // handled automatically by actual Functions
		IntField: in.A,
	}, nil
}

// TODO: Add and remove a nested struct
//       Partial updats via interstitial serialization
//       type change

// migrationN must always return type Function
func migrationN(f any, m migration) (any, error) {
	// The final step of the test migrations is to convert to
	// an actual Funciton object such that the core code can
	// remain typesafe internally.
	in, ok := f.(v2)
	if !ok {
		return nil, errors.New("migrateTestToFuncton received unexpected input type")
	}
	// Map test members to any Function member currently in use; and it doesn't
	// matter which.  It just hast to match the assertions in the tests.
	return Function{SpecVersion: "0.3.0", // handled automatically by actual Functions
		Image: fmt.Sprintf("%v", in.IntField),
	}, nil
}

// TestMigrate ensures that the basic tasks of migrations can be handled:
// Field Addition
// Field Renaming
// Field Removal
func TestMigrate(t *testing.T) {
	var (
		err        error
		root, done = Mktemp(t)
		filepath   = filepath.Join(root, FunctionFile)
	)
	defer done()

	tt := []migration{
		{"0.0.0", v0_0_0{}, v0_0_0{}, migration0}, // must define a genesis migration for unversioned functions
		{"0.1.0", v0_0_0{}, v1{}, migration1},
		{"0.2.0", v1{}, v2{}, migration2},
		{"0.3.0", v2{}, Function{}, migrationN}, // must define a final migration which outputs a Function
	}
	// write is a test helper which can write untyped (test) functions,
	// since they do not have an actual Function's .Write method.
	write := func(f any) {
		bytes, _ := yaml.Marshal(&f)
		os.WriteFile(filepath, bytes, os.ModePerm)
	}

	// Final Migration:
	// f(N-1).A           -> fN.Image
	// f(N-1).StringField -> fN.Name

	// Base case
	// Start with an the genesis struct which has no version field
	// and ensure it can be migrated all the way to the final Function without
	// failing.
	/*
		write(v0_0_0{})
		if _, err = Migrate(root, tt); err != nil {
			t.Fatalf("base case failed: %v", err)
		}
	*/

	// Add Field
	// A field can be added.  Everything parses, is typesafe and the value
	// carries through to the final Function.
	write(v1{
		SpecVersion: "0.1.0", // SpecVersion handled autmatically by actual Functions
		A:           1,
	})

	/* DEBUGGING
	tp := reflect.TypeOf(v1{})
	file, err := os.Open(filepath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	d := yaml.NewDecoder(file)
	y := reflect.New(tp).Interface()

	if err := d.Decode(&y); err != nil {
		t.Fatal(err)
	}
	fmt.Printf("#### %#v\n", y)
	t.Fatal("stopping")
	*/

	// fmt.Printf("%v/func.yaml\n", root)
	// time.Sleep(30 * time.Second)
	fN, err := Migrate(root, tt)
	if err != nil {
		t.Fatal(err)
	}
	if fN.Image != "1" {
		t.Fatalf("expected '1', got '%v'", fN.Image)
	}

}

// TestMigrated ensures that the .Migrated() method returns whether or not the
// migrations were applied based on its self-reported .SpecVersion member.
/* TODO: ensure cases are encapsulated in new tests and remove
func TestMigrated(t *testing.T) {
	vNext := semver.New(LastSpecVersion(Migrations))
	vNext.BumpMajor()

	tests := []struct {
		name     string
		f        Function
		migrated bool
	}{{
		name:     "no migration stamp",
		f:        Function{},
		migrated: false, // function with no specVersion stamp should be not migrated.
	}, {
		name:     "explicit small specVersion",
		f:        Function{SpecVersion: "0.0.1"},
		migrated: false,
	}, {
		name:     "latest specVersion",
		f:        Function{SpecVersion: LastSpecVersion(Migrations)},
		migrated: true,
	}, {
		name:     "future specVersion",
		f:        Function{SpecVersion: vNext.String()},
		migrated: true,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.f.Migrated(Migrations) != test.migrated {
				t.Errorf("Expected %q.Migrated() to be %t when latest is %q",
					test.f.SpecVersion, test.migrated, LastSpecVersion(Migrations))
			}
		})
	}
}
*/

// TestMigrate ensures that functions have migrations apply the specVersion
// stamp on instantiation indicating migrations have been applied.
// TODO: merge into test above or rename?
func Test_0_0_0Migration(t *testing.T) {
	// Load an old function, as it an earlier version it has registered migrations
	// that will need to be applied.
	root := "testdata/migrations/v0.19.0"

	// Instantiate the function with the antiquated structure, which should cause
	// migrations to be applied in order, and result in a function whose version
	// compatibility is equivalent to the latest registered migration.
	f, err := NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.SpecVersion != LastSpecVersion(Migrations) {
		t.Fatalf("Function was not migrated to %v on instantiation: specVersion is %v",
			LastSpecVersion(Migrations), f.SpecVersion)
	}
}

// TestMigrateToCreationStamp ensures that the creation timestamp migration
// introduced for functions 0.19.0 and earlier is applied.
func Test0_19_0Migration(t *testing.T) {
	// Load a function of version 0.19.0, which should have the migration applied
	root := "testdata/migrations/v0.19.0"

	now := time.Now()
	f, err := NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}

	if f.Created.Before(now) {
		t.Fatalf("migration not applied: expected timestamp to be now, got %v.", f.Created)
	}
}

// TestMigrateToBuilderImages ensures that the migration which migrates
// from "builder" and "builders" to "builderImages" is applied.  This results
// in the attributes being removed and no errors on load of the function with
// old schema.
func Test0_23_0Migration(t *testing.T) {
	// Load a function created prior to the adoption of the builder images map
	// (was created with 'builder' and 'builders' which does not support different
	// builder implementations.
	root := "testdata/migrations/v0.23.0"

	// Without the migration, instantiating the older function would error
	// because its strict unmarshalling would fail parsing the unexpected
	// 'builder' and 'builders' members.
	_, err := NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
}

// TestMigrateToBuilderImagesCustom ensures that the migration to builderImages
// correctly carries forward a customized value for 'builder'.
func Test0_23_0MigrationCustomized(t *testing.T) {
	// An early version of a function which includes a customized value for
	// the 'builder'.  This should be correctly carried forward to
	// the namespaced 'builderImages' map as image for the "pack" builder.
	root := "testdata/migrations/v0.23.0-customized"
	expected := "example.com/user/custom-builder" // set in testdata func.yaml

	f, err := NewFunction(root)
	if err != nil {
		t.Fatal(f)
	}
	i, ok := f.Build.BuilderImages["pack"]
	if !ok {
		t.Fatal("migrated function does not include the pack builder images")
	}
	if i != expected {
		t.Fatalf("migrated function expected builder image '%v', got '%v'", expected, i)
	}
}

// TestMigrateToSpecVersion ensures that a func.yaml file with a "version" field
// is migrated to use the field name "specVersion"
func Test0_25_0Migration(t *testing.T) {
	root := "testdata/migrations/v0.25.0"
	f, err := NewFunction(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.SpecVersion != LastSpecVersion(Migrations) {
		t.Fatal("migrated function does not include the Migration field")
	}
}

// TestMigrateToSpecs ensures that the migration to the sub-specs format from
// the previous Function structure works
/* TODO needs Git as miggration subtype first
func TestMigrateToSpecs(t *testing.T) {
	root := "testdata/migrations/v0.34.0"
	expectedGit := Git{URL: "http://test-url", Revision: "test revision", ContextDir: "/test/context/dir"}
	expectedNamespace := "test-namespace"
	var expectedEnvs []Env
	var expectedVolumes []Volume

	f, err := NewFunction(root)
	if err != nil {
		t.Error(err)
		t.Fatal(f)
	}

	if f.Build.Git != expectedGit {
		t.Fatalf("migrated Function expected Git '%v', got '%v'", expectedGit, f.Build.Git)
	}

	if f.Deploy.Namespace != expectedNamespace {
		t.Fatalf("migrated Function expected Namespace '%v', got '%v'", expectedNamespace, f.Deploy.Namespace)
	}

	if len(f.Run.Envs) != len(expectedEnvs) {
		t.Fatalf("migrated Function expected Run Envs '%v', got '%v'", len(expectedEnvs), len(f.Run.Envs))
	}

	if len(f.Run.Volumes) != len(expectedVolumes) {
		t.Fatalf("migrated Function expected Run Volumes '%v', got '%v'", len(expectedEnvs), len(f.Run.Envs))
	}

}
*/

// TestMigrateFromInvokeStructure tests that migration from f.Invocation.Format to
// f.Invoke works
func Test0_35_0Migration(t *testing.T) {
	root0 := "testdata/migrations/v0.35.0"
	expectedInvoke := "" // empty because http is default and not written in yaml file

	f0, err := NewFunction(root0)
	if err != nil {
		t.Error(err)
		t.Fatal(f0)
	}
	if f0.Invoke != expectedInvoke {
		t.Fatalf("migrated Function expected Invoke '%v', got '%v'", expectedInvoke, f0.Invoke)
	}

	root1 := "testdata/migrations/v0.35.0-nondefault"
	expectedInvoke = "cloudevent"
	f1, err := NewFunction(root1)
	if err != nil {
		t.Error(err)
		t.Fatal(f1)
	}
	if f1.Invoke != expectedInvoke {
		t.Fatalf("migrated Function expected Invoke '%v', got '%v'", expectedInvoke, f0.Invoke)
	}
}

// TODO: Test0_36_0Migration
