package conformance

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Scardice/quickjs_nodejs/eventloop"
	quickjs "github.com/buke/quickjs-go"
)

type wptURLVector struct {
	Input string `json:"input"`
	Base  string `json:"base"`
	Href  string `json:"href"`
}

func suiteRoot(name string) (string, error) {
	if name != "wpt" && name != "test262" && name != "wycheproof" {
		return "", fmt.Errorf("unsupported conformance suite %q", name)
	}

	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			root := filepath.Join(dir, "testdata", name)
			if info, err := os.Stat(root); err != nil || !info.IsDir() {
				return "", fmt.Errorf("%s test suite is unavailable; run git submodule update --init --recursive", name)
			}
			return root, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("repository root containing go.mod was not found")
		}
		dir = parent
	}
}

func firstWPTURLVector(path string) (wptURLVector, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return wptURLVector{}, err
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(data, &entries); err != nil {
		return wptURLVector{}, err
	}
	for _, entry := range entries {
		if len(entry) == 0 || entry[0] == '"' {
			continue
		}
		var vector wptURLVector
		if err := json.Unmarshal(entry, &vector); err != nil {
			return wptURLVector{}, err
		}
		if vector.Input != "" && vector.Href != "" {
			return vector, nil
		}
	}
	return wptURLVector{}, errors.New("WPT URL test data contains no successful vector")
}

func runTest262Script(root, testPath string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	rt := quickjs.NewRuntime(
		quickjs.WithOwnerGoroutineCheck(true),
		quickjs.WithStrictOSThread(true),
		quickjs.WithModuleImport(false),
	)
	if rt == nil {
		return errors.New("create QuickJS runtime")
	}
	defer rt.Close()
	ctx := rt.NewContextWithOptions(quickjs.NoBootstrap())
	if ctx == nil {
		return errors.New("create QuickJS context")
	}
	defer ctx.Close()

	for _, path := range []string{
		filepath.Join(root, "harness", "sta.js"),
		filepath.Join(root, "harness", "assert.js"),
		filepath.Join(root, filepath.FromSlash(testPath)),
	} {
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		value := ctx.Eval(string(source))
		if value == nil {
			return fmt.Errorf("%s returned nil", path)
		}
		if value.IsException() {
			value.Free()
			return fmt.Errorf("%s: %w", path, ctx.Exception())
		}
		value.Free()
	}
	return nil
}

// WPTTestResult is one testharness.js test result.
type WPTTestResult struct {
	Name    string `json:"name"`
	Status  int    `json:"status"`
	Message string `json:"message"`
}

// WPTHarnessResult is the completed result reported by testharness.js.
type WPTHarnessResult struct {
	Tests    []WPTTestResult `json:"tests"`
	Status   int             `json:"status"`
	Message  string          `json:"message"`
	Complete bool            `json:"-"`
}

func runWPTHarness(root, testPath string, installers ...eventloop.GlobalInstaller) (WPTHarnessResult, error) {
	loop, err := eventloop.New(
		eventloop.WithModuleImport(false),
		eventloop.WithGlobals(installers...),
	)
	if err != nil {
		return WPTHarnessResult{}, err
	}
	defer func() {
		_ = loop.Close()
	}()

	var encoded string
	err = loop.Run(func(ctx *quickjs.Context) error {
		if err := evalWPTHarnessSource(ctx, `globalThis.self = globalThis; globalThis.location = { search: "", protocol: "https:", hostname: "web-platform.test", href: "https://web-platform.test/" }; globalThis.GLOBAL = { isWindow: () => false, isWorker: () => false, isShadowRealm: () => false }`); err != nil {
			return err
		}
		for _, path := range append([]string{filepath.Join(root, "resources", "testharness.js")}, wptScriptPaths(root, testPath)...) {
			source, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if err := evalWPTHarnessSource(ctx, string(source)); err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
		}

		promise := ctx.Eval(`new Promise(resolve => {
			add_completion_callback((tests, status) => resolve(JSON.stringify({
				tests: tests.map(test => ({
					name: test.name,
					status: test.status,
					message: String(test.message || "")
				})),
				status: status.status,
				message: String(status.message || "")
			})));
		})`)
		if promise == nil {
			return errors.New("WPT completion returned nil")
		}
		if promise.IsException() {
			defer promise.Free()
			return ctx.Exception()
		}
		if !promise.IsPromise() {
			promise.Free()
			return errors.New("WPT completion did not return a promise")
		}
		value := ctx.Await(promise)
		promise.Free()
		if value == nil {
			return errors.New("WPT completion await returned nil")
		}
		defer value.Free()
		if value.IsException() {
			return ctx.Exception()
		}
		encoded = value.ToString()
		return nil
	})
	if err != nil {
		return WPTHarnessResult{}, err
	}

	var result WPTHarnessResult
	if err := json.Unmarshal([]byte(encoded), &result); err != nil {
		return WPTHarnessResult{}, err
	}
	result.Complete = true
	return result, nil
}

func evalWPTHarnessSource(ctx *quickjs.Context, source string) error {
	value := ctx.Eval(source)
	if value == nil {
		return errors.New("WPT source returned nil")
	}
	defer value.Free()
	if value.IsException() {
		return ctx.Exception()
	}
	return nil
}

func wptScriptPaths(root, testPath string) []string {
	sourcePath := filepath.Join(root, filepath.FromSlash(testPath))
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		return []string{sourcePath}
	}

	paths := make([]string, 0)
	for _, line := range strings.Split(string(source), "\n") {
		const prefix = "// META: script="
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		resource := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		if strings.HasPrefix(resource, "/") {
			paths = append(paths, filepath.Join(root, filepath.FromSlash(resource)))
			continue
		}
		paths = append(paths, filepath.Join(filepath.Dir(sourcePath), filepath.FromSlash(resource)))
	}
	return append(paths, sourcePath)
}
