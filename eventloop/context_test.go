package eventloop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Scardice/quickjs_nodejs/module"
	quickjs "github.com/buke/quickjs-go"
)

func TestContextAdapterBindAndOwnerCalls(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	adapter := loop.Context()
	if adapter == nil || adapter != loop.Context() {
		t.Fatal("Context did not return a stable adapter")
	}
	binding := &contextBinding{Value: 2}
	if err := loop.ContextTask(func(ctx *Context) error {
		if ctx.Raw() == nil {
			return errors.New("adapter has no raw context")
		}
		return ctx.Bind("host", binding)
	}); err != nil {
		t.Fatal(err)
	}
	if err := loop.Start(); err != nil {
		t.Fatal(err)
	}
	var result string
	if err := loop.DoContext(func(ctx *Context) error {
		value := ctx.Raw().Eval(`host.value = 5; [host.value, host.add(7), host.value].join("|")`)
		if value == nil {
			return errors.New("binding evaluation returned nil")
		}
		defer value.Free()
		if value.IsException() {
			return fmt.Errorf("binding evaluation failed: %v", ctx.Raw().Exception())
		}
		result = value.ToString()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if result != "5|12|5" || binding.Value != 5 {
		t.Fatalf("binding result = %q, value = %d", result, binding.Value)
	}
}

func TestContextAdapterDoFromForeignGoroutine(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()
	if err := loop.Start(); err != nil {
		t.Fatal(err)
	}
	var called atomic.Bool
	done := make(chan error, 1)
	go func() {
		done <- loop.DoContext(func(ctx *Context) error {
			called.Store(ctx.Raw() != nil)
			return nil
		})
	}()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !called.Load() {
		t.Fatal("foreign DoContext did not execute on an active context")
	}
}

func TestEventLoopResourceCloseAndReloadGeneration(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()
	oldGeneration := loop.Generation()
	oldRaw := loop.Context().Raw()
	var closed atomic.Int32
	var closeGID atomic.Uint64
	resource := resourceFunc(func() error {
		closed.Add(1)
		closeGID.Store(currentGoroutineID())
		return nil
	})
	if !loop.RegisterResource(resource) {
		t.Fatal("resource registration failed")
	}
	if err := loop.Reload(); err != nil {
		t.Fatal(err)
	}
	if loop.Generation() != oldGeneration+1 {
		t.Fatalf("generation = %d, want %d", loop.Generation(), oldGeneration+1)
	}
	if loop.Context().Raw() == oldRaw || loop.Context().Raw() == nil {
		t.Fatal("reload did not replace the raw context")
	}
	if closed.Load() != 1 || closeGID.Load() == 0 {
		t.Fatalf("resource close count/goroutine = %d/%d", closed.Load(), closeGID.Load())
	}
	if err := loop.Start(); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	if !loop.Schedule(func(*quickjs.Context) error {
		calls.Add(1)
		return nil
	}) {
		t.Fatal("new-generation task was rejected")
	}
	waitFor(t, time.Second, func() bool { return calls.Load() == 1 })
}

func TestEventLoopReloadDropsOldGenerationWork(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	oldGeneration := loop.Generation()
	var calls atomic.Int32
	if !loop.Schedule(func(*quickjs.Context) error {
		calls.Add(1)
		return nil
	}) {
		t.Fatal("old-generation task was rejected before reload")
	}
	if loop.SetTimeout(func(*quickjs.Context) error {
		calls.Add(1)
		return nil
	}, time.Millisecond) == nil {
		t.Fatal("old-generation timer was rejected before reload")
	}
	if err := loop.Reload(); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(loop.CheckGeneration(oldGeneration), ErrStaleGeneration) {
		t.Fatal("old generation was not rejected")
	}
	if err := loop.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(25 * time.Millisecond)
	if calls.Load() != 0 {
		t.Fatalf("old-generation work ran after reload: %d calls", calls.Load())
	}
}

func TestEventLoopDisablesDefaultFileESMLoaderWithoutDisablingRegisteredModules(t *testing.T) {
	registry := module.NewRegistry()
	if err := registry.Add(module.Definition{
		Name:    "esm:controlled",
		Exports: []module.Export{{Name: "answer", Spec: quickjs.MarshalSpec{Value: int64(42)}}},
	}); err != nil {
		t.Fatal(err)
	}
	loop, err := New(WithRegistry(registry), WithModuleImport(false))
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	filename := filepath.Join(t.TempDir(), "blocked.js")
	if err := os.WriteFile(filename, []byte(`export const answer = 7;`), 0o600); err != nil {
		t.Fatal(err)
	}
	var result string
	if err := loop.Run(func(ctx *quickjs.Context) error {
		value := ctx.Eval(`(async () => {
			const registered = await import("esm:controlled");
			try {
				await import(`+strconv.Quote(filename)+`);
				return registered.answer + "|loaded";
			} catch (_) {
				return registered.answer + "|blocked";
			}
		})()`, quickjs.EvalAwait(true))
		if value == nil {
			return errors.New("module import result is nil")
		}
		defer value.Free()
		if value.IsException() {
			return ctx.Exception()
		}
		result = value.ToString()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if result != "42|blocked" {
		t.Fatalf("module loader result = %q, want 42|blocked", result)
	}
}

func TestEventLoopDefaultSupportsFileBackedESM(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	filename := filepath.Join(t.TempDir(), "module.js")
	if err := os.WriteFile(filename, []byte(`export const answer = 42;`), 0o600); err != nil {
		t.Fatal(err)
	}
	var result string
	if err := loop.Run(func(ctx *quickjs.Context) error {
		value := ctx.Eval(`(async () => (await import(`+strconv.Quote(filename)+`)).answer)()`, quickjs.EvalAwait(true))
		if value == nil {
			return errors.New("file ESM result is nil")
		}
		defer value.Free()
		if value.IsException() {
			return ctx.Exception()
		}
		result = value.ToString()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if result != "42" {
		t.Fatalf("file ESM result = %q, want 42", result)
	}
}

type contextBinding struct {
	Value  int    `js:"value"`
	Hidden string `json:"-"`
}

func (b *contextBinding) Add(value int) (int, error) {
	return b.Value + value, nil
}

type resourceFunc func() error

func (resource resourceFunc) Close() error { return resource() }
