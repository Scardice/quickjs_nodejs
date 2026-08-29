// Package module contains memory-backed ESM module registration for QuickJS.
package module

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	quickjs "github.com/buke/quickjs-go"
)

// Export describes one named ESM export.
type Export struct {
	Name string
	Spec quickjs.ValueSpec
}

// Definition describes one canonical module and its optional node: aliases.
type Definition struct {
	Name    string
	Aliases []string
	Exports []Export
}

// Registry stores immutable module definitions and materializes them per Context.
type Registry struct {
	mu          sync.RWMutex
	definitions map[string]Definition
	order       []string

	sourceLoader  SourceLoader
	pathResolver  PathResolver
	baseDir       string
	globalFolders []string
	sources       map[string][]byte
	native        map[string]nativeDefinition
	registryID    uint64
}

// NewRegistry creates an empty module registry.
func NewRegistry(options ...RegistryOption) *Registry {
	registry := &Registry{
		definitions: make(map[string]Definition),
		sources:     make(map[string][]byte),
		native:      make(map[string]nativeDefinition),
		registryID:  newRegistryID(),
	}
	for _, option := range options {
		if option != nil {
			option(registry)
		}
	}
	return registry
}

// Add adds a canonical module and all of its aliases.
func (r *Registry) Add(def Definition) error {
	if r == nil {
		return fmt.Errorf("module registry is nil")
	}
	canonical, err := validateModuleName(def.Name)
	if err != nil {
		return err
	}
	if len(def.Exports) == 0 {
		return fmt.Errorf("module %q has no exports", canonical)
	}

	copyDef := Definition{
		Name:    canonical,
		Aliases: make([]string, len(def.Aliases)),
		Exports: make([]Export, len(def.Exports)),
	}
	copy(copyDef.Aliases, def.Aliases)
	copy(copyDef.Exports, def.Exports)
	for i := range copyDef.Exports {
		if copyDef.Exports[i].Name == "" {
			return fmt.Errorf("module %q has an empty export name", canonical)
		}
		if copyDef.Exports[i].Spec == nil {
			return fmt.Errorf("module %q export %q has a nil spec", canonical, copyDef.Exports[i].Name)
		}
	}

	names := make([]string, 0, len(copyDef.Aliases)+1)
	names = append(names, canonical)
	seen := map[string]struct{}{canonical: {}}
	for i, alias := range copyDef.Aliases {
		name, err := validateModuleName(alias)
		if err != nil {
			return fmt.Errorf("module %q alias %d: %w", canonical, i, err)
		}
		if _, ok := seen[name]; ok {
			return fmt.Errorf("module %q has duplicate alias %q", canonical, name)
		}
		seen[name] = struct{}{}
		copyDef.Aliases[i] = name
		names = append(names, name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, name := range names {
		if existing, ok := r.definitions[name]; ok {
			return fmt.Errorf("module name %q already registered by %q", name, existing.Name)
		}
		if existing, ok := r.native[name]; ok {
			return fmt.Errorf("module name %q already registered as native module %q", name, existing.name)
		}
	}
	for _, name := range names {
		r.definitions[name] = copyDef
	}
	r.order = append(r.order, canonical)
	return nil
}

// Names returns all registered canonical names and aliases in stable order.
func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.definitions))
	for _, canonical := range r.order {
		def := r.definitions[canonical]
		names = append(names, def.Name)
		names = append(names, def.Aliases...)
	}
	sort.Strings(names)
	return names
}

// Register builds every registered ESM module in ctx. CommonJS is enabled
// separately with EnableRequire and never changes ESM import behavior.
func (r *Registry) Register(ctx *quickjs.Context) error {
	if r == nil {
		return fmt.Errorf("module registry is nil")
	}
	if ctx == nil {
		return fmt.Errorf("module context is nil")
	}

	r.mu.RLock()
	defs := make([]Definition, 0, len(r.order))
	for _, canonical := range r.order {
		def := r.definitions[canonical]
		def.Aliases = append([]string(nil), def.Aliases...)
		def.Exports = append([]Export(nil), def.Exports...)
		defs = append(defs, def)
	}
	r.mu.RUnlock()

	for _, def := range defs {
		if err := r.prepareDefinition(ctx, def); err != nil {
			return err
		}
	}
	return nil
}

// RegisterModule builds one canonical module or alias in ctx.
func (r *Registry) RegisterModule(ctx *quickjs.Context, name string) error {
	if r == nil {
		return fmt.Errorf("module registry is nil")
	}
	if ctx == nil {
		return fmt.Errorf("module context is nil")
	}
	name, err := validateModuleName(name)
	if err != nil {
		return err
	}

	r.mu.RLock()
	def, ok := r.definitions[name]
	r.mu.RUnlock()
	if !ok {
		return fmt.Errorf("module %q is not registered", name)
	}
	return r.prepareDefinition(ctx, def)
}

func buildDefinition(ctx *quickjs.Context, name string, exports []Export) error {
	builder := quickjs.NewModuleBuilder(name)
	for _, exported := range exports {
		builder.ExportValue(exported.Name, exported.Spec)
	}
	if err := builder.Build(ctx); err != nil {
		return fmt.Errorf("build module %q: %w", name, err)
	}
	return nil
}

func validateModuleName(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("module name is empty")
	}
	if strings.TrimSpace(name) != name {
		return "", fmt.Errorf("module name %q has surrounding whitespace", name)
	}
	if strings.ContainsAny(name, "\r\n\t") {
		return "", fmt.Errorf("module name %q contains control whitespace", name)
	}
	return name, nil
}
