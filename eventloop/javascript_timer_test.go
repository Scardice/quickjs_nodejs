package eventloop

import (
	"errors"
	"testing"
	"time"

	quickjs "github.com/buke/quickjs-go"
)

func TestEventLoopJavaScriptTimers(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()
	if err := loop.Start(); err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	if !loop.Schedule(func(ctx *quickjs.Context) error {
		value := ctx.Eval(`
			globalThis.jsCount = 0;
			globalThis.jsTimeout = false;
			globalThis.jsCanceled = false;
			globalThis.jsImmediate = false;
			const intervalID = setInterval(() => {
				jsCount++;
				if (jsCount === 3) {
					clearInterval(intervalID);
				}
			}, 2);
			setTimeout(() => { jsTimeout = true }, 7);
			const canceledID = setTimeout(() => { jsCanceled = true }, 5);
			clearTimeout(canceledID);
			setImmediate(() => { jsImmediate = true });
		`)
		if value == nil {
			return errors.New("timer setup returned nil")
		}
		defer value.Free()
		if value.IsException() {
			return errors.New(ctx.Exception().Error())
		}
		close(started)
		return nil
	}) {
		t.Fatal("timer setup was rejected")
	}
	select {
	case <-started:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timer setup did not run")
	}

	waitFor(t, time.Second, func() bool {
		return loop.Do(func(ctx *quickjs.Context) error {
			value := ctx.Eval(`jsCount === 3 && jsTimeout && jsImmediate && !jsCanceled`)
			if value == nil {
				return errors.New("timer assertion returned nil")
			}
			defer value.Free()
			if value.IsException() || !value.ToBool() {
				return errors.New("javascript timers are not complete")
			}
			return nil
		}) == nil
	})
}
