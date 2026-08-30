// Package eventloop owns a QuickJS runtime, context, and scheduler on one OS thread.
package eventloop

import (
	"bytes"
	"container/heap"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Scardice/quickjs_nodejs/limits"
	"github.com/Scardice/quickjs_nodejs/module"
	quickjs "github.com/buke/quickjs-go"
)

var (
	// ErrClosed reports that the event loop has been closed permanently.
	ErrClosed = errors.New("event loop is closed")
	// ErrStopped reports that a synchronous task requires a running loop.
	ErrStopped = errors.New("event loop is stopped")
	// ErrAlreadyRunning reports an attempt to run a blocking task on a running loop.
	ErrAlreadyRunning = errors.New("event loop is already running")
	// ErrNilTask reports a nil task callback.
	ErrNilTask = errors.New("event loop task is nil")
	// ErrStaleGeneration reports a callback targeting an older reloaded context.
	ErrStaleGeneration = errors.New("event loop context generation is stale")
)

const contextJobPollInterval = 10 * time.Millisecond

// Task is executed on the event loop owner goroutine.
type Task func(ctx *quickjs.Context) error

// GlobalInstaller installs explicit globals in one QuickJS context.
type GlobalInstaller func(ctx *quickjs.Context) error

// Logger receives errors from background tasks and QuickJS pump failures.
type Logger interface {
	Debug(message string)
	Debugf(format string, args ...any)
	Info(message string)
	Infof(format string, args ...any)
	Warn(message string)
	Warnf(format string, args ...any)
	Error(message string)
	Errorf(format string, args ...any)
}

type discardLogger struct{}

func (discardLogger) Debug(string)          {}
func (discardLogger) Debugf(string, ...any) {}
func (discardLogger) Info(string)           {}
func (discardLogger) Infof(string, ...any)  {}
func (discardLogger) Warn(string)           {}
func (discardLogger) Warnf(string, ...any)  {}
func (discardLogger) Error(string)          {}
func (discardLogger) Errorf(string, ...any) {}

type config struct {
	registry       *module.Registry
	globals        []GlobalInstaller
	logger         Logger
	moduleImport   bool
	resourceLimits limits.Config
}

// Option configures a new EventLoop.
type Option func(*config)

// WithRegistry registers the supplied in-memory ESM modules in the new context.
func WithRegistry(registry *module.Registry) Option {
	return func(cfg *config) {
		cfg.registry = registry
	}
}

// WithModuleImport controls QuickJS's default file-backed ESM loader. Native
// modules registered with WithRegistry remain available when it is disabled.
func WithModuleImport(enabled bool) Option {
	return func(cfg *config) {
		cfg.moduleImport = enabled
	}
}

// WithGlobals installs the supplied globals after module registration.
func WithGlobals(installers ...GlobalInstaller) Option {
	return func(cfg *config) {
		for _, installer := range installers {
			if installer != nil {
				cfg.globals = append(cfg.globals, installer)
			}
		}
	}
}

// WithLogger sets the logger used for background failures.
func WithLogger(logger Logger) Option {
	return func(cfg *config) {
		if logger != nil {
			cfg.logger = logger
		}
	}
}

// WithResourceLimits configures opt-in CPU execution limits for the owner
// runtime. Zero values retain the existing unlimited behavior.
func WithResourceLimits(resourceLimits limits.Config) Option {
	return func(cfg *config) {
		cfg.resourceLimits = resourceLimits
	}
}

type loopState uint8

const (
	stateNew loopState = iota
	stateRunning
	stateStopped
	stateClosed
)

type commandKind uint8

const (
	commandSchedule commandKind = iota
	commandAddTimer
	commandStart
	commandStop
	commandDo
	commandRun
	commandContextTask
	commandDoContextTask
	commandRunContext
	commandRegisterResource
	commandReload
	commandClose
)

type command struct {
	kind        commandKind
	generation  uint64
	task        Task
	contextTask func(*Context) error
	timer       *timerEntry
	resource    Resource
	reply       chan error
}

// EventLoop owns all QuickJS operations and scheduler state.
type EventLoop struct {
	commands chan command
	wakeup   chan struct{}
	ready    chan error
	done     chan struct{}

	closeOnce sync.Once
	closeErr  error
	closed    atomic.Bool
	nextID    atomic.Uint64
	ownerGID  atomic.Uint64

	logger         Logger
	registry       *module.Registry
	globals        []GlobalInstaller
	moduleImport   bool
	resourceLimits limits.Config

	rt  *quickjs.Runtime
	ctx *quickjs.Context

	adapter    *Context
	resources  []Resource
	generation atomic.Uint64

	timerCallbacks *quickjs.Value
	jsTimers       map[uint64]*timerEntry

	state     loopState
	tasks     []Task
	timers    timerHeap
	sequence  uint64
	ownerInit bool
}

func New(opts ...Option) (*EventLoop, error) {
	cfg := config{logger: discardLogger{}, moduleImport: true}
	for _, option := range opts {
		if option != nil {
			option(&cfg)
		}
	}
	if err := cfg.resourceLimits.Validate(); err != nil {
		return nil, err
	}

	loop := &EventLoop{
		commands:       make(chan command, 256),
		wakeup:         make(chan struct{}, 1),
		ready:          make(chan error, 1),
		done:           make(chan struct{}),
		logger:         cfg.logger,
		registry:       cfg.registry,
		globals:        append([]GlobalInstaller(nil), cfg.globals...),
		moduleImport:   cfg.moduleImport,
		resourceLimits: cfg.resourceLimits,
	}
	go loop.owner()
	if err := <-loop.ready; err != nil {
		<-loop.done
		return nil, err
	}
	return loop, nil
}

func (l *EventLoop) owner() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(l.done)

	l.ownerGID.Store(currentGoroutineID())
	if err := l.initializeGeneration(1); err != nil {
		l.failInitialization(err)
		return
	}
	l.ready <- nil

	for l.state != stateClosed {
		if l.state != stateRunning {
			l.handleCommand(<-l.commands)
			continue
		}
		if !l.pumpOnce() {
			l.waitForWork()
		}
	}
}

func (l *EventLoop) failInitialization(err error) {
	l.closed.Store(true)
	l.state = stateClosed
	l.closeResources()
	if l.timerCallbacks != nil {
		l.timerCallbacks.Free()
		l.timerCallbacks = nil
	}
	if l.ctx != nil {
		unbindContextOwner(l.ctx)
		l.ctx.Close()
		l.ctx = nil
	}
	if l.rt != nil {
		l.rt.Close()
		l.rt = nil
	}
	l.ready <- fmt.Errorf("event loop initialization failed: %w", err)
}

// Run starts or resumes the loop, executes task, and drains until quiescent.
func (l *EventLoop) Run(task Task) error {
	if task == nil {
		return ErrNilTask
	}
	if l.isOwner() {
		if l.closed.Load() {
			return ErrClosed
		}
		if l.state == stateRunning {
			return ErrAlreadyRunning
		}
		l.state = stateRunning
		return l.runUntilIdle(task)
	}
	if l.closed.Load() {
		return ErrClosed
	}
	return l.call(command{kind: commandRun, task: task})
}

// Do executes task synchronously while the loop is running.
func (l *EventLoop) Do(task Task) error {
	if task == nil {
		return ErrNilTask
	}
	if l.isOwner() {
		if l.closed.Load() || l.state == stateClosed {
			return ErrClosed
		}
		if l.state != stateRunning {
			return ErrStopped
		}
		return runTask(l.ctx, task)
	}
	if l.closed.Load() {
		return ErrClosed
	}
	return l.call(command{kind: commandDo, task: task})
}

// Schedule queues task without touching QuickJS. It is safe from any goroutine.
func (l *EventLoop) Schedule(task Task) bool {
	if task == nil || l.closed.Load() {
		return false
	}
	return l.enqueue(command{kind: commandSchedule, generation: l.Generation(), task: task})
}

// SetTimeout queues a one-shot callback.
func (l *EventLoop) SetTimeout(task Task, delay time.Duration) *Timer {
	if task == nil || l.closed.Load() {
		return nil
	}
	if delay < 0 {
		delay = 0
	}
	handle := &timerHandle{loop: l, id: l.nextID.Add(1)}
	entry := &timerEntry{
		id:         handle.id,
		generation: l.Generation(),
		deadline:   time.Now().Add(delay),
		task:       task,
		kind:       timerTimeout,
		handle:     handle,
		sequence:   l.nextID.Add(1),
	}
	if !l.enqueue(command{kind: commandAddTimer, generation: entry.generation, timer: entry}) {
		handle.cancelled.Store(true)
		return nil
	}
	return &Timer{handle: handle}
}

// SetInterval queues a repeating callback.
func (l *EventLoop) SetInterval(task Task, interval time.Duration) *Interval {
	if task == nil || l.closed.Load() {
		return nil
	}
	if interval <= 0 {
		interval = time.Nanosecond
	}
	handle := &timerHandle{loop: l, id: l.nextID.Add(1)}
	entry := &timerEntry{
		id:         handle.id,
		generation: l.Generation(),
		deadline:   time.Now().Add(interval),
		interval:   interval,
		task:       task,
		kind:       timerInterval,
		handle:     handle,
		sequence:   l.nextID.Add(1),
	}
	if !l.enqueue(command{kind: commandAddTimer, generation: entry.generation, timer: entry}) {
		handle.cancelled.Store(true)
		return nil
	}
	return &Interval{handle: handle}
}

// SetImmediate queues a callback for the next scheduler turn.
func (l *EventLoop) SetImmediate(task Task) *Immediate {
	if task == nil || l.closed.Load() {
		return nil
	}
	handle := &timerHandle{loop: l, id: l.nextID.Add(1)}
	entry := &timerEntry{
		id:         handle.id,
		generation: l.Generation(),
		deadline:   time.Now(),
		task:       task,
		kind:       timerImmediate,
		handle:     handle,
		sequence:   l.nextID.Add(1),
	}
	if !l.enqueue(command{kind: commandAddTimer, generation: entry.generation, timer: entry}) {
		handle.cancelled.Store(true)
		return nil
	}
	return &Immediate{handle: handle}
}

// ClearTimeout cancels a one-shot timer.
func (l *EventLoop) ClearTimeout(timer *Timer) {
	if timer != nil {
		timer.Cancel()
	}
}

// ClearInterval cancels a repeating timer.
func (l *EventLoop) ClearInterval(interval *Interval) {
	if interval != nil {
		interval.Cancel()
	}
}

// ClearImmediate cancels a next-turn callback.
func (l *EventLoop) ClearImmediate(immediate *Immediate) {
	if immediate != nil {
		immediate.Cancel()
	}
}

// Start starts or resumes continuous pumping.
func (l *EventLoop) Start() error {
	if l.isOwner() {
		if l.closed.Load() || l.state == stateClosed {
			return ErrClosed
		}
		l.state = stateRunning
		return nil
	}
	if l.closed.Load() {
		return ErrClosed
	}
	return l.call(command{kind: commandStart})
}

// Stop pauses pumping while retaining queued tasks and timers.
func (l *EventLoop) Stop() error {
	if l.isOwner() {
		if l.closed.Load() || l.state == stateClosed {
			return ErrClosed
		}
		l.state = stateStopped
		return nil
	}
	if l.closed.Load() {
		return ErrClosed
	}
	return l.call(command{kind: commandStop})
}

// Close permanently stops the owner and releases QuickJS resources.
func (l *EventLoop) Close() error {
	if l.isOwner() {
		l.closeOnce.Do(func() {
			l.closeOnOwner()
		})
		return l.closeErr
	}
	l.closeOnce.Do(func() {
		if l.closed.Load() {
			l.closeErr = ErrClosed
			return
		}
		l.closeErr = l.call(command{kind: commandClose})
	})
	<-l.done
	return l.closeErr
}

func (l *EventLoop) call(cmd command) error {
	cmd.reply = make(chan error, 1)
	select {
	case <-l.done:
		return ErrClosed
	case l.commands <- cmd:
	}
	select {
	case err := <-cmd.reply:
		return err
	case <-l.done:
		return ErrClosed
	}
}

func (l *EventLoop) enqueue(cmd command) bool {
	select {
	case <-l.done:
		return false
	default:
	}
	select {
	case <-l.done:
		return false
	case l.commands <- cmd:
		return true
	default:
		return false
	}
}

func (l *EventLoop) wake() {
	select {
	case l.wakeup <- struct{}{}:
	default:
	}
}
func (l *EventLoop) handleCommand(cmd command) {
	var err error
	switch cmd.kind {
	case commandSchedule:
		if l.state != stateClosed && cmd.generation == l.Generation() && cmd.task != nil {
			l.tasks = append(l.tasks, cmd.task)
		}
	case commandAddTimer:
		if l.state == stateClosed || cmd.generation != l.Generation() || cmd.timer == nil || cmd.timer.generation != l.Generation() || cmd.timer.handle.isCancelled() {
			if cmd.timer != nil && cmd.timer.handle != nil {
				cmd.timer.handle.cancelled.Store(true)
			}
			break
		}
		heap.Push(&l.timers, cmd.timer)
	case commandStart:
		if l.state == stateClosed || l.closed.Load() {
			err = ErrClosed
		} else {
			l.state = stateRunning
		}
	case commandStop:
		if l.state == stateClosed || l.closed.Load() {
			err = ErrClosed
		} else {
			l.state = stateStopped
		}
	case commandDo:
		if l.state == stateClosed || l.closed.Load() {
			err = ErrClosed
		} else if l.state != stateRunning {
			err = ErrStopped
		} else {
			err = l.runTask(cmd.task)
		}
	case commandRun:
		if l.state == stateClosed || l.closed.Load() {
			err = ErrClosed
		} else if l.state == stateRunning {
			err = ErrAlreadyRunning
		} else {
			l.state = stateRunning
			err = l.runUntilIdle(cmd.task)
		}
	case commandContextTask:
		if l.state == stateClosed || l.closed.Load() {
			err = ErrClosed
		} else {
			err = l.runContextTask(cmd.contextTask)
		}
	case commandDoContextTask:
		if l.state == stateClosed || l.closed.Load() {
			err = ErrClosed
		} else if l.state != stateRunning {
			err = ErrStopped
		} else {
			err = l.runContextTask(cmd.contextTask)
		}
	case commandRunContext:
		if l.state == stateClosed || l.closed.Load() {
			err = ErrClosed
		} else {
			err = l.runContextOnOwner(cmd.contextTask)
		}
	case commandRegisterResource:
		if l.state == stateClosed || l.closed.Load() {
			err = ErrClosed
		} else if cmd.resource == nil {
			err = errors.New("event loop resource is nil")
		} else {
			l.resources = append(l.resources, cmd.resource)
		}
	case commandReload:
		err = l.reloadOnOwner()
	case commandClose:
		l.closeOnOwner()
	}
	if cmd.reply != nil {
		cmd.reply <- err
	}
}

func (l *EventLoop) runUntilIdle(task Task) error {
	if err := l.runTask(task); err != nil {
		l.state = stateStopped
		return err
	}
	for l.state == stateRunning {
		if !l.pumpOnce() {
			if !l.hasPendingWork() {
				l.state = stateStopped
				break
			}
			l.waitForWork()
		}
	}
	return nil
}

func (l *EventLoop) pumpOnce() bool {
	progressed := l.drainCommands()
	if l.state != stateRunning || l.ctx == nil {
		return progressed
	}

	if len(l.tasks) > 0 {
		task := l.tasks[0]
		l.tasks[0] = nil
		l.tasks = l.tasks[1:]
		if err := l.runTask(task); err != nil {
			l.logger.Errorf("event loop background task failed: %v", err)
		}
		progressed = true
	} else if l.fireDueTimer() {
		progressed = true
	}
	if l.state != stateRunning {
		return progressed
	}
	_ = l.withExecuteTimeout(func() error {
		l.ctx.ProcessJobs()
		if l.ctx.LoopOnce() == 0 {
			progressed = true
		}
		return nil
	})
	return progressed
}

func (l *EventLoop) drainCommands() bool {
	progressed := false
	for {
		select {
		case cmd := <-l.commands:
			l.handleCommand(cmd)
			progressed = true
		default:
			return progressed
		}
	}
}

func (l *EventLoop) fireDueTimer() bool {
	l.timers.discardCancelled()
	entry := l.timers.Peek()
	if entry == nil || entry.deadline.After(time.Now()) {
		return false
	}
	heap.Pop(&l.timers)
	if entry.generation != l.Generation() {
		entry.handle.cancelled.Store(true)
		return true
	}
	if entry.handle.isCancelled() {
		return true
	}
	if err := l.runTask(entry.task); err != nil {
		l.logger.Errorf("event loop timer failed: %v", err)
	}
	if entry.kind == timerInterval && !entry.handle.isCancelled() && l.state == stateRunning {
		entry.deadline = entry.deadline.Add(entry.interval)
		entry.sequence = l.nextID.Add(1)
		heap.Push(&l.timers, entry)
	}
	return true
}

func (l *EventLoop) hasPendingWork() bool {
	l.timers.discardCancelled()
	return len(l.tasks) > 0 || l.timers.Len() > 0
}

func (l *EventLoop) waitForWork() {
	l.timers.discardCancelled()
	wait := contextJobPollInterval
	if entry := l.timers.Peek(); entry != nil {
		delay := time.Until(entry.deadline)
		if delay < 0 {
			delay = 0
		}
		if delay < wait {
			wait = delay
		}
	}
	timer := time.NewTimer(wait)
	defer func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}()

	select {
	case cmd := <-l.commands:
		l.handleCommand(cmd)
	case <-l.wakeup:
	case <-timer.C:
	case <-l.done:
	}
}

func (l *EventLoop) closeOnOwner() {
	if l.state == stateClosed {
		return
	}
	l.state = stateClosed
	l.closed.Store(true)
	l.disposeGeneration()
}

func (l *EventLoop) runTask(task Task) error {
	return l.withExecuteTimeout(func() error {
		return runTask(l.ctx, task)
	})
}

func (l *EventLoop) runContextTask(task func(*Context) error) error {
	if task == nil {
		return ErrNilTask
	}
	return l.withExecuteTimeout(func() error {
		return task(l.adapter)
	})
}

func (l *EventLoop) withExecuteTimeout(run func() error) error {
	if run == nil {
		return nil
	}
	if l == nil || l.rt == nil || l.resourceLimits.ExecuteTimeout <= 0 {
		return run()
	}
	timeoutSeconds := uint64(l.resourceLimits.ExecuteTimeout / time.Second)
	if l.resourceLimits.ExecuteTimeout%time.Second != 0 {
		timeoutSeconds++
	}
	if timeoutSeconds == 0 {
		timeoutSeconds = 1
	}
	l.rt.SetExecuteTimeout(timeoutSeconds)
	defer l.rt.SetExecuteTimeout(0)
	return run()
}
func (l *EventLoop) isOwner() bool {
	return l.ownerInit && l.ownerGID.Load() != 0 && l.ownerGID.Load() == currentGoroutineID()
}

func runTask(ctx *quickjs.Context, task Task) (err error) {
	if task == nil {
		return ErrNilTask
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("event loop task panic: %v", recovered)
		}
	}()
	return task(ctx)
}

func currentGoroutineID() uint64 {
	var buffer [64]byte
	n := runtime.Stack(buffer[:], false)
	fields := bytes.Fields(buffer[:n])
	if len(fields) < 2 {
		return 0
	}
	id, err := strconv.ParseUint(string(fields[1]), 10, 64)
	if err != nil {
		return 0
	}
	return id
}
