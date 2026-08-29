package structuredclone

import (
	"testing"

	"github.com/Scardice/quickjs_nodejs/internal/testutil"
	quickjs "github.com/buke/quickjs-go"
)

func TestStructuredCloneCopiesCyclesAndBuiltins(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := InstallGlobal(ctx); err != nil {
			t.Fatal(err)
		}
		result := ctx.Eval(`(() => {
			const source = { nested: { value: 3 }, date: new Date(1234), map: new Map([["a", 1]]), set: new Set([2]), bytes: new Uint8Array([4, 5]) };
			source.self = source;
			const copy = structuredClone(source);
			copy.nested.value = 9;
			copy.bytes[0] = 8;
			return [copy !== source, copy.nested.value, source.nested.value, copy.self === copy, copy.date.getTime(), copy.map.get("a"), copy.set.has(2), copy.bytes[0], source.bytes[0]].join("|");
		})()`)
		if result == nil {
			t.Fatal("structuredClone evaluation returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("structuredClone evaluation failed: %v", ctx.Exception())
		}
		if got, want := result.ToString(), "true|9|3|true|1234|1|true|8|4"; got != want {
			t.Fatalf("structuredClone result = %q, want %q", got, want)
		}
	})
}

func TestStructuredCloneRejectsFunctions(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := InstallGlobal(ctx); err != nil {
			t.Fatal(err)
		}
		result := ctx.Eval(`(() => { try { structuredClone(() => 1); return "no-error"; } catch (error) { return error.name; } })()`)
		if result == nil {
			t.Fatal("structuredClone rejection returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("structuredClone rejection evaluation failed: %v", ctx.Exception())
		}
		if got, want := result.ToString(), "DataCloneError"; got != want {
			t.Fatalf("structuredClone rejection = %q, want %q", got, want)
		}
	})
}
