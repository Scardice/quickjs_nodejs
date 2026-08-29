package module

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	quickjs "github.com/buke/quickjs-go"
)

const (
	cacheKeyPrefix = "__quickjs_nodejs_registry_cache_"
	esmKeySuffix   = "_esm"
)

func (r *Registry) cacheKey() string {
	return fmt.Sprintf("%s%d", cacheKeyPrefix, r.registryID)
}

func (r *Registry) esmKey() string {
	return r.cacheKey() + esmKeySuffix
}

func (r *Registry) objectCache(ctx *quickjs.Context, key string) (*quickjs.Value, error) {
	if r == nil {
		return nil, errors.New("module registry is nil")
	}
	if ctx == nil {
		return nil, errors.New("module context is nil")
	}
	globals := ctx.Globals()
	if globals == nil {
		return nil, errors.New("module context is closed")
	}
	cache := globals.Get(key)
	if cache != nil && cache.IsObject() && !cache.IsNull() {
		return cache, nil
	}
	if cache != nil {
		cache.Free()
	}
	cache = ctx.Eval("Object.create(null)")
	if cache == nil || cache.IsException() {
		if cache != nil {
			cache.Free()
		}
		return nil, fmt.Errorf("create module cache")
	}
	globals.Set(key, cache)
	return globals.Get(key), nil
}

func (r *Registry) cachedValue(ctx *quickjs.Context, key, name string) (*quickjs.Value, bool, error) {
	cache, err := r.objectCache(ctx, key)
	if err != nil {
		return nil, false, err
	}
	defer cache.Free()
	value := cache.Get(name)
	if value == nil || value.IsUndefined() || value.IsNull() {
		if value != nil {
			value.Free()
		}
		return nil, false, nil
	}
	return value, true, nil
}

func (r *Registry) cacheValue(ctx *quickjs.Context, key, name string, value *quickjs.Value) error {
	if value == nil || value.Context() != ctx {
		return fmt.Errorf("module cache value %q belongs to a different context", name)
	}
	cache, err := r.objectCache(ctx, key)
	if err != nil {
		return err
	}
	defer cache.Free()
	if !cache.DefinePropertyValue(name, value, quickjs.PropConfigurable|quickjs.PropWritable|quickjs.PropEnumerable) {
		return fmt.Errorf("cache module %q", name)
	}
	return nil
}

func (r *Registry) deleteCachedValue(ctx *quickjs.Context, key, name string) {
	cache, err := r.objectCache(ctx, key)
	if err != nil {
		return
	}
	cache.Delete(name)
	cache.Free()
}

func (r *Registry) markESM(ctx *quickjs.Context, name string) bool {
	cache, err := r.objectCache(ctx, r.esmKey())
	if err != nil {
		return false
	}
	defer cache.Free()
	value := cache.Get(name)
	if value != nil && !value.IsUndefined() {
		value.Free()
		return true
	}
	if value != nil {
		value.Free()
	}
	flag := ctx.NewBool(true)
	cache.Set(name, flag)
	return false
}

func (r *Registry) buildCachedESM(ctx *quickjs.Context, def Definition) error {
	if r.markESM(ctx, def.Name) {
		return nil
	}
	build := func(name string) error {
		builder := quickjs.NewModuleBuilder(name)
		for _, exported := range def.Exports {
			exportName := exported.Name
			canonical := def.Name
			builder.ExportValue(exportName, quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				cached, ok, err := r.cachedValue(ctx, r.cacheKey(), canonical)
				if err != nil {
					return nil, err
				}
				if !ok {
					return nil, fmt.Errorf("module %q is not initialized", canonical)
				}
				value := cached.Get(exportName)
				cached.Free()
				if value == nil {
					return nil, fmt.Errorf("module %q export %q is unavailable", canonical, exportName)
				}
				return value, nil
			}})
		}
		if err := builder.Build(ctx); err != nil {
			return fmt.Errorf("build module %q: %w", name, err)
		}
		return nil
	}
	if err := build(def.Name); err != nil {
		r.deleteCachedValue(ctx, r.esmKey(), def.Name)
		return err
	}
	for _, alias := range def.Aliases {
		if err := build(alias); err != nil {
			return err
		}
	}
	return nil
}

func (r *Registry) prepareDefinition(ctx *quickjs.Context, def Definition) error {
	if cached, ok, err := r.cachedValue(ctx, r.cacheKey(), def.Name); err != nil {
		return err
	} else if ok {
		cached.Free()
		return r.buildCachedESM(ctx, def)
	}

	exports := ctx.NewObject()
	if exports == nil {
		return fmt.Errorf("create exports object for %q", def.Name)
	}
	for _, exported := range def.Exports {
		value, err := materializeExport(ctx, exported.Spec)
		if err != nil {
			exports.Free()
			return fmt.Errorf("materialize module %q export %q: %w", def.Name, exported.Name, err)
		}
		if value == nil || value.Context() != ctx {
			if value != nil {
				value.Free()
			}
			exports.Free()
			return fmt.Errorf("module %q export %q belongs to a different context", def.Name, exported.Name)
		}
		if !exports.DefinePropertyValue(exported.Name, value, quickjs.PropConfigurable|quickjs.PropWritable|quickjs.PropEnumerable) {
			value.Free()
			exports.Free()
			return fmt.Errorf("set module %q export %q", def.Name, exported.Name)
		}
		value.Free()
	}
	if err := r.cacheValue(ctx, r.cacheKey(), def.Name, exports); err != nil {
		exports.Free()
		return err
	}
	exports.Free()
	for _, alias := range def.Aliases {
		canonical, ok, err := r.cachedValue(ctx, r.cacheKey(), def.Name)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("module %q cache disappeared", def.Name)
		}
		if err := r.cacheValue(ctx, r.cacheKey(), alias, canonical); err != nil {
			canonical.Free()
			return err
		}
		canonical.Free()
	}
	return r.buildCachedESM(ctx, def)
}

func materializeExport(ctx *quickjs.Context, spec quickjs.ValueSpec) (value *quickjs.Value, err error) {
	if spec == nil {
		return nil, errors.New("export value is required")
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("export factory panicked: %v", recovered)
			value = nil
		}
	}()
	return spec.Materialize(ctx)
}

func (r *Registry) definition(name string) (Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.definitions[name]
	return def, ok
}

func (r *Registry) nativeDefinition(name string) (nativeDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.native[name]
	return def, ok
}

func (r *Registry) loadStatic(ctx *quickjs.Context, name string) (*quickjs.Value, error) {
	def, ok := r.definition(name)
	if !ok {
		return nil, ErrInvalidModule
	}
	if err := r.prepareDefinition(ctx, def); err != nil {
		return nil, err
	}
	value, ok, err := r.cachedValue(ctx, r.cacheKey(), def.Name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("module %q is not initialized", def.Name)
	}
	return value, nil
}

func (r *Registry) loadNative(ctx *quickjs.Context, name string) (*quickjs.Value, error) {
	def, ok := r.nativeDefinition(name)
	if !ok {
		return nil, ErrNoSuchBuiltin
	}
	if cached, ok, err := r.cachedValue(ctx, r.cacheKey(), def.name); err != nil {
		return nil, err
	} else if ok {
		moduleExports := cached.Get("exports")
		cached.Free()
		if moduleExports == nil {
			return nil, fmt.Errorf("native module %q has no exports", def.name)
		}
		return moduleExports, nil
	}

	moduleValue := ctx.NewObject()
	exports := ctx.NewObject()
	if moduleValue == nil || exports == nil {
		if moduleValue != nil {
			moduleValue.Free()
		}
		if exports != nil {
			exports.Free()
		}
		return nil, fmt.Errorf("create native module %q", def.name)
	}
	if !moduleValue.DefinePropertyValue("exports", exports, quickjs.PropConfigurable|quickjs.PropWritable) {
		moduleValue.Free()
		exports.Free()
		return nil, fmt.Errorf("initialize native module %q", def.name)
	}
	exports.Free()
	if err := r.cacheValue(ctx, r.cacheKey(), def.name, moduleValue); err != nil {
		moduleValue.Free()
		return nil, err
	}
	moduleValue.Free()

	cached, ok, err := r.cachedValue(ctx, r.cacheKey(), def.name)
	if err != nil || !ok {
		return nil, fmt.Errorf("cache native module %q", def.name)
	}
	if err := callNativeLoader(ctx, def.loader, cached); err != nil {
		cached.Free()
		r.deleteCachedValue(ctx, r.cacheKey(), def.name)
		return nil, fmt.Errorf("load native module %q: %w", def.name, err)
	}
	for _, alias := range def.aliases {
		canonical, ok, aliasErr := r.cachedValue(ctx, r.cacheKey(), def.name)
		if aliasErr != nil {
			cached.Free()
			return nil, aliasErr
		}
		if !ok {
			cached.Free()
			return nil, fmt.Errorf("native module %q cache disappeared", def.name)
		}
		if aliasErr = r.cacheValue(ctx, r.cacheKey(), alias, canonical); aliasErr != nil {
			canonical.Free()
			cached.Free()
			return nil, aliasErr
		}
		canonical.Free()
	}
	moduleExports := cached.Get("exports")
	cached.Free()
	if moduleExports == nil {
		return nil, fmt.Errorf("native module %q has no exports", def.name)
	}
	return moduleExports, nil
}

func callNativeLoader(ctx *quickjs.Context, loader NativeLoader, moduleValue *quickjs.Value) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("native loader panicked: %v", recovered)
		}
	}()
	return loader(ctx, moduleValue)
}

// EnableRequire installs globalThis.require for this registry. It does not
// enable any native module unless the host registered it or added its ESM
// Definition explicitly.
func (r *Registry) EnableRequire(ctx *quickjs.Context) error {
	if r == nil {
		return errors.New("module registry is nil")
	}
	if ctx == nil {
		return errors.New("module context is nil")
	}
	if ctx.Globals() == nil {
		return errors.New("module context is closed")
	}
	requireValue := r.newRequireFunction(ctx, "")
	if requireValue == nil {
		return errors.New("create require function")
	}
	ctx.Globals().Set("require", requireValue)
	return nil
}

// RequireModule is a context-bound CommonJS facade returned by Enable.
// Values returned by Require are owned by the caller and must be released on
// the owner goroutine.
type RequireModule struct {
	registry *Registry
	context  *quickjs.Context
}

// Enable installs globalThis.require and returns a context-bound facade.
// It is a convenience wrapper around EnableRequire for hosts that prefer the
// Goja-style explicit enable step.
func (r *Registry) Enable(ctx *quickjs.Context) *RequireModule {
	if r == nil || ctx == nil {
		return nil
	}
	if err := r.EnableRequire(ctx); err != nil {
		return nil
	}
	return &RequireModule{registry: r, context: ctx}
}

// Require resolves a module from this facade's top-level base directory.
func (m *RequireModule) Require(specifier string) (*quickjs.Value, error) {
	if m == nil || m.registry == nil || m.context == nil {
		return nil, errors.New("require module is not enabled")
	}
	return m.registry.Require(m.context, "", specifier)
}

func (r *Registry) newRequireFunction(ctx *quickjs.Context, parent string) *quickjs.Value {
	requireValue := ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if len(args) == 0 || args[0] == nil || args[0].IsUndefined() {
			return ctx.ThrowTypeError("require() expects a module specifier")
		}
		value, err := r.Require(ctx, parent, args[0].ToString())
		if err != nil {
			return ctx.ThrowError(err)
		}
		return value
	})
	if requireValue == nil {
		return nil
	}
	resolveValue := ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if len(args) == 0 || args[0] == nil || args[0].IsUndefined() {
			return ctx.ThrowTypeError("require.resolve() expects a module specifier")
		}
		resolved, err := r.resolveForRequire(ctx, parent, args[0].ToString())
		if err != nil {
			return ctx.ThrowError(err)
		}
		return ctx.NewString(resolved)
	})
	if resolveValue != nil {
		requireValue.DefinePropertyValue("resolve", resolveValue, quickjs.PropConfigurable|quickjs.PropWritable)
		resolveValue.Free()
	}
	return requireValue
}

// Require resolves and loads a module from a CommonJS parent filename. The
// returned Value is owned by the caller and must be freed on the owner thread.
func (r *Registry) Require(ctx *quickjs.Context, parent, specifier string) (*quickjs.Value, error) {
	if r == nil {
		return nil, errors.New("module registry is nil")
	}
	if ctx == nil {
		return nil, errors.New("module context is nil")
	}
	if strings.TrimSpace(specifier) == "" {
		return nil, errors.New("module specifier is empty")
	}
	if value, err := r.loadNamed(ctx, specifier); err == nil {
		return value, nil
	} else if err != ErrInvalidModule && err != ErrNoSuchBuiltin {
		return nil, err
	}
	if !isPathRequest(specifier) {
		base := r.parentDirectory(parent)
		for _, folder := range r.searchFolders(base) {
			candidate := r.resolvePath(folder, specifier)
			value, err := r.loadAsFileOrDirectory(ctx, candidate)
			if err == nil {
				return value, nil
			}
			if !errors.Is(err, ErrModuleNotFound) {
				return nil, err
			}
		}
		return nil, fmt.Errorf("cannot find module %q", specifier)
	}
	candidate := r.resolvePath(r.parentDirectory(parent), specifier)
	value, err := r.loadAsFileOrDirectory(ctx, candidate)
	if err != nil {
		return nil, fmt.Errorf("cannot find module %q: %w", specifier, err)
	}
	return value, nil
}

func (r *Registry) loadNamed(ctx *quickjs.Context, specifier string) (*quickjs.Value, error) {
	if def, ok := r.definition(specifier); ok {
		return r.loadStatic(ctx, def.Name)
	}
	if def, ok := r.nativeDefinition(specifier); ok {
		return r.loadNative(ctx, def.name)
	}
	if strings.HasPrefix(specifier, "node:") {
		bare := strings.TrimPrefix(specifier, "node:")
		if def, ok := r.definition(bare); ok {
			return r.loadStatic(ctx, def.Name)
		}
		if def, ok := r.nativeDefinition(bare); ok {
			return r.loadNative(ctx, def.name)
		}
	}
	return nil, ErrInvalidModule
}

func (r *Registry) parentDirectory(parent string) string {
	if parent != "" {
		return filepath.Dir(parent)
	}
	r.mu.RLock()
	base := r.baseDir
	r.mu.RUnlock()
	if base == "" {
		return "."
	}
	return base
}

func (r *Registry) searchFolders(base string) []string {
	folders := r.configuredGlobalFolders()
	for current := filepath.Clean(base); ; current = filepath.Dir(current) {
		folders = append(folders, filepath.Join(current, "node_modules"))
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return folders
}

func isPathRequest(specifier string) bool {
	return specifier == "." || specifier == ".." || specifier == "./" || specifier == "../" ||
		strings.HasPrefix(specifier, "./") || strings.HasPrefix(specifier, "../") ||
		strings.HasPrefix(specifier, "/") || filepath.IsAbs(filepath.FromSlash(specifier))
}

func (r *Registry) resolveForRequire(ctx *quickjs.Context, parent, specifier string) (string, error) {
	if _, err := r.loadNamed(ctx, specifier); err == nil {
		return specifier, nil
	}
	if !isPathRequest(specifier) {
		for _, folder := range r.searchFolders(r.parentDirectory(parent)) {
			candidate := r.resolvePath(folder, specifier)
			if _, err := r.loadAsFileOrDirectory(ctx, candidate); err == nil {
				return candidate, nil
			}
		}
		return "", fmt.Errorf("cannot find module %q", specifier)
	}
	candidate := r.resolvePath(r.parentDirectory(parent), specifier)
	if _, err := r.loadAsFileOrDirectory(ctx, candidate); err != nil {
		return "", err
	}
	return candidate, nil
}

func (r *Registry) loadAsFileOrDirectory(ctx *quickjs.Context, path string) (*quickjs.Value, error) {
	if value, err := r.loadAsFile(ctx, path); value != nil || err != nil && !errors.Is(err, ErrModuleNotFound) {
		return value, err
	}
	return r.loadAsDirectory(ctx, path)
}

func (r *Registry) loadAsFile(ctx *quickjs.Context, path string) (*quickjs.Value, error) {
	candidates := []string{path}
	if filepath.Ext(path) == "" {
		candidates = append(candidates, path+".js", path+".json")
	}
	for _, candidate := range candidates {
		value, err := r.loadSourceModule(ctx, candidate)
		if err == nil {
			return value, nil
		}
		if !errors.Is(err, ErrModuleNotFound) {
			return nil, err
		}
	}
	return nil, ErrModuleNotFound
}

func (r *Registry) loadAsDirectory(ctx *quickjs.Context, path string) (*quickjs.Value, error) {
	packagePath := filepath.Join(path, "package.json")
	if data, err := r.source(packagePath); err == nil {
		var metadata struct {
			Main string `json:"main"`
		}
		if json.Unmarshal(data, &metadata) == nil && metadata.Main != "" {
			mainPath := r.resolvePath(path, metadata.Main)
			if value, mainErr := r.loadAsFileOrDirectory(ctx, mainPath); mainErr == nil {
				return value, nil
			} else if !errors.Is(mainErr, ErrModuleNotFound) {
				return nil, mainErr
			}
		}
	} else if !errors.Is(err, ErrModuleNotFound) {
		return nil, err
	}
	for _, candidate := range []string{filepath.Join(path, "index.js"), filepath.Join(path, "index.json")} {
		value, err := r.loadSourceModule(ctx, candidate)
		if err == nil {
			return value, nil
		}
		if !errors.Is(err, ErrModuleNotFound) {
			return nil, err
		}
	}
	return nil, ErrModuleNotFound
}

func (r *Registry) loadSourceModule(ctx *quickjs.Context, filename string) (*quickjs.Value, error) {
	canonical := r.resolvePath("", filename)
	if cached, ok, err := r.cachedValue(ctx, r.cacheKey(), canonical); err != nil {
		return nil, err
	} else if ok {
		if cached.Has("exports") {
			value := cached.Get("exports")
			cached.Free()
			return value, nil
		}
		return cached, nil
	}
	data, err := r.source(canonical)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(filepath.Ext(canonical), ".json") {
		return r.evaluateCommonJS(ctx, canonical, []byte("module.exports = JSON.parse("+strconv.Quote(string(data))+");"))
	}
	return r.evaluateCommonJS(ctx, canonical, data)
}

// LoadMain evaluates an entry CommonJS source with the supplied canonical
// filename. It is the bridge used by hosts that already read a plugin file.
func (r *Registry) LoadMain(ctx *quickjs.Context, filename, source string) (*quickjs.Value, error) {
	if r == nil {
		return nil, errors.New("module registry is nil")
	}
	if ctx == nil {
		return nil, errors.New("module context is nil")
	}
	if filename == "" {
		return nil, errors.New("module filename is empty")
	}
	return r.evaluateCommonJS(ctx, r.resolvePath("", filename), []byte(source))
}

func (r *Registry) evaluateCommonJS(ctx *quickjs.Context, filename string, source []byte) (*quickjs.Value, error) {
	if cached, ok, err := r.cachedValue(ctx, r.cacheKey(), filename); err != nil {
		return nil, err
	} else if ok {
		if cached.Has("exports") {
			value := cached.Get("exports")
			cached.Free()
			return value, nil
		}
		return cached, nil
	}

	moduleValue := ctx.NewObject()
	exports := ctx.NewObject()
	if moduleValue == nil || exports == nil {
		if moduleValue != nil {
			moduleValue.Free()
		}
		if exports != nil {
			exports.Free()
		}
		return nil, fmt.Errorf("create CommonJS module %q", filename)
	}
	if !moduleValue.DefinePropertyValue("exports", exports, quickjs.PropConfigurable|quickjs.PropWritable) {
		moduleValue.Free()
		exports.Free()
		return nil, fmt.Errorf("initialize CommonJS module %q", filename)
	}
	exports.Free()
	id := ctx.NewString(filename)
	moduleValue.DefinePropertyValue("id", id, quickjs.PropConfigurable|quickjs.PropWritable)
	id.Free()
	filenameValue := ctx.NewString(filename)
	moduleValue.DefinePropertyValue("filename", filenameValue, quickjs.PropConfigurable|quickjs.PropWritable)
	filenameValue.Free()
	loaded := ctx.NewBool(false)
	moduleValue.DefinePropertyValue("loaded", loaded, quickjs.PropConfigurable|quickjs.PropWritable)
	loaded.Free()
	if err := r.cacheValue(ctx, r.cacheKey(), filename, moduleValue); err != nil {
		moduleValue.Free()
		return nil, err
	}
	moduleValue.Free()

	cached, ok, err := r.cachedValue(ctx, r.cacheKey(), filename)
	if err != nil || !ok {
		return nil, fmt.Errorf("cache CommonJS module %q", filename)
	}
	exportsValue := cached.Get("exports")
	if exportsValue == nil {
		cached.Free()
		r.deleteCachedValue(ctx, r.cacheKey(), filename)
		return nil, fmt.Errorf("CommonJS module %q has no exports", filename)
	}
	requireValue := r.newRequireFunction(ctx, filename)
	if requireValue == nil {
		exportsValue.Free()
		cached.Free()
		r.deleteCachedValue(ctx, r.cacheKey(), filename)
		return nil, fmt.Errorf("create require for %q", filename)
	}
	wrapperSource := "(function(exports, require, module, __filename, __dirname) {\n" + string(source) + "\n})"
	wrapper := ctx.Eval(wrapperSource, quickjs.EvalFileName(filename))
	if wrapper == nil || wrapper.IsException() || !wrapper.IsFunction() {
		if wrapper != nil {
			if wrapper.IsException() {
				err = ctx.Exception()
			} else {
				err = ErrInvalidModule
			}
			wrapper.Free()
		}
		if err == nil {
			err = ErrInvalidModule
		}
		requireValue.Free()
		exportsValue.Free()
		cached.Free()
		r.deleteCachedValue(ctx, r.cacheKey(), filename)
		return nil, fmt.Errorf("evaluate CommonJS module %q: %w", filename, err)
	}
	dirname := filepath.Dir(filename)
	filenameArg := ctx.NewString(filename)
	dirnameArg := ctx.NewString(dirname)
	result := wrapper.Execute(exportsValue, exportsValue, requireValue, cached, filenameArg, dirnameArg)
	wrapper.Free()
	requireValue.Free()
	exportsValue.Free()
	filenameArg.Free()
	dirnameArg.Free()
	if result == nil || result.IsException() {
		if result != nil {
			err = ctx.Exception()
			result.Free()
		}
		cached.Free()
		r.deleteCachedValue(ctx, r.cacheKey(), filename)
		if err == nil {
			err = ErrInvalidModule
		}
		return nil, fmt.Errorf("evaluate CommonJS module %q: %w", filename, err)
	}
	result.Free()
	loadedValue := ctx.NewBool(true)
	cached.DefinePropertyValue("loaded", loadedValue, quickjs.PropConfigurable|quickjs.PropWritable)
	loadedValue.Free()
	value := cached.Get("exports")
	cached.Free()
	if value == nil {
		return nil, fmt.Errorf("CommonJS module %q has no exports", filename)
	}
	return value, nil
}

// ClearContext releases registry-owned bookkeeping. QuickJS cache values are
// held by the Context and are released by Context.Close; this method exists so
// hosts can clear any future provider state before closing a Context.
func (r *Registry) ClearContext(ctx *quickjs.Context) {
	_ = ctx
}
