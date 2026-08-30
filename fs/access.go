// Package fs provides a policy-controlled filesystem module for QuickJS.
package fs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	// ModuleName is the canonical Node-compatible filesystem module name.
	ModuleName = "fs"
	// PromisesModuleName is the canonical Promise filesystem module name.
	PromisesModuleName = "fs/promises"

	// ErrCodeAccessDenied is assigned to JavaScript errors when the root jail
	// or host policy rejects an operation.
	ErrCodeAccessDenied = "ERR_FS_ACCESS_DENIED"
	// ErrCodeIO is assigned to JavaScript errors returned by the host filesystem.
	ErrCodeIO = "ERR_FS_IO"
)

// Operation identifies one filesystem capability that a Policy may approve.
type Operation string

const (
	OperationReadFile  Operation = "readFile"
	OperationWriteFile Operation = "writeFile"
	OperationMkdir     Operation = "mkdir"
	OperationReadDir   Operation = "readdir"
	OperationStat      Operation = "stat"
	OperationLstat     Operation = "lstat"
	OperationUnlink    Operation = "unlink"
	OperationRename    Operation = "rename"
)

// Request describes a normalized filesystem operation. In root mode Path and
// Destination are root-relative; in unrestricted mode they are host-absolute.
type Request struct {
	Operation   Operation
	Path        string
	Destination string
	Sync        bool
}

// Policy decides whether one root-contained filesystem operation is allowed.
// Promise operations invoke Policy on a worker goroutine; implementations must
// be safe for concurrent calls.
type Policy func(Request) error

// SymlinkPolicy decides whether an operation that encounters a symbolic link
// may proceed. It runs after Policy and receives the same normalized request.
type SymlinkPolicy func(Request) error

// PathResolver converts a host virtual path to a root-relative path. It runs
// on the QuickJS calling thread before a Promise operation starts.
type PathResolver func(path string) (string, error)

// Config controls one fs module instance.
type Config struct {
	Root               string
	Policy             Policy
	PathResolver       PathResolver
	SymlinkPolicy      SymlinkPolicy
	UnrestrictedAccess bool
	Sync               bool
}

// Option configures one fs module instance.
type Option func(*Config)

// WithRoot confines every operation to root. An empty or unreadable root denies
// all operations.
func WithRoot(root string) Option {
	return func(config *Config) { config.Root = root }
}

// WithPolicy installs the host authorization callback. A nil policy denies all
// operations.
func WithPolicy(policy Policy) Option {
	return func(config *Config) { config.Policy = policy }
}

// WithPathResolver installs an optional host virtual-path resolver. The
// returned path remains subject to root-jail validation and Policy.
func WithPathResolver(resolver PathResolver) Option {
	return func(config *Config) { config.PathResolver = resolver }
}

// WithSymlinkPolicy installs authorization for operations that encounter a
// symbolic link. Unrestricted access denies those operations when unset.
func WithSymlinkPolicy(policy SymlinkPolicy) Option {
	return func(config *Config) { config.SymlinkPolicy = policy }
}

// WithUnrestrictedAccess removes the root jail. It is intended only for
// trusted host configurations and remains subject to Policy.
func WithUnrestrictedAccess() Option {
	return func(config *Config) { config.UnrestrictedAccess = true }
}

// WithSync controls whether fs exposes its synchronous methods. Promise APIs
// remain available regardless of this setting.
func WithSync(enabled bool) Option {
	return func(config *Config) { config.Sync = enabled }
}

func applyOptions(options []Option) Config {
	config := Config{}
	for _, option := range options {
		if option != nil {
			option(&config)
		}
	}
	return config
}

type access struct {
	config Config
	root   string
	err    error
}

func newAccess(config Config) access {
	if config.UnrestrictedAccess {
		if config.Root != "" {
			return access{config: config, err: denied("unrestricted fs cannot have a root")}
		}
		return access{config: config}
	}
	if config.Root == "" {
		return access{config: config, err: denied("fs root is required")}
	}
	absolute, err := filepath.Abs(config.Root)
	if err != nil {
		return access{config: config, err: denied("resolve fs root: %v", err)}
	}
	root, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return access{config: config, err: denied("resolve fs root: %v", err)}
	}
	info, err := os.Stat(root)
	if err != nil {
		return access{config: config, err: denied("stat fs root: %v", err)}
	}
	if !info.IsDir() {
		return access{config: config, err: denied("fs root is not a directory")}
	}
	return access{config: config, root: filepath.Clean(root)}
}

func (a access) preparePath(path string) (string, error) {
	if a.err != nil {
		return "", a.err
	}
	if a.config.PathResolver != nil {
		resolved, err := a.config.PathResolver(path)
		if err != nil {
			return "", denied("resolve fs path %q: %v", path, err)
		}
		path = resolved
	}
	if a.config.UnrestrictedAccess {
		if path == "" {
			return "", denied("path is required")
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return "", denied("resolve unrestricted path: %v", err)
		}
		return filepath.Clean(absolute), nil
	}
	return normalizePath(path)
}

func (a access) readFile(path string, encoding string, sync bool) (fileContents, error) {
	target, virtual, symlink, err := a.resolveExisting(path)
	if err != nil {
		return fileContents{}, err
	}
	if err := a.authorize(OperationReadFile, virtual, "", sync, symlink); err != nil {
		return fileContents{}, err
	}
	contents, err := os.ReadFile(target)
	if err != nil {
		return fileContents{}, err
	}
	return fileContents{data: contents, encoding: encoding}, nil
}

func (a access) writeFile(path string, data []byte, sync bool) error {
	target, virtual, symlink, err := a.resolveWriteTarget(path)
	if err != nil {
		return err
	}
	if err := a.authorize(OperationWriteFile, virtual, "", sync, symlink); err != nil {
		return err
	}
	return os.WriteFile(target, data, 0o600)
}

func (a access) mkdir(path string, sync bool) error {
	target, virtual, symlink, err := a.resolveWriteTarget(path)
	if err != nil {
		return err
	}
	if err := a.authorize(OperationMkdir, virtual, "", sync, symlink); err != nil {
		return err
	}
	return os.Mkdir(target, 0o700)
}

func (a access) readDir(path string, sync bool) ([]string, error) {
	target, virtual, symlink, err := a.resolveExisting(path)
	if err != nil {
		return nil, err
	}
	if err := a.authorize(OperationReadDir, virtual, "", sync, symlink); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for index, entry := range entries {
		names[index] = entry.Name()
	}
	return names, nil
}

func (a access) stat(path string, follow bool, sync bool) (fs.FileInfo, error) {
	var (
		target  string
		virtual string
		symlink bool
		err     error
	)
	if follow {
		target, virtual, symlink, err = a.resolveExisting(path)
	} else {
		target, virtual, symlink, err = a.resolveLeaf(path)
		if err == nil {
			var leafSymlink bool
			leafSymlink, err = isSymlink(target)
			symlink = symlink || leafSymlink
		}
	}
	if err != nil {
		return nil, err
	}
	operation := OperationStat
	if !follow {
		operation = OperationLstat
	}
	if err := a.authorize(operation, virtual, "", sync, symlink); err != nil {
		return nil, err
	}
	if follow {
		return os.Stat(target)
	}
	return os.Lstat(target)
}

func (a access) unlink(path string, sync bool) error {
	target, virtual, symlink, err := a.resolveLeaf(path)
	if err != nil {
		return err
	}
	leafSymlink, err := isSymlink(target)
	if err != nil {
		return err
	}
	if err := a.authorize(OperationUnlink, virtual, "", sync, symlink || leafSymlink); err != nil {
		return err
	}
	info, err := os.Lstat(target)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("unlink %q: is a directory", virtual)
	}
	return os.Remove(target)
}

func (a access) rename(source, destination string, sync bool) error {
	sourceTarget, sourceVirtual, sourceSymlink, err := a.resolveLeaf(source)
	if err != nil {
		return err
	}
	sourceInfo, err := os.Lstat(sourceTarget)
	if err != nil {
		return err
	}
	destinationTarget, destinationVirtual, destinationSymlink, err := a.resolveWriteTarget(destination)
	if err != nil {
		return err
	}
	if err := a.authorize(
		OperationRename,
		sourceVirtual,
		destinationVirtual,
		sync,
		sourceSymlink || sourceInfo.Mode()&os.ModeSymlink != 0 || destinationSymlink,
	); err != nil {
		return err
	}
	return os.Rename(sourceTarget, destinationTarget)
}

func (a access) resolveExisting(path string) (string, string, bool, error) {
	target, virtual, symlink, err := a.resolveLeaf(path)
	if err != nil {
		return "", "", false, err
	}
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", "", false, err
	}
	if !a.config.UnrestrictedAccess && !isWithin(a.root, resolved) {
		return "", "", false, denied("path %q resolves outside fs root", virtual)
	}
	return resolved, virtual, symlink || !samePath(target, resolved), nil
}

func (a access) resolveWriteTarget(path string) (string, string, bool, error) {
	target, virtual, symlink, err := a.resolveLeaf(path)
	if err != nil {
		return "", "", false, err
	}
	leafSymlink, err := isSymlink(target)
	if err != nil {
		return "", "", false, err
	}
	if leafSymlink && !a.config.UnrestrictedAccess {
		return "", "", false, denied("path %q is a symlink", virtual)
	}
	return target, virtual, symlink || leafSymlink, nil
}

func (a access) resolveLeaf(path string) (string, string, bool, error) {
	if a.err != nil {
		return "", "", false, a.err
	}
	if a.config.UnrestrictedAccess {
		target, err := filepath.Abs(path)
		if err != nil {
			return "", "", false, err
		}
		originalParent := filepath.Dir(target)
		parent, err := filepath.EvalSymlinks(originalParent)
		if err != nil {
			return "", "", false, err
		}
		return filepath.Join(parent, filepath.Base(target)), filepath.ToSlash(target), !samePath(originalParent, parent), nil
	}

	virtual, err := normalizePath(path)
	if err != nil {
		return "", "", false, err
	}
	originalParent := filepath.Join(a.root, filepath.FromSlash(filepath.Dir(virtual)))
	parent, err := filepath.EvalSymlinks(originalParent)
	if err != nil {
		return "", "", false, err
	}
	if !isWithin(a.root, parent) {
		return "", "", false, denied("path %q resolves outside fs root", virtual)
	}
	return filepath.Join(parent, filepath.Base(virtual)), virtual, !samePath(originalParent, parent), nil
}

func (a access) authorize(operation Operation, path, destination string, sync bool, symlink bool) error {
	request := Request{
		Operation:   operation,
		Path:        path,
		Destination: destination,
		Sync:        sync,
	}
	if a.config.Policy == nil {
		return denied("fs policy is required")
	}
	if err := a.config.Policy(request); err != nil {
		return denied("fs policy rejected %s %q: %v", operation, path, err)
	}
	if !symlink {
		return nil
	}
	if a.config.SymlinkPolicy == nil {
		if a.config.UnrestrictedAccess {
			return denied("fs symlink policy is required")
		}
		return nil
	}
	if err := a.config.SymlinkPolicy(request); err != nil {
		return denied("fs symlink policy rejected %s %q: %v", operation, path, err)
	}
	return nil
}

func isSymlink(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.Mode()&os.ModeSymlink != 0, nil
}

func samePath(left, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}

func normalizePath(path string) (string, error) {
	if path == "" || filepath.IsAbs(path) || filepath.VolumeName(path) != "" {
		return "", denied("path must be a non-empty relative path")
	}
	cleaned := filepath.Clean(path)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", denied("path escapes fs root")
	}
	return filepath.ToSlash(cleaned), nil
}

func isWithin(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

type accessDeniedError struct {
	message string
}

func (err *accessDeniedError) Error() string {
	return err.message
}

func denied(format string, args ...any) error {
	return &accessDeniedError{message: fmt.Sprintf(format, args...)}
}

type fileContents struct {
	data     []byte
	encoding string
}
