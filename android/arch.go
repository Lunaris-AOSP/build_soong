// Copyright 2015 Google Inc. All rights reserved.
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

package android

import (
	"encoding"
	"fmt"
	"reflect"
	"runtime"
	"slices"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

/*
Example blueprints file containing all variant property groups, with comment listing what type
of variants get properties in that group:

module {
    arch: {
        arm: {
            // Host or device variants with arm architecture
        },
        arm64: {
            // Host or device variants with arm64 architecture
        },
        x86: {
            // Host or device variants with x86 architecture
        },
        x86_64: {
            // Host or device variants with x86_64 architecture
        },
    },
    multilib: {
        lib32: {
            // Host or device variants for 32-bit architectures
        },
        lib64: {
            // Host or device variants for 64-bit architectures
        },
    },
    target: {
        android: {
            // Device variants (implies Bionic)
        },
        host: {
            // Host variants
        },
        bionic: {
            // Bionic (device and host) variants
        },
        linux_bionic: {
            // Bionic host variants
        },
        linux: {
            // Bionic (device and host) and Linux glibc variants
        },
        linux_glibc: {
            // Linux host variants (using non-Bionic libc)
        },
        darwin: {
            // Darwin host variants
        },
        windows: {
            // Windows host variants
        },
        not_windows: {
            // Non-windows host variants
        },
        android_arm: {
            // Any <os>_<arch> combination restricts to that os and arch
        },
    },
}
*/

// An Arch indicates a single CPU architecture.
type Arch struct {
	// The type of the architecture (arm, arm64, x86, or x86_64).
	ArchType ArchType

	// The variant of the architecture, for example "armv7-a" or "armv7-a-neon" for arm.
	ArchVariant string

	// The variant of the CPU, for example "cortex-a53" for arm64.
	CpuVariant string

	// The list of Android app ABIs supported by the CPU architecture, for example "arm64-v8a".
	Abi []string

	// The list of arch-specific features supported by the CPU architecture, for example "neon".
	ArchFeatures []string
}

// String returns the Arch as a string.  The value is used as the name of the variant created
// by archMutator.
func (a Arch) String() string {
	s := a.ArchType.String()
	if a.ArchVariant != "" {
		s += "_" + a.ArchVariant
	}
	if a.CpuVariant != "" {
		s += "_" + a.CpuVariant
	}
	return s
}

// ArchType is used to define the 4 supported architecture types (arm, arm64, x86, x86_64), as
// well as the "common" architecture used for modules that support multiple architectures, for
// example Java modules.
type ArchType struct {
	// Name is the name of the architecture type, "arm", "arm64", "x86", or "x86_64".
	Name string

	// Field is the name of the field used in properties that refer to the architecture, e.g. "Arm64".
	Field string

	// Multilib is either "lib32" or "lib64" for 32-bit or 64-bit architectures.
	Multilib string
}

// String returns the name of the ArchType.
func (a ArchType) String() string {
	return a.Name
}

func (a ArchType) Bitness() string {
	if a.Multilib == "lib32" {
		return "32"
	}
	if a.Multilib == "lib64" {
		return "64"
	}
	panic("Bitness is not defined for the common variant")
}

const COMMON_VARIANT = "common"

var (
	archTypeList []ArchType

	Arm     = newArch("arm", "lib32")
	Arm64   = newArch("arm64", "lib64")
	Riscv64 = newArch("riscv64", "lib64")
	X86     = newArch("x86", "lib32")
	X86_64  = newArch("x86_64", "lib64")

	Common = ArchType{
		Name: COMMON_VARIANT,
	}
)

var archTypeMap = map[string]ArchType{}

func newArch(name, multilib string) ArchType {
	archType := ArchType{
		Name:     name,
		Field:    proptools.FieldNameForProperty(name),
		Multilib: multilib,
	}
	archTypeList = append(archTypeList, archType)
	archTypeMap[name] = archType
	return archType
}

// ArchTypeList returns a slice copy of the 4 supported ArchTypes for arm,
// arm64, x86 and x86_64.
func ArchTypeList() []ArchType {
	return append([]ArchType(nil), archTypeList...)
}

// MarshalText allows an ArchType to be serialized through any encoder that supports
// encoding.TextMarshaler.
func (a ArchType) MarshalText() ([]byte, error) {
	return []byte(a.String()), nil
}

var _ encoding.TextMarshaler = ArchType{}

// UnmarshalText allows an ArchType to be deserialized through any decoder that supports
// encoding.TextUnmarshaler.
func (a *ArchType) UnmarshalText(text []byte) error {
	if u, ok := archTypeMap[string(text)]; ok {
		*a = u
		return nil
	}

	return fmt.Errorf("unknown ArchType %q", text)
}

var _ encoding.TextUnmarshaler = &ArchType{}

// OsClass is an enum that describes whether a variant of a module runs on the host, on the device,
// or is generic.
type OsClass int

const (
	// Generic is used for variants of modules that are not OS-specific.
	Generic OsClass = iota
	// Device is used for variants of modules that run on the device.
	Device
	// Host is used for variants of modules that run on the host.
	Host
)

// String returns the OsClass as a string.
func (class OsClass) String() string {
	switch class {
	case Generic:
		return "generic"
	case Device:
		return "device"
	case Host:
		return "host"
	default:
		panic(fmt.Errorf("unknown class %d", class))
	}
}

// OsType describes an OS variant of a module.
type OsType struct {
	// Name is the name of the OS.  It is also used as the name of the property in Android.bp
	// files.
	Name string

	// Field is the name of the OS converted to an exported field name, i.e. with the first
	// character capitalized.
	Field string

	// Class is the OsClass of the OS.
	Class OsClass

	// DefaultDisabled is set when the module variants for the OS should not be created unless
	// the module explicitly requests them.  This is used to limit Windows cross compilation to
	// only modules that need it.
	DefaultDisabled bool
}

// String returns the name of the OsType.
func (os OsType) String() string {
	return os.Name
}

// Bionic returns true if the OS uses the Bionic libc runtime, i.e. if the OS is Android or
// is Linux with Bionic.
func (os OsType) Bionic() bool {
	return os == Android || os == LinuxBionic
}

// Linux returns true if the OS uses the Linux kernel, i.e. if the OS is Android or is Linux
// with or without the Bionic libc runtime.
func (os OsType) Linux() bool {
	return os == Android || os == Linux || os == LinuxBionic || os == LinuxMusl
}

// newOsType constructs an OsType and adds it to the global lists.
func newOsType(name string, class OsClass, defDisabled bool, archTypes ...ArchType) OsType {
	checkCalledFromInit()
	os := OsType{
		Name:  name,
		Field: proptools.FieldNameForProperty(name),
		Class: class,

		DefaultDisabled: defDisabled,
	}
	osTypeList = append(osTypeList, os)

	if _, found := commonTargetMap[name]; found {
		panic(fmt.Errorf("Found Os type duplicate during OsType registration: %q", name))
	} else {
		commonTargetMap[name] = Target{Os: os, Arch: CommonArch}
	}
	osArchTypeMap[os] = archTypes

	return os
}

// osByName returns the OsType that has the given name, or NoOsType if none match.
func osByName(name string) OsType {
	for _, os := range osTypeList {
		if os.Name == name {
			return os
		}
	}

	return NoOsType
}

var (
	// osTypeList contains a list of all the supported OsTypes, including ones not supported
	// by the current build host or the target device.
	osTypeList []OsType
	// commonTargetMap maps names of OsTypes to the corresponding common Target, i.e. the
	// Target with the same OsType and the common ArchType.
	commonTargetMap = make(map[string]Target)
	// osArchTypeMap maps OsTypes to the list of supported ArchTypes for that OS.
	osArchTypeMap = map[OsType][]ArchType{}

	// NoOsType is a placeholder for when no OS is needed.
	NoOsType OsType
	// Linux is the OS for the Linux kernel plus the glibc runtime.
	Linux = newOsType("linux_glibc", Host, false, X86, X86_64)
	// LinuxMusl is the OS for the Linux kernel plus the musl runtime.
	LinuxMusl = newOsType("linux_musl", Host, false, X86, X86_64, Arm64, Arm)
	// Darwin is the OS for MacOS/Darwin host machines.
	Darwin = newOsType("darwin", Host, false, Arm64, X86_64)
	// LinuxBionic is the OS for the Linux kernel plus the Bionic libc runtime, but without the
	// rest of Android.
	LinuxBionic = newOsType("linux_bionic", Host, false, Arm64, X86_64)
	// Windows the OS for Windows host machines.
	Windows = newOsType("windows", Host, true, X86, X86_64)
	// Android is the OS for target devices that run all of Android, including the Linux kernel
	// and the Bionic libc runtime.
	Android = newOsType("android", Device, false, Arm, Arm64, Riscv64, X86, X86_64)

	// CommonOS is a pseudo OSType for a common OS variant, which is OsType agnostic and which
	// has dependencies on all the OS variants.
	CommonOS = newOsType("common_os", Generic, false)

	// CommonArch is the Arch for all modules that are os-specific but not arch specific,
	// for example most Java modules.
	CommonArch = Arch{ArchType: Common}
)

// OsTypeList returns a slice copy of the supported OsTypes.
func OsTypeList() []OsType {
	return append([]OsType(nil), osTypeList...)
}

// Target specifies the OS and architecture that a module is being compiled for.
type Target struct {
	// Os the OS that the module is being compiled for (e.g. "linux_glibc", "android").
	Os OsType
	// Arch is the architecture that the module is being compiled for.
	Arch Arch
	// NativeBridge is NativeBridgeEnabled if the architecture is supported using NativeBridge
	// (i.e. arm on x86) for this device.
	NativeBridge NativeBridgeSupport
	// NativeBridgeHostArchName is the name of the real architecture that is used to implement
	// the NativeBridge architecture.  For example, for arm on x86 this would be "x86".
	NativeBridgeHostArchName string
	// NativeBridgeRelativePath is the name of the subdirectory that will contain NativeBridge
	// libraries and binaries.
	NativeBridgeRelativePath string

	// HostCross is true when the target cannot run natively on the current build host.
	// For example, linux_glibc_x86 returns true on a regular x86/i686/Linux machines, but returns false
	// on Mac (different OS), or on 64-bit only i686/Linux machines (unsupported arch).
	HostCross bool
}

// NativeBridgeSupport is an enum that specifies if a Target supports NativeBridge.
type NativeBridgeSupport bool

const (
	NativeBridgeDisabled NativeBridgeSupport = false
	NativeBridgeEnabled  NativeBridgeSupport = true
)

// String returns the OS and arch variations used for the Target.
func (target Target) String() string {
	return target.OsVariation() + "_" + target.ArchVariation()
}

// OsVariation returns the name of the variation used by the osMutator for the Target.
func (target Target) OsVariation() string {
	return target.Os.String()
}

// ArchVariation returns the name of the variation used by the archMutator for the Target.
func (target Target) ArchVariation() string {
	var variation string
	if target.NativeBridge {
		variation = "native_bridge_"
	}
	variation += target.Arch.String()

	return variation
}

// Variations returns a list of blueprint.Variations for the osMutator and archMutator for the
// Target.
func (target Target) Variations() []blueprint.Variation {
	return []blueprint.Variation{
		{Mutator: "os", Variation: target.OsVariation()},
		{Mutator: "arch", Variation: target.ArchVariation()},
	}
}

// osMutator splits an arch-specific module into a variant for each OS that is enabled for the
// module.  It uses the HostOrDevice value passed to InitAndroidArchModule and the
// device_supported and host_supported properties to determine which OsTypes are enabled for this
// module, then searches through the Targets to determine which have enabled Targets for this
// module.
type osTransitionMutator struct{}

type allOsInfo struct {
	Os         map[string]OsType
	Variations []string
}

var allOsProvider = blueprint.NewMutatorProvider[*allOsInfo]("os_propagate")

// moduleOSList collects a list of OSTypes supported by this module based on the HostOrDevice
// value passed to InitAndroidArchModule and the device_supported and host_supported properties.
func moduleOSList(ctx ConfigContext, base *ModuleBase) []OsType {
	var moduleOSList []OsType
	for _, os := range osTypeList {
		for _, t := range ctx.Config().Targets[os] {
			if base.supportsTarget(t) {
				moduleOSList = append(moduleOSList, os)
				break
			}
		}
	}

	if base.commonProperties.CreateCommonOSVariant {
		// A CommonOS variant was requested so add it to the list of OS variants to
		// create. It needs to be added to the end because it needs to depend on the
		// the other variants and inter variant dependencies can only be created from a
		// later variant in that list to an earlier one. That is because variants are
		// always processed in the order in which they are created.
		moduleOSList = append(moduleOSList, CommonOS)
	}

	return moduleOSList
}

func (o *osTransitionMutator) Split(ctx BaseModuleContext) []string {
	module := ctx.Module()
	base := module.base()

	// Nothing to do for modules that are not architecture specific (e.g. a genrule).
	if !base.ArchSpecific() {
		return []string{""}
	}

	moduleOSList := moduleOSList(ctx, base)

	// If there are no supported OSes then disable the module.
	if len(moduleOSList) == 0 {
		base.Disable()
		return []string{""}
	}

	// Convert the list of supported OsTypes to the variation names.
	osNames := make([]string, len(moduleOSList))
	osMapping := make(map[string]OsType, len(moduleOSList))
	for i, os := range moduleOSList {
		osNames[i] = os.String()
		osMapping[osNames[i]] = os
	}

	SetProvider(ctx, allOsProvider, &allOsInfo{
		Os:         osMapping,
		Variations: osNames,
	})

	return osNames
}

func (o *osTransitionMutator) OutgoingTransition(ctx OutgoingTransitionContext, sourceVariation string) string {
	return sourceVariation
}

func (o *osTransitionMutator) IncomingTransition(ctx IncomingTransitionContext, incomingVariation string) string {
	module := ctx.Module()
	base := module.base()

	if !base.ArchSpecific() {
		return ""
	}

	return incomingVariation
}

func (o *osTransitionMutator) Mutate(ctx BottomUpMutatorContext, variation string) {
	module := ctx.Module()
	base := module.base()

	if variation == "" {
		return
	}

	allOsInfo, ok := ModuleProvider(ctx, allOsProvider)
	if !ok {
		panic(fmt.Errorf("missing allOsProvider"))
	}

	// Annotate this variant with which OS it was created for, and
	// squash the appropriate OS-specific properties into the top level properties.
	base.commonProperties.CompileOS = allOsInfo.Os[variation]
	base.setOSProperties(ctx)

	if variation == CommonOS.String() {
		// A CommonOS variant was requested so add dependencies from it (the last one in
		// the list) to the OS type specific variants.
		osList := allOsInfo.Variations[:len(allOsInfo.Variations)-1]
		for _, os := range osList {
			variation := []blueprint.Variation{{"os", os}}
			ctx.AddVariationDependencies(variation, commonOsToOsSpecificVariantTag, ctx.ModuleName())
		}
	}
}

type archDepTag struct {
	blueprint.BaseDependencyTag
	name string
}

// Identifies the dependency from CommonOS variant to the os specific variants.
var commonOsToOsSpecificVariantTag = archDepTag{name: "common os to os specific"}

// Get the OsType specific variants for the current CommonOS variant.
//
// The returned list will only contain enabled OsType specific variants of the
// module referenced in the supplied context. An empty list is returned if there
// are no enabled variants or the supplied context is not for an CommonOS
// variant.
func GetOsSpecificVariantsOfCommonOSVariant(mctx BaseModuleContext) []Module {
	var variants []Module
	mctx.VisitDirectDeps(func(m Module) {
		if mctx.OtherModuleDependencyTag(m) == commonOsToOsSpecificVariantTag {
			if m.Enabled(mctx) {
				variants = append(variants, m)
			}
		}
	})
	return variants
}

var DarwinUniversalVariantTag = archDepTag{name: "darwin universal binary"}

// archTransitionMutator splits a module into a variant for each Target requested by the module.  Target selection
// for a module is in three levels, OsClass, multilib, and then Target.
// OsClass selection is determined by:
//   - The HostOrDeviceSupported value passed in to InitAndroidArchModule by the module type factory, which selects
//     whether the module type can compile for host, device or both.
//   - The host_supported and device_supported properties on the module.
//
// If host is supported for the module, the Host and HostCross OsClasses are selected.  If device is supported
// for the module, the Device OsClass is selected.
// Within each selected OsClass, the multilib selection is determined by:
//   - The compile_multilib property if it set (which may be overridden by target.android.compile_multilib or
//     target.host.compile_multilib).
//   - The default multilib passed to InitAndroidArchModule if compile_multilib was not set.
//
// Valid multilib values include:
//
//	"both": compile for all Targets supported by the OsClass (generally x86_64 and x86, or arm64 and arm).
//	"first": compile for only a single preferred Target supported by the OsClass.  This is generally x86_64 or arm64,
//	    but may be arm for a 32-bit only build.
//	"32": compile for only a single 32-bit Target supported by the OsClass.
//	"64": compile for only a single 64-bit Target supported by the OsClass.
//	"common": compile a for a single Target that will work on all Targets supported by the OsClass (for example Java).
//
// Once the list of Targets is determined, the module is split into a variant for each Target.
//
// Modules can be initialized with InitAndroidMultiTargetsArchModule, in which case they will be split by OsClass,
// but will have a common Target that is expected to handle all other selected Targets via ctx.MultiTargets().
type archTransitionMutator struct{}

type allArchInfo struct {
	Targets      map[string]Target
	MultiTargets []Target
	Primary      string
	Multilib     string
}

var allArchProvider = blueprint.NewMutatorProvider[*allArchInfo]("arch_propagate")

func (a *archTransitionMutator) Split(ctx BaseModuleContext) []string {
	module := ctx.Module()
	base := module.base()

	if !base.ArchSpecific() {
		return []string{""}
	}

	os := base.commonProperties.CompileOS
	if os == CommonOS {
		// Do not create arch specific variants for the CommonOS variant.
		return []string{""}
	}

	osTargets := ctx.Config().Targets[os]

	image := base.commonProperties.ImageVariation
	// Filter NativeBridge targets unless they are explicitly supported.
	// Skip creating native bridge variants for non-core modules.
	if os == Android && !(base.IsNativeBridgeSupported() && image == CoreVariation) {
		osTargets = slices.DeleteFunc(slices.Clone(osTargets), func(t Target) bool {
			return bool(t.NativeBridge)
		})
	}

	// Filter HostCross targets if disabled.
	if base.HostSupported() && !base.HostCrossSupported() {
		osTargets = slices.DeleteFunc(slices.Clone(osTargets), func(t Target) bool {
			return t.HostCross
		})
	}

	// only the primary arch in the ramdisk / vendor_ramdisk / recovery partition
	if os == Android && (module.InstallInRecovery() || module.InstallInRamdisk() || module.InstallInVendorRamdisk() || module.InstallInDebugRamdisk()) {
		osTargets = []Target{osTargets[0]}
	}

	// Windows builds always prefer 32-bit
	prefer32 := os == Windows

	// Determine the multilib selection for this module.
	multilib, extraMultilib := decodeMultilib(ctx, base)

	// Convert the multilib selection into a list of Targets.
	targets, err := decodeMultilibTargets(multilib, osTargets, prefer32)
	if err != nil {
		ctx.ModuleErrorf("%s", err.Error())
	}

	// If there are no supported targets disable the module.
	if len(targets) == 0 {
		base.Disable()
		return []string{""}
	}

	// If the module is using extraMultilib, decode the extraMultilib selection into
	// a separate list of Targets.
	var multiTargets []Target
	if extraMultilib != "" {
		multiTargets, err = decodeMultilibTargets(extraMultilib, osTargets, prefer32)
		if err != nil {
			ctx.ModuleErrorf("%s", err.Error())
		}
		multiTargets = filterHostCross(multiTargets, targets[0].HostCross)
	}

	// Recovery is always the primary architecture, filter out any other architectures.
	// Common arch is also allowed
	if image == RecoveryVariation {
		primaryArch := ctx.Config().DevicePrimaryArchType()
		targets = filterToArch(targets, primaryArch, Common)
		multiTargets = filterToArch(multiTargets, primaryArch, Common)
	}

	// If there are no supported targets disable the module.
	if len(targets) == 0 {
		base.Disable()
		return []string{""}
	}

	// Convert the targets into a list of arch variation names.
	targetNames := make([]string, len(targets))
	targetMapping := make(map[string]Target, len(targets))
	for i, target := range targets {
		targetNames[i] = target.ArchVariation()
		targetMapping[targetNames[i]] = targets[i]
	}

	SetProvider(ctx, allArchProvider, &allArchInfo{
		Targets:      targetMapping,
		MultiTargets: multiTargets,
		Primary:      targetNames[0],
		Multilib:     multilib,
	})
	return targetNames
}

func (a *archTransitionMutator) OutgoingTransition(ctx OutgoingTransitionContext, sourceVariation string) string {
	return sourceVariation
}

func (a *archTransitionMutator) IncomingTransition(ctx IncomingTransitionContext, incomingVariation string) string {
	module := ctx.Module()
	base := module.base()

	if !base.ArchSpecific() {
		return ""
	}

	os := base.commonProperties.CompileOS
	if os == CommonOS {
		// Do not create arch specific variants for the CommonOS variant.
		return ""
	}

	multilib, _ := decodeMultilib(ctx, base)
	if multilib == "common" {
		return "common"
	}
	return incomingVariation
}

func (a *archTransitionMutator) Mutate(ctx BottomUpMutatorContext, variation string) {
	module := ctx.Module()
	base := module.base()
	os := base.commonProperties.CompileOS

	if os == CommonOS {
		// Make sure that the target related properties are initialized for the
		// CommonOS variant.
		addTargetProperties(module, commonTargetMap[os.Name], nil, true)
		return
	}

	if variation == "" {
		return
	}

	if !base.ArchSpecific() {
		panic(fmt.Errorf("found variation %q for non arch specifc module", variation))
	}

	allArchInfo, ok := ModuleProvider(ctx, allArchProvider)
	if !ok {
		return
	}

	target, ok := allArchInfo.Targets[variation]
	if !ok {
		panic(fmt.Errorf("missing Target for %q", variation))
	}
	primary := variation == allArchInfo.Primary
	multiTargets := allArchInfo.MultiTargets

	// Annotate the new variant with which Target it was created for, and
	// squash the appropriate arch-specific properties into the top level properties.
	addTargetProperties(ctx.Module(), target, multiTargets, primary)
	base.setArchProperties(ctx)

	// Install support doesn't understand Darwin+Arm64
	if os == Darwin && target.HostCross {
		base.commonProperties.SkipInstall = true
	}

	// Create a dependency for Darwin Universal binaries from the primary to secondary
	// architecture. The module itself will be responsible for calling lipo to merge the outputs.
	if os == Darwin {
		isUniversalBinary := allArchInfo.Multilib == "darwin_universal" && len(allArchInfo.Targets) == 2
		isPrimary := variation == ctx.Config().BuildArch.String()
		hasSecondaryConfigured := len(ctx.Config().Targets[Darwin]) > 1
		if isUniversalBinary && isPrimary && hasSecondaryConfigured {
			secondaryArch := ctx.Config().Targets[Darwin][1].Arch.String()
			variation := []blueprint.Variation{{"arch", secondaryArch}}
			ctx.AddVariationDependencies(variation, DarwinUniversalVariantTag, ctx.ModuleName())
		}
	}

}

// addTargetProperties annotates a variant with the Target is is being compiled for, the list
// of additional Targets it is supporting (if any), and whether it is the primary Target for
// the module.
func addTargetProperties(m Module, target Target, multiTargets []Target, primaryTarget bool) {
	m.base().commonProperties.CompileTarget = target
	m.base().commonProperties.CompileMultiTargets = multiTargets
	m.base().commonProperties.CompilePrimary = primaryTarget
	m.base().commonProperties.ArchReady = true
}

// decodeMultilib returns the appropriate compile_multilib property for the module, or the default
// multilib from the factory's call to InitAndroidArchModule if none was set.  For modules that
// called InitAndroidMultiTargetsArchModule it always returns "common" for multilib, and returns
// the actual multilib in extraMultilib.
func decodeMultilib(ctx ConfigContext, base *ModuleBase) (multilib, extraMultilib string) {
	os := base.commonProperties.CompileOS
	ignorePrefer32OnDevice := ctx.Config().IgnorePrefer32OnDevice()
	// First check the "android.compile_multilib" or "host.compile_multilib" properties.
	switch os.Class {
	case Device:
		multilib = String(base.commonProperties.Target.Android.Compile_multilib)
	case Host:
		multilib = String(base.commonProperties.Target.Host.Compile_multilib)
	}

	// If those aren't set, try the "compile_multilib" property.
	if multilib == "" {
		multilib = String(base.commonProperties.Compile_multilib)
	}

	// If that wasn't set, use the default multilib set by the factory.
	if multilib == "" {
		multilib = base.commonProperties.Default_multilib
	}

	// If a device is configured with multiple targets, this option
	// force all device targets that prefer32 to be compiled only as
	// the first target.
	if ignorePrefer32OnDevice && os.Class == Device && (multilib == "prefer32" || multilib == "first_prefer32") {
		multilib = "first"
	}

	if base.commonProperties.UseTargetVariants {
		// Darwin has the concept of "universal binaries" which is implemented in Soong by
		// building both x86_64 and arm64 variants, and having select module types know how to
		// merge the outputs of their corresponding variants together into a final binary. Most
		// module types don't need to understand this logic, as we only build a small portion
		// of the tree for Darwin, and only module types writing macho files need to do the
		// merging.
		//
		// This logic is not enabled for:
		//  "common", as it's not an arch-specific variant
		//  "32", as Darwin never has a 32-bit variant
		//  !UseTargetVariants, as the module has opted into handling the arch-specific logic on
		//    its own.
		if os == Darwin && multilib != "common" && multilib != "32" {
			multilib = "darwin_universal"
		}

		return multilib, ""
	} else {
		// For app modules a single arch variant will be created per OS class which is expected to handle all the
		// selected arches.  Return the common-type as multilib and any Android.bp provided multilib as extraMultilib
		if multilib == base.commonProperties.Default_multilib {
			multilib = "first"
		}
		return base.commonProperties.Default_multilib, multilib
	}
}

// filterToArch takes a list of Targets and an ArchType, and returns a modified list that contains
// only Targets that have the specified ArchTypes.
func filterToArch(targets []Target, archs ...ArchType) []Target {
	for i := 0; i < len(targets); i++ {
		found := false
		for _, arch := range archs {
			if targets[i].Arch.ArchType == arch {
				found = true
				break
			}
		}
		if !found {
			targets = append(targets[:i], targets[i+1:]...)
			i--
		}
	}
	return targets
}

// filterHostCross takes a list of Targets and a hostCross value, and returns a modified list
// that contains only Targets that have the specified HostCross.
func filterHostCross(targets []Target, hostCross bool) []Target {
	for i := 0; i < len(targets); i++ {
		if targets[i].HostCross != hostCross {
			targets = append(targets[:i], targets[i+1:]...)
			i--
		}
	}
	return targets
}

// archPropRoot is a struct type used as the top level of the arch-specific properties.  It
// contains the "arch", "multilib", and "target" property structs.  It is used to split up the
// property structs to limit how much is allocated when a single arch-specific property group is
// used.  The types are interface{} because they will hold instances of runtime-created types.
type archPropRoot struct {
	Arch, Multilib, Target interface{}
}

// archPropTypeDesc holds the runtime-created types for the property structs to instantiate to
// create an archPropRoot property struct.
type archPropTypeDesc struct {
	arch, multilib, target reflect.Type
}

// createArchPropTypeDesc takes a reflect.Type that is either a struct or a pointer to a struct, and
// returns lists of reflect.Types that contains the arch-variant properties inside structs for each
// arch, multilib and target property.
//
// This is a relatively expensive operation, so the results are cached in the global
// archPropTypeMap.  It is constructed entirely based on compile-time data, so there is no need
// to isolate the results between multiple tests running in parallel.
func createArchPropTypeDesc(props reflect.Type) []archPropTypeDesc {
	// Each property struct shard will be nested many times under the runtime generated arch struct,
	// which can hit the limit of 64kB for the name of runtime generated structs.  They are nested
	// 97 times now, which may grow in the future, plus there is some overhead for the containing
	// type.  This number may need to be reduced if too many are added, but reducing it too far
	// could cause problems if a single deeply nested property no longer fits in the name.
	const maxArchTypeNameSize = 500

	// Convert the type to a new set of types that contains only the arch-specific properties
	// (those that are tagged with `android:"arch_variant"`), and sharded into multiple types
	// to keep the runtime-generated names under the limit.
	propShards, _ := proptools.FilterPropertyStructSharded(props, maxArchTypeNameSize, filterArchStruct)

	// If the type has no arch-specific properties there is nothing to do.
	if len(propShards) == 0 {
		return nil
	}

	var ret []archPropTypeDesc
	for _, props := range propShards {

		// variantFields takes a list of variant property field names and returns a list the
		// StructFields with the names and the type of the current shard.
		variantFields := func(names []string) []reflect.StructField {
			ret := make([]reflect.StructField, len(names))

			for i, name := range names {
				ret[i].Name = name
				ret[i].Type = props
			}

			return ret
		}

		// Create a type that contains the properties in this shard repeated for each
		// architecture, architecture variant, and architecture feature.
		archFields := make([]reflect.StructField, len(archTypeList))
		for i, arch := range archTypeList {
			var variants []string

			for _, archVariant := range archVariants[arch] {
				archVariant := variantReplacer.Replace(archVariant)
				variants = append(variants, proptools.FieldNameForProperty(archVariant))
			}
			for _, cpuVariant := range cpuVariants[arch] {
				cpuVariant := variantReplacer.Replace(cpuVariant)
				variants = append(variants, proptools.FieldNameForProperty(cpuVariant))
			}
			for _, feature := range archFeatures[arch] {
				feature := variantReplacer.Replace(feature)
				variants = append(variants, proptools.FieldNameForProperty(feature))
			}

			// Create the StructFields for each architecture variant architecture feature
			// (e.g. "arch.arm.cortex-a53" or "arch.arm.neon").
			fields := variantFields(variants)

			// Create the StructField for the architecture itself (e.g. "arch.arm").  The special
			// "BlueprintEmbed" name is used by Blueprint to put the properties in the
			// parent struct.
			fields = append([]reflect.StructField{{
				Name:      "BlueprintEmbed",
				Type:      props,
				Anonymous: true,
			}}, fields...)

			archFields[i] = reflect.StructField{
				Name: arch.Field,
				Type: reflect.StructOf(fields),
			}
		}

		// Create the type of the "arch" property struct for this shard.
		archType := reflect.StructOf(archFields)

		// Create the type for the "multilib" property struct for this shard, containing the
		// "multilib.lib32" and "multilib.lib64" property structs.
		multilibType := reflect.StructOf(variantFields([]string{"Lib32", "Lib64"}))

		// Start with a list of the special targets
		targets := []string{
			"Host",
			"Android64",
			"Android32",
			"Bionic",
			"Glibc",
			"Musl",
			"Linux",
			"Host_linux",
			"Not_windows",
			"Arm_on_x86",
			"Arm_on_x86_64",
			"Native_bridge",
		}
		for _, os := range osTypeList {
			// Add all the OSes.
			targets = append(targets, os.Field)

			// Add the OS/Arch combinations, e.g. "android_arm64".
			for _, archType := range osArchTypeMap[os] {
				targets = append(targets, GetCompoundTargetField(os, archType))

				// Also add the special "linux_<arch>", "bionic_<arch>" , "glibc_<arch>", and
				// "musl_<arch>" property structs.
				if os.Linux() {
					target := "Linux_" + archType.Name
					if !InList(target, targets) {
						targets = append(targets, target)
					}
				}
				if os.Linux() && os.Class == Host {
					target := "Host_linux_" + archType.Name
					if !InList(target, targets) {
						targets = append(targets, target)
					}
				}
				if os.Bionic() {
					target := "Bionic_" + archType.Name
					if !InList(target, targets) {
						targets = append(targets, target)
					}
				}
				if os == Linux {
					target := "Glibc_" + archType.Name
					if !InList(target, targets) {
						targets = append(targets, target)
					}
				}
				if os == LinuxMusl {
					target := "Musl_" + archType.Name
					if !InList(target, targets) {
						targets = append(targets, target)
					}
				}
			}
		}

		// Create the type for the "target" property struct for this shard.
		targetType := reflect.StructOf(variantFields(targets))

		// Return a descriptor of the 3 runtime-created types.
		ret = append(ret, archPropTypeDesc{
			arch:     reflect.PtrTo(archType),
			multilib: reflect.PtrTo(multilibType),
			target:   reflect.PtrTo(targetType),
		})
	}
	return ret
}

// variantReplacer converts architecture variant or architecture feature names into names that
// are valid for an Android.bp file.
var variantReplacer = strings.NewReplacer("-", "_", ".", "_")

// filterArchStruct returns true if the given field is an architecture specific property.
func filterArchStruct(field reflect.StructField, prefix string) (bool, reflect.StructField) {
	if proptools.HasTag(field, "android", "arch_variant") {
		// The arch_variant field isn't necessary past this point
		// Instead of wasting space, just remove it. Go also has a
		// 16-bit limit on structure name length. The name is constructed
		// based on the Go source representation of the structure, so
		// the tag names count towards that length.

		androidTag := field.Tag.Get("android")
		values := strings.Split(androidTag, ",")

		if string(field.Tag) != `android:"`+strings.Join(values, ",")+`"` {
			panic(fmt.Errorf("unexpected tag format %q", field.Tag))
		}
		// these tags don't need to be present in the runtime generated struct type.
		// However replace_instead_of_append does, because it's read by the blueprint
		// property extending util functions, which can operate on these generated arch
		// property structs.
		values = RemoveListFromList(values, []string{"arch_variant", "variant_prepend", "path"})
		if len(values) > 0 {
			if values[0] != "replace_instead_of_append" || len(values) > 1 {
				panic(fmt.Errorf("unknown tags %q in field %q", values, prefix+field.Name))
			}
			field.Tag = `android:"replace_instead_of_append"`
		} else {
			field.Tag = ``
		}
		return true, field
	}
	return false, field
}

// archPropTypeMap contains a cache of the results of createArchPropTypeDesc for each type.  It is
// shared across all Contexts, but is constructed based only on compile-time information so there
// is no risk of contaminating one Context with data from another.
var archPropTypeMap OncePer

// initArchModule adds the architecture-specific property structs to a Module.
func initArchModule(m Module) {

	base := m.base()

	if len(base.archProperties) != 0 {
		panic(fmt.Errorf("module %s already has archProperties", m.Name()))
	}

	getStructType := func(properties interface{}) reflect.Type {
		propertiesValue := reflect.ValueOf(properties)
		t := propertiesValue.Type()
		if propertiesValue.Kind() != reflect.Ptr {
			panic(fmt.Errorf("properties must be a pointer to a struct, got %T",
				propertiesValue.Interface()))
		}

		propertiesValue = propertiesValue.Elem()
		if propertiesValue.Kind() != reflect.Struct {
			panic(fmt.Errorf("properties must be a pointer to a struct, got a pointer to %T",
				propertiesValue.Interface()))
		}
		return t
	}

	for _, properties := range m.GetProperties() {
		t := getStructType(properties)
		// Get or create the arch-specific property struct types for this property struct type.
		archPropTypes := archPropTypeMap.Once(NewCustomOnceKey(t), func() interface{} {
			return createArchPropTypeDesc(t)
		}).([]archPropTypeDesc)

		// Instantiate one of each arch-specific property struct type and add it to the
		// properties for the Module.
		var archProperties []interface{}
		for _, t := range archPropTypes {
			archProperties = append(archProperties, &archPropRoot{
				Arch:     reflect.Zero(t.arch).Interface(),
				Multilib: reflect.Zero(t.multilib).Interface(),
				Target:   reflect.Zero(t.target).Interface(),
			})
		}
		base.archProperties = append(base.archProperties, archProperties)
		m.AddProperties(archProperties...)
	}

}

func maybeBlueprintEmbed(src reflect.Value) reflect.Value {
	// If the value of the field is a struct (as opposed to a pointer to a struct) then step
	// into the BlueprintEmbed field.
	if src.Kind() == reflect.Struct {
		return src.FieldByName("BlueprintEmbed")
	} else {
		return src
	}
}

// Merges the property struct in srcValue into dst.
func mergePropertyStruct(ctx ArchVariantContext, dst interface{}, srcValue reflect.Value) {
	src := maybeBlueprintEmbed(srcValue).Interface()

	// order checks the `android:"variant_prepend"` tag to handle properties where the
	// arch-specific value needs to come before the generic value, for example for lists of
	// include directories.
	order := func(dstField, srcField reflect.StructField) (proptools.Order, error) {
		if proptools.HasTag(dstField, "android", "variant_prepend") {
			return proptools.Prepend, nil
		} else {
			return proptools.Append, nil
		}
	}

	// Squash the located property struct into the destination property struct.
	err := proptools.ExtendMatchingProperties([]interface{}{dst}, src, nil, order)
	if err != nil {
		if propertyErr, ok := err.(*proptools.ExtendPropertyError); ok {
			ctx.PropertyErrorf(propertyErr.Property, "%s", propertyErr.Err.Error())
		} else {
			panic(err)
		}
	}
}

// Returns the immediate child of the input property struct that corresponds to
// the sub-property "field".
func getChildPropertyStruct(ctx ArchVariantContext,
	src reflect.Value, field, userFriendlyField string) (reflect.Value, bool) {

	// Step into non-nil pointers to structs in the src value.
	if src.Kind() == reflect.Ptr {
		if src.IsNil() {
			return reflect.Value{}, false
		}
		src = src.Elem()
	}

	// Find the requested field in the src struct.
	child := src.FieldByName(proptools.FieldNameForProperty(field))
	if !child.IsValid() {
		ctx.ModuleErrorf("field %q does not exist", userFriendlyField)
		return reflect.Value{}, false
	}

	if child.IsZero() {
		return reflect.Value{}, false
	}

	return child, true
}

// Squash the appropriate OS-specific property structs into the matching top level property structs
// based on the CompileOS value that was annotated on the variant.
func (m *ModuleBase) setOSProperties(ctx BottomUpMutatorContext) {
	os := m.commonProperties.CompileOS

	for i := range m.archProperties {
		genProps := m.GetProperties()[i]
		if m.archProperties[i] == nil {
			continue
		}
		for _, archProperties := range m.archProperties[i] {
			archPropValues := reflect.ValueOf(archProperties).Elem()

			targetProp := archPropValues.FieldByName("Target").Elem()

			// Handle host-specific properties in the form:
			// target: {
			//     host: {
			//         key: value,
			//     },
			// },
			if os.Class == Host {
				field := "Host"
				prefix := "target.host"
				if hostProperties, ok := getChildPropertyStruct(ctx, targetProp, field, prefix); ok {
					mergePropertyStruct(ctx, genProps, hostProperties)
				}
			}

			// Handle target OS generalities of the form:
			// target: {
			//     bionic: {
			//         key: value,
			//     },
			// }
			if os.Linux() {
				field := "Linux"
				prefix := "target.linux"
				if linuxProperties, ok := getChildPropertyStruct(ctx, targetProp, field, prefix); ok {
					mergePropertyStruct(ctx, genProps, linuxProperties)
				}
			}

			if os.Linux() && os.Class == Host {
				field := "Host_linux"
				prefix := "target.host_linux"
				if linuxProperties, ok := getChildPropertyStruct(ctx, targetProp, field, prefix); ok {
					mergePropertyStruct(ctx, genProps, linuxProperties)
				}
			}

			if os.Bionic() {
				field := "Bionic"
				prefix := "target.bionic"
				if bionicProperties, ok := getChildPropertyStruct(ctx, targetProp, field, prefix); ok {
					mergePropertyStruct(ctx, genProps, bionicProperties)
				}
			}

			if os == Linux {
				field := "Glibc"
				prefix := "target.glibc"
				if bionicProperties, ok := getChildPropertyStruct(ctx, targetProp, field, prefix); ok {
					mergePropertyStruct(ctx, genProps, bionicProperties)
				}
			}

			if os == LinuxMusl {
				field := "Musl"
				prefix := "target.musl"
				if bionicProperties, ok := getChildPropertyStruct(ctx, targetProp, field, prefix); ok {
					mergePropertyStruct(ctx, genProps, bionicProperties)
				}
			}

			// Handle target OS properties in the form:
			// target: {
			//     linux_glibc: {
			//         key: value,
			//     },
			//     not_windows: {
			//         key: value,
			//     },
			//     android {
			//         key: value,
			//     },
			// },
			field := os.Field
			prefix := "target." + os.Name
			if osProperties, ok := getChildPropertyStruct(ctx, targetProp, field, prefix); ok {
				mergePropertyStruct(ctx, genProps, osProperties)
			}

			if os.Class == Host && os != Windows {
				field := "Not_windows"
				prefix := "target.not_windows"
				if notWindowsProperties, ok := getChildPropertyStruct(ctx, targetProp, field, prefix); ok {
					mergePropertyStruct(ctx, genProps, notWindowsProperties)
				}
			}

			// Handle 64-bit device properties in the form:
			// target {
			//     android64 {
			//         key: value,
			//     },
			//     android32 {
			//         key: value,
			//     },
			// },
			// WARNING: this is probably not what you want to use in your blueprints file, it selects
			// options for all targets on a device that supports 64-bit binaries, not just the targets
			// that are being compiled for 64-bit.  Its expected use case is binaries like linker and
			// debuggerd that need to know when they are a 32-bit process running on a 64-bit device
			if os.Class == Device {
				if ctx.Config().Android64() {
					field := "Android64"
					prefix := "target.android64"
					if android64Properties, ok := getChildPropertyStruct(ctx, targetProp, field, prefix); ok {
						mergePropertyStruct(ctx, genProps, android64Properties)
					}
				} else {
					field := "Android32"
					prefix := "target.android32"
					if android32Properties, ok := getChildPropertyStruct(ctx, targetProp, field, prefix); ok {
						mergePropertyStruct(ctx, genProps, android32Properties)
					}
				}
			}
		}
	}
}

// Returns the struct containing the properties specific to the given
// architecture type. These look like this in Blueprint files:
//
//	arch: {
//	    arm64: {
//	        key: value,
//	    },
//	},
//
// This struct will also contain sub-structs containing to the architecture/CPU
// variants and features that themselves contain properties specific to those.
func getArchTypeStruct(ctx ArchVariantContext, archProperties interface{}, archType ArchType) (reflect.Value, bool) {
	archPropValues := reflect.ValueOf(archProperties).Elem()
	archProp := archPropValues.FieldByName("Arch").Elem()
	prefix := "arch." + archType.Name
	return getChildPropertyStruct(ctx, archProp, archType.Name, prefix)
}

// Returns the struct containing the properties specific to a given multilib
// value. These look like this in the Blueprint file:
//
//	multilib: {
//	    lib32: {
//	        key: value,
//	    },
//	},
func getMultilibStruct(ctx ArchVariantContext, archProperties interface{}, archType ArchType) (reflect.Value, bool) {
	archPropValues := reflect.ValueOf(archProperties).Elem()
	multilibProp := archPropValues.FieldByName("Multilib").Elem()
	return getChildPropertyStruct(ctx, multilibProp, archType.Multilib, "multilib."+archType.Multilib)
}

func GetCompoundTargetField(os OsType, arch ArchType) string {
	return os.Field + "_" + arch.Name
}

// Returns the structs corresponding to the properties specific to the given
// architecture and OS in archProperties.
func getArchProperties(ctx BaseModuleContext, archProperties interface{}, arch Arch, os OsType, nativeBridgeEnabled bool) []reflect.Value {
	result := make([]reflect.Value, 0)
	archPropValues := reflect.ValueOf(archProperties).Elem()

	targetProp := archPropValues.FieldByName("Target").Elem()

	archType := arch.ArchType

	if arch.ArchType != Common {
		archStruct, ok := getArchTypeStruct(ctx, archProperties, arch.ArchType)
		if ok {
			result = append(result, archStruct)

			// Handle arch-variant-specific properties in the form:
			// arch: {
			//     arm: {
			//         variant: {
			//             key: value,
			//         },
			//     },
			// },
			v := variantReplacer.Replace(arch.ArchVariant)
			if v != "" {
				prefix := "arch." + archType.Name + "." + v
				if variantProperties, ok := getChildPropertyStruct(ctx, archStruct, v, prefix); ok {
					result = append(result, variantProperties)
				}
			}

			// Handle cpu-variant-specific properties in the form:
			// arch: {
			//     arm: {
			//         variant: {
			//             key: value,
			//         },
			//     },
			// },
			if arch.CpuVariant != arch.ArchVariant {
				c := variantReplacer.Replace(arch.CpuVariant)
				if c != "" {
					prefix := "arch." + archType.Name + "." + c
					if cpuVariantProperties, ok := getChildPropertyStruct(ctx, archStruct, c, prefix); ok {
						result = append(result, cpuVariantProperties)
					}
				}
			}

			// Handle arch-feature-specific properties in the form:
			// arch: {
			//     arm: {
			//         feature: {
			//             key: value,
			//         },
			//     },
			// },
			for _, feature := range arch.ArchFeatures {
				prefix := "arch." + archType.Name + "." + feature
				if featureProperties, ok := getChildPropertyStruct(ctx, archStruct, feature, prefix); ok {
					result = append(result, featureProperties)
				}
			}
		}

		if multilibProperties, ok := getMultilibStruct(ctx, archProperties, archType); ok {
			result = append(result, multilibProperties)
		}

		// Handle combined OS-feature and arch specific properties in the form:
		// target: {
		//     bionic_x86: {
		//         key: value,
		//     },
		// }
		if os.Linux() {
			field := "Linux_" + arch.ArchType.Name
			userFriendlyField := "target.linux_" + arch.ArchType.Name
			if linuxProperties, ok := getChildPropertyStruct(ctx, targetProp, field, userFriendlyField); ok {
				result = append(result, linuxProperties)
			}
		}

		if os.Bionic() {
			field := "Bionic_" + archType.Name
			userFriendlyField := "target.bionic_" + archType.Name
			if bionicProperties, ok := getChildPropertyStruct(ctx, targetProp, field, userFriendlyField); ok {
				result = append(result, bionicProperties)
			}
		}

		// Handle combined OS and arch specific properties in the form:
		// target: {
		//     linux_glibc_x86: {
		//         key: value,
		//     },
		//     linux_glibc_arm: {
		//         key: value,
		//     },
		//     android_arm {
		//         key: value,
		//     },
		//     android_x86 {
		//         key: value,
		//     },
		// },
		field := GetCompoundTargetField(os, archType)
		userFriendlyField := "target." + os.Name + "_" + archType.Name
		if osArchProperties, ok := getChildPropertyStruct(ctx, targetProp, field, userFriendlyField); ok {
			result = append(result, osArchProperties)
		}

		if os == Linux {
			field := "Glibc_" + archType.Name
			userFriendlyField := "target.glibc_" + "_" + archType.Name
			if osArchProperties, ok := getChildPropertyStruct(ctx, targetProp, field, userFriendlyField); ok {
				result = append(result, osArchProperties)
			}
		}

		if os == LinuxMusl {
			field := "Musl_" + archType.Name
			userFriendlyField := "target.musl_" + "_" + archType.Name
			if osArchProperties, ok := getChildPropertyStruct(ctx, targetProp, field, userFriendlyField); ok {
				result = append(result, osArchProperties)
			}
		}
	}

	// Handle arm on x86 properties in the form:
	// target {
	//     arm_on_x86 {
	//         key: value,
	//     },
	//     arm_on_x86_64 {
	//         key: value,
	//     },
	// },
	if os.Class == Device {
		if arch.ArchType == X86 && (hasArmAbi(arch) ||
			hasArmAndroidArch(ctx.Config().Targets[Android])) {
			field := "Arm_on_x86"
			userFriendlyField := "target.arm_on_x86"
			if armOnX86Properties, ok := getChildPropertyStruct(ctx, targetProp, field, userFriendlyField); ok {
				result = append(result, armOnX86Properties)
			}
		}
		if arch.ArchType == X86_64 && (hasArmAbi(arch) ||
			hasArmAndroidArch(ctx.Config().Targets[Android])) {
			field := "Arm_on_x86_64"
			userFriendlyField := "target.arm_on_x86_64"
			if armOnX8664Properties, ok := getChildPropertyStruct(ctx, targetProp, field, userFriendlyField); ok {
				result = append(result, armOnX8664Properties)
			}
		}
		if os == Android && nativeBridgeEnabled {
			userFriendlyField := "Native_bridge"
			prefix := "target.native_bridge"
			if nativeBridgeProperties, ok := getChildPropertyStruct(ctx, targetProp, userFriendlyField, prefix); ok {
				result = append(result, nativeBridgeProperties)
			}
		}
	}

	return result
}

// Squash the appropriate arch-specific property structs into the matching top level property
// structs based on the CompileTarget value that was annotated on the variant.
func (m *ModuleBase) setArchProperties(ctx BottomUpMutatorContext) {
	arch := m.Arch()
	os := m.Os()

	for i := range m.archProperties {
		genProps := m.GetProperties()[i]
		if m.archProperties[i] == nil {
			continue
		}

		propStructs := make([]reflect.Value, 0)
		for _, archProperty := range m.archProperties[i] {
			propStructShard := getArchProperties(ctx, archProperty, arch, os, m.Target().NativeBridge == NativeBridgeEnabled)
			propStructs = append(propStructs, propStructShard...)
		}

		for _, propStruct := range propStructs {
			mergePropertyStruct(ctx, genProps, propStruct)
		}
	}
}

// determineBuildOS stores the OS and architecture used for host targets used during the build into
// config based on the runtime OS and architecture determined by Go and the product configuration.
func determineBuildOS(config *config) {
	config.BuildOS = func() OsType {
		switch runtime.GOOS {
		case "linux":
			if Bool(config.productVariables.HostMusl) || runtime.GOARCH == "arm64" {
				return LinuxMusl
			}
			return Linux
		case "darwin":
			return Darwin
		default:
			panic(fmt.Sprintf("unsupported OS: %s", runtime.GOOS))
		}
	}()

	config.BuildArch = func() ArchType {
		switch runtime.GOOS {
		case "linux":
			switch runtime.GOARCH {
			case "amd64":
				return X86_64
			case "arm64":
				return Arm64
			default:
				panic(fmt.Sprintf("unsupported arch: %s", runtime.GOARCH))
			}
		case "darwin":
			switch runtime.GOARCH {
			case "amd64":
				return X86_64
			default:
				panic(fmt.Sprintf("unsupported arch: %s", runtime.GOARCH))
			}
		default:
			panic(fmt.Sprintf("unsupported OS: %s", runtime.GOOS))
		}
	}()

}

// Convert the arch product variables into a list of targets for each OsType.
func decodeTargetProductVariables(config *config) (map[OsType][]Target, error) {
	variables := config.productVariables

	targets := make(map[OsType][]Target)
	var targetErr error

	type targetConfig struct {
		os                       OsType
		archName                 string
		archVariant              *string
		cpuVariant               *string
		abi                      []string
		nativeBridgeEnabled      NativeBridgeSupport
		nativeBridgeHostArchName *string
		nativeBridgeRelativePath *string
	}

	addTarget := func(target targetConfig) {
		if targetErr != nil {
			return
		}

		arch, err := decodeArch(target.os, target.archName, target.archVariant, target.cpuVariant, target.abi)
		if err != nil {
			targetErr = err
			return
		}
		nativeBridgeRelativePathStr := String(target.nativeBridgeRelativePath)
		nativeBridgeHostArchNameStr := String(target.nativeBridgeHostArchName)

		// Use guest arch as relative install path by default
		if target.nativeBridgeEnabled && nativeBridgeRelativePathStr == "" {
			nativeBridgeRelativePathStr = arch.ArchType.String()
		}

		// A target is considered as HostCross if it's a host target which can't run natively on
		// the currently configured build machine (either because the OS is different or because of
		// the unsupported arch)
		hostCross := false
		if target.os.Class == Host {
			var osSupported bool
			if target.os == config.BuildOS {
				osSupported = true
			} else if config.BuildOS.Linux() && target.os.Linux() {
				// LinuxBionic and Linux are compatible
				osSupported = true
			} else {
				osSupported = false
			}

			var archSupported bool
			if arch.ArchType == Common {
				archSupported = true
			} else if arch.ArchType.Name == *variables.HostArch {
				archSupported = true
			} else if variables.HostSecondaryArch != nil && arch.ArchType.Name == *variables.HostSecondaryArch {
				archSupported = true
			} else {
				archSupported = false
			}
			if !osSupported || !archSupported {
				hostCross = true
			}
		}

		targets[target.os] = append(targets[target.os],
			Target{
				Os:                       target.os,
				Arch:                     arch,
				NativeBridge:             target.nativeBridgeEnabled,
				NativeBridgeHostArchName: nativeBridgeHostArchNameStr,
				NativeBridgeRelativePath: nativeBridgeRelativePathStr,
				HostCross:                hostCross,
			})
	}

	if variables.HostArch == nil {
		return nil, fmt.Errorf("No host primary architecture set")
	}

	// The primary host target, which must always exist.
	addTarget(targetConfig{os: config.BuildOS, archName: *variables.HostArch, nativeBridgeEnabled: NativeBridgeDisabled})

	// An optional secondary host target.
	if variables.HostSecondaryArch != nil && *variables.HostSecondaryArch != "" {
		addTarget(targetConfig{os: config.BuildOS, archName: *variables.HostSecondaryArch, nativeBridgeEnabled: NativeBridgeDisabled})
	}

	// Optional cross-compiled host targets, generally Windows.
	if String(variables.CrossHost) != "" {
		crossHostOs := osByName(*variables.CrossHost)
		if crossHostOs == NoOsType {
			return nil, fmt.Errorf("Unknown cross host OS %q", *variables.CrossHost)
		}

		if String(variables.CrossHostArch) == "" {
			return nil, fmt.Errorf("No cross-host primary architecture set")
		}

		// The primary cross-compiled host target.
		addTarget(targetConfig{os: crossHostOs, archName: *variables.CrossHostArch, nativeBridgeEnabled: NativeBridgeDisabled})

		// An optional secondary cross-compiled host target.
		if variables.CrossHostSecondaryArch != nil && *variables.CrossHostSecondaryArch != "" {
			addTarget(targetConfig{os: crossHostOs, archName: *variables.CrossHostSecondaryArch, nativeBridgeEnabled: NativeBridgeDisabled})
		}
	}

	// Optional device targets
	if variables.DeviceArch != nil && *variables.DeviceArch != "" {
		// The primary device target.
		addTarget(targetConfig{
			os:                  Android,
			archName:            *variables.DeviceArch,
			archVariant:         variables.DeviceArchVariant,
			cpuVariant:          variables.DeviceCpuVariant,
			abi:                 variables.DeviceAbi,
			nativeBridgeEnabled: NativeBridgeDisabled,
		})

		// An optional secondary device target.
		if variables.DeviceSecondaryArch != nil && *variables.DeviceSecondaryArch != "" {
			addTarget(targetConfig{
				os:                  Android,
				archName:            *variables.DeviceSecondaryArch,
				archVariant:         variables.DeviceSecondaryArchVariant,
				cpuVariant:          variables.DeviceSecondaryCpuVariant,
				abi:                 variables.DeviceSecondaryAbi,
				nativeBridgeEnabled: NativeBridgeDisabled,
			})
		}

		// An optional NativeBridge device target.
		if variables.NativeBridgeArch != nil && *variables.NativeBridgeArch != "" {
			addTarget(targetConfig{
				os:                       Android,
				archName:                 *variables.NativeBridgeArch,
				archVariant:              variables.NativeBridgeArchVariant,
				cpuVariant:               variables.NativeBridgeCpuVariant,
				abi:                      variables.NativeBridgeAbi,
				nativeBridgeEnabled:      NativeBridgeEnabled,
				nativeBridgeHostArchName: variables.DeviceArch,
				nativeBridgeRelativePath: variables.NativeBridgeRelativePath,
			})
		}

		// An optional secondary NativeBridge device target.
		if variables.DeviceSecondaryArch != nil && *variables.DeviceSecondaryArch != "" &&
			variables.NativeBridgeSecondaryArch != nil && *variables.NativeBridgeSecondaryArch != "" {
			addTarget(targetConfig{
				os:                       Android,
				archName:                 *variables.NativeBridgeSecondaryArch,
				archVariant:              variables.NativeBridgeSecondaryArchVariant,
				cpuVariant:               variables.NativeBridgeSecondaryCpuVariant,
				abi:                      variables.NativeBridgeSecondaryAbi,
				nativeBridgeEnabled:      NativeBridgeEnabled,
				nativeBridgeHostArchName: variables.DeviceSecondaryArch,
				nativeBridgeRelativePath: variables.NativeBridgeSecondaryRelativePath,
			})
		}
	}

	if targetErr != nil {
		return nil, targetErr
	}

	return targets, nil
}

// hasArmAbi returns true if arch has at least one arm ABI
func hasArmAbi(arch Arch) bool {
	return PrefixInList(arch.Abi, "arm")
}

// hasArmAndroidArch returns true if targets has at least
// one arm Android arch (possibly native bridged)
func hasArmAndroidArch(targets []Target) bool {
	for _, target := range targets {
		if target.Os == Android &&
			(target.Arch.ArchType == Arm || target.Arch.ArchType == Arm64) {
			return true
		}
	}
	return false
}

// archConfig describes a built-in configuration.
type archConfig struct {
	Arch        string   `json:"arch"`
	ArchVariant string   `json:"arch_variant"`
	CpuVariant  string   `json:"cpu_variant"`
	Abi         []string `json:"abis"`
}

// getNdkAbisConfig returns the list of archConfigs that are used for building
// the API stubs and static libraries that are included in the NDK.
func getNdkAbisConfig() []archConfig {
	return []archConfig{
		{"arm64", "armv8-a-branchprot", "", []string{"arm64-v8a"}},
		{"arm", "armv7-a-neon", "", []string{"armeabi-v7a"}},
		{"riscv64", "", "", []string{"riscv64"}},
		{"x86_64", "", "", []string{"x86_64"}},
		{"x86", "", "", []string{"x86"}},
	}
}

// getAmlAbisConfig returns a list of archConfigs for the ABIs supported by mainline modules.
func getAmlAbisConfig() []archConfig {
	return []archConfig{
		{"arm64", "armv8-a", "", []string{"arm64-v8a"}},
		{"arm", "armv7-a-neon", "", []string{"armeabi-v7a"}},
		{"x86_64", "", "", []string{"x86_64"}},
		{"x86", "", "", []string{"x86"}},
	}
}

// decodeArchSettings converts a list of archConfigs into a list of Targets for the given OsType.
func decodeAndroidArchSettings(archConfigs []archConfig) ([]Target, error) {
	var ret []Target

	for _, config := range archConfigs {
		arch, err := decodeArch(Android, config.Arch, &config.ArchVariant,
			&config.CpuVariant, config.Abi)
		if err != nil {
			return nil, err
		}

		ret = append(ret, Target{
			Os:   Android,
			Arch: arch,
		})
	}

	return ret, nil
}

// decodeArch converts a set of strings from product variables into an Arch struct.
func decodeArch(os OsType, arch string, archVariant, cpuVariant *string, abi []string) (Arch, error) {
	// Verify the arch is valid
	archType, ok := archTypeMap[arch]
	if !ok {
		return Arch{}, fmt.Errorf("unknown arch %q", arch)
	}

	a := Arch{
		ArchType:    archType,
		ArchVariant: String(archVariant),
		CpuVariant:  String(cpuVariant),
		Abi:         abi,
	}

	// Convert generic arch variants into the empty string.
	if a.ArchVariant == a.ArchType.Name || a.ArchVariant == "generic" {
		a.ArchVariant = ""
	}

	// Convert generic CPU variants into the empty string.
	if a.CpuVariant == a.ArchType.Name || a.CpuVariant == "generic" {
		a.CpuVariant = ""
	}

	if a.ArchVariant != "" {
		if validArchVariants := archVariants[archType]; !InList(a.ArchVariant, validArchVariants) {
			return Arch{}, fmt.Errorf("[%q] unknown arch variant %q, support variants: %q", archType, a.ArchVariant, validArchVariants)
		}
	}

	if a.CpuVariant != "" {
		if validCpuVariants := cpuVariants[archType]; !InList(a.CpuVariant, validCpuVariants) {
			return Arch{}, fmt.Errorf("[%q] unknown cpu variant %q, support variants: %q", archType, a.CpuVariant, validCpuVariants)
		}
	}

	// Filter empty ABIs out of the list.
	for i := 0; i < len(a.Abi); i++ {
		if a.Abi[i] == "" {
			a.Abi = append(a.Abi[:i], a.Abi[i+1:]...)
			i--
		}
	}

	// Set ArchFeatures from the arch type. for Android OS, other os-es do not specify features
	if os == Android {
		if featureMap, ok := androidArchFeatureMap[archType]; ok {
			a.ArchFeatures = featureMap[a.ArchVariant]
		}
	}

	return a, nil
}

// filterMultilibTargets takes a list of Targets and a multilib value and returns a new list of
// Targets containing only those that have the given multilib value.
func filterMultilibTargets(targets []Target, multilib string) []Target {
	var ret []Target
	for _, t := range targets {
		if t.Arch.ArchType.Multilib == multilib {
			ret = append(ret, t)
		}
	}
	return ret
}

// getCommonTargets returns the set of Os specific common architecture targets for each Os in a list
// of targets.
func getCommonTargets(targets []Target) []Target {
	var ret []Target
	set := make(map[string]bool)

	for _, t := range targets {
		if _, found := set[t.Os.String()]; !found {
			set[t.Os.String()] = true
			common := commonTargetMap[t.Os.String()]
			common.HostCross = t.HostCross
			ret = append(ret, common)
		}
	}

	return ret
}

// FirstTarget takes a list of Targets and a list of multilib values and returns a list of Targets
// that contains zero or one Target for each OsType and HostCross, selecting the one that matches
// the earliest filter.
func FirstTarget(targets []Target, filters ...string) []Target {
	// find the first target from each OS
	var ret []Target
	type osHostCross struct {
		os        OsType
		hostCross bool
	}
	set := make(map[osHostCross]bool)

	for _, filter := range filters {
		buildTargets := filterMultilibTargets(targets, filter)
		for _, t := range buildTargets {
			key := osHostCross{t.Os, t.HostCross}
			if _, found := set[key]; !found {
				set[key] = true
				ret = append(ret, t)
			}
		}
	}
	return ret
}

// decodeMultilibTargets uses the module's multilib setting to select one or more targets from a
// list of Targets.
func decodeMultilibTargets(multilib string, targets []Target, prefer32 bool) ([]Target, error) {
	var buildTargets []Target

	switch multilib {
	case "common":
		buildTargets = getCommonTargets(targets)
	case "both":
		if prefer32 {
			buildTargets = append(buildTargets, filterMultilibTargets(targets, "lib32")...)
			buildTargets = append(buildTargets, filterMultilibTargets(targets, "lib64")...)
		} else {
			buildTargets = append(buildTargets, filterMultilibTargets(targets, "lib64")...)
			buildTargets = append(buildTargets, filterMultilibTargets(targets, "lib32")...)
		}
	case "32":
		buildTargets = filterMultilibTargets(targets, "lib32")
	case "64":
		buildTargets = filterMultilibTargets(targets, "lib64")
	case "first":
		if prefer32 {
			buildTargets = FirstTarget(targets, "lib32", "lib64")
		} else {
			buildTargets = FirstTarget(targets, "lib64", "lib32")
		}
	case "first_prefer32":
		buildTargets = FirstTarget(targets, "lib32", "lib64")
	case "prefer32":
		buildTargets = filterMultilibTargets(targets, "lib32")
		if len(buildTargets) == 0 {
			buildTargets = filterMultilibTargets(targets, "lib64")
		}
	case "darwin_universal":
		buildTargets = filterMultilibTargets(targets, "lib64")
		// Reverse the targets so that the first architecture can depend on the second
		// architecture module in order to merge the outputs.
		ReverseSliceInPlace(buildTargets)
	default:
		return nil, fmt.Errorf(`compile_multilib must be "both", "first", "32", "64", "prefer32" or "first_prefer32" found %q`,
			multilib)
	}

	return buildTargets, nil
}

// ArchVariantContext defines the limited context necessary to retrieve arch_variant properties.
type ArchVariantContext interface {
	ModuleErrorf(fmt string, args ...interface{})
	PropertyErrorf(property, fmt string, args ...interface{})
}
