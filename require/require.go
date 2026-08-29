// Package require exposes the QuickJS CommonJS compatibility layer.
package require

import "github.com/Scardice/quickjs_nodejs/module"

// Registry is the shared ESM/CommonJS registry implementation.
type Registry = module.Registry

// Definition and Export remain available for callers that register ESM
// modules through the same registry.
type Definition = module.Definition
type RequireModule = module.RequireModule
type Export = module.Export

// Provider contracts are aliases so there is no second loader implementation.
type SourceLoader = module.SourceLoader
type PathResolver = module.PathResolver
type NativeLoader = module.NativeLoader
type Option = module.RegistryOption

var (
	NewRegistry                 = module.NewRegistry
	NewRegistryWithLoader       = func(loader SourceLoader) *Registry { return NewRegistry(WithSourceLoader(loader)) }
	WithSourceLoader            = module.WithSourceLoader
	WithLoader                  = module.WithSourceLoader
	WithPathResolver            = module.WithPathResolver
	WithBaseDir                 = module.WithBaseDir
	WithGlobalFolders           = module.WithGlobalFolders
	DefaultSourceLoader         = module.DefaultSourceLoader
	DefaultPathResolver         = module.DefaultPathResolver
	ErrModuleNotFound           = module.ErrModuleNotFound
	ErrInvalidModule            = module.ErrInvalidModule
	ErrNoSuchBuiltin            = module.ErrNoSuchBuiltin
	ModuleFileDoesNotExistError = module.ErrModuleNotFound
	InvalidModuleError          = module.ErrInvalidModule
	NoSuchBuiltInModuleError    = module.ErrNoSuchBuiltin
)
