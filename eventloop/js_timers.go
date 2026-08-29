package eventloop

import (
	"container/heap"
	"errors"
	"fmt"
	"math"
	"time"

	quickjs "github.com/buke/quickjs-go"
)

const (
	maxTimerID       = uint64(^uint32(0))
	maxTimerDuration = time.Duration(1<<63 - 1)
)

func (l *EventLoop) installJSTimers() error {
	bindings := []struct {
		name string
		fn   func(*quickjs.Context, *quickjs.Value, []*quickjs.Value) *quickjs.Value
	}{
		{name: "setTimeout", fn: l.jsSetTimeout},
		{name: "setInterval", fn: l.jsSetInterval},
		{name: "setImmediate", fn: l.jsSetImmediate},
		{name: "clearTimeout", fn: l.jsClearTimer},
		{name: "clearInterval", fn: l.jsClearTimer},
		{name: "clearImmediate", fn: l.jsClearTimer},
	}
	globals := l.ctx.Globals()
	if globals == nil {
		return errors.New("quickjs global object initialization failed")
	}
	for _, binding := range bindings {
		value := l.ctx.NewFunction(binding.fn)
		if value == nil {
			return fmt.Errorf("create JavaScript timer binding %q failed", binding.name)
		}
		globals.Set(binding.name, value)
	}
	return nil
}

func (l *EventLoop) jsSetTimeout(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	return l.jsAddTimer(ctx, args, timerTimeout)
}

func (l *EventLoop) jsSetInterval(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	return l.jsAddTimer(ctx, args, timerInterval)
}

func (l *EventLoop) jsSetImmediate(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	return l.jsAddTimer(ctx, args, timerImmediate)
}

func (l *EventLoop) jsAddTimer(ctx *quickjs.Context, args []*quickjs.Value, kind timerKind) *quickjs.Value {
	if len(args) == 0 || !args[0].IsFunction() {
		return ctx.ThrowTypeError("timer callback must be a function")
	}
	id := l.nextID.Add(1)
	handle := &timerHandle{loop: l, id: id}
	interval := time.Duration(0)
	delay := time.Duration(0)
	if kind == timerInterval {
		interval = jsDelay(args, 1)
		if interval <= 0 {
			interval = time.Nanosecond
		}
		delay = interval
	} else if kind == timerTimeout {
		delay = jsDelay(args, 1)
	}
	entry := &timerEntry{
		id:         id,
		generation: l.Generation(),
		deadline:   time.Now().Add(delay),
		interval:   interval,
		kind:       kind,
		handle:     handle,
		sequence:   l.nextID.Add(1),
		task: func(inner *quickjs.Context) error {
			return l.runJSCallback(inner, id, kind == timerInterval)
		},
	}
	l.timerCallbacks.SetInt64(int64(id), args[0])
	l.jsTimers[id] = entry
	heap.Push(&l.timers, entry)
	return ctx.NewInt64(int64(id))
}

func (l *EventLoop) jsClearTimer(_ *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	if len(args) == 0 {
		return nil
	}
	value := args[0].ToInt64()
	if value < 0 {
		return nil
	}
	l.cancelJSTimer(uint64(value))
	return nil
}

func (l *EventLoop) cancelJSTimer(id uint64) {
	entry, ok := l.jsTimers[id]
	if !ok {
		return
	}
	entry.handle.cancel()
	delete(l.jsTimers, id)
	if id <= maxTimerID {
		l.timerCallbacks.DeleteIdx(uint32(id))
	}
}
func (l *EventLoop) runJSCallback(ctx *quickjs.Context, id uint64, repeating bool) error {
	entry, ok := l.jsTimers[id]
	if !ok || entry.handle.isCancelled() || entry.generation != l.Generation() {
		if ok {
			l.cancelJSTimer(id)
		}
		return nil
	}
	callback := l.timerCallbacks.GetInt64(int64(id))
	if callback == nil || !callback.IsFunction() {
		if callback != nil {
			callback.Free()
		}
		l.cancelJSTimer(id)
		return errors.New("timer callback was released")
	}
	this := ctx.NewUndefined()
	result := callback.Execute(this)
	this.Free()
	callback.Free()
	if !repeating {
		delete(l.jsTimers, id)
		if id <= maxTimerID {
			l.timerCallbacks.DeleteIdx(uint32(id))
		}
	}
	if result == nil {
		return errors.New("timer callback execution returned nil")
	}
	defer result.Free()
	if result.IsException() {
		if err := ctx.Exception(); err != nil {
			return fmt.Errorf("JavaScript timer callback: %w", err)
		}
		return errors.New("JavaScript timer callback failed")
	}
	return nil
}

func jsDelay(args []*quickjs.Value, index int) time.Duration {
	if len(args) <= index {
		return 0
	}
	milliseconds := args[index].ToFloat64()
	if math.IsNaN(milliseconds) || milliseconds <= 0 {
		return 0
	}
	maxMilliseconds := float64(maxTimerDuration) / float64(time.Millisecond)
	if math.IsInf(milliseconds, 1) || milliseconds >= maxMilliseconds {
		return maxTimerDuration
	}
	return time.Duration(milliseconds * float64(time.Millisecond))
}
