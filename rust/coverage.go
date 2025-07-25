// Copyright 2020 The Android Open Source Project
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

package rust

import (
	"android/soong/android"

	"github.com/google/blueprint"

	"android/soong/cc"
)

var ProfilerBuiltins = "libprofiler_builtins.rust_sysroot"

// Add '%c' to default specifier after we resolve http://b/210012154
const profileInstrFlag = "-fprofile-instr-generate=/data/misc/trace/clang-%p-%m.profraw"

type coverage struct {
	Properties cc.CoverageProperties

	// Whether binaries containing this module need --coverage added to their ldflags
	linkCoverage bool
}

func (cov *coverage) props() []interface{} {
	return []interface{}{&cov.Properties}
}

func getClangProfileLibraryName(ctx ModuleContextIntf) string {
	if ctx.RustModule().UseSdk() {
		return "libprofile-clang-extras_ndk"
	} else {
		return "libprofile-clang-extras"
	}
}

func (cov *coverage) deps(ctx DepsContext, deps Deps) Deps {
	if cov.Properties.NeedCoverageVariant {
		if ctx.Device() {
			ctx.AddVariationDependencies([]blueprint.Variation{
				{Mutator: "link", Variation: "static"},
			}, cc.CoverageDepTag, getClangProfileLibraryName(ctx))
		}

		// no_std modules are missing libprofiler_builtins which provides coverage, so we need to add it as a dependency.
		if rustModule, ok := ctx.Module().(*Module); ok && rustModule.compiler.noStdlibs() {
			ctx.AddVariationDependencies([]blueprint.Variation{{Mutator: "rust_libraries", Variation: "rlib"}}, rlibDepTag, ProfilerBuiltins)
		}
	}

	return deps
}

func (cov *coverage) flags(ctx ModuleContext, flags Flags, deps PathDeps) (Flags, PathDeps) {

	if !ctx.DeviceConfig().NativeCoverageEnabled() {
		return flags, deps
	}

	if cov.Properties.CoverageEnabled {
		flags.Coverage = true
		flags.RustFlags = append(flags.RustFlags,
			"-C instrument-coverage", "-g")
		if ctx.Device() {
			m := ctx.GetDirectDepProxyWithTag(getClangProfileLibraryName(ctx), cc.CoverageDepTag)
			coverage := android.OtherModuleProviderOrDefault(ctx, m, cc.LinkableInfoProvider)
			flags.LinkFlags = append(flags.LinkFlags,
				profileInstrFlag, "-g", coverage.OutputFile.Path().String(), "-Wl,--wrap,open")
			deps.StaticLibs = append(deps.StaticLibs, coverage.OutputFile.Path())
		}

		// no_std modules are missing libprofiler_builtins which provides coverage, so we need to add it as a dependency.
		if rustModule, ok := ctx.Module().(*Module); ok && rustModule.compiler.noStdlibs() {
			m := ctx.GetDirectDepProxyWithTag(ProfilerBuiltins, rlibDepTag)
			profiler_builtins := android.OtherModuleProviderOrDefault(ctx, m, cc.LinkableInfoProvider)
			deps.RLibs = append(deps.RLibs, RustLibrary{Path: profiler_builtins.OutputFile.Path(), CrateName: profiler_builtins.CrateName})
		}

		if cc.EnableContinuousCoverage(ctx) {
			flags.RustFlags = append(flags.RustFlags, "-C llvm-args=--runtime-counter-relocation")
			flags.LinkFlags = append(flags.LinkFlags, "-Wl,-mllvm,-runtime-counter-relocation")
		}
	}

	return flags, deps
}

func (cov *coverage) begin(ctx BaseModuleContext) {
	// Update useSdk and sdkVersion args if Rust modules become SDK aware.
	cov.Properties = cc.SetCoverageProperties(ctx, cov.Properties, ctx.RustModule().nativeCoverage(), false, "")
}
