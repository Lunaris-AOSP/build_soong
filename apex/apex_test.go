// Copyright 2018 Google Inc. All rights reserved.
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

package apex

import (
	"fmt"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"android/soong/aconfig/codegen"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/bpf"
	"android/soong/cc"
	"android/soong/dexpreopt"
	prebuilt_etc "android/soong/etc"
	"android/soong/filesystem"
	"android/soong/java"
	"android/soong/rust"
	"android/soong/sh"
)

// names returns name list from white space separated string
func names(s string) (ns []string) {
	for _, n := range strings.Split(s, " ") {
		if len(n) > 0 {
			ns = append(ns, n)
		}
	}
	return
}

func testApexError(t *testing.T, pattern, bp string, preparers ...android.FixturePreparer) {
	t.Helper()
	android.GroupFixturePreparers(
		prepareForApexTest,
		android.GroupFixturePreparers(preparers...),
	).
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(pattern)).
		RunTestWithBp(t, bp)
}

func testApex(t *testing.T, bp string, preparers ...android.FixturePreparer) *android.TestContext {
	t.Helper()

	optionalBpPreparer := android.NullFixturePreparer
	if bp != "" {
		optionalBpPreparer = android.FixtureWithRootAndroidBp(bp)
	}

	result := android.GroupFixturePreparers(
		prepareForApexTest,
		android.GroupFixturePreparers(preparers...),
		optionalBpPreparer,
	).RunTest(t)

	return result.TestContext
}

func withFiles(files android.MockFS) android.FixturePreparer {
	return files.AddToFixture()
}

func withManifestPackageNameOverrides(specs []string) android.FixturePreparer {
	return android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
		variables.ManifestPackageNameOverrides = specs
	})
}

func withApexGlobalMinSdkVersionOverride(minSdkOverride *string) android.FixturePreparer {
	return android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
		variables.ApexGlobalMinSdkVersionOverride = minSdkOverride
	})
}

var withBinder32bit = android.FixtureModifyProductVariables(
	func(variables android.FixtureProductVariables) {
		variables.Binder32bit = proptools.BoolPtr(true)
	},
)

var withUnbundledBuild = android.FixtureModifyProductVariables(
	func(variables android.FixtureProductVariables) {
		variables.Unbundled_build = proptools.BoolPtr(true)
	},
)

// Legacy preparer used for running tests within the apex package.
//
// This includes everything that was needed to run any test in the apex package prior to the
// introduction of the test fixtures. Tests that are being converted to use fixtures directly
// rather than through the testApex...() methods should avoid using this and instead use the
// various preparers directly, using android.GroupFixturePreparers(...) to group them when
// necessary.
//
// deprecated
var prepareForApexTest = android.GroupFixturePreparers(
	// General preparers in alphabetical order as test infrastructure will enforce correct
	// registration order.
	android.PrepareForTestWithAndroidBuildComponents,
	bpf.PrepareForTestWithBpf,
	cc.PrepareForTestWithCcBuildComponents,
	java.PrepareForTestWithDexpreopt,
	prebuilt_etc.PrepareForTestWithPrebuiltEtc,
	rust.PrepareForTestWithRustDefaultModules,
	sh.PrepareForTestWithShBuildComponents,
	codegen.PrepareForTestWithAconfigBuildComponents,

	PrepareForTestWithApexBuildComponents,

	// Additional apex test specific preparers.
	android.FixtureAddTextFile("system/sepolicy/Android.bp", `
		filegroup {
			name: "myapex-file_contexts",
			srcs: [
				"apex/myapex-file_contexts",
			],
		}
	`),
	prepareForTestWithMyapex,
	android.FixtureMergeMockFs(android.MockFS{
		"a.java":                 nil,
		"PrebuiltAppFoo.apk":     nil,
		"PrebuiltAppFooPriv.apk": nil,
		"apex_manifest.json":     nil,
		"AndroidManifest.xml":    nil,
		"system/sepolicy/apex/myapex.updatable-file_contexts":         nil,
		"system/sepolicy/apex/myapex2-file_contexts":                  nil,
		"system/sepolicy/apex/otherapex-file_contexts":                nil,
		"system/sepolicy/apex/com.android.vndk-file_contexts":         nil,
		"system/sepolicy/apex/com.android.vndk.current-file_contexts": nil,
		"mylib.cpp":                            nil,
		"mytest.cpp":                           nil,
		"mytest1.cpp":                          nil,
		"mytest2.cpp":                          nil,
		"mytest3.cpp":                          nil,
		"myprebuilt":                           nil,
		"my_include":                           nil,
		"foo/bar/MyClass.java":                 nil,
		"prebuilt.jar":                         nil,
		"prebuilt.so":                          nil,
		"vendor/foo/devkeys/test.x509.pem":     nil,
		"vendor/foo/devkeys/test.pk8":          nil,
		"testkey.x509.pem":                     nil,
		"testkey.pk8":                          nil,
		"testkey.override.x509.pem":            nil,
		"testkey.override.pk8":                 nil,
		"vendor/foo/devkeys/testkey.avbpubkey": nil,
		"vendor/foo/devkeys/testkey.pem":       nil,
		"NOTICE":                               nil,
		"custom_notice":                        nil,
		"custom_notice_for_static_lib":         nil,
		"testkey2.avbpubkey":                   nil,
		"testkey2.pem":                         nil,
		"myapex-arm64.apex":                    nil,
		"myapex-arm.apex":                      nil,
		"myapex.apks":                          nil,
		"frameworks/base/api/current.txt":      nil,
		"framework/aidl/a.aidl":                nil,
		"dummy.txt":                            nil,
		"baz":                                  nil,
		"bar/baz":                              nil,
		"testdata/baz":                         nil,
		"AppSet.apks":                          nil,
		"foo.rs":                               nil,
		"libfoo.jar":                           nil,
		"libbar.jar":                           nil,
	},
	),

	android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
		variables.DefaultAppCertificate = proptools.StringPtr("vendor/foo/devkeys/test")
		variables.CertificateOverrides = []string{"myapex_keytest:myapex.certificate.override"}
		variables.Platform_sdk_codename = proptools.StringPtr("Q")
		variables.Platform_sdk_final = proptools.BoolPtr(false)
		// "Tiramisu" needs to be in the next line for compatibility with soong code,
		// not because of these tests specifically (it's not used by the tests)
		variables.Platform_version_active_codenames = []string{"Q", "Tiramisu"}
		variables.BuildId = proptools.StringPtr("TEST.BUILD_ID")
	}),
)

var prepareForTestWithMyapex = android.FixtureMergeMockFs(android.MockFS{
	"system/sepolicy/apex/myapex-file_contexts": nil,
})

var prepareForTestWithOtherapex = android.FixtureMergeMockFs(android.MockFS{
	"system/sepolicy/apex/otherapex-file_contexts": nil,
})

// ensure that 'result' equals 'expected'
func ensureEquals(t *testing.T, result string, expected string) {
	t.Helper()
	if result != expected {
		t.Errorf("%q != %q", expected, result)
	}
}

// ensure that 'result' contains 'expected'
func ensureContains(t *testing.T, result string, expected string) {
	t.Helper()
	if !strings.Contains(result, expected) {
		t.Errorf("%q is not found in %q", expected, result)
	}
}

// ensure that 'result' contains 'expected' exactly one time
func ensureContainsOnce(t *testing.T, result string, expected string) {
	t.Helper()
	count := strings.Count(result, expected)
	if count != 1 {
		t.Errorf("%q is found %d times (expected 1 time) in %q", expected, count, result)
	}
}

// ensures that 'result' does not contain 'notExpected'
func ensureNotContains(t *testing.T, result string, notExpected string) {
	t.Helper()
	if strings.Contains(result, notExpected) {
		t.Errorf("%q is found in %q", notExpected, result)
	}
}

func ensureMatches(t *testing.T, result string, expectedRex string) {
	t.Helper()
	ok, err := regexp.MatchString(expectedRex, result)
	if err != nil {
		t.Fatalf("regexp failure trying to match %s against `%s` expression: %s", result, expectedRex, err)
		return
	}
	if !ok {
		t.Errorf("%s does not match regular expession %s", result, expectedRex)
	}
}

func ensureListContainsMatch(t *testing.T, result []string, expectedRex string) {
	t.Helper()
	p := regexp.MustCompile(expectedRex)
	if android.IndexListPred(func(s string) bool { return p.MatchString(s) }, result) == -1 {
		t.Errorf("%q is not found in %v", expectedRex, result)
	}
}

func ensureListContains(t *testing.T, result []string, expected string) {
	t.Helper()
	if !android.InList(expected, result) {
		t.Errorf("%q is not found in %v", expected, result)
	}
}

func ensureListNotContains(t *testing.T, result []string, notExpected string) {
	t.Helper()
	if android.InList(notExpected, result) {
		t.Errorf("%q is found in %v", notExpected, result)
	}
}

func ensureListEmpty(t *testing.T, result []string) {
	t.Helper()
	if len(result) > 0 {
		t.Errorf("%q is expected to be empty", result)
	}
}

func ensureListNotEmpty(t *testing.T, result []string) {
	t.Helper()
	if len(result) == 0 {
		t.Errorf("%q is expected to be not empty", result)
	}
}

// Minimal test
func TestBasicApex(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex_defaults {
			name: "myapex-defaults",
			manifest: ":myapex.manifest",
			androidManifest: ":myapex.androidmanifest",
			key: "myapex.key",
			binaries: ["foo.rust"],
			native_shared_libs: [
				"mylib",
				"libfoo.ffi",
			],
			rust_dyn_libs: ["libfoo.dylib.rust"],
			multilib: {
				both: {
					binaries: ["foo"],
				}
			},
			java_libs: [
				"myjar",
				"myjar_dex",
			],
			updatable: false,
		}

		apex {
			name: "myapex",
			defaults: ["myapex-defaults"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		filegroup {
			name: "myapex.manifest",
			srcs: ["apex_manifest.json"],
		}

		filegroup {
			name: "myapex.androidmanifest",
			srcs: ["AndroidManifest.xml"],
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: [
				"mylib2",
				"libbar.ffi",
			],
			system_shared_libs: [],
			stl: "none",
			// TODO: remove //apex_available:platform
			apex_available: [
				"//apex_available:platform",
				"myapex",
			],
		}

		cc_binary {
			name: "foo",
			srcs: ["mylib.cpp"],
			compile_multilib: "both",
			multilib: {
					lib32: {
							suffix: "32",
					},
					lib64: {
							suffix: "64",
					},
			},
			symlinks: ["foo_link_"],
			symlink_preferred_arch: true,
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		rust_binary {
			name: "foo.rust",
			srcs: ["foo.rs"],
			rlibs: ["libfoo.rlib.rust"],
			rustlibs: ["libfoo.transitive.dylib.rust"],
			apex_available: ["myapex"],
		}

		rust_library_rlib {
			name: "libfoo.rlib.rust",
			srcs: ["foo.rs"],
			crate_name: "foo",
			apex_available: ["myapex"],
			shared_libs: ["libfoo.shared_from_rust"],
		}

		cc_library_shared {
			name: "libfoo.shared_from_rust",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: ["myapex"],
		}

		rust_library_dylib {
			name: "libfoo.dylib.rust",
			srcs: ["foo.rs"],
			crate_name: "foo",
			apex_available: ["myapex"],
		}

		rust_library_dylib {
			name: "libfoo.transitive.dylib.rust",
			srcs: ["foo.rs"],
			crate_name: "foo",
			apex_available: ["myapex"],
		}

		rust_ffi_shared {
			name: "libfoo.ffi",
			srcs: ["foo.rs"],
			crate_name: "foo",
			apex_available: ["myapex"],
		}

		rust_ffi_shared {
			name: "libbar.ffi",
			srcs: ["foo.rs"],
			crate_name: "bar",
			apex_available: ["myapex"],
		}

		cc_library_shared {
			name: "mylib2",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			static_libs: ["libstatic"],
			// TODO: remove //apex_available:platform
			apex_available: [
				"//apex_available:platform",
				"myapex",
			],
		}

		cc_prebuilt_library_shared {
			name: "mylib2",
			srcs: ["prebuilt.so"],
			// TODO: remove //apex_available:platform
			apex_available: [
				"//apex_available:platform",
				"myapex",
			],
		}

		cc_library_static {
			name: "libstatic",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			// TODO: remove //apex_available:platform
			apex_available: [
				"//apex_available:platform",
				"myapex",
			],
		}

		java_library {
			name: "myjar",
			srcs: ["foo/bar/MyClass.java"],
			stem: "myjar_stem",
			sdk_version: "none",
			system_modules: "none",
			static_libs: ["myotherjar"],
			libs: ["mysharedjar"],
			// TODO: remove //apex_available:platform
			apex_available: [
				"//apex_available:platform",
				"myapex",
			],
			compile_dex: true,
		}

		dex_import {
			name: "myjar_dex",
			jars: ["prebuilt.jar"],
			apex_available: [
				"//apex_available:platform",
				"myapex",
			],
		}

		java_library {
			name: "myotherjar",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			// TODO: remove //apex_available:platform
			apex_available: [
				"//apex_available:platform",
				"myapex",
			],
		}

		java_library {
			name: "mysharedjar",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
		}
	`)

	apexRule := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Rule("apexRule")

	// Make sure that Android.mk is created
	ab := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Module().(*apexBundle)
	data := android.AndroidMkDataForTest(t, ctx, ab)
	var builder strings.Builder
	data.Custom(&builder, ab.BaseModuleName(), "TARGET_", "", data)

	androidMk := builder.String()
	ensureContains(t, androidMk, "LOCAL_MODULE := mylib.myapex\n")
	ensureNotContains(t, androidMk, "LOCAL_MODULE := mylib.com.android.myapex\n")

	optFlags := apexRule.Args["opt_flags"]
	ensureContains(t, optFlags, "--pubkey vendor/foo/devkeys/testkey.avbpubkey")
	// Ensure that the NOTICE output is being packaged as an asset.
	ensureContains(t, optFlags, "--assets_dir out/soong/.intermediates/myapex/android_common_myapex/NOTICE")

	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that main rule creates an output
	ensureContains(t, apexRule.Output.String(), "myapex.apex.unsigned")

	// Ensure that apex variant is created for the direct dep
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib"), "android_arm64_armv8-a_shared_apex10000")
	ensureListContains(t, ctx.ModuleVariantsForTests("myjar"), "android_common_apex10000")
	ensureListContains(t, ctx.ModuleVariantsForTests("myjar_dex"), "android_common_apex10000")
	ensureListContains(t, ctx.ModuleVariantsForTests("foo.rust"), "android_arm64_armv8-a_apex10000")
	ensureListContains(t, ctx.ModuleVariantsForTests("libfoo.ffi"), "android_arm64_armv8-a_shared_apex10000")

	// Ensure that apex variant is created for the indirect dep
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib2"), "android_arm64_armv8-a_shared_apex10000")
	ensureListContains(t, ctx.ModuleVariantsForTests("myotherjar"), "android_common_apex10000")
	ensureListContains(t, ctx.ModuleVariantsForTests("libfoo.rlib.rust"), "android_arm64_armv8-a_rlib_dylib-std_apex10000")
	ensureListContains(t, ctx.ModuleVariantsForTests("libfoo.dylib.rust"), "android_arm64_armv8-a_dylib_apex10000")
	ensureListContains(t, ctx.ModuleVariantsForTests("libfoo.transitive.dylib.rust"), "android_arm64_armv8-a_dylib_apex10000")
	ensureListContains(t, ctx.ModuleVariantsForTests("libbar.ffi"), "android_arm64_armv8-a_shared_apex10000")
	ensureListContains(t, ctx.ModuleVariantsForTests("libfoo.shared_from_rust"), "android_arm64_armv8-a_shared_apex10000")

	// Ensure that both direct and indirect deps are copied into apex
	ensureContains(t, copyCmds, "image.apex/lib64/mylib.so")
	ensureContains(t, copyCmds, "image.apex/lib64/mylib2.so")
	ensureContains(t, copyCmds, "image.apex/javalib/myjar_stem.jar")
	ensureContains(t, copyCmds, "image.apex/javalib/myjar_dex.jar")
	ensureContains(t, copyCmds, "image.apex/lib64/libfoo.dylib.rust.dylib.so")
	ensureContains(t, copyCmds, "image.apex/lib64/libfoo.transitive.dylib.rust.dylib.so")
	ensureContains(t, copyCmds, "image.apex/lib64/libfoo.ffi.so")
	ensureContains(t, copyCmds, "image.apex/lib64/libbar.ffi.so")
	ensureContains(t, copyCmds, "image.apex/lib64/libfoo.shared_from_rust.so")
	// .. but not for java libs
	ensureNotContains(t, copyCmds, "image.apex/javalib/myotherjar.jar")
	ensureNotContains(t, copyCmds, "image.apex/javalib/msharedjar.jar")

	// Ensure that the platform variant ends with _shared or _common
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib"), "android_arm64_armv8-a_shared")
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib2"), "android_arm64_armv8-a_shared")
	ensureListContains(t, ctx.ModuleVariantsForTests("myjar"), "android_common")
	ensureListContains(t, ctx.ModuleVariantsForTests("myotherjar"), "android_common")
	ensureListContains(t, ctx.ModuleVariantsForTests("mysharedjar"), "android_common")

	// Ensure that dynamic dependency to java libs are not included
	ensureListNotContains(t, ctx.ModuleVariantsForTests("mysharedjar"), "android_common_myapex")

	// Ensure that all symlinks are present.
	found_foo_link_64 := false
	found_foo := false
	for _, cmd := range strings.Split(copyCmds, " && ") {
		if strings.HasPrefix(cmd, "ln -sfn foo64") {
			if strings.HasSuffix(cmd, "bin/foo") {
				found_foo = true
			} else if strings.HasSuffix(cmd, "bin/foo_link_64") {
				found_foo_link_64 = true
			}
		}
	}
	good := found_foo && found_foo_link_64
	if !good {
		t.Errorf("Could not find all expected symlinks! foo: %t, foo_link_64: %t. Command was %s", found_foo, found_foo_link_64, copyCmds)
	}

	fullDepsInfo := strings.Split(android.ContentFromFileRuleForTests(t, ctx,
		ctx.ModuleForTests(t, "myapex", "android_common_myapex").Output("depsinfo/fulllist.txt")), "\n")
	ensureListContains(t, fullDepsInfo, "  myjar(minSdkVersion:(no version)) <- myapex")
	ensureListContains(t, fullDepsInfo, "  mylib2(minSdkVersion:(no version)) <- mylib")
	ensureListContains(t, fullDepsInfo, "  myotherjar(minSdkVersion:(no version)) <- myjar")
	ensureListContains(t, fullDepsInfo, "  mysharedjar(minSdkVersion:(no version)) (external) <- myjar")

	flatDepsInfo := strings.Split(android.ContentFromFileRuleForTests(t, ctx,
		ctx.ModuleForTests(t, "myapex", "android_common_myapex").Output("depsinfo/flatlist.txt")), "\n")
	ensureListContains(t, flatDepsInfo, "myjar(minSdkVersion:(no version))")
	ensureListContains(t, flatDepsInfo, "mylib2(minSdkVersion:(no version))")
	ensureListContains(t, flatDepsInfo, "myotherjar(minSdkVersion:(no version))")
	ensureListContains(t, flatDepsInfo, "mysharedjar(minSdkVersion:(no version)) (external)")
}

func TestDefaults(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex_defaults {
			name: "myapex-defaults",
			key: "myapex.key",
			prebuilts: ["myetc"],
			native_shared_libs: ["mylib"],
			java_libs: ["myjar"],
			apps: ["AppFoo"],
			rros: ["rro"],
			bpfs: ["bpf", "netdTest"],
			updatable: false,
		}

		prebuilt_etc {
			name: "myetc",
			src: "myprebuilt",
		}

		apex {
			name: "myapex",
			defaults: ["myapex-defaults"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		java_library {
			name: "myjar",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			apex_available: [ "myapex" ],
			compile_dex: true,
		}

		android_app {
			name: "AppFoo",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			apex_available: [ "myapex" ],
		}

		runtime_resource_overlay {
			name: "rro",
			theme: "blue",
		}

		bpf {
			name: "bpf",
			srcs: ["bpf.c", "bpf2.c"],
		}

		bpf {
			name: "netdTest",
			srcs: ["netdTest.c"],
			sub_dir: "netd",
		}

	`)
	ensureExactContents(t, ctx, "myapex", "android_common_myapex", []string{
		"etc/myetc",
		"javalib/myjar.jar",
		"lib64/mylib.so",
		"app/AppFoo@TEST.BUILD_ID/AppFoo.apk",
		"overlay/blue/rro.apk",
		"etc/bpf/bpf.o",
		"etc/bpf/bpf2.o",
		"etc/bpf/netd/netdTest.o",
	})
}

func TestApexManifest(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`)

	module := ctx.ModuleForTests(t, "myapex", "android_common_myapex")
	args := module.Rule("apexRule").Args
	if manifest := args["manifest"]; manifest != module.Output("apex_manifest.pb").Output.String() {
		t.Error("manifest should be apex_manifest.pb, but " + manifest)
	}
}

func TestApexManifestMinSdkVersion(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex_defaults {
			name: "my_defaults",
			key: "myapex.key",
			product_specific: true,
			file_contexts: ":my-file-contexts",
			updatable: false,
		}
		apex {
			name: "myapex_30",
			min_sdk_version: "30",
			defaults: ["my_defaults"],
		}

		apex {
			name: "myapex_current",
			min_sdk_version: "current",
			defaults: ["my_defaults"],
		}

		apex {
			name: "myapex_none",
			defaults: ["my_defaults"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		filegroup {
			name: "my-file-contexts",
			srcs: ["product_specific_file_contexts"],
		}
	`, withFiles(map[string][]byte{
		"product_specific_file_contexts": nil,
	}), android.FixtureModifyProductVariables(
		func(variables android.FixtureProductVariables) {
			variables.Unbundled_build = proptools.BoolPtr(true)
			variables.Always_use_prebuilt_sdks = proptools.BoolPtr(false)
		}), android.FixtureMergeEnv(map[string]string{
		"UNBUNDLED_BUILD_TARGET_SDK_WITH_API_FINGERPRINT": "true",
	}))

	testCases := []struct {
		module        string
		minSdkVersion string
	}{
		{
			module:        "myapex_30",
			minSdkVersion: "30",
		},
		{
			module:        "myapex_current",
			minSdkVersion: "Q.$$(cat out/soong/api_fingerprint.txt)",
		},
		{
			module:        "myapex_none",
			minSdkVersion: "Q.$$(cat out/soong/api_fingerprint.txt)",
		},
	}
	for _, tc := range testCases {
		module := ctx.ModuleForTests(t, tc.module, "android_common_"+tc.module)
		args := module.Rule("apexRule").Args
		optFlags := args["opt_flags"]
		if !strings.Contains(optFlags, "--min_sdk_version "+tc.minSdkVersion) {
			t.Errorf("%s: Expected min_sdk_version=%s, got: %s", tc.module, tc.minSdkVersion, optFlags)
		}
	}
}

func TestApexWithDessertSha(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex_defaults {
			name: "my_defaults",
			key: "myapex.key",
			product_specific: true,
			file_contexts: ":my-file-contexts",
			updatable: false,
		}
		apex {
			name: "myapex_30",
			min_sdk_version: "30",
			defaults: ["my_defaults"],
		}

		apex {
			name: "myapex_current",
			min_sdk_version: "current",
			defaults: ["my_defaults"],
		}

		apex {
			name: "myapex_none",
			defaults: ["my_defaults"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		filegroup {
			name: "my-file-contexts",
			srcs: ["product_specific_file_contexts"],
		}
	`, withFiles(map[string][]byte{
		"product_specific_file_contexts": nil,
	}), android.FixtureModifyProductVariables(
		func(variables android.FixtureProductVariables) {
			variables.Unbundled_build = proptools.BoolPtr(true)
			variables.Always_use_prebuilt_sdks = proptools.BoolPtr(false)
		}), android.FixtureMergeEnv(map[string]string{
		"UNBUNDLED_BUILD_TARGET_SDK_WITH_DESSERT_SHA": "UpsideDownCake.abcdefghijklmnopqrstuvwxyz123456",
	}))

	testCases := []struct {
		module        string
		minSdkVersion string
	}{
		{
			module:        "myapex_30",
			minSdkVersion: "30",
		},
		{
			module:        "myapex_current",
			minSdkVersion: "UpsideDownCake.abcdefghijklmnopqrstuvwxyz123456",
		},
		{
			module:        "myapex_none",
			minSdkVersion: "UpsideDownCake.abcdefghijklmnopqrstuvwxyz123456",
		},
	}
	for _, tc := range testCases {
		module := ctx.ModuleForTests(t, tc.module, "android_common_"+tc.module)
		args := module.Rule("apexRule").Args
		optFlags := args["opt_flags"]
		if !strings.Contains(optFlags, "--min_sdk_version "+tc.minSdkVersion) {
			t.Errorf("%s: Expected min_sdk_version=%s, got: %s", tc.module, tc.minSdkVersion, optFlags)
		}
	}
}

func TestFileContexts(t *testing.T) {
	t.Parallel()
	for _, vendor := range []bool{true, false} {
		prop := ""
		if vendor {
			prop = "vendor: true,\n"
		}
		ctx := testApex(t, `
			apex {
				name: "myapex",
				key: "myapex.key",
				updatable: false,
				`+prop+`
			}

			apex_key {
				name: "myapex.key",
				public_key: "testkey.avbpubkey",
				private_key: "testkey.pem",
			}
		`)

		rule := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Output("file_contexts")
		if vendor {
			android.AssertStringDoesContain(t, "should force-label as vendor_apex_metadata_file",
				rule.RuleParams.Command,
				"apex_manifest\\\\.pb u:object_r:vendor_apex_metadata_file:s0")
		} else {
			android.AssertStringDoesContain(t, "should force-label as system_file",
				rule.RuleParams.Command,
				"apex_manifest\\\\.pb u:object_r:system_file:s0")
		}
	}
}

func TestApexWithStubs(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: [
				"mylib",
				"mylib3",
				"libmylib3_rs",
			],
			binaries: ["foo.rust"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: [
				"mylib2",
				"mylib3#impl",
				"libmylib2_rs",
				"libmylib3_rs#impl",
				"my_prebuilt_platform_lib",
				"my_prebuilt_platform_stub_only_lib",
			],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "mylib2",
			srcs: ["mylib.cpp"],
			cflags: ["-include mylib.h"],
			system_shared_libs: [],
			stl: "none",
			stubs: {
				symbol_file: "mylib2.map.txt",
				versions: ["1", "2", "3"],
			},
		}

		rust_ffi {
			name: "libmylib2_rs",
			crate_name: "mylib2",
			srcs: ["mylib.rs"],
			stubs: {
				symbol_file: "mylib2.map.txt",
				versions: ["1", "2", "3"],
			},
		}

		cc_library {
			name: "mylib3",
			srcs: ["mylib.cpp"],
			shared_libs: ["mylib4"],
			system_shared_libs: [],
			stl: "none",
			stubs: {
				symbol_file: "mylib3.map.txt",
				versions: ["10", "11", "12"],
			},
			apex_available: [ "myapex" ],
		}

		rust_ffi {
			name: "libmylib3_rs",
			crate_name: "mylib3",
			srcs: ["mylib.rs"],
			shared_libs: ["mylib4.from_rust"],
			stubs: {
				symbol_file: "mylib3.map.txt",
				versions: ["10", "11", "12"],
			},
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "mylib4",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "mylib4.from_rust",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_prebuilt_library_shared {
			name: "my_prebuilt_platform_lib",
			stubs: {
				symbol_file: "my_prebuilt_platform_lib.map.txt",
				versions: ["1", "2", "3"],
			},
			srcs: ["foo.so"],
		}

		// Similar to my_prebuilt_platform_lib, but this library only provides stubs, i.e. srcs is empty
		cc_prebuilt_library_shared {
			name: "my_prebuilt_platform_stub_only_lib",
			stubs: {
				symbol_file: "my_prebuilt_platform_stub_only_lib.map.txt",
				versions: ["1", "2", "3"],
			}
		}

		rust_binary {
			name: "foo.rust",
			srcs: ["foo.rs"],
			shared_libs: [
				"libfoo.shared_from_rust",
				"libfoo_rs.shared_from_rust",
			],
			prefer_rlib: true,
			apex_available: ["myapex"],
		}

		cc_library_shared {
			name: "libfoo.shared_from_rust",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["10", "11", "12"],
			},
		}

		rust_ffi {
			name: "libfoo_rs.shared_from_rust",
			crate_name: "foo_rs",
			srcs: ["mylib.rs"],
			stubs: {
				versions: ["10", "11", "12"],
			},
		}

	`)

	apexRule := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that direct non-stubs dep is always included
	ensureContains(t, copyCmds, "image.apex/lib64/mylib.so")

	// Ensure that indirect stubs dep is not included
	ensureNotContains(t, copyCmds, "image.apex/lib64/mylib2.so")
	ensureNotContains(t, copyCmds, "image.apex/lib64/libmylib2_rs.so")

	// Ensure that direct stubs dep is included
	ensureContains(t, copyCmds, "image.apex/lib64/mylib3.so")
	ensureContains(t, copyCmds, "image.apex/lib64/libmylib3_rs.so")

	mylibLdFlags := ctx.ModuleForTests(t, "mylib", "android_arm64_armv8-a_shared_apex10000").Rule("ld").Args["libFlags"]

	// Ensure that mylib is linking with the latest version of stubs for mylib2
	ensureContains(t, mylibLdFlags, "mylib2/android_arm64_armv8-a_shared_current/mylib2.so")
	ensureContains(t, mylibLdFlags, "libmylib2_rs/android_arm64_armv8-a_shared_current/unstripped/libmylib2_rs.so")
	// ... and not linking to the non-stub (impl) variant of mylib2
	ensureNotContains(t, mylibLdFlags, "mylib2/android_arm64_armv8-a_shared/mylib2.so")
	ensureNotContains(t, mylibLdFlags, "libmylib2_rs/android_arm64_armv8-a_shared/unstripped/libmylib2_rs.so")

	// Ensure that mylib is linking with the non-stub (impl) of mylib3 (because the dependency is added with mylib3#impl)
	ensureContains(t, mylibLdFlags, "mylib3/android_arm64_armv8-a_shared_apex10000/mylib3.so")
	ensureContains(t, mylibLdFlags, "libmylib3_rs/android_arm64_armv8-a_shared_apex10000/unstripped/libmylib3_rs.so")
	// .. and not linking to the stubs variant of mylib3
	ensureNotContains(t, mylibLdFlags, "mylib3/android_arm64_armv8-a_shared_12/mylib3.so")
	ensureNotContains(t, mylibLdFlags, "libmylib3_rs/android_arm64_armv8-a_shared_12/unstripped/mylib3.so")

	// Comment out this test. Now it fails after the optimization of sharing "cflags" in cc/cc.go
	// is replaced by sharing of "cFlags" in cc/builder.go.
	// The "cflags" contains "-include mylib.h", but cFlags contained only a reference to the
	// module variable representing "cflags". So it was not detected by ensureNotContains.
	// Now "cFlags" is a reference to a module variable like $flags1, which includes all previous
	// content of "cflags". ModuleForTests...Args["cFlags"] returns the full string of $flags1,
	// including the original cflags's "-include mylib.h".
	//
	// Ensure that stubs libs are built without -include flags
	// mylib2Cflags := ctx.ModuleForTests(t, "mylib2", "android_arm64_armv8-a_static").Rule("cc").Args["cFlags"]
	// ensureNotContains(t, mylib2Cflags, "-include ")

	// Ensure that genstub for platform-provided lib is invoked with --systemapi
	ensureContains(t, ctx.ModuleForTests(t, "mylib2", "android_arm64_armv8-a_shared_3").Rule("genStubSrc").Args["flags"], "--systemapi")
	ensureContains(t, ctx.ModuleForTests(t, "libmylib2_rs", "android_arm64_armv8-a_shared_3").Rule("genStubSrc").Args["flags"], "--systemapi")
	// Ensure that genstub for apex-provided lib is invoked with --apex
	ensureContains(t, ctx.ModuleForTests(t, "mylib3", "android_arm64_armv8-a_shared_12").Rule("genStubSrc").Args["flags"], "--apex")
	ensureContains(t, ctx.ModuleForTests(t, "libmylib3_rs", "android_arm64_armv8-a_shared_12").Rule("genStubSrc").Args["flags"], "--apex")

	ensureExactContents(t, ctx, "myapex", "android_common_myapex", []string{
		"lib64/mylib.so",
		"lib64/mylib3.so",
		"lib64/libmylib3_rs.so",
		"lib64/mylib4.so",
		"lib64/mylib4.from_rust.so",
		"bin/foo.rust",

		"lib64/libstd.dylib.so", // implicit rust ffi dep
	})

	// Ensure that stub dependency from a rust module is not included
	ensureNotContains(t, copyCmds, "image.apex/lib64/libfoo.shared_from_rust.so")
	ensureNotContains(t, copyCmds, "image.apex/lib64/libfoo_rs.shared_from_rust.so")
	// The rust module is linked to the stub cc library
	rustDeps := ctx.ModuleForTests(t, "foo.rust", "android_arm64_armv8-a_apex10000").Rule("rustc").Args["linkFlags"]
	ensureContains(t, rustDeps, "libfoo.shared_from_rust/android_arm64_armv8-a_shared_current/libfoo.shared_from_rust.so")
	ensureContains(t, rustDeps, "libfoo_rs.shared_from_rust/android_arm64_armv8-a_shared_current/unstripped/libfoo_rs.shared_from_rust.so")
	ensureNotContains(t, rustDeps, "libfoo.shared_from_rust/android_arm64_armv8-a_shared/libfoo.shared_from_rust.so")
	ensureNotContains(t, rustDeps, "libfoo_rs.shared_from_rust/android_arm64_armv8-a_shared/unstripped/libfoo_rs.shared_from_rust.so")

	apexManifestRule := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Rule("apexManifestRule")
	ensureListContains(t, names(apexManifestRule.Args["requireNativeLibs"]), "libfoo.shared_from_rust.so")
	ensureListContains(t, names(apexManifestRule.Args["requireNativeLibs"]), "libfoo_rs.shared_from_rust.so")

	// Ensure that mylib is linking with the latest version of stubs for my_prebuilt_platform_lib
	ensureContains(t, mylibLdFlags, "my_prebuilt_platform_lib/android_arm64_armv8-a_shared_current/my_prebuilt_platform_lib.so")
	// ... and not linking to the non-stub (impl) variant of my_prebuilt_platform_lib
	ensureNotContains(t, mylibLdFlags, "my_prebuilt_platform_lib/android_arm64_armv8-a_shared/my_prebuilt_platform_lib.so")
	// Ensure that genstub for platform-provided lib is invoked with --systemapi
	ensureContains(t, ctx.ModuleForTests(t, "my_prebuilt_platform_lib", "android_arm64_armv8-a_shared_3").Rule("genStubSrc").Args["flags"], "--systemapi")

	// Ensure that mylib is linking with the latest version of stubs for my_prebuilt_platform_lib
	ensureContains(t, mylibLdFlags, "my_prebuilt_platform_stub_only_lib/android_arm64_armv8-a_shared_current/my_prebuilt_platform_stub_only_lib.so")
	// ... and not linking to the non-stub (impl) variant of my_prebuilt_platform_lib
	ensureNotContains(t, mylibLdFlags, "my_prebuilt_platform_stub_only_lib/android_arm64_armv8-a_shared/my_prebuilt_platform_stub_only_lib.so")
	// Ensure that genstub for platform-provided lib is invoked with --systemapi
	ensureContains(t, ctx.ModuleForTests(t, "my_prebuilt_platform_stub_only_lib", "android_arm64_armv8-a_shared_3").Rule("genStubSrc").Args["flags"], "--systemapi")
}

func TestApexShouldNotEmbedStubVariant(t *testing.T) {
	t.Parallel()
	testApexError(t, `module "myapex" .*: native_shared_libs: "libbar" is a stub`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			vendor: true,
			updatable: false,
			native_shared_libs: ["libbar"], // should not add an LLNDK stub in a vendor apex
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "libbar",
			srcs: ["mylib.cpp"],
			llndk: {
				symbol_file: "libbar.map.txt",
			}
		}
	`)
}

func TestApexCanUsePrivateApis(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			binaries: ["foo.rust"],
			updatable: false,
			platform_apis: true,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: [
				"mylib2",
				"libmylib2_rust"
			],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "mylib2",
			srcs: ["mylib.cpp"],
			cflags: ["-include mylib.h"],
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["1", "2", "3"],
			},
		}

		rust_ffi {
			name: "libmylib2_rust",
			crate_name: "mylib2_rust",
			srcs: ["mylib.rs"],
			stubs: {
				versions: ["1", "2", "3"],
			},
		}

		rust_binary {
			name: "foo.rust",
			srcs: ["foo.rs"],
			shared_libs: [
				"libfoo.shared_from_rust",
				"libmylib_rust.shared_from_rust"
			],
			prefer_rlib: true,
			apex_available: ["myapex"],
		}

		cc_library_shared {
			name: "libfoo.shared_from_rust",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["10", "11", "12"],
			},
		}
		rust_ffi {
			name: "libmylib_rust.shared_from_rust",
			crate_name: "mylib_rust",
			srcs: ["mylib.rs"],
			stubs: {
				versions: ["1", "2", "3"],
			},
		}

	`)

	apexRule := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that indirect stubs dep is not included
	ensureNotContains(t, copyCmds, "image.apex/lib64/mylib2.so")
	ensureNotContains(t, copyCmds, "image.apex/lib64/libmylib_rust.so")
	ensureNotContains(t, copyCmds, "image.apex/lib64/libmylib_rust.shared_from_rust.so")
	ensureNotContains(t, copyCmds, "image.apex/lib64/libfoo.shared_from_rust.so")

	// Ensure that we are using non-stub variants of mylib2 and libfoo.shared_from_rust (because
	// of the platform_apis: true)
	mylibLdFlags := ctx.ModuleForTests(t, "mylib", "android_arm64_armv8-a_shared_apex10000_p").Rule("ld").Args["libFlags"]
	ensureNotContains(t, mylibLdFlags, "mylib2/android_arm64_armv8-a_shared_current/mylib2.so")
	ensureContains(t, mylibLdFlags, "mylib2/android_arm64_armv8-a_shared/mylib2.so")
	ensureNotContains(t, mylibLdFlags, "libmylib2_rust/android_arm64_armv8-a_shared_current/unstripped/libmylib2_rust.so")
	ensureContains(t, mylibLdFlags, "libmylib2_rust/android_arm64_armv8-a_shared/unstripped/libmylib2_rust.so")
	rustDeps := ctx.ModuleForTests(t, "foo.rust", "android_arm64_armv8-a_apex10000_p").Rule("rustc").Args["linkFlags"]
	ensureNotContains(t, rustDeps, "libfoo.shared_from_rust/android_arm64_armv8-a_shared_current/libfoo.shared_from_rust.so")
	ensureContains(t, rustDeps, "libfoo.shared_from_rust/android_arm64_armv8-a_shared/libfoo.shared_from_rust.so")
	ensureNotContains(t, rustDeps, "libmylib_rust.shared_from_rust/android_arm64_armv8-a_shared_current/unstripped/libmylib_rust.shared_from_rust.so")
	ensureContains(t, rustDeps, "libmylib_rust.shared_from_rust/android_arm64_armv8-a_shared/unstripped/libmylib_rust.shared_from_rust.so")
}

func TestApexWithStubsWithMinSdkVersion(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: [
				"mylib",
				"mylib3",
				"libmylib3_rust",
			],
			min_sdk_version: "29",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: [
				"mylib2",
				"mylib3#impl",
				"libmylib2_rust",
				"libmylib3_rust#impl",
			],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
			min_sdk_version: "28",
		}

		cc_library {
			name: "mylib2",
			srcs: ["mylib.cpp"],
			cflags: ["-include mylib.h"],
			system_shared_libs: [],
			stl: "none",
			stubs: {
				symbol_file: "mylib2.map.txt",
				versions: ["28", "29", "30", "current"],
			},
			min_sdk_version: "28",
		}

		rust_ffi {
			name: "libmylib2_rust",
			crate_name: "mylib2_rust",
			srcs: ["mylib.rs"],
			stubs: {
				symbol_file: "mylib2.map.txt",
				versions: ["28", "29", "30", "current"],
			},
			min_sdk_version: "28",
		}

		cc_library {
			name: "mylib3",
			srcs: ["mylib.cpp"],
			shared_libs: ["mylib4"],
			system_shared_libs: [],
			stl: "none",
			stubs: {
				symbol_file: "mylib3.map.txt",
				versions: ["28", "29", "30", "current"],
			},
			apex_available: [ "myapex" ],
			min_sdk_version: "28",
		}

		rust_ffi {
			name: "libmylib3_rust",
			crate_name: "mylib3_rust",
			srcs: ["mylib.rs"],
			shared_libs: ["libmylib4.from_rust"],
			stubs: {
				symbol_file: "mylib3.map.txt",
				versions: ["28", "29", "30", "current"],
			},
			apex_available: [ "myapex" ],
			min_sdk_version: "28",
		}

		cc_library {
			name: "mylib4",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
			min_sdk_version: "28",
		}

		rust_ffi {
			name: "libmylib4.from_rust",
			crate_name: "mylib4",
			srcs: ["mylib.rs"],
			apex_available: [ "myapex" ],
			min_sdk_version: "28",
		}
	`)

	apexRule := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that direct non-stubs dep is always included
	ensureContains(t, copyCmds, "image.apex/lib64/mylib.so")
	ensureContains(t, copyCmds, "image.apex/lib64/mylib3.so")
	ensureContains(t, copyCmds, "image.apex/lib64/libmylib3_rust.so")

	// Ensure that indirect stubs dep is not included
	ensureNotContains(t, copyCmds, "image.apex/lib64/mylib2.so")
	ensureNotContains(t, copyCmds, "image.apex/lib64/libmylib2_rust.so")

	// Ensure that direct stubs dep is included
	ensureContains(t, copyCmds, "image.apex/lib64/mylib3.so")

	mylibLdFlags := ctx.ModuleForTests(t, "mylib", "android_arm64_armv8-a_shared_apex29").Rule("ld").Args["libFlags"]

	// Ensure that mylib is linking with the latest version of stub for mylib2
	ensureContains(t, mylibLdFlags, "mylib2/android_arm64_armv8-a_shared_current/mylib2.so")
	ensureContains(t, mylibLdFlags, "libmylib2_rust/android_arm64_armv8-a_shared_current/unstripped/libmylib2_rust.so")
	// ... and not linking to the non-stub (impl) variant of mylib2
	ensureNotContains(t, mylibLdFlags, "mylib2/android_arm64_armv8-a_shared/mylib2.so")
	ensureNotContains(t, mylibLdFlags, "libmylib2_rust/android_arm64_armv8-a_shared/unstripped/libmylib2_rust.so")

	// Ensure that mylib is linking with the non-stub (impl) of mylib3 (because the dependency is added with mylib3#impl)
	ensureContains(t, mylibLdFlags, "mylib3/android_arm64_armv8-a_shared_apex29/mylib3.so")
	ensureContains(t, mylibLdFlags, "libmylib3_rust/android_arm64_armv8-a_shared_apex29/unstripped/libmylib3_rust.so")
	// .. and not linking to the stubs variant of mylib3
	ensureNotContains(t, mylibLdFlags, "mylib3/android_arm64_armv8-a_shared_29/mylib3.so")
	ensureNotContains(t, mylibLdFlags, "libmylib3_rust/android_arm64_armv8-a_shared_29/unstripped/libmylib3_rust.so")

	// Ensure that stubs libs are built without -include flags
	mylib2Cflags := ctx.ModuleForTests(t, "mylib2", "android_arm64_armv8-a_shared_29").Rule("cc").Args["cFlags"]
	ensureNotContains(t, mylib2Cflags, "-include ")

	// Ensure that genstub is invoked with --systemapi
	ensureContains(t, ctx.ModuleForTests(t, "mylib2", "android_arm64_armv8-a_shared_29").Rule("genStubSrc").Args["flags"], "--systemapi")
	ensureContains(t, ctx.ModuleForTests(t, "libmylib2_rust", "android_arm64_armv8-a_shared_29").Rule("cc.genStubSrc").Args["flags"], "--systemapi")

	ensureExactContents(t, ctx, "myapex", "android_common_myapex", []string{
		"lib64/mylib.so",
		"lib64/mylib3.so",
		"lib64/libmylib3_rust.so",
		"lib64/mylib4.so",
		"lib64/libmylib4.from_rust.so",

		"lib64/libstd.dylib.so", // by the implicit dependency from foo.rust
	})
}

func TestApex_PlatformUsesLatestStubFromApex(t *testing.T) {
	t.Parallel()
	//   myapex (Z)
	//      mylib -----------------.
	//                             |
	//   otherapex (29)            |
	//      libstub's versions: 29 Z current
	//                                  |
	//   <platform>                     |
	//      libplatform ----------------'
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			min_sdk_version: "Z", // non-final
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: [
				"libstub",
				"libstub_rust",
			],
			apex_available: ["myapex"],
			min_sdk_version: "Z",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		apex {
			name: "otherapex",
			key: "myapex.key",
			native_shared_libs: [
				"libstub",
				"libstub_rust",
			],
			min_sdk_version: "29",
		}

		cc_library {
			name: "libstub",
			srcs: ["mylib.cpp"],
			stubs: {
				versions: ["29", "Z", "current"],
			},
			apex_available: ["otherapex"],
			min_sdk_version: "29",
		}

		rust_ffi {
			name: "libstub_rust",
			crate_name: "stub_rust",
			srcs: ["mylib.rs"],
			stubs: {
				versions: ["29", "Z", "current"],
			},
			apex_available: ["otherapex"],
			min_sdk_version: "29",
		}

		// platform module depending on libstub from otherapex should use the latest stub("current")
		cc_library {
			name: "libplatform",
			srcs: ["mylib.cpp"],
			shared_libs: [
				"libstub",
				"libstub_rust",
			],
		}
	`,
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.Platform_sdk_codename = proptools.StringPtr("Z")
			variables.Platform_sdk_final = proptools.BoolPtr(false)
			variables.Platform_version_active_codenames = []string{"Z"}
		}),
	)

	// Ensure that mylib from myapex is built against the latest stub (current)
	mylibCflags := ctx.ModuleForTests(t, "mylib", "android_arm64_armv8-a_static_apex10000").Rule("cc").Args["cFlags"]
	ensureContains(t, mylibCflags, "-D__LIBSTUB_API__=10000 ")
	// rust stubs do not emit -D__LIBFOO_API__ flags as this is deprecated behavior for cc stubs

	mylibLdflags := ctx.ModuleForTests(t, "mylib", "android_arm64_armv8-a_shared_apex10000").Rule("ld").Args["libFlags"]
	ensureContains(t, mylibLdflags, "libstub/android_arm64_armv8-a_shared_current/libstub.so ")
	ensureContains(t, mylibLdflags, "libstub_rust/android_arm64_armv8-a_shared_current/unstripped/libstub_rust.so ")

	// Ensure that libplatform is built against latest stub ("current") of mylib3 from the apex
	libplatformCflags := ctx.ModuleForTests(t, "libplatform", "android_arm64_armv8-a_static").Rule("cc").Args["cFlags"]
	ensureContains(t, libplatformCflags, "-D__LIBSTUB_API__=10000 ") // "current" maps to 10000
	// rust stubs do not emit -D__LIBFOO_API__ flags as this is deprecated behavior for cc stubs

	libplatformLdflags := ctx.ModuleForTests(t, "libplatform", "android_arm64_armv8-a_shared").Rule("ld").Args["libFlags"]
	ensureContains(t, libplatformLdflags, "libstub/android_arm64_armv8-a_shared_current/libstub.so ")
	ensureContains(t, libplatformLdflags, "libstub_rust/android_arm64_armv8-a_shared_current/unstripped/libstub_rust.so ")
}

func TestApexWithExplicitStubsDependency(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex2",
			key: "myapex2.key",
			native_shared_libs: ["mylib"],
			updatable: false,
		}

		apex_key {
			name: "myapex2.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: [
				"libfoo#10",
				"libfoo_rust#10"
			],
			static_libs: ["libbaz"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex2" ],
		}

		cc_library {
			name: "libfoo",
			srcs: ["mylib.cpp"],
			shared_libs: ["libbar"],
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["10", "20", "30"],
			},
		}

		rust_ffi {
			name: "libfoo_rust",
			crate_name: "foo_rust",
			srcs: ["mylib.cpp"],
			shared_libs: ["libbar.from_rust"],
			stubs: {
				versions: ["10", "20", "30"],
			},
		}

		cc_library {
			name: "libbar",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
		}

		cc_library {
			name: "libbar.from_rust",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
		}

		cc_library_static {
			name: "libbaz",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex2" ],
		}

	`)

	apexRule := ctx.ModuleForTests(t, "myapex2", "android_common_myapex2").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that direct non-stubs dep is always included
	ensureContains(t, copyCmds, "image.apex/lib64/mylib.so")

	// Ensure that indirect stubs dep is not included
	ensureNotContains(t, copyCmds, "image.apex/lib64/libfoo.so")
	ensureNotContains(t, copyCmds, "image.apex/lib64/libfoo_rust.so")

	// Ensure that dependency of stubs is not included
	ensureNotContains(t, copyCmds, "image.apex/lib64/libbar.so")
	ensureNotContains(t, copyCmds, "image.apex/lib64/libbar.from_rust.so")

	mylibLdFlags := ctx.ModuleForTests(t, "mylib", "android_arm64_armv8-a_shared_apex10000").Rule("ld").Args["libFlags"]

	// Ensure that mylib is linking with version 10 of libfoo
	ensureContains(t, mylibLdFlags, "libfoo/android_arm64_armv8-a_shared_10/libfoo.so")
	ensureContains(t, mylibLdFlags, "libfoo_rust/android_arm64_armv8-a_shared_10/unstripped/libfoo_rust.so")
	// ... and not linking to the non-stub (impl) variant of libfoo
	ensureNotContains(t, mylibLdFlags, "libfoo/android_arm64_armv8-a_shared/libfoo.so")
	ensureNotContains(t, mylibLdFlags, "libfoo_rust/android_arm64_armv8-a_shared/unstripped/libfoo_rust.so")

	libFooStubsLdFlags := ctx.ModuleForTests(t, "libfoo", "android_arm64_armv8-a_shared_10").Rule("ld").Args["libFlags"]
	libFooRustStubsLdFlags := ctx.ModuleForTests(t, "libfoo_rust", "android_arm64_armv8-a_shared_10").Rule("ld").Args["libFlags"]

	// Ensure that libfoo stubs is not linking to libbar (since it is a stubs)
	ensureNotContains(t, libFooStubsLdFlags, "libbar.so")
	ensureNotContains(t, libFooRustStubsLdFlags, "libbar.from_rust.so")

	fullDepsInfo := strings.Split(android.ContentFromFileRuleForTests(t, ctx,
		ctx.ModuleForTests(t, "myapex2", "android_common_myapex2").Output("depsinfo/fulllist.txt")), "\n")
	ensureListContains(t, fullDepsInfo, "  libfoo(minSdkVersion:(no version)) (external) <- mylib")

	flatDepsInfo := strings.Split(android.ContentFromFileRuleForTests(t, ctx,
		ctx.ModuleForTests(t, "myapex2", "android_common_myapex2").Output("depsinfo/flatlist.txt")), "\n")
	ensureListContains(t, flatDepsInfo, "libfoo(minSdkVersion:(no version)) (external)")
}

func TestApexWithRuntimeLibsDependency(t *testing.T) {
	t.Parallel()
	/*
		myapex
		  |
		  v   (runtime_libs)
		mylib ------+------> libfoo [provides stub]
			    |
			    `------> libbar
	*/
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			static_libs: ["libstatic"],
			shared_libs: ["libshared"],
			runtime_libs: [
				"libfoo",
				"libbar",
				"libfoo_rs",
			],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "libfoo",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["10", "20", "30"],
			},
		}

		rust_ffi {
			name: "libfoo_rs",
			crate_name: "foo_rs",
			srcs: ["mylib.rs"],
			stubs: {
				versions: ["10", "20", "30"],
			},
		}

		cc_library {
			name: "libbar",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "libstatic",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
			runtime_libs: ["libstatic_to_runtime"],
		}

		cc_library {
			name: "libshared",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
			runtime_libs: ["libshared_to_runtime"],
		}

		cc_library {
			name: "libstatic_to_runtime",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "libshared_to_runtime",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}
	`)

	apexRule := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that direct non-stubs dep is always included
	ensureContains(t, copyCmds, "image.apex/lib64/mylib.so")

	// Ensure that indirect stubs dep is not included
	ensureNotContains(t, copyCmds, "image.apex/lib64/libfoo.so")
	ensureNotContains(t, copyCmds, "image.apex/lib64/libfoo_rs.so")

	// Ensure that runtime_libs dep in included
	ensureContains(t, copyCmds, "image.apex/lib64/libbar.so")
	ensureContains(t, copyCmds, "image.apex/lib64/libshared.so")
	ensureContains(t, copyCmds, "image.apex/lib64/libshared_to_runtime.so")

	ensureNotContains(t, copyCmds, "image.apex/lib64/libstatic_to_runtime.so")

	apexManifestRule := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Rule("apexManifestRule")
	ensureListEmpty(t, names(apexManifestRule.Args["provideNativeLibs"]))
	ensureListContains(t, names(apexManifestRule.Args["requireNativeLibs"]), "libfoo.so")
	ensureListContains(t, names(apexManifestRule.Args["requireNativeLibs"]), "libfoo_rs.so")
}

var prepareForTestOfRuntimeApexWithHwasan = android.GroupFixturePreparers(
	cc.PrepareForTestWithCcBuildComponents,
	PrepareForTestWithApexBuildComponents,
	android.FixtureAddTextFile("bionic/apex/Android.bp", `
		apex {
			name: "com.android.runtime",
			key: "com.android.runtime.key",
			native_shared_libs: ["libc"],
			updatable: false,
		}

		apex_key {
			name: "com.android.runtime.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`),
	android.FixtureAddFile("system/sepolicy/apex/com.android.runtime-file_contexts", nil),
)

func TestRuntimeApexShouldInstallHwasanIfLibcDependsOnIt(t *testing.T) {
	t.Parallel()
	result := android.GroupFixturePreparers(prepareForTestOfRuntimeApexWithHwasan).RunTestWithBp(t, `
		cc_library {
			name: "libc",
			no_libcrt: true,
			nocrt: true,
			no_crt_pad_segment: true,
			stl: "none",
			system_shared_libs: [],
			stubs: { versions: ["1"] },
			apex_available: ["com.android.runtime"],

			sanitize: {
				hwaddress: true,
			}
		}

		cc_prebuilt_library_shared {
			name: "libclang_rt.hwasan",
			no_libcrt: true,
			nocrt: true,
			no_crt_pad_segment: true,
			stl: "none",
			system_shared_libs: [],
			srcs: [""],
			stubs: { versions: ["1"] },
			stem: "libclang_rt.hwasan-aarch64-android",

			sanitize: {
				never: true,
			},
			apex_available: [
				"//apex_available:anyapex",
				"//apex_available:platform",
			],
		}	`)
	ctx := result.TestContext

	ensureExactContents(t, ctx, "com.android.runtime", "android_common_hwasan_com.android.runtime", []string{
		"lib64/bionic/libc.so",
		"lib64/bionic/libclang_rt.hwasan-aarch64-android.so",
	})

	hwasan := ctx.ModuleForTests(t, "libclang_rt.hwasan", "android_arm64_armv8-a_shared")

	installed := hwasan.Description("install libclang_rt.hwasan")
	ensureContains(t, installed.Output.String(), "/system/lib64/bootstrap/libclang_rt.hwasan-aarch64-android.so")

	symlink := hwasan.Description("install symlink libclang_rt.hwasan")
	ensureEquals(t, symlink.Args["fromPath"], "/apex/com.android.runtime/lib64/bionic/libclang_rt.hwasan-aarch64-android.so")
	ensureContains(t, symlink.Output.String(), "/system/lib64/libclang_rt.hwasan-aarch64-android.so")
}

func TestRuntimeApexShouldInstallHwasanIfHwaddressSanitized(t *testing.T) {
	t.Parallel()
	result := android.GroupFixturePreparers(
		prepareForTestOfRuntimeApexWithHwasan,
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.SanitizeDevice = []string{"hwaddress"}
		}),
	).RunTestWithBp(t, `
		cc_library {
			name: "libc",
			no_libcrt: true,
			nocrt: true,
			no_crt_pad_segment: true,
			stl: "none",
			system_shared_libs: [],
			stubs: { versions: ["1"] },
			apex_available: ["com.android.runtime"],
		}

		cc_prebuilt_library_shared {
			name: "libclang_rt.hwasan",
			no_libcrt: true,
			nocrt: true,
			no_crt_pad_segment: true,
			stl: "none",
			system_shared_libs: [],
			srcs: [""],
			stubs: { versions: ["1"] },
			stem: "libclang_rt.hwasan-aarch64-android",

			sanitize: {
				never: true,
			},
			apex_available: [
				"//apex_available:anyapex",
				"//apex_available:platform",
			],
		}
		`)
	ctx := result.TestContext

	ensureExactContents(t, ctx, "com.android.runtime", "android_common_hwasan_com.android.runtime", []string{
		"lib64/bionic/libc.so",
		"lib64/bionic/libclang_rt.hwasan-aarch64-android.so",
	})

	hwasan := ctx.ModuleForTests(t, "libclang_rt.hwasan", "android_arm64_armv8-a_shared")

	installed := hwasan.Description("install libclang_rt.hwasan")
	ensureContains(t, installed.Output.String(), "/system/lib64/bootstrap/libclang_rt.hwasan-aarch64-android.so")

	symlink := hwasan.Description("install symlink libclang_rt.hwasan")
	ensureEquals(t, symlink.Args["fromPath"], "/apex/com.android.runtime/lib64/bionic/libclang_rt.hwasan-aarch64-android.so")
	ensureContains(t, symlink.Output.String(), "/system/lib64/libclang_rt.hwasan-aarch64-android.so")
}

func TestApexDependsOnLLNDKTransitively(t *testing.T) {
	t.Parallel()
	testcases := []struct {
		name          string
		minSdkVersion string
		apexVariant   string
		shouldLink    string
		shouldNotLink []string
	}{
		{
			name:          "unspecified version links to the latest",
			minSdkVersion: "",
			apexVariant:   "apex10000",
			shouldLink:    "current",
			shouldNotLink: []string{"29", "30"},
		},
		{
			name:          "always use the latest",
			minSdkVersion: "min_sdk_version: \"29\",",
			apexVariant:   "apex29",
			shouldLink:    "current",
			shouldNotLink: []string{"29", "30"},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testApex(t, `
			apex {
				name: "myapex",
				key: "myapex.key",
				native_shared_libs: ["mylib"],
				updatable: false,
				`+tc.minSdkVersion+`
			}

			apex_key {
				name: "myapex.key",
				public_key: "testkey.avbpubkey",
				private_key: "testkey.pem",
			}

			cc_library {
				name: "mylib",
				srcs: ["mylib.cpp"],
				vendor_available: true,
				shared_libs: ["libbar"],
				system_shared_libs: [],
				stl: "none",
				apex_available: [ "myapex" ],
				min_sdk_version: "29",
			}

			cc_library {
				name: "libbar",
				srcs: ["mylib.cpp"],
				system_shared_libs: [],
				stl: "none",
				stubs: { versions: ["29","30"] },
				llndk: {
					symbol_file: "libbar.map.txt",
				}
			}
			`,
				withUnbundledBuild,
			)

			// Ensure that LLNDK dep is not included
			ensureExactContents(t, ctx, "myapex", "android_common_myapex", []string{
				"lib64/mylib.so",
			})

			// Ensure that LLNDK dep is required
			apexManifestRule := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Rule("apexManifestRule")
			ensureListEmpty(t, names(apexManifestRule.Args["provideNativeLibs"]))
			ensureListContains(t, names(apexManifestRule.Args["requireNativeLibs"]), "libbar.so")

			mylibLdFlags := ctx.ModuleForTests(t, "mylib", "android_arm64_armv8-a_shared_"+tc.apexVariant).Rule("ld").Args["libFlags"]
			ensureContains(t, mylibLdFlags, "libbar/android_arm64_armv8-a_shared_"+tc.shouldLink+"/libbar.so")
			for _, ver := range tc.shouldNotLink {
				ensureNotContains(t, mylibLdFlags, "libbar/android_arm64_armv8-a_shared_"+ver+"/libbar.so")
			}

			mylibCFlags := ctx.ModuleForTests(t, "mylib", "android_arm64_armv8-a_static_"+tc.apexVariant).Rule("cc").Args["cFlags"]
			ver := tc.shouldLink
			if tc.shouldLink == "current" {
				ver = strconv.Itoa(android.FutureApiLevelInt)
			}
			ensureContains(t, mylibCFlags, "__LIBBAR_API__="+ver)
		})
	}
}

func TestApexWithSystemLibsStubs(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib", "mylib_shared", "libdl", "libm", "libmylib_rs"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			system_shared_libs: ["libc"],
			shared_libs: ["libdl#27", "libm#impl"],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		rust_ffi {
			name: "libmylib_rs",
			crate_name: "mylib_rs",
			shared_libs: ["libvers#27", "libm#impl"],
			srcs: ["mylib.rs"],
			apex_available: [ "myapex" ],
		}

		cc_library_shared {
			name: "mylib_shared",
			srcs: ["mylib.cpp"],
			shared_libs: ["libdl#27"],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "libBootstrap",
			srcs: ["mylib.cpp"],
			stl: "none",
			bootstrap: true,
		}

		rust_ffi {
			name: "libbootstrap_rs",
			srcs: ["mylib.cpp"],
			crate_name: "bootstrap_rs",
			bootstrap: true,
		}

		cc_library {
			name: "libvers",
			srcs: ["mylib.cpp"],
			stl: "none",
			stubs: { versions: ["27","30"] },
		}
	`)

	apexRule := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that mylib, libmylib_rs, libm, libdl, libstd.dylib.so (from Rust) are included.
	ensureContains(t, copyCmds, "image.apex/lib64/mylib.so")
	ensureContains(t, copyCmds, "image.apex/lib64/libmylib_rs.so")
	ensureContains(t, copyCmds, "image.apex/lib64/bionic/libm.so")
	ensureContains(t, copyCmds, "image.apex/lib64/bionic/libdl.so")
	ensureContains(t, copyCmds, "image.apex/lib64/libstd.dylib.so")

	// Ensure that libc and liblog (from Rust) is not included (since it has stubs and not listed in native_shared_libs)
	ensureNotContains(t, copyCmds, "image.apex/lib64/bionic/libc.so")
	ensureNotContains(t, copyCmds, "image.apex/lib64/liblog.so")

	mylibLdFlags := ctx.ModuleForTests(t, "mylib", "android_arm64_armv8-a_shared_apex10000").Rule("ld").Args["libFlags"]
	mylibRsFlags := ctx.ModuleForTests(t, "libmylib_rs", "android_arm64_armv8-a_shared_apex10000").Rule("rustc").Args["linkFlags"]
	mylibCFlags := ctx.ModuleForTests(t, "mylib", "android_arm64_armv8-a_static_apex10000").Rule("cc").Args["cFlags"]
	mylibSharedCFlags := ctx.ModuleForTests(t, "mylib_shared", "android_arm64_armv8-a_shared_apex10000").Rule("cc").Args["cFlags"]

	// For dependency to libc
	// Ensure that mylib is linking with the latest version of stubs
	ensureContains(t, mylibLdFlags, "libc/android_arm64_armv8-a_shared_current/libc.so")
	// ... and not linking to the non-stub (impl) variant
	ensureNotContains(t, mylibLdFlags, "libc/android_arm64_armv8-a_shared/libc.so")
	// ... Cflags from stub is correctly exported to mylib
	ensureContains(t, mylibCFlags, "__LIBC_API__=10000")
	ensureContains(t, mylibSharedCFlags, "__LIBC_API__=10000")

	// For dependency to libm
	// Ensure that mylib is linking with the non-stub (impl) variant
	ensureContains(t, mylibLdFlags, "libm/android_arm64_armv8-a_shared_apex10000/libm.so")
	// ... and not linking to the stub variant
	ensureNotContains(t, mylibLdFlags, "libm/android_arm64_armv8-a_shared_29/libm.so")
	// ... and is not compiling with the stub
	ensureNotContains(t, mylibCFlags, "__LIBM_API__=29")
	ensureNotContains(t, mylibSharedCFlags, "__LIBM_API__=29")

	// For dependency to libdl
	// Ensure that mylib is linking with the specified version of stubs
	ensureContains(t, mylibLdFlags, "libdl/android_arm64_armv8-a_shared_27/libdl.so")
	// ... and not linking to the other versions of stubs
	ensureNotContains(t, mylibLdFlags, "libdl/android_arm64_armv8-a_shared_28/libdl.so")
	ensureNotContains(t, mylibLdFlags, "libdl/android_arm64_armv8-a_shared_29/libdl.so")
	// ... and not linking to the non-stub (impl) variant
	ensureNotContains(t, mylibLdFlags, "libdl/android_arm64_armv8-a_shared_apex10000/libdl.so")
	// ... Cflags from stub is correctly exported to mylib
	ensureContains(t, mylibCFlags, "__LIBDL_API__=27")
	ensureContains(t, mylibSharedCFlags, "__LIBDL_API__=27")

	// Rust checks
	// For dependency to libc, liblog
	// Ensure that libmylib_rs is linking with the latest versions of stubs
	ensureContains(t, mylibRsFlags, "libc/android_arm64_armv8-a_shared_current/libc.so")
	ensureContains(t, mylibRsFlags, "liblog/android_arm64_armv8-a_shared_current/liblog.so")
	// ... and not linking to the non-stub (impl) variants
	ensureNotContains(t, mylibRsFlags, "libc/android_arm64_armv8-a_shared/libc.so")
	ensureNotContains(t, mylibRsFlags, "liblog/android_arm64_armv8-a_shared/liblog.so")

	// For libm dependency (explicit)
	// Ensure that mylib is linking with the non-stub (impl) variant
	ensureContains(t, mylibRsFlags, "libm/android_arm64_armv8-a_shared_apex10000/libm.so")
	// ... and not linking to the stub variant
	ensureNotContains(t, mylibRsFlags, "libm/android_arm64_armv8-a_shared_29/libm.so")

	// For dependency to libvers
	// (We do not use libdl#27 as Rust links the system libs implicitly and does
	// not currently have a system_shared_libs equivalent to prevent this)
	// Ensure that mylib is linking with the specified version of stubs
	ensureContains(t, mylibRsFlags, "libvers/android_arm64_armv8-a_shared_27/libvers.so")
	// ... and not linking to the other versions of stubs
	ensureNotContains(t, mylibRsFlags, "libvers/android_arm64_armv8-a_shared_30/libvers.so")
	// ... and not linking to the non-stub (impl) variant
	ensureNotContains(t, mylibRsFlags, "libvers/android_arm64_armv8-a_shared_apex10000/libvers.so")

	// Ensure that libBootstrap is depending on the platform variant of bionic libs
	libFlags := ctx.ModuleForTests(t, "libBootstrap", "android_arm64_armv8-a_shared").Rule("ld").Args["libFlags"]
	ensureContains(t, libFlags, "libc/android_arm64_armv8-a_shared/libc.so")
	ensureContains(t, libFlags, "libm/android_arm64_armv8-a_shared/libm.so")
	ensureContains(t, libFlags, "libdl/android_arm64_armv8-a_shared/libdl.so")

	// Ensure that libbootstrap_rs is depending on the platform variant of bionic libs
	libRsFlags := ctx.ModuleForTests(t, "libbootstrap_rs", "android_arm64_armv8-a_shared").Rule("rustc").Args["linkFlags"]
	ensureContains(t, libRsFlags, "libc/android_arm64_armv8-a_shared/libc.so")
	ensureContains(t, libRsFlags, "libm/android_arm64_armv8-a_shared/libm.so")
	ensureContains(t, libRsFlags, "libdl/android_arm64_armv8-a_shared/libdl.so")
}

func TestApexMinSdkVersion_NativeModulesShouldBeBuiltAgainstStubs(t *testing.T) {
	t.Parallel()
	// there are three links between liba --> libz.
	// 1) myapex -> libx -> liba -> libz    : this should be #30 link
	// 2) otherapex -> liby -> liba -> libz : this should be #30 link
	// 3) (platform) -> liba -> libz        : this should be non-stub link
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["libx"],
			min_sdk_version: "29",
		}

		apex {
			name: "otherapex",
			key: "myapex.key",
			native_shared_libs: ["liby"],
			min_sdk_version: "30",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "libx",
			shared_libs: ["liba"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
			min_sdk_version: "29",
		}

		cc_library {
			name: "liby",
			shared_libs: ["liba"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "otherapex" ],
			min_sdk_version: "29",
		}

		cc_library {
			name: "liba",
			shared_libs: ["libz", "libz_rs"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [
				"//apex_available:anyapex",
				"//apex_available:platform",
			],
			min_sdk_version: "29",
		}

		cc_library {
			name: "libz",
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["28", "30"],
			},
		}

		rust_ffi {
			name: "libz_rs",
			crate_name: "z_rs",
			srcs: ["foo.rs"],
			stubs: {
				versions: ["28", "30"],
			},
		}
	`)

	expectLink := func(from, from_variant, to, to_variant string) {
		ldArgs := ctx.ModuleForTests(t, from, "android_arm64_armv8-a_"+from_variant).Rule("ld").Args["libFlags"]
		ensureContains(t, ldArgs, "android_arm64_armv8-a_"+to_variant+"/"+to+".so")
	}
	expectNoLink := func(from, from_variant, to, to_variant string) {
		ldArgs := ctx.ModuleForTests(t, from, "android_arm64_armv8-a_"+from_variant).Rule("ld").Args["libFlags"]
		ensureNotContains(t, ldArgs, "android_arm64_armv8-a_"+to_variant+"/"+to+".so")
	}
	// platform liba is linked to non-stub version
	expectLink("liba", "shared", "libz", "shared")
	expectLink("liba", "shared", "unstripped/libz_rs", "shared")
	// liba in myapex is linked to current
	expectLink("liba", "shared_apex29", "libz", "shared_current")
	expectNoLink("liba", "shared_apex29", "libz", "shared_30")
	expectNoLink("liba", "shared_apex29", "libz", "shared_28")
	expectNoLink("liba", "shared_apex29", "libz", "shared")
	expectLink("liba", "shared_apex29", "unstripped/libz_rs", "shared_current")
	expectNoLink("liba", "shared_apex29", "unstripped/libz_rs", "shared_30")
	expectNoLink("liba", "shared_apex29", "unstripped/libz_rs", "shared_28")
	expectNoLink("liba", "shared_apex29", "unstripped/libz_rs", "shared")
	// liba in otherapex is linked to current
	expectLink("liba", "shared_apex30", "libz", "shared_current")
	expectNoLink("liba", "shared_apex30", "libz", "shared_30")
	expectNoLink("liba", "shared_apex30", "libz", "shared_28")
	expectNoLink("liba", "shared_apex30", "libz", "shared")
	expectLink("liba", "shared_apex30", "unstripped/libz_rs", "shared_current")
	expectNoLink("liba", "shared_apex30", "unstripped/libz_rs", "shared_30")
	expectNoLink("liba", "shared_apex30", "unstripped/libz_rs", "shared_28")
	expectNoLink("liba", "shared_apex30", "unstripped/libz_rs", "shared")
}

func TestApexMinSdkVersion_SupportsCodeNames(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["libx"],
			min_sdk_version: "R",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "libx",
			shared_libs: ["libz"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
			min_sdk_version: "R",
		}

		cc_library {
			name: "libz",
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["29", "R"],
			},
		}
	`,
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.Platform_version_active_codenames = []string{"R"}
		}),
	)

	expectLink := func(from, from_variant, to, to_variant string) {
		ldArgs := ctx.ModuleForTests(t, from, "android_arm64_armv8-a_"+from_variant).Rule("ld").Args["libFlags"]
		ensureContains(t, ldArgs, "android_arm64_armv8-a_"+to_variant+"/"+to+".so")
	}
	expectNoLink := func(from, from_variant, to, to_variant string) {
		ldArgs := ctx.ModuleForTests(t, from, "android_arm64_armv8-a_"+from_variant).Rule("ld").Args["libFlags"]
		ensureNotContains(t, ldArgs, "android_arm64_armv8-a_"+to_variant+"/"+to+".so")
	}
	expectLink("libx", "shared_apex10000", "libz", "shared_current")
	expectNoLink("libx", "shared_apex10000", "libz", "shared_R")
	expectNoLink("libx", "shared_apex10000", "libz", "shared_29")
	expectNoLink("libx", "shared_apex10000", "libz", "shared")
}

func TestApexMinSdkVersion_SupportsCodeNames_JavaLibs(t *testing.T) {
	t.Parallel()
	testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			java_libs: ["libx"],
			min_sdk_version: "S",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_library {
			name: "libx",
			srcs: ["a.java"],
			apex_available: [ "myapex" ],
			sdk_version: "current",
			min_sdk_version: "S", // should be okay
			compile_dex: true,
		}
	`,
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.Platform_version_active_codenames = []string{"S"}
			variables.Platform_sdk_codename = proptools.StringPtr("S")
		}),
	)
}

func TestApexMinSdkVersion_DefaultsToLatest(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["libx"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "libx",
			shared_libs: ["libz"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "libz",
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["1", "2"],
			},
		}
	`)

	expectLink := func(from, from_variant, to, to_variant string) {
		ldArgs := ctx.ModuleForTests(t, from, "android_arm64_armv8-a_"+from_variant).Rule("ld").Args["libFlags"]
		ensureContains(t, ldArgs, "android_arm64_armv8-a_"+to_variant+"/"+to+".so")
	}
	expectNoLink := func(from, from_variant, to, to_variant string) {
		ldArgs := ctx.ModuleForTests(t, from, "android_arm64_armv8-a_"+from_variant).Rule("ld").Args["libFlags"]
		ensureNotContains(t, ldArgs, "android_arm64_armv8-a_"+to_variant+"/"+to+".so")
	}
	expectLink("libx", "shared_apex10000", "libz", "shared_current")
	expectNoLink("libx", "shared_apex10000", "libz", "shared_1")
	expectNoLink("libx", "shared_apex10000", "libz", "shared_2")
	expectNoLink("libx", "shared_apex10000", "libz", "shared")
}

func TestApexMinSdkVersion_InVendorApex(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			updatable: true,
			vendor: true,
			min_sdk_version: "29",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			vendor_available: true,
			min_sdk_version: "29",
			shared_libs: ["libbar"],
		}

		cc_library {
			name: "libbar",
			stubs: { versions: ["29", "30"] },
			llndk: { symbol_file: "libbar.map.txt" },
		}
	`)

	vendorVariant := "android_vendor_arm64_armv8-a"

	mylib := ctx.ModuleForTests(t, "mylib", vendorVariant+"_shared_apex29")

	// Ensure that mylib links with "current" LLNDK
	libFlags := names(mylib.Rule("ld").Args["libFlags"])
	ensureListContains(t, libFlags, "out/soong/.intermediates/libbar/"+vendorVariant+"_shared/libbar.so")

	// Ensure that mylib is targeting 29
	ccRule := ctx.ModuleForTests(t, "mylib", vendorVariant+"_static_apex29").Output("obj/mylib.o")
	ensureContains(t, ccRule.Args["cFlags"], "-target aarch64-linux-android29")

	// Ensure that the correct variant of crtbegin_so is used.
	crtBegin := mylib.Rule("ld").Args["crtBegin"]
	ensureContains(t, crtBegin, "out/soong/.intermediates/"+cc.DefaultCcCommonTestModulesDir+"crtbegin_so/"+vendorVariant+"_apex29/crtbegin_so.o")

	// Ensure that the crtbegin_so used by the APEX is targeting 29
	cflags := ctx.ModuleForTests(t, "crtbegin_so", vendorVariant+"_apex29").Rule("cc").Args["cFlags"]
	android.AssertStringDoesContain(t, "cflags", cflags, "-target aarch64-linux-android29")
}

func TestTrackAllowedDepsForAndroidApex(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "com.android.myapex",
			key: "myapex.key",
			updatable: true,
			native_shared_libs: [
				"mylib",
				"yourlib",
			],
			min_sdk_version: "29",
		}

		apex {
			name: "myapex2",
			key: "myapex.key",
			updatable: false,
			native_shared_libs: ["yourlib"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: ["libbar", "libbar_rs"],
			min_sdk_version: "29",
			apex_available: ["com.android.myapex"],
		}

		cc_library {
			name: "libbar",
			stubs: { versions: ["29", "30"] },
		}

		rust_ffi {
			name: "libbar_rs",
			crate_name: "bar_rs",
			srcs: ["bar.rs"],
			stubs: { versions: ["29", "30"] },
		}

		cc_library {
			name: "yourlib",
			srcs: ["mylib.cpp"],
			min_sdk_version: "29",
			apex_available: ["com.android.myapex", "myapex2", "//apex_available:platform"],
		}
	`, withFiles(android.MockFS{
		"packages/modules/common/build/allowed_deps.txt": nil,
	}),
		android.FixtureMergeMockFs(android.MockFS{
			"system/sepolicy/apex/com.android.myapex-file_contexts": nil,
		}))

	depsinfo := ctx.SingletonForTests(t, "apex_depsinfo_singleton")
	inputs := depsinfo.Rule("generateApexDepsInfoFilesRule").BuildParams.Inputs.Strings()
	android.AssertStringListContains(t, "updatable com.android.myapex should generate depsinfo file", inputs,
		"out/soong/.intermediates/com.android.myapex/android_common_com.android.myapex/depsinfo/flatlist.txt")
	android.AssertStringListDoesNotContain(t, "non-updatable myapex2 should not generate depsinfo file", inputs,
		"out/soong/.intermediates/myapex2/android_common_myapex2/depsinfo/flatlist.txt")

	myapex := ctx.ModuleForTests(t, "com.android.myapex", "android_common_com.android.myapex")
	flatlist := strings.Split(android.ContentFromFileRuleForTests(t, ctx,
		myapex.Output("depsinfo/flatlist.txt")), "\n")
	android.AssertStringListContains(t, "deps with stubs should be tracked in depsinfo as external dep",
		flatlist, "libbar(minSdkVersion:(no version)) (external)")
	android.AssertStringListContains(t, "deps with stubs should be tracked in depsinfo as external dep",
		flatlist, "libbar_rs(minSdkVersion:(no version)) (external)")
	android.AssertStringListDoesNotContain(t, "do not track if not available for platform",
		flatlist, "mylib:(minSdkVersion:29)")
	android.AssertStringListContains(t, "track platform-available lib",
		flatlist, "yourlib(minSdkVersion:29)")
}

func TestNotTrackAllowedDepsForNonAndroidApex(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			updatable: true,
			native_shared_libs: [
				"mylib",
				"yourlib",
			],
			min_sdk_version: "29",
		}

		apex {
			name: "myapex2",
			key: "myapex.key",
			updatable: false,
			native_shared_libs: ["yourlib"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: ["libbar"],
			min_sdk_version: "29",
			apex_available: ["myapex"],
		}

		cc_library {
			name: "libbar",
			stubs: { versions: ["29", "30"] },
		}

		cc_library {
			name: "yourlib",
			srcs: ["mylib.cpp"],
			min_sdk_version: "29",
			apex_available: ["myapex", "myapex2", "//apex_available:platform"],
		}
	`, withFiles(android.MockFS{
		"packages/modules/common/build/allowed_deps.txt": nil,
	}))

	depsinfo := ctx.SingletonForTests(t, "apex_depsinfo_singleton")
	inputs := depsinfo.Rule("generateApexDepsInfoFilesRule").BuildParams.Inputs.Strings()
	android.AssertStringListDoesNotContain(t, "updatable myapex should generate depsinfo file", inputs,
		"out/soong/.intermediates/myapex/android_common_myapex/depsinfo/flatlist.txt")
	android.AssertStringListDoesNotContain(t, "non-updatable myapex2 should not generate depsinfo file", inputs,
		"out/soong/.intermediates/myapex2/android_common_myapex2/depsinfo/flatlist.txt")
}

func TestTrackAllowedDeps_SkipWithoutAllowedDepsTxt(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "com.android.myapex",
			key: "myapex.key",
			updatable: true,
			min_sdk_version: "29",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`,
		android.FixtureMergeMockFs(android.MockFS{
			"system/sepolicy/apex/com.android.myapex-file_contexts": nil,
		}))
	depsinfo := ctx.SingletonForTests(t, "apex_depsinfo_singleton")
	if nil != depsinfo.MaybeRule("generateApexDepsInfoFilesRule").Output {
		t.Error("apex_depsinfo_singleton shouldn't run when allowed_deps.txt doesn't exist")
	}
}

func TestPlatformUsesLatestStubsFromApexes(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["libx", "libx_rs"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "libx",
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
			stubs: {
				versions: ["1", "2"],
			},
		}

		rust_ffi {
			name: "libx_rs",
			crate_name: "x_rs",
			srcs: ["x.rs"],
			apex_available: [ "myapex" ],
			stubs: {
				versions: ["1", "2"],
			},
		}

		cc_library {
			name: "libz",
			shared_libs: ["libx", "libx_rs",],
			system_shared_libs: [],
			stl: "none",
		}
	`)

	expectLink := func(from, from_variant, to, to_variant string) {
		t.Helper()
		ldArgs := ctx.ModuleForTests(t, from, "android_arm64_armv8-a_"+from_variant).Rule("ld").Args["libFlags"]
		ensureContains(t, ldArgs, "android_arm64_armv8-a_"+to_variant+"/"+to+".so")
	}
	expectNoLink := func(from, from_variant, to, to_variant string) {
		t.Helper()
		ldArgs := ctx.ModuleForTests(t, from, "android_arm64_armv8-a_"+from_variant).Rule("ld").Args["libFlags"]
		ensureNotContains(t, ldArgs, "android_arm64_armv8-a_"+to_variant+"/"+to+".so")
	}
	expectLink("libz", "shared", "libx", "shared_current")
	expectNoLink("libz", "shared", "libx", "shared_2")
	expectLink("libz", "shared", "unstripped/libx_rs", "shared_current")
	expectNoLink("libz", "shared", "unstripped/libx_rs", "shared_2")
	expectNoLink("libz", "shared", "libz", "shared_1")
	expectNoLink("libz", "shared", "libz", "shared")
}

var prepareForTestWithSantitizeHwaddress = android.FixtureModifyProductVariables(
	func(variables android.FixtureProductVariables) {
		variables.SanitizeDevice = []string{"hwaddress"}
	},
)

func TestQApexesUseLatestStubsInBundledBuildsAndHWASAN(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["libx"],
			min_sdk_version: "29",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "libx",
			shared_libs: ["libbar", "libbar_rs"],
			apex_available: [ "myapex" ],
			min_sdk_version: "29",
		}

		rust_ffi {
			name: "libbar_rs",
			crate_name: "bar_rs",
			srcs: ["bar.rs"],
			stubs: { versions: ["29", "30"] },
		}

		cc_library {
			name: "libbar",
			stubs: {
				versions: ["29", "30"],
			},
		}
	`,
		prepareForTestWithSantitizeHwaddress,
	)
	expectLink := func(from, from_variant, to, to_variant string) {
		ld := ctx.ModuleForTests(t, from, "android_arm64_armv8-a_"+from_variant).Rule("ld")
		libFlags := ld.Args["libFlags"]
		ensureContains(t, libFlags, "android_arm64_armv8-a_"+to_variant+"/"+to+".so")
	}
	expectLink("libx", "shared_hwasan_apex29", "libbar", "shared_current")
	expectLink("libx", "shared_hwasan_apex29", "unstripped/libbar_rs", "shared_current")
}

func TestQTargetApexUsesStaticUnwinder(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["libx"],
			min_sdk_version: "29",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "libx",
			apex_available: [ "myapex" ],
			min_sdk_version: "29",
		}
	`)

	// ensure apex variant of c++ is linked with static unwinder
	cm := ctx.ModuleForTests(t, "libc++", "android_arm64_armv8-a_shared_apex29").Module().(*cc.Module)
	ensureListContains(t, cm.Properties.AndroidMkStaticLibs, "libunwind")
	// note that platform variant is not.
	cm = ctx.ModuleForTests(t, "libc++", "android_arm64_armv8-a_shared").Module().(*cc.Module)
	ensureListNotContains(t, cm.Properties.AndroidMkStaticLibs, "libunwind")
}

func TestApexMinSdkVersion_ErrorIfIncompatibleVersion(t *testing.T) {
	t.Parallel()
	testApexError(t, `module "mylib".*: should support min_sdk_version\(29\)`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			min_sdk_version: "29",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [
				"myapex",
			],
			min_sdk_version: "30",
		}
	`)

	testApexError(t, `module "libfoo.ffi".*: should support min_sdk_version\(29\)`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["libfoo.ffi"],
			min_sdk_version: "29",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		rust_ffi_shared {
			name: "libfoo.ffi",
			srcs: ["foo.rs"],
			crate_name: "foo",
			apex_available: [
				"myapex",
			],
			min_sdk_version: "30",
		}
	`)

	testApexError(t, `module "libfoo".*: should support min_sdk_version\(29\)`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			java_libs: ["libfoo"],
			min_sdk_version: "29",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_import {
			name: "libfoo",
			jars: ["libfoo.jar"],
			apex_available: [
				"myapex",
			],
			min_sdk_version: "30",
			compile_dex: true,
		}
	`)

	// Skip check for modules compiling against core API surface
	testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			java_libs: ["libfoo"],
			min_sdk_version: "29",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_library {
			name: "libfoo",
			srcs: ["Foo.java"],
			apex_available: [
				"myapex",
			],
			// Compile against core API surface
			sdk_version: "core_current",
			min_sdk_version: "30",
			compile_dex: true,
		}
	`)

}

func TestApexMinSdkVersion_Okay(t *testing.T) {
	t.Parallel()
	testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["libfoo"],
			java_libs: ["libbar"],
			min_sdk_version: "29",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "libfoo",
			srcs: ["mylib.cpp"],
			shared_libs: ["libfoo_dep"],
			apex_available: ["myapex"],
			min_sdk_version: "29",
		}

		cc_library {
			name: "libfoo_dep",
			srcs: ["mylib.cpp"],
			apex_available: ["myapex"],
			min_sdk_version: "29",
		}

		java_library {
			name: "libbar",
			sdk_version: "current",
			srcs: ["a.java"],
			static_libs: [
				"libbar_dep",
				"libbar_import_dep",
			],
			apex_available: ["myapex"],
			min_sdk_version: "29",
			compile_dex: true,
		}

		java_library {
			name: "libbar_dep",
			sdk_version: "current",
			srcs: ["a.java"],
			apex_available: ["myapex"],
			min_sdk_version: "29",
		}

		java_import {
			name: "libbar_import_dep",
			jars: ["libbar.jar"],
			apex_available: ["myapex"],
			min_sdk_version: "29",
		}
	`)
}

func TestApexMinSdkVersion_MinApiForArch(t *testing.T) {
	t.Parallel()
	// Tests that an apex dependency with min_sdk_version higher than the
	// min_sdk_version of the apex is allowed as long as the dependency's
	// min_sdk_version is less than or equal to the api level that the
	// architecture was introduced in.  In this case, arm64 didn't exist
	// until api level 21, so the arm64 code will never need to run on
	// an api level 20 device, even if other architectures of the apex
	// will.
	testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["libfoo"],
			min_sdk_version: "20",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "libfoo",
			srcs: ["mylib.cpp"],
			apex_available: ["myapex"],
			min_sdk_version: "21",
			stl: "none",
		}
	`)
}

func TestJavaStableSdkVersion(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name          string
		expectedError string
		bp            string
		preparer      android.FixturePreparer
	}{
		{
			name: "Non-updatable apex with non-stable dep",
			bp: `
				apex {
					name: "myapex",
					java_libs: ["myjar"],
					key: "myapex.key",
					updatable: false,
				}
				apex_key {
					name: "myapex.key",
					public_key: "testkey.avbpubkey",
					private_key: "testkey.pem",
				}
				java_library {
					name: "myjar",
					srcs: ["foo/bar/MyClass.java"],
					sdk_version: "test_current",
					apex_available: ["myapex"],
					compile_dex: true,
				}
			`,
		},
		{
			name: "Updatable apex with stable dep",
			bp: `
				apex {
					name: "myapex",
					java_libs: ["myjar"],
					key: "myapex.key",
					updatable: true,
					min_sdk_version: "29",
				}
				apex_key {
					name: "myapex.key",
					public_key: "testkey.avbpubkey",
					private_key: "testkey.pem",
				}
				java_library {
					name: "myjar",
					srcs: ["foo/bar/MyClass.java"],
					sdk_version: "current",
					apex_available: ["myapex"],
					min_sdk_version: "29",
					compile_dex: true,
				}
			`,
		},
		{
			name:          "Updatable apex with non-stable dep",
			expectedError: "cannot depend on \"myjar\"",
			bp: `
				apex {
					name: "myapex",
					java_libs: ["myjar"],
					key: "myapex.key",
					updatable: true,
				}
				apex_key {
					name: "myapex.key",
					public_key: "testkey.avbpubkey",
					private_key: "testkey.pem",
				}
				java_library {
					name: "myjar",
					srcs: ["foo/bar/MyClass.java"],
					sdk_version: "test_current",
					apex_available: ["myapex"],
					compile_dex: true,
				}
			`,
		},
		{
			name:          "Updatable apex with non-stable legacy core platform dep",
			expectedError: `\Qcannot depend on "myjar-uses-legacy": non stable SDK core_platform_current - uses legacy core platform\E`,
			bp: `
				apex {
					name: "myapex",
					java_libs: ["myjar-uses-legacy"],
					key: "myapex.key",
					updatable: true,
				}
				apex_key {
					name: "myapex.key",
					public_key: "testkey.avbpubkey",
					private_key: "testkey.pem",
				}
				java_library {
					name: "myjar-uses-legacy",
					srcs: ["foo/bar/MyClass.java"],
					sdk_version: "core_platform",
					apex_available: ["myapex"],
					compile_dex: true,
				}
			`,
			preparer: java.FixtureUseLegacyCorePlatformApi("myjar-uses-legacy"),
		},
		{
			name: "Updatable apex with non-stable transitive dep",
			// This is not actually detecting that the transitive dependency is unstable, rather it is
			// detecting that the transitive dependency is building against a wider API surface than the
			// module that depends on it is using.
			expectedError: "compiles against Android API, but dependency \"transitive-jar\" is compiling against private API.",
			bp: `
				apex {
					name: "myapex",
					java_libs: ["myjar"],
					key: "myapex.key",
					updatable: true,
				}
				apex_key {
					name: "myapex.key",
					public_key: "testkey.avbpubkey",
					private_key: "testkey.pem",
				}
				java_library {
					name: "myjar",
					srcs: ["foo/bar/MyClass.java"],
					sdk_version: "current",
					apex_available: ["myapex"],
					static_libs: ["transitive-jar"],
					compile_dex: true,
				}
				java_library {
					name: "transitive-jar",
					srcs: ["foo/bar/MyClass.java"],
					sdk_version: "core_platform",
					apex_available: ["myapex"],
				}
			`,
		},
	}

	for _, test := range testCases {
		if test.name != "Updatable apex with non-stable legacy core platform dep" {
			continue
		}
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			errorHandler := android.FixtureExpectsNoErrors
			if test.expectedError != "" {
				errorHandler = android.FixtureExpectsAtLeastOneErrorMatchingPattern(test.expectedError)
			}
			android.GroupFixturePreparers(
				java.PrepareForTestWithJavaDefaultModules,
				PrepareForTestWithApexBuildComponents,
				prepareForTestWithMyapex,
				android.OptionalFixturePreparer(test.preparer),
			).
				ExtendWithErrorHandler(errorHandler).
				RunTestWithBp(t, test.bp)
		})
	}
}

func TestApexMinSdkVersion_ErrorIfDepIsNewer(t *testing.T) {
	testApexError(t, `module "mylib2".*: should support min_sdk_version\(29\) for "myapex"`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			min_sdk_version: "29",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: ["mylib2"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [
				"myapex",
			],
			min_sdk_version: "29",
		}

		// indirect part of the apex
		cc_library {
			name: "mylib2",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [
				"myapex",
			],
			min_sdk_version: "30",
		}
	`)
}

func TestApexMinSdkVersion_ErrorIfDepIsNewer_Java(t *testing.T) {
	t.Parallel()
	testApexError(t, `module "bar".*: should support min_sdk_version\(29\) for "myapex"`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			apps: ["AppFoo"],
			min_sdk_version: "29",
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		android_app {
			name: "AppFoo",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "current",
			min_sdk_version: "29",
			system_modules: "none",
			stl: "none",
			static_libs: ["bar"],
			apex_available: [ "myapex" ],
		}

		java_library {
			name: "bar",
			sdk_version: "current",
			srcs: ["a.java"],
			apex_available: [ "myapex" ],
		}
	`)
}

func TestApexMinSdkVersion_OkayEvenWhenDepIsNewer_IfItSatisfiesApexMinSdkVersion(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			min_sdk_version: "29",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		// mylib will link to mylib2#current
		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: ["mylib2"],
			system_shared_libs: [],
			stl: "none",
			apex_available: ["myapex", "otherapex"],
			min_sdk_version: "29",
		}

		cc_library {
			name: "mylib2",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: ["otherapex"],
			stubs: { versions: ["29", "30"] },
			min_sdk_version: "30",
		}

		apex {
			name: "otherapex",
			key: "myapex.key",
			native_shared_libs: ["mylib", "mylib2"],
			min_sdk_version: "30",
		}
	`)
	expectLink := func(from, from_variant, to, to_variant string) {
		ld := ctx.ModuleForTests(t, from, "android_arm64_armv8-a_"+from_variant).Rule("ld")
		libFlags := ld.Args["libFlags"]
		ensureContains(t, libFlags, "android_arm64_armv8-a_"+to_variant+"/"+to+".so")
	}
	expectLink("mylib", "shared_apex29", "mylib2", "shared_current")
	expectLink("mylib", "shared_apex30", "mylib2", "shared_current")
}

func TestApexMinSdkVersion_WorksWithSdkCodename(t *testing.T) {
	t.Parallel()
	withSAsActiveCodeNames := android.FixtureModifyProductVariables(
		func(variables android.FixtureProductVariables) {
			variables.Platform_sdk_codename = proptools.StringPtr("S")
			variables.Platform_version_active_codenames = []string{"S"}
		},
	)
	testApexError(t, `libbar.*: should support min_sdk_version\(S\)`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["libfoo"],
			min_sdk_version: "S",
		}
		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
		cc_library {
			name: "libfoo",
			shared_libs: ["libbar"],
			apex_available: ["myapex"],
			min_sdk_version: "29",
		}
		cc_library {
			name: "libbar",
			apex_available: ["myapex"],
		}
	`, withSAsActiveCodeNames)
}

func TestApexMinSdkVersion_WorksWithActiveCodenames(t *testing.T) {
	t.Parallel()
	withSAsActiveCodeNames := android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
		variables.Platform_sdk_codename = proptools.StringPtr("S")
		variables.Platform_version_active_codenames = []string{"S", "T"}
	})
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["libfoo"],
			min_sdk_version: "S",
		}
		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
		cc_library {
			name: "libfoo",
			shared_libs: ["libbar"],
			apex_available: ["myapex"],
			min_sdk_version: "S",
		}
		cc_library {
			name: "libbar",
			stubs: {
				symbol_file: "libbar.map.txt",
				versions: ["30", "S", "T"],
			},
		}
	`, withSAsActiveCodeNames)

	// ensure libfoo is linked with current version of libbar stub
	libfoo := ctx.ModuleForTests(t, "libfoo", "android_arm64_armv8-a_shared_apex10000")
	libFlags := libfoo.Rule("ld").Args["libFlags"]
	ensureContains(t, libFlags, "android_arm64_armv8-a_shared_current/libbar.so")
}

func TestFilesInSubDir(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			binaries: ["mybin", "mybin.rust"],
			prebuilts: ["myetc"],
			compile_multilib: "both",
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		prebuilt_etc {
			name: "myetc",
			src: "myprebuilt",
			sub_dir: "foo/bar",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			relative_install_path: "foo/bar",
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_binary {
			name: "mybin",
			srcs: ["mylib.cpp"],
			relative_install_path: "foo/bar",
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		rust_binary {
			name: "mybin.rust",
			srcs: ["foo.rs"],
			relative_install_path: "rust_subdir",
			apex_available: [ "myapex" ],
		}
	`)

	generateFsRule := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Rule("generateFsConfig")
	cmd := generateFsRule.RuleParams.Command

	// Ensure that the subdirectories are all listed
	ensureContains(t, cmd, "/etc ")
	ensureContains(t, cmd, "/etc/foo ")
	ensureContains(t, cmd, "/etc/foo/bar ")
	ensureContains(t, cmd, "/lib64 ")
	ensureContains(t, cmd, "/lib64/foo ")
	ensureContains(t, cmd, "/lib64/foo/bar ")
	ensureContains(t, cmd, "/lib ")
	ensureContains(t, cmd, "/lib/foo ")
	ensureContains(t, cmd, "/lib/foo/bar ")
	ensureContains(t, cmd, "/bin ")
	ensureContains(t, cmd, "/bin/foo ")
	ensureContains(t, cmd, "/bin/foo/bar ")
	ensureContains(t, cmd, "/bin/rust_subdir ")
}

func TestFilesInSubDirWhenNativeBridgeEnabled(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			multilib: {
				both: {
					native_shared_libs: ["mylib"],
					binaries: ["mybin"],
				},
			},
			compile_multilib: "both",
			native_bridge_supported: true,
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			relative_install_path: "foo/bar",
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
			native_bridge_supported: true,
		}

		cc_binary {
			name: "mybin",
			relative_install_path: "foo/bar",
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
			native_bridge_supported: true,
			compile_multilib: "both", // default is "first" for binary
			multilib: {
				lib64: {
					suffix: "64",
				},
			},
		}
	`, android.PrepareForNativeBridgeEnabled)
	ensureExactContents(t, ctx, "myapex", "android_common_myapex", []string{
		"bin/foo/bar/mybin",
		"bin/foo/bar/mybin64",
		"bin/arm/foo/bar/mybin",
		"bin/arm64/foo/bar/mybin64",
		"lib/foo/bar/mylib.so",
		"lib/arm/foo/bar/mylib.so",
		"lib64/foo/bar/mylib.so",
		"lib64/arm64/foo/bar/mylib.so",
	})
}

func TestVendorApex(t *testing.T) {
	t.Parallel()
	result := android.GroupFixturePreparers(
		prepareForApexTest,
		android.FixtureModifyConfig(android.SetKatiEnabledForTests),
	).RunTestWithBp(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			binaries: ["mybin"],
			vendor: true,
			updatable: false,
		}
		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
		cc_binary {
			name: "mybin",
			vendor: true,
			shared_libs: ["libfoo"],
		}
		cc_library {
			name: "libfoo",
			proprietary: true,
		}
	`)

	ensureExactContents(t, result.TestContext, "myapex", "android_common_myapex", []string{
		"bin/mybin",
		"lib64/libfoo.so",
		// TODO(b/159195575): Add an option to use VNDK libs from VNDK APEX
		"lib64/libc++.so",
	})

	apexBundle := result.ModuleForTests(t, "myapex", "android_common_myapex").Module().(*apexBundle)
	data := android.AndroidMkDataForTest(t, result.TestContext, apexBundle)
	name := apexBundle.BaseModuleName()
	prefix := "TARGET_"
	var builder strings.Builder
	data.Custom(&builder, name, prefix, "", data)
	androidMk := android.StringRelativeToTop(result.Config, builder.String())
	installPath := "out/target/product/test_device/vendor/apex"
	ensureContains(t, androidMk, "LOCAL_MODULE_PATH := "+installPath)

	apexManifestRule := result.ModuleForTests(t, "myapex", "android_common_myapex").Rule("apexManifestRule")
	requireNativeLibs := names(apexManifestRule.Args["requireNativeLibs"])
	ensureListNotContains(t, requireNativeLibs, ":vndk")
}

func TestProductVariant(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			updatable: false,
			product_specific: true,
			binaries: ["foo"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_binary {
			name: "foo",
			product_available: true,
			apex_available: ["myapex"],
			srcs: ["foo.cpp"],
		}
	`)

	cflags := strings.Fields(
		ctx.ModuleForTests(t, "foo", "android_product_arm64_armv8-a_apex10000").Rule("cc").Args["cFlags"])
	ensureListContains(t, cflags, "-D__ANDROID_VNDK__")
	ensureListContains(t, cflags, "-D__ANDROID_APEX__")
	ensureListContains(t, cflags, "-D__ANDROID_PRODUCT__")
	ensureListNotContains(t, cflags, "-D__ANDROID_VENDOR__")
}

func TestApex_withPrebuiltFirmware(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name           string
		additionalProp string
	}{
		{"system apex with prebuilt_firmware", ""},
		{"vendor apex with prebuilt_firmware", "vendor: true,"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testApex(t, `
				apex {
					name: "myapex",
					key: "myapex.key",
					prebuilts: ["myfirmware"],
					updatable: false,
					`+tc.additionalProp+`
				}
				apex_key {
					name: "myapex.key",
					public_key: "testkey.avbpubkey",
					private_key: "testkey.pem",
				}
				prebuilt_firmware {
					name: "myfirmware",
					src: "myfirmware.bin",
					filename_from_src: true,
					`+tc.additionalProp+`
				}
			`)
			ensureExactContents(t, ctx, "myapex", "android_common_myapex", []string{
				"etc/firmware/myfirmware.bin",
			})
		})
	}
}

func TestAndroidMk_VendorApexRequired(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			vendor: true,
			native_shared_libs: ["mylib"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			vendor_available: true,
		}
	`)

	apexBundle := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Module().(*apexBundle)
	data := android.AndroidMkDataForTest(t, ctx, apexBundle)
	name := apexBundle.BaseModuleName()
	prefix := "TARGET_"
	var builder strings.Builder
	data.Custom(&builder, name, prefix, "", data)
	androidMk := builder.String()
	ensureContains(t, androidMk, "LOCAL_REQUIRED_MODULES := libc++.vendor.myapex:64 mylib.vendor.myapex:64 libc.vendor libm.vendor libdl.vendor\n")
}

func TestAndroidMkWritesCommonProperties(t *testing.T) {
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			vintf_fragments: ["fragment.xml"],
			init_rc: ["init.rc"],
			updatable: false,
		}
		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
		cc_binary {
			name: "mybin",
		}
	`)

	apexBundle := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Module().(*apexBundle)
	data := android.AndroidMkDataForTest(t, ctx, apexBundle)
	name := apexBundle.BaseModuleName()
	prefix := "TARGET_"
	var builder strings.Builder
	data.Custom(&builder, name, prefix, "", data)
	androidMk := builder.String()
	ensureContains(t, androidMk, "LOCAL_FULL_VINTF_FRAGMENTS := fragment.xml\n")
	ensureContains(t, androidMk, "LOCAL_FULL_INIT_RC := init.rc\n")
}

func TestStaticLinking(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["1", "2", "3"],
			},
			apex_available: ["myapex"],
		}

		rust_ffi {
			name: "libmylib_rs",
			crate_name: "mylib_rs",
			srcs: ["mylib.rs"],
			stubs: {
				versions: ["1", "2", "3"],
			},
			apex_available: ["myapex"],
		}

		cc_binary {
			name: "not_in_apex",
			srcs: ["mylib.cpp"],
			static_libs: ["mylib", "libmylib_rs"],
			static_executable: true,
			system_shared_libs: [],
			stl: "none",
		}
	`)

	ldFlags := ctx.ModuleForTests(t, "not_in_apex", "android_arm64_armv8-a").Rule("ld").Args["libFlags"]

	// Ensure that not_in_apex is linking with the static variant of mylib
	ensureContains(t, ldFlags, "mylib/android_arm64_armv8-a_static/mylib.a")
	ensureContains(t, ldFlags, "generated_rust_staticlib/librustlibs.a")
}

func TestKeys(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex_keytest",
			key: "myapex.key",
			certificate: ":myapex.certificate",
			native_shared_libs: ["mylib"],
			file_contexts: ":myapex-file_contexts",
			updatable: false,
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex_keytest" ],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		android_app_certificate {
			name: "myapex.certificate",
			certificate: "testkey",
		}

		android_app_certificate {
			name: "myapex.certificate.override",
			certificate: "testkey.override",
		}

	`)

	// check the APEX keys
	keys := ctx.ModuleForTests(t, "myapex.key", "android_common").Module().(*apexKey)

	if keys.publicKeyFile.String() != "vendor/foo/devkeys/testkey.avbpubkey" {
		t.Errorf("public key %q is not %q", keys.publicKeyFile.String(),
			"vendor/foo/devkeys/testkey.avbpubkey")
	}
	if keys.privateKeyFile.String() != "vendor/foo/devkeys/testkey.pem" {
		t.Errorf("private key %q is not %q", keys.privateKeyFile.String(),
			"vendor/foo/devkeys/testkey.pem")
	}

	// check the APK certs. It should be overridden to myapex.certificate.override
	certs := ctx.ModuleForTests(t, "myapex_keytest", "android_common_myapex_keytest").Rule("signapk").Args["certificates"]
	if certs != "testkey.override.x509.pem testkey.override.pk8" {
		t.Errorf("cert and private key %q are not %q", certs,
			"testkey.override.509.pem testkey.override.pk8")
	}
}

func TestCertificate(t *testing.T) {
	t.Parallel()
	t.Run("if unspecified, it defaults to DefaultAppCertificate", func(t *testing.T) {
		t.Parallel()
		ctx := testApex(t, `
			apex {
				name: "myapex",
				key: "myapex.key",
				updatable: false,
			}
			apex_key {
				name: "myapex.key",
				public_key: "testkey.avbpubkey",
				private_key: "testkey.pem",
			}`)
		rule := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Rule("signapk")
		expected := "vendor/foo/devkeys/test.x509.pem vendor/foo/devkeys/test.pk8"
		if actual := rule.Args["certificates"]; actual != expected {
			t.Errorf("certificates should be %q, not %q", expected, actual)
		}
	})
	t.Run("override when unspecified", func(t *testing.T) {
		t.Parallel()
		ctx := testApex(t, `
			apex {
				name: "myapex_keytest",
				key: "myapex.key",
				file_contexts: ":myapex-file_contexts",
				updatable: false,
			}
			apex_key {
				name: "myapex.key",
				public_key: "testkey.avbpubkey",
				private_key: "testkey.pem",
			}
			android_app_certificate {
				name: "myapex.certificate.override",
				certificate: "testkey.override",
			}`)
		rule := ctx.ModuleForTests(t, "myapex_keytest", "android_common_myapex_keytest").Rule("signapk")
		expected := "testkey.override.x509.pem testkey.override.pk8"
		if actual := rule.Args["certificates"]; actual != expected {
			t.Errorf("certificates should be %q, not %q", expected, actual)
		}
	})
	t.Run("if specified as :module, it respects the prop", func(t *testing.T) {
		t.Parallel()
		ctx := testApex(t, `
			apex {
				name: "myapex",
				key: "myapex.key",
				certificate: ":myapex.certificate",
				updatable: false,
			}
			apex_key {
				name: "myapex.key",
				public_key: "testkey.avbpubkey",
				private_key: "testkey.pem",
			}
			android_app_certificate {
				name: "myapex.certificate",
				certificate: "testkey",
			}`)
		rule := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Rule("signapk")
		expected := "testkey.x509.pem testkey.pk8"
		if actual := rule.Args["certificates"]; actual != expected {
			t.Errorf("certificates should be %q, not %q", expected, actual)
		}
	})
	t.Run("override when specifiec as <:module>", func(t *testing.T) {
		t.Parallel()
		ctx := testApex(t, `
			apex {
				name: "myapex_keytest",
				key: "myapex.key",
				file_contexts: ":myapex-file_contexts",
				certificate: ":myapex.certificate",
				updatable: false,
			}
			apex_key {
				name: "myapex.key",
				public_key: "testkey.avbpubkey",
				private_key: "testkey.pem",
			}
			android_app_certificate {
				name: "myapex.certificate.override",
				certificate: "testkey.override",
			}`)
		rule := ctx.ModuleForTests(t, "myapex_keytest", "android_common_myapex_keytest").Rule("signapk")
		expected := "testkey.override.x509.pem testkey.override.pk8"
		if actual := rule.Args["certificates"]; actual != expected {
			t.Errorf("certificates should be %q, not %q", expected, actual)
		}
	})
	t.Run("if specified as name, finds it from DefaultDevKeyDir", func(t *testing.T) {
		t.Parallel()
		ctx := testApex(t, `
			apex {
				name: "myapex",
				key: "myapex.key",
				certificate: "testkey",
				updatable: false,
			}
			apex_key {
				name: "myapex.key",
				public_key: "testkey.avbpubkey",
				private_key: "testkey.pem",
			}`,
			android.MockFS{
				"vendor/foo/devkeys/testkey.x509.pem": nil,
			}.AddToFixture(),
		)
		rule := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Rule("signapk")
		expected := "vendor/foo/devkeys/testkey.x509.pem vendor/foo/devkeys/testkey.pk8"
		if actual := rule.Args["certificates"]; actual != expected {
			t.Errorf("certificates should be %q, not %q", expected, actual)
		}
	})
	t.Run("override when specified as <name>", func(t *testing.T) {
		t.Parallel()
		ctx := testApex(t, `
			apex {
				name: "myapex_keytest",
				key: "myapex.key",
				file_contexts: ":myapex-file_contexts",
				certificate: "testkey",
				updatable: false,
			}
			apex_key {
				name: "myapex.key",
				public_key: "testkey.avbpubkey",
				private_key: "testkey.pem",
			}
			android_app_certificate {
				name: "myapex.certificate.override",
				certificate: "testkey.override",
			}`)
		rule := ctx.ModuleForTests(t, "myapex_keytest", "android_common_myapex_keytest").Rule("signapk")
		expected := "testkey.override.x509.pem testkey.override.pk8"
		if actual := rule.Args["certificates"]; actual != expected {
			t.Errorf("certificates should be %q, not %q", expected, actual)
		}
	})
}

func TestMacro(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib", "mylib2"],
			updatable: false,
		}

		apex {
			name: "otherapex",
			key: "myapex.key",
			native_shared_libs: ["mylib", "mylib2"],
			min_sdk_version: "29",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [
				"myapex",
				"otherapex",
			],
			recovery_available: true,
			min_sdk_version: "29",
		}
		cc_library {
			name: "mylib2",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [
				"myapex",
				"otherapex",
			],
			static_libs: ["mylib3"],
			recovery_available: true,
			min_sdk_version: "29",
		}
		cc_library {
			name: "mylib3",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [
				"myapex",
				"otherapex",
			],
			recovery_available: true,
			min_sdk_version: "29",
		}
	`)

	// non-APEX variant does not have __ANDROID_APEX__ defined
	mylibCFlags := ctx.ModuleForTests(t, "mylib", "android_arm64_armv8-a_static").Rule("cc").Args["cFlags"]
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_APEX__")

	// APEX variant has __ANDROID_APEX__ and __ANDROID_APEX__ defined
	mylibCFlags = ctx.ModuleForTests(t, "mylib", "android_arm64_armv8-a_static_apex10000").Rule("cc").Args["cFlags"]
	ensureContains(t, mylibCFlags, "-D__ANDROID_APEX__")

	// APEX variant has __ANDROID_APEX__ and __ANDROID_APEX__ defined
	mylibCFlags = ctx.ModuleForTests(t, "mylib", "android_arm64_armv8-a_static_apex29").Rule("cc").Args["cFlags"]
	ensureContains(t, mylibCFlags, "-D__ANDROID_APEX__")

	// When a cc_library sets use_apex_name_macro: true each apex gets a unique variant and
	// each variant defines additional macros to distinguish which apex variant it is built for

	// non-APEX variant does not have __ANDROID_APEX__ defined
	mylibCFlags = ctx.ModuleForTests(t, "mylib3", "android_arm64_armv8-a_static").Rule("cc").Args["cFlags"]
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_APEX__")

	// recovery variant does not set __ANDROID_APEX__
	mylibCFlags = ctx.ModuleForTests(t, "mylib3", "android_recovery_arm64_armv8-a_static").Rule("cc").Args["cFlags"]
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_APEX__")

	// non-APEX variant does not have __ANDROID_APEX__ defined
	mylibCFlags = ctx.ModuleForTests(t, "mylib2", "android_arm64_armv8-a_static").Rule("cc").Args["cFlags"]
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_APEX__")

	// recovery variant does not set __ANDROID_APEX__
	mylibCFlags = ctx.ModuleForTests(t, "mylib2", "android_recovery_arm64_armv8-a_static").Rule("cc").Args["cFlags"]
	ensureNotContains(t, mylibCFlags, "-D__ANDROID_APEX__")
}

func TestHeaderLibsDependency(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library_headers {
			name: "mylib_headers",
			export_include_dirs: ["my_include"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			header_libs: ["mylib_headers"],
			export_header_lib_headers: ["mylib_headers"],
			stubs: {
				versions: ["1", "2", "3"],
			},
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "otherlib",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			shared_libs: ["mylib"],
		}
	`)

	cFlags := ctx.ModuleForTests(t, "otherlib", "android_arm64_armv8-a_static").Rule("cc").Args["cFlags"]

	// Ensure that the include path of the header lib is exported to 'otherlib'
	ensureContains(t, cFlags, "-Imy_include")
}

type fileInApex struct {
	path   string // path in apex
	src    string // src path
	isLink bool
}

func (f fileInApex) String() string {
	return f.src + ":" + f.path
}

func (f fileInApex) match(expectation string) bool {
	parts := strings.Split(expectation, ":")
	if len(parts) == 1 {
		match, _ := path.Match(parts[0], f.path)
		return match
	}
	if len(parts) == 2 {
		matchSrc, _ := path.Match(parts[0], f.src)
		matchDst, _ := path.Match(parts[1], f.path)
		return matchSrc && matchDst
	}
	panic("invalid expected file specification: " + expectation)
}

func getFiles(t *testing.T, ctx *android.TestContext, moduleName, variant string) []fileInApex {
	t.Helper()
	module := ctx.ModuleForTests(t, moduleName, variant)
	apexRule := module.MaybeRule("apexRule")
	apexDir := "/image.apex/"
	copyCmds := apexRule.Args["copy_commands"]
	var ret []fileInApex
	for _, cmd := range strings.Split(copyCmds, "&&") {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}
		terms := strings.Split(cmd, " ")
		var dst, src string
		var isLink bool
		switch terms[0] {
		case "mkdir":
		case "cp":
			if len(terms) != 3 && len(terms) != 4 {
				t.Fatal("copyCmds contains invalid cp command", cmd)
			}
			dst = terms[len(terms)-1]
			src = terms[len(terms)-2]
			isLink = false
		case "ln":
			if len(terms) != 3 && len(terms) != 4 {
				// ln LINK TARGET or ln -s LINK TARGET
				t.Fatal("copyCmds contains invalid ln command", cmd)
			}
			dst = terms[len(terms)-1]
			src = terms[len(terms)-2]
			isLink = true
		default:
			t.Fatalf("copyCmds should contain mkdir/cp commands only: %q", cmd)
		}
		if dst != "" {
			index := strings.Index(dst, apexDir)
			if index == -1 {
				t.Fatal("copyCmds should copy a file to "+apexDir, cmd)
			}
			dstFile := dst[index+len(apexDir):]
			ret = append(ret, fileInApex{path: dstFile, src: src, isLink: isLink})
		}
	}
	return ret
}

func assertFileListEquals(t *testing.T, expectedFiles []string, actualFiles []fileInApex) {
	t.Helper()
	var failed bool
	var surplus []string
	filesMatched := make(map[string]bool)
	for _, file := range actualFiles {
		matchFound := false
		for _, expected := range expectedFiles {
			if file.match(expected) {
				matchFound = true
				filesMatched[expected] = true
				break
			}
		}
		if !matchFound {
			surplus = append(surplus, file.String())
		}
	}

	if len(surplus) > 0 {
		sort.Strings(surplus)
		t.Log("surplus files", surplus)
		failed = true
	}

	if len(expectedFiles) > len(filesMatched) {
		var missing []string
		for _, expected := range expectedFiles {
			if !filesMatched[expected] {
				missing = append(missing, expected)
			}
		}
		sort.Strings(missing)
		t.Log("missing files", missing)
		failed = true
	}
	if failed {
		t.Fail()
	}
}

func ensureExactContents(t *testing.T, ctx *android.TestContext, moduleName, variant string, files []string) {
	assertFileListEquals(t, files, getFiles(t, ctx, moduleName, variant))
}

func ensureExactDeapexedContents(t *testing.T, ctx *android.TestContext, moduleName string, variant string, files []string) {
	deapexer := ctx.ModuleForTests(t, moduleName, variant).Description("deapex")
	outputs := make([]string, 0, len(deapexer.ImplicitOutputs)+1)
	if deapexer.Output != nil {
		outputs = append(outputs, deapexer.Output.String())
	}
	for _, output := range deapexer.ImplicitOutputs {
		outputs = append(outputs, output.String())
	}
	actualFiles := make([]fileInApex, 0, len(outputs))
	for _, output := range outputs {
		dir := "/deapexer/"
		pos := strings.LastIndex(output, dir)
		if pos == -1 {
			t.Fatal("Unknown deapexer output ", output)
		}
		path := output[pos+len(dir):]
		actualFiles = append(actualFiles, fileInApex{path: path, src: "", isLink: false})
	}
	assertFileListEquals(t, files, actualFiles)
}

func vndkLibrariesTxtFiles(vers ...string) (result string) {
	for _, v := range vers {
		for _, txt := range []string{"llndk", "vndkcore", "vndksp", "vndkprivate", "vndkproduct"} {
			result += `
					prebuilt_etc {
						name: "` + txt + `.libraries.` + v + `.txt",
						src: "dummy.txt",
					}
				`
		}
	}
	return
}

func TestVndkApexVersion(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex_vndk {
			name: "com.android.vndk.v27",
			key: "myapex.key",
			file_contexts: ":myapex-file_contexts",
			vndk_version: "27",
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		vndk_prebuilt_shared {
			name: "libvndk27",
			version: "27",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			target_arch: "arm64",
			arch: {
				arm: {
					srcs: ["libvndk27_arm.so"],
				},
				arm64: {
					srcs: ["libvndk27_arm64.so"],
				},
			},
			apex_available: [ "com.android.vndk.v27" ],
		}

		vndk_prebuilt_shared {
			name: "libvndk27",
			version: "27",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			target_arch: "x86_64",
			arch: {
				x86: {
					srcs: ["libvndk27_x86.so"],
				},
				x86_64: {
					srcs: ["libvndk27_x86_64.so"],
				},
			},
		}
		`+vndkLibrariesTxtFiles("27"),
		withFiles(map[string][]byte{
			"libvndk27_arm.so":    nil,
			"libvndk27_arm64.so":  nil,
			"libvndk27_x86.so":    nil,
			"libvndk27_x86_64.so": nil,
		}))

	ensureExactContents(t, ctx, "com.android.vndk.v27", "android_common", []string{
		"lib/libvndk27_arm.so",
		"lib64/libvndk27_arm64.so",
		"etc/*",
	})
}

func TestVndkApexNameRule(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex_vndk {
			name: "com.android.vndk.v29",
			key: "myapex.key",
			file_contexts: ":myapex-file_contexts",
			vndk_version: "29",
			updatable: false,
		}
		apex_vndk {
			name: "com.android.vndk.v28",
			key: "myapex.key",
			file_contexts: ":myapex-file_contexts",
			vndk_version: "28",
			updatable: false,
		}
		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}`+vndkLibrariesTxtFiles("28", "29"))

	assertApexName := func(expected, moduleName string) {
		module := ctx.ModuleForTests(t, moduleName, "android_common")
		apexManifestRule := module.Rule("apexManifestRule")
		ensureContains(t, apexManifestRule.Args["opt"], "-v name "+expected)
	}

	assertApexName("com.android.vndk.v29", "com.android.vndk.v29")
	assertApexName("com.android.vndk.v28", "com.android.vndk.v28")
}

func TestVndkApexDoesntSupportNativeBridgeSupported(t *testing.T) {
	t.Parallel()
	testApexError(t, `module "com.android.vndk.v30" .*: native_bridge_supported: .* doesn't support native bridge binary`, `
		apex_vndk {
			name: "com.android.vndk.v30",
			key: "com.android.vndk.v30.key",
			file_contexts: ":myapex-file_contexts",
			native_bridge_supported: true,
		}

		apex_key {
			name: "com.android.vndk.v30.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		vndk_prebuilt_shared {
			name: "libvndk",
			version: "30",
			target_arch: "arm",
			srcs: ["mylib.cpp"],
			vendor_available: true,
			product_available: true,
			native_bridge_supported: true,
			vndk: {
				enabled: true,
			},
		}
	`)
}

func TestVndkApexWithBinder32(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex_vndk {
			name: "com.android.vndk.v27",
			key: "myapex.key",
			file_contexts: ":myapex-file_contexts",
			vndk_version: "27",
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		vndk_prebuilt_shared {
			name: "libvndk27",
			version: "27",
			target_arch: "arm",
			vendor_available: true,
			product_available: true,
			vndk: {
				enabled: true,
			},
			arch: {
				arm: {
					srcs: ["libvndk27.so"],
				}
			},
		}

		vndk_prebuilt_shared {
			name: "libvndk27",
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
					srcs: ["libvndk27binder32.so"],
				}
			},
			apex_available: [ "com.android.vndk.v27" ],
		}
		`+vndkLibrariesTxtFiles("27"),
		withFiles(map[string][]byte{
			"libvndk27.so":         nil,
			"libvndk27binder32.so": nil,
		}),
		withBinder32bit,
		android.FixtureModifyConfig(func(config android.Config) {
			target := android.Target{
				Os: android.Android,
				Arch: android.Arch{
					ArchType:    android.Arm,
					ArchVariant: "armv7-a-neon",
					Abi:         []string{"armeabi-v7a"},
				},
				NativeBridge:             android.NativeBridgeDisabled,
				NativeBridgeHostArchName: "",
				NativeBridgeRelativePath: "",
			}
			config.Targets[android.Android] = []android.Target{target}
			config.AndroidFirstDeviceTarget = target
		}),
	)

	ensureExactContents(t, ctx, "com.android.vndk.v27", "android_common", []string{
		"lib/libvndk27binder32.so",
		"etc/*",
	})
}

func TestDependenciesInApexManifest(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex_nodep",
			key: "myapex.key",
			native_shared_libs: ["lib_nodep"],
			compile_multilib: "both",
			file_contexts: ":myapex-file_contexts",
			updatable: false,
		}

		apex {
			name: "myapex_dep",
			key: "myapex.key",
			native_shared_libs: ["lib_dep"],
			compile_multilib: "both",
			file_contexts: ":myapex-file_contexts",
			updatable: false,
		}

		apex {
			name: "myapex_provider",
			key: "myapex.key",
			native_shared_libs: ["libfoo"],
			compile_multilib: "both",
			file_contexts: ":myapex-file_contexts",
			updatable: false,
		}

		apex {
			name: "myapex_selfcontained",
			key: "myapex.key",
			native_shared_libs: ["lib_dep_on_bar", "libbar"],
			compile_multilib: "both",
			file_contexts: ":myapex-file_contexts",
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "lib_nodep",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex_nodep" ],
		}

		cc_library {
			name: "lib_dep",
			srcs: ["mylib.cpp"],
			shared_libs: ["libfoo"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [
				"myapex_dep",
				"myapex_provider",
				"myapex_selfcontained",
			],
		}

		cc_library {
			name: "lib_dep_on_bar",
			srcs: ["mylib.cpp"],
			shared_libs: ["libbar"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [
				"myapex_selfcontained",
			],
		}


		cc_library {
			name: "libfoo",
			srcs: ["mytest.cpp"],
			stubs: {
				versions: ["1"],
			},
			system_shared_libs: [],
			stl: "none",
			apex_available: [
				"myapex_provider",
			],
		}

		cc_library {
			name: "libbar",
			srcs: ["mytest.cpp"],
			stubs: {
				versions: ["1"],
			},
			system_shared_libs: [],
			stl: "none",
			apex_available: [
				"myapex_selfcontained",
			],
		}

	`)

	var apexManifestRule android.TestingBuildParams
	var provideNativeLibs, requireNativeLibs []string

	apexManifestRule = ctx.ModuleForTests(t, "myapex_nodep", "android_common_myapex_nodep").Rule("apexManifestRule")
	provideNativeLibs = names(apexManifestRule.Args["provideNativeLibs"])
	requireNativeLibs = names(apexManifestRule.Args["requireNativeLibs"])
	ensureListEmpty(t, provideNativeLibs)
	ensureListEmpty(t, requireNativeLibs)

	apexManifestRule = ctx.ModuleForTests(t, "myapex_dep", "android_common_myapex_dep").Rule("apexManifestRule")
	provideNativeLibs = names(apexManifestRule.Args["provideNativeLibs"])
	requireNativeLibs = names(apexManifestRule.Args["requireNativeLibs"])
	ensureListEmpty(t, provideNativeLibs)
	ensureListContains(t, requireNativeLibs, "libfoo.so")

	apexManifestRule = ctx.ModuleForTests(t, "myapex_provider", "android_common_myapex_provider").Rule("apexManifestRule")
	provideNativeLibs = names(apexManifestRule.Args["provideNativeLibs"])
	requireNativeLibs = names(apexManifestRule.Args["requireNativeLibs"])
	ensureListContains(t, provideNativeLibs, "libfoo.so")
	ensureListEmpty(t, requireNativeLibs)

	apexManifestRule = ctx.ModuleForTests(t, "myapex_selfcontained", "android_common_myapex_selfcontained").Rule("apexManifestRule")
	provideNativeLibs = names(apexManifestRule.Args["provideNativeLibs"])
	requireNativeLibs = names(apexManifestRule.Args["requireNativeLibs"])
	ensureListContains(t, provideNativeLibs, "libbar.so")
	ensureListEmpty(t, requireNativeLibs)
}

func TestOverrideApexManifestDefaultVersion(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [
				"//apex_available:platform",
				"myapex",
			],
		}
	`, android.FixtureMergeEnv(map[string]string{
		"OVERRIDE_APEX_MANIFEST_DEFAULT_VERSION": "1234",
	}))

	module := ctx.ModuleForTests(t, "myapex", "android_common_myapex")
	apexManifestRule := module.Rule("apexManifestRule")
	ensureContains(t, apexManifestRule.Args["default_version"], "1234")
}

func TestCompileMultilibProp(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		compileMultiLibProp string
		containedLibs       []string
		notContainedLibs    []string
	}{
		{
			containedLibs: []string{
				"image.apex/lib64/mylib.so",
				"image.apex/lib/mylib.so",
			},
			compileMultiLibProp: `compile_multilib: "both",`,
		},
		{
			containedLibs:       []string{"image.apex/lib64/mylib.so"},
			notContainedLibs:    []string{"image.apex/lib/mylib.so"},
			compileMultiLibProp: `compile_multilib: "first",`,
		},
		{
			containedLibs:    []string{"image.apex/lib64/mylib.so"},
			notContainedLibs: []string{"image.apex/lib/mylib.so"},
			// compile_multilib, when unset, should result to the same output as when compile_multilib is "first"
		},
		{
			containedLibs:       []string{"image.apex/lib64/mylib.so"},
			notContainedLibs:    []string{"image.apex/lib/mylib.so"},
			compileMultiLibProp: `compile_multilib: "64",`,
		},
		{
			containedLibs:       []string{"image.apex/lib/mylib.so"},
			notContainedLibs:    []string{"image.apex/lib64/mylib.so"},
			compileMultiLibProp: `compile_multilib: "32",`,
		},
	}
	for _, testCase := range testCases {
		ctx := testApex(t, fmt.Sprintf(`
			apex {
				name: "myapex",
				key: "myapex.key",
				%s
				native_shared_libs: ["mylib"],
				updatable: false,
			}
			apex_key {
				name: "myapex.key",
				public_key: "testkey.avbpubkey",
				private_key: "testkey.pem",
			}
			cc_library {
				name: "mylib",
				srcs: ["mylib.cpp"],
				apex_available: [
					"//apex_available:platform",
					"myapex",
			],
			}
		`, testCase.compileMultiLibProp),
		)
		module := ctx.ModuleForTests(t, "myapex", "android_common_myapex")
		apexRule := module.Rule("apexRule")
		copyCmds := apexRule.Args["copy_commands"]
		for _, containedLib := range testCase.containedLibs {
			ensureContains(t, copyCmds, containedLib)
		}
		for _, notContainedLib := range testCase.notContainedLibs {
			ensureNotContains(t, copyCmds, notContainedLib)
		}
	}
}

func TestNonTestApex(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib_common"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib_common",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [
					"//apex_available:platform",
				  "myapex",
		  ],
		}
	`)

	module := ctx.ModuleForTests(t, "myapex", "android_common_myapex")
	apexRule := module.Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	if apex, ok := module.Module().(*apexBundle); !ok || apex.testApex {
		t.Log("Apex was a test apex!")
		t.Fail()
	}
	// Ensure that main rule creates an output
	ensureContains(t, apexRule.Output.String(), "myapex.apex.unsigned")

	// Ensure that apex variant is created for the direct dep
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib_common"), "android_arm64_armv8-a_shared_apex10000")

	// Ensure that both direct and indirect deps are copied into apex
	ensureContains(t, copyCmds, "image.apex/lib64/mylib_common.so")

	// Ensure that the platform variant ends with _shared
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib_common"), "android_arm64_armv8-a_shared")

	if !ctx.ModuleForTests(t, "mylib_common", "android_arm64_armv8-a_shared_apex10000").Module().(*cc.Module).InAnyApex() {
		t.Log("Found mylib_common not in any apex!")
		t.Fail()
	}
}

func TestTestApex(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex_test {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib_common_test"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib_common_test",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			// TODO: remove //apex_available:platform
			apex_available: [
				"//apex_available:platform",
				"myapex",
			],
		}
	`)

	module := ctx.ModuleForTests(t, "myapex", "android_common_myapex")
	apexRule := module.Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	if apex, ok := module.Module().(*apexBundle); !ok || !apex.testApex {
		t.Log("Apex was not a test apex!")
		t.Fail()
	}
	// Ensure that main rule creates an output
	ensureContains(t, apexRule.Output.String(), "myapex.apex.unsigned")

	// Ensure that apex variant is created for the direct dep
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib_common_test"), "android_arm64_armv8-a_shared_apex10000")

	// Ensure that both direct and indirect deps are copied into apex
	ensureContains(t, copyCmds, "image.apex/lib64/mylib_common_test.so")

	// Ensure that the platform variant ends with _shared
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib_common_test"), "android_arm64_armv8-a_shared")
}

func TestLibzVendorIsntStable(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
	apex {
		name: "myapex",
		key: "myapex.key",
		updatable: false,
		binaries: ["mybin"],
	}
	apex {
		name: "myvendorapex",
		key: "myapex.key",
		file_contexts: "myvendorapex_file_contexts",
		vendor: true,
		updatable: false,
		binaries: ["mybin"],
	}
	apex_key {
		name: "myapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}
	cc_binary {
		name: "mybin",
		vendor_available: true,
		system_shared_libs: [],
		stl: "none",
		shared_libs: ["libz"],
		apex_available: ["//apex_available:anyapex"],
	}
	cc_library {
		name: "libz",
		vendor_available: true,
		system_shared_libs: [],
		stl: "none",
		stubs: {
			versions: ["28", "30"],
		},
		target: {
			vendor: {
				no_stubs: true,
			},
		},
	}
	`, withFiles(map[string][]byte{
		"myvendorapex_file_contexts": nil,
	}))

	// libz provides stubs for core variant.
	{
		ensureExactContents(t, ctx, "myapex", "android_common_myapex", []string{
			"bin/mybin",
		})
		apexManifestRule := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Rule("apexManifestRule")
		android.AssertStringEquals(t, "should require libz", apexManifestRule.Args["requireNativeLibs"], "libz.so")
	}
	// libz doesn't provide stubs for vendor variant.
	{
		ensureExactContents(t, ctx, "myvendorapex", "android_common_myvendorapex", []string{
			"bin/mybin",
			"lib64/libz.so",
		})
		apexManifestRule := ctx.ModuleForTests(t, "myvendorapex", "android_common_myvendorapex").Rule("apexManifestRule")
		android.AssertStringEquals(t, "should not require libz", apexManifestRule.Args["requireNativeLibs"], "")
	}
}

func TestApexWithTarget(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			updatable: false,
			multilib: {
				first: {
					native_shared_libs: ["mylib_common"],
				}
			},
			target: {
				android: {
					multilib: {
						first: {
							native_shared_libs: ["mylib"],
						}
					}
				},
				host: {
					multilib: {
						first: {
							native_shared_libs: ["mylib2"],
						}
					}
				}
			}
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			// TODO: remove //apex_available:platform
			apex_available: [
				"//apex_available:platform",
				"myapex",
			],
		}

		cc_library {
			name: "mylib_common",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			compile_multilib: "first",
			// TODO: remove //apex_available:platform
			apex_available: [
				"//apex_available:platform",
				"myapex",
			],
		}

		cc_library {
			name: "mylib2",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			compile_multilib: "first",
		}
	`)

	apexRule := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that main rule creates an output
	ensureContains(t, apexRule.Output.String(), "myapex.apex.unsigned")

	// Ensure that apex variant is created for the direct dep
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib"), "android_arm64_armv8-a_shared_apex10000")
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib_common"), "android_arm64_armv8-a_shared_apex10000")
	ensureListNotContains(t, ctx.ModuleVariantsForTests("mylib2"), "android_arm64_armv8-a_shared_apex10000")

	// Ensure that both direct and indirect deps are copied into apex
	ensureContains(t, copyCmds, "image.apex/lib64/mylib.so")
	ensureContains(t, copyCmds, "image.apex/lib64/mylib_common.so")
	ensureNotContains(t, copyCmds, "image.apex/lib64/mylib2.so")

	// Ensure that the platform variant ends with _shared
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib"), "android_arm64_armv8-a_shared")
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib_common"), "android_arm64_armv8-a_shared")
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib2"), "android_arm64_armv8-a_shared")
}

func TestApexWithArch(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			updatable: false,
			native_shared_libs: ["mylib.generic"],
			arch: {
				arm64: {
					native_shared_libs: ["mylib.arm64"],
					exclude_native_shared_libs: ["mylib.generic"],
				},
				x86_64: {
					native_shared_libs: ["mylib.x64"],
					exclude_native_shared_libs: ["mylib.generic"],
				},
			}
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib.generic",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			// TODO: remove //apex_available:platform
			apex_available: [
				"//apex_available:platform",
				"myapex",
			],
		}

		cc_library {
			name: "mylib.arm64",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			// TODO: remove //apex_available:platform
			apex_available: [
				"//apex_available:platform",
				"myapex",
			],
		}

		cc_library {
			name: "mylib.x64",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			// TODO: remove //apex_available:platform
			apex_available: [
				"//apex_available:platform",
				"myapex",
			],
		}
	`)

	apexRule := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that apex variant is created for the direct dep
	ensureListContains(t, ctx.ModuleVariantsForTests("mylib.arm64"), "android_arm64_armv8-a_shared_apex10000")
	ensureListNotContains(t, ctx.ModuleVariantsForTests("mylib.generic"), "android_arm64_armv8-a_shared_apex10000")
	ensureListNotContains(t, ctx.ModuleVariantsForTests("mylib.x64"), "android_arm64_armv8-a_shared_apex10000")

	// Ensure that both direct and indirect deps are copied into apex
	ensureContains(t, copyCmds, "image.apex/lib64/mylib.arm64.so")
	ensureNotContains(t, copyCmds, "image.apex/lib64/mylib.x64.so")
}

func TestApexWithShBinary(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			sh_binaries: ["myscript"],
			updatable: false,
			compile_multilib: "both",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		sh_binary {
			name: "myscript",
			src: "mylib.cpp",
			filename: "myscript.sh",
			sub_dir: "script",
		}
	`)

	apexRule := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	ensureContains(t, copyCmds, "image.apex/bin/script/myscript.sh")
}

func TestApexInVariousPartition(t *testing.T) {
	t.Parallel()
	testcases := []struct {
		propName, partition string
	}{
		{"", "system"},
		{"product_specific: true", "product"},
		{"soc_specific: true", "vendor"},
		{"proprietary: true", "vendor"},
		{"vendor: true", "vendor"},
		{"system_ext_specific: true", "system_ext"},
	}
	for _, tc := range testcases {
		t.Run(tc.propName+":"+tc.partition, func(t *testing.T) {
			t.Parallel()
			ctx := testApex(t, `
				apex {
					name: "myapex",
					key: "myapex.key",
					updatable: false,
					`+tc.propName+`
				}

				apex_key {
					name: "myapex.key",
					public_key: "testkey.avbpubkey",
					private_key: "testkey.pem",
				}
			`)

			apex := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Module().(*apexBundle)
			expected := "out/target/product/test_device/" + tc.partition + "/apex"
			actual := apex.installDir.RelativeToTop().String()
			if actual != expected {
				t.Errorf("wrong install path. expected %q. actual %q", expected, actual)
			}
		})
	}
}

func TestFileContexts_FindInDefaultLocationIfNotSet(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`)
	module := ctx.ModuleForTests(t, "myapex", "android_common_myapex")
	rule := module.Output("file_contexts")
	ensureContains(t, rule.RuleParams.Command, "cat system/sepolicy/apex/myapex-file_contexts")
}

func TestFileContexts_ShouldBeUnderSystemSepolicyForSystemApexes(t *testing.T) {
	t.Parallel()
	testApexError(t, `"myapex" .*: file_contexts: should be under system/sepolicy`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			file_contexts: "my_own_file_contexts",
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`, withFiles(map[string][]byte{
		"my_own_file_contexts": nil,
	}))
}

func TestFileContexts_ProductSpecificApexes(t *testing.T) {
	t.Parallel()
	testApexError(t, `"myapex" .*: file_contexts: cannot find`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			product_specific: true,
			file_contexts: "product_specific_file_contexts",
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`)

	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			product_specific: true,
			file_contexts: "product_specific_file_contexts",
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`, withFiles(map[string][]byte{
		"product_specific_file_contexts": nil,
	}))
	module := ctx.ModuleForTests(t, "myapex", "android_common_myapex")
	rule := module.Output("file_contexts")
	ensureContains(t, rule.RuleParams.Command, "cat product_specific_file_contexts")
}

func TestFileContexts_SetViaFileGroup(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			product_specific: true,
			file_contexts: ":my-file-contexts",
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		filegroup {
			name: "my-file-contexts",
			srcs: ["product_specific_file_contexts"],
		}
	`, withFiles(map[string][]byte{
		"product_specific_file_contexts": nil,
	}))
	module := ctx.ModuleForTests(t, "myapex", "android_common_myapex")
	rule := module.Output("file_contexts")
	ensureContains(t, rule.RuleParams.Command, "cat product_specific_file_contexts")
}

func TestApexKeyFromOtherModule(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex_key {
			name: "myapex.key",
			public_key: ":my.avbpubkey",
			private_key: ":my.pem",
			product_specific: true,
		}

		filegroup {
			name: "my.avbpubkey",
			srcs: ["testkey2.avbpubkey"],
		}

		filegroup {
			name: "my.pem",
			srcs: ["testkey2.pem"],
		}
	`)

	apex_key := ctx.ModuleForTests(t, "myapex.key", "android_common").Module().(*apexKey)
	expected_pubkey := "testkey2.avbpubkey"
	actual_pubkey := apex_key.publicKeyFile.String()
	if actual_pubkey != expected_pubkey {
		t.Errorf("wrong public key path. expected %q. actual %q", expected_pubkey, actual_pubkey)
	}
	expected_privkey := "testkey2.pem"
	actual_privkey := apex_key.privateKeyFile.String()
	if actual_privkey != expected_privkey {
		t.Errorf("wrong private key path. expected %q. actual %q", expected_privkey, actual_privkey)
	}
}

func TestPrebuilt(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		prebuilt_apex {
			name: "myapex",
			arch: {
				arm64: {
					src: "myapex-arm64.apex",
				},
				arm: {
					src: "myapex-arm.apex",
				},
			},
		}
	`)

	testingModule := ctx.ModuleForTests(t, "myapex", "android_common_prebuilt_myapex")
	prebuilt := testingModule.Module().(*Prebuilt)

	expectedInput := "myapex-arm64.apex"
	if prebuilt.inputApex.String() != expectedInput {
		t.Errorf("inputApex invalid. expected: %q, actual: %q", expectedInput, prebuilt.inputApex.String())
	}
	android.AssertStringDoesContain(t, "Invalid provenance metadata file",
		prebuilt.ProvenanceMetaDataFile().String(), "soong/.intermediates/provenance_metadata/myapex/provenance_metadata.textproto")
	rule := testingModule.Rule("genProvenanceMetaData")
	android.AssertStringEquals(t, "Invalid input", "myapex-arm64.apex", rule.Inputs[0].String())
	android.AssertStringEquals(t, "Invalid output", "out/soong/.intermediates/provenance_metadata/myapex/provenance_metadata.textproto", rule.Output.String())
	android.AssertStringEquals(t, "Invalid args", "myapex", rule.Args["module_name"])
	android.AssertStringEquals(t, "Invalid args", "/system/apex/myapex.apex", rule.Args["install_path"])

	entries := android.AndroidMkEntriesForTest(t, ctx, testingModule.Module())[0]
	android.AssertStringEquals(t, "unexpected LOCAL_SOONG_MODULE_TYPE", "prebuilt_apex", entries.EntryMap["LOCAL_SOONG_MODULE_TYPE"][0])
}

func TestPrebuiltMissingSrc(t *testing.T) {
	t.Parallel()
	testApexError(t, `module "myapex" variant "android_common_prebuilt_myapex".*: prebuilt_apex does not support "arm64_armv8-a"`, `
		prebuilt_apex {
			name: "myapex",
		}
	`)
}

func TestPrebuiltFilenameOverride(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		prebuilt_apex {
			name: "myapex",
			src: "myapex-arm.apex",
			filename: "notmyapex.apex",
		}
	`)

	testingModule := ctx.ModuleForTests(t, "myapex", "android_common_prebuilt_myapex")
	p := testingModule.Module().(*Prebuilt)

	expected := "notmyapex.apex"
	if p.installFilename != expected {
		t.Errorf("installFilename invalid. expected: %q, actual: %q", expected, p.installFilename)
	}
	rule := testingModule.Rule("genProvenanceMetaData")
	android.AssertStringEquals(t, "Invalid input", "myapex-arm.apex", rule.Inputs[0].String())
	android.AssertStringEquals(t, "Invalid output", "out/soong/.intermediates/provenance_metadata/myapex/provenance_metadata.textproto", rule.Output.String())
	android.AssertStringEquals(t, "Invalid args", "myapex", rule.Args["module_name"])
	android.AssertStringEquals(t, "Invalid args", "/system/apex/notmyapex.apex", rule.Args["install_path"])
}

func TestApexSetFilenameOverride(t *testing.T) {
	t.Parallel()
	testApex(t, `
		apex_set {
 			name: "com.company.android.myapex",
			apex_name: "com.android.myapex",
			set: "company-myapex.apks",
      filename: "com.company.android.myapex.apex"
		}
	`).ModuleForTests(t, "com.company.android.myapex", "android_common_prebuilt_com.android.myapex")

	testApex(t, `
		apex_set {
 			name: "com.company.android.myapex",
			apex_name: "com.android.myapex",
			set: "company-myapex.apks",
      filename: "com.company.android.myapex.capex"
		}
	`).ModuleForTests(t, "com.company.android.myapex", "android_common_prebuilt_com.android.myapex")

	testApexError(t, `filename should end in .apex or .capex for apex_set`, `
		apex_set {
 			name: "com.company.android.myapex",
			apex_name: "com.android.myapex",
			set: "company-myapex.apks",
      filename: "some-random-suffix"
		}
	`)
}

func TestPrebuiltOverrides(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		prebuilt_apex {
			name: "myapex.prebuilt",
			src: "myapex-arm.apex",
			overrides: [
				"myapex",
			],
		}
	`)

	testingModule := ctx.ModuleForTests(t, "myapex.prebuilt", "android_common_prebuilt_myapex.prebuilt")
	p := testingModule.Module().(*Prebuilt)

	expected := []string{"myapex"}
	actual := android.AndroidMkEntriesForTest(t, ctx, p)[0].EntryMap["LOCAL_OVERRIDES_MODULES"]
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("Incorrect LOCAL_OVERRIDES_MODULES value '%s', expected '%s'", actual, expected)
	}
	rule := testingModule.Rule("genProvenanceMetaData")
	android.AssertStringEquals(t, "Invalid input", "myapex-arm.apex", rule.Inputs[0].String())
	android.AssertStringEquals(t, "Invalid output", "out/soong/.intermediates/provenance_metadata/myapex.prebuilt/provenance_metadata.textproto", rule.Output.String())
	android.AssertStringEquals(t, "Invalid args", "myapex.prebuilt", rule.Args["module_name"])
	android.AssertStringEquals(t, "Invalid args", "/system/apex/myapex.prebuilt.apex", rule.Args["install_path"])
}

func TestPrebuiltApexName(t *testing.T) {
	t.Parallel()
	testApex(t, `
		prebuilt_apex {
			name: "com.company.android.myapex",
			apex_name: "com.android.myapex",
			src: "company-myapex-arm.apex",
		}
	`).ModuleForTests(t, "com.company.android.myapex", "android_common_prebuilt_com.android.myapex")

	testApex(t, `
		apex_set {
			name: "com.company.android.myapex",
			apex_name: "com.android.myapex",
			set: "company-myapex.apks",
		}
	`).ModuleForTests(t, "com.company.android.myapex", "android_common_prebuilt_com.android.myapex")
}

func TestPrebuiltApexNameWithPlatformBootclasspath(t *testing.T) {
	t.Parallel()
	_ = android.GroupFixturePreparers(
		java.PrepareForTestWithJavaDefaultModules,
		PrepareForTestWithApexBuildComponents,
		android.FixtureWithRootAndroidBp(`
			platform_bootclasspath {
				name: "platform-bootclasspath",
				fragments: [
					{
						apex: "com.android.art",
						module: "art-bootclasspath-fragment",
					},
				],
			}

			prebuilt_apex {
				name: "com.company.android.art",
				apex_name: "com.android.art",
				src: "com.company.android.art-arm.apex",
				exported_bootclasspath_fragments: ["art-bootclasspath-fragment"],
			}

			prebuilt_apex {
				name: "com.android.art",
				src: "com.android.art-arm.apex",
				exported_bootclasspath_fragments: ["art-bootclasspath-fragment"],
			}

			prebuilt_bootclasspath_fragment {
				name: "art-bootclasspath-fragment",
				image_name: "art",
				contents: ["core-oj"],
				hidden_api: {
					annotation_flags: "my-bootclasspath-fragment/annotation-flags.csv",
					metadata: "my-bootclasspath-fragment/metadata.csv",
					index: "my-bootclasspath-fragment/index.csv",
					stub_flags: "my-bootclasspath-fragment/stub-flags.csv",
					all_flags: "my-bootclasspath-fragment/all-flags.csv",
				},
			}

			java_import {
				name: "core-oj",
				jars: ["prebuilt.jar"],
			}
		`),
	).RunTest(t)
}

// A minimal context object for use with DexJarBuildPath
type moduleErrorfTestCtx struct {
}

func (ctx moduleErrorfTestCtx) ModuleErrorf(format string, args ...interface{}) {
}

func TestBootDexJarsFromSourcesAndPrebuilts(t *testing.T) {
	t.Parallel()
	preparer := android.GroupFixturePreparers(
		java.FixtureConfigureApexBootJars("myapex:libfoo", "myapex:libbar"),
		// Make sure that the frameworks/base/Android.bp file exists as otherwise hidden API encoding
		// is disabled.
		android.FixtureAddTextFile("frameworks/base/Android.bp", ""),

		// Make sure that we have atleast one platform library so that we can check the monolithic hiddenapi
		// file creation.
		java.FixtureConfigureBootJars("platform:foo"),
		android.FixtureModifyMockFS(func(fs android.MockFS) {
			fs["platform/Android.bp"] = []byte(`
		java_library {
			name: "foo",
			srcs: ["Test.java"],
			compile_dex: true,
		}
		`)
			fs["platform/Test.java"] = nil
		}),
	)

	checkHiddenAPIIndexFromClassesInputs := func(t *testing.T, ctx *android.TestContext, expectedIntermediateInputs string) {
		t.Helper()
		platformBootclasspath := ctx.ModuleForTests(t, "platform-bootclasspath", "android_common")
		var rule android.TestingBuildParams

		rule = platformBootclasspath.Output("hiddenapi-monolithic/index-from-classes.csv")
		java.CheckHiddenAPIRuleInputs(t, "intermediate index", expectedIntermediateInputs, rule)
	}

	checkHiddenAPIIndexFromFlagsInputs := func(t *testing.T, ctx *android.TestContext, expectedIntermediateInputs string) {
		t.Helper()
		platformBootclasspath := ctx.ModuleForTests(t, "platform-bootclasspath", "android_common")
		var rule android.TestingBuildParams

		rule = platformBootclasspath.Output("hiddenapi-index.csv")
		java.CheckHiddenAPIRuleInputs(t, "monolithic index", expectedIntermediateInputs, rule)
	}

	fragment := java.ApexVariantReference{
		Apex:   proptools.StringPtr("myapex"),
		Module: proptools.StringPtr("my-bootclasspath-fragment"),
	}

	t.Run("prebuilt only", func(t *testing.T) {
		t.Parallel()
		bp := `
		prebuilt_apex {
			name: "myapex",
			arch: {
				arm64: {
					src: "myapex-arm64.apex",
				},
				arm: {
					src: "myapex-arm.apex",
				},
			},
			exported_bootclasspath_fragments: ["my-bootclasspath-fragment"],
		}

		prebuilt_bootclasspath_fragment {
			name: "my-bootclasspath-fragment",
			contents: ["libfoo", "libbar"],
			apex_available: ["myapex"],
			hidden_api: {
				annotation_flags: "my-bootclasspath-fragment/annotation-flags.csv",
				metadata: "my-bootclasspath-fragment/metadata.csv",
				index: "my-bootclasspath-fragment/index.csv",
				signature_patterns: "my-bootclasspath-fragment/signature-patterns.csv",
				filtered_stub_flags: "my-bootclasspath-fragment/filtered-stub-flags.csv",
				filtered_flags: "my-bootclasspath-fragment/filtered-flags.csv",
			},
		}

		java_sdk_library_import {
			name: "libfoo",
			public: {
				jars: ["libfoo.jar"],
			},
			apex_available: ["myapex"],
			shared_library: false,
			permitted_packages: ["foo"],
		}

		java_sdk_library_import {
			name: "libbar",
			public: {
				jars: ["libbar.jar"],
			},
			apex_available: ["myapex"],
			shared_library: false,
			permitted_packages: ["bar"],
		}
	`

		ctx := testDexpreoptWithApexes(t, bp, "", preparer, fragment)

		// Verify the correct module jars contribute to the hiddenapi index file.
		checkHiddenAPIIndexFromClassesInputs(t, ctx, `out/soong/.intermediates/platform/foo/android_common/javac/foo.jar`)
		checkHiddenAPIIndexFromFlagsInputs(t, ctx, `
			my-bootclasspath-fragment/index.csv
			out/soong/.intermediates/frameworks/base/boot/platform-bootclasspath/android_common/hiddenapi-monolithic/index-from-classes.csv
			out/soong/.intermediates/packages/modules/com.android.art/art-bootclasspath-fragment/android_common_com.android.art/modular-hiddenapi/index.csv
		`)
	})

	t.Run("apex_set only", func(t *testing.T) {
		t.Parallel()
		bp := `
		apex_set {
			name: "myapex",
			set: "myapex.apks",
			exported_bootclasspath_fragments: ["my-bootclasspath-fragment"],
			exported_systemserverclasspath_fragments: ["my-systemserverclasspath-fragment"],
		}

		prebuilt_bootclasspath_fragment {
			name: "my-bootclasspath-fragment",
			contents: ["libfoo", "libbar"],
			apex_available: ["myapex"],
			hidden_api: {
				annotation_flags: "my-bootclasspath-fragment/annotation-flags.csv",
				metadata: "my-bootclasspath-fragment/metadata.csv",
				index: "my-bootclasspath-fragment/index.csv",
				signature_patterns: "my-bootclasspath-fragment/signature-patterns.csv",
				filtered_stub_flags: "my-bootclasspath-fragment/filtered-stub-flags.csv",
				filtered_flags: "my-bootclasspath-fragment/filtered-flags.csv",
			},
		}

		prebuilt_systemserverclasspath_fragment {
			name: "my-systemserverclasspath-fragment",
			contents: ["libbaz"],
			apex_available: ["myapex"],
		}

		java_sdk_library_import {
			name: "libfoo",
			public: {
				jars: ["libfoo.jar"],
			},
			apex_available: ["myapex"],
			shared_library: false,
			permitted_packages: ["libfoo"],
		}


		java_sdk_library_import {
			name: "libbar",
			public: {
				jars: ["libbar.jar"],
			},
			apex_available: ["myapex"],
			shared_library: false,
			permitted_packages: ["bar"],
		}

		java_sdk_library_import {
			name: "libbaz",
			public: {
				jars: ["libbaz.jar"],
			},
			apex_available: ["myapex"],
			shared_library: false,
			permitted_packages: ["baz"],
		}
	`

		ctx := testDexpreoptWithApexes(t, bp, "", preparer, fragment)

		// Verify the correct module jars contribute to the hiddenapi index file.
		checkHiddenAPIIndexFromClassesInputs(t, ctx, `out/soong/.intermediates/platform/foo/android_common/javac/foo.jar`)
		checkHiddenAPIIndexFromFlagsInputs(t, ctx, `
			my-bootclasspath-fragment/index.csv
			out/soong/.intermediates/frameworks/base/boot/platform-bootclasspath/android_common/hiddenapi-monolithic/index-from-classes.csv
			out/soong/.intermediates/packages/modules/com.android.art/art-bootclasspath-fragment/android_common_com.android.art/modular-hiddenapi/index.csv
		`)

		myApex := ctx.ModuleForTests(t, "myapex", "android_common_prebuilt_myapex").Module()

		overrideNames := []string{
			"",
			"myjavalib.myapex",
			"libfoo.myapex",
			"libbar.myapex",
			"libbaz.myapex",
		}
		mkEntries := android.AndroidMkEntriesForTest(t, ctx, myApex)
		for i, e := range mkEntries {
			g := e.OverrideName
			if w := overrideNames[i]; w != g {
				t.Errorf("Expected override name %q, got %q", w, g)
			}
		}

	})

	t.Run("prebuilt with source library preferred", func(t *testing.T) {
		t.Parallel()
		bp := `
		prebuilt_apex {
			name: "myapex",
			arch: {
				arm64: {
					src: "myapex-arm64.apex",
				},
				arm: {
					src: "myapex-arm.apex",
				},
			},
			exported_bootclasspath_fragments: ["my-bootclasspath-fragment"],
		}

		prebuilt_bootclasspath_fragment {
			name: "my-bootclasspath-fragment",
			contents: ["libfoo", "libbar"],
			apex_available: ["myapex"],
			hidden_api: {
				annotation_flags: "my-bootclasspath-fragment/annotation-flags.csv",
				metadata: "my-bootclasspath-fragment/metadata.csv",
				index: "my-bootclasspath-fragment/index.csv",
				stub_flags: "my-bootclasspath-fragment/stub-flags.csv",
				all_flags: "my-bootclasspath-fragment/all-flags.csv",
			},
		}

		java_import {
			name: "libfoo",
			jars: ["libfoo.jar"],
			apex_available: ["myapex"],
			sdk_version: "core_current",
		}

		java_library {
			name: "libfoo",
			srcs: ["foo/bar/MyClass.java"],
			apex_available: ["myapex"],
			sdk_version: "core_current",
		}

		java_sdk_library_import {
			name: "libbar",
			public: {
				jars: ["libbar.jar"],
			},
			apex_available: ["myapex"],
			shared_library: false,
		}

		java_sdk_library {
			name: "libbar",
			srcs: ["foo/bar/MyClass.java"],
			unsafe_ignore_missing_latest_api: true,
			apex_available: ["myapex"],
		}
	`

		// In this test the source (java_library) libfoo is active since the
		// prebuilt (java_import) defaults to prefer:false. However the
		// prebuilt_apex module always depends on the prebuilt, and so it doesn't
		// find the dex boot jar in it. We either need to disable the source libfoo
		// or make the prebuilt libfoo preferred.
		testDexpreoptWithApexes(t, bp, `module "platform-bootclasspath" variant ".*": module libfoo{.*} does not provide a dex jar`, preparer, fragment)
		// dexbootjar check is skipped if AllowMissingDependencies is true
		preparerAllowMissingDeps := android.GroupFixturePreparers(
			preparer,
			android.PrepareForTestWithAllowMissingDependencies,
		)
		testDexpreoptWithApexes(t, bp, "", preparerAllowMissingDeps, fragment)
	})

	t.Run("prebuilt library preferred with source", func(t *testing.T) {
		t.Parallel()
		bp := `
		apex {
			name: "myapex",
			key: "myapex.key",
			updatable: false,
			bootclasspath_fragments: ["my-bootclasspath-fragment"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		bootclasspath_fragment {
			name: "my-bootclasspath-fragment",
			contents: ["libfoo", "libbar"],
			apex_available: ["myapex"],
			hidden_api: {
				split_packages: ["*"],
			},
		}

		prebuilt_apex {
			name: "myapex",
			prefer: true,
			arch: {
				arm64: {
					src: "myapex-arm64.apex",
				},
				arm: {
					src: "myapex-arm.apex",
				},
			},
			exported_bootclasspath_fragments: ["my-bootclasspath-fragment"],
		}

		prebuilt_bootclasspath_fragment {
			name: "my-bootclasspath-fragment",
			prefer: true,
			contents: ["libfoo", "libbar"],
			apex_available: ["myapex"],
			hidden_api: {
				annotation_flags: "my-bootclasspath-fragment/annotation-flags.csv",
				metadata: "my-bootclasspath-fragment/metadata.csv",
				index: "my-bootclasspath-fragment/index.csv",
				signature_patterns: "my-bootclasspath-fragment/signature-patterns.csv",
				filtered_stub_flags: "my-bootclasspath-fragment/filtered-stub-flags.csv",
				filtered_flags: "my-bootclasspath-fragment/filtered-flags.csv",
			},
		}

		java_sdk_library_import {
			name: "libfoo",
			prefer: true,
			public: {
				jars: ["libfoo.jar"],
			},
			apex_available: ["myapex"],
			shared_library: false,
			permitted_packages: ["libfoo"],
		}

		java_library {
			name: "libfoo",
			srcs: ["foo/bar/MyClass.java"],
			apex_available: ["myapex"],
			installable: true,
			sdk_version: "core_current",
		}

		java_sdk_library_import {
			name: "libbar",
			prefer: true,
			public: {
				jars: ["libbar.jar"],
			},
			apex_available: ["myapex"],
			shared_library: false,
			permitted_packages: ["bar"],
		}

		java_sdk_library {
			name: "libbar",
			srcs: ["foo/bar/MyClass.java"],
			unsafe_ignore_missing_latest_api: true,
			apex_available: ["myapex"],
			compile_dex: true,
		}
	`

		ctx := testDexpreoptWithApexes(t, bp, "", preparer, fragment)

		// Verify the correct module jars contribute to the hiddenapi index file.
		checkHiddenAPIIndexFromClassesInputs(t, ctx, `out/soong/.intermediates/platform/foo/android_common/javac/foo.jar`)
		checkHiddenAPIIndexFromFlagsInputs(t, ctx, `
			my-bootclasspath-fragment/index.csv
			out/soong/.intermediates/frameworks/base/boot/platform-bootclasspath/android_common/hiddenapi-monolithic/index-from-classes.csv
			out/soong/.intermediates/packages/modules/com.android.art/art-bootclasspath-fragment/android_common_com.android.art/modular-hiddenapi/index.csv
		`)
	})

	t.Run("prebuilt with source apex preferred", func(t *testing.T) {
		t.Parallel()
		bp := `
		apex {
			name: "myapex",
			key: "myapex.key",
			updatable: false,
			bootclasspath_fragments: ["my-bootclasspath-fragment"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		bootclasspath_fragment {
			name: "my-bootclasspath-fragment",
			contents: ["libfoo", "libbar"],
			apex_available: ["myapex"],
			hidden_api: {
				split_packages: ["*"],
			},
		}

		prebuilt_apex {
			name: "myapex",
			arch: {
				arm64: {
					src: "myapex-arm64.apex",
				},
				arm: {
					src: "myapex-arm.apex",
				},
			},
			exported_bootclasspath_fragments: ["my-bootclasspath-fragment"],
		}

		prebuilt_bootclasspath_fragment {
			name: "my-bootclasspath-fragment",
			contents: ["libfoo", "libbar"],
			apex_available: ["myapex"],
			hidden_api: {
				annotation_flags: "my-bootclasspath-fragment/annotation-flags.csv",
				metadata: "my-bootclasspath-fragment/metadata.csv",
				index: "my-bootclasspath-fragment/index.csv",
				signature_patterns: "my-bootclasspath-fragment/signature-patterns.csv",
				filtered_stub_flags: "my-bootclasspath-fragment/filtered-stub-flags.csv",
				filtered_flags: "my-bootclasspath-fragment/filtered-flags.csv",
			},
		}

		java_import {
			name: "libfoo",
			jars: ["libfoo.jar"],
			apex_available: ["myapex"],
			sdk_version: "core_current",
		}

		java_library {
			name: "libfoo",
			srcs: ["foo/bar/MyClass.java"],
			apex_available: ["myapex"],
			permitted_packages: ["foo"],
			installable: true,
			sdk_version: "core_current",
		}

		java_sdk_library_import {
			name: "libbar",
			public: {
				jars: ["libbar.jar"],
			},
			apex_available: ["myapex"],
			shared_library: false,
		}

		java_sdk_library {
			name: "libbar",
			srcs: ["foo/bar/MyClass.java"],
			unsafe_ignore_missing_latest_api: true,
			apex_available: ["myapex"],
			permitted_packages: ["bar"],
			compile_dex: true,
			sdk_version: "core_current",
		}
	`

		ctx := testDexpreoptWithApexes(t, bp, "", preparer, fragment)

		// Verify the correct module jars contribute to the hiddenapi index file.
		checkHiddenAPIIndexFromClassesInputs(t, ctx, `out/soong/.intermediates/platform/foo/android_common/javac/foo.jar`)
		checkHiddenAPIIndexFromFlagsInputs(t, ctx, `
			out/soong/.intermediates/frameworks/base/boot/platform-bootclasspath/android_common/hiddenapi-monolithic/index-from-classes.csv
			out/soong/.intermediates/my-bootclasspath-fragment/android_common_myapex/modular-hiddenapi/index.csv
			out/soong/.intermediates/packages/modules/com.android.art/art-bootclasspath-fragment/android_common_com.android.art/modular-hiddenapi/index.csv
		`)
	})

	t.Run("prebuilt preferred with source apex disabled", func(t *testing.T) {
		t.Parallel()
		bp := `
		apex {
			name: "myapex",
			enabled: false,
			key: "myapex.key",
			bootclasspath_fragments: ["my-bootclasspath-fragment"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		bootclasspath_fragment {
			name: "my-bootclasspath-fragment",
			enabled: false,
			contents: ["libfoo", "libbar"],
			apex_available: ["myapex"],
			hidden_api: {
				split_packages: ["*"],
			},
		}

		prebuilt_apex {
			name: "myapex",
			arch: {
				arm64: {
					src: "myapex-arm64.apex",
				},
				arm: {
					src: "myapex-arm.apex",
				},
			},
			exported_bootclasspath_fragments: ["my-bootclasspath-fragment"],
		}

		prebuilt_bootclasspath_fragment {
			name: "my-bootclasspath-fragment",
			contents: ["libfoo", "libbar"],
			apex_available: ["myapex"],
			hidden_api: {
				annotation_flags: "my-bootclasspath-fragment/annotation-flags.csv",
				metadata: "my-bootclasspath-fragment/metadata.csv",
				index: "my-bootclasspath-fragment/index.csv",
				signature_patterns: "my-bootclasspath-fragment/signature-patterns.csv",
				filtered_stub_flags: "my-bootclasspath-fragment/filtered-stub-flags.csv",
				filtered_flags: "my-bootclasspath-fragment/filtered-flags.csv",
			},
		}

		java_import {
			name: "libfoo",
			jars: ["libfoo.jar"],
			apex_available: ["myapex"],
			permitted_packages: ["foo"],
		}

		java_library {
			name: "libfoo",
			enabled: false,
			srcs: ["foo/bar/MyClass.java"],
			apex_available: ["myapex"],
			installable: true,
		}

		java_sdk_library_import {
			name: "libbar",
			public: {
				jars: ["libbar.jar"],
			},
			apex_available: ["myapex"],
			shared_library: false,
			permitted_packages: ["bar"],
			prefer: true,
		}

		java_sdk_library {
			name: "libbar",
			srcs: ["foo/bar/MyClass.java"],
			unsafe_ignore_missing_latest_api: true,
			apex_available: ["myapex"],
			compile_dex: true,
		}
	`
		// This test disables libbar, which causes the ComponentDepsMutator to add
		// deps on libbar.stubs and other sub-modules that don't exist. We can
		// enable AllowMissingDependencies to work around that, but enabling that
		// causes extra checks for missing source files to dex_bootjars, so add those
		// to the mock fs as well.
		preparer2 := android.GroupFixturePreparers(
			preparer,
			android.PrepareForTestWithAllowMissingDependencies,
			android.FixtureMergeMockFs(map[string][]byte{
				"build/soong/scripts/check_boot_jars/package_allowed_list.txt": nil,
				"frameworks/base/boot/boot-profile.txt":                        nil,
			}),
		)

		ctx := testDexpreoptWithApexes(t, bp, "", preparer2, fragment)

		// Verify the correct module jars contribute to the hiddenapi index file.
		checkHiddenAPIIndexFromClassesInputs(t, ctx, `out/soong/.intermediates/platform/foo/android_common/javac/foo.jar`)
		checkHiddenAPIIndexFromFlagsInputs(t, ctx, `
			my-bootclasspath-fragment/index.csv
			out/soong/.intermediates/frameworks/base/boot/platform-bootclasspath/android_common/hiddenapi-monolithic/index-from-classes.csv
			out/soong/.intermediates/packages/modules/com.android.art/art-bootclasspath-fragment/android_common_com.android.art/modular-hiddenapi/index.csv
		`)
	})

	t.Run("Co-existing unflagged apexes should create a duplicate module error", func(t *testing.T) {
		t.Parallel()
		bp := `
		// Source
		apex {
			name: "myapex",
			enabled: false,
			key: "myapex.key",
			bootclasspath_fragments: ["my-bootclasspath-fragment"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		// Prebuilt
		prebuilt_apex {
			name: "myapex.v1",
			source_apex_name: "myapex",
			arch: {
				arm64: {
					src: "myapex-arm64.apex",
				},
				arm: {
					src: "myapex-arm.apex",
				},
			},
			exported_bootclasspath_fragments: ["my-bootclasspath-fragment"],
			prefer: true,
		}
		prebuilt_apex {
			name: "myapex.v2",
			source_apex_name: "myapex",
			arch: {
				arm64: {
					src: "myapex-arm64.apex",
				},
				arm: {
					src: "myapex-arm.apex",
				},
			},
			exported_bootclasspath_fragments: ["my-bootclasspath-fragment"],
			prefer: true,
		}

		prebuilt_bootclasspath_fragment {
			name: "my-bootclasspath-fragment",
			contents: ["libfoo", "libbar"],
			apex_available: ["myapex"],
			hidden_api: {
				annotation_flags: "my-bootclasspath-fragment/annotation-flags.csv",
				metadata: "my-bootclasspath-fragment/metadata.csv",
				index: "my-bootclasspath-fragment/index.csv",
				stub_flags: "my-bootclasspath-fragment/stub-flags.csv",
				all_flags: "my-bootclasspath-fragment/all-flags.csv",
			},
			prefer: true,
		}

		java_import {
			name: "libfoo",
			jars: ["libfoo.jar"],
			apex_available: ["myapex"],
			prefer: true,
		}
		java_import {
			name: "libbar",
			jars: ["libbar.jar"],
			apex_available: ["myapex"],
			prefer: true,
		}
	`

		testDexpreoptWithApexes(t, bp, "Multiple prebuilt modules prebuilt_myapex.v1 and prebuilt_myapex.v2 have been marked as preferred for this source module", preparer, fragment)
	})

}

func TestApexWithTests(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex_test {
			name: "myapex",
			key: "myapex.key",
			updatable: false,
			tests: [
				"mytest",
			],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		filegroup {
			name: "fg",
			srcs: [
				"baz",
				"bar/baz"
			],
		}

		cc_test {
			name: "mytest",
			gtest: false,
			srcs: ["mytest.cpp"],
			relative_install_path: "test",
			shared_libs: ["mylib"],
			system_shared_libs: [],
			stl: "none",
			data: [":fg"],
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
		}

		filegroup {
			name: "fg2",
			srcs: [
				"testdata/baz"
			],
		}
	`)

	apexRule := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that test dep (and their transitive dependencies) are copied into apex.
	ensureContains(t, copyCmds, "image.apex/bin/test/mytest")
	ensureContains(t, copyCmds, "image.apex/lib64/mylib.so")

	//Ensure that test data are copied into apex.
	ensureContains(t, copyCmds, "image.apex/bin/test/baz")
	ensureContains(t, copyCmds, "image.apex/bin/test/bar/baz")

	// Ensure the module is correctly translated.
	bundle := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Module().(*apexBundle)
	data := android.AndroidMkDataForTest(t, ctx, bundle)
	name := bundle.BaseModuleName()
	prefix := "TARGET_"
	var builder strings.Builder
	data.Custom(&builder, name, prefix, "", data)
	androidMk := builder.String()
	ensureContains(t, androidMk, "LOCAL_MODULE := mytest.myapex\n")
	ensureContains(t, androidMk, "LOCAL_MODULE := myapex\n")
}

func TestErrorsIfDepsAreNotEnabled(t *testing.T) {
	t.Parallel()
	testApexError(t, `module "myapex" .* depends on disabled module "libfoo"`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["libfoo"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "libfoo",
			stl: "none",
			system_shared_libs: [],
			enabled: false,
			apex_available: ["myapex"],
		}
	`)
	testApexError(t, `module "myapex" .* depends on disabled module "myjar"`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			java_libs: ["myjar"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_library {
			name: "myjar",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			enabled: false,
			apex_available: ["myapex"],
			compile_dex: true,
		}
	`)
}

func TestApexWithJavaImport(t *testing.T) {
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			java_libs: ["myjavaimport"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_import {
			name: "myjavaimport",
			apex_available: ["myapex"],
			jars: ["my.jar"],
			compile_dex: true,
		}
	`)

	module := ctx.ModuleForTests(t, "myapex", "android_common_myapex")
	apexRule := module.Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]
	ensureContains(t, copyCmds, "image.apex/javalib/myjavaimport.jar")
}

func TestApexWithApps(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			apps: [
				"AppFoo",
				"AppFooPriv",
			],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		android_app {
			name: "AppFoo",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "current",
			system_modules: "none",
			use_embedded_native_libs: true,
			jni_libs: ["libjni"],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		android_app {
			name: "AppFooPriv",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "current",
			system_modules: "none",
			privileged: true,
			privapp_allowlist: "privapp_allowlist_com.android.AppFooPriv.xml",
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_library_shared {
			name: "libjni",
			srcs: ["mylib.cpp"],
			shared_libs: ["libfoo"],
			stl: "none",
			system_shared_libs: [],
			apex_available: [ "myapex" ],
			sdk_version: "current",
		}

		cc_library_shared {
			name: "libfoo",
			stl: "none",
			system_shared_libs: [],
			apex_available: [ "myapex" ],
			sdk_version: "current",
		}
	`)

	module := ctx.ModuleForTests(t, "myapex", "android_common_myapex")
	apexRule := module.Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	ensureContains(t, copyCmds, "image.apex/app/AppFoo@TEST.BUILD_ID/AppFoo.apk")
	ensureContains(t, copyCmds, "image.apex/priv-app/AppFooPriv@TEST.BUILD_ID/AppFooPriv.apk")
	ensureContains(t, copyCmds, "image.apex/etc/permissions/privapp_allowlist_com.android.AppFooPriv.xml")

	appZipRule := ctx.ModuleForTests(t, "AppFoo", "android_common_apex10000").Description("zip jni libs")
	// JNI libraries are uncompressed
	if args := appZipRule.Args["jarArgs"]; !strings.Contains(args, "-L 0") {
		t.Errorf("jni libs are not uncompressed for AppFoo")
	}
	// JNI libraries including transitive deps are
	for _, jni := range []string{"libjni", "libfoo"} {
		jniOutput := ctx.ModuleForTests(t, jni, "android_arm64_armv8-a_sdk_shared_apex10000").Module().(*cc.Module).OutputFile().RelativeToTop()
		// ... embedded inside APK (jnilibs.zip)
		ensureListContains(t, appZipRule.Implicits.Strings(), jniOutput.String())
		// ... and not directly inside the APEX
		ensureNotContains(t, copyCmds, "image.apex/lib64/"+jni+".so")
	}

	apexBundle := module.Module().(*apexBundle)
	data := android.AndroidMkDataForTest(t, ctx, apexBundle)
	var builder strings.Builder
	data.Custom(&builder, apexBundle.Name(), "TARGET_", "", data)
	androidMk := builder.String()
	ensureContains(t, androidMk, "LOCAL_MODULE := AppFooPriv.myapex")
	ensureContains(t, androidMk, "LOCAL_MODULE := AppFoo.myapex")
	ensureMatches(t, androidMk, "LOCAL_SOONG_INSTALLED_MODULE := \\S+AppFooPriv.apk")
	ensureMatches(t, androidMk, "LOCAL_SOONG_INSTALLED_MODULE := \\S+AppFoo.apk")
	ensureMatches(t, androidMk, "LOCAL_SOONG_INSTALL_PAIRS := \\S+AppFooPriv.apk")
	ensureContains(t, androidMk, "LOCAL_SOONG_INSTALL_PAIRS := privapp_allowlist_com.android.AppFooPriv.xml:$(PRODUCT_OUT)/apex/myapex/etc/permissions/privapp_allowlist_com.android.AppFooPriv.xml")
}

func TestApexWithAppImportBuildId(t *testing.T) {
	t.Parallel()
	invalidBuildIds := []string{"../", "a b", "a/b", "a/b/../c", "/a"}
	for _, id := range invalidBuildIds {
		message := fmt.Sprintf("Unable to use build id %s as filename suffix", id)
		fixture := android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.BuildId = proptools.StringPtr(id)
		})
		testApexError(t, message, `apex {
			name: "myapex",
			key: "myapex.key",
			apps: ["AppFooPrebuilt"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		android_app_import {
			name: "AppFooPrebuilt",
			apk: "PrebuiltAppFoo.apk",
			presigned: true,
			apex_available: ["myapex"],
		}
	`, fixture)
	}
}

func TestApexWithAppImports(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			apps: [
				"AppFooPrebuilt",
				"AppFooPrivPrebuilt",
			],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		android_app_import {
			name: "AppFooPrebuilt",
			apk: "PrebuiltAppFoo.apk",
			presigned: true,
			dex_preopt: {
				enabled: false,
			},
			apex_available: ["myapex"],
		}

		android_app_import {
			name: "AppFooPrivPrebuilt",
			apk: "PrebuiltAppFooPriv.apk",
			privileged: true,
			presigned: true,
			dex_preopt: {
				enabled: false,
			},
			filename: "AwesomePrebuiltAppFooPriv.apk",
			apex_available: ["myapex"],
		}
	`)

	module := ctx.ModuleForTests(t, "myapex", "android_common_myapex")
	apexRule := module.Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	ensureContains(t, copyCmds, "image.apex/app/AppFooPrebuilt@TEST.BUILD_ID/AppFooPrebuilt.apk")
	ensureContains(t, copyCmds, "image.apex/priv-app/AppFooPrivPrebuilt@TEST.BUILD_ID/AwesomePrebuiltAppFooPriv.apk")
}

func TestApexWithAppImportsPrefer(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			apps: [
				"AppFoo",
			],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		android_app {
			name: "AppFoo",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			apex_available: [ "myapex" ],
		}

		android_app_import {
			name: "AppFoo",
			apk: "AppFooPrebuilt.apk",
			filename: "AppFooPrebuilt.apk",
			presigned: true,
			prefer: true,
			apex_available: ["myapex"],
		}
	`, withFiles(map[string][]byte{
		"AppFooPrebuilt.apk": nil,
	}))

	ensureExactContents(t, ctx, "myapex", "android_common_myapex", []string{
		"app/AppFoo@TEST.BUILD_ID/AppFooPrebuilt.apk",
	})
}

func TestApexWithTestHelperApp(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			apps: [
				"TesterHelpAppFoo",
			],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		android_test_helper_app {
			name: "TesterHelpAppFoo",
			srcs: ["foo/bar/MyClass.java"],
			apex_available: [ "myapex" ],
			sdk_version: "test_current",
		}

	`)

	module := ctx.ModuleForTests(t, "myapex", "android_common_myapex")
	apexRule := module.Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	ensureContains(t, copyCmds, "image.apex/app/TesterHelpAppFoo@TEST.BUILD_ID/TesterHelpAppFoo.apk")
}

func TestApexPropertiesShouldBeDefaultable(t *testing.T) {
	t.Parallel()
	// libfoo's apex_available comes from cc_defaults
	testApexError(t, `requires "libfoo" that doesn't list the APEX under 'apex_available'.`, `
	apex {
		name: "myapex",
		key: "myapex.key",
		native_shared_libs: ["libfoo"],
		updatable: false,
	}

	apex_key {
		name: "myapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}

	apex {
		name: "otherapex",
		key: "myapex.key",
		native_shared_libs: ["libfoo"],
		updatable: false,
	}

	cc_defaults {
		name: "libfoo-defaults",
		apex_available: ["otherapex"],
	}

	cc_library {
		name: "libfoo",
		defaults: ["libfoo-defaults"],
		stl: "none",
		system_shared_libs: [],
	}`)
}

func TestApexAvailable_DirectDep(t *testing.T) {
	t.Parallel()
	// libfoo is not available to myapex, but only to otherapex
	testApexError(t, "requires \"libfoo\" that doesn't list the APEX under 'apex_available'.", `
	apex {
		name: "myapex",
		key: "myapex.key",
		native_shared_libs: ["libfoo"],
		updatable: false,
	}

	apex_key {
		name: "myapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}

	apex {
		name: "otherapex",
		key: "otherapex.key",
		native_shared_libs: ["libfoo"],
		updatable: false,
	}

	apex_key {
		name: "otherapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}

	cc_library {
		name: "libfoo",
		stl: "none",
		system_shared_libs: [],
		apex_available: ["otherapex"],
	}`)

	// 'apex_available' check is bypassed for /product apex with a specific prefix.
	// TODO: b/352818241 - Remove below two cases after APEX availability is enforced for /product APEXes.
	testApex(t, `
	apex {
		name: "com.sdv.myapex",
		key: "myapex.key",
		native_shared_libs: ["libfoo"],
		updatable: false,
		product_specific: true,
	}

	apex_key {
		name: "myapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}

	apex {
		name: "com.any.otherapex",
		key: "otherapex.key",
		native_shared_libs: ["libfoo"],
		updatable: false,
	}

	apex_key {
		name: "otherapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}

	cc_library {
		name: "libfoo",
		stl: "none",
		system_shared_libs: [],
		apex_available: ["com.any.otherapex"],
		product_specific: true,
	}`,
		android.FixtureMergeMockFs(android.MockFS{
			"system/sepolicy/apex/com.sdv.myapex-file_contexts":    nil,
			"system/sepolicy/apex/com.any.otherapex-file_contexts": nil,
		}))

	// 'apex_available' check is not bypassed for non-product apex with a specific prefix.
	testApexError(t, "requires \"libfoo\" that doesn't list the APEX under 'apex_available'.", `
	apex {
		name: "com.sdv.myapex",
		key: "myapex.key",
		native_shared_libs: ["libfoo"],
		updatable: false,
	}

	apex_key {
		name: "myapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}

	apex {
		name: "com.any.otherapex",
		key: "otherapex.key",
		native_shared_libs: ["libfoo"],
		updatable: false,
	}

	apex_key {
		name: "otherapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}

	cc_library {
		name: "libfoo",
		stl: "none",
		system_shared_libs: [],
		apex_available: ["com.any.otherapex"],
	}`,
		android.FixtureMergeMockFs(android.MockFS{
			"system/sepolicy/apex/com.sdv.myapex-file_contexts":    nil,
			"system/sepolicy/apex/com.any.otherapex-file_contexts": nil,
		}))
}

func TestApexAvailable_IndirectDep(t *testing.T) {
	t.Parallel()
	// libbbaz is an indirect dep
	testApexError(t, `requires "libbaz" that doesn't list the APEX under 'apex_available'.\n\nDependency path:
.*via tag apex\.dependencyTag\{"sharedLib"\}
.*-> libfoo.*link:shared.*
.*via tag cc\.libraryDependencyTag.*Kind:sharedLibraryDependency.*
.*-> libbar.*link:shared.*
.*via tag cc\.libraryDependencyTag.*Kind:sharedLibraryDependency.*
.*-> libbaz.*link:shared.*`, `
	apex {
		name: "myapex",
		key: "myapex.key",
		native_shared_libs: ["libfoo"],
		updatable: false,
	}

	apex_key {
		name: "myapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}

	cc_library {
		name: "libfoo",
		stl: "none",
		shared_libs: ["libbar"],
		system_shared_libs: [],
		apex_available: ["myapex"],
	}

	cc_library {
		name: "libbar",
		stl: "none",
		shared_libs: ["libbaz"],
		system_shared_libs: [],
		apex_available: ["myapex"],
	}

	cc_library {
		name: "libbaz",
		stl: "none",
		system_shared_libs: [],
	}`)

	// 'apex_available' check is bypassed for /product apex with a specific prefix.
	// TODO: b/352818241 - Remove below two cases after APEX availability is enforced for /product APEXes.
	testApex(t, `
		apex {
			name: "com.sdv.myapex",
			key: "myapex.key",
			native_shared_libs: ["libfoo"],
			updatable: false,
			product_specific: true,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "libfoo",
			stl: "none",
			shared_libs: ["libbar"],
			system_shared_libs: [],
			apex_available: ["com.sdv.myapex"],
			product_specific: true,
		}

		cc_library {
			name: "libbar",
			stl: "none",
			shared_libs: ["libbaz"],
			system_shared_libs: [],
			apex_available: ["com.sdv.myapex"],
			product_specific: true,
		}

		cc_library {
			name: "libbaz",
			stl: "none",
			system_shared_libs: [],
			product_specific: true,
		}`,
		android.FixtureMergeMockFs(android.MockFS{
			"system/sepolicy/apex/com.sdv.myapex-file_contexts": nil,
		}))

	// 'apex_available' check is not bypassed for non-product apex with a specific prefix.
	testApexError(t, `requires "libbaz" that doesn't list the APEX under 'apex_available'.`, `
		apex {
			name: "com.sdv.myapex",
			key: "myapex.key",
			native_shared_libs: ["libfoo"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "libfoo",
			stl: "none",
			shared_libs: ["libbar"],
			system_shared_libs: [],
			apex_available: ["com.sdv.myapex"],
		}

		cc_library {
			name: "libbar",
			stl: "none",
			shared_libs: ["libbaz"],
			system_shared_libs: [],
			apex_available: ["com.sdv.myapex"],
		}

		cc_library {
			name: "libbaz",
			stl: "none",
			system_shared_libs: [],
		}`,
		android.FixtureMergeMockFs(android.MockFS{
			"system/sepolicy/apex/com.sdv.myapex-file_contexts": nil,
		}))
}

func TestApexAvailable_IndirectStaticDep(t *testing.T) {
	t.Parallel()
	testApex(t, `
	apex {
		name: "myapex",
		key: "myapex.key",
		native_shared_libs: ["libfoo"],
		updatable: false,
	}

	apex_key {
		name: "myapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}

	cc_library {
		name: "libfoo",
		stl: "none",
		static_libs: ["libbar"],
		system_shared_libs: [],
		apex_available: ["myapex"],
	}

	cc_library {
		name: "libbar",
		stl: "none",
		shared_libs: ["libbaz"],
		system_shared_libs: [],
		apex_available: ["myapex"],
	}

	cc_library {
		name: "libbaz",
		stl: "none",
		system_shared_libs: [],
	}`)

	testApexError(t, `requires "libbar" that doesn't list the APEX under 'apex_available'.`, `
	apex {
		name: "myapex",
		key: "myapex.key",
		native_shared_libs: ["libfoo"],
		updatable: false,
	}

	apex_key {
		name: "myapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}

	cc_library {
		name: "libfoo",
		stl: "none",
		static_libs: ["libbar"],
		system_shared_libs: [],
		apex_available: ["myapex"],
	}

	cc_library {
		name: "libbar",
		stl: "none",
		system_shared_libs: [],
	}`)
}

func TestApexAvailable_InvalidApexName(t *testing.T) {
	t.Parallel()
	testApexError(t, "\"otherapex\" is not a valid module name", `
	apex {
		name: "myapex",
		key: "myapex.key",
		native_shared_libs: ["libfoo"],
		updatable: false,
	}

	apex_key {
		name: "myapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}

	cc_library {
		name: "libfoo",
		stl: "none",
		system_shared_libs: [],
		apex_available: ["otherapex"],
	}`)

	testApex(t, `
	apex {
		name: "myapex",
		key: "myapex.key",
		native_shared_libs: ["libfoo", "libbar"],
		updatable: false,
	}

	apex_key {
		name: "myapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}

	cc_library {
		name: "libfoo",
		stl: "none",
		system_shared_libs: [],
		runtime_libs: ["libbaz"],
		apex_available: ["myapex"],
	}

	cc_library {
		name: "libbar",
		stl: "none",
		system_shared_libs: [],
		apex_available: ["//apex_available:anyapex"],
	}

	cc_library {
		name: "libbaz",
		stl: "none",
		system_shared_libs: [],
		stubs: {
			versions: ["10", "20", "30"],
		},
	}`)
}

func TestApexAvailable_ApexAvailableNameWithVersionCodeError(t *testing.T) {
	t.Parallel()
	t.Run("negative variant_version produces error", func(t *testing.T) {
		t.Parallel()
		testApexError(t, "expected an integer between 0-9; got -1", `
			apex {
				name: "myapex",
				key: "myapex.key",
				apex_available_name: "com.android.foo",
				variant_version: "-1",
				updatable: false,
			}
			apex_key {
				name: "myapex.key",
				public_key: "testkey.avbpubkey",
				private_key: "testkey.pem",
			}
		`)
	})

	t.Run("variant_version greater than 9 produces error", func(t *testing.T) {
		t.Parallel()
		testApexError(t, "expected an integer between 0-9; got 10", `
			apex {
				name: "myapex",
				key: "myapex.key",
				apex_available_name: "com.android.foo",
				variant_version: "10",
				updatable: false,
			}
			apex_key {
				name: "myapex.key",
				public_key: "testkey.avbpubkey",
				private_key: "testkey.pem",
			}
		`)
	})
}

func TestApexAvailable_ApexAvailableNameWithVersionCode(t *testing.T) {
	t.Parallel()
	context := android.GroupFixturePreparers(
		android.PrepareForIntegrationTestWithAndroid,
		PrepareForTestWithApexBuildComponents,
		android.FixtureMergeMockFs(android.MockFS{
			"system/sepolicy/apex/foo-file_contexts": nil,
			"system/sepolicy/apex/bar-file_contexts": nil,
		}),
	)
	result := context.RunTestWithBp(t, `
		apex {
			name: "foo",
			key: "myapex.key",
			apex_available_name: "com.android.foo",
			variant_version: "0",
			updatable: false,
		}
		apex {
			name: "bar",
			key: "myapex.key",
			apex_available_name: "com.android.foo",
			variant_version: "3",
			updatable: false,
		}
		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
		override_apex {
			name: "myoverrideapex",
			base: "bar",
		}
	`)

	fooManifestRule := result.ModuleForTests(t, "foo", "android_common_foo").Rule("apexManifestRule")
	fooExpectedDefaultVersion := testDefaultUpdatableModuleVersion
	fooActualDefaultVersion := fooManifestRule.Args["default_version"]
	if fooActualDefaultVersion != fooExpectedDefaultVersion {
		t.Errorf("expected to find defaultVersion %q; got %q", fooExpectedDefaultVersion, fooActualDefaultVersion)
	}

	barManifestRule := result.ModuleForTests(t, "bar", "android_common_bar").Rule("apexManifestRule")
	defaultVersionInt, _ := strconv.Atoi(testDefaultUpdatableModuleVersion)
	barExpectedDefaultVersion := fmt.Sprint(defaultVersionInt + 3)
	barActualDefaultVersion := barManifestRule.Args["default_version"]
	if barActualDefaultVersion != barExpectedDefaultVersion {
		t.Errorf("expected to find defaultVersion %q; got %q", barExpectedDefaultVersion, barActualDefaultVersion)
	}

	overrideBarManifestRule := result.ModuleForTests(t, "bar", "android_common_myoverrideapex_myoverrideapex").Rule("apexManifestRule")
	overrideBarActualDefaultVersion := overrideBarManifestRule.Args["default_version"]
	if overrideBarActualDefaultVersion != barExpectedDefaultVersion {
		t.Errorf("expected to find defaultVersion %q; got %q", barExpectedDefaultVersion, barActualDefaultVersion)
	}
}

func TestApexAvailable_ApexAvailableName(t *testing.T) {
	t.Parallel()
	t.Run("using name of apex that sets apex_available_name is not allowed", func(t *testing.T) {
		t.Parallel()
		testApexError(t, "Consider adding \"myapex\" to 'apex_available' property of \"AppFoo\"", `
			apex {
				name: "myapex_sminus",
				key: "myapex.key",
				apps: ["AppFoo"],
				apex_available_name: "myapex",
				updatable: false,
			}
			apex {
				name: "myapex",
				key: "myapex.key",
				apps: ["AppFoo"],
				updatable: false,
			}
			apex_key {
				name: "myapex.key",
				public_key: "testkey.avbpubkey",
				private_key: "testkey.pem",
			}
			android_app {
				name: "AppFoo",
				srcs: ["foo/bar/MyClass.java"],
				sdk_version: "none",
				system_modules: "none",
				apex_available: [ "myapex_sminus" ],
			}`,
			android.FixtureMergeMockFs(android.MockFS{
				"system/sepolicy/apex/myapex_sminus-file_contexts": nil,
			}),
		)
	})

	t.Run("apex_available_name allows module to be used in two different apexes", func(t *testing.T) {
		t.Parallel()
		testApex(t, `
			apex {
				name: "myapex_sminus",
				key: "myapex.key",
				apps: ["AppFoo"],
				apex_available_name: "myapex",
				updatable: false,
			}
			apex {
				name: "myapex",
				key: "myapex.key",
				apps: ["AppFoo"],
				updatable: false,
			}
			apex_key {
				name: "myapex.key",
				public_key: "testkey.avbpubkey",
				private_key: "testkey.pem",
			}
			android_app {
				name: "AppFoo",
				srcs: ["foo/bar/MyClass.java"],
				sdk_version: "none",
				system_modules: "none",
				apex_available: [ "myapex" ],
			}`,
			android.FixtureMergeMockFs(android.MockFS{
				"system/sepolicy/apex/myapex_sminus-file_contexts": nil,
			}),
		)
	})

	t.Run("override_apexes work with apex_available_name", func(t *testing.T) {
		t.Parallel()
		testApex(t, `
			override_apex {
				name: "myoverrideapex_sminus",
				base: "myapex_sminus",
				key: "myapex.key",
				apps: ["AppFooOverride"],
			}
			override_apex {
				name: "myoverrideapex",
				base: "myapex",
				key: "myapex.key",
				apps: ["AppFooOverride"],
			}
			apex {
				name: "myapex_sminus",
				key: "myapex.key",
				apps: ["AppFoo"],
				apex_available_name: "myapex",
				updatable: false,
			}
			apex {
				name: "myapex",
				key: "myapex.key",
				apps: ["AppFoo"],
				updatable: false,
			}
			apex_key {
				name: "myapex.key",
				public_key: "testkey.avbpubkey",
				private_key: "testkey.pem",
			}
			android_app {
				name: "AppFooOverride",
				srcs: ["foo/bar/MyClass.java"],
				sdk_version: "none",
				system_modules: "none",
				apex_available: [ "myapex" ],
			}
			android_app {
				name: "AppFoo",
				srcs: ["foo/bar/MyClass.java"],
				sdk_version: "none",
				system_modules: "none",
				apex_available: [ "myapex" ],
			}`,
			android.FixtureMergeMockFs(android.MockFS{
				"system/sepolicy/apex/myapex_sminus-file_contexts": nil,
			}),
		)
	})
}

func TestApexAvailable_ApexAvailableNameWithOverrides(t *testing.T) {
	t.Parallel()
	context := android.GroupFixturePreparers(
		android.PrepareForIntegrationTestWithAndroid,
		PrepareForTestWithApexBuildComponents,
		java.PrepareForTestWithDexpreopt,
		android.FixtureMergeMockFs(android.MockFS{
			"system/sepolicy/apex/myapex-file_contexts":        nil,
			"system/sepolicy/apex/myapex_sminus-file_contexts": nil,
		}),
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.BuildId = proptools.StringPtr("buildid")
		}),
	)
	context.RunTestWithBp(t, `
	override_apex {
		name: "myoverrideapex_sminus",
		base: "myapex_sminus",
	}
	override_apex {
		name: "myoverrideapex",
		base: "myapex",
	}
	apex {
		name: "myapex",
		key: "myapex.key",
		apps: ["AppFoo"],
		updatable: false,
	}
	apex {
		name: "myapex_sminus",
		apex_available_name: "myapex",
		key: "myapex.key",
		apps: ["AppFoo_sminus"],
		updatable: false,
	}
	apex_key {
		name: "myapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}
	android_app {
		name: "AppFoo",
		srcs: ["foo/bar/MyClass.java"],
		sdk_version: "none",
		system_modules: "none",
		apex_available: [ "myapex" ],
	}
	android_app {
		name: "AppFoo_sminus",
		srcs: ["foo/bar/MyClass.java"],
		sdk_version: "none",
		min_sdk_version: "29",
		system_modules: "none",
		apex_available: [ "myapex" ],
	}`)
}

func TestApexAvailable_CheckForPlatform(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
	apex {
		name: "myapex",
		key: "myapex.key",
		native_shared_libs: ["libbar", "libbaz"],
		updatable: false,
	}

	apex_key {
		name: "myapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}

	cc_library {
		name: "libfoo",
		stl: "none",
		system_shared_libs: [],
		shared_libs: ["libbar"],
		apex_available: ["//apex_available:platform"],
	}

	cc_library {
		name: "libfoo2",
		stl: "none",
		system_shared_libs: [],
		shared_libs: ["libbaz"],
		apex_available: ["//apex_available:platform"],
	}

	cc_library {
		name: "libbar",
		stl: "none",
		system_shared_libs: [],
		apex_available: ["myapex"],
	}

	cc_library {
		name: "libbaz",
		stl: "none",
		system_shared_libs: [],
		apex_available: ["myapex"],
		stubs: {
			versions: ["1"],
		},
	}`)

	// libfoo shouldn't be available to platform even though it has "//apex_available:platform",
	// because it depends on libbar which isn't available to platform
	libfoo := ctx.ModuleForTests(t, "libfoo", "android_arm64_armv8-a_shared").Module().(*cc.Module)
	if libfoo.NotAvailableForPlatform() != true {
		t.Errorf("%q shouldn't be available to platform", libfoo.String())
	}

	// libfoo2 however can be available to platform because it depends on libbaz which provides
	// stubs
	libfoo2 := ctx.ModuleForTests(t, "libfoo2", "android_arm64_armv8-a_shared").Module().(*cc.Module)
	if libfoo2.NotAvailableForPlatform() == true {
		t.Errorf("%q should be available to platform", libfoo2.String())
	}
}

func TestApexAvailable_CreatedForApex(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
	apex {
		name: "myapex",
		key: "myapex.key",
		native_shared_libs: ["libfoo"],
		updatable: false,
	}

	apex_key {
		name: "myapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}

	cc_library {
		name: "libfoo",
		stl: "none",
		system_shared_libs: [],
		apex_available: ["myapex"],
		static: {
			apex_available: ["//apex_available:platform"],
		},
	}`)

	libfooShared := ctx.ModuleForTests(t, "libfoo", "android_arm64_armv8-a_shared").Module().(*cc.Module)
	if libfooShared.NotAvailableForPlatform() != true {
		t.Errorf("%q shouldn't be available to platform", libfooShared.String())
	}
	libfooStatic := ctx.ModuleForTests(t, "libfoo", "android_arm64_armv8-a_static").Module().(*cc.Module)
	if libfooStatic.NotAvailableForPlatform() != false {
		t.Errorf("%q should be available to platform", libfooStatic.String())
	}
}

func TestApexAvailable_PrefixMatch(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		apexAvailable string
		expectedError string
	}{
		{
			name:          "prefix matches correctly",
			apexAvailable: "com.foo.*",
		},
		{
			name:          "prefix doesn't match",
			apexAvailable: "com.bar.*",
			expectedError: `Consider .* "com.foo\.\*"`,
		},
		{
			name:          "short prefix",
			apexAvailable: "com.*",
			expectedError: "requires two or more components",
		},
		{
			name:          "wildcard not in the end",
			apexAvailable: "com.*.foo",
			expectedError: "should end with .*",
		},
		{
			name:          "wildcard in the middle",
			apexAvailable: "com.foo*.*",
			expectedError: "not allowed in the middle",
		},
		{
			name:          "hint with prefix pattern",
			apexAvailable: "//apex_available:platform",
			expectedError: "Consider adding \"com.foo.bar\" or \"com.foo.*\"",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			errorHandler := android.FixtureExpectsNoErrors
			if tc.expectedError != "" {
				errorHandler = android.FixtureExpectsAtLeastOneErrorMatchingPattern(tc.expectedError)
			}
			context := android.GroupFixturePreparers(
				prepareForApexTest,
				android.FixtureMergeMockFs(android.MockFS{
					"system/sepolicy/apex/com.foo.bar-file_contexts": nil,
				}),
			).ExtendWithErrorHandler(errorHandler)

			context.RunTestWithBp(t, `
				apex {
					name: "com.foo.bar",
					key: "myapex.key",
					native_shared_libs: ["libfoo"],
					updatable: false,
				}

				apex_key {
					name: "myapex.key",
					public_key: "testkey.avbpubkey",
					private_key: "testkey.pem",
				}

				cc_library {
					name: "libfoo",
					stl: "none",
					system_shared_libs: [],
					apex_available: ["`+tc.apexAvailable+`"],
				}`)
		})
	}
	testApexError(t, `Consider adding "com.foo" to`, `
		apex {
			name: "com.foo", // too short for a partner apex
			key: "myapex.key",
			native_shared_libs: ["libfoo"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "libfoo",
			stl: "none",
			system_shared_libs: [],
		}
	`)
}

func TestApexValidation_UsesProperPartitionTag(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			updatable: false,
			vendor: true,
		}
		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`, android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
		// vendor path should not affect "partition tag"
		variables.VendorPath = proptools.StringPtr("system/vendor")
	}))

	module := ctx.ModuleForTests(t, "myapex", "android_common_myapex")
	android.AssertStringEquals(t, "partition tag for host_apex_verifier",
		"vendor",
		module.Output("host_apex_verifier.timestamp").Args["partition_tag"])
	android.AssertStringEquals(t, "partition tag for apex_sepolicy_tests",
		"vendor",
		module.Output("apex_sepolicy_tests.timestamp").Args["partition_tag"])
}

func TestApexValidation_TestApexCanSkipInitRcCheck(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex_test {
			name: "myapex",
			key: "myapex.key",
			skip_validations: {
				host_apex_verifier: true,
			},
			updatable: false,
		}
		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`)

	validations := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Rule("signapk").Validations.Strings()
	if android.SuffixInList(validations, "host_apex_verifier.timestamp") {
		t.Error("should not run host_apex_verifier")
	}
}

func TestApexValidation_TestApexCheckInitRc(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex_test {
			name: "myapex",
			key: "myapex.key",
			updatable: false,
		}
		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`)

	validations := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Rule("signapk").Validations.Strings()
	if !android.SuffixInList(validations, "host_apex_verifier.timestamp") {
		t.Error("should run host_apex_verifier")
	}
}

func TestOverrideApex(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			apps: ["app"],
			bpfs: ["bpf"],
			prebuilts: ["myetc"],
			overrides: ["oldapex"],
			updatable: false,
		}

		override_apex {
			name: "override_myapex",
			base: "myapex",
			apps: ["override_app"],
			bpfs: ["overrideBpf"],
			prebuilts: ["override_myetc"],
			overrides: ["unknownapex"],
			compile_multilib: "first",
			multilib: {
				lib32: {
					native_shared_libs: ["mylib32"],
				},
				lib64: {
					native_shared_libs: ["mylib64"],
				},
			},
			logging_parent: "com.foo.bar",
			package_name: "test.overridden.package",
			key: "mynewapex.key",
			certificate: ":myapex.certificate",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		apex_key {
			name: "mynewapex.key",
			public_key: "testkey2.avbpubkey",
			private_key: "testkey2.pem",
		}

		android_app_certificate {
			name: "myapex.certificate",
			certificate: "testkey",
		}

		android_app {
			name: "app",
			srcs: ["foo/bar/MyClass.java"],
			package_name: "foo",
			sdk_version: "none",
			system_modules: "none",
			apex_available: [ "myapex" ],
		}

		override_android_app {
			name: "override_app",
			base: "app",
			package_name: "bar",
		}

		bpf {
			name: "bpf",
			srcs: ["bpf.c"],
		}

		bpf {
			name: "overrideBpf",
			srcs: ["overrideBpf.c"],
		}

		prebuilt_etc {
			name: "myetc",
			src: "myprebuilt",
		}

		prebuilt_etc {
			name: "override_myetc",
			src: "override_myprebuilt",
		}

		cc_library {
			name: "mylib32",
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "mylib64",
			apex_available: [ "myapex" ],
		}
	`, withManifestPackageNameOverrides([]string{"myapex:com.android.myapex"}))

	originalVariant := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Module().(android.OverridableModule)
	overriddenVariant := ctx.ModuleForTests(t, "myapex", "android_common_override_myapex_override_myapex").Module().(android.OverridableModule)
	if originalVariant.GetOverriddenBy() != "" {
		t.Errorf("GetOverriddenBy should be empty, but was %q", originalVariant.GetOverriddenBy())
	}
	if overriddenVariant.GetOverriddenBy() != "override_myapex" {
		t.Errorf("GetOverriddenBy should be \"override_myapex\", but was %q", overriddenVariant.GetOverriddenBy())
	}

	module := ctx.ModuleForTests(t, "myapex", "android_common_override_myapex_override_myapex")
	apexRule := module.Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	ensureNotContains(t, copyCmds, "image.apex/app/app@TEST.BUILD_ID/app.apk")
	ensureContains(t, copyCmds, "image.apex/app/override_app@TEST.BUILD_ID/override_app.apk")

	ensureNotContains(t, copyCmds, "image.apex/etc/bpf/bpf.o")
	ensureContains(t, copyCmds, "image.apex/etc/bpf/overrideBpf.o")

	ensureNotContains(t, copyCmds, "image.apex/etc/myetc")
	ensureContains(t, copyCmds, "image.apex/etc/override_myetc")

	apexBundle := module.Module().(*apexBundle)
	name := apexBundle.Name()
	if name != "override_myapex" {
		t.Errorf("name should be \"override_myapex\", but was %q", name)
	}

	if apexBundle.overridableProperties.Logging_parent != "com.foo.bar" {
		t.Errorf("override_myapex should have logging parent (com.foo.bar), but was %q.", apexBundle.overridableProperties.Logging_parent)
	}

	optFlags := apexRule.Args["opt_flags"]
	ensureContains(t, optFlags, "--override_apk_package_name test.overridden.package")
	ensureContains(t, optFlags, "--pubkey testkey2.avbpubkey")

	signApkRule := module.Rule("signapk")
	ensureEquals(t, signApkRule.Args["certificates"], "testkey.x509.pem testkey.pk8")

	data := android.AndroidMkDataForTest(t, ctx, apexBundle)
	var builder strings.Builder
	data.Custom(&builder, name, "TARGET_", "", data)
	androidMk := builder.String()
	ensureContains(t, androidMk, "LOCAL_MODULE := override_app.override_myapex")
	ensureContains(t, androidMk, "LOCAL_MODULE := overrideBpf.o.override_myapex")
	ensureContains(t, androidMk, "LOCAL_MODULE_STEM := override_myapex.apex")
	ensureContains(t, androidMk, "LOCAL_OVERRIDES_MODULES := unknownapex myapex")
	ensureNotContains(t, androidMk, "LOCAL_MODULE := app.myapex")
	ensureNotContains(t, androidMk, "LOCAL_MODULE := bpf.myapex")
	ensureNotContains(t, androidMk, "LOCAL_MODULE := override_app.myapex")
	ensureNotContains(t, androidMk, "LOCAL_MODULE_STEM := myapex.apex")
}

func TestMinSdkVersionOverride(t *testing.T) {
	t.Parallel()
	// Override from 29 to 31
	minSdkOverride31 := "31"
	ctx := testApex(t, `
			apex {
					name: "myapex",
					key: "myapex.key",
					native_shared_libs: ["mylib"],
					updatable: true,
					min_sdk_version: "29"
			}

			override_apex {
					name: "override_myapex",
					base: "myapex",
					logging_parent: "com.foo.bar",
					package_name: "test.overridden.package"
			}

			apex_key {
					name: "myapex.key",
					public_key: "testkey.avbpubkey",
					private_key: "testkey.pem",
			}

			cc_library {
					name: "mylib",
					srcs: ["mylib.cpp"],
					runtime_libs: ["libbar"],
					system_shared_libs: [],
					stl: "none",
					apex_available: [ "myapex" ],
					min_sdk_version: "apex_inherit"
			}

			cc_library {
					name: "libbar",
					srcs: ["mylib.cpp"],
					system_shared_libs: [],
					stl: "none",
					apex_available: [ "myapex" ],
					min_sdk_version: "apex_inherit"
			}

	`, withApexGlobalMinSdkVersionOverride(&minSdkOverride31))

	apexRule := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that direct non-stubs dep is always included
	ensureContains(t, copyCmds, "image.apex/lib64/mylib.so")

	// Ensure that runtime_libs dep in included
	ensureContains(t, copyCmds, "image.apex/lib64/libbar.so")

	// Ensure libraries target overridden min_sdk_version value
	ensureListContains(t, ctx.ModuleVariantsForTests("libbar"), "android_arm64_armv8-a_shared_apex31")
}

func TestMinSdkVersionOverrideToLowerVersionNoOp(t *testing.T) {
	t.Parallel()
	// Attempt to override from 31 to 29, should be a NOOP
	minSdkOverride29 := "29"
	ctx := testApex(t, `
			apex {
					name: "myapex",
					key: "myapex.key",
					native_shared_libs: ["mylib"],
					updatable: true,
					min_sdk_version: "31"
			}

			override_apex {
					name: "override_myapex",
					base: "myapex",
					logging_parent: "com.foo.bar",
					package_name: "test.overridden.package"
			}

			apex_key {
					name: "myapex.key",
					public_key: "testkey.avbpubkey",
					private_key: "testkey.pem",
			}

			cc_library {
					name: "mylib",
					srcs: ["mylib.cpp"],
					runtime_libs: ["libbar"],
					system_shared_libs: [],
					stl: "none",
					apex_available: [ "myapex" ],
					min_sdk_version: "apex_inherit"
			}

			cc_library {
					name: "libbar",
					srcs: ["mylib.cpp"],
					system_shared_libs: [],
					stl: "none",
					apex_available: [ "myapex" ],
					min_sdk_version: "apex_inherit"
			}

	`, withApexGlobalMinSdkVersionOverride(&minSdkOverride29))

	apexRule := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Rule("apexRule")
	copyCmds := apexRule.Args["copy_commands"]

	// Ensure that direct non-stubs dep is always included
	ensureContains(t, copyCmds, "image.apex/lib64/mylib.so")

	// Ensure that runtime_libs dep in included
	ensureContains(t, copyCmds, "image.apex/lib64/libbar.so")

	// Ensure libraries target the original min_sdk_version value rather than the overridden
	ensureListContains(t, ctx.ModuleVariantsForTests("libbar"), "android_arm64_armv8-a_shared_apex31")
}

func TestLegacyAndroid10Support(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			min_sdk_version: "29",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			stl: "libc++",
			system_shared_libs: [],
			apex_available: [ "myapex" ],
			min_sdk_version: "29",
		}
	`, withUnbundledBuild)

	module := ctx.ModuleForTests(t, "myapex", "android_common_myapex")
	args := module.Rule("apexRule").Args
	ensureContains(t, args["opt_flags"], "--manifest_json "+module.Output("apex_manifest.json").Output.String())

	// The copies of the libraries in the apex should have one more dependency than
	// the ones outside the apex, namely the unwinder. Ideally we should check
	// the dependency names directly here but for some reason the names are blank in
	// this test.
	for _, lib := range []string{"libc++", "mylib"} {
		apexImplicits := ctx.ModuleForTests(t, lib, "android_arm64_armv8-a_shared_apex29").Rule("ld").Implicits
		nonApexImplicits := ctx.ModuleForTests(t, lib, "android_arm64_armv8-a_shared").Rule("ld").Implicits
		if len(apexImplicits) != len(nonApexImplicits)+1 {
			t.Errorf("%q missing unwinder dep", lib)
		}
	}
}

var filesForSdkLibrary = android.MockFS{
	"api/current.txt":        nil,
	"api/removed.txt":        nil,
	"api/system-current.txt": nil,
	"api/system-removed.txt": nil,
	"api/test-current.txt":   nil,
	"api/test-removed.txt":   nil,

	"100/public/api/foo.txt":         nil,
	"100/public/api/foo-removed.txt": nil,
	"100/system/api/foo.txt":         nil,
	"100/system/api/foo-removed.txt": nil,

	// For java_sdk_library_import
	"a.jar": nil,
}

func TestJavaSDKLibrary(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			java_libs: ["foo"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_sdk_library {
			name: "foo",
			srcs: ["a.java"],
			api_packages: ["foo"],
			apex_available: [ "myapex" ],
		}

		prebuilt_apis {
			name: "sdk",
			api_dirs: ["100"],
		}
	`, withFiles(filesForSdkLibrary))

	// java_sdk_library installs both impl jar and permission XML
	ensureExactContents(t, ctx, "myapex", "android_common_myapex", []string{
		"javalib/foo.jar",
		"etc/permissions/foo.xml",
	})
	// Permission XML should point to the activated path of impl jar of java_sdk_library
	sdkLibrary := ctx.ModuleForTests(t, "foo.xml", "android_common_myapex").Output("foo.xml")
	contents := android.ContentFromFileRuleForTests(t, ctx, sdkLibrary)
	ensureMatches(t, contents, "<library\\n\\s+name=\\\"foo\\\"\\n\\s+file=\\\"/apex/myapex/javalib/foo.jar\\\"")
}

func TestJavaSDKLibraryOverrideApexes(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		override_apex {
			name: "mycompanyapex",
			base: "myapex",
		}
		apex {
			name: "myapex",
			key: "myapex.key",
			java_libs: ["foo"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_sdk_library {
			name: "foo",
			srcs: ["a.java"],
			api_packages: ["foo"],
			apex_available: [ "myapex" ],
		}

		prebuilt_apis {
			name: "sdk",
			api_dirs: ["100"],
		}
	`, withFiles(filesForSdkLibrary))

	// Permission XML should point to the activated path of impl jar of java_sdk_library.
	// Since override variants (com.mycompany.android.foo) are installed in the same package as the overridden variant
	// (com.android.foo), the filepath should not contain override apex name.
	sdkLibrary := ctx.ModuleForTests(t, "foo.xml", "android_common_mycompanyapex").Output("foo.xml")
	contents := android.ContentFromFileRuleForTests(t, ctx, sdkLibrary)
	ensureMatches(t, contents, "<library\\n\\s+name=\\\"foo\\\"\\n\\s+file=\\\"/apex/myapex/javalib/foo.jar\\\"")
}

func TestJavaSDKLibrary_WithinApex(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			java_libs: ["foo", "bar"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_sdk_library {
			name: "foo",
			srcs: ["a.java"],
			api_packages: ["foo"],
			apex_available: ["myapex"],
			sdk_version: "none",
			system_modules: "none",
			compile_dex: true,
		}

		java_library {
			name: "bar",
			srcs: ["a.java"],
			libs: ["foo.impl"],
			apex_available: ["myapex"],
			sdk_version: "none",
			system_modules: "none",
			compile_dex: true,
		}

		prebuilt_apis {
			name: "sdk",
			api_dirs: ["100"],
		}
	`, withFiles(filesForSdkLibrary))

	// java_sdk_library installs both impl jar and permission XML
	ensureExactContents(t, ctx, "myapex", "android_common_myapex", []string{
		"javalib/bar.jar",
		"javalib/foo.jar",
		"etc/permissions/foo.xml",
	})

	// The bar library should depend on the implementation jar.
	barLibrary := ctx.ModuleForTests(t, "bar", "android_common_apex10000").Rule("javac")
	if expected, actual := `^-classpath [^:]*/turbine/foo\.jar$`, barLibrary.Args["classpath"]; !regexp.MustCompile(expected).MatchString(actual) {
		t.Errorf("expected %q, found %#q", expected, actual)
	}
}

func TestJavaSDKLibrary_CrossBoundary(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			java_libs: ["foo"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_sdk_library {
			name: "foo",
			srcs: ["a.java"],
			api_packages: ["foo"],
			apex_available: ["myapex"],
			sdk_version: "none",
			system_modules: "none",
		}

		java_library {
			name: "bar",
			srcs: ["a.java"],
			libs: ["foo.stubs"],
			sdk_version: "none",
			system_modules: "none",
		}

		prebuilt_apis {
			name: "sdk",
			api_dirs: ["100"],
		}
	`, withFiles(filesForSdkLibrary))

	// java_sdk_library installs both impl jar and permission XML
	ensureExactContents(t, ctx, "myapex", "android_common_myapex", []string{
		"javalib/foo.jar",
		"etc/permissions/foo.xml",
	})

	// The bar library should depend on the stubs jar.
	barLibrary := ctx.ModuleForTests(t, "bar", "android_common").Rule("javac")
	if expected, actual := `^-classpath [^:]*/foo\.stubs\.from-text/foo\.stubs\.from-text\.jar$`, barLibrary.Args["classpath"]; !regexp.MustCompile(expected).MatchString(actual) {
		t.Errorf("expected %q, found %#q", expected, actual)
	}
}

func TestJavaSDKLibrary_ImportPreferred(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		prebuilt_apis {
			name: "sdk",
			api_dirs: ["100"],
		}`,
		withFiles(map[string][]byte{
			"apex/a.java":             nil,
			"apex/apex_manifest.json": nil,
			"apex/Android.bp": []byte(`
		package {
			default_visibility: ["//visibility:private"],
		}

		apex {
			name: "myapex",
			key: "myapex.key",
			java_libs: ["foo", "bar"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_library {
			name: "bar",
			srcs: ["a.java"],
			libs: ["foo.impl"],
			apex_available: ["myapex"],
			sdk_version: "none",
			system_modules: "none",
			compile_dex: true,
		}
`),
			"source/a.java":          nil,
			"source/api/current.txt": nil,
			"source/api/removed.txt": nil,
			"source/Android.bp": []byte(`
		package {
			default_visibility: ["//visibility:private"],
		}

		java_sdk_library {
			name: "foo",
			visibility: ["//apex"],
			srcs: ["a.java"],
			api_packages: ["foo"],
			apex_available: ["myapex"],
			sdk_version: "none",
			system_modules: "none",
			public: {
				enabled: true,
			},
			compile_dex: true,
		}
`),
			"prebuilt/a.jar": nil,
			"prebuilt/Android.bp": []byte(`
		package {
			default_visibility: ["//visibility:private"],
		}

		java_sdk_library_import {
			name: "foo",
			visibility: ["//apex", "//source"],
			apex_available: ["myapex"],
			prefer: true,
			public: {
				jars: ["a.jar"],
			},
			compile_dex: true,
		}
`),
		}), withFiles(filesForSdkLibrary),
	)

	// java_sdk_library installs both impl jar and permission XML
	ensureExactContents(t, ctx, "myapex", "android_common_myapex", []string{
		"javalib/bar.jar",
		"javalib/foo.jar",
		"etc/permissions/foo.xml",
	})

	// The bar library should depend on the implementation jar.
	barLibrary := ctx.ModuleForTests(t, "bar", "android_common_apex10000").Rule("javac")
	if expected, actual := `^-classpath [^:]*/turbine/foo\.jar$`, barLibrary.Args["classpath"]; !regexp.MustCompile(expected).MatchString(actual) {
		t.Errorf("expected %q, found %#q", expected, actual)
	}
}

func TestJavaSDKLibrary_ImportOnly(t *testing.T) {
	t.Parallel()
	testApexError(t, `java_libs: "foo" is not configured to be compiled into dex`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			java_libs: ["foo"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_sdk_library_import {
			name: "foo",
			apex_available: ["myapex"],
			prefer: true,
			public: {
				jars: ["a.jar"],
			},
		}

	`, withFiles(filesForSdkLibrary))
}

func TestCompatConfig(t *testing.T) {
	t.Parallel()
	result := android.GroupFixturePreparers(
		prepareForApexTest,
		java.PrepareForTestWithPlatformCompatConfig,
	).RunTestWithBp(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			compat_configs: ["myjar-platform-compat-config"],
			java_libs: ["myjar"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		platform_compat_config {
		    name: "myjar-platform-compat-config",
		    src: ":myjar",
		}

		java_library {
			name: "myjar",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			apex_available: [ "myapex" ],
			compile_dex: true,
		}

		// Make sure that a preferred prebuilt does not affect the apex contents.
		prebuilt_platform_compat_config {
			name: "myjar-platform-compat-config",
			metadata: "compat-config/metadata.xml",
			prefer: true,
		}
	`)
	ctx := result.TestContext
	ensureExactContents(t, ctx, "myapex", "android_common_myapex", []string{
		"etc/compatconfig/myjar-platform-compat-config.xml",
		"javalib/myjar.jar",
	})
}

func TestNoDupeApexFiles(t *testing.T) {
	t.Parallel()
	android.GroupFixturePreparers(
		android.PrepareForTestWithAndroidBuildComponents,
		PrepareForTestWithApexBuildComponents,
		prepareForTestWithMyapex,
		prebuilt_etc.PrepareForTestWithPrebuiltEtc,
	).
		ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern("is provided by two different files")).
		RunTestWithBp(t, `
			apex {
				name: "myapex",
				key: "myapex.key",
				prebuilts: ["foo", "bar"],
				updatable: false,
			}

			apex_key {
				name: "myapex.key",
				public_key: "testkey.avbpubkey",
				private_key: "testkey.pem",
			}

			prebuilt_etc {
				name: "foo",
				src: "myprebuilt",
				filename_from_src: true,
			}

			prebuilt_etc {
				name: "bar",
				src: "myprebuilt",
				filename_from_src: true,
			}
		`)
}

func TestApexUnwantedTransitiveDeps(t *testing.T) {
	t.Parallel()
	bp := `
	apex {
		name: "myapex",
		key: "myapex.key",
		native_shared_libs: ["libfoo"],
		updatable: false,
		unwanted_transitive_deps: ["libbar"],
	}

	apex_key {
		name: "myapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}

	cc_library {
		name: "libfoo",
		srcs: ["foo.cpp"],
		shared_libs: ["libbar"],
		apex_available: ["myapex"],
	}

	cc_library {
		name: "libbar",
		srcs: ["bar.cpp"],
		apex_available: ["myapex"],
	}`
	ctx := testApex(t, bp)
	ensureExactContents(t, ctx, "myapex", "android_common_myapex", []string{
		"*/libc++.so",
		"*/libfoo.so",
		// not libbar.so
	})
}

func TestRejectNonInstallableJavaLibrary(t *testing.T) {
	t.Parallel()
	testApexError(t, `"myjar" is not configured to be compiled into dex`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			java_libs: ["myjar"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_library {
			name: "myjar",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			compile_dex: false,
			apex_available: ["myapex"],
		}
	`)
}

func TestSymlinksFromApexToSystem(t *testing.T) {
	t.Parallel()
	bp := `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			java_libs: ["myjar"],
			updatable: false,
		}

		apex {
			name: "myapex.updatable",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			java_libs: ["myjar"],
			updatable: true,
			min_sdk_version: "33",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: [
				"myotherlib",
				"myotherlib_ext",
			],
			system_shared_libs: [],
			stl: "none",
			apex_available: [
				"myapex",
				"myapex.updatable",
				"//apex_available:platform",
			],
			min_sdk_version: "33",
		}

		cc_library {
			name: "myotherlib",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [
				"myapex",
				"myapex.updatable",
				"//apex_available:platform",
			],
			min_sdk_version: "33",
		}

		cc_library {
			name: "myotherlib_ext",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			system_ext_specific: true,
			stl: "none",
			apex_available: [
				"myapex",
				"myapex.updatable",
				"//apex_available:platform",
			],
			min_sdk_version: "33",
		}

		java_library {
			name: "myjar",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			static_libs: ["myotherjar"],
			apex_available: [
				"myapex",
				"myapex.updatable",
				"//apex_available:platform",
			],
			min_sdk_version: "33",
			compile_dex: true,
		}

		java_library {
			name: "myotherjar",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			apex_available: [
				"myapex",
				"myapex.updatable",
				"//apex_available:platform",
			],
			min_sdk_version: "33",
		}
	`

	ensureRealfileExists := func(t *testing.T, files []fileInApex, file string) {
		for _, f := range files {
			if f.path == file {
				if f.isLink {
					t.Errorf("%q is not a real file", file)
				}
				return
			}
		}
		t.Errorf("%q is not found", file)
	}

	ensureSymlinkExists := func(t *testing.T, files []fileInApex, file string, target string) {
		for _, f := range files {
			if f.path == file {
				if !f.isLink {
					t.Errorf("%q is not a symlink", file)
				}
				if f.src != target {
					t.Errorf("expected symlink target to be %q, got %q", target, f.src)
				}
				return
			}
		}
		t.Errorf("%q is not found", file)
	}

	// For unbundled build, symlink shouldn't exist regardless of whether an APEX
	// is updatable or not
	ctx := testApex(t, bp, withUnbundledBuild)
	files := getFiles(t, ctx, "myapex", "android_common_myapex")
	ensureRealfileExists(t, files, "javalib/myjar.jar")
	ensureRealfileExists(t, files, "lib64/mylib.so")
	ensureRealfileExists(t, files, "lib64/myotherlib.so")
	ensureRealfileExists(t, files, "lib64/myotherlib_ext.so")

	files = getFiles(t, ctx, "myapex.updatable", "android_common_myapex.updatable")
	ensureRealfileExists(t, files, "javalib/myjar.jar")
	ensureRealfileExists(t, files, "lib64/mylib.so")
	ensureRealfileExists(t, files, "lib64/myotherlib.so")
	ensureRealfileExists(t, files, "lib64/myotherlib_ext.so")

	// For bundled build, symlink to the system for the non-updatable APEXes only
	ctx = testApex(t, bp)
	files = getFiles(t, ctx, "myapex", "android_common_myapex")
	ensureRealfileExists(t, files, "javalib/myjar.jar")
	ensureRealfileExists(t, files, "lib64/mylib.so")
	ensureSymlinkExists(t, files, "lib64/myotherlib.so", "/system/lib64/myotherlib.so")             // this is symlink
	ensureSymlinkExists(t, files, "lib64/myotherlib_ext.so", "/system_ext/lib64/myotherlib_ext.so") // this is symlink

	files = getFiles(t, ctx, "myapex.updatable", "android_common_myapex.updatable")
	ensureRealfileExists(t, files, "javalib/myjar.jar")
	ensureRealfileExists(t, files, "lib64/mylib.so")
	ensureRealfileExists(t, files, "lib64/myotherlib.so")     // this is a real file
	ensureRealfileExists(t, files, "lib64/myotherlib_ext.so") // this is a real file
}

func TestSymlinksFromApexToSystemRequiredModuleNames(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library_shared {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: ["myotherlib"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [
				"myapex",
				"//apex_available:platform",
			],
		}

		cc_prebuilt_library_shared {
			name: "myotherlib",
			srcs: ["prebuilt.so"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [
				"myapex",
				"//apex_available:platform",
			],
		}
	`)

	apexBundle := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Module().(*apexBundle)
	data := android.AndroidMkDataForTest(t, ctx, apexBundle)
	var builder strings.Builder
	data.Custom(&builder, apexBundle.BaseModuleName(), "TARGET_", "", data)
	androidMk := builder.String()
	// `myotherlib` is added to `myapex` as symlink
	ensureContains(t, androidMk, "LOCAL_MODULE := mylib.myapex\n")
	ensureNotContains(t, androidMk, "LOCAL_MODULE := prebuilt_myotherlib.myapex\n")
	ensureNotContains(t, androidMk, "LOCAL_MODULE := myotherlib.myapex\n")
	// `myapex` should have `myotherlib` in its required line, not `prebuilt_myotherlib`
	ensureContains(t, androidMk, "LOCAL_REQUIRED_MODULES := mylib.myapex:64 myotherlib:64\n")
}

func TestApexWithJniLibs(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			binaries: ["mybin"],
			jni_libs: ["mylib", "mylib3", "libfoo.rust"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			shared_libs: ["mylib2"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "mylib2",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		// Used as both a JNI library and a regular shared library.
		cc_library {
			name: "mylib3",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_binary {
			name: "mybin",
			srcs: ["mybin.cpp"],
			shared_libs: ["mylib3"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		rust_ffi_shared {
			name: "libfoo.rust",
			crate_name: "foo",
			srcs: ["foo.rs"],
			shared_libs: ["libfoo.shared_from_rust"],
			prefer_rlib: true,
			apex_available: ["myapex"],
		}

		cc_library_shared {
			name: "libfoo.shared_from_rust",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["10", "11", "12"],
			},
		}

	`)

	rule := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Rule("apexManifestRule")
	// Notice mylib2.so (transitive dep) is not added as a jni_lib
	ensureEquals(t, rule.Args["opt"], "-a jniLibs libfoo.rust.so mylib.so mylib3.so")
	ensureExactContents(t, ctx, "myapex", "android_common_myapex", []string{
		"bin/mybin",
		"lib64/mylib.so",
		"lib64/mylib2.so",
		"lib64/mylib3.so",
		"lib64/libfoo.rust.so",
	})

	// b/220397949
	ensureListContains(t, names(rule.Args["requireNativeLibs"]), "libfoo.shared_from_rust.so")
}

func TestAppBundle(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			apps: ["AppFoo"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		android_app {
			name: "AppFoo",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			apex_available: [ "myapex" ],
		}
		`, withManifestPackageNameOverrides([]string{"AppFoo:com.android.foo"}))

	bundleConfigRule := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Output("bundle_config.json")
	content := android.ContentFromFileRuleForTests(t, ctx, bundleConfigRule)

	ensureContains(t, content, `"compression":{"uncompressed_glob":["apex_payload.img","apex_manifest.*"]}`)
	ensureContains(t, content, `"apex_config":{"apex_embedded_apk_config":[{"package_name":"com.android.foo","path":"app/AppFoo@TEST.BUILD_ID/AppFoo.apk"}]}`)
}

func TestAppSetBundle(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			apps: ["AppSet"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		android_app_set {
			name: "AppSet",
			set: "AppSet.apks",
		}`)
	mod := ctx.ModuleForTests(t, "myapex", "android_common_myapex")
	bundleConfigRule := mod.Output("bundle_config.json")
	content := android.ContentFromFileRuleForTests(t, ctx, bundleConfigRule)
	ensureContains(t, content, `"compression":{"uncompressed_glob":["apex_payload.img","apex_manifest.*"]}`)
	s := mod.Rule("apexRule").Args["copy_commands"]
	copyCmds := regexp.MustCompile(" *&& *").Split(s, -1)
	if len(copyCmds) != 4 {
		t.Fatalf("Expected 4 commands, got %d in:\n%s", len(copyCmds), s)
	}
	ensureMatches(t, copyCmds[0], "^rm -rf .*/app/AppSet@TEST.BUILD_ID$")
	ensureMatches(t, copyCmds[1], "^mkdir -p .*/app/AppSet@TEST.BUILD_ID$")
	ensureMatches(t, copyCmds[2], "^cp -f .*/app/AppSet@TEST.BUILD_ID/AppSet.apk$")
	ensureMatches(t, copyCmds[3], "^unzip .*-d .*/app/AppSet@TEST.BUILD_ID .*/AppSet.zip$")

	// Ensure that canned_fs_config has an entry for the app set zip file
	generateFsRule := mod.Rule("generateFsConfig")
	cmd := generateFsRule.RuleParams.Command
	ensureContains(t, cmd, "AppSet.zip")
}

func TestAppSetBundlePrebuilt(t *testing.T) {
	bp := `
		apex_set {
			name: "myapex",
			filename: "foo_v2.apex",
			sanitized: {
				none: { set: "myapex.apks", },
				hwaddress: { set: "myapex.hwasan.apks", },
			},
		}
	`
	ctx := testApex(t, bp, prepareForTestWithSantitizeHwaddress)

	// Check that the extractor produces the correct output file from the correct input file.
	extractorOutput := "out/soong/.intermediates/myapex/android_common_prebuilt_myapex/extracted/myapex.hwasan.apks"

	m := ctx.ModuleForTests(t, "myapex", "android_common_prebuilt_myapex")
	extractedApex := m.Output(extractorOutput)

	android.AssertArrayString(t, "extractor input", []string{"myapex.hwasan.apks"}, extractedApex.Inputs.Strings())

	// Ditto for the apex.
	m = ctx.ModuleForTests(t, "myapex", "android_common_prebuilt_myapex")
	copiedApex := m.Output("out/soong/.intermediates/myapex/android_common_prebuilt_myapex/foo_v2.apex")

	android.AssertStringEquals(t, "myapex input", extractorOutput, copiedApex.Input.String())
}

func TestApexSetApksModuleAssignment(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex_set {
			name: "myapex",
			set: ":myapex_apks_file",
		}

		filegroup {
			name: "myapex_apks_file",
			srcs: ["myapex.apks"],
		}
	`)

	m := ctx.ModuleForTests(t, "myapex", "android_common_prebuilt_myapex")

	// Check that the extractor produces the correct apks file from the input module
	extractorOutput := "out/soong/.intermediates/myapex/android_common_prebuilt_myapex/extracted/myapex.apks"
	extractedApex := m.Output(extractorOutput)

	android.AssertArrayString(t, "extractor input", []string{"myapex.apks"}, extractedApex.Inputs.Strings())
}

func testDexpreoptWithApexes(t *testing.T, bp, errmsg string, preparer android.FixturePreparer, fragments ...java.ApexVariantReference) *android.TestContext {
	t.Helper()

	fs := android.MockFS{
		"a.java":              nil,
		"a.jar":               nil,
		"apex_manifest.json":  nil,
		"AndroidManifest.xml": nil,
		"system/sepolicy/apex/myapex-file_contexts":                  nil,
		"system/sepolicy/apex/some-updatable-apex-file_contexts":     nil,
		"system/sepolicy/apex/some-non-updatable-apex-file_contexts": nil,
		"system/sepolicy/apex/com.android.art.debug-file_contexts":   nil,
		"framework/aidl/a.aidl":                                      nil,
	}

	errorHandler := android.FixtureExpectsNoErrors
	if errmsg != "" {
		errorHandler = android.FixtureExpectsAtLeastOneErrorMatchingPattern(errmsg)
	}

	result := android.GroupFixturePreparers(
		cc.PrepareForTestWithCcDefaultModules,
		java.PrepareForTestWithHiddenApiBuildComponents,
		java.PrepareForTestWithDexpreopt,
		java.PrepareForTestWithJavaSdkLibraryFiles,
		PrepareForTestWithApexBuildComponents,
		preparer,
		fs.AddToFixture(),
		android.FixtureModifyMockFS(func(fs android.MockFS) {
			if _, ok := fs["frameworks/base/boot/Android.bp"]; !ok {
				insert := ""
				for _, fragment := range fragments {
					insert += fmt.Sprintf("{apex: %q, module: %q},\n", *fragment.Apex, *fragment.Module)
				}
				fs["frameworks/base/boot/Android.bp"] = []byte(fmt.Sprintf(`
					platform_bootclasspath {
						name: "platform-bootclasspath",
						fragments: [
							{apex: "com.android.art", module: "art-bootclasspath-fragment"},
  						%s
						],
					}
				`, insert))
			}
		}),
		// Dexpreopt for boot jars requires the ART boot image profile.
		java.PrepareApexBootJarModule("com.android.art", "core-oj"),
		dexpreopt.FixtureSetArtBootJars("com.android.art:core-oj"),
		dexpreopt.FixtureSetBootImageProfiles("art/build/boot/boot-image-profile.txt"),
	).
		ExtendWithErrorHandler(errorHandler).
		RunTestWithBp(t, bp)

	return result.TestContext
}

func TestUpdatable_should_set_min_sdk_version(t *testing.T) {
	t.Parallel()
	testApexError(t, `"myapex" .*: updatable: updatable APEXes should set min_sdk_version`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			updatable: true,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`)
}

func TestUpdatableDefault_should_set_min_sdk_version(t *testing.T) {
	t.Parallel()
	testApexError(t, `"myapex" .*: updatable: updatable APEXes should set min_sdk_version`, `
		apex {
			name: "myapex",
			key: "myapex.key",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`)
}

func TestUpdatable_should_not_set_generate_classpaths_proto(t *testing.T) {
	t.Parallel()
	testApexError(t, `"mysystemserverclasspathfragment" .* it must not set generate_classpaths_proto to false`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			systemserverclasspath_fragments: [
				"mysystemserverclasspathfragment",
			],
			min_sdk_version: "29",
			updatable: true,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_library {
			name: "foo",
			srcs: ["b.java"],
			min_sdk_version: "29",
			installable: true,
			apex_available: [
				"myapex",
			],
			sdk_version: "current",
		}

		systemserverclasspath_fragment {
			name: "mysystemserverclasspathfragment",
			generate_classpaths_proto: false,
			contents: [
				"foo",
			],
			apex_available: [
				"myapex",
			],
		}
	`,
		dexpreopt.FixtureSetApexSystemServerJars("myapex:foo"),
	)
}

func TestDexpreoptAccessDexFilesFromPrebuiltApex(t *testing.T) {
	t.Parallel()
	preparer := java.FixtureConfigureApexBootJars("myapex:libfoo")
	t.Run("prebuilt no source", func(t *testing.T) {
		t.Parallel()
		fragment := java.ApexVariantReference{
			Apex:   proptools.StringPtr("myapex"),
			Module: proptools.StringPtr("my-bootclasspath-fragment"),
		}

		testDexpreoptWithApexes(t, `
			prebuilt_apex {
				name: "myapex" ,
				arch: {
					arm64: {
						src: "myapex-arm64.apex",
					},
					arm: {
						src: "myapex-arm.apex",
					},
				},
				exported_bootclasspath_fragments: ["my-bootclasspath-fragment"],
			}

			prebuilt_bootclasspath_fragment {
				name: "my-bootclasspath-fragment",
				contents: ["libfoo"],
				apex_available: ["myapex"],
				hidden_api: {
					annotation_flags: "my-bootclasspath-fragment/annotation-flags.csv",
					metadata: "my-bootclasspath-fragment/metadata.csv",
					index: "my-bootclasspath-fragment/index.csv",
					signature_patterns: "my-bootclasspath-fragment/signature-patterns.csv",
					filtered_stub_flags: "my-bootclasspath-fragment/filtered-stub-flags.csv",
					filtered_flags: "my-bootclasspath-fragment/filtered-flags.csv",
				},
			}

		java_sdk_library_import {
			name: "libfoo",
			prefer: true,
			public: {
				jars: ["libfoo.jar"],
			},
			apex_available: ["myapex"],
			shared_library: false,
			permitted_packages: ["libfoo"],
		}
		`, "", preparer, fragment)
	})
}

func testBootJarPermittedPackagesRules(t *testing.T, errmsg, bp string, bootJars []string, rules []android.Rule) {
	t.Helper()
	bp += `
	apex_key {
		name: "myapex.key",
		public_key: "testkey.avbpubkey",
		private_key: "testkey.pem",
	}`
	fs := android.MockFS{
		"lib1/src/A.java": nil,
		"lib2/src/B.java": nil,
		"system/sepolicy/apex/myapex-file_contexts": nil,
	}

	errorHandler := android.FixtureExpectsNoErrors
	if errmsg != "" {
		errorHandler = android.FixtureExpectsAtLeastOneErrorMatchingPattern(errmsg)
	}

	android.GroupFixturePreparers(
		android.PrepareForTestWithAndroidBuildComponents,
		java.PrepareForTestWithJavaBuildComponents,
		PrepareForTestWithApexBuildComponents,
		android.PrepareForTestWithNeverallowRules(rules),
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			apexBootJars := make([]string, 0, len(bootJars))
			for _, apexBootJar := range bootJars {
				apexBootJars = append(apexBootJars, "myapex:"+apexBootJar)
			}
			variables.ApexBootJars = android.CreateTestConfiguredJarList(apexBootJars)
		}),
		fs.AddToFixture(),
	).
		ExtendWithErrorHandler(errorHandler).
		RunTestWithBp(t, bp)
}

func TestApexPermittedPackagesRules(t *testing.T) {
	t.Parallel()
	testcases := []struct {
		name                 string
		expectedError        string
		bp                   string
		bootJars             []string
		bcpPermittedPackages map[string][]string
	}{

		{
			name:          "Non-Bootclasspath apex jar not satisfying allowed module packages.",
			expectedError: "",
			bp: `
				java_library {
					name: "bcp_lib1",
					srcs: ["lib1/src/*.java"],
					permitted_packages: ["foo.bar"],
					apex_available: ["myapex"],
					sdk_version: "none",
					system_modules: "none",
					compile_dex: true,
				}
				java_library {
					name: "nonbcp_lib2",
					srcs: ["lib2/src/*.java"],
					apex_available: ["myapex"],
					permitted_packages: ["a.b"],
					sdk_version: "none",
					system_modules: "none",
					compile_dex: true,
				}
				apex {
					name: "myapex",
					key: "myapex.key",
					java_libs: ["bcp_lib1", "nonbcp_lib2"],
					updatable: false,
				}`,
			bootJars: []string{"bcp_lib1"},
			bcpPermittedPackages: map[string][]string{
				"bcp_lib1": []string{
					"foo.bar",
				},
			},
		},
		{
			name:          "Bootclasspath apex jar not satisfying allowed module packages.",
			expectedError: `(?s)module "bcp_lib2" .* which is restricted because bcp_lib2 bootjar may only use these package prefixes: foo.bar. Please consider the following alternatives:\n    1. If the offending code is from a statically linked library, consider removing that dependency and using an alternative already in the bootclasspath, or perhaps a shared library.    2. Move the offending code into an allowed package.\n    3. Jarjar the offending code. Please be mindful of the potential system health implications of bundling that code, particularly if the offending jar is part of the bootclasspath.`,
			bp: `
				java_library {
					name: "bcp_lib1",
					srcs: ["lib1/src/*.java"],
					apex_available: ["myapex"],
					permitted_packages: ["foo.bar"],
					sdk_version: "none",
					system_modules: "none",
					compile_dex: true,
				}
				java_library {
					name: "bcp_lib2",
					srcs: ["lib2/src/*.java"],
					apex_available: ["myapex"],
					permitted_packages: ["foo.bar", "bar.baz"],
					sdk_version: "none",
					system_modules: "none",
					compile_dex: true,
				}
				apex {
					name: "myapex",
					key: "myapex.key",
					java_libs: ["bcp_lib1", "bcp_lib2"],
					updatable: false,
				}
			`,
			bootJars: []string{"bcp_lib1", "bcp_lib2"},
			bcpPermittedPackages: map[string][]string{
				"bcp_lib1": []string{
					"foo.bar",
				},
				"bcp_lib2": []string{
					"foo.bar",
				},
			},
		},
		{
			name:          "Updateable Bootclasspath apex jar not satisfying allowed module packages.",
			expectedError: "",
			bp: `
				java_library {
					name: "bcp_lib_restricted",
					srcs: ["lib1/src/*.java"],
					apex_available: ["myapex"],
					permitted_packages: ["foo.bar"],
					sdk_version: "none",
					min_sdk_version: "29",
					system_modules: "none",
					compile_dex: true,
				}
				java_library {
					name: "bcp_lib_unrestricted",
					srcs: ["lib2/src/*.java"],
					apex_available: ["myapex"],
					permitted_packages: ["foo.bar", "bar.baz"],
					sdk_version: "none",
					min_sdk_version: "29",
					system_modules: "none",
					compile_dex: true,
				}
				apex {
					name: "myapex",
					key: "myapex.key",
					java_libs: ["bcp_lib_restricted", "bcp_lib_unrestricted"],
					updatable: true,
					min_sdk_version: "29",
				}
			`,
			bootJars: []string{"bcp_lib1", "bcp_lib2"},
			bcpPermittedPackages: map[string][]string{
				"bcp_lib1_non_updateable": []string{
					"foo.bar",
				},
				// bcp_lib2_updateable has no entry here since updateable bcp can contain new packages - tracking via an allowlist is not necessary
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rules := createBcpPermittedPackagesRules(tc.bcpPermittedPackages)
			testBootJarPermittedPackagesRules(t, tc.expectedError, tc.bp, tc.bootJars, rules)
		})
	}
}

// TODO(jungjw): Move this to proptools
func intPtr(i int) *int {
	return &i
}

func TestApexSet(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex_set {
			name: "myapex",
			set: "myapex.apks",
			filename: "foo_v2.apex",
			overrides: ["foo"],
		}
	`,
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.Platform_sdk_version = intPtr(30)
		}),
		android.FixtureModifyConfig(func(config android.Config) {
			config.Targets[android.Android] = []android.Target{
				{Os: android.Android, Arch: android.Arch{ArchType: android.Arm, ArchVariant: "armv7-a-neon", Abi: []string{"armeabi-v7a"}}},
				{Os: android.Android, Arch: android.Arch{ArchType: android.Arm64, ArchVariant: "armv8-a", Abi: []string{"arm64-v8a"}}},
			}
		}),
	)

	m := ctx.ModuleForTests(t, "myapex", "android_common_prebuilt_myapex")

	// Check extract_apks tool parameters.
	extractedApex := m.Output("extracted/myapex.apks")
	actual := extractedApex.Args["abis"]
	expected := "ARMEABI_V7A,ARM64_V8A"
	if actual != expected {
		t.Errorf("Unexpected abis parameter - expected %q vs actual %q", expected, actual)
	}
	actual = extractedApex.Args["sdk-version"]
	expected = "30"
	if actual != expected {
		t.Errorf("Unexpected abis parameter - expected %q vs actual %q", expected, actual)
	}

	m = ctx.ModuleForTests(t, "myapex", "android_common_prebuilt_myapex")
	a := m.Module().(*ApexSet)
	expectedOverrides := []string{"foo"}
	actualOverrides := android.AndroidMkEntriesForTest(t, ctx, a)[0].EntryMap["LOCAL_OVERRIDES_MODULES"]
	if !reflect.DeepEqual(actualOverrides, expectedOverrides) {
		t.Errorf("Incorrect LOCAL_OVERRIDES_MODULES - expected %q vs actual %q", expectedOverrides, actualOverrides)
	}
}

func TestApexSet_NativeBridge(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex_set {
			name: "myapex",
			set: "myapex.apks",
			filename: "foo_v2.apex",
			overrides: ["foo"],
		}
	`,
		android.FixtureModifyConfig(func(config android.Config) {
			config.Targets[android.Android] = []android.Target{
				{Os: android.Android, Arch: android.Arch{ArchType: android.X86_64, ArchVariant: "", Abi: []string{"x86_64"}}},
				{Os: android.Android, Arch: android.Arch{ArchType: android.Arm64, ArchVariant: "armv8-a", Abi: []string{"arm64-v8a"}}, NativeBridge: android.NativeBridgeEnabled},
			}
		}),
	)

	m := ctx.ModuleForTests(t, "myapex", "android_common_prebuilt_myapex")

	// Check extract_apks tool parameters. No native bridge arch expected
	extractedApex := m.Output("extracted/myapex.apks")
	android.AssertStringEquals(t, "abis", "X86_64", extractedApex.Args["abis"])
}

func TestNoStaticLinkingToStubsLib(t *testing.T) {
	t.Parallel()
	testApexError(t, `.*required by "mylib" is a native library providing stub.*`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			static_libs: ["otherlib"],
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
		}

		cc_library {
			name: "otherlib",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["1", "2", "3"],
			},
			apex_available: [ "myapex" ],
		}
	`)
}

func TestApexKeysTxt(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			updatable: false,
			custom_sign_tool: "sign_myapex",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`)

	myapex := ctx.ModuleForTests(t, "myapex", "android_common_myapex")
	content := android.ContentFromFileRuleForTests(t, ctx, myapex.Output("apexkeys.txt"))
	ensureContains(t, content, `name="myapex.apex" public_key="vendor/foo/devkeys/testkey.avbpubkey" private_key="vendor/foo/devkeys/testkey.pem" container_certificate="vendor/foo/devkeys/test.x509.pem" container_private_key="vendor/foo/devkeys/test.pk8" partition="system" sign_tool="sign_myapex"`)
}

func TestApexKeysTxtOverrides(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			updatable: false,
			custom_sign_tool: "sign_myapex",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		prebuilt_apex {
			name: "myapex",
			prefer: true,
			arch: {
				arm64: {
					src: "myapex-arm64.apex",
				},
				arm: {
					src: "myapex-arm.apex",
				},
			},
		}

		apex_set {
			name: "myapex_set",
			set: "myapex.apks",
			filename: "myapex_set.apex",
			overrides: ["myapex"],
		}
	`)

	content := android.ContentFromFileRuleForTests(t, ctx,
		ctx.ModuleForTests(t, "myapex", "android_common_myapex").Output("apexkeys.txt"))
	ensureContains(t, content, `name="myapex.apex" public_key="vendor/foo/devkeys/testkey.avbpubkey" private_key="vendor/foo/devkeys/testkey.pem" container_certificate="vendor/foo/devkeys/test.x509.pem" container_private_key="vendor/foo/devkeys/test.pk8" partition="system" sign_tool="sign_myapex"`)
	content = android.ContentFromFileRuleForTests(t, ctx,
		ctx.ModuleForTests(t, "myapex_set", "android_common_prebuilt_myapex_set").Output("apexkeys.txt"))
	ensureContains(t, content, `name="myapex_set.apex" public_key="PRESIGNED" private_key="PRESIGNED" container_certificate="PRESIGNED" container_private_key="PRESIGNED" partition="system"`)
}

func TestAllowedFiles(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			apps: ["app"],
			allowed_files: "allowed.txt",
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		android_app {
			name: "app",
			srcs: ["foo/bar/MyClass.java"],
			package_name: "foo",
			sdk_version: "none",
			system_modules: "none",
			apex_available: [ "myapex" ],
		}
	`, withFiles(map[string][]byte{
		"sub/Android.bp": []byte(`
			override_apex {
				name: "override_myapex",
				base: "myapex",
				apps: ["override_app"],
				allowed_files: ":allowed",
			}
			// Overridable "path" property should be referenced indirectly
			filegroup {
				name: "allowed",
				srcs: ["allowed.txt"],
			}
			override_android_app {
				name: "override_app",
				base: "app",
				package_name: "bar",
			}
			`),
	}))

	rule := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Rule("diffApexContentRule")
	if expected, actual := "allowed.txt", rule.Args["allowed_files_file"]; expected != actual {
		t.Errorf("allowed_files_file: expected %q but got %q", expected, actual)
	}

	rule2 := ctx.ModuleForTests(t, "myapex", "android_common_override_myapex_override_myapex").Rule("diffApexContentRule")
	if expected, actual := "sub/allowed.txt", rule2.Args["allowed_files_file"]; expected != actual {
		t.Errorf("allowed_files_file: expected %q but got %q", expected, actual)
	}
}

func TestNonPreferredPrebuiltDependency(t *testing.T) {
	t.Parallel()
	testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			stubs: {
				versions: ["current"],
			},
			apex_available: ["myapex"],
		}

		cc_prebuilt_library_shared {
			name: "mylib",
			prefer: false,
			srcs: ["prebuilt.so"],
			stubs: {
				versions: ["current"],
			},
			apex_available: ["myapex"],
		}
	`)
}

func TestCompressedApex(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			compressible: true,
			updatable: false,
		}
		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`,
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.CompressedApex = proptools.BoolPtr(true)
		}),
	)

	compressRule := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Rule("compressRule")
	ensureContains(t, compressRule.Output.String(), "myapex.capex.unsigned")

	signApkRule := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Description("sign compressedApex")
	ensureEquals(t, signApkRule.Input.String(), compressRule.Output.String())

	// Make sure output of bundle is .capex
	ab := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Module().(*apexBundle)
	ensureContains(t, ab.outputFile.String(), "myapex.capex")

	// Verify android.mk rules
	data := android.AndroidMkDataForTest(t, ctx, ab)
	var builder strings.Builder
	data.Custom(&builder, ab.BaseModuleName(), "TARGET_", "", data)
	androidMk := builder.String()
	ensureContains(t, androidMk, "LOCAL_MODULE_STEM := myapex.capex\n")
}

func TestCompressedApexIsDisabledWhenUsingErofs(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			compressible: true,
			updatable: false,
			payload_fs_type: "erofs",
		}
		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`,
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.CompressedApex = proptools.BoolPtr(true)
		}),
	)

	compressRule := ctx.ModuleForTests(t, "myapex", "android_common_myapex").MaybeRule("compressRule")
	if compressRule.Rule != nil {
		t.Error("erofs apex should not be compressed")
	}
}

func TestApexSet_ShouldRespectCompressedApexFlag(t *testing.T) {
	t.Parallel()
	for _, compressionEnabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("compressionEnabled=%v", compressionEnabled), func(t *testing.T) {
			t.Parallel()
			ctx := testApex(t, `
				apex_set {
					name: "com.company.android.myapex",
					apex_name: "com.android.myapex",
					set: "company-myapex.apks",
				}
			`, android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
				variables.CompressedApex = proptools.BoolPtr(compressionEnabled)
			}),
			)

			build := ctx.ModuleForTests(t, "com.company.android.myapex", "android_common_prebuilt_com.android.myapex").Output("com.company.android.myapex.apex")
			if compressionEnabled {
				ensureEquals(t, build.Rule.String(), "android/soong/android.Cp")
			} else {
				ensureEquals(t, build.Rule.String(), "android/apex.decompressApex")
			}
		})
	}
}

func TestPreferredPrebuiltSharedLibDep(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			apex_available: ["myapex"],
			shared_libs: ["otherlib"],
			system_shared_libs: [],
		}

		cc_library {
			name: "otherlib",
			srcs: ["mylib.cpp"],
			stubs: {
				versions: ["current"],
			},
		}

		cc_prebuilt_library_shared {
			name: "otherlib",
			prefer: true,
			srcs: ["prebuilt.so"],
			stubs: {
				versions: ["current"],
			},
		}
	`)

	ab := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Module().(*apexBundle)
	data := android.AndroidMkDataForTest(t, ctx, ab)
	var builder strings.Builder
	data.Custom(&builder, ab.BaseModuleName(), "TARGET_", "", data)
	androidMk := builder.String()

	// The make level dependency needs to be on otherlib - prebuilt_otherlib isn't
	// a thing there.
	ensureContains(t, androidMk, "LOCAL_REQUIRED_MODULES := libc++:64 mylib.myapex:64 otherlib\n")
}

func TestExcludeDependency(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
			apex_available: ["myapex"],
			shared_libs: ["mylib2"],
			target: {
				apex: {
					exclude_shared_libs: ["mylib2"],
				},
			},
		}

		cc_library {
			name: "mylib2",
			srcs: ["mylib.cpp"],
			system_shared_libs: [],
			stl: "none",
		}
	`)

	// Check if mylib is linked to mylib2 for the non-apex target
	ldFlags := ctx.ModuleForTests(t, "mylib", "android_arm64_armv8-a_shared").Rule("ld").Args["libFlags"]
	ensureContains(t, ldFlags, "mylib2/android_arm64_armv8-a_shared/mylib2.so")

	// Make sure that the link doesn't occur for the apex target
	ldFlags = ctx.ModuleForTests(t, "mylib", "android_arm64_armv8-a_shared_apex10000").Rule("ld").Args["libFlags"]
	ensureNotContains(t, ldFlags, "mylib2/android_arm64_armv8-a_shared_apex10000/mylib2.so")

	// It shouldn't appear in the copy cmd as well.
	copyCmds := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Rule("apexRule").Args["copy_commands"]
	ensureNotContains(t, copyCmds, "image.apex/lib64/mylib2.so")
}

func TestPrebuiltStubLibDep(t *testing.T) {
	t.Parallel()
	bpBase := `
		apex {
			name: "myapex",
			key: "myapex.key",
			native_shared_libs: ["mylib"],
			updatable: false,
		}
		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
		cc_library {
			name: "mylib",
			srcs: ["mylib.cpp"],
			apex_available: ["myapex"],
			shared_libs: ["stublib"],
			system_shared_libs: [],
		}
		apex {
			name: "otherapex",
			enabled: %s,
			key: "myapex.key",
			native_shared_libs: ["stublib"],
			updatable: false,
		}
	`

	stublibSourceBp := `
		cc_library {
			name: "stublib",
			srcs: ["mylib.cpp"],
			apex_available: ["otherapex"],
			system_shared_libs: [],
			stl: "none",
			stubs: {
				versions: ["1"],
			},
		}
	`

	stublibPrebuiltBp := `
		cc_prebuilt_library_shared {
			name: "stublib",
			srcs: ["prebuilt.so"],
			apex_available: ["otherapex"],
			stubs: {
				versions: ["1"],
			},
			%s
		}
	`

	tests := []struct {
		name             string
		stublibBp        string
		usePrebuilt      bool
		modNames         []string // Modules to collect AndroidMkEntries for
		otherApexEnabled []string
	}{
		{
			name:             "only_source",
			stublibBp:        stublibSourceBp,
			usePrebuilt:      false,
			modNames:         []string{"stublib"},
			otherApexEnabled: []string{"true", "false"},
		},
		{
			name:             "source_preferred",
			stublibBp:        stublibSourceBp + fmt.Sprintf(stublibPrebuiltBp, ""),
			usePrebuilt:      false,
			modNames:         []string{"stublib", "prebuilt_stublib"},
			otherApexEnabled: []string{"true", "false"},
		},
		{
			name:             "prebuilt_preferred",
			stublibBp:        stublibSourceBp + fmt.Sprintf(stublibPrebuiltBp, "prefer: true,"),
			usePrebuilt:      true,
			modNames:         []string{"stublib", "prebuilt_stublib"},
			otherApexEnabled: []string{"false"}, // No "true" since APEX cannot depend on prebuilt.
		},
		{
			name:             "only_prebuilt",
			stublibBp:        fmt.Sprintf(stublibPrebuiltBp, ""),
			usePrebuilt:      true,
			modNames:         []string{"stublib"},
			otherApexEnabled: []string{"false"}, // No "true" since APEX cannot depend on prebuilt.
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			for _, otherApexEnabled := range test.otherApexEnabled {
				t.Run("otherapex_enabled_"+otherApexEnabled, func(t *testing.T) {
					t.Parallel()
					ctx := testApex(t, fmt.Sprintf(bpBase, otherApexEnabled)+test.stublibBp)

					type modAndMkEntries struct {
						mod       *cc.Module
						mkEntries android.AndroidMkInfo
					}
					entries := []*modAndMkEntries{}

					// Gather shared lib modules that are installable
					for _, modName := range test.modNames {
						for _, variant := range ctx.ModuleVariantsForTests(modName) {
							if !strings.HasPrefix(variant, "android_arm64_armv8-a_shared") {
								continue
							}
							mod := ctx.ModuleForTests(t, modName, variant).Module().(*cc.Module)
							if !mod.Enabled(android.PanickingConfigAndErrorContext(ctx)) || mod.IsHideFromMake() {
								continue
							}
							info := android.AndroidMkInfoForTest(t, ctx, mod)
							ents := []android.AndroidMkInfo{info.PrimaryInfo}
							ents = append(ents, info.ExtraInfo...)
							for _, ent := range ents {
								if ent.Disabled {
									continue
								}
								entries = append(entries, &modAndMkEntries{
									mod:       mod,
									mkEntries: ent,
								})
							}
						}
					}

					var entry *modAndMkEntries = nil
					for _, ent := range entries {
						if strings.Join(ent.mkEntries.EntryMap["LOCAL_MODULE"], ",") == "stublib" {
							if entry != nil {
								t.Errorf("More than one AndroidMk entry for \"stublib\": %s and %s", entry.mod, ent.mod)
							} else {
								entry = ent
							}
						}
					}

					if entry == nil {
						t.Errorf("AndroidMk entry for \"stublib\" missing")
					} else {
						isPrebuilt := entry.mod.Prebuilt() != nil
						if isPrebuilt != test.usePrebuilt {
							t.Errorf("Wrong module for \"stublib\" AndroidMk entry: got prebuilt %t, want prebuilt %t", isPrebuilt, test.usePrebuilt)
						}
						if !entry.mod.IsStubs() {
							t.Errorf("Module for \"stublib\" AndroidMk entry isn't a stub: %s", entry.mod)
						}
						if entry.mkEntries.EntryMap["LOCAL_NOT_AVAILABLE_FOR_PLATFORM"] != nil {
							t.Errorf("AndroidMk entry for \"stublib\" has LOCAL_NOT_AVAILABLE_FOR_PLATFORM set: %+v", entry.mkEntries)
						}
						cflags := entry.mkEntries.EntryMap["LOCAL_EXPORT_CFLAGS"]
						expected := "-D__STUBLIB_API__=10000"
						if !android.InList(expected, cflags) {
							t.Errorf("LOCAL_EXPORT_CFLAGS expected to have %q, but got %q", expected, cflags)
						}
					}
				})
			}
		})
	}
}

func TestApexJavaCoverage(t *testing.T) {
	t.Parallel()
	bp := `
		apex {
			name: "myapex",
			key: "myapex.key",
			java_libs: ["mylib"],
			bootclasspath_fragments: ["mybootclasspathfragment"],
			systemserverclasspath_fragments: ["mysystemserverclasspathfragment"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_library {
			name: "mylib",
			srcs: ["mylib.java"],
			apex_available: ["myapex"],
			compile_dex: true,
		}

		bootclasspath_fragment {
			name: "mybootclasspathfragment",
			contents: ["mybootclasspathlib"],
			apex_available: ["myapex"],
			hidden_api: {
				split_packages: ["*"],
			},
		}

		java_library {
			name: "mybootclasspathlib",
			srcs: ["mybootclasspathlib.java"],
			apex_available: ["myapex"],
			compile_dex: true,
			sdk_version: "current",
		}

		systemserverclasspath_fragment {
			name: "mysystemserverclasspathfragment",
			contents: ["mysystemserverclasspathlib"],
			apex_available: ["myapex"],
		}

		java_library {
			name: "mysystemserverclasspathlib",
			srcs: ["mysystemserverclasspathlib.java"],
			apex_available: ["myapex"],
			compile_dex: true,
		}
	`

	result := android.GroupFixturePreparers(
		PrepareForTestWithApexBuildComponents,
		prepareForTestWithMyapex,
		java.PrepareForTestWithJavaDefaultModules,
		android.PrepareForTestWithAndroidBuildComponents,
		android.FixtureWithRootAndroidBp(bp),
		dexpreopt.FixtureSetApexBootJars("myapex:mybootclasspathlib"),
		dexpreopt.FixtureSetApexSystemServerJars("myapex:mysystemserverclasspathlib"),
		java.PrepareForTestWithJacocoInstrumentation,
	).RunTest(t)

	// Make sure jacoco ran on both mylib and mybootclasspathlib
	if result.ModuleForTests(t, "mylib", "android_common_apex10000").MaybeRule("jacoco").Rule == nil {
		t.Errorf("Failed to find jacoco rule for mylib")
	}
	if result.ModuleForTests(t, "mybootclasspathlib", "android_common_apex10000").MaybeRule("jacoco").Rule == nil {
		t.Errorf("Failed to find jacoco rule for mybootclasspathlib")
	}
	if result.ModuleForTests(t, "mysystemserverclasspathlib", "android_common_apex10000").MaybeRule("jacoco").Rule == nil {
		t.Errorf("Failed to find jacoco rule for mysystemserverclasspathlib")
	}
}

func TestProhibitStaticExecutable(t *testing.T) {
	t.Parallel()
	testApexError(t, `executable mybin is static`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			binaries: ["mybin"],
			min_sdk_version: "29",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		cc_binary {
			name: "mybin",
			srcs: ["mylib.cpp"],
			relative_install_path: "foo/bar",
			static_executable: true,
			system_shared_libs: [],
			stl: "none",
			apex_available: [ "myapex" ],
			min_sdk_version: "29",
		}
	`)

	testApexError(t, `executable mybin.rust is static`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			binaries: ["mybin.rust"],
			min_sdk_version: "29",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		rust_binary {
			name: "mybin.rust",
			srcs: ["foo.rs"],
			static_executable: true,
			apex_available: ["myapex"],
			min_sdk_version: "29",
		}
	`)
}

func TestAndroidMk_DexpreoptBuiltInstalledForApex(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			updatable: false,
			java_libs: ["foo"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_library {
			name: "foo",
			srcs: ["foo.java"],
			apex_available: ["myapex"],
			installable: true,
		}
	`,
		dexpreopt.FixtureSetApexSystemServerJars("myapex:foo"),
	)

	apexBundle := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Module().(*apexBundle)
	data := android.AndroidMkDataForTest(t, ctx, apexBundle)
	var builder strings.Builder
	data.Custom(&builder, apexBundle.BaseModuleName(), "TARGET_", "", data)
	androidMk := builder.String()
	out := ctx.Config().OutDir()
	ensureContains(t, androidMk, "LOCAL_SOONG_INSTALL_PAIRS += "+
		filepath.Join(out, "soong/.intermediates/foo/android_common_apex10000/dexpreopt/foo/oat/arm64/javalib.odex")+
		":"+
		filepath.Join(out, "target/product/test_device/system/framework/oat/arm64/apex@myapex@javalib@foo.jar@classes.odex")+
		" "+
		filepath.Join(out, "soong/.intermediates/foo/android_common_apex10000/dexpreopt/foo/oat/arm64/javalib.vdex")+
		":"+
		filepath.Join(out, "target/product/test_device/system/framework/oat/arm64/apex@myapex@javalib@foo.jar@classes.vdex")+
		"\n")
}

func TestAndroidMk_RequiredModules(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			updatable: false,
			java_libs: ["foo"],
			required: ["otherapex"],
		}

		apex {
			name: "otherapex",
			key: "myapex.key",
			updatable: false,
			java_libs: ["foo"],
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_library {
			name: "foo",
			srcs: ["foo.java"],
			apex_available: ["myapex", "otherapex"],
			installable: true,
		}
	`)

	apexBundle := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Module().(*apexBundle)
	data := android.AndroidMkDataForTest(t, ctx, apexBundle)
	var builder strings.Builder
	data.Custom(&builder, apexBundle.BaseModuleName(), "TARGET_", "", data)
	androidMk := builder.String()
	ensureContains(t, androidMk, "LOCAL_REQUIRED_MODULES := foo.myapex otherapex")
}

func TestAndroidMk_RequiredDeps(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`)

	bundle := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Module().(*apexBundle)
	bundle.makeModulesToInstall = append(bundle.makeModulesToInstall, "foo")
	data := android.AndroidMkDataForTest(t, ctx, bundle)
	var builder strings.Builder
	data.Custom(&builder, bundle.BaseModuleName(), "TARGET_", "", data)
	androidMk := builder.String()
	ensureContains(t, androidMk, "LOCAL_REQUIRED_MODULES := foo\n")
}

func TestApexOutputFileProducer(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		ref           string
		expected_data []string
	}{
		{
			name:          "test_using_output",
			ref:           ":myapex",
			expected_data: []string{"out/soong/.intermediates/myapex/android_common_myapex/myapex.capex:myapex.capex"},
		},
		{
			name:          "test_using_apex",
			ref:           ":myapex{.apex}",
			expected_data: []string{"out/soong/.intermediates/myapex/android_common_myapex/myapex.apex:myapex.apex"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testApex(t, `
					apex {
						name: "myapex",
						key: "myapex.key",
						compressible: true,
						updatable: false,
					}

					apex_key {
						name: "myapex.key",
						public_key: "testkey.avbpubkey",
						private_key: "testkey.pem",
					}

					java_test {
						name: "`+tc.name+`",
						srcs: ["a.java"],
						data: ["`+tc.ref+`"],
					}
				`,
				android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
					variables.CompressedApex = proptools.BoolPtr(true)
				}))
			javaTest := ctx.ModuleForTests(t, tc.name, "android_common").Module().(*java.Test)
			data := android.AndroidMkEntriesForTest(t, ctx, javaTest)[0].EntryMap["LOCAL_COMPATIBILITY_SUPPORT_FILES"]
			android.AssertStringPathsRelativeToTopEquals(t, "data", ctx.Config(), tc.expected_data, data)
		})
	}
}

func TestSdkLibraryCanHaveHigherMinSdkVersion(t *testing.T) {
	t.Parallel()
	preparer := android.GroupFixturePreparers(
		PrepareForTestWithApexBuildComponents,
		prepareForTestWithMyapex,
		java.PrepareForTestWithJavaSdkLibraryFiles,
		java.PrepareForTestWithJavaDefaultModules,
		android.PrepareForTestWithAndroidBuildComponents,
		dexpreopt.FixtureSetApexBootJars("myapex:mybootclasspathlib"),
		dexpreopt.FixtureSetApexSystemServerJars("myapex:mysystemserverclasspathlib"),
	)

	// Test java_sdk_library in bootclasspath_fragment may define higher min_sdk_version than the apex
	t.Run("bootclasspath_fragment jar has higher min_sdk_version than apex", func(t *testing.T) {
		t.Parallel()
		preparer.RunTestWithBp(t, `
			apex {
				name: "myapex",
				key: "myapex.key",
				bootclasspath_fragments: ["mybootclasspathfragment"],
				min_sdk_version: "30",
				updatable: false,
			}

			apex_key {
				name: "myapex.key",
				public_key: "testkey.avbpubkey",
				private_key: "testkey.pem",
			}

			bootclasspath_fragment {
				name: "mybootclasspathfragment",
				contents: ["mybootclasspathlib"],
				apex_available: ["myapex"],
				hidden_api: {
					split_packages: ["*"],
				},
			}

			java_sdk_library {
				name: "mybootclasspathlib",
				srcs: ["mybootclasspathlib.java"],
				apex_available: ["myapex"],
				compile_dex: true,
				unsafe_ignore_missing_latest_api: true,
				min_sdk_version: "31",
				static_libs: ["util"],
				sdk_version: "core_current",
			}

			java_library {
				name: "util",
                srcs: ["a.java"],
				apex_available: ["myapex"],
				min_sdk_version: "31",
				static_libs: ["another_util"],
				sdk_version: "core_current",
			}

			java_library {
				name: "another_util",
                srcs: ["a.java"],
				min_sdk_version: "31",
				apex_available: ["myapex"],
				sdk_version: "core_current",
			}
		`)
	})

	// Test java_sdk_library in systemserverclasspath_fragment may define higher min_sdk_version than the apex
	t.Run("systemserverclasspath_fragment jar has higher min_sdk_version than apex", func(t *testing.T) {
		t.Parallel()
		preparer.RunTestWithBp(t, `
			apex {
				name: "myapex",
				key: "myapex.key",
				systemserverclasspath_fragments: ["mysystemserverclasspathfragment"],
				min_sdk_version: "30",
				updatable: false,
			}

			apex_key {
				name: "myapex.key",
				public_key: "testkey.avbpubkey",
				private_key: "testkey.pem",
			}

			systemserverclasspath_fragment {
				name: "mysystemserverclasspathfragment",
				contents: ["mysystemserverclasspathlib"],
				apex_available: ["myapex"],
			}

			java_sdk_library {
				name: "mysystemserverclasspathlib",
				srcs: ["mysystemserverclasspathlib.java"],
				apex_available: ["myapex"],
				compile_dex: true,
				min_sdk_version: "32",
				unsafe_ignore_missing_latest_api: true,
				static_libs: ["util"],
			}

			java_library {
				name: "util",
                srcs: ["a.java"],
				apex_available: ["myapex"],
				min_sdk_version: "31",
				static_libs: ["another_util"],
			}

			java_library {
				name: "another_util",
                srcs: ["a.java"],
				min_sdk_version: "31",
				apex_available: ["myapex"],
			}
		`)
	})

	t.Run("bootclasspath_fragment jar must set min_sdk_version", func(t *testing.T) {
		t.Parallel()
		preparer.
			RunTestWithBp(t, `
				apex {
					name: "myapex",
					key: "myapex.key",
					bootclasspath_fragments: ["mybootclasspathfragment"],
					min_sdk_version: "30",
					updatable: false,
				}

				apex_key {
					name: "myapex.key",
					public_key: "testkey.avbpubkey",
					private_key: "testkey.pem",
				}

				bootclasspath_fragment {
					name: "mybootclasspathfragment",
					contents: ["mybootclasspathlib"],
					apex_available: ["myapex"],
					hidden_api: {
						split_packages: ["*"],
					},
				}

				java_sdk_library {
					name: "mybootclasspathlib",
					srcs: ["mybootclasspathlib.java"],
					apex_available: ["myapex"],
					compile_dex: true,
					unsafe_ignore_missing_latest_api: true,
					sdk_version: "current",
					min_sdk_version: "30",
				}
		`)
	})

	t.Run("systemserverclasspath_fragment jar must set min_sdk_version", func(t *testing.T) {
		t.Parallel()
		preparer.ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(`module "mysystemserverclasspathlib".*must set min_sdk_version`)).
			RunTestWithBp(t, `
				apex {
					name: "myapex",
					key: "myapex.key",
					systemserverclasspath_fragments: ["mysystemserverclasspathfragment"],
					min_sdk_version: "30",
					updatable: false,
				}

				apex_key {
					name: "myapex.key",
					public_key: "testkey.avbpubkey",
					private_key: "testkey.pem",
				}

				systemserverclasspath_fragment {
					name: "mysystemserverclasspathfragment",
					contents: ["mysystemserverclasspathlib"],
					apex_available: ["myapex"],
				}

				java_sdk_library {
					name: "mysystemserverclasspathlib",
					srcs: ["mysystemserverclasspathlib.java"],
					apex_available: ["myapex"],
					compile_dex: true,
					unsafe_ignore_missing_latest_api: true,
				}
		`)
	})
}

// Verifies that the APEX depends on all the Make modules in the list.
func ensureContainsRequiredDeps(t *testing.T, ctx *android.TestContext, moduleName, variant string, deps []string) {
	a := ctx.ModuleForTests(t, moduleName, variant).Module().(*apexBundle)
	for _, dep := range deps {
		android.AssertStringListContains(t, "", a.makeModulesToInstall, dep)
	}
}

// Verifies that the APEX does not depend on any of the Make modules in the list.
func ensureDoesNotContainRequiredDeps(t *testing.T, ctx *android.TestContext, moduleName, variant string, deps []string) {
	a := ctx.ModuleForTests(t, moduleName, variant).Module().(*apexBundle)
	for _, dep := range deps {
		android.AssertStringListDoesNotContain(t, "", a.makeModulesToInstall, dep)
	}
}

func TestApexStrictUpdtabilityLint(t *testing.T) {
	t.Parallel()
	bpTemplate := `
		apex {
			name: "myapex",
			key: "myapex.key",
			java_libs: ["myjavalib"],
			updatable: %v,
			min_sdk_version: "29",
		}
		apex_key {
			name: "myapex.key",
		}
		java_library {
			name: "myjavalib",
			srcs: ["MyClass.java"],
			apex_available: [ "myapex" ],
			lint: {
				strict_updatability_linting: %v,
				%s
			},
			sdk_version: "current",
			min_sdk_version: "29",
			compile_dex: true,
		}
		`
	fs := android.MockFS{
		"lint-baseline.xml": nil,
	}

	testCases := []struct {
		testCaseName                    string
		apexUpdatable                   bool
		javaStrictUpdtabilityLint       bool
		lintFileExists                  bool
		disallowedFlagExpectedOnApex    bool
		disallowedFlagExpectedOnJavalib bool
	}{
		{
			testCaseName:                    "lint-baseline.xml does not exist, no disallowed flag necessary in lint cmd",
			apexUpdatable:                   true,
			javaStrictUpdtabilityLint:       true,
			lintFileExists:                  false,
			disallowedFlagExpectedOnApex:    false,
			disallowedFlagExpectedOnJavalib: false,
		},
		{
			testCaseName:                    "non-updatable apex respects strict_updatability of javalib",
			apexUpdatable:                   false,
			javaStrictUpdtabilityLint:       false,
			lintFileExists:                  true,
			disallowedFlagExpectedOnApex:    false,
			disallowedFlagExpectedOnJavalib: false,
		},
		{
			testCaseName:                    "non-updatable apex respects strict updatability of javalib",
			apexUpdatable:                   false,
			javaStrictUpdtabilityLint:       true,
			lintFileExists:                  true,
			disallowedFlagExpectedOnApex:    false,
			disallowedFlagExpectedOnJavalib: true,
		},
		{
			testCaseName:                    "updatable apex checks strict updatability of javalib",
			apexUpdatable:                   true,
			javaStrictUpdtabilityLint:       false,
			lintFileExists:                  true,
			disallowedFlagExpectedOnApex:    true,
			disallowedFlagExpectedOnJavalib: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.testCaseName, func(t *testing.T) {
			t.Parallel()
			fixtures := []android.FixturePreparer{}
			baselineProperty := ""
			if testCase.lintFileExists {
				fixtures = append(fixtures, fs.AddToFixture())
				baselineProperty = "baseline_filename: \"lint-baseline.xml\""
			}
			bp := fmt.Sprintf(bpTemplate, testCase.apexUpdatable, testCase.javaStrictUpdtabilityLint, baselineProperty)

			result := testApex(t, bp, fixtures...)

			checkModule := func(m android.TestingBuildParams, name string, expectStrictUpdatability bool) {
				if expectStrictUpdatability {
					if m.Rule == nil {
						t.Errorf("expected strict updatability check rule on %s", name)
					} else {
						android.AssertStringDoesContain(t, fmt.Sprintf("strict updatability check rule for %s", name),
							m.RuleParams.Command, "--disallowed_issues NewApi")
						android.AssertStringListContains(t, fmt.Sprintf("strict updatability check baselines for %s", name),
							m.Inputs.Strings(), "lint-baseline.xml")
					}
				} else {
					if m.Rule != nil {
						t.Errorf("expected no strict updatability check rule on %s", name)
					}
				}
			}

			myjavalib := result.ModuleForTests(t, "myjavalib", "android_common_apex29")
			apex := result.ModuleForTests(t, "myapex", "android_common_myapex")
			apexStrictUpdatabilityCheck := apex.MaybeOutput("lint_strict_updatability_check.stamp")
			javalibStrictUpdatabilityCheck := myjavalib.MaybeOutput("lint_strict_updatability_check.stamp")

			checkModule(apexStrictUpdatabilityCheck, "myapex", testCase.disallowedFlagExpectedOnApex)
			checkModule(javalibStrictUpdatabilityCheck, "myjavalib", testCase.disallowedFlagExpectedOnJavalib)
		})
	}
}

// checks transtive deps of an apex coming from bootclasspath_fragment
func TestApexStrictUpdtabilityLintBcpFragmentDeps(t *testing.T) {
	t.Parallel()
	bp := `
		apex {
			name: "myapex",
			key: "myapex.key",
			bootclasspath_fragments: ["mybootclasspathfragment"],
			updatable: true,
			min_sdk_version: "29",
		}
		apex_key {
			name: "myapex.key",
		}
		bootclasspath_fragment {
			name: "mybootclasspathfragment",
			contents: ["myjavalib"],
			apex_available: ["myapex"],
			hidden_api: {
				split_packages: ["*"],
			},
		}
		java_library {
			name: "myjavalib",
			srcs: ["MyClass.java"],
			apex_available: [ "myapex" ],
			sdk_version: "current",
			min_sdk_version: "29",
			compile_dex: true,
			lint: {
				baseline_filename: "lint-baseline.xml",
			}
		}
		`
	fs := android.MockFS{
		"lint-baseline.xml": nil,
	}

	result := testApex(t, bp, dexpreopt.FixtureSetApexBootJars("myapex:myjavalib"), fs.AddToFixture())
	apex := result.ModuleForTests(t, "myapex", "android_common_myapex")
	apexStrictUpdatabilityCheck := apex.Output("lint_strict_updatability_check.stamp")
	android.AssertStringDoesContain(t, "strict updatability check rule for myapex",
		apexStrictUpdatabilityCheck.RuleParams.Command, "--disallowed_issues NewApi")
	android.AssertStringListContains(t, "strict updatability check baselines for myapex",
		apexStrictUpdatabilityCheck.Inputs.Strings(), "lint-baseline.xml")
}

func TestApexLintBcpFragmentSdkLibDeps(t *testing.T) {
	t.Parallel()
	bp := `
		apex {
			name: "myapex",
			key: "myapex.key",
			bootclasspath_fragments: ["mybootclasspathfragment"],
			min_sdk_version: "29",
			java_libs: [
				"jacocoagent",
			],
		}
		apex_key {
			name: "myapex.key",
		}
		bootclasspath_fragment {
			name: "mybootclasspathfragment",
			contents: ["foo"],
			apex_available: ["myapex"],
			hidden_api: {
				split_packages: ["*"],
			},
		}
		java_sdk_library {
			name: "foo",
			srcs: ["MyClass.java"],
			apex_available: [ "myapex" ],
			sdk_version: "current",
			min_sdk_version: "29",
			compile_dex: true,
		}
		`
	fs := android.MockFS{
		"lint-baseline.xml": nil,
	}

	result := android.GroupFixturePreparers(
		prepareForApexTest,
		java.PrepareForTestWithJavaSdkLibraryFiles,
		java.PrepareForTestWithJacocoInstrumentation,
		java.FixtureWithLastReleaseApis("foo"),
		android.FixtureMergeMockFs(fs),
	).RunTestWithBp(t, bp)

	myapex := result.ModuleForTests(t, "myapex", "android_common_myapex")
	lintReportInputs := strings.Join(myapex.Output("lint-report-xml.zip").Inputs.Strings(), " ")
	android.AssertStringDoesContain(t,
		"myapex lint report expected to contain that of the sdk library impl lib as an input",
		lintReportInputs, "foo.impl")
}

// updatable apexes should propagate updatable=true to its apps
func TestUpdatableApexEnforcesAppUpdatability(t *testing.T) {
	t.Parallel()
	bp := `
		apex {
			name: "myapex",
			key: "myapex.key",
			updatable: true,
			apps: [
				"myapp",
			],
			min_sdk_version: "30",
		}
		apex_key {
			name: "myapex.key",
		}
		android_app {
			name: "myapp",
			apex_available: [
				"myapex",
			],
			sdk_version: "current",
			min_sdk_version: "30",
		}
		`
	_ = android.GroupFixturePreparers(
		prepareForApexTest,
	).ExtendWithErrorHandler(android.FixtureExpectsOneErrorPattern("app dependency myapp must have updatable: true")).
		RunTestWithBp(t, bp)
}

func TestCannedFsConfig(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}`)
	mod := ctx.ModuleForTests(t, "myapex", "android_common_myapex")
	generateFsRule := mod.Rule("generateFsConfig")
	cmd := generateFsRule.RuleParams.Command

	ensureContains(t, cmd, `( echo '/ 1000 1000 0755'; echo '/apex_manifest.json 1000 1000 0644'; echo '/apex_manifest.pb 1000 1000 0644'; ) >`)
}

func TestCannedFsConfig_HasCustomConfig(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			canned_fs_config: "my_config",
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}`)
	mod := ctx.ModuleForTests(t, "myapex", "android_common_myapex")
	generateFsRule := mod.Rule("generateFsConfig")
	cmd := generateFsRule.RuleParams.Command

	// Ensure that canned_fs_config has "cat my_config" at the end
	ensureContains(t, cmd, `( echo '/ 1000 1000 0755'; echo '/apex_manifest.json 1000 1000 0644'; echo '/apex_manifest.pb 1000 1000 0644'; cat my_config ) >`)
}

func TestStubLibrariesMultipleApexViolation(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		desc          string
		hasStubs      bool
		apexAvailable string
		expectedError string
	}{
		{
			desc:          "non-stub library can have multiple apex_available",
			hasStubs:      false,
			apexAvailable: `["myapex", "otherapex"]`,
		},
		{
			desc:          "stub library should not be available to anyapex",
			hasStubs:      true,
			apexAvailable: `["//apex_available:anyapex"]`,
			expectedError: "Stub libraries should have a single apex_available.*anyapex",
		},
		{
			desc:          "stub library should not be available to multiple apexes",
			hasStubs:      true,
			apexAvailable: `["myapex", "otherapex"]`,
			expectedError: "Stub libraries should have a single apex_available.*myapex.*otherapex",
		},
		{
			desc:          "stub library can be available to a core apex and a test apex using apex_available_name",
			hasStubs:      true,
			apexAvailable: `["myapex"]`,
		},
	}
	bpTemplate := `
		cc_library {
			name: "libfoo",
			%v
			apex_available: %v,
		}
		apex {
			name: "myapex",
			key: "apex.key",
			updatable: false,
			native_shared_libs: ["libfoo"],
		}
		apex {
			name: "otherapex",
			key: "apex.key",
			updatable: false,
		}
		apex_test {
			name: "test_myapex",
			key: "apex.key",
			updatable: false,
			native_shared_libs: ["libfoo"],
			apex_available_name: "myapex",
		}
		apex_key {
			name: "apex.key",
		}
	`
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			stubs := ""
			if tc.hasStubs {
				stubs = `stubs: {symbol_file: "libfoo.map.txt"},`
			}
			bp := fmt.Sprintf(bpTemplate, stubs, tc.apexAvailable)
			mockFsFixturePreparer := android.FixtureModifyMockFS(func(fs android.MockFS) {
				fs["system/sepolicy/apex/test_myapex-file_contexts"] = nil
			})
			if tc.expectedError == "" {
				testApex(t, bp, mockFsFixturePreparer)
			} else {
				testApexError(t, tc.expectedError, bp, mockFsFixturePreparer)
			}
		})
	}
}

func TestFileSystemShouldSkipApexLibraries(t *testing.T) {
	t.Parallel()
	context := android.GroupFixturePreparers(
		android.PrepareForIntegrationTestWithAndroid,
		cc.PrepareForIntegrationTestWithCc,
		PrepareForTestWithApexBuildComponents,
		prepareForTestWithMyapex,
		filesystem.PrepareForTestWithFilesystemBuildComponents,
	)
	result := context.RunTestWithBp(t, `
		android_system_image {
			name: "myfilesystem",
			deps: [
				"libfoo",
			],
			linker_config: {
				gen_linker_config: true,
				linker_config_srcs: ["linker.config.json"],
			},
		}

		cc_library {
			name: "libfoo",
			shared_libs: [
				"libbar",
			],
			stl: "none",
		}

		cc_library {
			name: "libbar",
			stl: "none",
			apex_available: ["myapex"],
		}

		apex {
			name: "myapex",
			native_shared_libs: ["libbar"],
			key: "myapex.key",
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`)

	inputs := result.ModuleForTests(t, "myfilesystem", "android_common").Output("myfilesystem.img").Implicits
	android.AssertStringListDoesNotContain(t, "filesystem should not have libbar",
		inputs.Strings(),
		"out/soong/.intermediates/libbar/android_arm64_armv8-a_shared/libbar.so")
}

var apex_default_bp = `
		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		filegroup {
			name: "myapex.manifest",
			srcs: ["apex_manifest.json"],
		}

		filegroup {
			name: "myapex.androidmanifest",
			srcs: ["AndroidManifest.xml"],
		}
`

func TestAconfigFilesJavaDeps(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, apex_default_bp+`
		apex {
			name: "myapex",
			manifest: ":myapex.manifest",
			androidManifest: ":myapex.androidmanifest",
			key: "myapex.key",
			java_libs: [
				"my_java_library_foo",
				"my_java_library_bar",
			],
			updatable: false,
		}

		java_library {
			name: "my_java_library_foo",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			static_libs: ["my_java_aconfig_library_foo"],
			apex_available: [
				"myapex",
			],
			compile_dex: true,
		}

		java_library {
			name: "my_java_library_bar",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			static_libs: ["my_java_aconfig_library_bar"],
			apex_available: [
				"myapex",
			],
			compile_dex: true,
		}

		aconfig_declarations {
			name: "my_aconfig_declarations_foo",
			package: "com.example.package",
			container: "myapex",
			srcs: ["foo.aconfig"],
		}

		java_aconfig_library {
			name: "my_java_aconfig_library_foo",
			aconfig_declarations: "my_aconfig_declarations_foo",
			apex_available: [
				"myapex",
			],
		}

		aconfig_declarations {
			name: "my_aconfig_declarations_bar",
			package: "com.example.package",
			container: "myapex",
			srcs: ["bar.aconfig"],
		}

		java_aconfig_library {
			name: "my_java_aconfig_library_bar",
			aconfig_declarations: "my_aconfig_declarations_bar",
			apex_available: [
				"myapex",
			],
		}
	`)

	mod := ctx.ModuleForTests(t, "myapex", "android_common_myapex")
	s := mod.Rule("apexRule").Args["copy_commands"]
	copyCmds := regexp.MustCompile(" *&& *").Split(s, -1)
	if len(copyCmds) != 14 {
		t.Fatalf("Expected 14 commands, got %d in:\n%s", len(copyCmds), s)
	}

	ensureListContainsMatch(t, copyCmds, "^cp -f .*/aconfig_flags.pb .*/image.apex/etc/aconfig_flags.pb")
	ensureListContainsMatch(t, copyCmds, "^cp -f .*/package.map .*/image.apex/etc/package.map")
	ensureListContainsMatch(t, copyCmds, "^cp -f .*/flag.map .*/image.apex/etc/flag.map")
	ensureListContainsMatch(t, copyCmds, "^cp -f .*/flag.val .*/image.apex/etc/flag.val")
	ensureListContainsMatch(t, copyCmds, "^cp -f .*/flag.info.*/image.apex/etc/flag.info")

	inputs := []string{
		"my_aconfig_declarations_foo/intermediate.pb",
		"my_aconfig_declarations_bar/intermediate.pb",
	}
	VerifyAconfigRule(t, &mod, "combine_aconfig_declarations", inputs, "android_common_myapex/aconfig_flags.pb", "", "")
	VerifyAconfigRule(t, &mod, "create_aconfig_package_map_file", inputs, "android_common_myapex/package.map", "myapex", "package_map")
	VerifyAconfigRule(t, &mod, "create_aconfig_flag_map_file", inputs, "android_common_myapex/flag.map", "myapex", "flag_map")
	VerifyAconfigRule(t, &mod, "create_aconfig_flag_val_file", inputs, "android_common_myapex/flag.val", "myapex", "flag_val")
	VerifyAconfigRule(t, &mod, "create_aconfig_flag_info_file", inputs, "android_common_myapex/flag.info", "myapex", "flag_info")
}

func TestAconfigFilesJavaAndCcDeps(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, apex_default_bp+`
		apex {
			name: "myapex",
			manifest: ":myapex.manifest",
			androidManifest: ":myapex.androidmanifest",
			key: "myapex.key",
			java_libs: [
				"my_java_library_foo",
			],
			native_shared_libs: [
				"my_cc_library_bar",
			],
			binaries: [
				"my_cc_binary_baz",
			],
			updatable: false,
		}

		java_library {
			name: "my_java_library_foo",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			static_libs: ["my_java_aconfig_library_foo"],
			apex_available: [
				"myapex",
			],
			compile_dex: true,
		}

		cc_library {
			name: "my_cc_library_bar",
			srcs: ["foo/bar/MyClass.cc"],
			static_libs: [
				"my_cc_aconfig_library_bar",
				"my_cc_aconfig_library_baz",
			],
			apex_available: [
				"myapex",
			],
		}

		cc_binary {
			name: "my_cc_binary_baz",
			srcs: ["foo/bar/MyClass.cc"],
			static_libs: ["my_cc_aconfig_library_baz"],
			apex_available: [
				"myapex",
			],
		}

		aconfig_declarations {
			name: "my_aconfig_declarations_foo",
			package: "com.example.package",
			container: "myapex",
			srcs: ["foo.aconfig"],
		}

		java_aconfig_library {
			name: "my_java_aconfig_library_foo",
			aconfig_declarations: "my_aconfig_declarations_foo",
			apex_available: [
				"myapex",
			],
		}

		aconfig_declarations {
			name: "my_aconfig_declarations_bar",
			package: "com.example.package",
			container: "myapex",
			srcs: ["bar.aconfig"],
		}

		cc_aconfig_library {
			name: "my_cc_aconfig_library_bar",
			aconfig_declarations: "my_aconfig_declarations_bar",
			apex_available: [
				"myapex",
			],
		}

		aconfig_declarations {
			name: "my_aconfig_declarations_baz",
			package: "com.example.package",
			container: "myapex",
			srcs: ["baz.aconfig"],
		}

		cc_aconfig_library {
			name: "my_cc_aconfig_library_baz",
			aconfig_declarations: "my_aconfig_declarations_baz",
			apex_available: [
				"myapex",
			],
		}

		cc_library {
			name: "server_configurable_flags",
			srcs: ["server_configurable_flags.cc"],
		}
		cc_library {
			name: "libbase",
			srcs: ["libbase.cc"],
			apex_available: [
				"myapex",
			],
		}
		cc_library {
			name: "libaconfig_storage_read_api_cc",
			srcs: ["libaconfig_storage_read_api_cc.cc"],
		}
	`)

	mod := ctx.ModuleForTests(t, "myapex", "android_common_myapex")
	s := mod.Rule("apexRule").Args["copy_commands"]
	copyCmds := regexp.MustCompile(" *&& *").Split(s, -1)
	if len(copyCmds) != 18 {
		t.Fatalf("Expected 18 commands, got %d in:\n%s", len(copyCmds), s)
	}

	ensureListContainsMatch(t, copyCmds, "^cp -f .*/aconfig_flags.pb .*/image.apex/etc/aconfig_flags.pb")
	ensureListContainsMatch(t, copyCmds, "^cp -f .*/package.map .*/image.apex/etc/package.map")
	ensureListContainsMatch(t, copyCmds, "^cp -f .*/flag.map .*/image.apex/etc/flag.map")
	ensureListContainsMatch(t, copyCmds, "^cp -f .*/flag.val .*/image.apex/etc/flag.val")
	ensureListContainsMatch(t, copyCmds, "^cp -f .*/flag.info .*/image.apex/etc/flag.info")

	inputs := []string{
		"my_aconfig_declarations_foo/intermediate.pb",
		"my_cc_library_bar/android_arm64_armv8-a_shared_apex10000/myapex/aconfig_merged.pb",
		"my_aconfig_declarations_baz/intermediate.pb",
	}
	VerifyAconfigRule(t, &mod, "combine_aconfig_declarations", inputs, "android_common_myapex/aconfig_flags.pb", "", "")
	VerifyAconfigRule(t, &mod, "create_aconfig_package_map_file", inputs, "android_common_myapex/package.map", "myapex", "package_map")
	VerifyAconfigRule(t, &mod, "create_aconfig_flag_map_file", inputs, "android_common_myapex/flag.map", "myapex", "flag_map")
	VerifyAconfigRule(t, &mod, "create_aconfig_flag_val_file", inputs, "android_common_myapex/flag.val", "myapex", "flag_val")
	VerifyAconfigRule(t, &mod, "create_aconfig_flag_info_file", inputs, "android_common_myapex/flag.info", "myapex", "flag_info")
}

func TestAconfigFilesRustDeps(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, apex_default_bp+`
		apex {
			name: "myapex",
			manifest: ":myapex.manifest",
			androidManifest: ":myapex.androidmanifest",
			key: "myapex.key",
			native_shared_libs: [
				"libmy_rust_library",
			],
			binaries: [
				"my_rust_binary",
			],
			rust_dyn_libs: [
				"libmy_rust_dylib",
			],
			updatable: false,
		}

		rust_library {
			name: "liblazy_static", // test mock
			crate_name: "lazy_static",
			srcs: ["src/lib.rs"],
			apex_available: [
				"myapex",
			],
		}

		rust_library {
			name: "libaconfig_storage_read_api", // test mock
			crate_name: "aconfig_storage_read_api",
			srcs: ["src/lib.rs"],
			apex_available: [
				"myapex",
			],
		}

		rust_library {
			name: "liblogger", // test mock
			crate_name: "logger",
			srcs: ["src/lib.rs"],
			apex_available: [
				"myapex",
			],
		}

		rust_library {
			name: "liblog_rust", // test mock
			crate_name: "log_rust",
			srcs: ["src/lib.rs"],
			apex_available: [
				"myapex",
			],
		}

		rust_ffi_shared {
			name: "libmy_rust_library",
			srcs: ["src/lib.rs"],
			rustlibs: ["libmy_rust_aconfig_library_foo"],
			crate_name: "my_rust_library",
			apex_available: [
				"myapex",
			],
		}

		rust_library_dylib {
			name: "libmy_rust_dylib",
			srcs: ["foo/bar/MyClass.rs"],
			rustlibs: ["libmy_rust_aconfig_library_bar"],
			crate_name: "my_rust_dylib",
			apex_available: [
				"myapex",
			],
		}

		rust_binary {
			name: "my_rust_binary",
			srcs: ["foo/bar/MyClass.rs"],
			rustlibs: [
				"libmy_rust_aconfig_library_baz",
				"libmy_rust_dylib",
			],
			apex_available: [
				"myapex",
			],
		}

		aconfig_declarations {
			name: "my_aconfig_declarations_foo",
			package: "com.example.package",
			container: "myapex",
			srcs: ["foo.aconfig"],
		}

		aconfig_declarations {
			name: "my_aconfig_declarations_bar",
			package: "com.example.package",
			container: "myapex",
			srcs: ["bar.aconfig"],
		}

		aconfig_declarations {
			name: "my_aconfig_declarations_baz",
			package: "com.example.package",
			container: "myapex",
			srcs: ["baz.aconfig"],
		}

		rust_aconfig_library {
			name: "libmy_rust_aconfig_library_foo",
			aconfig_declarations: "my_aconfig_declarations_foo",
			crate_name: "my_rust_aconfig_library_foo",
			apex_available: [
				"myapex",
			],
		}

		rust_aconfig_library {
			name: "libmy_rust_aconfig_library_bar",
			aconfig_declarations: "my_aconfig_declarations_bar",
			crate_name: "my_rust_aconfig_library_bar",
			apex_available: [
				"myapex",
			],
		}

		rust_aconfig_library {
			name: "libmy_rust_aconfig_library_baz",
			aconfig_declarations: "my_aconfig_declarations_baz",
			crate_name: "my_rust_aconfig_library_baz",
			apex_available: [
				"myapex",
			],
		}
	`)

	mod := ctx.ModuleForTests(t, "myapex", "android_common_myapex")
	s := mod.Rule("apexRule").Args["copy_commands"]
	copyCmds := regexp.MustCompile(" *&& *").Split(s, -1)
	if len(copyCmds) != 32 {
		t.Fatalf("Expected 32 commands, got %d in:\n%s", len(copyCmds), s)
	}

	ensureListContainsMatch(t, copyCmds, "^cp -f .*/aconfig_flags.pb .*/image.apex/etc/aconfig_flags.pb")
	ensureListContainsMatch(t, copyCmds, "^cp -f .*/package.map .*/image.apex/etc/package.map")
	ensureListContainsMatch(t, copyCmds, "^cp -f .*/flag.map .*/image.apex/etc/flag.map")
	ensureListContainsMatch(t, copyCmds, "^cp -f .*/flag.val .*/image.apex/etc/flag.val")
	ensureListContainsMatch(t, copyCmds, "^cp -f .*/flag.info .*/image.apex/etc/flag.info")

	inputs := []string{
		"my_aconfig_declarations_foo/intermediate.pb",
		"my_aconfig_declarations_bar/intermediate.pb",
		"my_aconfig_declarations_baz/intermediate.pb",
		"my_rust_binary/android_arm64_armv8-a_apex10000/myapex/aconfig_merged.pb",
	}
	VerifyAconfigRule(t, &mod, "combine_aconfig_declarations", inputs, "android_common_myapex/aconfig_flags.pb", "", "")
	VerifyAconfigRule(t, &mod, "create_aconfig_package_map_file", inputs, "android_common_myapex/package.map", "myapex", "package_map")
	VerifyAconfigRule(t, &mod, "create_aconfig_flag_map_file", inputs, "android_common_myapex/flag.map", "myapex", "flag_map")
	VerifyAconfigRule(t, &mod, "create_aconfig_flag_val_file", inputs, "android_common_myapex/flag.val", "myapex", "flag_val")
	VerifyAconfigRule(t, &mod, "create_aconfig_flag_info_file", inputs, "android_common_myapex/flag.info", "myapex", "flag_info")
}

func VerifyAconfigRule(t *testing.T, mod *android.TestingModule, desc string, inputs []string, output string, container string, file_type string) {
	aconfigRule := mod.Description(desc)
	s := " " + aconfigRule.Args["cache_files"]
	aconfigArgs := regexp.MustCompile(" --cache ").Split(s, -1)[1:]
	if len(aconfigArgs) != len(inputs) {
		t.Fatalf("Expected %d commands, got %d in:\n%s", len(inputs), len(aconfigArgs), s)
	}

	ensureEquals(t, container, aconfigRule.Args["container"])
	ensureEquals(t, file_type, aconfigRule.Args["file_type"])

	buildParams := aconfigRule.BuildParams
	for _, input := range inputs {
		android.EnsureListContainsSuffix(t, aconfigArgs, input)
		android.EnsureListContainsSuffix(t, buildParams.Inputs.Strings(), input)
	}

	ensureContains(t, buildParams.Output.String(), output)
}

func TestAconfigFilesOnlyMatchCurrentApex(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, apex_default_bp+`
		apex {
			name: "myapex",
			manifest: ":myapex.manifest",
			androidManifest: ":myapex.androidmanifest",
			key: "myapex.key",
			java_libs: [
				"my_java_library_foo",
				"other_java_library_bar",
			],
			updatable: false,
		}

		java_library {
			name: "my_java_library_foo",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			static_libs: ["my_java_aconfig_library_foo"],
			apex_available: [
				"myapex",
			],
			compile_dex: true,
		}

		java_library {
			name: "other_java_library_bar",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			static_libs: ["other_java_aconfig_library_bar"],
			apex_available: [
				"myapex",
			],
			compile_dex: true,
		}

		aconfig_declarations {
			name: "my_aconfig_declarations_foo",
			package: "com.example.package",
			container: "myapex",
			srcs: ["foo.aconfig"],
		}

		java_aconfig_library {
			name: "my_java_aconfig_library_foo",
			aconfig_declarations: "my_aconfig_declarations_foo",
			apex_available: [
				"myapex",
			],
		}

		aconfig_declarations {
			name: "other_aconfig_declarations_bar",
			package: "com.example.package",
			container: "otherapex",
			srcs: ["bar.aconfig"],
		}

		java_aconfig_library {
			name: "other_java_aconfig_library_bar",
			aconfig_declarations: "other_aconfig_declarations_bar",
			apex_available: [
				"myapex",
			],
		}
	`)

	mod := ctx.ModuleForTests(t, "myapex", "android_common_myapex")
	combineAconfigRule := mod.Rule("All_aconfig_declarations_dump")
	s := " " + combineAconfigRule.Args["cache_files"]
	aconfigArgs := regexp.MustCompile(" --cache ").Split(s, -1)[1:]
	if len(aconfigArgs) != 1 {
		t.Fatalf("Expected 1 commands, got %d in:\n%s", len(aconfigArgs), s)
	}
	android.EnsureListContainsSuffix(t, aconfigArgs, "my_aconfig_declarations_foo/intermediate.pb")

	buildParams := combineAconfigRule.BuildParams
	if len(buildParams.Inputs) != 1 {
		t.Fatalf("Expected 1 input, got %d", len(buildParams.Inputs))
	}
	android.EnsureListContainsSuffix(t, buildParams.Inputs.Strings(), "my_aconfig_declarations_foo/intermediate.pb")
	ensureContains(t, buildParams.Output.String(), "android_common_myapex/aconfig_flags.pb")
}

func TestAconfigFilesRemoveDuplicates(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, apex_default_bp+`
		apex {
			name: "myapex",
			manifest: ":myapex.manifest",
			androidManifest: ":myapex.androidmanifest",
			key: "myapex.key",
			java_libs: [
				"my_java_library_foo",
				"my_java_library_bar",
			],
			updatable: false,
		}

		java_library {
			name: "my_java_library_foo",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			static_libs: ["my_java_aconfig_library_foo"],
			apex_available: [
				"myapex",
			],
			compile_dex: true,
		}

		java_library {
			name: "my_java_library_bar",
			srcs: ["foo/bar/MyClass.java"],
			sdk_version: "none",
			system_modules: "none",
			static_libs: ["my_java_aconfig_library_bar"],
			apex_available: [
				"myapex",
			],
			compile_dex: true,
		}

		aconfig_declarations {
			name: "my_aconfig_declarations_foo",
			package: "com.example.package",
			container: "myapex",
			srcs: ["foo.aconfig"],
		}

		java_aconfig_library {
			name: "my_java_aconfig_library_foo",
			aconfig_declarations: "my_aconfig_declarations_foo",
			apex_available: [
				"myapex",
			],
		}

		java_aconfig_library {
			name: "my_java_aconfig_library_bar",
			aconfig_declarations: "my_aconfig_declarations_foo",
			apex_available: [
				"myapex",
			],
		}
	`)

	mod := ctx.ModuleForTests(t, "myapex", "android_common_myapex")
	combineAconfigRule := mod.Rule("All_aconfig_declarations_dump")
	s := " " + combineAconfigRule.Args["cache_files"]
	aconfigArgs := regexp.MustCompile(" --cache ").Split(s, -1)[1:]
	if len(aconfigArgs) != 1 {
		t.Fatalf("Expected 1 commands, got %d in:\n%s", len(aconfigArgs), s)
	}
	android.EnsureListContainsSuffix(t, aconfigArgs, "my_aconfig_declarations_foo/intermediate.pb")

	buildParams := combineAconfigRule.BuildParams
	if len(buildParams.Inputs) != 1 {
		t.Fatalf("Expected 1 input, got %d", len(buildParams.Inputs))
	}
	android.EnsureListContainsSuffix(t, buildParams.Inputs.Strings(), "my_aconfig_declarations_foo/intermediate.pb")
	ensureContains(t, buildParams.Output.String(), "android_common_myapex/aconfig_flags.pb")
}

// Test that the boot jars come from the _selected_ apex prebuilt
// RELEASE_APEX_CONTIRBUTIONS_* build flags will be used to select the correct prebuilt for a specific release config
func TestBootDexJarsMultipleApexPrebuilts(t *testing.T) {
	t.Parallel()
	checkBootDexJarPath := func(t *testing.T, ctx *android.TestContext, stem string, bootDexJarPath string) {
		t.Helper()
		s := ctx.ModuleForTests(t, "dex_bootjars", "android_common")
		foundLibfooJar := false
		base := stem + ".jar"
		for _, output := range s.AllOutputs() {
			if filepath.Base(output) == base {
				foundLibfooJar = true
				buildRule := s.Output(output)
				android.AssertStringEquals(t, "boot dex jar path", bootDexJarPath, buildRule.Input.String())
			}
		}
		if !foundLibfooJar {
			t.Errorf("Rule for libfoo.jar missing in dex_bootjars singleton outputs %q", android.StringPathsRelativeToTop(ctx.Config().SoongOutDir(), s.AllOutputs()))
		}
	}

	// Check that the boot jars of the selected apex are run through boot_jars_package_check
	// This validates that the jars on the bootclasspath do not contain packages outside an allowlist
	checkBootJarsPackageCheck := func(t *testing.T, ctx *android.TestContext, expectedBootJar string) {
		platformBcp := ctx.ModuleForTests(t, "platform-bootclasspath", "android_common")
		bootJarsCheckRule := platformBcp.Rule("boot_jars_package_check")
		android.AssertStringMatches(t, "Could not find the correct boot dex jar in package check rule", bootJarsCheckRule.RuleParams.Command, "build/soong/scripts/check_boot_jars/package_allowed_list.txt.*"+expectedBootJar)
	}

	// Check that the boot jars used to generate the monolithic hiddenapi flags come from the selected apex
	checkBootJarsForMonolithicHiddenapi := func(t *testing.T, ctx *android.TestContext, expectedBootJar string) {
		monolithicHiddenapiFlagsCmd := ctx.ModuleForTests(t, "platform-bootclasspath", "android_common").Output("out/soong/hiddenapi/hiddenapi-stub-flags.txt").RuleParams.Command
		android.AssertStringMatches(t, "Could not find the correct boot dex jar in monolithic hiddenapi flags generation command", monolithicHiddenapiFlagsCmd, "--boot-dex="+expectedBootJar)
	}

	bp := `
		// Source APEX.

		java_library {
			name: "framework-foo",
			srcs: ["foo.java"],
			installable: true,
			apex_available: [
				"com.android.foo",
			],
		}

		bootclasspath_fragment {
			name: "foo-bootclasspath-fragment",
			contents: ["framework-foo"],
			apex_available: [
				"com.android.foo",
			],
			hidden_api: {
				split_packages: ["*"],
			},
		}

		apex_key {
			name: "com.android.foo.key",
			public_key: "com.android.foo.avbpubkey",
			private_key: "com.android.foo.pem",
		}

		apex {
			name: "com.android.foo",
			key: "com.android.foo.key",
			bootclasspath_fragments: ["foo-bootclasspath-fragment"],
			updatable: false,
		}

		// Prebuilt APEX.

		java_sdk_library_import {
			name: "framework-foo",
			public: {
				jars: ["foo.jar"],
			},
			apex_available: ["com.android.foo"],
			shared_library: false,
		}

		prebuilt_bootclasspath_fragment {
			name: "foo-bootclasspath-fragment",
			contents: ["framework-foo"],
			hidden_api: {
				annotation_flags: "my-bootclasspath-fragment/annotation-flags.csv",
				metadata: "my-bootclasspath-fragment/metadata.csv",
				index: "my-bootclasspath-fragment/index.csv",
				stub_flags: "my-bootclasspath-fragment/stub-flags.csv",
				all_flags: "my-bootclasspath-fragment/all-flags.csv",
			},
			apex_available: [
				"com.android.foo",
			],
		}

		prebuilt_apex {
			name: "com.android.foo",
			apex_name: "com.android.foo",
			src: "com.android.foo-arm.apex",
			exported_bootclasspath_fragments: ["foo-bootclasspath-fragment"],
		}

		// Another Prebuilt ART APEX
		prebuilt_apex {
			name: "com.android.foo.v2",
			apex_name: "com.android.foo", // Used to determine the API domain
			src: "com.android.foo-arm.apex",
			exported_bootclasspath_fragments: ["foo-bootclasspath-fragment"],
		}

		// APEX contribution modules

		apex_contributions {
			name: "foo.source.contributions",
			api_domain: "com.android.foo",
			contents: ["com.android.foo"],
		}

		apex_contributions {
			name: "foo.prebuilt.contributions",
			api_domain: "com.android.foo",
			contents: ["prebuilt_com.android.foo"],
		}

		apex_contributions {
			name: "foo.prebuilt.v2.contributions",
			api_domain: "com.android.foo",
			contents: ["com.android.foo.v2"], // prebuilt_ prefix is missing because of prebuilt_rename mutator
		}
	`

	testCases := []struct {
		desc                      string
		selectedApexContributions string
		expectedBootJar           string
	}{
		{
			desc:                      "Source apex com.android.foo is selected, bootjar should come from source java library",
			selectedApexContributions: "foo.source.contributions",
			expectedBootJar:           "out/soong/.intermediates/foo-bootclasspath-fragment/android_common_com.android.foo/hiddenapi-modular/encoded/framework-foo.jar",
		},
		{
			desc:                      "Prebuilt apex prebuilt_com.android.foo is selected, profile should come from .prof deapexed from the prebuilt",
			selectedApexContributions: "foo.prebuilt.contributions",
			expectedBootJar:           "out/soong/.intermediates/prebuilt_com.android.foo/android_common_prebuilt_com.android.foo/deapexer/javalib/framework-foo.jar",
		},
		{
			desc:                      "Prebuilt apex prebuilt_com.android.foo.v2 is selected, profile should come from .prof deapexed from the prebuilt",
			selectedApexContributions: "foo.prebuilt.v2.contributions",
			expectedBootJar:           "out/soong/.intermediates/com.android.foo.v2/android_common_prebuilt_com.android.foo/deapexer/javalib/framework-foo.jar",
		},
	}

	fragment := java.ApexVariantReference{
		Apex:   proptools.StringPtr("com.android.foo"),
		Module: proptools.StringPtr("foo-bootclasspath-fragment"),
	}

	for _, tc := range testCases {
		preparer := android.GroupFixturePreparers(
			java.FixtureConfigureApexBootJars("com.android.foo:framework-foo"),
			android.FixtureMergeMockFs(map[string][]byte{
				"system/sepolicy/apex/com.android.foo-file_contexts": nil,
			}),
			// Make sure that we have atleast one platform library so that we can check the monolithic hiddenapi
			// file creation.
			java.FixtureConfigureBootJars("platform:foo"),
			android.FixtureModifyMockFS(func(fs android.MockFS) {
				fs["platform/Android.bp"] = []byte(`
		java_library {
			name: "foo",
			srcs: ["Test.java"],
			compile_dex: true,
		}
		`)
				fs["platform/Test.java"] = nil
			}),

			android.PrepareForTestWithBuildFlag("RELEASE_APEX_CONTRIBUTIONS_ADSERVICES", tc.selectedApexContributions),
		)
		ctx := testDexpreoptWithApexes(t, bp, "", preparer, fragment)
		checkBootDexJarPath(t, ctx, "framework-foo", tc.expectedBootJar)
		checkBootJarsPackageCheck(t, ctx, tc.expectedBootJar)
		checkBootJarsForMonolithicHiddenapi(t, ctx, tc.expectedBootJar)
	}
}

// Test that product packaging installs the selected mainline module (either source or a specific prebuilt)
// RELEASE_APEX_CONTIRBUTIONS_* build flags will be used to select the correct prebuilt for a specific release config
func TestInstallationRulesForMultipleApexPrebuilts(t *testing.T) {
	t.Parallel()
	// for a mainline module family, check that only the flagged soong module is visible to make
	checkHideFromMake := func(t *testing.T, ctx *android.TestContext, visibleModuleName string, hiddenModuleNames []string) {
		variation := func(moduleName string) string {
			ret := "android_common_prebuilt_com.android.foo"
			if moduleName == "com.google.android.foo" {
				ret = "android_common_com.google.android.foo"
			}
			return ret
		}

		visibleModule := ctx.ModuleForTests(t, visibleModuleName, variation(visibleModuleName)).Module()
		android.AssertBoolEquals(t, "Apex "+visibleModuleName+" selected using apex_contributions should be visible to make", false, visibleModule.IsHideFromMake())

		for _, hiddenModuleName := range hiddenModuleNames {
			hiddenModule := ctx.ModuleForTests(t, hiddenModuleName, variation(hiddenModuleName)).Module()
			android.AssertBoolEquals(t, "Apex "+hiddenModuleName+" not selected using apex_contributions should be hidden from make", true, hiddenModule.IsHideFromMake())

		}
	}

	bp := `
		apex_key {
			name: "com.android.foo.key",
			public_key: "com.android.foo.avbpubkey",
			private_key: "com.android.foo.pem",
		}

		// AOSP source apex
		apex {
			name: "com.android.foo",
			key: "com.android.foo.key",
			updatable: false,
		}

		// Google source apex
		override_apex {
			name: "com.google.android.foo",
			base: "com.android.foo",
			key: "com.android.foo.key",
		}

		// Prebuilt Google APEX.

		prebuilt_apex {
			name: "com.google.android.foo",
			apex_name: "com.android.foo",
			src: "com.android.foo-arm.apex",
			prefer: true, // prefer is set to true on both the prebuilts to induce an error if flagging is not present
		}

		// Another Prebuilt Google APEX
		prebuilt_apex {
			name: "com.google.android.foo.v2",
			apex_name: "com.android.foo",
			source_apex_name: "com.google.android.foo",
			src: "com.android.foo-arm.apex",
			prefer: true, // prefer is set to true on both the prebuilts to induce an error if flagging is not present
		}

		// APEX contribution modules

		apex_contributions {
			name: "foo.source.contributions",
			api_domain: "com.android.foo",
			contents: ["com.google.android.foo"],
		}

		apex_contributions {
			name: "foo.prebuilt.contributions",
			api_domain: "com.android.foo",
			contents: ["prebuilt_com.google.android.foo"],
		}

		apex_contributions {
			name: "foo.prebuilt.v2.contributions",
			api_domain: "com.android.foo",
			contents: ["prebuilt_com.google.android.foo.v2"],
		}

		// This is an incompatible module because it selects multiple versions of the same mainline module
		apex_contributions {
			name: "foo.prebuilt.duplicate.contributions",
			api_domain: "com.android.foo",
			contents: [
			    "prebuilt_com.google.android.foo",
			    "prebuilt_com.google.android.foo.v2",
			],
		}
	`

	testCases := []struct {
		desc                      string
		selectedApexContributions string
		expectedVisibleModuleName string
		expectedHiddenModuleNames []string
		expectedError             string
	}{
		{
			desc:                      "Source apex is selected, prebuilts should be hidden from make",
			selectedApexContributions: "foo.source.contributions",
			expectedVisibleModuleName: "com.google.android.foo",
			expectedHiddenModuleNames: []string{"prebuilt_com.google.android.foo", "prebuilt_com.google.android.foo.v2"},
		},
		{
			desc:                      "Prebuilt apex prebuilt_com.android.foo is selected, source and the other prebuilt should be hidden from make",
			selectedApexContributions: "foo.prebuilt.contributions",
			expectedVisibleModuleName: "prebuilt_com.google.android.foo",
			expectedHiddenModuleNames: []string{"com.google.android.foo", "prebuilt_com.google.android.foo.v2"},
		},
		{
			desc:                      "Prebuilt apex prebuilt_com.android.fooi.v2 is selected, source and the other prebuilt should be hidden from make",
			selectedApexContributions: "foo.prebuilt.v2.contributions",
			expectedVisibleModuleName: "prebuilt_com.google.android.foo.v2",
			expectedHiddenModuleNames: []string{"com.google.android.foo", "prebuilt_com.google.android.foo"},
		},
		{
			desc:                      "Multiple versions of a prebuilt apex is selected in the same release config",
			selectedApexContributions: "foo.prebuilt.duplicate.contributions",
			expectedError:             "Found duplicate variations of the same module in apex_contributions: prebuilt_com.google.android.foo and prebuilt_com.google.android.foo.v2",
		},
	}

	for _, tc := range testCases {
		preparer := android.GroupFixturePreparers(
			android.FixtureMergeMockFs(map[string][]byte{
				"system/sepolicy/apex/com.android.foo-file_contexts": nil,
			}),
			android.PrepareForTestWithBuildFlag("RELEASE_APEX_CONTRIBUTIONS_ADSERVICES", tc.selectedApexContributions),
		)
		if tc.expectedError != "" {
			preparer = preparer.ExtendWithErrorHandler(android.FixtureExpectsOneErrorPattern(tc.expectedError))
			testApex(t, bp, preparer)
			return
		}
		ctx := testApex(t, bp, preparer)

		// Check that
		// 1. The contents of the selected apex_contributions are visible to make
		// 2. The rest of the apexes in the mainline module family (source or other prebuilt) is hidden from make
		checkHideFromMake(t, ctx, tc.expectedVisibleModuleName, tc.expectedHiddenModuleNames)

		// Check that source_apex_name is written as LOCAL_MODULE for make packaging
		if tc.expectedVisibleModuleName == "prebuilt_com.google.android.foo.v2" {
			apex := ctx.ModuleForTests(t, "prebuilt_com.google.android.foo.v2", "android_common_prebuilt_com.android.foo").Module()
			entries := android.AndroidMkEntriesForTest(t, ctx, apex)[0]

			expected := "com.google.android.foo"
			actual := entries.EntryMap["LOCAL_MODULE"][0]
			android.AssertStringEquals(t, "LOCAL_MODULE", expected, actual)
		}
	}
}

// Test that product packaging installs the selected mainline module in workspaces withtout source mainline module
func TestInstallationRulesForMultipleApexPrebuiltsWithoutSource(t *testing.T) {
	t.Parallel()
	// for a mainline module family, check that only the flagged soong module is visible to make
	checkHideFromMake := func(t *testing.T, ctx *android.TestContext, visibleModuleNames []string, hiddenModuleNames []string) {
		variation := func(moduleName string) string {
			ret := "android_common_com.android.adservices"
			if moduleName == "com.google.android.adservices" || moduleName == "com.google.android.adservices.v2" {
				ret = "android_common_prebuilt_com.android.adservices"
			}
			return ret
		}

		for _, visibleModuleName := range visibleModuleNames {
			visibleModule := ctx.ModuleForTests(t, visibleModuleName, variation(visibleModuleName)).Module()
			android.AssertBoolEquals(t, "Apex "+visibleModuleName+" selected using apex_contributions should be visible to make", false, visibleModule.IsHideFromMake())
		}

		for _, hiddenModuleName := range hiddenModuleNames {
			hiddenModule := ctx.ModuleForTests(t, hiddenModuleName, variation(hiddenModuleName)).Module()
			android.AssertBoolEquals(t, "Apex "+hiddenModuleName+" not selected using apex_contributions should be hidden from make", true, hiddenModule.IsHideFromMake())

		}
	}

	bp := `
		apex_key {
			name: "com.android.adservices.key",
			public_key: "com.android.adservices.avbpubkey",
			private_key: "com.android.adservices.pem",
		}

		// AOSP source apex
		apex {
			name: "com.android.adservices",
			key: "com.android.adservices.key",
			updatable: false,
		}

		// Prebuilt Google APEX.

		prebuilt_apex {
			name: "com.google.android.adservices",
			apex_name: "com.android.adservices",
			src: "com.android.foo-arm.apex",
		}

		// Another Prebuilt Google APEX
		prebuilt_apex {
			name: "com.google.android.adservices.v2",
			apex_name: "com.android.adservices",
			src: "com.android.foo-arm.apex",
		}

		// APEX contribution modules


		apex_contributions {
			name: "adservices.prebuilt.contributions",
			api_domain: "com.android.adservices",
			contents: ["prebuilt_com.google.android.adservices"],
		}

		apex_contributions {
			name: "adservices.prebuilt.v2.contributions",
			api_domain: "com.android.adservices",
			contents: ["prebuilt_com.google.android.adservices.v2"],
		}
	`

	testCases := []struct {
		desc                       string
		selectedApexContributions  string
		expectedVisibleModuleNames []string
		expectedHiddenModuleNames  []string
	}{
		{
			desc:                       "No apex contributions selected, source aosp apex should be visible, and mainline prebuilts should be hidden",
			selectedApexContributions:  "",
			expectedVisibleModuleNames: []string{"com.android.adservices"},
			expectedHiddenModuleNames:  []string{"com.google.android.adservices", "com.google.android.adservices.v2"},
		},
		{
			desc:                       "Prebuilt apex prebuilt_com.android.adservices is selected",
			selectedApexContributions:  "adservices.prebuilt.contributions",
			expectedVisibleModuleNames: []string{"com.android.adservices", "com.google.android.adservices"},
			expectedHiddenModuleNames:  []string{"com.google.android.adservices.v2"},
		},
		{
			desc:                       "Prebuilt apex prebuilt_com.android.adservices.v2 is selected",
			selectedApexContributions:  "adservices.prebuilt.v2.contributions",
			expectedVisibleModuleNames: []string{"com.android.adservices", "com.google.android.adservices.v2"},
			expectedHiddenModuleNames:  []string{"com.google.android.adservices"},
		},
	}

	for _, tc := range testCases {
		preparer := android.GroupFixturePreparers(
			android.FixtureMergeMockFs(map[string][]byte{
				"system/sepolicy/apex/com.android.adservices-file_contexts": nil,
			}),
			android.PrepareForTestWithBuildFlag("RELEASE_APEX_CONTRIBUTIONS_ADSERVICES", tc.selectedApexContributions),
		)
		ctx := testApex(t, bp, preparer)

		checkHideFromMake(t, ctx, tc.expectedVisibleModuleNames, tc.expectedHiddenModuleNames)
	}
}

func TestAconfifDeclarationsValidation(t *testing.T) {
	t.Parallel()
	aconfigDeclarationLibraryString := func(moduleNames []string) (ret string) {
		for _, moduleName := range moduleNames {
			ret += fmt.Sprintf(`
			aconfig_declarations {
				name: "%[1]s",
				package: "com.example.package",
				container: "system",
				srcs: [
					"%[1]s.aconfig",
				],
			}
			java_aconfig_library {
				name: "%[1]s-lib",
				aconfig_declarations: "%[1]s",
			}
			`, moduleName)
		}
		return ret
	}

	result := android.GroupFixturePreparers(
		prepareForApexTest,
		java.PrepareForTestWithJavaSdkLibraryFiles,
		java.FixtureWithLastReleaseApis("foo"),
	).RunTestWithBp(t, `
		java_library {
			name: "baz-java-lib",
			static_libs: [
				"baz-lib",
			],
		}
		filegroup {
			name: "qux-filegroup",
			device_common_srcs: [
				":qux-lib{.generated_srcjars}",
			],
		}
		filegroup {
			name: "qux-another-filegroup",
			srcs: [
				":qux-filegroup",
			],
		}
		java_library {
			name: "quux-java-lib",
			srcs: [
				"a.java",
			],
			libs: [
				"quux-lib",
			],
		}
		java_sdk_library {
			name: "foo",
			srcs: [
				":qux-another-filegroup",
			],
			api_packages: ["foo"],
			system: {
				enabled: true,
			},
			module_lib: {
				enabled: true,
			},
			test: {
				enabled: true,
			},
			static_libs: [
				"bar-lib",
			],
			libs: [
				"baz-java-lib",
				"quux-java-lib",
			],
			aconfig_declarations: [
				"bar",
			],
		}
	`+aconfigDeclarationLibraryString([]string{"bar", "baz", "qux", "quux"}))

	m := result.ModuleForTests(t, "foo.stubs.source", "android_common")
	outDir := "out/soong/.intermediates"

	// Arguments passed to aconfig to retrieve the state of the flags defined in the
	// textproto files
	aconfigFlagArgs := m.Output("released-flags-exportable.pb").Args["flags_path"]

	// "bar-lib" is a static_lib of "foo" and is passed to metalava as classpath. Thus the
	// cache file provided by the associated aconfig_declarations module "bar" should be passed
	// to aconfig.
	android.AssertStringDoesContain(t, "cache file of a java_aconfig_library static_lib "+
		"passed as an input",
		aconfigFlagArgs, fmt.Sprintf("%s/%s/intermediate.pb", outDir, "bar"))

	// "baz-java-lib", which statically depends on "baz-lib", is a lib of "foo" and is passed
	// to metalava as classpath. Thus the cache file provided by the associated
	// aconfig_declarations module "baz" should be passed to aconfig.
	android.AssertStringDoesContain(t, "cache file of a lib that statically depends on "+
		"java_aconfig_library passed as an input",
		aconfigFlagArgs, fmt.Sprintf("%s/%s/intermediate.pb", outDir, "baz"))

	// "qux-lib" is passed to metalava as src via the filegroup, thus the cache file provided by
	// the associated aconfig_declarations module "qux" should be passed to aconfig.
	android.AssertStringDoesContain(t, "cache file of srcs java_aconfig_library passed as an "+
		"input",
		aconfigFlagArgs, fmt.Sprintf("%s/%s/intermediate.pb", outDir, "qux"))

	// "quux-java-lib" is a lib of "foo" and is passed to metalava as classpath, but does not
	// statically depend on "quux-lib". Therefore, the cache file provided by the associated
	// aconfig_declarations module "quux" should not be passed to aconfig.
	android.AssertStringDoesNotContain(t, "cache file of a lib that does not statically "+
		"depend on java_aconfig_library not passed as an input",
		aconfigFlagArgs, fmt.Sprintf("%s/%s/intermediate.pb", outDir, "quux"))
}

func TestMultiplePrebuiltsWithSameBase(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			prebuilts: ["myetc", "myetc2"],
			min_sdk_version: "29",
		}
		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		prebuilt_etc {
			name: "myetc",
			src: "myprebuilt",
			filename: "myfilename",
		}
		prebuilt_etc {
			name: "myetc2",
			sub_dir: "mysubdir",
			src: "myprebuilt",
			filename: "myfilename",
		}
	`, withFiles(android.MockFS{
		"packages/modules/common/build/allowed_deps.txt": nil,
	}))

	ab := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Module().(*apexBundle)
	data := android.AndroidMkDataForTest(t, ctx, ab)
	var builder strings.Builder
	data.Custom(&builder, ab.BaseModuleName(), "TARGET_", "", data)
	androidMk := builder.String()

	android.AssertStringDoesContain(t, "not found", androidMk, "LOCAL_MODULE := etc_myfilename.myapex")
	android.AssertStringDoesContain(t, "not found", androidMk, "LOCAL_MODULE := etc_mysubdir_myfilename.myapex")
}

func TestApexMinSdkVersionOverride(t *testing.T) {
	t.Parallel()
	checkMinSdkVersion := func(t *testing.T, module android.TestingModule, expectedMinSdkVersion string) {
		args := module.Rule("apexRule").Args
		optFlags := args["opt_flags"]
		if !strings.Contains(optFlags, "--min_sdk_version "+expectedMinSdkVersion) {
			t.Errorf("%s: Expected min_sdk_version=%s, got: %s", module.Module(), expectedMinSdkVersion, optFlags)
		}
	}

	checkHasDep := func(t *testing.T, ctx *android.TestContext, m android.Module, wantDep android.Module) {
		t.Helper()
		found := false
		ctx.VisitDirectDeps(m, func(dep blueprint.Module) {
			if dep == wantDep {
				found = true
			}
		})
		if !found {
			t.Errorf("Could not find a dependency from %v to %v\n", m, wantDep)
		}
	}

	ctx := testApex(t, `
		apex {
			name: "com.android.apex30",
			min_sdk_version: "30",
			key: "apex30.key",
			java_libs: ["javalib"],
		}

		java_library {
			name: "javalib",
			srcs: ["A.java"],
			apex_available: ["com.android.apex30"],
			min_sdk_version: "30",
			sdk_version: "current",
			compile_dex: true,
		}

		override_apex {
			name: "com.mycompany.android.apex30",
			base: "com.android.apex30",
		}

		override_apex {
			name: "com.mycompany.android.apex31",
			base: "com.android.apex30",
			min_sdk_version: "31",
		}

		apex_key {
			name: "apex30.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

	`, android.FixtureMergeMockFs(android.MockFS{
		"system/sepolicy/apex/com.android.apex30-file_contexts": nil,
	}),
	)

	baseModule := ctx.ModuleForTests(t, "com.android.apex30", "android_common_com.android.apex30")
	checkMinSdkVersion(t, baseModule, "30")

	// Override module, but uses same min_sdk_version
	overridingModuleSameMinSdkVersion := ctx.ModuleForTests(t, "com.android.apex30", "android_common_com.mycompany.android.apex30_com.mycompany.android.apex30")
	javalibApex30Variant := ctx.ModuleForTests(t, "javalib", "android_common_apex30")
	checkMinSdkVersion(t, overridingModuleSameMinSdkVersion, "30")
	checkHasDep(t, ctx, overridingModuleSameMinSdkVersion.Module(), javalibApex30Variant.Module())

	// Override module, uses different min_sdk_version
	overridingModuleDifferentMinSdkVersion := ctx.ModuleForTests(t, "com.android.apex30", "android_common_com.mycompany.android.apex31_com.mycompany.android.apex31")
	javalibApex31Variant := ctx.ModuleForTests(t, "javalib", "android_common_apex31")
	checkMinSdkVersion(t, overridingModuleDifferentMinSdkVersion, "31")
	checkHasDep(t, ctx, overridingModuleDifferentMinSdkVersion.Module(), javalibApex31Variant.Module())
}

func TestOverrideApexWithPrebuiltApexPreferred(t *testing.T) {
	t.Parallel()
	context := android.GroupFixturePreparers(
		android.PrepareForIntegrationTestWithAndroid,
		PrepareForTestWithApexBuildComponents,
		android.FixtureMergeMockFs(android.MockFS{
			"system/sepolicy/apex/foo-file_contexts": nil,
		}),
	)
	res := context.RunTestWithBp(t, `
		apex {
			name: "foo",
			key: "myapex.key",
			apex_available_name: "com.android.foo",
			variant_version: "0",
			updatable: false,
		}
		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
		prebuilt_apex {
			name: "foo",
			src: "foo.apex",
			prefer: true,
		}
		override_apex {
			name: "myoverrideapex",
			base: "foo",
		}
	`)

	java.CheckModuleHasDependency(t, res.TestContext, "myoverrideapex", "android_common_myoverrideapex", "foo")
}

func TestUpdatableApexMinSdkVersionCurrent(t *testing.T) {
	t.Parallel()
	testApexError(t, `"myapex" .*: updatable: updatable APEXes should not set min_sdk_version to current. Please use a finalized API level or a recognized in-development codename`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			updatable: true,
			min_sdk_version: "current",
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`)
}

func TestPrebuiltStubNoinstall(t *testing.T) {
	t.Parallel()
	testFunc := func(t *testing.T, expectLibfooOnSystemLib bool, fs android.MockFS) {
		result := android.GroupFixturePreparers(
			prepareForApexTest,
			android.PrepareForTestWithAndroidMk,
			android.PrepareForTestWithMakevars,
			android.FixtureMergeMockFs(fs),
		).RunTest(t)

		ldRule := result.ModuleForTests(t, "installedlib", "android_arm64_armv8-a_shared").Rule("ld")
		android.AssertStringDoesContain(t, "", ldRule.Args["libFlags"], "android_arm64_armv8-a_shared_current/libfoo.so")

		installRules := result.InstallMakeRulesForTesting(t)

		var installedlibRule *android.InstallMakeRule
		for i, rule := range installRules {
			if rule.Target == "out/target/product/test_device/system/lib/installedlib.so" {
				if installedlibRule != nil {
					t.Errorf("Duplicate install rules for %s", rule.Target)
				}
				installedlibRule = &installRules[i]
			}
		}
		if installedlibRule == nil {
			t.Errorf("No install rule found for installedlib")
			return
		}

		if expectLibfooOnSystemLib {
			android.AssertStringListContains(t,
				"installedlib doesn't have install dependency on libfoo impl",
				installedlibRule.OrderOnlyDeps,
				"out/target/product/test_device/system/lib/libfoo.so")
		} else {
			android.AssertStringListDoesNotContain(t,
				"installedlib has install dependency on libfoo stub",
				installedlibRule.Deps,
				"out/target/product/test_device/system/lib/libfoo.so")
			android.AssertStringListDoesNotContain(t,
				"installedlib has order-only install dependency on libfoo stub",
				installedlibRule.OrderOnlyDeps,
				"out/target/product/test_device/system/lib/libfoo.so")
		}
	}

	prebuiltLibfooBp := []byte(`
		cc_prebuilt_library {
			name: "libfoo",
			prefer: true,
			srcs: ["libfoo.so"],
			stubs: {
				versions: ["1"],
			},
			apex_available: ["apexfoo"],
		}
	`)

	apexfooBp := []byte(`
		apex {
			name: "apexfoo",
			key: "apexfoo.key",
			native_shared_libs: ["libfoo"],
			updatable: false,
			compile_multilib: "both",
		}
		apex_key {
			name: "apexfoo.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
	`)

	installedlibBp := []byte(`
		cc_library {
			name: "installedlib",
			shared_libs: ["libfoo"],
		}
	`)

	t.Run("prebuilt stub (without source): no install", func(t *testing.T) {
		t.Parallel()
		testFunc(
			t,
			/*expectLibfooOnSystemLib=*/ false,
			android.MockFS{
				"prebuilts/module_sdk/art/current/Android.bp": prebuiltLibfooBp,
				"apexfoo/Android.bp":                          apexfooBp,
				"system/sepolicy/apex/apexfoo-file_contexts":  nil,
				"Android.bp": installedlibBp,
			},
		)
	})

	disabledSourceLibfooBp := []byte(`
		cc_library {
			name: "libfoo",
			enabled: false,
			stubs: {
				versions: ["1"],
			},
			apex_available: ["apexfoo"],
		}
	`)

	t.Run("prebuilt stub (with disabled source): no install", func(t *testing.T) {
		t.Parallel()
		testFunc(
			t,
			/*expectLibfooOnSystemLib=*/ false,
			android.MockFS{
				"prebuilts/module_sdk/art/current/Android.bp": prebuiltLibfooBp,
				"impl/Android.bp":                            disabledSourceLibfooBp,
				"apexfoo/Android.bp":                         apexfooBp,
				"system/sepolicy/apex/apexfoo-file_contexts": nil,
				"Android.bp":                                 installedlibBp,
			},
		)
	})
}

func TestSdkLibraryTransitiveClassLoaderContext(t *testing.T) {
	t.Parallel()
	// This test case tests that listing the impl lib instead of the top level java_sdk_library
	// in libs of android_app and java_library does not lead to class loader context device/host
	// path mismatch errors.
	android.GroupFixturePreparers(
		prepareForApexTest,
		android.PrepareForIntegrationTestWithAndroid,
		PrepareForTestWithApexBuildComponents,
		android.FixtureModifyEnv(func(env map[string]string) {
			env["DISABLE_CONTAINER_CHECK"] = "true"
		}),
		withFiles(filesForSdkLibrary),
		android.FixtureMergeMockFs(android.MockFS{
			"system/sepolicy/apex/com.android.foo30-file_contexts": nil,
		}),
	).RunTestWithBp(t, `
		apex {
		name: "com.android.foo30",
		key: "myapex.key",
		updatable: true,
		bootclasspath_fragments: [
			"foo-bootclasspath-fragment",
		],
		java_libs: [
			"bar",
		],
		apps: [
			"bar-app",
		],
		min_sdk_version: "30",
		}
		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}
		bootclasspath_fragment {
			name: "foo-bootclasspath-fragment",
			contents: [
				"framework-foo",
			],
			apex_available: [
				"com.android.foo30",
			],
			hidden_api: {
				split_packages: ["*"]
			},
		}

		java_sdk_library {
			name: "framework-foo",
			srcs: [
				"A.java"
			],
			unsafe_ignore_missing_latest_api: true,
			apex_available: [
				"com.android.foo30",
			],
			compile_dex: true,
			sdk_version: "core_current",
			shared_library: false,
		}

		java_library {
			name: "bar",
			srcs: [
				"A.java"
			],
			libs: [
				"framework-foo.impl",
			],
			apex_available: [
				"com.android.foo30",
			],
			sdk_version: "core_current",
			compile_dex: true,
		}

		java_library {
			name: "baz",
			srcs: [
				"A.java"
			],
			libs: [
				"bar",
			],
			sdk_version: "core_current",
		}

		android_app {
			name: "bar-app",
			srcs: [
				"A.java"
			],
			libs: [
				"baz",
				"framework-foo.impl",
			],
			apex_available: [
				"com.android.foo30",
			],
			sdk_version: "core_current",
			min_sdk_version: "30",
			manifest: "AndroidManifest.xml",
			updatable: true,
		}
       `)
}

// If an apex sets system_ext_specific: true, its systemserverclasspath libraries must set this property as well.
func TestApexSSCPJarMustBeInSamePartitionAsApex(t *testing.T) {
	t.Parallel()
	testApexError(t, `foo is an apex systemserver jar, but its partition does not match the partition of its containing apex`, `
		apex {
			name: "myapex",
			key: "myapex.key",
			systemserverclasspath_fragments: [
				"mysystemserverclasspathfragment",
			],
			min_sdk_version: "29",
			updatable: true,
			system_ext_specific: true,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_library {
			name: "foo",
			srcs: ["b.java"],
			min_sdk_version: "29",
			installable: true,
			apex_available: [
				"myapex",
			],
			sdk_version: "current",
		}

		systemserverclasspath_fragment {
			name: "mysystemserverclasspathfragment",
			contents: [
				"foo",
			],
			apex_available: [
				"myapex",
			],
		}
	`,
		dexpreopt.FixtureSetApexSystemServerJars("myapex:foo"),
	)
}

// partitions should not package the artifacts that are included inside the apex.
func TestFilesystemWithApexDeps(t *testing.T) {
	t.Parallel()
	result := testApex(t, `
		android_filesystem {
			name: "myfilesystem",
			deps: ["myapex"],
		}
		apex {
			name: "myapex",
			key: "myapex.key",
			binaries: ["binfoo"],
			native_shared_libs: ["libfoo"],
			apps: ["appfoo"],
			updatable: false,
		}
		apex_key {
			name: "myapex.key",
		}
		cc_binary {
			name: "binfoo",
			apex_available: ["myapex"],
		}
		cc_library {
			name: "libfoo",
			apex_available: ["myapex"],
		}
		android_app {
			name: "appfoo",
			sdk_version: "current",
			apex_available: ["myapex"],
		}
	`, filesystem.PrepareForTestWithFilesystemBuildComponents)

	partition := result.ModuleForTests(t, "myfilesystem", "android_common")
	fileList := android.ContentFromFileRuleForTests(t, result, partition.Output("fileList"))
	android.AssertDeepEquals(t, "filesystem with apex", "apex/myapex.apex\n", fileList)
}

func TestVintfFragmentInApex(t *testing.T) {
	t.Parallel()
	ctx := testApex(t, apex_default_bp+`
		apex {
			name: "myapex",
			manifest: ":myapex.manifest",
			androidManifest: ":myapex.androidmanifest",
			key: "myapex.key",
			binaries: [ "mybin" ],
			updatable: false,
		}

		cc_binary {
			name: "mybin",
			srcs: ["mybin.cpp"],
			vintf_fragment_modules: ["my_vintf_fragment.xml"],
			apex_available: [ "myapex" ],
		}

		vintf_fragment {
			name: "my_vintf_fragment.xml",
			src: "my_vintf_fragment.xml",
		}
	`)

	generateFsRule := ctx.ModuleForTests(t, "myapex", "android_common_myapex").Rule("generateFsConfig")
	cmd := generateFsRule.RuleParams.Command

	// Ensure that vintf fragment file is being installed
	ensureContains(t, cmd, "/etc/vintf/my_vintf_fragment.xml ")
}

func TestNoVintfFragmentInUpdatableApex(t *testing.T) {
	t.Parallel()
	testApexError(t, `VINTF fragment .* is not supported in updatable APEX`, apex_default_bp+`
		apex {
			name: "myapex",
			manifest: ":myapex.manifest",
			key: "myapex.key",
			binaries: [ "mybin" ],
			updatable: true,
			min_sdk_version: "29",
		}

		cc_binary {
			name: "mybin",
			srcs: ["mybin.cpp"],
			vintf_fragment_modules: ["my_vintf_fragment.xml"],
			apex_available: [ "myapex" ],
			min_sdk_version: "29",
		}

		vintf_fragment {
			name: "my_vintf_fragment.xml",
			src: "my_vintf_fragment.xml",
		}
	`)
}
