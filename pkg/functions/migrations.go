package functions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/coreos/go-semver/semver"
	"gopkg.in/yaml.v2"
)

type v1 struct {
	SpecVersion string `yaml:"specVersion"`
	A           int    `yaml:"a"`
}

type v2 struct { // Rename member A
	SpecVersion string `yaml:"specVersion"`
	IntField    int    `yaml:"intField"`
}

type v4 struct { // Add nested struct
	SpecVersion string `yaml:"specVersion"`
	IntField    int    `yaml:"intField"`
	StructField v4_FieldC
}
type v4_FieldC struct {
	StringField string
}

type v5 struct { // Remove nested struct
	SpecVersion string `yaml:"specVersion"`
	IntField    int    `yaml:"intField"`
}

// Migrations
//
// All Function Migrations in ascending order.
// version for the migration if necessary)
// The version is the most recently tagged func minor version.
// No two migrations may have the exasame version number: use a patch
// version when more than one migration are introduced between releases.
var Migrations = []migration{
	// {"0.19.0", migrateToCreationStamp},
	// {"0.23.0", migrateToBuilderImages},
	// {"0.25.0", migrateToSpecVersion},
	// {"0.34.0", migrateToSpecsStructure},
	// {"0.35.0", migrateFromInvokeStructure},
	// {"0.36.0", migrateToLocalConfig},
	// New Migrations Here.
}

// Migrate instantiates a Function at path using any known function version,
// applies all migrations and returns the most recent Function type.
func Migrate(root string, mm []migration) (f Function, err error) {
	var x any
	path := filepath.Join(root, FunctionFile)

	// Preconditions
	//
	// The last migration returns a Function type
	// All keys are parseable a Semver.
	if err = assertLastMigrationIsFunction(mm); err != nil {
		return
	}
	if err = assertMigrationsUseSemver(mm); err != nil {
		return
	}
	fmt.Println("assertions passed")

	// Instantiate the Function
	//
	// Instantiate function at the given path using the latest version struct
	// possible from migrations.  This iterates over the output types of
	// each migration, indicating the function at the given path's structure
	// should match that ensured by its associated migration.
	if x, err = instantiateLatest(path, mm); err != nil {
		return
	}
	fmt.Printf("instantiated latest possible. x(%v):\n%#v\n", reflect.TypeOf(x), x)

	// Shift to tempfile
	// TODO: stick in OS temp location

	// Migrate
	return migrate(x, mm)
}

func assertLastMigrationIsFunction(mm []migration) (err error) {
	if len(mm) == 0 {
		return
	}
	if t := mm[len(mm)-1].outType(); t != reflect.TypeOf(Function{}) {
		err = fmt.Errorf("The last migration's output type must be Function, but got %v", t)
	}
	return
}

func assertMigrationsUseSemver(mm []migration) (err error) {
	if len(mm) == 0 {
		return
	}
	v0 := semver.New("0.0.0")
	for i, m := range mm {
		v1, err := semver.NewVersion(m.version)
		if err != nil {
			return fmt.Errorf("unable to load migration %v version specified ('%v') failed to parse as a semver: %w", i, m.version, err)
		}
		if v1.LessThan(*v0) {
			return fmt.Errorf("migration %v is out of order.  Migrations must be defined in ascending order.", i)
		}
	}
	return
}

// instantateLatest function structure from the given path.
// Try to instantiate path as the output type of the given migration,
func instantiateLatest(path string, mm []migration) (any, error) {
	if len(mm) == 0 {
		// TODO: This logic rquires there always be at least one migration,
		// the one which yields a final (current) function (the default constructor)
		return nil, errors.New("no migrations defined")
	}
	var (
		x        any
		firstErr error
		err      error
	)
	for i := len(mm) - 1; i >= 0; i-- {
		fmt.Printf("\n instantiateLatest - checking mm[%v].version = %v\n", i, mm[i].version)
		if x, err = instantiateAs(path, mm[i].outType()); err != nil {
			fmt.Printf("   decode error: %v\n", err)
			if firstErr == nil {
				firstErr = err
			}
			continue // Errors decoding indicates the next older should be tried.
		}
		fmt.Printf("- instantiated. type: %v\n", reflect.TypeOf(x))
		xv, err := versionOf(x)
		if err != nil {
			fmt.Printf("- error versionOf: %v\n", err)
			return nil, err // Errors parsing SpecVersion fail fast
		}
		fmt.Printf("- instantiated as version: %v\n", xv)
		if semver.New(mm[i].version).Equal(*xv) {
			return x, nil // successfully loaded struct with matching version
		}
		fmt.Printf("- version mismatch. %v != %v\n", xv, semver.New(mm[i].version))
	}
	if firstErr == nil {
		err = errors.New("unable to load path as a Function of any version")
	}
	return nil, err
}

// instantiateAs returns the path instantiated as the given type.
func instantiateAs(path string, t reflect.Type) (any, error) {
	fmt.Printf("attempting instantiation as %v\n", t)
	fmt.Printf("  file: %v\n", path)
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	/* TODO: this works, but results in a runtime type of a map of interfaces
	 * rather than the expected type 't'

	d := yaml.NewDecoder(file)
	y := reflect.New(t).Interface()
	err = d.Decode(&y)
	fmt.Printf("Instantiated: %#v\n", y)
	return y, err
	*/

	// DEBUGGING:
	d := yaml.NewDecoder(file)

	switch {
	case t == reflect.TypeOf(Function{}):
		y := Function{}
		err = d.Decode(&y)
		fmt.Printf("  file decoded as: %#v\n", y)
		return y, err
	case t == reflect.TypeOf(v0_0_0{}):
		y := v0_0_0{}
		err = d.Decode(&y)
		fmt.Printf("  file decoded as: %#v\n", y)
		return y, err
	case t == reflect.TypeOf(v1{}):
		y := v1{}
		err = d.Decode(&y)
		fmt.Printf("  file decoded as: %#v\n", y)
		return y, err
	case t == reflect.TypeOf(v2{}):
		y := v2{}
		err = d.Decode(&y)
		fmt.Printf("  file decoded as: %#v\n", y)
		return y, err
	}
	return nil, errors.New("unregistered type")
}

// versionOf X as a semver. Errors if not parseable as a semver.
// Returns v0.0.0 if the field either does not exist or is empty.
func versionOf(x any) (version *semver.Version, err error) {
	value := reflect.ValueOf(x)
	field := value.FieldByName("SpecVersion")
	specVersion := field.String()
	if field.Kind() == reflect.Invalid || specVersion == "" {
		// if SpecVersion was not found it is set to the special string <invalid value>
		// because .String() always returns the zero value of a String.
		// Check here for the special case Kind == Invalid and treat this as meaning
		// when x has no field SpecVersion, that is equivalent to SpecVersion="".
		// which is equvalant to the genesis version v0.0.0
		return semver.New("0.0.0"), nil
	}
	return semver.NewVersion(specVersion)
}

func migrate(x any, mm []migration) (f Function, err error) {
	for _, m := range mm {
		fmt.Printf("   applying migration %v\n", m.version)
		// Skip if this migration is unneeded (specVersion is >= migration version)
		if !needsMigration(x, m) {
			fmt.Printf("   migration %v not needed\n", m.version)
			continue
		}
		if x, err = m.migrate(x, m); err != nil {
			return f, fmt.Errorf("unable to apply migration %v: %w", m.version, err)
		}
		fmt.Println("   migration completed")
	}
	fmt.Println("   migrations complete")
	return x.(Function), nil
}

// needsMigration returns true unless the given object has a SpecVersion
// field whose value is > the passed migration.
func needsMigration(x any, m migration) bool {
	v, err := versionOf(x)
	if err != nil {
		return true
	}
	mv := semver.New(m.version)
	return v.LessThan(*mv)
}

// LastSpecVersion returns the string value for the most recent migration
func LastSpecVersion(mm []migration) string {
	return mm[len(mm)-1].version
}

// Types
// --------------
type v0_0_0 struct{} // The genesis version

// migration defines a migrator version below which it is needed.
type migration struct {
	version string
	in, out any
	migrate func(any, migration) (any, error)
}

func (m migration) inType() reflect.Type {
	return reflect.TypeOf(m.in)
}

func (m migration) outType() reflect.Type {
	return reflect.TypeOf(m.out)
}

type writeable interface {
	Write() error
}

func write(f any) error {
	if _, ok := f.(writeable); !ok {
		return errors.New("migration type must be writeable")
	}
	return f.(writeable).Write()
}

// Migrations Registry
// -------------------

// Migration Implementations
// ------------------------------------

// migrateToCreationStamp
// The initial migration which brings a function from
// some unknown point in the past to the point at which it is versioned,
// migrated and includes a creation timestamp.  Without this migration,
// instantiation of old functions will fail with a "Function at path X not
// initialized" in func versions above v0.19.0
//
// This migration must be aware of the difference between a function which
// was previously created (but with no created stamp), and a function which
// exists only in memory and should legitimately fail the .Initialized() check.
// The only way to know is to check a side effect of earlier versions:
// are the `.Name` and `.Runtime` fields populated.  This was the way the
// `.Initialized` check was implemented prior to versioning being introduced, so
// it is equivalent logically to use this here as well.

// In summary:  if the creation stamp is zero, but name and runtime fields are
// populated, then this is an old function and should be migrated to having a
// created stamp.  Otherwise, this is an in-memory (new) function that is
// currently in the process of being created and as such need not be mutated
// to consider this migration having been evaluated.
func migrateToCreationStamp(f Function, m migration) (Function, error) {
	// For functions with no creation timestamp, but appear to have been pre-
	// existing, populate their created stamp and version.
	// Yes, it's a little gnarly, but bootstrapping into the loveliness of a
	// versioned/migrated system takes cleaning up the trash.
	if f.Created.IsZero() { // If there is no created stamp
		if f.Name != "" && f.Runtime != "" { // and it appears to be an old function
			f.Created = time.Now() // Migrate it to having a timestamp.
		}
	}
	f.SpecVersion = m.version // Record this migration was evaluated.
	return f, nil
}

// migrateToBuilderImages
// Prior to this migration, 'builder' and 'builders' attributes of a function
// were specific to buildpack builds.  In addition, the separation of the two
// fields was to facilitate the use of "named" inbuilt builders, which ended
// up not being necessary.  With the addition of the S2I builder implementation,
// it is necessary to differentiate builders for use when building via Pack vs
// builder for use when building with S2I.  Furthermore, now that the builder
// itself is a user-supplied parameter, the short-hand of calling builder images
// simply "builder" is not possible, since that term more correctly refers to
// the builder being used (S2I, pack, or some future implementation), while this
// field very specifically refers to the image the chosen builder should use
// (in leau of the inbuilt default).
//
// For an example of the situation:  the 'builder' member used to instruct the
// system to use that builder _image_ in all cases.  This of course restricts
// the system from being able to build with anything other than the builder
// implementation to which that builder image applies (pack or s2i).  Further,
// always including this value in the serialized func.yaml causes this value to
// be pegged/immutable (without manual intervention), which hampers our ability
// to change out the underlying builder image with future versions.
//
// The 'builder' and 'builders' members have therefore been removed in favor
// of 'builderImages', which is keyed by the short name of the builder
// implementation (currently 'pack' and 's2i').  Its existence is optional,
// with the default value being provided in the associated builder's impl.
// Should the value exist, this indicates the user has overridden the value,
// or is using a fully custom language pack.
//
// This migration allows pre-builder-image functions to load despite their
// inclusion of the now removed 'builder' member.  If the user had provided
// a customized builder image, that value is preserved as the builder image
// for the 'pack' builder in the new version (s2i did not exist prior).
// See associated unit tests.
func migrateToBuilderImages(f1 Function, m migration) (Function, error) {
	// Load the function using pertinent parts of the previous version's schema:
	f0Filename := filepath.Join(f1.Root, FunctionFile)
	bb, err := os.ReadFile(f0Filename)
	if err != nil {
		return f1, errors.New("migration 'migrateToBuilderImages' error: " + err.Error())
	}
	f0 := migrateToBuilderImages_previousFunction{}
	if err = yaml.Unmarshal(bb, &f0); err != nil {
		return f1, errors.New("migration 'migrateToBuilderImages' error: " + err.Error())
	}

	// At time of this migration, the default pack builder image for all language
	// runtimes is:
	defaultPackBuilderImage := "gcr.io/paketo-buildpacks/builder:base"

	// If the old function had defined something custom
	if f0.Builder != "" && f0.Builder != defaultPackBuilderImage {
		// carry it forward as the new pack builder image
		if f1.Build.BuilderImages == nil {
			f1.Build.BuilderImages = make(map[string]string)
		}
		f1.Build.BuilderImages["pack"] = f0.Builder
	}

	// Flag f1 as having had the migration applied
	f1.SpecVersion = m.version
	return f1, nil
}

type migrateToBuilderImages_previousFunction struct {
	Builder string `yaml:"builder"`
}

// migrateToSpecVersion updates a func.yaml file to use SpecVersion
// instead of Version to track the migration numbers
func migrateToSpecVersion(f Function, m migration) (Function, error) {
	// Load the function func.yaml file
	f0Filename := filepath.Join(f.Root, FunctionFile)
	bb, err := os.ReadFile(f0Filename)
	if err != nil {
		return f, errors.New("migration 'migrateToSpecVersion' error: " + err.Error())
	}

	// Only handle the Version field if it exists
	f0 := migrateToSpecVersion_previousFunction{}
	if err = yaml.Unmarshal(bb, &f0); err != nil {
		return f, errors.New("migration 'migrateToSpecVersion' error: " + err.Error())
	}

	f.SpecVersion = m.version
	return f, nil
}

type migrateToSpecVersion_previousFunction struct {
	// Functions prior to 0.25 will have a Version field
	Version string `yaml:"version"`
}

// migrateToSpecsStructure migration makes sure use the sub-specs structs for
// build, run and deploy phases. To avoid unmarshalling issues with the old
// format this migration needs to be executed first. Further migrations will
// operate on this new struct with sub-specs
func migrateToSpecsStructure(f1 Function, m migration) (Function, error) {
	// Load the Function using pertinent parts of the previous version's schema:
	f0Filename := filepath.Join(f1.Root, FunctionFile)
	bb, err := os.ReadFile(f0Filename)
	if err != nil {
		return f1, errors.New("migration 'migrateToSpecsStructure' error: " + err.Error())
	}
	f0 := migrateToSpecs_previousFunction{}
	if err = yaml.Unmarshal(bb, &f0); err != nil {
		return f1, errors.New("migration 'migrateToSpecsStructure' error: " + err.Error())
	}

	//Append BuilderImages from old format, without destroying previous migrations
	if f0.BuilderImages != nil {
		for k, v := range f0.BuilderImages {
			f1.Build.BuilderImages[k] = v
		}
	}
	if f0.Buildpacks != nil {
		f1.Build.Buildpacks = append(f1.Build.Buildpacks, f0.Buildpacks...)
	}
	if f0.BuildEnvs != nil {
		f1.Build.BuildEnvs = append(f1.Build.BuildEnvs, f0.BuildEnvs...)
	}

	if f0.Volumes != nil {
		f1.Run.Volumes = append(f1.Run.Volumes, f0.Volumes...)
	}

	if f0.Envs != nil {
		f1.Run.Envs = append(f1.Run.Envs, f0.Envs...)
	}

	if f0.Annotations != nil {
		for k, v := range f0.Annotations {
			f1.Deploy.Annotations[k] = v
		}
	}

	if f0.Options.Resources != nil {
		f1.Deploy.Options.Resources = f0.Options.Resources
	}

	if f0.Options.Scale != nil {
		f1.Deploy.Options.Scale = f0.Options.Scale
	}

	if f0.Labels != nil {
		f1.Deploy.Labels = append(f1.Deploy.Labels, f0.Labels...)
	}

	if f0.HealthEndpoints.Readiness != "" {
		f1.Deploy.HealthEndpoints.Readiness = f0.HealthEndpoints.Readiness
	}

	if f0.HealthEndpoints.Liveness != "" {
		f1.Deploy.HealthEndpoints.Liveness = f0.HealthEndpoints.Liveness
	}

	f1.Deploy.Namespace = f0.Namespace
	f1.Build.Builder = f0.Builder
	f1.SpecVersion = m.version
	return f1, nil
}

type migrateToSpecs_previousFunction struct {
	Annotations     map[string]string `yaml:"annotations"`
	BuildEnvs       []Env             `yaml:"buildEnvs"`
	Builder         string            `yaml:"builder" jsonschema:"enum=pack,enum=s2i"`
	BuilderImages   map[string]string `yaml:"builderImages,omitempty"`
	Buildpacks      []string          `yaml:"buildpacks"`
	Envs            []Env             `yaml:"envs"`
	HealthEndpoints HealthEndpoints   `yaml:"healthEndpoints"`
	Labels          []Label           `yaml:"labels"`
	Namespace       string            `yaml:"namespace"`
	Options         Options           `yaml:"options"`
	Volumes         []Volume          `yaml:"volumes"`
}

// migrateFromInvokeStructure migrates functions prior 0.35.0 via changing
// the Invocation.format(string) to new Invoke(string) to minimalize func.yaml
// file. When Invoke now holds default value (http) it will not show up in
// func.yaml as the default value is implicitly expected. Otherwise if Invoke
// is non-default value, it will be written in func.yaml.
func migrateFromInvokeStructure(f1 Function, m migration) (Function, error) {
	// Load the Function using pertinent parts of the previous version's schema:
	f0Filename := filepath.Join(f1.Root, FunctionFile)
	bb, err := os.ReadFile(f0Filename)
	if err != nil {
		return f1, errors.New("migration 'migrateFromInvokeStructure' error: " + err.Error())
	}
	f0 := migrateFromInvokeStructure_previousFunction{}
	if err = yaml.Unmarshal(bb, &f0); err != nil {
		return f1, errors.New("migration 'migrateFromInvokeStructure' error: " + err.Error())
	}

	if f0.Invocation.Format != "" && f0.Invocation.Format != "http" {
		f1.Invoke = f0.Invocation.Format
	}

	// Flag f1 as having had the migration applied
	f1.SpecVersion = m.version
	return f1, nil
}

type migrateFromInvokeStructure_previousFunction struct {
	Invocation migrateFromInvokeStructure_invocation `yaml:"invocation,omitempty"`
}

type migrateFromInvokeStructure_invocation struct {
	Format string `yaml:"format,omitempty"`
}

// migrateToLocalConfig moves any existing Git settings and the remote
// deployment flag to the function's local config in .func/config.yaml
func migrateToLocalConfig(f1 Function, m migration) (Function, error) {
	f0Filename := filepath.Join(f1.Root, FunctionFile)
	bb, err := os.ReadFile(f0Filename)
	if err != nil {
		return f1, errors.New("migration 'migrateToLocalConfig' error: " + err.Error())
	}
	f0 := migrateToLocalConfig_previousFunction{}
	if err = yaml.Unmarshal(bb, &f0); err != nil {
		return f1, errors.New("migration 'migrateFromInvokeStructure' error: " + err.Error())
	}

	// Flag f1 as having had the migration applied
	f1.SpecVersion = m.version
	return f1, nil
}

type migrateToLocalConfig_previousFunction struct {
	// No pertinent parts of the earlier (remote, git etc should be ignored)

}
