package quickjs_nodejs

import (
	"runtime"
	"testing"

	quickjs "github.com/buke/quickjs-go"
)

func TestQuickJSSmoke(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	rt := quickjs.NewRuntime(quickjs.WithStrictOSThread(true))
	if rt == nil {
		t.Fatal("NewRuntime returned nil")
	}
	defer rt.Close()

	ctx := rt.NewContextWithOptions(quickjs.NoBootstrap())
	if ctx == nil {
		t.Fatal("NewContextWithOptions returned nil")
	}
	defer ctx.Close()

	value := ctx.Eval("1 + 2")
	if value == nil {
		t.Fatal("Eval returned nil")
	}
	defer value.Free()
	if value.ToInt32() != 3 {
		t.Fatalf("unexpected eval result: %v", value.ToInt32())
	}
}
