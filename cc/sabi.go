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
	"sync"

	"android/soong/android"
	"android/soong/cc/config"
)

var (
	lsdumpPathsLock sync.Mutex
	lsdumpKey       = android.NewOnceKey("lsdump")
)

func lsdumpPaths(config android.Config) *[]string {
	return config.Once(lsdumpKey, func() any {
		return &[]string{}
	}).(*[]string)
}

type lsdumpTag string

const (
	apexLsdumpTag     lsdumpTag = "APEX"
	llndkLsdumpTag    lsdumpTag = "LLNDK"
	platformLsdumpTag lsdumpTag = "PLATFORM"
	productLsdumpTag  lsdumpTag = "PRODUCT"
	vendorLsdumpTag   lsdumpTag = "VENDOR"
)

// Return the prebuilt ABI dump directory for a tag; an empty string for an opt-in dump.
func (tag *lsdumpTag) dirName() string {
	switch *tag {
	case apexLsdumpTag:
		return "platform"
	case llndkLsdumpTag:
		return "vndk"
	default:
		return ""
	}
}

// Properties for ABI compatibility checker in Android.bp.
type headerAbiCheckerProperties struct {
	// Enable ABI checks (even if this is not an LLNDK/VNDK lib)
	Enabled *bool

	// Path to a symbol file that specifies the symbols to be included in the generated
	// ABI dump file
	Symbol_file *string `android:"path"`

	// Symbol versions that should be ignored from the symbol file
	Exclude_symbol_versions []string

	// Symbol tags that should be ignored from the symbol file
	Exclude_symbol_tags []string

	// Run checks on all APIs (in addition to the ones referred by
	// one of exported ELF symbols.)
	Check_all_apis *bool

	// Extra flags passed to header-abi-diff
	Diff_flags []string

	// Opt-in reference dump directories
	Ref_dump_dirs []string
}

func (props *headerAbiCheckerProperties) enabled() bool {
	return Bool(props.Enabled)
}

func (props *headerAbiCheckerProperties) explicitlyDisabled() bool {
	return !BoolDefault(props.Enabled, true)
}

type SAbiProperties struct {
	// Whether ABI dump should be created for this module.
	// Set by `sabiTransitionMutator` if this module is a shared library that needs ABI check,
	// or a static library that is depended on by an ABI checked library.
	ShouldCreateSourceAbiDump bool `blueprint:"mutated"`

	// Include directories that may contain ABI information exported by a library.
	// These directories are passed to the header-abi-dumper.
	ReexportedIncludes       []string `blueprint:"mutated"`
	ReexportedSystemIncludes []string `blueprint:"mutated"`
}

type sabi struct {
	Properties SAbiProperties
}

func (sabi *sabi) props() []interface{} {
	return []interface{}{&sabi.Properties}
}

func (sabi *sabi) flags(ctx ModuleContext, flags Flags) Flags {
	// Filter out flags which libTooling don't understand.
	// This is here for legacy reasons and future-proof, in case the version of libTooling and clang
	// diverge.
	flags.Local.ToolingCFlags = config.ClangLibToolingFilterUnknownCflags(flags.Local.CFlags)
	flags.Global.ToolingCFlags = config.ClangLibToolingFilterUnknownCflags(flags.Global.CFlags)
	flags.Local.ToolingCppFlags = config.ClangLibToolingFilterUnknownCflags(flags.Local.CppFlags)
	flags.Global.ToolingCppFlags = config.ClangLibToolingFilterUnknownCflags(flags.Global.CppFlags)
	return flags
}

// Returns true if ABI dump should be created for this library, either because library is ABI
// checked or is depended on by an ABI checked library.
// Could be called as a nil receiver.
func (sabi *sabi) shouldCreateSourceAbiDump() bool {
	return sabi != nil && sabi.Properties.ShouldCreateSourceAbiDump
}

// Returns a slice of strings that represent the ABI dumps generated for this module.
func classifySourceAbiDump(m *Module) []lsdumpTag {
	result := []lsdumpTag{}
	headerAbiChecker := m.library.getHeaderAbiCheckerProperties(m)
	if headerAbiChecker.explicitlyDisabled() {
		return result
	}
	if !m.InProduct() && !m.InVendor() {
		if m.isImplementationForLLNDKPublic() {
			result = append(result, llndkLsdumpTag)
		}
		if m.library.HasStubsVariants() {
			result = append(result, apexLsdumpTag)
		}
		if headerAbiChecker.enabled() {
			result = append(result, platformLsdumpTag)
		}
	} else if headerAbiChecker.enabled() {
		if m.InProduct() {
			result = append(result, productLsdumpTag)
		}
		if m.InVendor() {
			result = append(result, vendorLsdumpTag)
		}
	}
	return result
}

type shouldCreateAbiDumpContext interface {
	android.ModuleProviderContext
	Module() android.Module
	Config() android.Config
}

var _ shouldCreateAbiDumpContext = android.ModuleContext(nil)
var _ shouldCreateAbiDumpContext = android.OutgoingTransitionContext(nil)

// Called from sabiTransitionMutator to check whether ABI dumps should be created for this module.
// ctx should be wrapping a native library type module.
func shouldCreateSourceAbiDumpForLibrary(ctx shouldCreateAbiDumpContext) bool {
	m, ok := ctx.Module().(*Module)
	if !ok {
		return false
	}

	// Only generate ABI dump for device modules.
	if !m.Device() {
		return false
	}

	// Only create ABI dump for native library module types.
	if m.library == nil {
		return false
	}

	// Don't create ABI dump for static libraries
	// The sabi variant will be propagated to dependencies of ABI checked libraries.
	if m.library.static() {
		return false
	}

	// Module is shared library type.

	// Don't check uninstallable modules.
	if m.IsHideFromMake() {
		return false
	}

	// Don't check ramdisk or recovery variants. Only check core, vendor or product variants.
	if m.InRamdisk() || m.InVendorRamdisk() || m.InRecovery() {
		return false
	}

	// Don't create ABI dump for prebuilts.
	if m.Prebuilt() != nil || m.IsSnapshotPrebuilt() {
		return false
	}

	// Coverage builds have extra symbols.
	if m.isCoverageVariant() {
		return false
	}

	// Some sanitizer variants may have different ABI.
	if m.sanitize != nil && !m.sanitize.isVariantOnProductionDevice() {
		return false
	}

	// Don't create ABI dump for stubs.
	if m.isNDKStubLibrary() || m.IsLlndk() || m.IsStubs() {
		return false
	}

	apexInfo, _ := android.ModuleProvider(ctx, android.ApexInfoProvider)
	if apexInfo.IsForPlatform() {
		// Bionic libraries that are installed to the bootstrap directory are not ABI checked.
		// Only the runtime APEX variants, which are the implementation libraries of bionic NDK stubs,
		// are checked.
		if InstallToBootstrap(m.BaseModuleName(), ctx.Config()) {
			return false
		}
	} else {
		// Don't create ABI dump if this library is for APEX but isn't exported.
		if !m.HasStubsVariants() {
			return false
		}
	}
	return len(classifySourceAbiDump(m)) > 0
}

// Mark the direct and transitive dependencies of libraries that need ABI check, so that ABI dumps
// of their dependencies would be generated.
type sabiTransitionMutator struct{}

func (s *sabiTransitionMutator) Split(ctx android.BaseModuleContext) []string {
	return []string{""}
}

func (s *sabiTransitionMutator) OutgoingTransition(ctx android.OutgoingTransitionContext, sourceVariation string) string {
	// Escape hatch to not check any ABI dump.
	if ctx.Config().IsEnvTrue("SKIP_ABI_CHECKS") {
		return ""
	}

	// Only create ABI dump for native shared libraries and their static library dependencies.
	if m, ok := ctx.Module().(*Module); ok && m.sabi != nil {
		if shouldCreateSourceAbiDumpForLibrary(ctx) {
			if IsStaticDepTag(ctx.DepTag()) || ctx.DepTag() == reuseObjTag {
				return "sabi"
			}
		} else if sourceVariation == "sabi" {
			if IsWholeStaticLib(ctx.DepTag()) || ctx.DepTag() == reuseObjTag {
				return "sabi"
			}
		}
	}

	return ""
}

func (s *sabiTransitionMutator) IncomingTransition(ctx android.IncomingTransitionContext, incomingVariation string) string {
	if incomingVariation == "" {
		return ""
	}

	if incomingVariation == "sabi" {
		if m, ok := ctx.Module().(*Module); ok && m.sabi != nil {
			return "sabi"
		}
	}

	return ""
}

func (s *sabiTransitionMutator) Mutate(ctx android.BottomUpMutatorContext, variation string) {
	if m, ok := ctx.Module().(*Module); ok && m.sabi != nil {
		if variation == "sabi" {
			m.sabi.Properties.ShouldCreateSourceAbiDump = true
			m.HideFromMake()
			m.Properties.PreventInstall = true
		} else if shouldCreateSourceAbiDumpForLibrary(ctx) {
			// Escape hatch to not check any ABI dump.
			if !ctx.Config().IsEnvTrue("SKIP_ABI_CHECKS") {
				m.sabi.Properties.ShouldCreateSourceAbiDump = true
			}
		}
	}
}

// Add an entry to the global list of lsdump. The list is exported to a Make variable by
// `cc.makeVarsProvider`.
func addLsdumpPath(config android.Config, lsdumpPath string) {
	lsdumpPaths := lsdumpPaths(config)
	lsdumpPathsLock.Lock()
	defer lsdumpPathsLock.Unlock()
	*lsdumpPaths = append(*lsdumpPaths, lsdumpPath)
}
