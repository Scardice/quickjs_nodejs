package process

import (
	"testing"

	"github.com/Scardice/quickjs_nodejs/internal/testutil"
	"github.com/Scardice/quickjs_nodejs/module"
	quickjs "github.com/buke/quickjs-go"
)

func TestProcessEnvSnapshotIsContextLocal(t *testing.T) {
	const key = "QUICKJS_NODEJS_PROCESS_TEST"
	registry := module.NewRegistry()
	if err := registry.Add(Module(WithEnvSnapshot(map[string]string{key: "host"}))); err != nil {
		t.Fatal(err)
	}
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := registry.Register(ctx); err != nil {
			t.Fatal(err)
		}
		result := ctx.Eval(`(async () => {
			const processModule = await import("process");
			processModule.default.env.QUICKJS_NODEJS_PROCESS_TEST = "changed";
			return processModule.env.QUICKJS_NODEJS_PROCESS_TEST + ":" + processModule.default.env.QUICKJS_NODEJS_PROCESS_TEST;
		})()`, quickjs.EvalAwait(true))
		if result == nil {
			t.Fatal("process evaluation returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("process evaluation failed: %v", ctx.Exception())
		}
		if got := result.ToString(); got != "host:changed" {
			t.Fatalf("unexpected process result %q", got)
		}
	})

	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := registry.Register(ctx); err != nil {
			t.Fatal(err)
		}
		result := ctx.Eval(`(async () => (await import("node:process")).env.QUICKJS_NODEJS_PROCESS_TEST)()`, quickjs.EvalAwait(true))
		if result == nil {
			t.Fatal("process alias evaluation returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("process alias evaluation failed: %v", ctx.Exception())
		}
		if got := result.ToString(); got != "host" {
			t.Fatalf("unexpected process snapshot %q", got)
		}
	})
}

func TestProcessGlobalInstallIsExplicit(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		implicit := ctx.Eval(`typeof process`)
		if got := implicit.ToString(); got != "undefined" {
			implicit.Free()
			t.Fatalf("process was installed implicitly as %q", got)
		}
		implicit.Free()
		if err := InstallGlobal(ctx); err != nil {
			t.Fatal(err)
		}
		result := ctx.Eval(`typeof process.env`)
		defer result.Free()
		if result.ToString() != "object" {
			t.Fatalf("unexpected global process type %q", result.ToString())
		}
	})
}

func TestProcessEnvDefaultsToEmpty(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := InstallGlobal(ctx); err != nil {
			t.Fatal(err)
		}
		result := ctx.Eval(`Object.keys(process.env).length`)
		defer result.Free()
		if result.IsException() {
			t.Fatalf("process env evaluation failed: %v", ctx.Exception())
		}
		if got := result.ToInt32(); got != 0 {
			t.Fatalf("default process.env is not empty: %d", got)
		}
	})
}
