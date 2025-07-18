// Copyright 2017 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cc

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"android/soong/aidl_library"
	"android/soong/android"

	"github.com/google/blueprint"
)

func init() {
	registerTestMutators(android.InitRegistrationContext)
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

var prepareForCcTest = android.GroupFixturePreparers(
	PrepareForIntegrationTestWithCc,
)

var apexVariationName = "apex28"
var apexVersion = "28"

func registerTestMutators(ctx android.RegistrationContext) {
	ctx.PostDepsMutators(func(ctx android.RegisterMutatorsContext) {
		ctx.Transition("apex", &testApexTransitionMutator{})
	})
}

type testApexTransitionMutator struct{}

func (t *testApexTransitionMutator) Split(ctx android.BaseModuleContext) []string {
	return []string{apexVariationName}
}

func (t *testApexTransitionMutator) OutgoingTransition(ctx android.OutgoingTransitionContext, sourceVariation string) string {
	return sourceVariation
}

func (t *testApexTransitionMutator) IncomingTransition(ctx android.IncomingTransitionContext, incomingVariation string) string {
	return incomingVariation
}

func (t *testApexTransitionMutator) Mutate(ctx android.BottomUpMutatorContext, variation string) {
	apexInfo := android.ApexInfo{
		ApexVariationName: apexVariationName,
		MinSdkVersion:     android.ApiLevelForTest(apexVersion),
	}
	android.SetProvider(ctx, android.ApexInfoProvider, apexInfo)
}

// testCcWithConfig runs tests using the prepareForCcTest
//
// See testCc for an explanation as to how to stop using this deprecated method.
//
// deprecated
func testCcWithConfig(t *testing.T, config android.Config) *android.TestContext {
	t.Helper()
	result := prepareForCcTest.RunTestWithConfig(t, config)
	return result.TestContext
}

// testCc runs tests using the prepareForCcTest
//
// Do not add any new usages of this, instead use the prepareForCcTest directly as it makes it much
// easier to customize the test behavior.
//
// If it is necessary to customize the behavior of an existing test that uses this then please first
// convert the test to using prepareForCcTest first and then in a following change add the
// appropriate fixture preparers. Keeping the conversion change separate makes it easy to verify
// that it did not change the test behavior unexpectedly.
//
// deprecated
func testCc(t *testing.T, bp string) *android.TestContext {
	t.Helper()
	result := prepareForCcTest.RunTestWithBp(t, bp)
	return result.TestContext
}

// testCcErrorWithConfig runs tests using the prepareForCcTest
//
// See testCc for an explanation as to how to stop using this deprecated method.
//
// deprecated
func testCcErrorWithConfig(t *testing.T, pattern string, config android.Config) {
	t.Helper()

	prepareForCcTest.
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(pattern)).
		RunTestWithConfig(t, config)
}

// testCcError runs tests using the prepareForCcTest
//
// See testCc for an explanation as to how to stop using this deprecated method.
//
// deprecated
func testCcError(t *testing.T, pattern string, bp string) {
	t.Helper()
	config := TestConfig(t.TempDir(), android.Android, nil, bp, nil)
	testCcErrorWithConfig(t, pattern, config)
	return
}

const (
	coreVariant     = "android_arm64_armv8-a_shared"
	vendorVariant   = "android_vendor_arm64_armv8-a_shared"
	productVariant  = "android_product_arm64_armv8-a_shared"
	recoveryVariant = "android_recovery_arm64_armv8-a_shared"
)

// Test that the PrepareForTestWithCcDefaultModules provides all the files that it uses by
// running it in a fixture that requires all source files to exist.
func TestPrepareForTestWithCcDefaultModules(t *testing.T) {
	android.GroupFixturePreparers(
		PrepareForTestWithCcDefaultModules,
		android.PrepareForTestDisallowNonExistentPaths,
	).RunTest(t)
}

func TestVendorSrc(t *testing.T) {
	t.Parallel()
	ctx := testCc(t, `
		cc_library {
			name: "libTest",
			srcs: ["foo.c"],
			no_libcrt: true,
			nocrt: true,
			system_shared_libs: [],
			vendor_available: true,
			target: {
				vendor: {
					srcs: ["bar.c"],
				},
			},
		}
	`)

	ld := ctx.ModuleForTests(t, "libTest", vendorVariant).Rule("ld")
	var objs []string
	for _, o := range ld.Inputs {
		objs = append(objs, o.Base())
	}
	if len(objs) != 2 || objs[0] != "foo.o" || objs[1] != "bar.o" {
		t.Errorf("inputs of libTest must be []string{\"foo.o\", \"bar.o\"}, but was %#v.", objs)
	}
}

func checkInstallPartition(t *testing.T, ctx *android.TestContext, name, variant, expected string) {
	mod := ctx.ModuleForTests(t, name, variant).Module().(*Module)
	partitionDefined := false
	checkPartition := func(specific bool, partition string) {
		if specific {
			if expected != partition && !partitionDefined {
				// The variant is installed to the 'partition'
				t.Errorf("%s variant of %q must not be installed to %s partition", variant, name, partition)
			}
			partitionDefined = true
		} else {
			// The variant is not installed to the 'partition'
			if expected == partition {
				t.Errorf("%s variant of %q must be installed to %s partition", variant, name, partition)
			}
		}
	}
	socSpecific := func(m *Module) bool {
		return m.SocSpecific() || m.InstallInVendor()
	}
	deviceSpecific := func(m *Module) bool {
		return m.DeviceSpecific() || m.InstallInOdm()
	}
	productSpecific := func(m *Module) bool {
		return m.ProductSpecific() || m.InstallInProduct()
	}
	systemExtSpecific := func(m *Module) bool {
		return m.SystemExtSpecific()
	}
	checkPartition(socSpecific(mod), "vendor")
	checkPartition(deviceSpecific(mod), "odm")
	checkPartition(productSpecific(mod), "product")
	checkPartition(systemExtSpecific(mod), "system_ext")
	if !partitionDefined && expected != "system" {
		t.Errorf("%s variant of %q is expected to be installed to %s partition,"+
			" but installed to system partition", variant, name, expected)
	}
}

func TestInstallPartition(t *testing.T) {
	t.Parallel()
	t.Helper()
	ctx := prepareForCcTest.RunTestWithBp(t, `
		cc_library {
			name: "libsystem",
		}
		cc_library {
			name: "libsystem_ext",
			system_ext_specific: true,
		}
		cc_library {
			name: "libproduct",
			product_specific: true,
		}
		cc_library {
			name: "libvendor",
			vendor: true,
		}
		cc_library {
			name: "libodm",
			device_specific: true,
		}
		cc_library {
			name: "liball_available",
			vendor_available: true,
			product_available: true,
		}
		cc_library {
			name: "libsystem_ext_all_available",
			system_ext_specific: true,
			vendor_available: true,
			product_available: true,
		}
		cc_library {
			name: "liball_available_odm",
			odm_available: true,
			product_available: true,
		}
		cc_library {
			name: "libproduct_vendoravailable",
			product_specific: true,
			vendor_available: true,
		}
		cc_library {
			name: "libproduct_odmavailable",
			product_specific: true,
			odm_available: true,
		}
	`).TestContext

	checkInstallPartition(t, ctx, "libsystem", coreVariant, "system")
	checkInstallPartition(t, ctx, "libsystem_ext", coreVariant, "system_ext")
	checkInstallPartition(t, ctx, "libproduct", productVariant, "product")
	checkInstallPartition(t, ctx, "libvendor", vendorVariant, "vendor")
	checkInstallPartition(t, ctx, "libodm", vendorVariant, "odm")

	checkInstallPartition(t, ctx, "liball_available", coreVariant, "system")
	checkInstallPartition(t, ctx, "liball_available", productVariant, "product")
	checkInstallPartition(t, ctx, "liball_available", vendorVariant, "vendor")

	checkInstallPartition(t, ctx, "libsystem_ext_all_available", coreVariant, "system_ext")
	checkInstallPartition(t, ctx, "libsystem_ext_all_available", productVariant, "product")
	checkInstallPartition(t, ctx, "libsystem_ext_all_available", vendorVariant, "vendor")

	checkInstallPartition(t, ctx, "liball_available_odm", coreVariant, "system")
	checkInstallPartition(t, ctx, "liball_available_odm", productVariant, "product")
	checkInstallPartition(t, ctx, "liball_available_odm", vendorVariant, "odm")

	checkInstallPartition(t, ctx, "libproduct_vendoravailable", productVariant, "product")
	checkInstallPartition(t, ctx, "libproduct_vendoravailable", vendorVariant, "vendor")

	checkInstallPartition(t, ctx, "libproduct_odmavailable", productVariant, "product")
	checkInstallPartition(t, ctx, "libproduct_odmavailable", vendorVariant, "odm")
}

func checkWriteFileOutput(t *testing.T, ctx *android.TestContext, params android.TestingBuildParams, expected []string) {
	t.Helper()
	content := android.ContentFromFileRuleForTests(t, ctx, params)
	actual := strings.FieldsFunc(content, func(r rune) bool { return r == '\n' })
	assertArrayString(t, actual, expected)
}

func TestDataLibs(t *testing.T) {
	t.Parallel()
	bp := `
		cc_test_library {
			name: "test_lib",
			srcs: ["test_lib.cpp"],
			gtest: false,
		}

		cc_test {
			name: "main_test",
			data_libs: ["test_lib"],
			gtest: false,
		}
 `

	config := TestConfig(t.TempDir(), android.Android, nil, bp, nil)

	ctx := testCcWithConfig(t, config)
	testingModule := ctx.ModuleForTests(t, "main_test", "android_arm_armv7-a-neon")
	testBinary := testingModule.Module().(*Module).linker.(*testBinary)
	outputFiles := testingModule.OutputFiles(ctx, t, "")
	if len(outputFiles) != 1 {
		t.Errorf("expected exactly one output file. output files: [%s]", outputFiles)
		return
	}
	if len(testBinary.dataPaths()) != 1 {
		t.Errorf("expected exactly one test data file. test data files: [%v]", testBinary.dataPaths())
		return
	}

	outputPath := outputFiles[0].String()
	testBinaryPath := testBinary.dataPaths()[0].SrcPath.String()

	if !strings.HasSuffix(outputPath, "/main_test") {
		t.Errorf("expected test output file to be 'main_test', but was '%s'", outputPath)
		return
	}
	if !strings.HasSuffix(testBinaryPath, "/test_lib.so") {
		t.Errorf("expected test data file to be 'test_lib.so', but was '%s'", testBinaryPath)
		return
	}
}

func TestDataLibsRelativeInstallPath(t *testing.T) {
	t.Parallel()
	bp := `
		cc_test_library {
			name: "test_lib",
			srcs: ["test_lib.cpp"],
			relative_install_path: "foo/bar/baz",
			gtest: false,
		}

		cc_binary {
			name: "test_bin",
			relative_install_path: "foo/bar/baz",
			compile_multilib: "both",
		}

		cc_test {
			name: "main_test",
			data_libs: ["test_lib"],
			data_bins: ["test_bin"],
			gtest: false,
		}
 `

	config := TestConfig(t.TempDir(), android.Android, nil, bp, nil)

	ctx := testCcWithConfig(t, config)
	testingModule := ctx.ModuleForTests(t, "main_test", "android_arm_armv7-a-neon")
	module := testingModule.Module()
	testBinary := module.(*Module).linker.(*testBinary)
	outputFiles := testingModule.OutputFiles(ctx, t, "")
	if len(outputFiles) != 1 {
		t.Fatalf("expected exactly one output file. output files: [%s]", outputFiles)
	}
	if len(testBinary.dataPaths()) != 2 {
		t.Fatalf("expected exactly one test data file. test data files: [%v]", testBinary.dataPaths())
	}

	outputPath := outputFiles[0].String()

	if !strings.HasSuffix(outputPath, "/main_test") {
		t.Errorf("expected test output file to be 'main_test', but was '%s'", outputPath)
	}
	entries := android.AndroidMkInfoForTest(t, ctx, module).PrimaryInfo
	if !strings.HasSuffix(entries.EntryMap["LOCAL_TEST_DATA"][0], ":test_lib.so:foo/bar/baz") {
		t.Errorf("expected LOCAL_TEST_DATA to end with `:test_lib.so:foo/bar/baz`,"+
			" but was '%s'", entries.EntryMap["LOCAL_TEST_DATA"][0])
	}
	if !strings.HasSuffix(entries.EntryMap["LOCAL_TEST_DATA"][1], ":test_bin:foo/bar/baz") {
		t.Errorf("expected LOCAL_TEST_DATA to end with `:test_bin:foo/bar/baz`,"+
			" but was '%s'", entries.EntryMap["LOCAL_TEST_DATA"][1])
	}
}

func TestTestBinaryTestSuites(t *testing.T) {
	t.Parallel()
	bp := `
		cc_test {
			name: "main_test",
			srcs: ["main_test.cpp"],
			test_suites: [
				"suite_1",
				"suite_2",
			],
			gtest: false,
		}
	`

	ctx := prepareForCcTest.RunTestWithBp(t, bp).TestContext
	module := ctx.ModuleForTests(t, "main_test", "android_arm_armv7-a-neon").Module()

	entries := android.AndroidMkInfoForTest(t, ctx, module).PrimaryInfo
	compatEntries := entries.EntryMap["LOCAL_COMPATIBILITY_SUITE"]
	if len(compatEntries) != 2 {
		t.Errorf("expected two elements in LOCAL_COMPATIBILITY_SUITE. got %d", len(compatEntries))
	}
	if compatEntries[0] != "suite_1" {
		t.Errorf("expected LOCAL_COMPATIBILITY_SUITE to be`suite_1`,"+
			" but was '%s'", compatEntries[0])
	}
	if compatEntries[1] != "suite_2" {
		t.Errorf("expected LOCAL_COMPATIBILITY_SUITE to be`suite_2`,"+
			" but was '%s'", compatEntries[1])
	}
}

func TestTestLibraryTestSuites(t *testing.T) {
	t.Parallel()
	bp := `
		cc_test_library {
			name: "main_test_lib",
			srcs: ["main_test_lib.cpp"],
			test_suites: [
				"suite_1",
				"suite_2",
			],
			gtest: false,
		}
	`

	ctx := prepareForCcTest.RunTestWithBp(t, bp).TestContext
	module := ctx.ModuleForTests(t, "main_test_lib", "android_arm_armv7-a-neon_shared").Module()

	entries := android.AndroidMkInfoForTest(t, ctx, module).PrimaryInfo
	compatEntries := entries.EntryMap["LOCAL_COMPATIBILITY_SUITE"]
	if len(compatEntries) != 2 {
		t.Errorf("expected two elements in LOCAL_COMPATIBILITY_SUITE. got %d", len(compatEntries))
	}
	if compatEntries[0] != "suite_1" {
		t.Errorf("expected LOCAL_COMPATIBILITY_SUITE to be`suite_1`,"+
			" but was '%s'", compatEntries[0])
	}
	if compatEntries[1] != "suite_2" {
		t.Errorf("expected LOCAL_COMPATIBILITY_SUITE to be`suite_2`,"+
			" but was '%s'", compatEntries[1])
	}
}

func TestDoubleLoadbleDep(t *testing.T) {
	t.Parallel()
	// okay to link : LLNDK -> double_loadable
	testCc(t, `
		cc_library {
			name: "libllndk",
			shared_libs: ["libdoubleloadable"],
			llndk: {
				symbol_file: "libllndk.map.txt",
			}
		}

		cc_library {
			name: "libdoubleloadable",
			vendor_available: true,
			product_available: true,
			double_loadable: true,
		}
	`)
	// okay to link : double_loadable -> double_loadable
	testCc(t, `
		cc_library {
			name: "libdoubleloadable1",
			shared_libs: ["libdoubleloadable2"],
			vendor_available: true,
			double_loadable: true,
		}

		cc_library {
			name: "libdoubleloadable2",
			vendor_available: true,
			double_loadable: true,
		}
	`)
	// okay to link : double_loadable -> double_loadable
	testCc(t, `
		cc_library {
			name: "libdoubleloadable",
			vendor_available: true,
			product_available: true,
			double_loadable: true,
			shared_libs: ["libnondoubleloadable"],
		}

		cc_library {
			name: "libnondoubleloadable",
			vendor_available: true,
			product_available: true,
			double_loadable: true,
		}
	`)
	// okay to link : LLNDK -> core-only -> vendor_available & double_loadable
	testCc(t, `
		cc_library {
			name: "libllndk",
			shared_libs: ["libcoreonly"],
			llndk: {
				symbol_file: "libllndk.map.txt",
			}
		}

		cc_library {
			name: "libcoreonly",
			shared_libs: ["libvendoravailable"],
		}

		// indirect dependency of LLNDK
		cc_library {
			name: "libvendoravailable",
			vendor_available: true,
			double_loadable: true,
		}
	`)
}

func TestDoubleLoadableDepError(t *testing.T) {
	t.Parallel()
	// Check whether an error is emitted when a LLNDK depends on a non-double_loadable lib.
	testCcError(t, "module \".*\" variant \".*\": link.* \".*\" which is not LL-NDK, VNDK-SP, .*double_loadable", `
		cc_library {
			name: "libllndk",
			shared_libs: ["libnondoubleloadable"],
			llndk: {
				symbol_file: "libllndk.map.txt",
			}
		}

		cc_library {
			name: "libnondoubleloadable",
			vendor_available: true,
			product_available: true,
		}
	`)

	// Check whether an error is emitted when a LLNDK depends on a non-double_loadable vendor_available lib.
	testCcError(t, "module \".*\" variant \".*\": link.* \".*\" which is not LL-NDK, VNDK-SP, .*double_loadable", `
		cc_library {
			name: "libllndk",
			no_libcrt: true,
			shared_libs: ["libnondoubleloadable"],
			llndk: {
				symbol_file: "libllndk.map.txt",
			}
		}

		cc_library {
			name: "libnondoubleloadable",
			vendor_available: true,
		}
	`)

	// Check whether an error is emitted when a LLNDK depends on a non-double_loadable indirectly.
	testCcError(t, "module \".*\" variant \".*\": link.* \".*\" which is not LL-NDK, VNDK-SP, .*double_loadable", `
		cc_library {
			name: "libllndk",
			shared_libs: ["libcoreonly"],
			llndk: {
				symbol_file: "libllndk.map.txt",
			}
		}

		cc_library {
			name: "libcoreonly",
			shared_libs: ["libvendoravailable"],
		}

		// indirect dependency of LLNDK
		cc_library {
			name: "libvendoravailable",
			vendor_available: true,
		}
	`)

	// The error is not from 'client' but from 'libllndk'
	testCcError(t, "module \"libllndk\".* links a library \"libnondoubleloadable\".*double_loadable", `
		cc_library {
			name: "client",
			vendor_available: true,
			double_loadable: true,
			shared_libs: ["libllndk"],
		}
		cc_library {
			name: "libllndk",
			shared_libs: ["libnondoubleloadable"],
			llndk: {
				symbol_file: "libllndk.map.txt",
			}
		}
		cc_library {
			name: "libnondoubleloadable",
			vendor_available: true,
		}
	`)
}

func TestMakeLinkType(t *testing.T) {
	t.Parallel()
	bp := `
		cc_library {
			name: "libvendor",
			vendor: true,
		}
		vndk_prebuilt_shared {
			name: "prevndk",
			version: "27",
			target_arch: "arm",
			binder32bit: true,
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			arch: {
				arm: {
					srcs: ["liba.so"],
				},
			},
		}
		cc_library {
			name: "libllndk",
			llndk: {
				symbol_file: "libllndk.map.txt",
			}
		}
		cc_library {
			name: "libllndkprivate",
			llndk: {
				symbol_file: "libllndkprivate.map.txt",
				private: true,
			}
		}
		llndk_libraries_txt {
			name: "llndk.libraries.txt",
		}
	`

	config := TestConfig(t.TempDir(), android.Android, nil, bp, nil)
	// native:vndk
	ctx := testCcWithConfig(t, config)

	vendorVariant27 := "android_vendor.27_arm64_armv8-a_shared"

	tests := []struct {
		variant  string
		name     string
		expected string
	}{
		{vendorVariant, "libvendor", "native:vendor"},
		{vendorVariant, "libllndk", "native:vndk"},
		{vendorVariant27, "prevndk.vndk.27.arm.binder32", "native:vendor"},
		{coreVariant, "libllndk", "native:platform"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			module := ctx.ModuleForTests(t, test.name, test.variant).Module().(*Module)
			assertString(t, module.makeLinkType, test.expected)
		})
	}
}

var staticLinkDepOrderTestCases = []struct {
	// This is a string representation of a map[moduleName][]moduleDependency .
	// It models the dependencies declared in an Android.bp file.
	inStatic string

	// This is a string representation of a map[moduleName][]moduleDependency .
	// It models the dependencies declared in an Android.bp file.
	inShared string

	// allOrdered is a string representation of a map[moduleName][]moduleDependency .
	// The keys of allOrdered specify which modules we would like to check.
	// The values of allOrdered specify the expected result (of the transitive closure of all
	// dependencies) for each module to test
	allOrdered string

	// outOrdered is a string representation of a map[moduleName][]moduleDependency .
	// The keys of outOrdered specify which modules we would like to check.
	// The values of outOrdered specify the expected result (of the ordered linker command line)
	// for each module to test.
	outOrdered string
}{
	// Simple tests
	{
		inStatic:   "",
		outOrdered: "",
	},
	{
		inStatic:   "a:",
		outOrdered: "a:",
	},
	{
		inStatic:   "a:b; b:",
		outOrdered: "a:b; b:",
	},
	// Tests of reordering
	{
		// diamond example
		inStatic:   "a:d,b,c; b:d; c:d; d:",
		outOrdered: "a:b,c,d; b:d; c:d; d:",
	},
	{
		// somewhat real example
		inStatic:   "bsdiff_unittest:b,c,d,e,f,g,h,i; e:b",
		outOrdered: "bsdiff_unittest:c,d,e,b,f,g,h,i; e:b",
	},
	{
		// multiple reorderings
		inStatic:   "a:b,c,d,e; d:b; e:c",
		outOrdered: "a:d,b,e,c; d:b; e:c",
	},
	{
		// should reorder without adding new transitive dependencies
		inStatic:   "bin:lib2,lib1;             lib1:lib2,liboptional",
		allOrdered: "bin:lib1,lib2,liboptional; lib1:lib2,liboptional",
		outOrdered: "bin:lib1,lib2;             lib1:lib2,liboptional",
	},
	{
		// multiple levels of dependencies
		inStatic:   "a:b,c,d,e,f,g,h; f:b,c,d; b:c,d; c:d",
		allOrdered: "a:e,f,b,c,d,g,h; f:b,c,d; b:c,d; c:d",
		outOrdered: "a:e,f,b,c,d,g,h; f:b,c,d; b:c,d; c:d",
	},
	// shared dependencies
	{
		// Note that this test doesn't recurse, to minimize the amount of logic it tests.
		// So, we don't actually have to check that a shared dependency of c will change the order
		// of a library that depends statically on b and on c.  We only need to check that if c has
		// a shared dependency on b, that that shows up in allOrdered.
		inShared:   "c:b",
		allOrdered: "c:b",
		outOrdered: "c:",
	},
	{
		// This test doesn't actually include any shared dependencies but it's a reminder of what
		// the second phase of the above test would look like
		inStatic:   "a:b,c; c:b",
		allOrdered: "a:c,b; c:b",
		outOrdered: "a:c,b; c:b",
	},
	// tiebreakers for when two modules specifying different orderings and there is no dependency
	// to dictate an order
	{
		// if the tie is between two modules at the end of a's deps, then a's order wins
		inStatic:   "a1:b,c,d,e; a2:b,c,e,d; b:d,e; c:e,d",
		outOrdered: "a1:b,c,d,e; a2:b,c,e,d; b:d,e; c:e,d",
	},
	{
		// if the tie is between two modules at the start of a's deps, then c's order is used
		inStatic:   "a1:d,e,b1,c1; b1:d,e; c1:e,d;   a2:d,e,b2,c2; b2:d,e; c2:d,e",
		outOrdered: "a1:b1,c1,e,d; b1:d,e; c1:e,d;   a2:b2,c2,d,e; b2:d,e; c2:d,e",
	},
	// Tests involving duplicate dependencies
	{
		// simple duplicate
		inStatic:   "a:b,c,c,b",
		outOrdered: "a:c,b",
	},
	{
		// duplicates with reordering
		inStatic:   "a:b,c,d,c; c:b",
		outOrdered: "a:d,c,b",
	},
	// Tests to confirm the nonexistence of infinite loops.
	// These cases should never happen, so as long as the test terminates and the
	// result is deterministic then that should be fine.
	{
		inStatic:   "a:a",
		outOrdered: "a:a",
	},
	{
		inStatic:   "a:b;   b:c;   c:a",
		allOrdered: "a:b,c; b:c,a; c:a,b",
		outOrdered: "a:b;   b:c;   c:a",
	},
	{
		inStatic:   "a:b,c;   b:c,a;   c:a,b",
		allOrdered: "a:c,a,b; b:a,b,c; c:b,c,a",
		outOrdered: "a:c,b;   b:a,c;   c:b,a",
	},
}

// converts from a string like "a:b,c; d:e" to (["a","b"], {"a":["b","c"], "d":["e"]}, [{"a", "a.o"}, {"b", "b.o"}])
func parseModuleDeps(text string) (modulesInOrder []android.Path, allDeps map[android.Path][]android.Path) {
	// convert from "a:b,c; d:e" to "a:b,c;d:e"
	strippedText := strings.Replace(text, " ", "", -1)
	if len(strippedText) < 1 {
		return []android.Path{}, make(map[android.Path][]android.Path, 0)
	}
	allDeps = make(map[android.Path][]android.Path, 0)

	// convert from "a:b,c;d:e" to ["a:b,c", "d:e"]
	moduleTexts := strings.Split(strippedText, ";")

	outputForModuleName := func(moduleName string) android.Path {
		return android.PathForTesting(moduleName)
	}

	for _, moduleText := range moduleTexts {
		// convert from "a:b,c" to ["a", "b,c"]
		components := strings.Split(moduleText, ":")
		if len(components) != 2 {
			panic(fmt.Sprintf("illegal module dep string %q from larger string %q; must contain one ':', not %v", moduleText, text, len(components)-1))
		}
		moduleName := components[0]
		moduleOutput := outputForModuleName(moduleName)
		modulesInOrder = append(modulesInOrder, moduleOutput)

		depString := components[1]
		// convert from "b,c" to ["b", "c"]
		depNames := strings.Split(depString, ",")
		if len(depString) < 1 {
			depNames = []string{}
		}
		var deps []android.Path
		for _, depName := range depNames {
			deps = append(deps, outputForModuleName(depName))
		}
		allDeps[moduleOutput] = deps
	}
	return modulesInOrder, allDeps
}

func TestStaticLibDepReordering(t *testing.T) {
	t.Parallel()
	ctx := testCc(t, `
	cc_library {
		name: "a",
		static_libs: ["b", "c", "d"],
		stl: "none",
	}
	cc_library {
		name: "b",
		stl: "none",
	}
	cc_library {
		name: "c",
		static_libs: ["b"],
		stl: "none",
	}
	cc_library {
		name: "d",
		stl: "none",
	}

	`)

	variant := "android_arm64_armv8-a_static"
	moduleA := ctx.ModuleForTests(t, "a", variant).Module().(*Module)
	staticLibInfo, _ := android.OtherModuleProvider(ctx, moduleA, StaticLibraryInfoProvider)
	actual := android.Paths(staticLibInfo.TransitiveStaticLibrariesForOrdering.ToList()).RelativeToTop()
	expected := GetOutputPaths(t, ctx, variant, []string{"a", "c", "b", "d"})

	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("staticDeps orderings were not propagated correctly"+
			"\nactual:   %v"+
			"\nexpected: %v",
			actual,
			expected,
		)
	}
}

func TestStaticLibDepReorderingWithShared(t *testing.T) {
	t.Parallel()
	ctx := testCc(t, `
	cc_library {
		name: "a",
		static_libs: ["b", "c"],
		stl: "none",
	}
	cc_library {
		name: "b",
		stl: "none",
	}
	cc_library {
		name: "c",
		shared_libs: ["b"],
		stl: "none",
	}

	`)

	variant := "android_arm64_armv8-a_static"
	moduleA := ctx.ModuleForTests(t, "a", variant).Module().(*Module)
	staticLibInfo, _ := android.OtherModuleProvider(ctx, moduleA, StaticLibraryInfoProvider)
	actual := android.Paths(staticLibInfo.TransitiveStaticLibrariesForOrdering.ToList()).RelativeToTop()
	expected := GetOutputPaths(t, ctx, variant, []string{"a", "c", "b"})

	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("staticDeps orderings did not account for shared libs"+
			"\nactual:   %v"+
			"\nexpected: %v",
			actual,
			expected,
		)
	}
}

func checkEquals(t *testing.T, message string, expected, actual interface{}) {
	t.Helper()
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf(message+
			"\nactual:   %v"+
			"\nexpected: %v",
			actual,
			expected,
		)
	}
}

func TestLlndkLibrary(t *testing.T) {
	t.Parallel()
	result := prepareForCcTest.RunTestWithBp(t, `
	cc_library {
		name: "libllndk",
		stubs: { versions: ["1", "2"] },
		llndk: {
			symbol_file: "libllndk.map.txt",
		},
		export_include_dirs: ["include"],
	}

	cc_prebuilt_library_shared {
		name: "libllndkprebuilt",
		stubs: { versions: ["1", "2"] , symbol_file: "libllndkprebuilt.map.txt" },
		llndk: {
			symbol_file: "libllndkprebuilt.map.txt",
		},
	}

	cc_library {
		name: "libllndk_with_external_headers",
		stubs: { versions: ["1", "2"] },
		llndk: {
			symbol_file: "libllndk.map.txt",
			export_llndk_headers: ["libexternal_llndk_headers"],
		},
		header_libs: ["libexternal_headers"],
		export_header_lib_headers: ["libexternal_headers"],
	}
	cc_library_headers {
		name: "libexternal_headers",
		export_include_dirs: ["include"],
		vendor_available: true,
		product_available: true,
	}
	cc_library_headers {
		name: "libexternal_llndk_headers",
		export_include_dirs: ["include_llndk"],
		export_system_include_dirs: ["include_system_llndk"],
		llndk: {
			symbol_file: "libllndk.map.txt",
		},
		vendor_available: true,
	}

	cc_library {
		name: "libllndk_with_override_headers",
		stubs: { versions: ["1", "2"] },
		llndk: {
			symbol_file: "libllndk.map.txt",
			override_export_include_dirs: ["include_llndk"],
		},
		export_include_dirs: ["include"],
	}

	cc_library {
		name: "libllndk_with_system_headers",
		llndk: {
			symbol_file: "libllndk.map.txt",
			export_llndk_headers: ["libexternal_llndk_headers"],
			export_headers_as_system: true,
		},
		export_include_dirs: ["include"],
		export_system_include_dirs: ["include_system"],
	}
	`)
	actual := result.ModuleVariantsForTests("libllndk")
	for i := 0; i < len(actual); i++ {
		if !strings.HasPrefix(actual[i], "android_vendor_") {
			actual = append(actual[:i], actual[i+1:]...)
			i--
		}
	}
	expected := []string{
		"android_vendor_arm64_armv8-a_shared",
		"android_vendor_arm_armv7-a-neon_shared",
	}
	android.AssertArrayString(t, "variants for llndk stubs", expected, actual)

	params := result.ModuleForTests(t, "libllndk", "android_vendor_arm_armv7-a-neon_shared").Description("generate stub")
	android.AssertSame(t, "use Vendor API level for default stubs", "35", params.Args["apiLevel"])

	checkExportedIncludeDirs := func(module, variant string, expectedSystemDirs []string, expectedDirs ...string) {
		t.Helper()
		m := result.ModuleForTests(t, module, variant).Module()
		f, _ := android.OtherModuleProvider(result, m, FlagExporterInfoProvider)
		android.AssertPathsRelativeToTopEquals(t, "exported include dirs for "+module+"["+variant+"]",
			expectedDirs, f.IncludeDirs)
		android.AssertPathsRelativeToTopEquals(t, "exported include dirs for "+module+"["+variant+"]",
			expectedSystemDirs, f.SystemIncludeDirs)
	}

	checkExportedIncludeDirs("libllndk", coreVariant, nil, "include")
	checkExportedIncludeDirs("libllndk", vendorVariant, nil, "include")
	checkExportedIncludeDirs("libllndk_with_external_headers", coreVariant, nil, "include")
	checkExportedIncludeDirs("libllndk_with_external_headers", vendorVariant,
		[]string{"include_system_llndk"}, "include_llndk")
	checkExportedIncludeDirs("libllndk_with_override_headers", coreVariant, nil, "include")
	checkExportedIncludeDirs("libllndk_with_override_headers", vendorVariant, nil, "include_llndk")
	checkExportedIncludeDirs("libllndk_with_system_headers", coreVariant, []string{"include_system"}, "include")
	checkExportedIncludeDirs("libllndk_with_system_headers", vendorVariant,
		[]string{"include_system", "include", "include_system_llndk"}, "include_llndk")

	checkAbiLinkerIncludeDirs := func(module string) {
		t.Helper()
		coreModule := result.ModuleForTests(t, module, coreVariant)
		abiCheckFlags := ""
		for _, output := range coreModule.AllOutputs() {
			if strings.HasSuffix(output, ".so.llndk.lsdump") {
				abiCheckFlags = coreModule.Output(output).Args["exportedHeaderFlags"]
			}
		}
		vendorModule := result.ModuleForTests(t, module, vendorVariant).Module()
		vendorInfo, _ := android.OtherModuleProvider(result, vendorModule, FlagExporterInfoProvider)
		vendorDirs := android.Concat(vendorInfo.IncludeDirs, vendorInfo.SystemIncludeDirs)
		android.AssertStringEquals(t, module+" has different exported include dirs for vendor variant and ABI check",
			android.JoinPathsWithPrefix(vendorDirs, "-I"), abiCheckFlags)
	}
	checkAbiLinkerIncludeDirs("libllndk")
	checkAbiLinkerIncludeDirs("libllndk_with_override_headers")
	checkAbiLinkerIncludeDirs("libllndk_with_external_headers")
	checkAbiLinkerIncludeDirs("libllndk_with_system_headers")
}

func TestLlndkHeaders(t *testing.T) {
	t.Parallel()
	ctx := testCc(t, `
	cc_library_headers {
		name: "libllndk_headers",
		export_include_dirs: ["my_include"],
		llndk: {
			llndk_headers: true,
		},
	}
	cc_library {
		name: "libllndk",
		llndk: {
			symbol_file: "libllndk.map.txt",
			export_llndk_headers: ["libllndk_headers"],
		}
	}

	cc_library {
		name: "libvendor",
		shared_libs: ["libllndk"],
		vendor: true,
		srcs: ["foo.c"],
		no_libcrt: true,
		nocrt: true,
	}
	`)

	// _static variant is used since _shared reuses *.o from the static variant
	cc := ctx.ModuleForTests(t, "libvendor", "android_vendor_arm_armv7-a-neon_static").Rule("cc")
	cflags := cc.Args["cFlags"]
	if !strings.Contains(cflags, "-Imy_include") {
		t.Errorf("cflags for libvendor must contain -Imy_include, but was %#v.", cflags)
	}
}

func checkRuntimeLibs(t *testing.T, expected []string, module *Module) {
	actual := module.Properties.AndroidMkRuntimeLibs
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("incorrect runtime_libs for shared libs"+
			"\nactual:   %v"+
			"\nexpected: %v",
			actual,
			expected,
		)
	}
}

const runtimeLibAndroidBp = `
	cc_library {
		name: "liball_available",
		vendor_available: true,
		product_available: true,
		no_libcrt : true,
		nocrt : true,
		system_shared_libs : [],
	}
	cc_library {
		name: "libvendor_available1",
		vendor_available: true,
		runtime_libs: ["liball_available"],
		no_libcrt : true,
		nocrt : true,
		system_shared_libs : [],
	}
	cc_library {
		name: "libvendor_available2",
		vendor_available: true,
		runtime_libs: ["liball_available"],
		target: {
			vendor: {
				exclude_runtime_libs: ["liball_available"],
			}
		},
		no_libcrt : true,
		nocrt : true,
		system_shared_libs : [],
	}
	cc_library {
		name: "libproduct_vendor",
		product_specific: true,
		vendor_available: true,
		no_libcrt : true,
		nocrt : true,
		system_shared_libs : [],
	}
	cc_library {
		name: "libcore",
		runtime_libs: ["liball_available"],
		no_libcrt : true,
		nocrt : true,
		system_shared_libs : [],
	}
	cc_library {
		name: "libvendor1",
		vendor: true,
		no_libcrt : true,
		nocrt : true,
		system_shared_libs : [],
	}
	cc_library {
		name: "libvendor2",
		vendor: true,
		runtime_libs: ["liball_available", "libvendor1", "libproduct_vendor"],
		no_libcrt : true,
		nocrt : true,
		system_shared_libs : [],
	}
	cc_library {
		name: "libproduct_available1",
		product_available: true,
		runtime_libs: ["liball_available"],
		no_libcrt : true,
		nocrt : true,
		system_shared_libs : [],
	}
	cc_library {
		name: "libproduct1",
		product_specific: true,
		no_libcrt : true,
		nocrt : true,
		system_shared_libs : [],
	}
	cc_library {
		name: "libproduct2",
		product_specific: true,
		runtime_libs: ["liball_available", "libproduct1", "libproduct_vendor"],
		no_libcrt : true,
		nocrt : true,
		system_shared_libs : [],
	}
`

func TestRuntimeLibs(t *testing.T) {
	t.Parallel()
	ctx := testCc(t, runtimeLibAndroidBp)

	// runtime_libs for core variants use the module names without suffixes.
	variant := "android_arm64_armv8-a_shared"

	module := ctx.ModuleForTests(t, "libvendor_available1", variant).Module().(*Module)
	checkRuntimeLibs(t, []string{"liball_available"}, module)

	module = ctx.ModuleForTests(t, "libproduct_available1", variant).Module().(*Module)
	checkRuntimeLibs(t, []string{"liball_available"}, module)

	module = ctx.ModuleForTests(t, "libcore", variant).Module().(*Module)
	checkRuntimeLibs(t, []string{"liball_available"}, module)

	// runtime_libs for vendor variants have '.vendor' suffixes if the modules have both core
	// and vendor variants.
	variant = "android_vendor_arm64_armv8-a_shared"

	module = ctx.ModuleForTests(t, "libvendor_available1", variant).Module().(*Module)
	checkRuntimeLibs(t, []string{"liball_available.vendor"}, module)

	module = ctx.ModuleForTests(t, "libvendor2", variant).Module().(*Module)
	checkRuntimeLibs(t, []string{"liball_available.vendor", "libvendor1", "libproduct_vendor.vendor"}, module)

	// runtime_libs for product variants have '.product' suffixes if the modules have both core
	// and product variants.
	variant = "android_product_arm64_armv8-a_shared"

	module = ctx.ModuleForTests(t, "libproduct_available1", variant).Module().(*Module)
	checkRuntimeLibs(t, []string{"liball_available.product"}, module)

	module = ctx.ModuleForTests(t, "libproduct2", variant).Module().(*Module)
	checkRuntimeLibs(t, []string{"liball_available.product", "libproduct1", "libproduct_vendor"}, module)
}

func TestExcludeRuntimeLibs(t *testing.T) {
	t.Parallel()
	ctx := testCc(t, runtimeLibAndroidBp)

	variant := "android_arm64_armv8-a_shared"
	module := ctx.ModuleForTests(t, "libvendor_available2", variant).Module().(*Module)
	checkRuntimeLibs(t, []string{"liball_available"}, module)

	variant = "android_vendor_arm64_armv8-a_shared"
	module = ctx.ModuleForTests(t, "libvendor_available2", variant).Module().(*Module)
	checkRuntimeLibs(t, nil, module)
}

func checkStaticLibs(t *testing.T, expected []string, module *Module) {
	t.Helper()
	actual := module.Properties.AndroidMkStaticLibs
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("incorrect static_libs"+
			"\nactual:   %v"+
			"\nexpected: %v",
			actual,
			expected,
		)
	}
}

const staticLibAndroidBp = `
	cc_library {
		name: "lib1",
	}
	cc_library {
		name: "lib2",
		static_libs: ["lib1"],
	}
`

func TestStaticLibDepExport(t *testing.T) {
	t.Parallel()
	ctx := testCc(t, staticLibAndroidBp)

	// Check the shared version of lib2.
	variant := "android_arm64_armv8-a_shared"
	module := ctx.ModuleForTests(t, "lib2", variant).Module().(*Module)
	checkStaticLibs(t, []string{"lib1", "libc++demangle", "libclang_rt.builtins"}, module)

	// Check the static version of lib2.
	variant = "android_arm64_armv8-a_static"
	module = ctx.ModuleForTests(t, "lib2", variant).Module().(*Module)
	// libc++_static is linked additionally.
	checkStaticLibs(t, []string{"lib1", "libc++_static", "libc++demangle", "libclang_rt.builtins"}, module)
}

var compilerFlagsTestCases = []struct {
	in  string
	out bool
}{
	{
		in:  "a",
		out: false,
	},
	{
		in:  "-a",
		out: true,
	},
	{
		in:  "-Ipath/to/something",
		out: false,
	},
	{
		in:  "-isystempath/to/something",
		out: false,
	},
	{
		in:  "--coverage",
		out: false,
	},
	{
		in:  "-include a/b",
		out: true,
	},
	{
		in:  "-include a/b c/d",
		out: false,
	},
	{
		in:  "-DMACRO",
		out: true,
	},
	{
		in:  "-DMAC RO",
		out: false,
	},
	{
		in:  "-a -b",
		out: false,
	},
	{
		in:  "-DMACRO=definition",
		out: true,
	},
	{
		in:  "-DMACRO=defi nition",
		out: true, // TODO(jiyong): this should be false
	},
	{
		in:  "-DMACRO(x)=x + 1",
		out: true,
	},
	{
		in:  "-DMACRO=\"defi nition\"",
		out: true,
	},
}

type mockContext struct {
	BaseModuleContext
	result bool
}

func (ctx *mockContext) PropertyErrorf(property, format string, args ...interface{}) {
	// CheckBadCompilerFlags calls this function when the flag should be rejected
	ctx.result = false
}

func TestCompilerFlags(t *testing.T) {
	t.Parallel()
	for _, testCase := range compilerFlagsTestCases {
		ctx := &mockContext{result: true}
		CheckBadCompilerFlags(ctx, "", []string{testCase.in})
		if ctx.result != testCase.out {
			t.Errorf("incorrect output:")
			t.Errorf("     input: %#v", testCase.in)
			t.Errorf("  expected: %#v", testCase.out)
			t.Errorf("       got: %#v", ctx.result)
		}
	}
}

func TestRecovery(t *testing.T) {
	t.Parallel()
	ctx := testCc(t, `
		cc_library_shared {
			name: "librecovery",
			recovery: true,
		}
		cc_library_shared {
			name: "librecovery32",
			recovery: true,
			compile_multilib:"32",
		}
		cc_library_shared {
			name: "libHalInRecovery",
			recovery_available: true,
			vendor: true,
		}
	`)

	variants := ctx.ModuleVariantsForTests("librecovery")
	const arm64 = "android_recovery_arm64_armv8-a_shared"
	if len(variants) != 1 || !android.InList(arm64, variants) {
		t.Errorf("variants of librecovery must be \"%s\" only, but was %#v", arm64, variants)
	}

	variants = ctx.ModuleVariantsForTests("librecovery32")
	if android.InList(arm64, variants) {
		t.Errorf("multilib was set to 32 for librecovery32, but its variants has %s.", arm64)
	}

	recoveryModule := ctx.ModuleForTests(t, "libHalInRecovery", recoveryVariant).Module().(*Module)
	if !recoveryModule.Platform() {
		t.Errorf("recovery variant of libHalInRecovery must not specific to device, soc, or product")
	}
}

func TestDataLibsPrebuiltSharedTestLibrary(t *testing.T) {
	t.Parallel()
	bp := `
		cc_prebuilt_test_library_shared {
			name: "test_lib",
			relative_install_path: "foo/bar/baz",
			srcs: ["srcpath/dontusethispath/baz.so"],
		}

		cc_test {
			name: "main_test",
			data_libs: ["test_lib"],
			gtest: false,
		}
 `

	config := TestConfig(t.TempDir(), android.Android, nil, bp, nil)

	ctx := testCcWithConfig(t, config)
	testingModule := ctx.ModuleForTests(t, "main_test", "android_arm_armv7-a-neon")
	module := testingModule.Module()
	testBinary := module.(*Module).linker.(*testBinary)
	outputFiles := testingModule.OutputFiles(ctx, t, "")
	if len(outputFiles) != 1 {
		t.Errorf("expected exactly one output file. output files: [%s]", outputFiles)
	}
	if len(testBinary.dataPaths()) != 1 {
		t.Errorf("expected exactly one test data file. test data files: [%v]", testBinary.dataPaths())
	}

	outputPath := outputFiles[0].String()

	if !strings.HasSuffix(outputPath, "/main_test") {
		t.Errorf("expected test output file to be 'main_test', but was '%s'", outputPath)
	}
	entries := android.AndroidMkInfoForTest(t, ctx, module).PrimaryInfo
	if !strings.HasSuffix(entries.EntryMap["LOCAL_TEST_DATA"][0], ":test_lib.so:foo/bar/baz") {
		t.Errorf("expected LOCAL_TEST_DATA to end with `:test_lib.so:foo/bar/baz`,"+
			" but was '%s'", entries.EntryMap["LOCAL_TEST_DATA"][0])
	}
}

func TestVersionedStubs(t *testing.T) {
	t.Parallel()
	ctx := testCc(t, `
		cc_library_shared {
			name: "libFoo",
			srcs: ["foo.c"],
			stubs: {
				symbol_file: "foo.map.txt",
				versions: ["1", "2", "3"],
			},
		}

		cc_library_shared {
			name: "libBar",
			srcs: ["bar.c"],
			shared_libs: ["libFoo#1"],
		}`)

	variants := ctx.ModuleVariantsForTests("libFoo")
	expectedVariants := []string{
		"android_arm64_armv8-a_shared",
		"android_arm64_armv8-a_shared_1",
		"android_arm64_armv8-a_shared_2",
		"android_arm64_armv8-a_shared_3",
		"android_arm64_armv8-a_shared_current",
		"android_arm_armv7-a-neon_shared",
		"android_arm_armv7-a-neon_shared_1",
		"android_arm_armv7-a-neon_shared_2",
		"android_arm_armv7-a-neon_shared_3",
		"android_arm_armv7-a-neon_shared_current",
	}
	variantsMismatch := false
	if len(variants) != len(expectedVariants) {
		variantsMismatch = true
	} else {
		for _, v := range expectedVariants {
			if !inList(v, variants) {
				variantsMismatch = false
			}
		}
	}
	if variantsMismatch {
		t.Errorf("variants of libFoo expected:\n")
		for _, v := range expectedVariants {
			t.Errorf("%q\n", v)
		}
		t.Errorf(", but got:\n")
		for _, v := range variants {
			t.Errorf("%q\n", v)
		}
	}

	libBarLinkRule := ctx.ModuleForTests(t, "libBar", "android_arm64_armv8-a_shared").Rule("ld")
	libFlags := libBarLinkRule.Args["libFlags"]
	libFoo1StubPath := "libFoo/android_arm64_armv8-a_shared_1/libFoo.so"
	if !strings.Contains(libFlags, libFoo1StubPath) {
		t.Errorf("%q is not found in %q", libFoo1StubPath, libFlags)
	}

	libBarCompileRule := ctx.ModuleForTests(t, "libBar", "android_arm64_armv8-a_shared").Rule("cc")
	cFlags := libBarCompileRule.Args["cFlags"]
	libFoo1VersioningMacro := "-D__LIBFOO_API__=1"
	if !strings.Contains(cFlags, libFoo1VersioningMacro) {
		t.Errorf("%q is not found in %q", libFoo1VersioningMacro, cFlags)
	}
}

func TestVersioningMacro(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ moduleName, expected string }{
		{"libc", "__LIBC_API__"},
		{"libfoo", "__LIBFOO_API__"},
		{"libfoo@1", "__LIBFOO_1_API__"},
		{"libfoo-v1", "__LIBFOO_V1_API__"},
		{"libfoo.v1", "__LIBFOO_V1_API__"},
	} {
		checkEquals(t, tc.moduleName, tc.expected, versioningMacroName(tc.moduleName))
	}
}

func pathsToBase(paths android.Paths) []string {
	var ret []string
	for _, p := range paths {
		ret = append(ret, p.Base())
	}
	return ret
}

func TestStaticLibArchiveArgs(t *testing.T) {
	t.Parallel()
	ctx := testCc(t, `
		cc_library_static {
			name: "foo",
			srcs: ["foo.c"],
		}

		cc_library_static {
			name: "bar",
			srcs: ["bar.c"],
		}

		cc_library_shared {
			name: "qux",
			srcs: ["qux.c"],
		}

		cc_library_static {
			name: "baz",
			srcs: ["baz.c"],
			static_libs: ["foo"],
			shared_libs: ["qux"],
			whole_static_libs: ["bar"],
		}`)

	variant := "android_arm64_armv8-a_static"
	arRule := ctx.ModuleForTests(t, "baz", variant).Rule("ar")

	// For static libraries, the object files of a whole static dep are included in the archive
	// directly
	if g, w := pathsToBase(arRule.Inputs), []string{"bar.o", "baz.o"}; !reflect.DeepEqual(w, g) {
		t.Errorf("Expected input objects %q, got %q", w, g)
	}

	// non whole static dependencies are not linked into the archive
	if len(arRule.Implicits) > 0 {
		t.Errorf("Expected 0 additional deps, got %q", arRule.Implicits)
	}
}

func TestSharedLibLinkingArgs(t *testing.T) {
	t.Parallel()
	ctx := testCc(t, `
		cc_library_static {
			name: "foo",
			srcs: ["foo.c"],
		}

		cc_library_static {
			name: "bar",
			srcs: ["bar.c"],
		}

		cc_library_shared {
			name: "qux",
			srcs: ["qux.c"],
		}

		cc_library_shared {
			name: "baz",
			srcs: ["baz.c"],
			static_libs: ["foo"],
			shared_libs: ["qux"],
			whole_static_libs: ["bar"],
		}`)

	variant := "android_arm64_armv8-a_shared"
	linkRule := ctx.ModuleForTests(t, "baz", variant).Rule("ld")
	libFlags := linkRule.Args["libFlags"]
	// When dynamically linking, we expect static dependencies to be found on the command line
	if expected := "foo.a"; !strings.Contains(libFlags, expected) {
		t.Errorf("Static lib %q was not found in %q", expected, libFlags)
	}
	// When dynamically linking, we expect whole static dependencies to be found on the command line
	if expected := "bar.a"; !strings.Contains(libFlags, expected) {
		t.Errorf("Static lib %q was not found in %q", expected, libFlags)
	}

	// When dynamically linking, we expect shared dependencies to be found on the command line
	if expected := "qux.so"; !strings.Contains(libFlags, expected) {
		t.Errorf("Shared lib %q was not found in %q", expected, libFlags)
	}

	// We should only have the objects from the shared library srcs, not the whole static dependencies
	if g, w := pathsToBase(linkRule.Inputs), []string{"baz.o"}; !reflect.DeepEqual(w, g) {
		t.Errorf("Expected input objects %q, got %q", w, g)
	}
}

func TestStaticExecutable(t *testing.T) {
	t.Parallel()
	ctx := testCc(t, `
		cc_binary {
			name: "static_test",
			srcs: ["foo.c", "baz.o"],
			static_executable: true,
		}`)

	variant := "android_arm64_armv8-a"
	binModuleRule := ctx.ModuleForTests(t, "static_test", variant).Rule("ld")
	libFlags := binModuleRule.Args["libFlags"]
	systemStaticLibs := []string{"libc.a", "libm.a"}
	for _, lib := range systemStaticLibs {
		if !strings.Contains(libFlags, lib) {
			t.Errorf("Static lib %q was not found in %q", lib, libFlags)
		}
	}
	systemSharedLibs := []string{"libc.so", "libm.so", "libdl.so"}
	for _, lib := range systemSharedLibs {
		if strings.Contains(libFlags, lib) {
			t.Errorf("Shared lib %q was found in %q", lib, libFlags)
		}
	}
}

func TestStaticDepsOrderWithStubs(t *testing.T) {
	t.Parallel()
	ctx := testCc(t, `
		cc_binary {
			name: "mybin",
			srcs: ["foo.c"],
			static_libs: ["libfooC", "libfooB"],
			static_executable: true,
			stl: "none",
		}

		cc_library {
			name: "libfooB",
			srcs: ["foo.c"],
			shared_libs: ["libfooC"],
			stl: "none",
		}

		cc_library {
			name: "libfooC",
			srcs: ["foo.c"],
			stl: "none",
			stubs: {
				versions: ["1"],
			},
		}`)

	mybin := ctx.ModuleForTests(t, "mybin", "android_arm64_armv8-a").Rule("ld")
	actual := mybin.Implicits[:2]
	expected := GetOutputPaths(t, ctx, "android_arm64_armv8-a_static", []string{"libfooB", "libfooC"})

	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("staticDeps orderings were not propagated correctly"+
			"\nactual:   %v"+
			"\nexpected: %v",
			actual,
			expected,
		)
	}
}

func TestErrorsIfAModuleDependsOnDisabled(t *testing.T) {
	t.Parallel()
	testCcError(t, `module "libA" .* depends on disabled module "libB"`, `
		cc_library {
			name: "libA",
			srcs: ["foo.c"],
			shared_libs: ["libB"],
			stl: "none",
		}

		cc_library {
			name: "libB",
			srcs: ["foo.c"],
			enabled: false,
			stl: "none",
		}
	`)
}

func VerifyAFLFuzzTargetVariant(t *testing.T, variant string) {
	bp := `
		cc_fuzz {
			name: "test_afl_fuzz_target",
			srcs: ["foo.c"],
			host_supported: true,
			static_libs: [
				"afl_fuzz_static_lib",
			],
			shared_libs: [
				"afl_fuzz_shared_lib",
			],
			fuzzing_frameworks: {
				afl: true,
				libfuzzer: false,
			},
		}
		cc_library {
			name: "afl_fuzz_static_lib",
			host_supported: true,
			srcs: ["static_file.c"],
		}
		cc_library {
			name: "libfuzzer_only_static_lib",
			host_supported: true,
			srcs: ["static_file.c"],
		}
		cc_library {
			name: "afl_fuzz_shared_lib",
			host_supported: true,
			srcs: ["shared_file.c"],
			static_libs: [
				"second_static_lib",
			],
		}
		cc_library_headers {
			name: "libafl_headers",
			vendor_available: true,
			host_supported: true,
			export_include_dirs: [
				"include",
				"instrumentation",
			],
		}
		cc_object {
			name: "afl-compiler-rt",
			vendor_available: true,
			host_supported: true,
			cflags: [
				"-fPIC",
			],
			srcs: [
				"instrumentation/afl-compiler-rt.o.c",
			],
		}
		cc_library {
			name: "second_static_lib",
			host_supported: true,
			srcs: ["second_file.c"],
		}
		cc_object {
			name: "aflpp_driver",
			host_supported: true,
			srcs: [
				"aflpp_driver.c",
			],
		}`

	testEnv := map[string]string{
		"FUZZ_FRAMEWORK": "AFL",
	}

	ctx := android.GroupFixturePreparers(prepareForCcTest, android.FixtureMergeEnv(testEnv)).RunTestWithBp(t, bp)

	checkPcGuardFlag := func(
		modName string, variantName string, shouldHave bool) {
		cc := ctx.ModuleForTests(t, modName, variantName).Rule("cc")

		cFlags, ok := cc.Args["cFlags"]
		if !ok {
			t.Errorf("Could not find cFlags for module %s and variant %s",
				modName, variantName)
		}

		if strings.Contains(
			cFlags, "-fsanitize-coverage=trace-pc-guard") != shouldHave {
			t.Errorf("Flag was found: %t. Expected to find flag:  %t. "+
				"Test failed for module %s and variant %s",
				!shouldHave, shouldHave, modName, variantName)
		}
	}

	moduleName := "test_afl_fuzz_target"
	checkPcGuardFlag(moduleName, variant+"_fuzzer", true)

	moduleName = "afl_fuzz_static_lib"
	checkPcGuardFlag(moduleName, variant+"_static", false)
	checkPcGuardFlag(moduleName, variant+"_static_fuzzer_afl", true)

	moduleName = "second_static_lib"
	checkPcGuardFlag(moduleName, variant+"_static", false)
	checkPcGuardFlag(moduleName, variant+"_static_fuzzer_afl", true)

	ctx.ModuleForTests(t, "afl_fuzz_shared_lib",
		"android_arm64_armv8-a_shared").Rule("cc")
	ctx.ModuleForTests(t, "afl_fuzz_shared_lib",
		"android_arm64_armv8-a_shared_fuzzer").Rule("cc")
}

func TestAFLFuzzTargetForDevice(t *testing.T) {
	t.Parallel()
	VerifyAFLFuzzTargetVariant(t, "android_arm64_armv8-a")
}

func TestAFLFuzzTargetForLinuxHost(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("requires linux")
	}

	VerifyAFLFuzzTargetVariant(t, "linux_glibc_x86_64")
}

// Simple smoke test for the cc_fuzz target that ensures the rule compiles
// correctly.
func TestFuzzTarget(t *testing.T) {
	t.Parallel()
	ctx := testCc(t, `
		cc_fuzz {
			name: "fuzz_smoke_test",
			srcs: ["foo.c"],
		}`)

	variant := "android_arm64_armv8-a_fuzzer"
	ctx.ModuleForTests(t, "fuzz_smoke_test", variant).Rule("cc")
}

func assertString(t *testing.T, got, expected string) {
	t.Helper()
	if got != expected {
		t.Errorf("expected %q got %q", expected, got)
	}
}

func assertArrayString(t *testing.T, got, expected []string) {
	t.Helper()
	if len(got) != len(expected) {
		t.Errorf("expected %d (%q) got (%d) %q", len(expected), expected, len(got), got)
		return
	}
	for i := range got {
		if got[i] != expected[i] {
			t.Errorf("expected %d-th %q (%q) got %q (%q)",
				i, expected[i], expected, got[i], got)
			return
		}
	}
}

func assertMapKeys(t *testing.T, m map[string]string, expected []string) {
	t.Helper()
	assertArrayString(t, android.SortedKeys(m), expected)
}

func TestDefaults(t *testing.T) {
	t.Parallel()
	ctx := testCc(t, `
		cc_defaults {
			name: "defaults",
			srcs: ["foo.c"],
			static: {
				srcs: ["bar.c"],
			},
			shared: {
				srcs: ["baz.c"],
			},
		}

		cc_library_static {
			name: "libstatic",
			defaults: ["defaults"],
		}

		cc_library_shared {
			name: "libshared",
			defaults: ["defaults"],
		}

		cc_library {
			name: "libboth",
			defaults: ["defaults"],
		}

		cc_binary {
			name: "binary",
			defaults: ["defaults"],
		}`)

	shared := ctx.ModuleForTests(t, "libshared", "android_arm64_armv8-a_shared").Rule("ld")
	if g, w := pathsToBase(shared.Inputs), []string{"foo.o", "baz.o"}; !reflect.DeepEqual(w, g) {
		t.Errorf("libshared ld rule wanted %q, got %q", w, g)
	}
	bothShared := ctx.ModuleForTests(t, "libboth", "android_arm64_armv8-a_shared").Rule("ld")
	if g, w := pathsToBase(bothShared.Inputs), []string{"foo.o", "baz.o"}; !reflect.DeepEqual(w, g) {
		t.Errorf("libboth ld rule wanted %q, got %q", w, g)
	}
	binary := ctx.ModuleForTests(t, "binary", "android_arm64_armv8-a").Rule("ld")
	if g, w := pathsToBase(binary.Inputs), []string{"foo.o"}; !reflect.DeepEqual(w, g) {
		t.Errorf("binary ld rule wanted %q, got %q", w, g)
	}

	static := ctx.ModuleForTests(t, "libstatic", "android_arm64_armv8-a_static").Rule("ar")
	if g, w := pathsToBase(static.Inputs), []string{"foo.o", "bar.o"}; !reflect.DeepEqual(w, g) {
		t.Errorf("libstatic ar rule wanted %q, got %q", w, g)
	}
	bothStatic := ctx.ModuleForTests(t, "libboth", "android_arm64_armv8-a_static").Rule("ar")
	if g, w := pathsToBase(bothStatic.Inputs), []string{"foo.o", "bar.o"}; !reflect.DeepEqual(w, g) {
		t.Errorf("libboth ar rule wanted %q, got %q", w, g)
	}
}

func TestProductVariableDefaults(t *testing.T) {
	t.Parallel()
	bp := `
		cc_defaults {
			name: "libfoo_defaults",
			srcs: ["foo.c"],
			cppflags: ["-DFOO"],
			product_variables: {
				debuggable: {
					cppflags: ["-DBAR"],
				},
			},
		}

		cc_library {
			name: "libfoo",
			defaults: ["libfoo_defaults"],
		}
	`

	result := android.GroupFixturePreparers(
		prepareForCcTest,
		android.PrepareForTestWithVariables,

		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.Debuggable = BoolPtr(true)
		}),
	).RunTestWithBp(t, bp)

	libfoo := result.Module("libfoo", "android_arm64_armv8-a_static").(*Module)
	android.AssertStringListContains(t, "cppflags", libfoo.flags.Local.CppFlags, "-DBAR")
}

func TestEmptyWholeStaticLibsAllowMissingDependencies(t *testing.T) {
	t.Parallel()
	bp := `
		cc_library_static {
			name: "libfoo",
			srcs: ["foo.c"],
			whole_static_libs: ["libbar"],
		}

		cc_library_static {
			name: "libbar",
			whole_static_libs: ["libmissing"],
		}
	`

	result := android.GroupFixturePreparers(
		prepareForCcTest,
		android.PrepareForTestWithAllowMissingDependencies,
	).RunTestWithBp(t, bp)

	libbar := result.ModuleForTests(t, "libbar", "android_arm64_armv8-a_static").Output("libbar.a")
	android.AssertDeepEquals(t, "libbar rule", android.ErrorRule, libbar.Rule)

	android.AssertStringDoesContain(t, "libbar error", libbar.Args["error"], "missing dependencies: libmissing")

	libfoo := result.ModuleForTests(t, "libfoo", "android_arm64_armv8-a_static").Output("libfoo.a")
	android.AssertStringListContains(t, "libfoo.a dependencies", libfoo.Inputs.Strings(), libbar.Output.String())
}

func TestInstallSharedLibs(t *testing.T) {
	t.Parallel()
	bp := `
		cc_binary {
			name: "bin",
			host_supported: true,
			shared_libs: ["libshared"],
			runtime_libs: ["libruntime"],
			srcs: [":gen"],
		}

		cc_library_shared {
			name: "libshared",
			host_supported: true,
			shared_libs: ["libtransitive"],
		}

		cc_library_shared {
			name: "libtransitive",
			host_supported: true,
		}

		cc_library_shared {
			name: "libruntime",
			host_supported: true,
		}

		cc_binary_host {
			name: "tool",
			srcs: ["foo.cpp"],
		}

		genrule {
			name: "gen",
			tools: ["tool"],
			out: ["gen.cpp"],
			cmd: "$(location tool) $(out)",
		}
	`

	config := TestConfig(t.TempDir(), android.Android, nil, bp, nil)
	ctx := testCcWithConfig(t, config)

	hostBin := ctx.ModuleForTests(t, "bin", config.BuildOSTarget.String()).Description("install")
	hostShared := ctx.ModuleForTests(t, "libshared", config.BuildOSTarget.String()+"_shared").Description("install")
	hostRuntime := ctx.ModuleForTests(t, "libruntime", config.BuildOSTarget.String()+"_shared").Description("install")
	hostTransitive := ctx.ModuleForTests(t, "libtransitive", config.BuildOSTarget.String()+"_shared").Description("install")
	hostTool := ctx.ModuleForTests(t, "tool", config.BuildOSTarget.String()).Description("install")

	if g, w := hostBin.Implicits.Strings(), hostShared.Output.String(); !android.InList(w, g) {
		t.Errorf("expected host bin dependency %q, got %q", w, g)
	}

	if g, w := hostBin.Implicits.Strings(), hostTransitive.Output.String(); !android.InList(w, g) {
		t.Errorf("expected host bin dependency %q, got %q", w, g)
	}

	if g, w := hostShared.Implicits.Strings(), hostTransitive.Output.String(); !android.InList(w, g) {
		t.Errorf("expected host bin dependency %q, got %q", w, g)
	}

	if g, w := hostBin.Implicits.Strings(), hostRuntime.Output.String(); !android.InList(w, g) {
		t.Errorf("expected host bin dependency %q, got %q", w, g)
	}

	if g, w := hostBin.Implicits.Strings(), hostTool.Output.String(); android.InList(w, g) {
		t.Errorf("expected no host bin dependency %q, got %q", w, g)
	}

	deviceBin := ctx.ModuleForTests(t, "bin", "android_arm64_armv8-a").Description("install")
	deviceShared := ctx.ModuleForTests(t, "libshared", "android_arm64_armv8-a_shared").Description("install")
	deviceTransitive := ctx.ModuleForTests(t, "libtransitive", "android_arm64_armv8-a_shared").Description("install")
	deviceRuntime := ctx.ModuleForTests(t, "libruntime", "android_arm64_armv8-a_shared").Description("install")

	if g, w := deviceBin.OrderOnly.Strings(), deviceShared.Output.String(); !android.InList(w, g) {
		t.Errorf("expected device bin dependency %q, got %q", w, g)
	}

	if g, w := deviceBin.OrderOnly.Strings(), deviceTransitive.Output.String(); !android.InList(w, g) {
		t.Errorf("expected device bin dependency %q, got %q", w, g)
	}

	if g, w := deviceShared.OrderOnly.Strings(), deviceTransitive.Output.String(); !android.InList(w, g) {
		t.Errorf("expected device bin dependency %q, got %q", w, g)
	}

	if g, w := deviceBin.OrderOnly.Strings(), deviceRuntime.Output.String(); !android.InList(w, g) {
		t.Errorf("expected device bin dependency %q, got %q", w, g)
	}

	if g, w := deviceBin.OrderOnly.Strings(), hostTool.Output.String(); android.InList(w, g) {
		t.Errorf("expected no device bin dependency %q, got %q", w, g)
	}

}

func TestStubsLibReexportsHeaders(t *testing.T) {
	t.Parallel()
	ctx := testCc(t, `
		cc_library_shared {
			name: "libclient",
			srcs: ["foo.c"],
			shared_libs: ["libfoo#1"],
		}

		cc_library_shared {
			name: "libfoo",
			srcs: ["foo.c"],
			shared_libs: ["libbar"],
			export_shared_lib_headers: ["libbar"],
			stubs: {
				symbol_file: "foo.map.txt",
				versions: ["1", "2", "3"],
			},
		}

		cc_library_shared {
			name: "libbar",
			export_include_dirs: ["include/libbar"],
			srcs: ["foo.c"],
		}`)

	cFlags := ctx.ModuleForTests(t, "libclient", "android_arm64_armv8-a_shared").Rule("cc").Args["cFlags"]

	if !strings.Contains(cFlags, "-Iinclude/libbar") {
		t.Errorf("expected %q in cflags, got %q", "-Iinclude/libbar", cFlags)
	}
}

func TestAidlLibraryWithHeaders(t *testing.T) {
	t.Parallel()
	ctx := android.GroupFixturePreparers(
		prepareForCcTest,
		aidl_library.PrepareForTestWithAidlLibrary,
		android.MockFS{
			"package_bar/Android.bp": []byte(`
			aidl_library {
				name: "bar",
				srcs: ["x/y/Bar.aidl"],
				hdrs: ["x/HeaderBar.aidl"],
				strip_import_prefix: "x",
			}
			`)}.AddToFixture(),
		android.MockFS{
			"package_foo/Android.bp": []byte(`
			aidl_library {
				name: "foo",
				srcs: ["a/b/Foo.aidl"],
				hdrs: ["a/HeaderFoo.aidl"],
				strip_import_prefix: "a",
				deps: ["bar"],
			}
			cc_library {
				name: "libfoo",
				aidl: {
					libs: ["foo"],
				}
			}
			`),
		}.AddToFixture(),
	).RunTest(t).TestContext

	libfoo := ctx.ModuleForTests(t, "libfoo", "android_arm64_armv8-a_static")

	android.AssertPathsRelativeToTopEquals(
		t,
		"aidl headers",
		[]string{
			"package_bar/x/HeaderBar.aidl",
			"package_foo/a/HeaderFoo.aidl",
			"package_foo/a/b/Foo.aidl",
			"out/soong/.intermediates/package_foo/libfoo/android_arm64_armv8-a_static/gen/aidl_library.sbox.textproto",
		},
		libfoo.Rule("aidl_library").Implicits,
	)

	manifest := android.RuleBuilderSboxProtoForTests(t, ctx, libfoo.Output("aidl_library.sbox.textproto"))
	aidlCommand := manifest.Commands[0].GetCommand()

	expectedAidlFlags := "-Ipackage_foo/a -Ipackage_bar/x"
	if !strings.Contains(aidlCommand, expectedAidlFlags) {
		t.Errorf("aidl command %q does not contain %q", aidlCommand, expectedAidlFlags)
	}

	outputs := strings.Join(libfoo.AllOutputs(), " ")

	android.AssertStringDoesContain(t, "aidl-generated header", outputs, "gen/aidl_library/b/BpFoo.h")
	android.AssertStringDoesContain(t, "aidl-generated header", outputs, "gen/aidl_library/b/BnFoo.h")
	android.AssertStringDoesContain(t, "aidl-generated header", outputs, "gen/aidl_library/b/Foo.h")
	android.AssertStringDoesContain(t, "aidl-generated cpp", outputs, "b/Foo.cpp")
	// Confirm that the aidl header doesn't get compiled to cpp and h files
	android.AssertStringDoesNotContain(t, "aidl-generated header", outputs, "gen/aidl_library/y/BpBar.h")
	android.AssertStringDoesNotContain(t, "aidl-generated header", outputs, "gen/aidl_library/y/BnBar.h")
	android.AssertStringDoesNotContain(t, "aidl-generated header", outputs, "gen/aidl_library/y/Bar.h")
	android.AssertStringDoesNotContain(t, "aidl-generated cpp", outputs, "y/Bar.cpp")
}

func TestAidlFlagsPassedToTheAidlCompiler(t *testing.T) {
	t.Parallel()
	ctx := android.GroupFixturePreparers(
		prepareForCcTest,
		aidl_library.PrepareForTestWithAidlLibrary,
	).RunTestWithBp(t, `
		cc_library {
			name: "libfoo",
			srcs: ["a/Foo.aidl"],
			aidl: { flags: ["-Werror"], },
		}
	`)

	libfoo := ctx.ModuleForTests(t, "libfoo", "android_arm64_armv8-a_static")
	manifest := android.RuleBuilderSboxProtoForTests(t, ctx.TestContext, libfoo.Output("aidl.sbox.textproto"))
	aidlCommand := manifest.Commands[0].GetCommand()
	expectedAidlFlag := "-Werror"
	if !strings.Contains(aidlCommand, expectedAidlFlag) {
		t.Errorf("aidl command %q does not contain %q", aidlCommand, expectedAidlFlag)
	}
}

func TestAidlFlagsWithMinSdkVersion(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		sdkVersion string
		variant    string
		expected   string
	}{
		{
			name:       "default is current",
			sdkVersion: "",
			variant:    "android_arm64_armv8-a_static",
			expected:   "platform_apis",
		},
		{
			name:       "use sdk_version",
			sdkVersion: `sdk_version: "29"`,
			variant:    "android_arm64_armv8-a_static",
			expected:   "platform_apis",
		},
		{
			name:       "use sdk_version(sdk variant)",
			sdkVersion: `sdk_version: "29"`,
			variant:    "android_arm64_armv8-a_sdk_static",
			expected:   "29",
		},
		{
			name:       "use min_sdk_version",
			sdkVersion: `min_sdk_version: "29"`,
			variant:    "android_arm64_armv8-a_static",
			expected:   "29",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := testCc(t, `
				cc_library {
					name: "libfoo",
					stl: "none",
					srcs: ["a/Foo.aidl"],
					`+tc.sdkVersion+`
				}
			`)
			libfoo := ctx.ModuleForTests(t, "libfoo", tc.variant)
			manifest := android.RuleBuilderSboxProtoForTests(t, ctx, libfoo.Output("aidl.sbox.textproto"))
			aidlCommand := manifest.Commands[0].GetCommand()
			expectedAidlFlag := "--min_sdk_version=" + tc.expected
			if !strings.Contains(aidlCommand, expectedAidlFlag) {
				t.Errorf("aidl command %q does not contain %q", aidlCommand, expectedAidlFlag)
			}
		})
	}
}

func TestInvalidAidlProp(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		description string
		bp          string
	}{
		{
			description: "Invalid use of aidl.libs and aidl.include_dirs",
			bp: `
			cc_library {
				name: "foo",
				aidl: {
					libs: ["foo_aidl"],
					include_dirs: ["bar/include"],
				}
			}
			`,
		},
		{
			description: "Invalid use of aidl.libs and aidl.local_include_dirs",
			bp: `
			cc_library {
				name: "foo",
				aidl: {
					libs: ["foo_aidl"],
					local_include_dirs: ["include"],
				}
			}
			`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			bp := `
			aidl_library {
				name: "foo_aidl",
				srcs: ["Foo.aidl"],
			} ` + testCase.bp
			android.GroupFixturePreparers(
				prepareForCcTest,
				aidl_library.PrepareForTestWithAidlLibrary.
					ExtendWithErrorHandler(android.FixtureExpectsOneErrorPattern("For aidl headers, please only use aidl.libs prop")),
			).RunTestWithBp(t, bp)
		})
	}
}

func TestMinSdkVersionInClangTriple(t *testing.T) {
	t.Parallel()
	ctx := testCc(t, `
		cc_library_shared {
			name: "libfoo",
			srcs: ["foo.c"],
			min_sdk_version: "29",
		}`)

	cFlags := ctx.ModuleForTests(t, "libfoo", "android_arm64_armv8-a_shared").Rule("cc").Args["cFlags"]
	android.AssertStringDoesContain(t, "min sdk version", cFlags, "-target aarch64-linux-android29")
}

func TestNonDigitMinSdkVersionInClangTriple(t *testing.T) {
	t.Parallel()
	bp := `
		cc_library_shared {
			name: "libfoo",
			srcs: ["foo.c"],
			min_sdk_version: "S",
		}
	`
	result := android.GroupFixturePreparers(
		prepareForCcTest,
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.Platform_version_active_codenames = []string{"UpsideDownCake", "Tiramisu"}
		}),
	).RunTestWithBp(t, bp)
	ctx := result.TestContext
	cFlags := ctx.ModuleForTests(t, "libfoo", "android_arm64_armv8-a_shared").Rule("cc").Args["cFlags"]
	android.AssertStringDoesContain(t, "min sdk version", cFlags, "-target aarch64-linux-android31")
}

func TestIncludeDirsExporting(t *testing.T) {
	t.Parallel()

	// Trim spaces from the beginning, end and immediately after any newline characters. Leaves
	// embedded newline characters alone.
	trimIndentingSpaces := func(s string) string {
		return strings.TrimSpace(regexp.MustCompile("(^|\n)\\s+").ReplaceAllString(s, "$1"))
	}

	checkPaths := func(t *testing.T, message string, expected string, paths android.Paths) {
		t.Helper()
		expected = trimIndentingSpaces(expected)
		actual := trimIndentingSpaces(strings.Join(android.FirstUniqueStrings(android.NormalizePathsForTesting(paths)), "\n"))
		if expected != actual {
			t.Errorf("%s: expected:\n%s\n actual:\n%s\n", message, expected, actual)
		}
	}

	type exportedChecker func(t *testing.T, name string, exported FlagExporterInfo)

	checkIncludeDirs := func(t *testing.T, ctx *android.TestContext, module android.Module, checkers ...exportedChecker) {
		t.Helper()
		exported, _ := android.OtherModuleProvider(ctx, module, FlagExporterInfoProvider)
		name := module.Name()

		for _, checker := range checkers {
			checker(t, name, exported)
		}
	}

	expectedIncludeDirs := func(expectedPaths string) exportedChecker {
		return func(t *testing.T, name string, exported FlagExporterInfo) {
			t.Helper()
			checkPaths(t, fmt.Sprintf("%s: include dirs", name), expectedPaths, exported.IncludeDirs)
		}
	}

	expectedSystemIncludeDirs := func(expectedPaths string) exportedChecker {
		return func(t *testing.T, name string, exported FlagExporterInfo) {
			t.Helper()
			checkPaths(t, fmt.Sprintf("%s: system include dirs", name), expectedPaths, exported.SystemIncludeDirs)
		}
	}

	expectedGeneratedHeaders := func(expectedPaths string) exportedChecker {
		return func(t *testing.T, name string, exported FlagExporterInfo) {
			t.Helper()
			checkPaths(t, fmt.Sprintf("%s: generated headers", name), expectedPaths, exported.GeneratedHeaders)
		}
	}

	expectedOrderOnlyDeps := func(expectedPaths string) exportedChecker {
		return func(t *testing.T, name string, exported FlagExporterInfo) {
			t.Helper()
			checkPaths(t, fmt.Sprintf("%s: order only deps", name), expectedPaths, exported.Deps)
		}
	}

	genRuleModules := `
		genrule {
			name: "genrule_foo",
			cmd: "generate-foo",
			out: [
				"generated_headers/foo/generated_header.h",
			],
			export_include_dirs: [
				"generated_headers",
			],
		}

		genrule {
			name: "genrule_bar",
			cmd: "generate-bar",
			out: [
				"generated_headers/bar/generated_header.h",
			],
			export_include_dirs: [
				"generated_headers",
			],
		}
	`

	t.Run("ensure exported include dirs are not automatically re-exported from shared_libs", func(t *testing.T) {
		ctx := testCc(t, genRuleModules+`
		cc_library {
			name: "libfoo",
			srcs: ["foo.c"],
			export_include_dirs: ["foo/standard"],
			export_system_include_dirs: ["foo/system"],
			generated_headers: ["genrule_foo"],
			export_generated_headers: ["genrule_foo"],
		}

		cc_library {
			name: "libbar",
			srcs: ["bar.c"],
			shared_libs: ["libfoo"],
			export_include_dirs: ["bar/standard"],
			export_system_include_dirs: ["bar/system"],
			generated_headers: ["genrule_bar"],
			export_generated_headers: ["genrule_bar"],
		}
		`)
		foo := ctx.ModuleForTests(t, "libfoo", "android_arm64_armv8-a_shared").Module()
		checkIncludeDirs(t, ctx, foo,
			expectedIncludeDirs(`
				foo/standard
				.intermediates/genrule_foo/gen/generated_headers
			`),
			expectedSystemIncludeDirs(`foo/system`),
			expectedGeneratedHeaders(`.intermediates/genrule_foo/gen/generated_headers/foo/generated_header.h`),
			expectedOrderOnlyDeps(`.intermediates/genrule_foo/gen/generated_headers/foo/generated_header.h`),
		)

		bar := ctx.ModuleForTests(t, "libbar", "android_arm64_armv8-a_shared").Module()
		checkIncludeDirs(t, ctx, bar,
			expectedIncludeDirs(`
				bar/standard
				.intermediates/genrule_bar/gen/generated_headers
			`),
			expectedSystemIncludeDirs(`bar/system`),
			expectedGeneratedHeaders(`.intermediates/genrule_bar/gen/generated_headers/bar/generated_header.h`),
			expectedOrderOnlyDeps(`.intermediates/genrule_bar/gen/generated_headers/bar/generated_header.h`),
		)
	})

	t.Run("ensure exported include dirs are automatically re-exported from whole_static_libs", func(t *testing.T) {
		ctx := testCc(t, genRuleModules+`
		cc_library {
			name: "libfoo",
			srcs: ["foo.c"],
			export_include_dirs: ["foo/standard"],
			export_system_include_dirs: ["foo/system"],
			generated_headers: ["genrule_foo"],
			export_generated_headers: ["genrule_foo"],
		}

		cc_library {
			name: "libbar",
			srcs: ["bar.c"],
			whole_static_libs: ["libfoo"],
			export_include_dirs: ["bar/standard"],
			export_system_include_dirs: ["bar/system"],
			generated_headers: ["genrule_bar"],
			export_generated_headers: ["genrule_bar"],
		}
		`)
		foo := ctx.ModuleForTests(t, "libfoo", "android_arm64_armv8-a_shared").Module()
		checkIncludeDirs(t, ctx, foo,
			expectedIncludeDirs(`
				foo/standard
				.intermediates/genrule_foo/gen/generated_headers
			`),
			expectedSystemIncludeDirs(`foo/system`),
			expectedGeneratedHeaders(`.intermediates/genrule_foo/gen/generated_headers/foo/generated_header.h`),
			expectedOrderOnlyDeps(`.intermediates/genrule_foo/gen/generated_headers/foo/generated_header.h`),
		)

		bar := ctx.ModuleForTests(t, "libbar", "android_arm64_armv8-a_shared").Module()
		checkIncludeDirs(t, ctx, bar,
			expectedIncludeDirs(`
				bar/standard
				foo/standard
				.intermediates/genrule_foo/gen/generated_headers
				.intermediates/genrule_bar/gen/generated_headers
			`),
			expectedSystemIncludeDirs(`
				bar/system
				foo/system
			`),
			expectedGeneratedHeaders(`
				.intermediates/genrule_foo/gen/generated_headers/foo/generated_header.h
				.intermediates/genrule_bar/gen/generated_headers/bar/generated_header.h
			`),
			expectedOrderOnlyDeps(`
				.intermediates/genrule_foo/gen/generated_headers/foo/generated_header.h
				.intermediates/genrule_bar/gen/generated_headers/bar/generated_header.h
			`),
		)
	})

	t.Run("ensure only aidl headers are exported", func(t *testing.T) {
		ctx := android.GroupFixturePreparers(
			prepareForCcTest,
			aidl_library.PrepareForTestWithAidlLibrary,
		).RunTestWithBp(t, `
		aidl_library {
			name: "libfoo_aidl",
			srcs: ["x/y/Bar.aidl"],
			strip_import_prefix: "x",
		}
		cc_library_shared {
			name: "libfoo",
			srcs: [
				"foo.c",
				"b.aidl",
				"a.proto",
			],
			aidl: {
				libs: ["libfoo_aidl"],
				export_aidl_headers: true,
			}
		}
		`).TestContext
		foo := ctx.ModuleForTests(t, "libfoo", "android_arm64_armv8-a_shared").Module()
		checkIncludeDirs(t, ctx, foo,
			expectedIncludeDirs(`
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/aidl
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/aidl_library
			`),
			expectedSystemIncludeDirs(``),
			expectedGeneratedHeaders(`
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/aidl/b.h
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/aidl/Bnb.h
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/aidl/Bpb.h
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/aidl_library/y/Bar.h
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/aidl_library/y/BnBar.h
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/aidl_library/y/BpBar.h
			`),
			expectedOrderOnlyDeps(`
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/aidl/b.h
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/aidl/Bnb.h
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/aidl/Bpb.h
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/aidl_library/y/Bar.h
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/aidl_library/y/BnBar.h
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/aidl_library/y/BpBar.h
			`),
		)
	})

	t.Run("ensure only proto headers are exported", func(t *testing.T) {
		ctx := testCc(t, genRuleModules+`
		cc_library_shared {
			name: "libfoo",
			srcs: [
				"foo.c",
				"b.aidl",
				"a.proto",
			],
			proto: {
				export_proto_headers: true,
			}
		}
		`)
		foo := ctx.ModuleForTests(t, "libfoo", "android_arm64_armv8-a_shared").Module()
		checkIncludeDirs(t, ctx, foo,
			expectedIncludeDirs(`
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/proto
			`),
			expectedSystemIncludeDirs(``),
			expectedGeneratedHeaders(`
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/proto/a.pb.h
			`),
			expectedOrderOnlyDeps(`
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/proto/a.pb.h
			`),
		)
	})

	t.Run("ensure only sysprop headers are exported", func(t *testing.T) {
		ctx := testCc(t, genRuleModules+`
		cc_library_shared {
			name: "libfoo",
			srcs: [
				"foo.c",
				"path/to/a.sysprop",
				"b.aidl",
				"a.proto",
			],
		}
		`)
		foo := ctx.ModuleForTests(t, "libfoo", "android_arm64_armv8-a_shared").Module()
		checkIncludeDirs(t, ctx, foo,
			expectedIncludeDirs(`
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/sysprop/include
			`),
			expectedSystemIncludeDirs(``),
			expectedGeneratedHeaders(`
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/sysprop/include/path/to/a.sysprop.h
			`),
			expectedOrderOnlyDeps(`
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/sysprop/include/path/to/a.sysprop.h
				.intermediates/libfoo/android_arm64_armv8-a_shared/gen/sysprop/public/include/path/to/a.sysprop.h
			`),
		)
	})
}

func TestIncludeDirectoryOrdering(t *testing.T) {
	t.Parallel()

	expectedPlatformFlags := []string{
		"-nostdlibinc",
	}

	baseExpectedFlags := []string{
		"${config.ArmThumbCflags}",
		"${config.ArmCflags}",
		"${config.CommonGlobalCflags}",
		"${config.DeviceGlobalCflags}",
		"${config.ExternalCflags}",
		"${config.ArmToolchainCflags}",
		"${config.ArmArmv7ANeonCflags}",
		"${config.ArmGenericCflags}",
	}

	expectedTargetNDKFlags := []string{
		"-target",
		"armv7a-linux-androideabi21",
	}

	expectedTargetPlatformFlags := []string{
		"-target",
		"armv7a-linux-androideabi10000",
	}

	expectedIncludes := []string{
		"external/foo/android_arm_export_include_dirs",
		"external/foo/lib32_export_include_dirs",
		"external/foo/arm_export_include_dirs",
		"external/foo/android_export_include_dirs",
		"external/foo/linux_export_include_dirs",
		"external/foo/export_include_dirs",
		"external/foo/android_arm_local_include_dirs",
		"external/foo/lib32_local_include_dirs",
		"external/foo/arm_local_include_dirs",
		"external/foo/android_local_include_dirs",
		"external/foo/linux_local_include_dirs",
		"external/foo/local_include_dirs",
		"external/foo",
		"external/foo/libheader1",
		"external/foo/libheader2",
		"external/foo/libwhole1",
		"external/foo/libwhole2",
		"external/foo/libstatic1",
		"external/foo/libstatic2",
		"external/foo/libshared1",
		"external/foo/libshared2",
		"external/foo/liblinux",
		"external/foo/libandroid",
		"external/foo/libarm",
		"external/foo/lib32",
		"external/foo/libandroid_arm",
	}

	expectedNDKSTLIncludes := []string{
		"defaults/cc/common/ndk_libc++_shared_include_dirs",
	}

	conly := []string{"-fPIC", "${config.CommonGlobalConlyflags}"}
	cppOnly := []string{"-fPIC", "${config.CommonGlobalCppflags}", "${config.DeviceGlobalCppflags}", "${config.ArmCppflags}"}

	cflags := []string{"-Werror", "-std=candcpp"}
	cstd := []string{"-std=gnu23", "-std=conly"}
	cppstd := []string{"-std=gnu++20", "-std=cpp", "-fno-rtti"}

	lastNDKFlags := []string{
		"--sysroot",
		"out/soong/ndk/sysroot",
	}

	lastPlatformIncludes := []string{
		"${config.CommonGlobalIncludes}",
	}

	testCases := []struct {
		name             string
		src              string
		expectedNDK      []string
		expectedPlatform []string
	}{
		{
			name: "c",
			src:  "foo.c",
			expectedNDK: slices.Concat(
				baseExpectedFlags,
				expectedTargetNDKFlags,
				conly,
				expectedIncludes,
				expectedNDKSTLIncludes,
				cflags,
				cstd,
				lastNDKFlags,
				[]string{"${config.NoOverrideGlobalCflags}", "${config.NoOverrideExternalGlobalCflags}"},
			),
			expectedPlatform: slices.Concat(
				expectedPlatformFlags,
				baseExpectedFlags,
				expectedTargetPlatformFlags,
				conly,
				expectedIncludes,
				cflags,
				cstd,
				lastPlatformIncludes,
				[]string{"${config.NoOverrideGlobalCflags}", "${config.NoOverrideExternalGlobalCflags}"},
			),
		},
		{
			name: "cc",
			src:  "foo.cc",
			expectedNDK: slices.Concat(
				baseExpectedFlags,
				expectedTargetNDKFlags,
				cppOnly,
				expectedIncludes,
				expectedNDKSTLIncludes,
				cflags,
				cppstd,
				lastNDKFlags,
				[]string{"${config.NoOverrideGlobalCflags}", "${config.NoOverrideExternalGlobalCflags}"},
			),
			expectedPlatform: slices.Concat(
				expectedPlatformFlags,
				baseExpectedFlags,
				expectedTargetPlatformFlags,
				cppOnly,
				expectedIncludes,
				cflags,
				cppstd,
				lastPlatformIncludes,
				[]string{"${config.NoOverrideGlobalCflags}", "${config.NoOverrideExternalGlobalCflags}"},
			),
		},
		{
			name: "assemble",
			src:  "foo.s",
			expectedNDK: slices.Concat(
				baseExpectedFlags,
				expectedTargetNDKFlags,
				[]string{"${config.CommonGlobalAsflags}"},
				expectedIncludes,
				expectedNDKSTLIncludes,
				lastNDKFlags,
			),
			expectedPlatform: slices.Concat(
				expectedPlatformFlags,
				baseExpectedFlags,
				expectedTargetPlatformFlags,
				[]string{"${config.CommonGlobalAsflags}"},
				expectedIncludes,
				lastPlatformIncludes,
			),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bp := fmt.Sprintf(`
		cc_library {
			name: "libfoo",
			srcs: ["%s"],
			cflags: ["-std=candcpp"],
			conlyflags: ["-std=conly"],
			cppflags: ["-std=cpp"],
			local_include_dirs: ["local_include_dirs"],
			export_include_dirs: ["export_include_dirs"],
			export_system_include_dirs: ["export_system_include_dirs"],
			static_libs: ["libstatic1", "libstatic2"],
			whole_static_libs: ["libwhole1", "libwhole2"],
			shared_libs: ["libshared1", "libshared2"],
			header_libs: ["libheader1", "libheader2"],
			target: {
				android: {
					shared_libs: ["libandroid"],
					local_include_dirs: ["android_local_include_dirs"],
					export_include_dirs: ["android_export_include_dirs"],
				},
				android_arm: {
					shared_libs: ["libandroid_arm"],
					local_include_dirs: ["android_arm_local_include_dirs"],
					export_include_dirs: ["android_arm_export_include_dirs"],
				},
				linux: {
					shared_libs: ["liblinux"],
					local_include_dirs: ["linux_local_include_dirs"],
					export_include_dirs: ["linux_export_include_dirs"],
				},
			},
			multilib: {
				lib32: {
					shared_libs: ["lib32"],
					local_include_dirs: ["lib32_local_include_dirs"],
					export_include_dirs: ["lib32_export_include_dirs"],
				},
			},
			arch: {
				arm: {
					shared_libs: ["libarm"],
					local_include_dirs: ["arm_local_include_dirs"],
					export_include_dirs: ["arm_export_include_dirs"],
				},
			},
			stl: "libc++",
			sdk_version: "minimum",
		}

		cc_library_headers {
			name: "libheader1",
			export_include_dirs: ["libheader1"],
			sdk_version: "minimum",
			stl: "none",
		}

		cc_library_headers {
			name: "libheader2",
			export_include_dirs: ["libheader2"],
			sdk_version: "minimum",
			stl: "none",
		}
	`, tc.src)

			libs := []string{
				"libstatic1",
				"libstatic2",
				"libwhole1",
				"libwhole2",
				"libshared1",
				"libshared2",
				"libandroid",
				"libandroid_arm",
				"liblinux",
				"lib32",
				"libarm",
			}

			for _, lib := range libs {
				bp += fmt.Sprintf(`
			cc_library {
				name: "%s",
				export_include_dirs: ["%s"],
				sdk_version: "minimum",
				stl: "none",
			}
		`, lib, lib)
			}

			runTest := func(t *testing.T, variant string, expected []string) {
				ctx := android.GroupFixturePreparers(
					PrepareForIntegrationTestWithCc,
					android.FixtureAddTextFile("external/foo/Android.bp", bp),
				).RunTest(t)
				cflags := ctx.ModuleForTests(t, "libfoo", variant).Output("obj/external/foo/foo.o").Args["cFlags"]

				var includes []string
				flags := strings.Split(cflags, " ")
				for _, flag := range flags {
					if strings.HasPrefix(flag, "-I") {
						includes = append(includes, strings.TrimPrefix(flag, "-I"))
					} else if flag == "-isystem" {
						// skip isystem, include next
					} else if len(flag) > 0 {
						includes = append(includes, flag)
					}
				}

				android.AssertArrayString(t, "includes", expected, includes)
			}

			t.Run("platform", func(t *testing.T) {
				runTest(t, "android_arm_armv7-a-neon_static", tc.expectedPlatform)
			})
			t.Run("ndk", func(t *testing.T) {
				runTest(t, "android_arm_armv7-a-neon_sdk_static", tc.expectedNDK)
			})
		})
	}

}

func TestAddnoOverride64GlobalCflags(t *testing.T) {
	t.Parallel()
	ctx := testCc(t, `
		cc_library_shared {
			name: "libclient",
			srcs: ["foo.c"],
			shared_libs: ["libfoo#1"],
		}

		cc_library_shared {
			name: "libfoo",
			srcs: ["foo.c"],
			shared_libs: ["libbar"],
			export_shared_lib_headers: ["libbar"],
			stubs: {
				symbol_file: "foo.map.txt",
				versions: ["1", "2", "3"],
			},
		}

		cc_library_shared {
			name: "libbar",
			export_include_dirs: ["include/libbar"],
			srcs: ["foo.c"],
		}`)

	cFlags := ctx.ModuleForTests(t, "libclient", "android_arm64_armv8-a_shared").Rule("cc").Args["cFlags"]

	if !strings.Contains(cFlags, "${config.NoOverride64GlobalCflags}") {
		t.Errorf("expected %q in cflags, got %q", "${config.NoOverride64GlobalCflags}", cFlags)
	}
}

func TestCcBuildBrokenClangProperty(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                     string
		clang                    bool
		BuildBrokenClangProperty bool
		err                      string
	}{
		{
			name:  "error when clang is set to false",
			clang: false,
			err:   "is no longer supported",
		},
		{
			name:  "error when clang is set to true",
			clang: true,
			err:   "property is deprecated, see Changes.md",
		},
		{
			name:                     "no error when BuildBrokenClangProperty is explicitly set to true",
			clang:                    true,
			BuildBrokenClangProperty: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bp := fmt.Sprintf(`
			cc_library {
			   name: "foo",
			   clang: %t,
			}`, test.clang)

			if test.err == "" {
				android.GroupFixturePreparers(
					prepareForCcTest,
					android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
						if test.BuildBrokenClangProperty {
							variables.BuildBrokenClangProperty = test.BuildBrokenClangProperty
						}
					}),
				).RunTestWithBp(t, bp)
			} else {
				prepareForCcTest.
					ExtendWithErrorHandler(android.FixtureExpectsOneErrorPattern(test.err)).
					RunTestWithBp(t, bp)
			}
		})
	}
}

func TestCcBuildBrokenClangAsFlags(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                    string
		clangAsFlags            []string
		BuildBrokenClangAsFlags bool
		err                     string
	}{
		{
			name:         "error when clang_asflags is set",
			clangAsFlags: []string{"-a", "-b"},
			err:          "clang_asflags: property is deprecated",
		},
		{
			name:                    "no error when BuildBrokenClangAsFlags is explicitly set to true",
			clangAsFlags:            []string{"-a", "-b"},
			BuildBrokenClangAsFlags: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bp := fmt.Sprintf(`
			cc_library {
			   name: "foo",
			   clang_asflags: %s,
			}`, `["`+strings.Join(test.clangAsFlags, `","`)+`"]`)

			if test.err == "" {
				android.GroupFixturePreparers(
					prepareForCcTest,
					android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
						if test.BuildBrokenClangAsFlags {
							variables.BuildBrokenClangAsFlags = test.BuildBrokenClangAsFlags
						}
					}),
				).RunTestWithBp(t, bp)
			} else {
				prepareForCcTest.
					ExtendWithErrorHandler(android.FixtureExpectsOneErrorPattern(test.err)).
					RunTestWithBp(t, bp)
			}
		})
	}
}

func TestCcBuildBrokenClangCFlags(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                   string
		clangCFlags            []string
		BuildBrokenClangCFlags bool
		err                    string
	}{
		{
			name:        "error when clang_cflags is set",
			clangCFlags: []string{"-a", "-b"},
			err:         "clang_cflags: property is deprecated",
		},
		{
			name:                   "no error when BuildBrokenClangCFlags is explicitly set to true",
			clangCFlags:            []string{"-a", "-b"},
			BuildBrokenClangCFlags: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bp := fmt.Sprintf(`
			cc_library {
			   name: "foo",
			   clang_cflags: %s,
			}`, `["`+strings.Join(test.clangCFlags, `","`)+`"]`)

			if test.err == "" {
				android.GroupFixturePreparers(
					prepareForCcTest,
					android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
						if test.BuildBrokenClangCFlags {
							variables.BuildBrokenClangCFlags = test.BuildBrokenClangCFlags
						}
					}),
				).RunTestWithBp(t, bp)
			} else {
				prepareForCcTest.
					ExtendWithErrorHandler(android.FixtureExpectsOneErrorPattern(test.err)).
					RunTestWithBp(t, bp)
			}
		})
	}
}

func TestStrippedAllOutputFile(t *testing.T) {
	t.Parallel()
	bp := `
		cc_library {
			name: "test_lib",
			srcs: ["test_lib.cpp"],
			dist: {
				targets: [ "dist_target" ],
				tag: "stripped_all",
			}
		}
 `
	config := TestConfig(t.TempDir(), android.Android, nil, bp, nil)
	ctx := testCcWithConfig(t, config)
	testingModule := ctx.ModuleForTests(t, "test_lib", "android_arm_armv7-a-neon_shared")
	outputFile := testingModule.OutputFiles(ctx, t, "stripped_all")
	if !strings.HasSuffix(outputFile.Strings()[0], "/stripped_all/test_lib.so") {
		t.Errorf("Unexpected output file: %s", outputFile.Strings()[0])
		return
	}
}

func TestImageVariants(t *testing.T) {
	t.Parallel()

	bp := `
	cc_binary {
		name: "binfoo",
		srcs: ["binfoo.cc"],
		vendor_available: true,
		product_available: true,
		shared_libs: ["libbar"]
	}
	cc_library {
		name: "libbar",
		srcs: ["libbar.cc"],
		vendor_available: true,
		product_available: true,
	}
	`

	ctx := prepareForCcTest.RunTestWithBp(t, bp)

	hasDep := func(m android.Module, wantDep android.Module) bool {
		t.Helper()
		var found bool
		ctx.VisitDirectDeps(m, func(dep blueprint.Module) {
			if dep == wantDep {
				found = true
			}
		})
		return found
	}

	testDepWithVariant := func(imageVariant string) {
		imageVariantStr := ""
		if imageVariant != "core" {
			imageVariantStr = "_" + imageVariant
		}
		binFooModule := ctx.ModuleForTests(t, "binfoo", "android"+imageVariantStr+"_arm64_armv8-a").Module()
		libBarModule := ctx.ModuleForTests(t, "libbar", "android"+imageVariantStr+"_arm64_armv8-a_shared").Module()
		android.AssertBoolEquals(t, "binfoo should have dependency on libbar with image variant "+imageVariant, true, hasDep(binFooModule, libBarModule))
	}

	testDepWithVariant("core")
	testDepWithVariant("vendor")
	testDepWithVariant("product")
}

func TestVendorOrProductVariantUsesPlatformSdkVersionAsDefault(t *testing.T) {
	t.Parallel()

	bp := `
		cc_library {
			name: "libfoo",
			srcs: ["libfoo.cc"],
			vendor_available: true,
			product_available: true,
		}

		cc_library {
			name: "libbar",
			srcs: ["libbar.cc"],
			vendor_available: true,
			product_available: true,
			min_sdk_version: "29",
		}
	`

	ctx := prepareForCcTest.RunTestWithBp(t, bp)
	testSdkVersionFlag := func(module, variant, version string) {
		flags := ctx.ModuleForTests(t, module, "android_"+variant+"_arm64_armv8-a_static").Rule("cc").Args["cFlags"]
		android.AssertStringDoesContain(t, "target SDK version", flags, "-target aarch64-linux-android"+version)
	}

	testSdkVersionFlag("libfoo", "vendor", "30")
	testSdkVersionFlag("libfoo", "product", "30")
	// target SDK version can be set explicitly with min_sdk_version
	testSdkVersionFlag("libbar", "vendor", "29")
	testSdkVersionFlag("libbar", "product", "29")
}

func TestClangVerify(t *testing.T) {
	t.Parallel()

	ctx := testCc(t, `
		cc_library {
			name: "lib_no_clang_verify",
			srcs: ["libnocv.cc"],
		}

		cc_library {
			name: "lib_clang_verify",
			srcs: ["libcv.cc"],
			clang_verify: true,
		}
	`)

	module := ctx.ModuleForTests(t, "lib_no_clang_verify", "android_arm64_armv8-a_shared")

	cFlags_no_cv := module.Rule("cc").Args["cFlags"]
	if strings.Contains(cFlags_no_cv, "-Xclang") || strings.Contains(cFlags_no_cv, "-verify") {
		t.Errorf("expected %q not in cflags, got %q", "-Xclang -verify", cFlags_no_cv)
	}

	cFlags_cv := ctx.ModuleForTests(t, "lib_clang_verify", "android_arm64_armv8-a_shared").Rule("cc").Args["cFlags"]
	if strings.Contains(cFlags_cv, "-Xclang") && strings.Contains(cFlags_cv, "-verify") {
		t.Errorf("expected %q in cflags, got %q", "-Xclang -verify", cFlags_cv)
	}
}

func TestCheckConflictingExplicitVersions(t *testing.T) {
	PrepareForIntegrationTestWithCc.
		ExtendWithErrorHandler(android.FixtureExpectsOneErrorPattern(
			`shared_libs: duplicate shared libraries with different explicit versions: "libbar" and "libbar#impl"`,
		)).
		RunTestWithBp(t, `
			cc_library {
				name: "libfoo",
				shared_libs: ["libbar", "libbar#impl"],
			}

			cc_library {
				name: "libbar",
			}
		`)
}
