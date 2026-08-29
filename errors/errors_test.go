package errors

import (
	"testing"

	"github.com/Scardice/quickjs_nodejs/internal/testutil"
	quickjs "github.com/buke/quickjs-go"
)

func TestErrorConstructorsAndCodes(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		errValue := NewTypeError(ctx, ErrCodeInvalidArgType, "bad %s", "value")
		if errValue == nil {
			t.Fatal("NewTypeError returned nil")
		}
		defer errValue.Free()
		if !errValue.IsError() {
			t.Fatal("NewTypeError did not return an Error object")
		}
		code := errValue.Get("code")
		defer code.Free()
		if got := code.ToString(); got != string(ErrCodeInvalidArgType) {
			t.Fatalf("unexpected error code %q", got)
		}
		message := errValue.Get("message")
		defer message.Free()
		if got := message.ToString(); got != "bad value" {
			t.Fatalf("unexpected error message %q", got)
		}
		toString := errValue.Get("toString")
		defer toString.Free()
		if got := toString.Execute(errValue).ToString(); got != "TypeError [ERR_INVALID_ARG_TYPE]: bad value" {
			t.Fatalf("unexpected error toString %q", got)
		}
	})
}

func TestThrowTypeErrorPreservesCode(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		fn := ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, _ []*quickjs.Value) *quickjs.Value {
			return ThrowTypeError(ctx, ErrCodeMissingArgs, "missing")
		})
		ctx.Globals().Set("fail", fn)
		result := ctx.Eval(`try { fail() } catch (err) { err.name + ":" + err.code + ":" + err.message }`, quickjs.EvalAwait(false))
		if result == nil {
			t.Fatal("evaluation returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("evaluation failed: %v", ctx.Exception())
		}
		if got := result.ToString(); got != "TypeError:ERR_MISSING_ARGS:missing" {
			t.Fatalf("unexpected thrown error %q", got)
		}
	})
}
