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

// Request describes a root-relative filesystem operation. Path and Destination
// are normalized with slash separators and never contain an escaping .. segment.
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

// Config controls one fs module instance.
type Config struct {
	Root   string
	Policy Policy
	Sync   bool
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

func (a access) readFile(path string, encoding string, sync bool) (fileContents, error) {
	target, virtual, err := a.resolveExisting(path)
	if err != nil {
		return fileContents{}, err
	}
	if err := a.authorize(OperationReadFile, virtual, "", sync); err != nil {
		return fileContents{}, err
	}
	contents, err := os.ReadFile(target)
	if err != nil {
		return fileContents{}, err
	}
	return fileContents{data: contents, encoding: encoding}, nil
}

func (a access) writeFile(path string, data []byte, sync bool) error {
	target, virtual, err := a.resolveWriteTarget(path)
	if err != nil {
		return err
	}
	if err := a.authorize(OperationWriteFile, virtual, "", sync); err != nil {
		return err
	}
	return os.WriteFile(target, data, 0o600)
}

func (a access) mkdir(path string, sync bool) error {
	target, virtual, err := a.resolveWriteTarget(path)
	if err != nil {
		return err
	}
	if err := a.authorize(OperationMkdir, virtual, "", sync); err != nil {
		return err
	}
	return os.Mkdir(target, 0o700)
}

func (a access) readDir(path string, sync bool) ([]string, error) {
	target, virtual, err := a.resolveExisting(path)
	if err != nil {
		return nil, err
	}
	if err := a.authorize(OperationReadDir, virtual, "", sync); err != nil {
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
		err     error
	)
	if follow {
		target, virtual, err = a.resolveExisting(path)
	} else {
		target, virtual, err = a.resolveLeaf(path)
	}
	if err != nil {
		return nil, err
	}
	operation := OperationStat
	if !follow {
		operation = OperationLstat
	}
	if err := a.authorize(operation, virtual, "", sync); err != nil {
		return nil, err
	}
	if follow {
		return os.Stat(target)
	}
	return os.Lstat(target)
}

func (a access) unlink(path string, sync bool) error {
	target, virtual, err := a.resolveLeaf(path)
	if err != nil {
		return err
	}
	if err := a.authorize(OperationUnlink, virtual, "", sync); err != nil {
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
	sourceTarget, sourceVirtual, err := a.resolveLeaf(source)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(sourceTarget); err != nil {
		return err
	}
	destinationTarget, destinationVirtual, err := a.resolveWriteTarget(destination)
	if err != nil {
		return err
	}
	if err := a.authorize(OperationRename, sourceVirtual, destinationVirtual, sync); err != nil {
		return err
	}
	return os.Rename(sourceTarget, destinationTarget)
}

func (a access) resolveExisting(path string) (string, string, error) {
	target, virtual, err := a.resolveLeaf(path)
	if err != nil {
		return "", "", err
	}
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", "", err
	}
	if !isWithin(a.root, resolved) {
		return "", "", denied("path %q resolves outside fs root", virtual)
	}
	return resolved, virtual, nil
}

func (a access) resolveWriteTarget(path string) (string, string, error) {
	target, virtual, err := a.resolveLeaf(path)
	if err != nil {
		return "", "", err
	}
	if info, err := os.Lstat(target); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", "", denied("path %q is a symlink", virtual)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}
	return target, virtual, nil
}

func (a access) resolveLeaf(path string) (string, string, error) {
	if a.err != nil {
		return "", "", a.err
	}
	virtual, err := normalizePath(path)
	if err != nil {
		return "", "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Join(a.root, filepath.FromSlash(filepath.Dir(virtual))))
	if err != nil {
		return "", "", err
	}
	if !isWithin(a.root, parent) {
		return "", "", denied("path %q resolves outside fs root", virtual)
	}
	return filepath.Join(parent, filepath.Base(virtual)), virtual, nil
}

func (a access) authorize(operation Operation, path, destination string, sync bool) error {
	if a.config.Policy == nil {
		return denied("fs policy is required")
	}
	if err := a.config.Policy(Request{
		Operation:   operation,
		Path:        path,
		Destination: destination,
		Sync:        sync,
	}); err != nil {
		return denied("fs policy rejected %s %q: %v", operation, path, err)
	}
	return nil
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
