package conformance

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	quickjs "github.com/buke/quickjs-go"
)

type wptURLVector struct {
	Input string `json:"input"`
	Base  string `json:"base"`
	Href  string `json:"href"`
}

func suiteRoot(name string) (string, error) {
	if name != "wpt" && name != "test262" {
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
