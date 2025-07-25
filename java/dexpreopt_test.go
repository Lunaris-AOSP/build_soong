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

package java

import (
	"fmt"
	"runtime"
	"testing"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/dexpreopt"
)

func init() {
	RegisterFakeRuntimeApexMutator()
}

func TestDexpreoptEnabled(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		bp          string
		moduleName  string
		apexVariant bool
		enabled     bool
	}{
		{
			name: "app",
			bp: `
				android_app {
					name: "foo",
					srcs: ["a.java"],
					sdk_version: "current",
				}`,
			enabled: true,
		},
		{
			name: "installable java library",
			bp: `
				java_library {
					name: "foo",
					installable: true,
					srcs: ["a.java"],
					sdk_version: "current",
				}`,
			enabled: true,
		},
		{
			name: "java binary",
			bp: `
				java_binary {
					name: "foo",
					srcs: ["a.java"],
					main_class: "foo.bar.jb",
				}`,
			enabled: true,
		},
		{
			name: "app without sources",
			bp: `
				android_app {
					name: "foo",
					sdk_version: "current",
				}`,
			enabled: false,
		},
		{
			name: "app with libraries",
			bp: `
				android_app {
					name: "foo",
					static_libs: ["lib"],
					sdk_version: "current",
				}

				java_library {
					name: "lib",
					srcs: ["a.java"],
					sdk_version: "current",
				}`,
			enabled: true,
		},
		{
			name: "installable java library without sources",
			bp: `
				java_library {
					name: "foo",
					installable: true,
					sdk_version: "current",
				}`,
			enabled: false,
		},
		{
			name: "static java library",
			bp: `
				java_library {
					name: "foo",
					srcs: ["a.java"],
					sdk_version: "current",
				}`,
			enabled: false,
		},
		{
			name: "java test",
			bp: `
				java_test {
					name: "foo",
					srcs: ["a.java"],
				}`,
			enabled: false,
		},
		{
			name: "android test",
			bp: `
				android_test {
					name: "foo",
					srcs: ["a.java"],
				}`,
			enabled: false,
		},
		{
			name: "android test helper app",
			bp: `
				android_test_helper_app {
					name: "foo",
					srcs: ["a.java"],
				}`,
			enabled: false,
		},
		{
			name: "compile_dex",
			bp: `
				java_library {
					name: "foo",
					srcs: ["a.java"],
					compile_dex: true,
					sdk_version: "current",
				}`,
			enabled: false,
		},
		{
			name: "dex_import",
			bp: `
				dex_import {
					name: "foo",
					jars: ["a.jar"],
				}`,
			enabled: true,
		},
		{
			name: "apex variant",
			bp: `
				java_library {
					name: "foo",
					installable: true,
					srcs: ["a.java"],
					apex_available: ["com.android.apex1"],
					sdk_version: "current",
				}`,
			apexVariant: true,
			enabled:     false,
		},
		{
			name: "apex variant of apex system server jar",
			bp: `
				java_library {
					name: "service-foo",
					installable: true,
					srcs: ["a.java"],
					apex_available: ["com.android.apex1"],
					sdk_version: "current",
				}`,
			moduleName:  "service-foo",
			apexVariant: true,
			enabled:     true,
		},
		{
			name: "apex variant of prebuilt apex system server jar",
			bp: `
				java_library {
					name: "prebuilt_service-foo",
					installable: true,
					srcs: ["a.java"],
					apex_available: ["com.android.apex1"],
					sdk_version: "current",
				}`,
			moduleName:  "prebuilt_service-foo",
			apexVariant: true,
			enabled:     true,
		},
		{
			name: "platform variant of apex system server jar",
			bp: `
				java_library {
					name: "service-foo",
					installable: true,
					srcs: ["a.java"],
					apex_available: ["com.android.apex1"],
					sdk_version: "current",
				}`,
			moduleName:  "service-foo",
			apexVariant: false,
			enabled:     false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			preparers := android.GroupFixturePreparers(
				PrepareForTestWithDexpreopt,
				PrepareForTestWithFakeApexMutator,
				dexpreopt.FixtureSetApexSystemServerJars("com.android.apex1:service-foo"),
			)

			result := preparers.RunTestWithBp(t, test.bp)
			ctx := result.TestContext

			moduleName := "foo"
			if test.moduleName != "" {
				moduleName = test.moduleName
			}

			variant := "android_common"
			if test.apexVariant {
				variant += "_apex1000"
			}

			dexpreopt := ctx.ModuleForTests(t, moduleName, variant).MaybeRule("dexpreopt")
			enabled := dexpreopt.Rule != nil

			if enabled != test.enabled {
				t.Fatalf("want dexpreopt %s, got %s", enabledString(test.enabled), enabledString(enabled))
			}
		})

	}
}

func enabledString(enabled bool) string {
	if enabled {
		return "enabled"
	} else {
		return "disabled"
	}
}

func TestDex2oatToolDeps(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		// The host binary paths checked below are build OS dependent.
		t.Skipf("Unsupported build OS %s", runtime.GOOS)
	}

	preparers := android.GroupFixturePreparers(
		cc.PrepareForTestWithCcDefaultModules,
		PrepareForTestWithDexpreoptWithoutFakeDex2oatd,
		dexpreopt.PrepareForTestByEnablingDexpreopt)

	testDex2oatToolDep := func(sourceEnabled, prebuiltEnabled, prebuiltPreferred bool,
		expectedDex2oatPath string) {
		name := fmt.Sprintf("sourceEnabled:%t,prebuiltEnabled:%t,prebuiltPreferred:%t",
			sourceEnabled, prebuiltEnabled, prebuiltPreferred)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			result := preparers.RunTestWithBp(t, fmt.Sprintf(`
					cc_binary {
						name: "dex2oatd",
						enabled: %t,
						host_supported: true,
					}
					cc_prebuilt_binary {
						name: "dex2oatd",
						enabled: %t,
						prefer: %t,
						host_supported: true,
						srcs: ["x86_64/bin/dex2oatd"],
					}
					java_library {
						name: "myjavalib",
					}
				`, sourceEnabled, prebuiltEnabled, prebuiltPreferred))
			pathContext := android.PathContextForTesting(result.Config)
			dex2oatPath := dexpreopt.GetCachedGlobalSoongConfig(pathContext).Dex2oat
			android.AssertStringEquals(t, "Testing "+name, expectedDex2oatPath, android.NormalizePathForTesting(dex2oatPath))
		})
	}

	sourceDex2oatPath := "../host/linux-x86/bin/dex2oatd"
	prebuiltDex2oatPath := ".intermediates/prebuilt_dex2oatd/linux_glibc_x86_64/dex2oatd"

	testDex2oatToolDep(true, false, false, sourceDex2oatPath)
	testDex2oatToolDep(true, true, false, sourceDex2oatPath)
	testDex2oatToolDep(true, true, true, prebuiltDex2oatPath)
	testDex2oatToolDep(false, true, false, prebuiltDex2oatPath)
}

func TestApexSystemServerDexpreoptInstalls(t *testing.T) {
	preparers := android.GroupFixturePreparers(
		PrepareForTestWithDexpreopt,
		PrepareForTestWithFakeApexMutator,
		dexpreopt.FixtureSetApexSystemServerJars("com.android.apex1:service-foo"),
	)

	// An APEX system server jar.
	result := preparers.RunTestWithBp(t, `
		java_library {
			name: "service-foo",
			installable: true,
			srcs: ["a.java"],
			apex_available: ["com.android.apex1"],
			sdk_version: "current",
		}`)
	ctx := result.TestContext
	module := ctx.ModuleForTests(t, "service-foo", "android_common_apex1000")
	library := module.Module().(*Library)

	installs := library.dexpreopter.ApexSystemServerDexpreoptInstalls()
	dexJars := library.dexpreopter.ApexSystemServerDexJars()

	android.AssertIntEquals(t, "install count", 2, len(installs))
	android.AssertIntEquals(t, "dexjar count", 1, len(dexJars))

	android.AssertPathRelativeToTopEquals(t, "installs[0] OutputPathOnHost",
		"out/soong/.intermediates/service-foo/android_common_apex1000/dexpreopt/service-foo/oat/arm64/javalib.odex",
		installs[0].OutputPathOnHost)

	android.AssertPathRelativeToTopEquals(t, "installs[0] InstallDirOnDevice",
		"out/target/product/test_device/system/framework/oat/arm64",
		installs[0].InstallDirOnDevice)

	android.AssertStringEquals(t, "installs[0] InstallFileOnDevice",
		"apex@com.android.apex1@javalib@service-foo.jar@classes.odex",
		installs[0].InstallFileOnDevice)

	android.AssertPathRelativeToTopEquals(t, "installs[1] OutputPathOnHost",
		"out/soong/.intermediates/service-foo/android_common_apex1000/dexpreopt/service-foo/oat/arm64/javalib.vdex",
		installs[1].OutputPathOnHost)

	android.AssertPathRelativeToTopEquals(t, "installs[1] InstallDirOnDevice",
		"out/target/product/test_device/system/framework/oat/arm64",
		installs[1].InstallDirOnDevice)

	android.AssertStringEquals(t, "installs[1] InstallFileOnDevice",
		"apex@com.android.apex1@javalib@service-foo.jar@classes.vdex",
		installs[1].InstallFileOnDevice)

	// Not an APEX system server jar.
	result = preparers.RunTestWithBp(t, `
		java_library {
			name: "foo",
			installable: true,
			srcs: ["a.java"],
			sdk_version: "current",
		}`)
	ctx = result.TestContext
	module = ctx.ModuleForTests(t, "foo", "android_common")
	library = module.Module().(*Library)

	installs = library.dexpreopter.ApexSystemServerDexpreoptInstalls()
	dexJars = library.dexpreopter.ApexSystemServerDexJars()

	android.AssertIntEquals(t, "install count", 0, len(installs))
	android.AssertIntEquals(t, "dexjar count", 0, len(dexJars))
}

func TestGenerateProfileEvenIfDexpreoptIsDisabled(t *testing.T) {
	preparers := android.GroupFixturePreparers(
		PrepareForTestWithJavaDefaultModules,
		PrepareForTestWithFakeApexMutator,
		dexpreopt.FixtureDisableDexpreopt(true),
	)

	result := preparers.RunTestWithBp(t, `
		java_library {
			name: "foo",
			installable: true,
			dex_preopt: {
				profile: "art-profile",
			},
			srcs: ["a.java"],
			sdk_version: "current",
		}`)

	ctx := result.TestContext
	dexpreopt := ctx.ModuleForTests(t, "foo", "android_common").MaybeRule("dexpreopt")

	expected := []string{"out/soong/.intermediates/foo/android_common/dexpreopt/foo/profile.prof"}

	android.AssertArrayString(t, "outputs", expected, dexpreopt.AllOutputs())
}
