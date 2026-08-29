package abort

import (
	"testing"

	"github.com/Scardice/quickjs_nodejs/eventloop"
	quickjs "github.com/buke/quickjs-go"
)

func TestAbortControllerPropagatesReasonAndListeners(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	var result string
	if err := loop.Run(func(ctx *quickjs.Context) error {
		if err := InstallGlobal(ctx); err != nil {
			return err
		}
		value := ctx.Eval(`(() => {
			const controller = new AbortController();
			const signal = controller.signal;
			let calls = 0;
			let seen = "";
			const listener = event => { calls++; seen = event.reason.message; };
			signal.addEventListener("abort", listener, { once: true });
			controller.abort(new Error("stop"));
			controller.abort(new Error("ignored"));
			let thrown = "";
			try { signal.throwIfAborted(); } catch (error) { thrown = error.message; }
			return [signal.aborted, calls, seen, thrown, signal.reason.message].join("|");
		})()`)
		if value == nil {
			return &testError{"abort evaluation returned nil"}
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
	if got, want := result, "true|1|stop|stop|stop"; got != want {
		t.Fatalf("abort result = %q, want %q", got, want)
	}
}

func TestAbortSignalTimeoutAbortsOnEventLoopTimer(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	var result string
	if err := loop.Run(func(ctx *quickjs.Context) error {
		if err := InstallGlobal(ctx); err != nil {
			return err
		}
		value := ctx.Eval(`(() => {
			const signal = AbortSignal.timeout(1);
			setTimeout(() => { globalThis.timeoutResult = [signal.aborted, signal.reason.name, signal.reason.message].join("|"); }, 5);
		})()`)
		if value == nil {
			return &testError{"timeout setup returned nil"}
		}
		defer value.Free()
		if value.IsException() {
			return ctx.Exception()
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := loop.Run(func(ctx *quickjs.Context) error {
		value := ctx.Globals().Get("timeoutResult")
		if value == nil {
			return &testError{"timeout result is nil"}
		}
		defer value.Free()
		result = value.ToString()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got, want := result, "true|TimeoutError|The operation timed out"; got != want {
		t.Fatalf("timeout result = %q, want %q", got, want)
	}
}

type testError struct{ message string }

func (e *testError) Error() string { return e.message }
