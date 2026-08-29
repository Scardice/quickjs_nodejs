package module

import (
	"testing"

	"github.com/Scardice/quickjs_nodejs/internal/testutil"
	quickjs "github.com/buke/quickjs-go"
)

func TestRegistryKeepsStaticESMImportsWorking(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Add(Definition{
		Name:    "esm:dependency",
		Exports: []Export{{Name: "answer", Spec: quickjs.MarshalSpec{Value: int64(41)}}},
	}); err != nil {
		t.Fatal(err)
	}
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := registry.Register(ctx); err != nil {
			t.Fatal(err)
		}
		result := ctx.Eval(`import { answer } from "esm:dependency"; globalThis.staticESM = answer + 1`, quickjs.EvalAwait(true))
		if result == nil {
			t.Fatal("static ESM evaluation returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("static ESM evaluation failed: %v", ctx.Exception())
		}
		value := ctx.Globals().Get("staticESM")
		if value == nil {
			t.Fatal("static ESM global result is nil")
		}
		defer value.Free()
		if got := value.ToInt32(); got != 42 {
			t.Fatalf("static ESM result = %d, want 42", got)
		}
	})
}

func TestQuickJSSourceESMSupportsStaticAndDynamicImports(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		dependency := ctx.LoadModule(`export const answer = 41;`, "esm:source-dependency")
		if dependency == nil {
			t.Fatal("source dependency load returned nil")
		}
		defer dependency.Free()
		if dependency.IsException() {
			t.Fatalf("source dependency load failed: %v", ctx.Exception())
		}

		loaded := ctx.LoadModule(`
			import { answer } from "esm:source-dependency";
			export const value = answer;
			export default answer + 1;
		`, "esm:source")
		if loaded == nil {
			t.Fatal("source ESM load returned nil")
		}
		defer loaded.Free()
		if loaded.IsException() {
			t.Fatalf("source ESM load failed: %v", ctx.Exception())
		}

		result := ctx.Eval(`(async () => {
			const dynamicModule = await import("esm:source");
			return [dynamicModule.value, dynamicModule.default, (await import("esm:source")) === dynamicModule].join("|");
		})()`, quickjs.EvalAwait(true))
		if result == nil {
			t.Fatal("source ESM import returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("source ESM import failed: %v", ctx.Exception())
		}
		if got, want := result.ToString(), "41|42|true"; got != want {
			t.Fatalf("source ESM result = %q, want %q", got, want)
		}
	})
}
