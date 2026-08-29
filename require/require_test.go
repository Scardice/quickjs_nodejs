package require_test

import (
	"errors"
	"testing"

	"github.com/Scardice/quickjs_nodejs/internal/testutil"
	"github.com/Scardice/quickjs_nodejs/module"
	"github.com/Scardice/quickjs_nodejs/require"
	quickjs "github.com/buke/quickjs-go"
)

func TestRequireLoadsRelativeJSONAndSharesCache(t *testing.T) {
	sources := map[string][]byte{
		"/app/main.js":   []byte(`const first = require("./lib"); const second = require("./lib.js"); module.exports = { first, second, filename: __filename, dirname: __dirname };`),
		"/app/lib.js":    []byte(`const data = require("./data.json"); data.count++; module.exports = data;`),
		"/app/data.json": []byte(`{"count": 0}`),
	}
	registry := require.NewRegistry(
		require.WithBaseDir("/app"),
		require.WithSourceLoader(func(filename string) ([]byte, error) {
			data, ok := sources[filename]
			if !ok {
				return nil, require.ErrModuleNotFound
			}
			return data, nil
		}),
	)
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if registry.Enable(ctx) == nil {
			t.Fatal("Enable returned nil")
		}
		value, err := registry.Require(ctx, "/app/main.js", "./main")
		if err != nil {
			t.Fatal(err)
		}
		ctx.Globals().Set("result", value)
		check := ctx.Eval(`[result.first === result.second, result.first.count, result.filename, result.dirname].join("|")`)
		if check == nil {
			t.Fatal("cache check returned nil")
		}
		defer check.Free()
		if check.IsException() {
			t.Fatalf("cache check failed: %v", ctx.Exception())
		}
		if got, want := check.ToString(), "true|1|/app/main.js|/app"; got != want {
			t.Fatalf("cache result = %q, want %q", got, want)
		}
	})
}

func TestRequireSupportsPartiallyInitializedCycles(t *testing.T) {
	sources := map[string][]byte{
		"/app/main.js": []byte(`const a = require("./a"); module.exports = [a.name, a.peer, a.peerPeer].join("|");`),
		"/app/a.js":    []byte(`exports.name = "a"; const b = require("./b"); exports.peer = b.name; exports.peerPeer = b.peer;`),
		"/app/b.js":    []byte(`exports.name = "b"; const a = require("./a"); exports.peer = a.name;`),
	}
	registry := require.NewRegistry(
		require.WithBaseDir("/app"),
		require.WithSourceLoader(func(filename string) ([]byte, error) {
			data, ok := sources[filename]
			if !ok {
				return nil, require.ErrModuleNotFound
			}
			return data, nil
		}),
	)
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := registry.EnableRequire(ctx); err != nil {
			t.Fatal(err)
		}
		value, err := registry.Require(ctx, "/app/main.js", "./main")
		if err != nil {
			t.Fatal(err)
		}
		defer value.Free()
		if got, want := value.ToString(), "a|b|a"; got != want {
			t.Fatalf("cycle result = %q, want %q", got, want)
		}
	})
}

func TestRequireResolvesPackageAndNodeAlias(t *testing.T) {
	sources := map[string][]byte{
		"/app/node_modules/pkg/package.json": []byte(`{"main":"src/index.js"}`),
		"/app/node_modules/pkg/src/index.js": []byte(`module.exports = { value: 7 };`),
	}
	registry := require.NewRegistry(
		require.WithBaseDir("/app"),
		require.WithSourceLoader(func(filename string) ([]byte, error) {
			data, ok := sources[filename]
			if !ok {
				return nil, require.ErrModuleNotFound
			}
			return data, nil
		}),
	)
	if err := registry.RegisterNativeModule("native:test", func(ctx *quickjs.Context, moduleValue *quickjs.Value) error {
		exports := moduleValue.Get("exports")
		defer exports.Free()
		number := ctx.NewInt32(11)
		defer number.Free()
		exports.Set("value", number)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := registry.EnableRequire(ctx); err != nil {
			t.Fatal(err)
		}
		value, err := registry.Require(ctx, "/app/main.js", "pkg")
		if err != nil {
			t.Fatal(err)
		}
		defer value.Free()
		if value.Get("value").ToInt32() != 7 {
			t.Fatal("package main was not resolved")
		}
		native, err := registry.Require(ctx, "/app/main.js", "node:native:test")
		if err != nil {
			t.Fatal(err)
		}
		defer native.Free()
		if native.Get("value").ToInt32() != 11 {
			t.Fatal("node alias native module was not resolved")
		}
	})
}

func TestRequireUsesSharedStaticModuleExports(t *testing.T) {
	registry := require.NewRegistry()
	if err := registry.Add(module.Definition{
		Name:    "shared",
		Aliases: []string{"node:shared"},
		Exports: []module.Export{{Name: "state", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
			state := ctx.NewObject()
			value := ctx.NewInt32(1)
			state.Set("value", value)
			return state, nil
		}}}},
	}); err != nil {
		t.Fatal(err)
	}
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := registry.Register(ctx); err != nil {
			t.Fatal(err)
		}
		if err := registry.EnableRequire(ctx); err != nil {
			t.Fatal(err)
		}
		value, err := registry.Require(ctx, "/app/main.js", "shared")
		if err != nil {
			t.Fatal(err)
		}
		ctx.Globals().Set("cjs", value)
		result := ctx.Eval(`(async () => { const esm = await import("shared"); esm.state.value = 9; return [cjs.state === esm.state, cjs.state.value].join("|"); })()`, quickjs.EvalAwait(true))
		if result == nil {
			t.Fatal("identity check returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("identity check failed: %v", ctx.Exception())
		}
		if got, want := result.ToString(), "true|9"; got != want {
			t.Fatalf("identity result = %q, want %q", got, want)
		}
	})
}

func TestRequireNeedsExplicitSourceLoader(t *testing.T) {
	registry := require.NewRegistry()
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := registry.EnableRequire(ctx); err != nil {
			t.Fatal(err)
		}
		_, err := registry.Require(ctx, "/app/main.js", "./missing")
		if !errors.Is(err, require.ErrModuleNotFound) {
			t.Fatalf("missing source error = %v", err)
		}
	})
}
