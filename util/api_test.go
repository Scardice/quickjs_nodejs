package util

import (
	"testing"

	"github.com/Scardice/quickjs_nodejs/internal/testutil"
	"github.com/Scardice/quickjs_nodejs/module"
	quickjs "github.com/buke/quickjs-go"
)

func TestUtilExportsInspectTypesAndPromiseAdapters(t *testing.T) {
	registry := module.NewRegistry()
	if err := registry.Add(Module()); err != nil {
		t.Fatal(err)
	}
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := registry.Register(ctx); err != nil {
			t.Fatal(err)
		}
		if err := registry.EnableRequire(ctx); err != nil {
			t.Fatal(err)
		}
		result := ctx.Eval(`(async () => {
			const util = require("util");
			const object = { a: 1 }; object.self = object;
			const inspected = util.inspect(object);
			const types = [util.types.isMap(new Map()), util.types.isDate(new Date()), util.types.isUint8Array(new Uint8Array(1))].join(",");
			const add = (value, callback) => callback(null, value + 1);
			const addAsync = util.promisify(add);
			const callbackified = util.callbackify(async value => value + 2);
			const callbackResult = await new Promise((resolve, reject) => callbackified(3, (error, value) => error ? reject(error) : resolve(value)));
			return [inspected, types, await addAsync(4), callbackResult].join("|");
		})()`, quickjs.EvalAwait(true))
		if result == nil {
			t.Fatal("util evaluation returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("util evaluation failed: %v", ctx.Exception())
		}
		if got, want := result.ToString(), "{ a: 1, self: [Circular] }|true,true,true|5|5"; got != want {
			t.Fatalf("util result = %q, want %q", got, want)
		}
	})
}
