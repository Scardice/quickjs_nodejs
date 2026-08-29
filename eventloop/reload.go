package eventloop

import (
	"errors"
	"fmt"

	"container/heap"

	quickjs "github.com/buke/quickjs-go"
)

func (l *EventLoop) initializeGeneration(generation uint64) error {
	l.rt = quickjs.NewRuntime(
		quickjs.WithOwnerGoroutineCheck(true),
		quickjs.WithStrictOSThread(true),
		quickjs.WithModuleImport(l.moduleImport),
	)
	if l.rt == nil {
		return errors.New("quickjs runtime initialization failed")
	}
	l.ctx = l.rt.NewContextWithOptions(quickjs.NoBootstrap())
	if l.ctx == nil {
		return errors.New("quickjs context initialization failed")
	}
	l.generation.Store(generation)
	if l.adapter == nil {
		l.adapter = &Context{loop: l, generation: generation}
	}
	l.adapter.raw = l.ctx
	l.adapter.generation = generation
	bindContextOwner(l.ctx, l)

	l.timerCallbacks = l.ctx.NewObject()
	if l.timerCallbacks == nil {
		return errors.New("quickjs timer callback registry initialization failed")
	}
	l.ownerInit = true
	l.jsTimers = make(map[uint64]*timerEntry)
	if err := l.installJSTimers(); err != nil {
		return err
	}
	if l.registry != nil {
		if err := l.registry.Register(l.ctx); err != nil {
			return err
		}
	}
	for _, installer := range l.globals {
		if err := installer(l.ctx); err != nil {
			return err
		}
	}
	l.ownerInit = true
	return nil
}

func (l *EventLoop) disposeGeneration() {
	l.tasks = nil
	for l.timers.Len() > 0 {
		entry := heap.Pop(&l.timers).(*timerEntry)
		if entry.handle != nil {
			entry.handle.cancelled.Store(true)
		}
	}
	for _, entry := range l.jsTimers {
		if entry != nil && entry.handle != nil {
			entry.handle.cancelled.Store(true)
		}
	}
	l.jsTimers = nil
	l.closeResources()
	if l.registry != nil && l.ctx != nil {
		l.registry.ClearContext(l.ctx)
	}
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
	if l.adapter != nil {
		l.adapter.raw = nil
	}
	l.ownerInit = false
}

// Reload replaces the QuickJS runtime and context on the same owner goroutine.
// Queued tasks, timers, module values, and registered host resources do not
// cross the generation boundary.
func (l *EventLoop) Reload() error {
	if l == nil {
		return ErrClosed
	}
	if l.isOwner() {
		return l.reloadOnOwner()
	}
	if l.closed.Load() {
		return ErrClosed
	}
	return l.call(command{kind: commandReload})
}

func (l *EventLoop) reloadOnOwner() error {
	if l.closed.Load() || l.state == stateClosed {
		return ErrClosed
	}
	previousState := l.state
	l.state = stateStopped
	l.disposeGeneration()
	generation := l.generation.Load() + 1
	if generation == 0 {
		generation = 1
	}
	if err := l.initializeGeneration(generation); err != nil {
		l.closed.Store(true)
		l.state = stateClosed
		l.disposeGeneration()
		return fmt.Errorf("event loop reload failed: %w", err)
	}
	l.closed.Store(false)
	l.state = previousState
	return nil
}
