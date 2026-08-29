package eventloop

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	quickjs "github.com/buke/quickjs-go"
)

type recordingLogger struct {
	mu      sync.Mutex
	errors  []string
	entries chan struct{}
}

func (l *recordingLogger) Debug(string)          {}
func (l *recordingLogger) Debugf(string, ...any) {}
func (l *recordingLogger) Info(string)           {}
func (l *recordingLogger) Infof(string, ...any)  {}
func (l *recordingLogger) Warn(string)           {}
func (l *recordingLogger) Warnf(string, ...any)  {}
func (l *recordingLogger) Error(message string) {
	l.mu.Lock()
	l.errors = append(l.errors, message)
	l.mu.Unlock()
	select {
	case l.entries <- struct{}{}:
	default:
	}
}
func (l *recordingLogger) Errorf(format string, args ...any) {
	l.Error(fmt.Sprintf(format, args...))
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if condition() {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("condition did not become true before timeout")
		case <-ticker.C:
		}
	}
}

func TestEventLoopScheduleOrder(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	var mu sync.Mutex
	got := make([]int, 0, 3)
	for _, value := range []int{1, 2, 3} {
		value := value
		if !loop.Schedule(func(*quickjs.Context) error {
			mu.Lock()
			got = append(got, value)
			mu.Unlock()
			return nil
		}) {
			t.Fatalf("schedule %d rejected", value)
		}
	}
	if err := loop.Start(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 500*time.Millisecond, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) == 3
	})
	mu.Lock()
	defer mu.Unlock()
	want := []int{1, 2, 3}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("schedule order = %v", got)
		}
	}
}

func TestEventLoopTimersAndClear(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()
	if err := loop.Start(); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	intervalCalls := 0
	timeoutCalls := 0
	var interval *Interval
	interval = loop.SetInterval(func(*quickjs.Context) error {
		mu.Lock()
		intervalCalls++
		calls := intervalCalls
		mu.Unlock()
		if calls == 3 {
			loop.ClearInterval(interval)
		}
		return nil
	}, 5*time.Millisecond)
	if interval == nil {
		t.Fatal("SetInterval returned nil")
	}
	if loop.SetTimeout(func(*quickjs.Context) error {
		mu.Lock()
		timeoutCalls++
		mu.Unlock()
		return nil
	}, 10*time.Millisecond) == nil {
		t.Fatal("SetTimeout returned nil")
	}

	waitFor(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return intervalCalls == 3 && timeoutCalls == 1
	})
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if intervalCalls != 3 || timeoutCalls != 1 {
		t.Fatalf("timer counts = interval %d timeout %d", intervalCalls, timeoutCalls)
	}
}

func TestEventLoopRunDrainsOneShotTimer(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	calls := 0
	if err := loop.Run(func(*quickjs.Context) error {
		loop.SetTimeout(func(*quickjs.Context) error {
			calls++
			return nil
		}, time.Millisecond)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("one-shot timer calls = %d", calls)
	}
}

func TestEventLoopStopRetainsQueuedTask(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()
	if err := loop.Start(); err != nil {
		t.Fatal(err)
	}
	if err := loop.Stop(); err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	if !loop.Schedule(func(*quickjs.Context) error {
		calls.Add(1)
		return nil
	}) {
		t.Fatal("schedule after stop was rejected")
	}
	time.Sleep(25 * time.Millisecond)
	if got := calls.Load(); got != 0 {
		t.Fatalf("stopped loop executed queued task: %d", got)
	}
	if err := loop.Start(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 500*time.Millisecond, func() bool { return calls.Load() == 1 })
}

func TestEventLoopCloseRejectsWork(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	if loop.SetTimeout(func(*quickjs.Context) error {
		calls++
		return nil
	}, 5*time.Millisecond) == nil {
		t.Fatal("SetTimeout returned nil before close")
	}
	if err := loop.Close(); err != nil {
		t.Fatal(err)
	}
	if loop.Schedule(func(*quickjs.Context) error { return nil }) {
		t.Fatal("Schedule accepted work after close")
	}
	if loop.SetTimeout(func(*quickjs.Context) error { return nil }, time.Millisecond) != nil {
		t.Fatal("SetTimeout accepted work after close")
	}
	if !errors.Is(loop.Start(), ErrClosed) {
		t.Fatal("Start did not return ErrClosed")
	}
	if calls != 0 {
		t.Fatalf("closed loop ran timer: %d", calls)
	}
}

func TestEventLoopRecoversBackgroundPanic(t *testing.T) {
	logger := &recordingLogger{entries: make(chan struct{}, 1)}
	loop, err := New(WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()
	if err := loop.Start(); err != nil {
		t.Fatal(err)
	}
	if !loop.Schedule(func(*quickjs.Context) error {
		panic("boom")
	}) {
		t.Fatal("panic task was rejected")
	}
	select {
	case <-logger.entries:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("background panic was not logged")
	}

	var calls atomic.Int32
	if !loop.Schedule(func(*quickjs.Context) error {
		calls.Add(1)
		return nil
	}) {
		t.Fatal("second task was rejected")
	}
	waitFor(t, 500*time.Millisecond, func() bool { return calls.Load() == 1 })
}

func TestEventLoopPromiseJobs(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()
	if err := loop.Start(); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	if !loop.Schedule(func(ctx *quickjs.Context) error {
		value := ctx.Eval(`Promise.resolve(1).then(() => { globalThis.done = 2 })`)
		if value == nil {
			return errors.New("promise evaluation returned nil")
		}
		value.Free()
		close(done)
		return nil
	}) {
		t.Fatal("promise task was rejected")
	}
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("promise task did not run")
	}
	if err := loop.Do(func(ctx *quickjs.Context) error {
		value := ctx.Eval(`globalThis.done`)
		if value == nil {
			return errors.New("done evaluation returned nil")
		}
		defer value.Free()
		if value.IsException() || value.ToInt32() != 2 {
			return errors.New("promise job did not update done")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
func TestEventLoopCloseDropsPendingWork(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	var timeoutCalls, intervalCalls, immediateCalls, scheduledCalls, promiseCalls int
	if loop.SetTimeout(func(*quickjs.Context) error {
		timeoutCalls++
		return nil
	}, time.Millisecond) == nil {
		t.Fatal("SetTimeout returned nil before close")
	}
	if loop.SetInterval(func(*quickjs.Context) error {
		intervalCalls++
		return nil
	}, time.Millisecond) == nil {
		t.Fatal("SetInterval returned nil before close")
	}
	if loop.SetImmediate(func(*quickjs.Context) error {
		immediateCalls++
		return nil
	}) == nil {
		t.Fatal("SetImmediate returned nil before close")
	}

	scheduled := make(chan bool, 1)
	go func() {
		scheduled <- loop.Schedule(func(*quickjs.Context) error {
			scheduledCalls++
			return nil
		})
	}()
	if !<-scheduled {
		t.Fatal("cross-goroutine Schedule was rejected before close")
	}

	if err := loop.Run(func(ctx *quickjs.Context) error {
		record := ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, _ []*quickjs.Value) *quickjs.Value {
			promiseCalls++
			return ctx.NewUndefined()
		})
		ctx.Globals().Set("record", record)
		promise := ctx.Eval(`Promise.resolve().then(() => record()); setTimeout(record, 0); setInterval(record, 1); setImmediate(record)`)
		if promise == nil {
			return errors.New("pending Promise evaluation returned nil")
		}
		promise.Free()
		return loop.Stop()
	}); err != nil {
		t.Fatal(err)
	}

	if err := loop.Close(); err != nil {
		t.Fatal(err)
	}
	if loop.Schedule(func(*quickjs.Context) error { return nil }) {
		t.Fatal("Schedule accepted work after close")
	}
	if loop.SetTimeout(func(*quickjs.Context) error { return nil }, time.Millisecond) != nil {
		t.Fatal("SetTimeout accepted work after close")
	}
	if !errors.Is(loop.Do(func(*quickjs.Context) error { return nil }), ErrClosed) {
		t.Fatal("Do did not return ErrClosed after close")
	}
	if timeoutCalls != 0 || intervalCalls != 0 || immediateCalls != 0 || scheduledCalls != 0 || promiseCalls != 0 {
		t.Fatalf("callbacks ran after close: timeout=%d interval=%d immediate=%d scheduled=%d promise=%d",
			timeoutCalls, intervalCalls, immediateCalls, scheduledCalls, promiseCalls)
	}
}
func TestEventLoopCloseFromOwnerDoesNotDeadlock(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.Run(func(*quickjs.Context) error {
		return loop.Close()
	}); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(loop.Start(), ErrClosed) {
		t.Fatal("Start did not return ErrClosed after owner Close")
	}
}
