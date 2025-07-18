// Copyright (C) 2021 The Android Open Source Project
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
	"strings"
	"testing"

	"android/soong/android"
	"android/soong/dexpreopt"
	"android/soong/java"

	"github.com/google/blueprint/proptools"
)

// Contains tests for platform_bootclasspath logic from java/platform_bootclasspath.go that requires
// apexes.

var prepareForTestWithPlatformBootclasspath = android.GroupFixturePreparers(
	java.PrepareForTestWithJavaDefaultModules,
	PrepareForTestWithApexBuildComponents,
)

func TestPlatformBootclasspath_Fragments(t *testing.T) {
	t.Parallel()
	result := android.GroupFixturePreparers(
		prepareForTestWithPlatformBootclasspath,
		prepareForTestWithMyapex,
		java.PrepareForTestWithJavaSdkLibraryFiles,
		java.FixtureWithLastReleaseApis("foo"),
		java.FixtureConfigureApexBootJars("myapex:bar"),
		android.FixtureWithRootAndroidBp(`
			platform_bootclasspath {
				name: "platform-bootclasspath",
				fragments: [
					{
						apex: "myapex",
						module:"bar-fragment",
					},
				],
				hidden_api: {
					unsupported: [
							"unsupported.txt",
					],
					removed: [
							"removed.txt",
					],
					max_target_r_low_priority: [
							"max-target-r-low-priority.txt",
					],
					max_target_q: [
							"max-target-q.txt",
					],
					max_target_p: [
							"max-target-p.txt",
					],
					max_target_o_low_priority: [
							"max-target-o-low-priority.txt",
					],
					blocked: [
							"blocked.txt",
					],
					unsupported_packages: [
							"unsupported-packages.txt",
					],
				},
			}

			apex {
				name: "myapex",
				key: "myapex.key",
				bootclasspath_fragments: [
					"bar-fragment",
				],
				updatable: false,
				min_sdk_version: "30", // R
			}

			apex_key {
				name: "myapex.key",
				public_key: "testkey.avbpubkey",
				private_key: "testkey.pem",
			}

			bootclasspath_fragment {
				name: "bar-fragment",
				contents: ["bar"],
				apex_available: ["myapex"],
				api: {
					stub_libs: ["foo"],
				},
				hidden_api: {
					unsupported: [
							"bar-unsupported.txt",
					],
					removed: [
							"bar-removed.txt",
					],
					max_target_r_low_priority: [
							"bar-max-target-r-low-priority.txt",
					],
					max_target_q: [
							"bar-max-target-q.txt",
					],
					max_target_p: [
							"bar-max-target-p.txt",
					],
					max_target_o_low_priority: [
							"bar-max-target-o-low-priority.txt",
					],
					blocked: [
							"bar-blocked.txt",
					],
					unsupported_packages: [
							"bar-unsupported-packages.txt",
					],
					split_packages: ["*"],
				},
			}

			java_library {
				name: "bar",
				apex_available: ["myapex"],
				srcs: ["a.java"],
				system_modules: "none",
				sdk_version: "none",
				compile_dex: true,
				permitted_packages: ["bar"],
				min_sdk_version: "30", // R
			}

			java_sdk_library {
				name: "foo",
				srcs: ["a.java"],
				public: {
					enabled: true,
				},
				compile_dex: true,
			}
		`),
	).RunTest(t)

	pbcp := result.Module("platform-bootclasspath", "android_common")
	info, _ := android.OtherModuleProvider(result, pbcp, java.MonolithicHiddenAPIInfoProvider)

	for _, category := range java.HiddenAPIFlagFileCategories {
		name := category.PropertyName()
		message := fmt.Sprintf("category %s", name)
		filename := strings.ReplaceAll(name, "_", "-")
		expected := []string{fmt.Sprintf("%s.txt", filename), fmt.Sprintf("bar-%s.txt", filename)}
		android.AssertPathsRelativeToTopEquals(t, message, expected, info.FlagsFilesByCategory[category])
	}

	android.AssertPathsRelativeToTopEquals(t, "annotation flags", []string{"out/soong/.intermediates/bar-fragment/android_common_myapex/modular-hiddenapi/annotation-flags.csv"}, info.AnnotationFlagsPaths)
	android.AssertPathsRelativeToTopEquals(t, "metadata flags", []string{"out/soong/.intermediates/bar-fragment/android_common_myapex/modular-hiddenapi/metadata.csv"}, info.MetadataPaths)
	android.AssertPathsRelativeToTopEquals(t, "index flags", []string{"out/soong/.intermediates/bar-fragment/android_common_myapex/modular-hiddenapi/index.csv"}, info.IndexPaths)

	android.AssertArrayString(t, "stub flags", []string{"out/soong/.intermediates/bar-fragment/android_common_myapex/modular-hiddenapi/filtered-stub-flags.csv:out/soong/.intermediates/bar-fragment/android_common_myapex/modular-hiddenapi/signature-patterns.csv"}, info.StubFlagSubsets.RelativeToTop())
	android.AssertArrayString(t, "all flags", []string{"out/soong/.intermediates/bar-fragment/android_common_myapex/modular-hiddenapi/filtered-flags.csv:out/soong/.intermediates/bar-fragment/android_common_myapex/modular-hiddenapi/signature-patterns.csv"}, info.FlagSubsets.RelativeToTop())
}

// TestPlatformBootclasspath_LegacyPrebuiltFragment verifies that the
// prebuilt_bootclasspath_fragment falls back to using the complete stub-flags/all-flags if the
// filtered files are not provided.
//
// TODO: Remove once all prebuilts use the filtered_... properties.
func TestPlatformBootclasspath_LegacyPrebuiltFragment(t *testing.T) {
	t.Parallel()
	result := android.GroupFixturePreparers(
		prepareForTestWithPlatformBootclasspath,
		java.FixtureConfigureApexBootJars("myapex:foo"),
		java.PrepareForTestWithJavaSdkLibraryFiles,
	).RunTestWithBp(t, `
		prebuilt_apex {
			name: "myapex",
			src: "myapex.apex",
			exported_bootclasspath_fragments: ["mybootclasspath-fragment"],
		}

		// A prebuilt java_sdk_library_import that is not preferred by default but will be preferred
		// because AlwaysUsePrebuiltSdks() is true.
		java_sdk_library_import {
			name: "foo",
			prefer: false,
			shared_library: false,
			permitted_packages: ["foo"],
			public: {
				jars: ["sdk_library/public/foo-stubs.jar"],
				stub_srcs: ["sdk_library/public/foo_stub_sources"],
				current_api: "sdk_library/public/foo.txt",
				removed_api: "sdk_library/public/foo-removed.txt",
				sdk_version: "current",
			},
			apex_available: ["myapex"],
		}

		prebuilt_bootclasspath_fragment {
			name: "mybootclasspath-fragment",
			apex_available: [
				"myapex",
			],
			contents: [
				"foo",
			],
			hidden_api: {
				stub_flags: "prebuilt-stub-flags.csv",
				annotation_flags: "prebuilt-annotation-flags.csv",
				metadata: "prebuilt-metadata.csv",
				index: "prebuilt-index.csv",
				all_flags: "prebuilt-all-flags.csv",
			},
		}

		platform_bootclasspath {
			name: "myplatform-bootclasspath",
			fragments: [
				{
					apex: "myapex",
					module:"mybootclasspath-fragment",
				},
			],
		}
`,
	)

	pbcp := result.Module("myplatform-bootclasspath", "android_common")
	info, _ := android.OtherModuleProvider(result, pbcp, java.MonolithicHiddenAPIInfoProvider)

	android.AssertArrayString(t, "stub flags", []string{"prebuilt-stub-flags.csv:out/soong/.intermediates/mybootclasspath-fragment/android_common_prebuilt_myapex/modular-hiddenapi/signature-patterns.csv"}, info.StubFlagSubsets.RelativeToTop())
	android.AssertArrayString(t, "all flags", []string{"prebuilt-all-flags.csv:out/soong/.intermediates/mybootclasspath-fragment/android_common_prebuilt_myapex/modular-hiddenapi/signature-patterns.csv"}, info.FlagSubsets.RelativeToTop())
}

func TestPlatformBootclasspathDependencies(t *testing.T) {
	t.Parallel()
	result := android.GroupFixturePreparers(
		prepareForTestWithPlatformBootclasspath,
		prepareForTestWithArtApex,
		prepareForTestWithMyapex,
		// Configure some libraries in the art and framework boot images.
		java.FixtureConfigureBootJars("com.android.art:baz", "com.android.art:quuz", "platform:foo"),
		java.FixtureConfigureApexBootJars("myapex:bar"),
		java.PrepareForTestWithJavaSdkLibraryFiles,
		java.FixtureWithLastReleaseApis("foo"),
		java.PrepareForTestWithDexpreopt,
		dexpreopt.FixtureDisableDexpreoptBootImages(false),
		android.PrepareForTestWithBuildFlag("RELEASE_HIDDEN_API_EXPORTABLE_STUBS", "true"),
	).RunTestWithBp(t, `
		apex {
			name: "com.android.art",
			key: "com.android.art.key",
 			bootclasspath_fragments: [
				"art-bootclasspath-fragment",
			],
			updatable: false,
		}

		apex_key {
			name: "com.android.art.key",
			public_key: "com.android.art.avbpubkey",
			private_key: "com.android.art.pem",
		}

		bootclasspath_fragment {
			name: "art-bootclasspath-fragment",
			image_name: "art",
			apex_available: [
				"com.android.art",
			],
			contents: [
				"baz",
				"quuz",
			],
			hidden_api: {
				split_packages: ["*"],
			},
		}

		java_library {
			name: "baz",
			apex_available: [
				"com.android.art",
			],
			srcs: ["b.java"],
			installable: true,
			sdk_version: "core_current",
		}

		// Add a java_import that is not preferred and so won't have an appropriate apex variant created
		// for it to make sure that the platform_bootclasspath doesn't try and add a dependency onto it.
		java_import {
			name: "baz",
			apex_available: [
				"com.android.art",
			],
			jars: ["b.jar"],
		}

		java_library {
			name: "quuz",
			apex_available: [
				"com.android.art",
			],
			srcs: ["b.java"],
			installable: true,
		}

		apex {
			name: "myapex",
			key: "myapex.key",
			bootclasspath_fragments: [
				"my-bootclasspath-fragment",
			],
			updatable: false,
		}

		bootclasspath_fragment {
			name: "my-bootclasspath-fragment",
			contents: ["bar"],
			apex_available: ["myapex"],
			hidden_api: {
				split_packages: ["*"],
			},
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_sdk_library {
			name: "foo",
			srcs: ["b.java"],
		}

		java_library {
			name: "bar",
			srcs: ["b.java"],
			installable: true,
			apex_available: ["myapex"],
			permitted_packages: ["bar"],
		}

		platform_bootclasspath {
			name: "myplatform-bootclasspath",

			fragments: [
				{
					apex: "com.android.art",
					module: "art-bootclasspath-fragment",
				},
				{
					apex: "myapex",
					module: "my-bootclasspath-fragment",
				},
			],
		}
`,
	)

	java.CheckPlatformBootclasspathModules(t, result, "myplatform-bootclasspath", []string{
		// The configured contents of BootJars.
		"com.android.art:baz",
		"com.android.art:quuz",
		"platform:foo",

		// The configured contents of ApexBootJars.
		"myapex:bar",
	})

	java.CheckPlatformBootclasspathFragments(t, result, "myplatform-bootclasspath", []string{
		"com.android.art:art-bootclasspath-fragment",
		"myapex:my-bootclasspath-fragment",
	})

	// Make sure that the myplatform-bootclasspath has the correct dependencies.
	java.CheckPlatformBootclasspathDependencies(t, result.TestContext, "myplatform-bootclasspath", "android_common", []string{
		// source vs prebuilt selection metadata module
		`platform:all_apex_contributions`,

		// The following are stubs.
		`platform:android_stubs_current_exportable`,
		`platform:android_system_stubs_current_exportable`,
		`platform:android_test_stubs_current_exportable`,
		`platform:legacy.core.platform.api.stubs.exportable`,

		// Needed for generating the boot image.
		`platform:dex2oatd`,

		// The configured contents of BootJars, via their apexes if necessary.
		`platform:com.android.art`,
		`platform:com.android.art`,
		`platform:foo`,

		// The configured contents of ApexBootJars, via their apex.
		`platform:myapex`,

		// The fragments via their apexes.
		`platform:com.android.art`,
		`platform:myapex`,

		// Impl lib of sdk_library for transitive srcjar generation
		`platform:foo.impl`,
	})
}

// TestPlatformBootclasspath_AlwaysUsePrebuiltSdks verifies that the build does not fail when
// AlwaysUsePrebuiltSdk() returns true.
func TestPlatformBootclasspath_AlwaysUsePrebuiltSdks(t *testing.T) {
	t.Parallel()
	result := android.GroupFixturePreparers(
		prepareForTestWithPlatformBootclasspath,
		prepareForTestWithMyapex,
		// Configure two libraries, the first is a java_sdk_library whose prebuilt will be used because
		// of AlwaysUsePrebuiltsSdk(). The second is a normal library that is unaffected. The order
		// matters, so that the dependencies resolved by the platform_bootclasspath matches the
		// configured list.
		java.FixtureConfigureApexBootJars("myapex:foo"),
		java.PrepareForTestWithJavaSdkLibraryFiles,
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.Always_use_prebuilt_sdks = proptools.BoolPtr(true)
		}),
		android.PrepareForTestWithBuildFlag("RELEASE_HIDDEN_API_EXPORTABLE_STUBS", "true"),

		java.FixtureWithPrebuiltApis(map[string][]string{
			"current": {},
			"30":      {"foo"},
		}),
	).RunTestWithBp(t, `
		apex {
			name: "myapex",
			key: "myapex.key",
			bootclasspath_fragments: [
				"mybootclasspath-fragment",
			],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_library {
			name: "bar",
			srcs: ["b.java"],
			installable: true,
			apex_available: ["myapex"],
			permitted_packages: ["bar"],
		}

		java_sdk_library {
			name: "foo",
			srcs: ["b.java"],
			shared_library: false,
			public: {
				enabled: true,
			},
			apex_available: ["myapex"],
			permitted_packages: ["foo"],
		}

		prebuilt_apex {
			name: "myapex",
			src: "myapex.apex",
			exported_bootclasspath_fragments: ["mybootclasspath-fragment"],
			prefer: true,
		}

		// A prebuilt java_sdk_library_import that is not preferred by default but will be preferred
		// because AlwaysUsePrebuiltSdks() is true.
		java_sdk_library_import {
			name: "foo",
			prefer: false,
			shared_library: false,
			permitted_packages: ["foo"],
			public: {
				jars: ["sdk_library/public/foo-stubs.jar"],
				stub_srcs: ["sdk_library/public/foo_stub_sources"],
				current_api: "sdk_library/public/foo.txt",
				removed_api: "sdk_library/public/foo-removed.txt",
				sdk_version: "current",
			},
			apex_available: ["myapex"],
		}

		// This always depends on the source foo module, its dependencies are not affected by the
		// AlwaysUsePrebuiltSdks().
		bootclasspath_fragment {
			name: "mybootclasspath-fragment",
			apex_available: [
				"myapex",
			],
			contents: [
				"foo", "bar",
			],
			hidden_api: {
				split_packages: ["*"],
			},
		}

		prebuilt_bootclasspath_fragment {
			name: "mybootclasspath-fragment",
			apex_available: [
				"myapex",
			],
			contents: [
				"foo",
			],
			hidden_api: {
				stub_flags: "",
				annotation_flags: "",
				metadata: "",
				index: "",
				all_flags: "",
			},
		}

		platform_bootclasspath {
			name: "myplatform-bootclasspath",
			fragments: [
				{
					apex: "myapex",
					module:"mybootclasspath-fragment",
				},
			],
		}
`,
	)

	java.CheckPlatformBootclasspathModules(t, result, "myplatform-bootclasspath", []string{
		// The configured contents of BootJars.
		"prebuilt_myapex:prebuilt_foo",
	})

	// Make sure that the myplatform-bootclasspath has the correct dependencies.
	java.CheckPlatformBootclasspathDependencies(t, result.TestContext, "myplatform-bootclasspath", "android_common", []string{
		// source vs prebuilt selection metadata module
		`platform:all_apex_contributions`,

		// The following are stubs.
		"platform:prebuilt_sdk_public_current_android",
		"platform:prebuilt_sdk_system_current_android",
		"platform:prebuilt_sdk_test_current_android",

		// Not a prebuilt as no prebuilt existed when it was added.
		"platform:legacy.core.platform.api.stubs.exportable",

		// The prebuilt library via the apex.
		"platform:prebuilt_myapex",

		// The fragments via the apex.
		"platform:prebuilt_myapex",

		// Impl lib of sdk_library for transitive srcjar generation
		"platform:foo.impl",
	})
}

// TestPlatformBootclasspath_IncludesRemainingApexJars verifies that any apex boot jar is present in
// platform_bootclasspath's classpaths.proto config, if the apex does not generate its own config
// by setting generate_classpaths_proto property to false.
func TestPlatformBootclasspath_IncludesRemainingApexJars(t *testing.T) {
	t.Parallel()
	result := android.GroupFixturePreparers(
		prepareForTestWithPlatformBootclasspath,
		prepareForTestWithMyapex,
		java.FixtureConfigureApexBootJars("myapex:foo"),
		android.FixtureWithRootAndroidBp(`
			platform_bootclasspath {
				name: "platform-bootclasspath",
				fragments: [
					{
						apex: "myapex",
						module:"foo-fragment",
					},
				],
			}

			apex {
				name: "myapex",
				key: "myapex.key",
				bootclasspath_fragments: ["foo-fragment"],
				updatable: false,
			}

			apex_key {
				name: "myapex.key",
				public_key: "testkey.avbpubkey",
				private_key: "testkey.pem",
			}

			bootclasspath_fragment {
				name: "foo-fragment",
				generate_classpaths_proto: false,
				contents: ["foo"],
				apex_available: ["myapex"],
				hidden_api: {
					split_packages: ["*"],
				},
			}

			java_library {
				name: "foo",
				srcs: ["a.java"],
				system_modules: "none",
				sdk_version: "none",
				compile_dex: true,
				apex_available: ["myapex"],
				permitted_packages: ["foo"],
			}
		`),
	).RunTest(t)

	java.CheckClasspathFragmentProtoContentInfoProvider(t, result,
		true,         // proto should be generated
		"myapex:foo", // apex doesn't generate its own config, so must be in platform_bootclasspath
		"bootclasspath.pb",
		"out/target/product/test_device/system/etc/classpaths",
	)
}

func TestBootJarNotInApex(t *testing.T) {
	t.Parallel()
	android.GroupFixturePreparers(
		prepareForTestWithPlatformBootclasspath,
		PrepareForTestWithApexBuildComponents,
		prepareForTestWithMyapex,
		java.FixtureConfigureApexBootJars("myapex:foo"),
	).ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(
		`module "myplatform-bootclasspath" variant ".*": failed to find module "foo" in apex "myapex"`)).
		RunTestWithBp(t, `
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

			java_library {
				name: "foo",
				srcs: ["b.java"],
				installable: true,
				apex_available: [
					"myapex",
				],
			}

			bootclasspath_fragment {
				name: "not-in-apex-fragment",
				contents: [
					"foo",
				],
				hidden_api: {
					split_packages: ["*"],
				},
			}

			platform_bootclasspath {
				name: "myplatform-bootclasspath",
			}
		`)
}

func TestBootFragmentNotInApex(t *testing.T) {
	t.Parallel()
	android.GroupFixturePreparers(
		prepareForTestWithPlatformBootclasspath,
		PrepareForTestWithApexBuildComponents,
		prepareForTestWithMyapex,
		java.FixtureConfigureApexBootJars("myapex:foo"),
	).ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(
		`library foo.*have no corresponding fragment.*`)).RunTestWithBp(t, `
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

			java_library {
				name: "foo",
				srcs: ["b.java"],
				installable: true,
				apex_available: ["myapex"],
				permitted_packages: ["foo"],
			}

			bootclasspath_fragment {
				name: "not-in-apex-fragment",
				contents: ["foo"],
				hidden_api: {
					split_packages: ["*"],
				},
			}

			platform_bootclasspath {
				name: "myplatform-bootclasspath",
			}
		`)
}

func TestNonBootJarInFragment(t *testing.T) {
	t.Parallel()
	android.GroupFixturePreparers(
		prepareForTestWithPlatformBootclasspath,
		PrepareForTestWithApexBuildComponents,
		prepareForTestWithMyapex,
		java.FixtureConfigureApexBootJars("myapex:foo"),
	).ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(
		`in contents must also be declared in PRODUCT_APEX_BOOT_JARS`)).
		RunTestWithBp(t, `
			apex {
				name: "myapex",
				key: "myapex.key",
				bootclasspath_fragments: ["apex-fragment"],
				updatable: false,
			}

			apex_key {
				name: "myapex.key",
				public_key: "testkey.avbpubkey",
				private_key: "testkey.pem",
			}

			java_library {
				name: "foo",
				srcs: ["b.java"],
				installable: true,
				apex_available: ["myapex"],
				permitted_packages: ["foo"],
			}

			java_library {
				name: "bar",
				srcs: ["b.java"],
				installable: true,
				apex_available: ["myapex"],
				permitted_packages: ["bar"],
			}

			bootclasspath_fragment {
				name: "apex-fragment",
				contents: ["foo", "bar"],
				apex_available:[ "myapex" ],
				hidden_api: {
					split_packages: ["*"],
				},
			}

			platform_bootclasspath {
				name: "myplatform-bootclasspath",
				fragments: [{
						apex: "myapex",
						module:"apex-fragment",
				}],
			}
		`)
}

// Skip bcp_fragment content validation of source apexes if prebuilts are active.
func TestNonBootJarInPrebuilts(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		description               string
		selectedApexContributions string
		expectedError             string
	}{
		{
			description:               "source is active",
			selectedApexContributions: "",
			expectedError:             "in contents must also be declared in PRODUCT_APEX_BOOT_JARS",
		},
		{
			description:               "prebuilts are active",
			selectedApexContributions: "myapex.prebuilt.contributions",
			expectedError:             "", // skip content validation of source bcp fragment
		},
	}
	bp := `
// Source
apex {
	name: "myapex",
	key: "myapex.key",
	bootclasspath_fragments: ["apex-fragment"],
	updatable: false,
	min_sdk_version: "29",
}

override_apex {
	name: "myapex.override", // overrides the min_sdk_version, thereby creating different variants of its transitive deps
	base: "myapex",
	min_sdk_version: "34",
}

apex_key {
	name: "myapex.key",
	public_key: "testkey.avbpubkey",
	private_key: "testkey.pem",
}

java_library {
	name: "foo",
	srcs: ["b.java"],
	installable: true,
	apex_available: ["myapex"],
	permitted_packages: ["foo"],
	min_sdk_version: "29",
}

java_library {
	name: "bar",
	srcs: ["b.java"],
	installable: true,
	apex_available: ["myapex"],
	permitted_packages: ["bar"],
	min_sdk_version: "29",
}

bootclasspath_fragment {
	name: "apex-fragment",
	contents: ["foo", "bar"],
	apex_available:[ "myapex" ],
	hidden_api: {
		split_packages: ["*"],
	},
}

platform_bootclasspath {
	name: "myplatform-bootclasspath",
	fragments: [{
			apex: "myapex",
			module:"apex-fragment",
	}],
}

// prebuilts
prebuilt_apex {
	name: "myapex",
		apex_name: "myapex",
		src: "myapex.apex",
		exported_bootclasspath_fragments: ["apex-fragment"],
	}

	prebuilt_bootclasspath_fragment {
		name: "apex-fragment",
		contents: ["foo"],
		hidden_api: {
			annotation_flags: "my-bootclasspath-fragment/annotation-flags.csv",
			metadata: "my-bootclasspath-fragment/metadata.csv",
			index: "my-bootclasspath-fragment/index.csv",
			stub_flags: "my-bootclasspath-fragment/stub-flags.csv",
			all_flags: "my-bootclasspath-fragment/all-flags.csv",
		},
	}
	java_import {
		name: "foo",
		jars: ["foo.jar"],
	}

apex_contributions {
	name: "myapex.prebuilt.contributions",
	api_domain: "myapex",
	contents: ["prebuilt_myapex"],
}
`

	for _, tc := range testCases {
		fixture := android.GroupFixturePreparers(
			prepareForTestWithPlatformBootclasspath,
			PrepareForTestWithApexBuildComponents,
			prepareForTestWithMyapex,
			java.FixtureConfigureApexBootJars("myapex:foo"),
			android.PrepareForTestWithBuildFlag("RELEASE_APEX_CONTRIBUTIONS_ADSERVICES", tc.selectedApexContributions),
		)
		if tc.expectedError != "" {
			fixture = fixture.ExtendWithErrorHandler(android.FixtureExpectsAtLeastOneErrorMatchingPattern(tc.expectedError))
		}
		fixture.RunTestWithBp(t, bp)
	}

}

// Source and prebuilt apex provide different set of boot jars
func TestNonBootJarMissingInPrebuiltFragment(t *testing.T) {
	t.Parallel()
	bp := `
		apex {
			name: "myapex",
			key: "myapex.key",
			bootclasspath_fragments: ["apex-fragment"],
			updatable: false,
		}

		apex_key {
			name: "myapex.key",
			public_key: "testkey.avbpubkey",
			private_key: "testkey.pem",
		}

		java_library {
			name: "foo",
			srcs: ["b.java"],
			installable: true,
			apex_available: ["myapex"],
			permitted_packages: ["foo"],
		}

		java_library {
			name: "bar",
			srcs: ["b.java"],
			installable: true,
			apex_available: ["myapex"],
			permitted_packages: ["bar"],
		}

		bootclasspath_fragment {
			name: "apex-fragment",
			contents: ["foo", "bar"],
			apex_available:[ "myapex" ],
			hidden_api: {
				split_packages: ["*"],
			},
		}

		prebuilt_apex {
			name: "com.google.android.myapex", // mainline prebuilt selection logic in soong relies on the naming convention com.google.android
			apex_name: "myapex",
			source_apex_name: "myapex",
			src: "myapex.apex",
			exported_bootclasspath_fragments: ["apex-fragment"],
		}

		java_import {
			name: "foo",
			jars: ["foo.jar"],
			apex_available: ["myapex"],
			permitted_packages: ["foo"],
		}

		prebuilt_bootclasspath_fragment {
			name: "apex-fragment",
			contents: ["foo"], // Unlike the source fragment, this is missing bar
			apex_available:[ "myapex" ],
			hidden_api: {
				annotation_flags: "my-bootclasspath-fragment/annotation-flags.csv",
				metadata: "my-bootclasspath-fragment/metadata.csv",
				index: "my-bootclasspath-fragment/index.csv",
				stub_flags: "my-bootclasspath-fragment/stub-flags.csv",
				all_flags: "my-bootclasspath-fragment/all-flags.csv",
			},
		}

		// Another prebuilt apex, but this is not selected during the build.
		prebuilt_apex {
			name: "com.google.android.myapex.v2", // mainline prebuilt selection logic in soong relies on the naming convention com.google.android
			apex_name: "myapex",
			source_apex_name: "myapex",
			src: "myapex.apex",
			exported_bootclasspath_fragments: ["apex-fragment.v2"],
		}

		java_import {
			name: "bar",
			jars: ["bar.jar"],
			apex_available: ["myapex"],
			permitted_packages: ["bar"],
		}

		prebuilt_bootclasspath_fragment {
			name: "apex-fragment.v2",
			contents: ["bar"], // Unlike the source fragment, this is missing foo
			apex_available:[ "myapex" ],
			hidden_api: {
				annotation_flags: "my-bootclasspath-fragment/annotation-flags.csv",
				metadata: "my-bootclasspath-fragment/metadata.csv",
				index: "my-bootclasspath-fragment/index.csv",
				stub_flags: "my-bootclasspath-fragment/stub-flags.csv",
				all_flags: "my-bootclasspath-fragment/all-flags.csv",
			},
		}


		apex_contributions {
			name: "my_apex_contributions",
			api_domain: "myapex",
			contents: [%v],
		}
	`
	testCases := []struct {
		desc                     string
		configuredBootJars       []string
		apexContributionContents string
		errorExpected            bool
	}{
		{
			desc:               "Source apex is selected, and APEX_BOOT_JARS is correctly configured for source apex builds",
			configuredBootJars: []string{"myapex:foo", "myapex:bar"},
		},
		{
			desc:               "Source apex is selected, and APEX_BOOT_JARS is missing bar",
			configuredBootJars: []string{"myapex:foo"},
			errorExpected:      true,
		},
		{
			desc:                     "Prebuilt apex is selected, and APEX_BOOT_JARS is correctly configured for prebuilt apex build",
			configuredBootJars:       []string{"myapex:foo"},
			apexContributionContents: `"prebuilt_com.google.android.myapex"`,
		},
		{
			desc:                     "Prebuilt apex is selected, and APEX_BOOT_JARS is missing foo",
			configuredBootJars:       []string{"myapex:bar"},
			apexContributionContents: `"prebuilt_com.google.android.myapex"`,
			errorExpected:            true,
		},
	}

	for _, tc := range testCases {
		fixture := android.GroupFixturePreparers(
			prepareForTestWithPlatformBootclasspath,
			PrepareForTestWithApexBuildComponents,
			prepareForTestWithMyapex,
			java.FixtureConfigureApexBootJars(tc.configuredBootJars...),
			android.PrepareForTestWithBuildFlag("RELEASE_APEX_CONTRIBUTIONS_ART", "my_apex_contributions"),
		)
		if tc.errorExpected {
			fixture = fixture.ExtendWithErrorHandler(
				android.FixtureExpectsAtLeastOneErrorMatchingPattern(`in contents.*must also be declared in PRODUCT_APEX_BOOT_JARS`),
			)
		}
		fixture.RunTestWithBp(t, fmt.Sprintf(bp, tc.apexContributionContents))
	}
}
