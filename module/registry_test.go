package module

import (
	"testing"

	"github.com/Scardice/quickjs_nodejs/internal/testutil"
	quickjs "github.com/buke/quickjs-go"
)

func TestRegistryBuildsCanonicalAndNodeAliasPerContext(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Add(Definition{
		Name:    "test:math",
		Aliases: []string{"node:test:math"},
		Exports: []Export{
			{Name: "answer", Spec: quickjs.MarshalSpec{Value: int64(42)}},
			{Name: "add", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
					if len(args) == 0 {
						return ctx.NewInt32(0)
					}
					return ctx.NewInt32(args[0].ToInt32() + 1)
				}), nil
			}}},
		},
	}); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"test:math", "node:test:math"} {
		t.Run(want, func(t *testing.T) {
			testutil.WithContext(t, func(ctx *quickjs.Context) {
				if err := registry.Register(ctx); err != nil {
					t.Fatal(err)
				}
				result := ctx.Eval(`(async () => {
					const m = await import("`+want+`");
					return String(m.answer) + ":" + String(m.add(4));
				})()`, quickjs.EvalAwait(true))
				if result == nil {
					t.Fatal("module evaluation returned nil")
				}
				defer result.Free()
				if result.IsException() {
					t.Fatalf("module evaluation failed: %v", ctx.Exception())
				}
				if got := result.ToString(); got != "42:5" {
					t.Fatalf("unexpected module result %q", got)
				}
			})
		})
	}
}

func TestRegistryRejectsDuplicateNamesAndInvalidDefinitions(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Add(Definition{Name: "dup", Exports: []Export{{Name: "x", Spec: quickjs.MarshalSpec{Value: 1}}}}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Add(Definition{Name: "dup", Exports: []Export{{Name: "x", Spec: quickjs.MarshalSpec{Value: 1}}}}); err == nil {
		t.Fatal("duplicate module name was accepted")
	}
	if err := registry.Add(Definition{Name: "bad", Exports: []Export{{Name: "x"}}}); err == nil {
		t.Fatal("nil export spec was accepted")
	}
}
