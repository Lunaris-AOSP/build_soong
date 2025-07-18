// Copyright 2019 Google Inc. All rights reserved.
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
	"android/soong/android"
	"github.com/google/blueprint"
)

type JavaPluginInfo struct {
	ProcessorClass *string
	GeneratesApi   bool
}

var JavaPluginInfoProvider = blueprint.NewProvider[JavaPluginInfo]()

type KotlinPluginInfo struct {
}

var KotlinPluginInfoProvider = blueprint.NewProvider[KotlinPluginInfo]()

func init() {
	registerJavaPluginBuildComponents(android.InitRegistrationContext)
}

func registerJavaPluginBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("java_plugin", PluginFactory)
	ctx.RegisterModuleType("kotlin_plugin", KotlinPluginFactory)
}

func PluginFactory() android.Module {
	module := &Plugin{}

	module.addHostProperties()
	module.AddProperties(&module.pluginProperties)

	InitJavaModule(module, android.HostSupported)

	return module
}

func KotlinPluginFactory() android.Module {
	module := &KotlinPlugin{}

	module.addHostProperties()

	InitJavaModule(module, android.HostSupported)

	return module
}

// Plugin describes a java_plugin module, a host java library that will be used by javac as an annotation processor.
type Plugin struct {
	Library

	pluginProperties PluginProperties
}

type PluginProperties struct {
	// The optional name of the class that javac will use to run the annotation processor.
	Processor_class *string

	// If true, assume the annotation processor will generate classes that are referenced from outside the module.
	// This necessitates disabling the turbine optimization on modules that use this plugin, which will reduce
	// parallelism and cause more recompilation for modules that depend on modules that use this plugin.
	Generates_api *bool
}

func (p *Plugin) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	p.Library.GenerateAndroidBuildActions(ctx)

	android.SetProvider(ctx, JavaPluginInfoProvider, JavaPluginInfo{
		ProcessorClass: p.pluginProperties.Processor_class,
		GeneratesApi:   Bool(p.pluginProperties.Generates_api),
	})
}

// Plugin describes a kotlin_plugin module, a host java/kotlin library that will be used by kotlinc as a compiler plugin.
type KotlinPlugin struct {
	Library
}

func (p *KotlinPlugin) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	p.Library.GenerateAndroidBuildActions(ctx)

	android.SetProvider(ctx, KotlinPluginInfoProvider, KotlinPluginInfo{})
}
