package module

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"

	quickjs "github.com/buke/quickjs-go"
)

// SourceLoader returns source bytes for a canonical module path. Hosts should
// return ErrModuleNotFound for paths that are not readable modules.
type SourceLoader func(filename string) ([]byte, error)

// PathResolver canonicalizes a module specifier relative to an already
// canonical base directory.
type PathResolver func(base, specifier string) string

// NativeLoader initializes module.exports for a native module.
type NativeLoader func(ctx *quickjs.Context, module *quickjs.Value) error

// ErrModuleNotFound tells the resolver to continue probing another candidate.
var ErrModuleNotFound = errors.New("module file does not exist")

// ErrInvalidModule reports malformed module source or package metadata.
var ErrInvalidModule = errors.New("invalid module")

// ErrNoSuchBuiltin reports an unknown native module.
var ErrNoSuchBuiltin = errors.New("no such built-in module")

// RegistryOption configures source and native module behavior.
type RegistryOption func(*Registry)

// WithSourceLoader installs the host-controlled source reader used by
// CommonJS. A nil loader disables path-backed source loading.
func WithSourceLoader(loader SourceLoader) RegistryOption {
	return func(r *Registry) { r.sourceLoader = loader }
}

// WithPathResolver installs the canonical path resolver used by CommonJS.
func WithPathResolver(resolver PathResolver) RegistryOption {
	return func(r *Registry) { r.pathResolver = resolver }
}

// WithBaseDir sets the base directory for top-level relative requires.
func WithBaseDir(base string) RegistryOption {
	return func(r *Registry) {
		if base != "" {
			r.baseDir = filepath.Clean(filepath.FromSlash(base))
		}
	}
}

// WithGlobalFolders appends package search roots used after local
// node_modules lookup.
func WithGlobalFolders(folders ...string) RegistryOption {
	return func(r *Registry) {
		r.globalFolders = append([]string(nil), folders...)
	}
}

// DefaultSourceLoader reads a regular file from the host filesystem. It is an
// explicit helper; Registry never selects it implicitly.
func DefaultSourceLoader(filename string) ([]byte, error) {
	file, err := os.Open(filename)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrModuleNotFound
		}
		if runtime.GOOS == "windows" && strings.Contains(err.Error(), "invalid") {
			return nil, ErrModuleNotFound
		}
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, ErrModuleNotFound
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// DefaultPathResolver joins a canonical base directory and a slash-separated
// module specifier, resolving symlinks when the target exists.
func DefaultPathResolver(base, specifier string) string {
	path := filepath.Join(base, filepath.FromSlash(specifier))
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}

type nativeDefinition struct {
	name    string
	aliases []string
	loader  NativeLoader
}

var nextRegistryID atomic.Uint64

func newRegistryID() uint64 {
	id := nextRegistryID.Add(1)
	if id == 0 {
		return nextRegistryID.Add(1)
	}
	return id
}

func normalizeNativeNames(name string) (string, []string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil, errors.New("native module name is empty")
	}
	if strings.ContainsAny(name, "\r\n\t") {
		return "", nil, errors.New("native module name contains control whitespace")
	}
	canonical := strings.TrimPrefix(name, "node:")
	if canonical == "" {
		return "", nil, errors.New("native module name is empty")
	}
	aliases := []string{name}
	if name == canonical {
		aliases = append(aliases, "node:"+canonical)
	} else {
		aliases = append(aliases, canonical)
	}
	return canonical, aliases, nil
}

// AddSource registers an in-memory CommonJS source using the registry's path
// normalization rules. It is useful for tests and embedded plugin stores.
func (r *Registry) AddSource(filename, source string) error {
	if r == nil {
		return errors.New("module registry is nil")
	}
	if filename == "" {
		return errors.New("module source filename is empty")
	}
	canonical := r.resolvePath("", filename)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sources == nil {
		r.sources = make(map[string][]byte)
	}
	r.sources[canonical] = []byte(source)
	return nil
}

// RegisterNativeModule maps a native CommonJS module to a host loader. Names
// without a node: prefix automatically receive the corresponding node: alias.
func (r *Registry) RegisterNativeModule(name string, loader NativeLoader) error {
	if r == nil {
		return errors.New("module registry is nil")
	}
	if loader == nil {
		return errors.New("native module loader is nil")
	}
	canonical, aliases, err := normalizeNativeNames(name)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.native == nil {
		r.native = make(map[string]nativeDefinition)
	}
	for _, candidate := range aliases {
		if _, ok := r.definitions[candidate]; ok {
			return errors.New("module name already registered: " + candidate)
		}
		if existing, ok := r.native[candidate]; ok {
			return errors.New("native module name already registered by " + existing.name)
		}
	}
	def := nativeDefinition{name: canonical, aliases: aliases, loader: loader}
	for _, candidate := range aliases {
		r.native[candidate] = def
	}
	return nil
}

func (r *Registry) source(filename string) ([]byte, error) {
	r.mu.RLock()
	if data, ok := r.sources[filename]; ok {
		copyData := append([]byte(nil), data...)
		r.mu.RUnlock()
		return copyData, nil
	}
	loader := r.sourceLoader
	r.mu.RUnlock()
	if loader == nil {
		return nil, ErrModuleNotFound
	}
	return loader(filename)
}
func (r *Registry) resolvePath(base, specifier string) string {
	r.mu.RLock()
	resolver := r.pathResolver
	baseDir := r.baseDir
	r.mu.RUnlock()
	if base == "" && !filepath.IsAbs(filepath.FromSlash(specifier)) {
		base = baseDir
	}
	if resolver != nil {
		return resolver(base, specifier)
	}
	return DefaultPathResolver(base, specifier)
}

func (r *Registry) configuredGlobalFolders() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]string(nil), r.globalFolders...)
}
