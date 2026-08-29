package testutil

import (
	"runtime"
	"testing"

	quickjs "github.com/buke/quickjs-go"
)

// WithContext creates a no-bootstrap QuickJS context on a locked OS thread.
func WithContext(t *testing.T, fn func(*quickjs.Context)) {
	t.Helper()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	rt := quickjs.NewRuntime(
		quickjs.WithOwnerGoroutineCheck(true),
		quickjs.WithStrictOSThread(true),
		quickjs.WithModuleImport(false),
	)
	if rt == nil {
		t.Fatal("NewRuntime returned nil")
	}
	defer rt.Close()

	ctx := rt.NewContextWithOptions(quickjs.NoBootstrap())
	if ctx == nil {
		t.Fatal("NewContextWithOptions returned nil")
	}
	defer ctx.Close()
	fn(ctx)
}
