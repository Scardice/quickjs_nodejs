package structuredclone

import (
	"testing"

	"github.com/Scardice/quickjs_nodejs/blob"
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
		result := ctx.Eval(`(() => { try { structuredClone(() => 1); return "no-error"; } catch (error) { return error.name + ":" + error.code; } })()`)
		if result == nil {
			t.Fatal("structuredClone rejection returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("structuredClone rejection evaluation failed: %v", ctx.Exception())
		}
		if got, want := result.ToString(), "DataCloneError:25"; got != want {
			t.Fatalf("structuredClone rejection = %q, want %q", got, want)
		}
	})
}

func TestStructuredCloneCopiesBlobAndFile(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := blob.InstallGlobal(ctx); err != nil {
			t.Fatal(err)
		}
		if err := InstallGlobal(ctx); err != nil {
			t.Fatal(err)
		}
		result := ctx.Eval(`(async () => {
			const source = new Blob(["data"], {type: "text/plain"});
			const copy = structuredClone(source);
			const file = new File(["report"], "report.txt", {type: "text/plain", lastModified: 123});
			const fileCopy = structuredClone(file);
			return [
				copy instanceof Blob,
				copy !== source,
				copy.type,
				await copy.text(),
				fileCopy instanceof File,
				fileCopy !== file,
				fileCopy.name,
				fileCopy.lastModified,
				await fileCopy.text()
			].join("|");
		})()`, quickjs.EvalAwait(true))
		if result == nil {
			t.Fatal("Blob structuredClone evaluation returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("Blob structuredClone evaluation failed: %v", ctx.Exception())
		}
		if got, want := result.ToString(), "true|true|text/plain|data|true|true|report.txt|123|report"; got != want {
			t.Fatalf("Blob structuredClone result = %q, want %q", got, want)
		}
	})
}

func TestStructuredCloneRetainsBuiltInWrappers(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := InstallGlobal(ctx); err != nil {
			t.Fatal(err)
		}
		result := ctx.Eval(`(() => {
			const expression = /value/gi;
			expression.lastIndex = 2;
			const error = new TypeError("broken");
			error.cause = "cause";
			error.extra = "discard";
			const copy = structuredClone({
				boolean: new Boolean(true),
				string: new String("value"),
				number: new Number(-0),
				bigint: Object(12n),
				expression,
				error
			});
			return [
				copy.boolean instanceof Boolean,
				copy.boolean.valueOf(),
				copy.string instanceof String,
				copy.string.valueOf(),
				copy.number instanceof Number,
				Object.is(copy.number.valueOf(), -0),
				copy.bigint instanceof BigInt,
				copy.bigint.valueOf() === 12n,
				copy.expression.lastIndex,
				copy.error instanceof TypeError,
				copy.error.message,
				copy.error.cause,
				copy.error.extra === undefined
			].join("|");
		})()`)
		if result == nil {
			t.Fatal("structuredClone wrapper evaluation returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("structuredClone wrapper evaluation failed: %v", ctx.Exception())
		}
		if got, want := result.ToString(), "true|true|true|value|true|true|true|true|0|true|broken|cause|true"; got != want {
			t.Fatalf("structuredClone wrappers = %q, want %q", got, want)
		}
	})
}

func TestStructuredCloneRetainsBlobWhenGlobalIsDeleted(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := InstallGlobal(ctx); err != nil {
			t.Fatal(err)
		}
		result := ctx.Eval(`(() => {
			const BlobConstructor = Blob;
			const original = new Blob(["data"]);
			delete globalThis.Blob;
			try {
				return structuredClone(original) instanceof BlobConstructor;
			} finally {
				globalThis.Blob = BlobConstructor;
			}
		})()`)
		if result == nil {
			t.Fatal("structuredClone Blob evaluation returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("structuredClone Blob evaluation failed: %v", ctx.Exception())
		}
		if !result.ToBool() {
			t.Fatal("structuredClone did not retain the installed Blob constructor")
		}
	})
}
