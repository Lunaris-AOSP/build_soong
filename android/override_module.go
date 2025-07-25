// Copyright (C) 2019 The Android Open Source Project
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

// This file contains all the foundation components for override modules and their base module
// types. Override modules are a kind of opposite of default modules in that they override certain
// properties of an existing base module whereas default modules provide base module data to be
// overridden. However, unlike default and defaultable module pairs, both override and overridable
// modules generate and output build actions, and it is up to product make vars to decide which one
// to actually build and install in the end. In other words, default modules and defaultable modules
// can be compared to abstract classes and concrete classes in C++ and Java. By the same analogy,
// both override and overridable modules act like concrete classes.
//
// There is one more crucial difference from the logic perspective. Unlike default pairs, most Soong
// actions happen in the base (overridable) module by creating a local variant for each override
// module based on it.

import (
	"fmt"
	"sort"
	"sync"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

// Interface for override module types, e.g. override_android_app, override_apex
type OverrideModule interface {
	Module

	getOverridingProperties() []interface{}
	setOverridingProperties(properties []interface{})

	getOverrideModuleProperties() *OverrideModuleProperties

	// Internal funcs to handle interoperability between override modules and prebuilts.
	// i.e. cases where an overriding module, too, is overridden by a prebuilt module.
	setOverriddenByPrebuilt(prebuilt Module)
	getOverriddenByPrebuilt() Module

	// Directory containing the Blueprint definition of the overriding module
	setModuleDir(string)
	ModuleDir() string
}

// Base module struct for override module types
type OverrideModuleBase struct {
	moduleProperties OverrideModuleProperties

	overridingProperties []interface{}

	overriddenByPrebuilt Module

	moduleDir string
}

type OverrideModuleProperties struct {
	// Name of the base module to be overridden
	Base *string

	// TODO(jungjw): Add an optional override_name bool flag.
}

func (o *OverrideModuleBase) setModuleDir(d string) {
	o.moduleDir = d
}

func (o *OverrideModuleBase) ModuleDir() string {
	return o.moduleDir
}

func (o *OverrideModuleBase) getOverridingProperties() []interface{} {
	return o.overridingProperties
}

func (o *OverrideModuleBase) setOverridingProperties(properties []interface{}) {
	o.overridingProperties = properties
}

func (o *OverrideModuleBase) getOverrideModuleProperties() *OverrideModuleProperties {
	return &o.moduleProperties
}

func (o *OverrideModuleBase) GetOverriddenModuleName() string {
	return proptools.String(o.moduleProperties.Base)
}

func (o *OverrideModuleBase) setOverriddenByPrebuilt(prebuilt Module) {
	o.overriddenByPrebuilt = prebuilt
}

func (o *OverrideModuleBase) getOverriddenByPrebuilt() Module {
	return o.overriddenByPrebuilt
}

func InitOverrideModule(m OverrideModule) {
	m.setOverridingProperties(m.GetProperties())

	m.AddProperties(m.getOverrideModuleProperties())
}

// Interface for overridable module types, e.g. android_app, apex
type OverridableModule interface {
	Module
	moduleBase() *OverridableModuleBase

	setOverridableProperties(prop []interface{})

	addOverride(o OverrideModule)
	getOverrides() []OverrideModule

	override(ctx BaseModuleContext, bm OverridableModule, o OverrideModule)
	GetOverriddenBy() string
	GetOverriddenByModuleDir() string

	setOverridesProperty(overridesProperties *[]string)

	// Due to complications with incoming dependencies, overrides are processed after DepsMutator.
	// So, overridable properties need to be handled in a separate, dedicated deps mutator.
	OverridablePropertiesDepsMutator(ctx BottomUpMutatorContext)
}

type overridableModuleProperties struct {
	OverriddenBy          string `blueprint:"mutated"`
	OverriddenByModuleDir string `blueprint:"mutated"`
}

// Base module struct for overridable module types
type OverridableModuleBase struct {
	// List of OverrideModules that override this base module
	overrides []OverrideModule
	// Used to parallelize registerOverrideMutator executions. Note that only addOverride locks this
	// mutex. It is because addOverride and getOverride are used in different mutators, and so are
	// guaranteed to be not mixed. (And, getOverride only reads from overrides, and so don't require
	// mutex locking.)
	overridesLock sync.Mutex

	overridableProperties []interface{}

	// If an overridable module has a property to list other modules that itself overrides, it should
	// set this to a pointer to the property through the InitOverridableModule function, so that
	// override information is propagated and aggregated correctly.
	overridesProperty *[]string

	overridableModuleProperties overridableModuleProperties
}

func InitOverridableModule(m OverridableModule, overridesProperty *[]string) {
	m.setOverridableProperties(m.(Module).GetProperties())
	m.setOverridesProperty(overridesProperty)
	m.AddProperties(&m.moduleBase().overridableModuleProperties)
}

func (o *OverridableModuleBase) moduleBase() *OverridableModuleBase {
	return o
}

func (b *OverridableModuleBase) setOverridableProperties(prop []interface{}) {
	b.overridableProperties = prop
}

func (b *OverridableModuleBase) addOverride(o OverrideModule) {
	b.overridesLock.Lock()
	b.overrides = append(b.overrides, o)
	b.overridesLock.Unlock()
}

// Should NOT be used in the same mutator as addOverride.
func (b *OverridableModuleBase) getOverrides() []OverrideModule {
	b.overridesLock.Lock()
	sort.Slice(b.overrides, func(i, j int) bool {
		return b.overrides[i].Name() < b.overrides[j].Name()
	})
	b.overridesLock.Unlock()
	return b.overrides
}

func (b *OverridableModuleBase) setOverridesProperty(overridesProperty *[]string) {
	b.overridesProperty = overridesProperty
}

// Overrides a base module with the given OverrideModule.
func (b *OverridableModuleBase) override(ctx BaseModuleContext, bm OverridableModule, o OverrideModule) {
	for _, p := range b.overridableProperties {
		for _, op := range o.getOverridingProperties() {
			if proptools.TypeEqual(p, op) {
				err := proptools.ExtendProperties(p, op, nil, proptools.OrderReplace)
				if err != nil {
					if propertyErr, ok := err.(*proptools.ExtendPropertyError); ok {
						ctx.OtherModulePropertyErrorf(bm, propertyErr.Property, "%s", propertyErr.Err.Error())
					} else {
						panic(err)
					}
				}
			}
		}
	}
	// Adds the base module to the overrides property, if exists, of the overriding module. See the
	// comment on OverridableModuleBase.overridesProperty for details.
	if b.overridesProperty != nil {
		*b.overridesProperty = append(*b.overridesProperty, ctx.OtherModuleName(bm))
	}
	b.overridableModuleProperties.OverriddenBy = o.Name()
	b.overridableModuleProperties.OverriddenByModuleDir = o.ModuleDir()
}

// GetOverriddenBy returns the name of the override module that has overridden this module.
// For example, if an override module foo has its 'base' property set to bar, then another local variant
// of bar is created and its properties are overriden by foo. This method returns bar when called from
// the new local variant. It returns "" when called from the original variant of bar.
func (b *OverridableModuleBase) GetOverriddenBy() string {
	return b.overridableModuleProperties.OverriddenBy
}

func (b *OverridableModuleBase) GetOverriddenByModuleDir() string {
	return b.overridableModuleProperties.OverriddenByModuleDir
}

func (b *OverridableModuleBase) OverridablePropertiesDepsMutator(ctx BottomUpMutatorContext) {
}

// Mutators for override/overridable modules. All the fun happens in these functions. It is critical
// to keep them in this order and not put any order mutators between them.
func RegisterOverridePostDepsMutators(ctx RegisterMutatorsContext) {
	ctx.BottomUp("override_deps", overrideModuleDepsMutator).MutatesDependencies() // modifies deps via addOverride
	ctx.Transition("override", &overrideTransitionMutator{})
	ctx.BottomUp("override_apply", overrideApplyMutator).MutatesDependencies()
	// overridableModuleDepsMutator calls OverridablePropertiesDepsMutator so that overridable modules can
	// add deps from overridable properties.
	ctx.BottomUp("overridable_deps", overridableModuleDepsMutator)
	// Because overridableModuleDepsMutator is run after PrebuiltPostDepsMutator,
	// prebuilt's ReplaceDependencies doesn't affect to those deps added by overridable properties.
	// By running PrebuiltPostDepsMutator again after overridableModuleDepsMutator, deps via overridable properties
	// can be replaced with prebuilts.
	ctx.BottomUp("replace_deps_on_prebuilts_for_overridable_deps_again", PrebuiltPostDepsMutator).UsesReplaceDependencies()
	ctx.BottomUp("replace_deps_on_override", replaceDepsOnOverridingModuleMutator).UsesReplaceDependencies()
}

type overrideBaseDependencyTag struct {
	blueprint.BaseDependencyTag
}

var overrideBaseDepTag overrideBaseDependencyTag

// Override module should always override the source module.
// Overrides are implemented as a variant of the overridden module, and the build actions are created in the
// module context of the overridden module.
// If we replace override module with the prebuilt of the overridden module, `GenerateAndroidBuildActions` for
// the override module will have a very different meaning.
func (tag overrideBaseDependencyTag) ReplaceSourceWithPrebuilt() bool {
	return false
}

// Adds dependency on the base module to the overriding module so that they can be visited in the
// next phase.
func overrideModuleDepsMutator(ctx BottomUpMutatorContext) {
	if module, ok := ctx.Module().(OverrideModule); ok {
		base := String(module.getOverrideModuleProperties().Base)
		if !ctx.OtherModuleExists(base) {
			ctx.PropertyErrorf("base", "%q is not a valid module name", base)
			return
		}
		baseModule := ctx.AddDependency(ctx.Module(), overrideBaseDepTag, *module.getOverrideModuleProperties().Base)[0]
		if o, ok := baseModule.(OverridableModule); ok {
			overrideModule := ctx.Module().(OverrideModule)
			overrideModule.setModuleDir(ctx.ModuleDir())
			o.addOverride(overrideModule)
		}
	}
}

// Now, goes through all overridable modules, finds all modules overriding them, creates a local
// variant for each of them, and performs the actual overriding operation by calling override().
type overrideTransitionMutator struct{}

func (overrideTransitionMutator) Split(ctx BaseModuleContext) []string {
	if b, ok := ctx.Module().(OverridableModule); ok {
		overrides := b.getOverrides()
		if len(overrides) == 0 {
			return []string{""}
		}
		variants := make([]string, len(overrides)+1)
		// The first variant is for the original, non-overridden, base module.
		variants[0] = ""
		for i, o := range overrides {
			variants[i+1] = o.(Module).Name()
		}
		return variants
	} else if o, ok := ctx.Module().(OverrideModule); ok {
		// Create a variant of the overriding module with its own name. This matches the above local
		// variant name rule for overridden modules, and thus allows ReplaceDependencies to match the
		// two.
		return []string{o.Name()}
	}

	return []string{""}
}

func (overrideTransitionMutator) OutgoingTransition(ctx OutgoingTransitionContext, sourceVariation string) string {
	if o, ok := ctx.Module().(OverrideModule); ok {
		if ctx.DepTag() == overrideBaseDepTag {
			return o.Name()
		}
	}

	// Variations are always local and shouldn't affect the variant used for dependencies
	return ""
}

func (overrideTransitionMutator) IncomingTransition(ctx IncomingTransitionContext, incomingVariation string) string {
	if _, ok := ctx.Module().(OverridableModule); ok {
		return incomingVariation
	} else if o, ok := ctx.Module().(OverrideModule); ok {
		// To allow dependencies to be added without having to know the variation.
		return o.Name()
	}

	return ""
}

func (overrideTransitionMutator) Mutate(ctx BottomUpMutatorContext, variation string) {
}

func overrideApplyMutator(ctx BottomUpMutatorContext) {
	if o, ok := ctx.Module().(OverrideModule); ok {
		overridableDeps := ctx.GetDirectDepsWithTag(overrideBaseDepTag)
		if len(overridableDeps) > 1 {
			panic(fmt.Errorf("expected a single dependency with overrideBaseDepTag, found %q", overridableDeps))
		} else if len(overridableDeps) == 1 {
			b := overridableDeps[0].(OverridableModule)
			b.override(ctx, b, o)

			checkPrebuiltReplacesOverride(ctx, b)
		}
	}
}

func checkPrebuiltReplacesOverride(ctx BottomUpMutatorContext, b OverridableModule) {
	// See if there's a prebuilt module that overrides this override module with prefer flag,
	// in which case we call HideFromMake on the corresponding variant later.
	prebuiltDeps := ctx.GetDirectDepsWithTag(PrebuiltDepTag)
	for _, prebuiltDep := range prebuiltDeps {
		prebuilt := GetEmbeddedPrebuilt(prebuiltDep)
		if prebuilt == nil {
			panic("PrebuiltDepTag leads to a non-prebuilt module " + prebuiltDep.Name())
		}
		if prebuilt.UsePrebuilt() {
			// The overriding module itself, too, is overridden by a prebuilt.
			// Perform the same check for replacement
			checkInvariantsForSourceAndPrebuilt(ctx, b, prebuiltDep)
			// Copy the flag and hide it in make
			b.ReplacedByPrebuilt()
		}
	}
}

func overridableModuleDepsMutator(ctx BottomUpMutatorContext) {
	ctx.Module().base().baseOverridablePropertiesDepsMutator(ctx)
	if b, ok := ctx.Module().(OverridableModule); ok && b.Enabled(ctx) {
		b.OverridablePropertiesDepsMutator(ctx)
	}
}

func replaceDepsOnOverridingModuleMutator(ctx BottomUpMutatorContext) {
	if b, ok := ctx.Module().(OverridableModule); ok {
		if o := b.GetOverriddenBy(); o != "" {
			// Redirect dependencies on the overriding module to this overridden module. Overriding
			// modules are basically pseudo modules, and all build actions are associated to overridden
			// modules. Therefore, dependencies on overriding modules need to be forwarded there as well.
			ctx.ReplaceDependencies(o)
		}
	}
}
