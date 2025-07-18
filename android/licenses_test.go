package android

import (
	"testing"

	"github.com/google/blueprint"
)

var licensesTests = []struct {
	name                string
	fs                  MockFS
	expectedErrors      []string
	effectivePackage    map[string]string
	effectiveNotices    map[string][]string
	effectiveKinds      map[string][]string
	effectiveConditions map[string][]string
}{
	{
		name: "invalid module type without licenses property",
		fs: map[string][]byte{
			"top/Android.bp": []byte(`
				mock_bad_module {
					name: "libexample",
				}`),
		},
		expectedErrors: []string{`module type "mock_bad_module" must have an applicable licenses property`},
	},
	{
		name: "license must exist",
		fs: map[string][]byte{
			"top/Android.bp": []byte(`
				mock_library {
					name: "libexample",
					licenses: ["notice"],
				}`),
		},
		expectedErrors: []string{`"libexample" depends on undefined module "notice"`},
	},
	{
		name: "all good",
		fs: map[string][]byte{
			"top/Android.bp": []byte(`
				license_kind {
					name: "notice",
					conditions: ["shownotice"],
				}

				license {
					name: "top_Apache2",
					license_kinds: ["notice"],
					package_name: "topDog",
					license_text: ["LICENSE", "NOTICE"],
				}

				mock_library {
					name: "libexample1",
					licenses: ["top_Apache2"],
				}`),
			"top/nested/Android.bp": []byte(`
				mock_library {
					name: "libnested",
					licenses: ["top_Apache2"],
				}`),
			"other/Android.bp": []byte(`
				mock_library {
					name: "libother",
					licenses: ["top_Apache2"],
				}`),
		},
		effectiveKinds: map[string][]string{
			"libexample1": []string{"notice"},
			"libnested":   []string{"notice"},
			"libother":    []string{"notice"},
		},
		effectivePackage: map[string]string{
			"libexample1": "topDog",
			"libnested":   "topDog",
			"libother":    "topDog",
		},
		effectiveConditions: map[string][]string{
			"libexample1": []string{"shownotice"},
			"libnested":   []string{"shownotice"},
			"libother":    []string{"shownotice"},
		},
		effectiveNotices: map[string][]string{
			"libexample1": []string{"top/LICENSE:topDog", "top/NOTICE:topDog"},
			"libnested":   []string{"top/LICENSE:topDog", "top/NOTICE:topDog"},
			"libother":    []string{"top/LICENSE:topDog", "top/NOTICE:topDog"},
		},
	},

	// Defaults propagation tests
	{
		// Check that licenses is the union of the defaults modules.
		name: "defaults union, basic",
		fs: map[string][]byte{
			"top/Android.bp": []byte(`
				license_kind {
					name: "top_notice",
					conditions: ["notice"],
				}

				license {
					name: "top_other",
					license_kinds: ["top_notice"],
				}

				mock_defaults {
					name: "libexample_defaults",
					licenses: ["top_other"],
				}
				mock_library {
					name: "libexample",
					licenses: ["nested_other"],
					defaults: ["libexample_defaults"],
				}
				mock_library {
					name: "libsamepackage",
					deps: ["libexample"],
				}`),
			"top/nested/Android.bp": []byte(`
				license_kind {
					name: "nested_notice",
					conditions: ["notice"],
				}

				license {
					name: "nested_other",
					license_kinds: ["nested_notice"],
				}

				mock_library {
					name: "libnested",
					deps: ["libexample"],
				}`),
			"other/Android.bp": []byte(`
				mock_library {
					name: "libother",
					deps: ["libexample"],
				}`),
		},
		effectiveKinds: map[string][]string{
			"libexample":     []string{"nested_notice", "top_notice"},
			"libsamepackage": []string{},
			"libnested":      []string{},
			"libother":       []string{},
		},
		effectiveConditions: map[string][]string{
			"libexample":     []string{"notice"},
			"libsamepackage": []string{},
			"libnested":      []string{},
			"libother":       []string{},
		},
	},
	{
		name: "defaults union, multiple defaults",
		fs: map[string][]byte{
			"top/Android.bp": []byte(`
				license {
					name: "top",
				}
				mock_defaults {
					name: "libexample_defaults_1",
					licenses: ["other"],
				}
				mock_defaults {
					name: "libexample_defaults_2",
					licenses: ["top_nested"],
				}
				mock_library {
					name: "libexample",
					defaults: ["libexample_defaults_1", "libexample_defaults_2"],
				}
				mock_library {
					name: "libsamepackage",
					deps: ["libexample"],
				}`),
			"top/nested/Android.bp": []byte(`
				license {
					name: "top_nested",
					license_text: ["LICENSE.txt"],
				}
				mock_library {
					name: "libnested",
					deps: ["libexample"],
				}`),
			"other/Android.bp": []byte(`
				license {
					name: "other",
				}
				mock_library {
					name: "libother",
					deps: ["libexample"],
				}`),
			"outsider/Android.bp": []byte(`
				mock_library {
					name: "liboutsider",
					deps: ["libexample"],
				}`),
		},
		effectiveKinds: map[string][]string{
			"libexample":     []string{},
			"libsamepackage": []string{},
			"libnested":      []string{},
			"libother":       []string{},
			"liboutsider":    []string{},
		},
		effectiveNotices: map[string][]string{
			"libexample":     []string{"top/nested/LICENSE.txt"},
			"libsamepackage": []string{},
			"libnested":      []string{},
			"libother":       []string{},
			"liboutsider":    []string{},
		},
	},

	// Defaults module's defaults_licenses tests
	{
		name: "defaults_licenses invalid",
		fs: map[string][]byte{
			"top/Android.bp": []byte(`
				mock_defaults {
					name: "top_defaults",
					licenses: ["notice"],
				}`),
		},
		expectedErrors: []string{`"top_defaults" depends on undefined module "notice"`},
	},
	{
		name: "defaults_licenses overrides package default",
		fs: map[string][]byte{
			"top/Android.bp": []byte(`
				package {
					default_applicable_licenses: ["by_exception_only"],
				}
				license {
					name: "by_exception_only",
				}
				license {
					name: "notice",
				}
				mock_defaults {
					name: "top_defaults",
					licenses: ["notice"],
				}
				mock_library {
					name: "libexample",
				}
				mock_library {
					name: "libdefaults",
					defaults: ["top_defaults"],
				}`),
		},
	},

	// Package default_applicable_licenses tests
	{
		name: "package default_applicable_licenses must exist",
		fs: map[string][]byte{
			"top/Android.bp": []byte(`
				package {
					default_applicable_licenses: ["notice"],
				}`),
		},
		expectedErrors: []string{`"//top" depends on undefined module "notice"`},
	},
	{
		// This test relies on the default licenses being legacy_public.
		name: "package default_applicable_licenses property used when no licenses specified",
		fs: map[string][]byte{
			"top/Android.bp": []byte(`
				package {
					default_applicable_licenses: ["top_notice"],
				}

				license {
					name: "top_notice",
				}
				mock_library {
					name: "libexample",
				}`),
			"outsider/Android.bp": []byte(`
				mock_library {
					name: "liboutsider",
					deps: ["libexample"],
				}`),
		},
	},
	{
		name: "package default_applicable_licenses not inherited to subpackages",
		fs: map[string][]byte{
			"top/Android.bp": []byte(`
				package {
					default_applicable_licenses: ["top_notice"],
				}
				license {
					name: "top_notice",
				}
				mock_library {
					name: "libexample",
				}`),
			"top/nested/Android.bp": []byte(`
				package {
					default_applicable_licenses: ["outsider"],
				}

				mock_library {
					name: "libnested",
				}`),
			"top/other/Android.bp": []byte(`
				mock_library {
					name: "libother",
				}`),
			"outsider/Android.bp": []byte(`
				license {
					name: "outsider",
				}
				mock_library {
					name: "liboutsider",
					deps: ["libexample", "libother", "libnested"],
				}`),
		},
	},
	{
		name: "verify that prebuilt dependencies are included",
		fs: map[string][]byte{
			"prebuilts/Android.bp": []byte(`
				license {
					name: "prebuilt"
				}
				prebuilt {
					name: "module",
					licenses: ["prebuilt"],
				}`),
			"top/sources/source_file": nil,
			"top/sources/Android.bp": []byte(`
				license {
					name: "top_sources"
				}
				source {
					name: "module",
					licenses: ["top_sources"],
				}`),
			"top/other/source_file": nil,
			"top/other/Android.bp": []byte(`
				source {
					name: "other",
					deps: [":module"],
				}`),
		},
	},
	{
		name: "verify that prebuilt dependencies are ignored for licenses reasons (preferred)",
		fs: map[string][]byte{
			"prebuilts/Android.bp": []byte(`
				license {
					name: "prebuilt"
				}
				prebuilt {
					name: "module",
					licenses: ["prebuilt"],
					prefer: true,
				}`),
			"top/sources/source_file": nil,
			"top/sources/Android.bp": []byte(`
				license {
					name: "top_sources"
				}
				source {
					name: "module",
					licenses: ["top_sources"],
				}`),
			"top/other/source_file": nil,
			"top/other/Android.bp": []byte(`
				source {
					name: "other",
					deps: [":module"],
				}`),
		},
	},
}

func TestLicenses(t *testing.T) {
	for _, test := range licensesTests {
		t.Run(test.name, func(t *testing.T) {
			// Customize the common license text fixture factory.
			result := GroupFixturePreparers(
				prepareForLicenseTest,
				FixtureRegisterWithContext(func(ctx RegistrationContext) {
					ctx.RegisterModuleType("mock_bad_module", newMockLicensesBadModule)
					ctx.RegisterModuleType("mock_library", newMockLicensesLibraryModule)
					ctx.RegisterModuleType("mock_defaults", defaultsLicensesFactory)
				}),
				test.fs.AddToFixture(),
			).
				ExtendWithErrorHandler(FixtureExpectsAllErrorsToMatchAPattern(test.expectedErrors)).
				RunTest(t)

			if test.effectivePackage != nil {
				checkEffectivePackage(t, result, test.effectivePackage)
			}

			if test.effectiveNotices != nil {
				checkEffectiveNotices(t, result, test.effectiveNotices)
			}

			if test.effectiveKinds != nil {
				checkEffectiveKinds(t, result, test.effectiveKinds)
			}

			if test.effectiveConditions != nil {
				checkEffectiveConditions(t, result, test.effectiveConditions)
			}
		})
	}
}

func checkEffectivePackage(t *testing.T, result *TestResult, effectivePackage map[string]string) {
	actualPackage := make(map[string]string)
	result.Context.Context.VisitAllModules(func(m blueprint.Module) {
		if _, ok := m.(*licenseModule); ok {
			return
		}
		if _, ok := m.(*licenseKindModule); ok {
			return
		}
		if _, ok := m.(*packageModule); ok {
			return
		}
		module, ok := m.(Module)
		if !ok {
			t.Errorf("%q not a module", m.Name())
			return
		}
		base := module.base()
		if base == nil {
			return
		}

		if base.commonProperties.Effective_package_name == nil {
			actualPackage[m.Name()] = ""
		} else {
			actualPackage[m.Name()] = *base.commonProperties.Effective_package_name
		}
	})

	for moduleName, expectedPackage := range effectivePackage {
		packageName, ok := actualPackage[moduleName]
		if !ok {
			packageName = ""
		}
		if expectedPackage != packageName {
			t.Errorf("effective package mismatch for module %q: expected %q, found %q", moduleName, expectedPackage, packageName)
		}
	}
}

func checkEffectiveNotices(t *testing.T, result *TestResult, effectiveNotices map[string][]string) {
	actualNotices := make(map[string][]string)
	result.Context.Context.VisitAllModules(func(m blueprint.Module) {
		if _, ok := m.(*licenseModule); ok {
			return
		}
		if _, ok := m.(*licenseKindModule); ok {
			return
		}
		if _, ok := m.(*packageModule); ok {
			return
		}
		module, ok := m.(Module)
		if !ok {
			t.Errorf("%q not a module", m.Name())
			return
		}
		base := module.base()
		if base == nil {
			return
		}
		actualNotices[m.Name()] = base.commonProperties.Effective_license_text.Strings()
	})

	for moduleName, expectedNotices := range effectiveNotices {
		notices, ok := actualNotices[moduleName]
		if !ok {
			notices = []string{}
		}
		if !compareUnorderedStringArrays(expectedNotices, notices) {
			t.Errorf("effective notice files mismatch for module %q: expected %q, found %q", moduleName, expectedNotices, notices)
		}
	}
}

func checkEffectiveKinds(t *testing.T, result *TestResult, effectiveKinds map[string][]string) {
	actualKinds := make(map[string][]string)
	result.Context.Context.VisitAllModules(func(m blueprint.Module) {
		if _, ok := m.(*licenseModule); ok {
			return
		}
		if _, ok := m.(*licenseKindModule); ok {
			return
		}
		if _, ok := m.(*packageModule); ok {
			return
		}
		module, ok := m.(Module)
		if !ok {
			t.Errorf("%q not a module", m.Name())
			return
		}
		base := module.base()
		if base == nil {
			return
		}
		actualKinds[m.Name()] = base.commonProperties.Effective_license_kinds
	})

	for moduleName, expectedKinds := range effectiveKinds {
		kinds, ok := actualKinds[moduleName]
		if !ok {
			kinds = []string{}
		}
		if !compareUnorderedStringArrays(expectedKinds, kinds) {
			t.Errorf("effective license kinds mismatch for module %q: expected %q, found %q", moduleName, expectedKinds, kinds)
		}
	}
}

func checkEffectiveConditions(t *testing.T, result *TestResult, effectiveConditions map[string][]string) {
	actualConditions := make(map[string][]string)
	result.Context.Context.VisitAllModules(func(m blueprint.Module) {
		if _, ok := m.(*licenseModule); ok {
			return
		}
		if _, ok := m.(*licenseKindModule); ok {
			return
		}
		if _, ok := m.(*packageModule); ok {
			return
		}
		module, ok := m.(Module)
		if !ok {
			t.Errorf("%q not a module", m.Name())
			return
		}
		base := module.base()
		if base == nil {
			return
		}
		actualConditions[m.Name()] = base.commonProperties.Effective_license_conditions
	})

	for moduleName, expectedConditions := range effectiveConditions {
		conditions, ok := actualConditions[moduleName]
		if !ok {
			conditions = []string{}
		}
		if !compareUnorderedStringArrays(expectedConditions, conditions) {
			t.Errorf("effective license conditions mismatch for module %q: expected %q, found %q", moduleName, expectedConditions, conditions)
		}
	}
}

func compareUnorderedStringArrays(expected, actual []string) bool {
	if len(expected) != len(actual) {
		return false
	}
	s := make(map[string]int)
	for _, v := range expected {
		s[v] += 1
	}
	for _, v := range actual {
		c, ok := s[v]
		if !ok {
			return false
		}
		if c < 1 {
			return false
		}
		s[v] -= 1
	}
	return true
}

type mockLicensesBadProperties struct {
	Visibility []string
}

type mockLicensesBadModule struct {
	ModuleBase
	DefaultableModuleBase
	properties mockLicensesBadProperties
}

func newMockLicensesBadModule() Module {
	m := &mockLicensesBadModule{}

	base := m.base()
	m.AddProperties(&base.nameProperties, &m.properties)

	// The default_visibility property needs to be checked and parsed by the visibility module during
	// its checking and parsing phases so make it the primary visibility property.
	setPrimaryVisibilityProperty(m, "visibility", &m.properties.Visibility)

	initAndroidModuleBase(m)
	InitDefaultableModule(m)

	return m
}

func (m *mockLicensesBadModule) GenerateAndroidBuildActions(ModuleContext) {
}

type mockLicensesLibraryProperties struct {
	Deps []string
}

type mockLicensesLibraryModule struct {
	ModuleBase
	DefaultableModuleBase
	properties mockLicensesLibraryProperties
}

func newMockLicensesLibraryModule() Module {
	m := &mockLicensesLibraryModule{}
	m.AddProperties(&m.properties)
	InitAndroidArchModule(m, HostAndDeviceSupported, MultilibCommon)
	InitDefaultableModule(m)
	return m
}

type dependencyLicensesTag struct {
	blueprint.BaseDependencyTag
	name string
}

func (j *mockLicensesLibraryModule) DepsMutator(ctx BottomUpMutatorContext) {
	ctx.AddVariationDependencies(nil, dependencyLicensesTag{name: "mockdeps"}, j.properties.Deps...)
}

func (p *mockLicensesLibraryModule) GenerateAndroidBuildActions(ModuleContext) {
}

type mockLicensesDefaults struct {
	ModuleBase
	DefaultsModuleBase
}

func defaultsLicensesFactory() Module {
	m := &mockLicensesDefaults{}
	InitDefaultsModule(m)
	return m
}
