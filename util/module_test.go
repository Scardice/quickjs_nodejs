package util

import (
	"testing"

	"github.com/Scardice/quickjs_nodejs/internal/testutil"
	"github.com/Scardice/quickjs_nodejs/module"
	quickjs "github.com/buke/quickjs-go"
)

func TestFormatPlaceholders(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		values := []*quickjs.Value{
			ctx.NewString("Test: %% %д %s %d, %j %o"),
			ctx.NewString("string"),
			ctx.NewInt32(42),
			ctx.NewObject(),
			ctx.NewObject(),
		}
		for _, value := range values {
			defer value.Free()
		}
		if got := Format(ctx, values); got != "Test: % %д string 42, {} {}" {
			t.Fatalf("unexpected format result %q", got)
		}
	})
}

func TestFormatMissingAndExtraArguments(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		format := ctx.NewString("Test: %s %d, %j")
		defer format.Free()
		if got := Format(ctx, []*quickjs.Value{format}); got != "Test: %s %d, %j" {
			t.Fatalf("unexpected no-argument result %q", got)
		}

		first := ctx.NewString("string")
		second := ctx.NewInt32(42)
		object := ctx.NewObject()
		extra := ctx.NewFloat64(42.42)
		defer first.Free()
		defer second.Free()
		defer object.Free()
		defer extra.Free()
		if got := Format(ctx, []*quickjs.Value{format, first, second, object, extra}); got != "Test: string 42, {} 42.42" {
			t.Fatalf("unexpected extra-argument result %q", got)
		}
	})
}

func TestModuleExportsFormat(t *testing.T) {
	registry := module.NewRegistry()
	if err := registry.Add(Module()); err != nil {
		t.Fatal(err)
	}
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := registry.Register(ctx); err != nil {
			t.Fatal(err)
		}
		result := ctx.Eval(`(async () => {
			const { format } = await import("node:util");
			return format("%s:%d", "ok", 7);
		})()`, quickjs.EvalAwait(true))
		if result == nil {
			t.Fatal("format evaluation returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("format evaluation failed: %v", ctx.Exception())
		}
		if got := result.ToString(); got != "ok:7" {
			t.Fatalf("unexpected module result %q", got)
		}
	})
}
