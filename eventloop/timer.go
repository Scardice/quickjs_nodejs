package eventloop

import (
	"container/heap"
	"sync/atomic"
	"time"
)

type timerKind uint8

const (
	timerTimeout timerKind = iota
	timerInterval
	timerImmediate
)

type timerHandle struct {
	loop      *EventLoop
	id        uint64
	cancelled atomic.Bool
}

func (h *timerHandle) cancel() {
	if h == nil {
		return
	}
	if h.cancelled.Swap(true) && h.loop != nil {
		return
	}
	if h.loop != nil {
		h.loop.wake()
	}
}

func (h *timerHandle) isCancelled() bool {
	return h == nil || h.cancelled.Load()
}

// Timer is a cancellable one-shot timer handle.
type Timer struct {
	handle *timerHandle
}

// Cancel prevents a timer callback from running.
func (t *Timer) Cancel() {
	if t != nil {
		t.handle.cancel()
	}
}

// Canceled reports whether the timer has been cancelled.
func (t *Timer) Canceled() bool {
	return t == nil || t.handle.isCancelled()
}

// Interval is a cancellable repeating timer handle.
type Interval struct {
	handle *timerHandle
}

// Cancel prevents future interval callbacks from running.
func (i *Interval) Cancel() {
	if i != nil {
		i.handle.cancel()
	}
}

// Canceled reports whether the interval has been cancelled.
func (i *Interval) Canceled() bool {
	return i == nil || i.handle.isCancelled()
}

// Immediate is a cancellable next-turn callback handle.
type Immediate struct {
	handle *timerHandle
}

// Cancel prevents an immediate callback from running.
func (i *Immediate) Cancel() {
	if i != nil {
		i.handle.cancel()
	}
}

// Canceled reports whether the immediate callback has been cancelled.
func (i *Immediate) Canceled() bool {
	return i == nil || i.handle.isCancelled()
}

type timerEntry struct {
	id         uint64
	generation uint64
	deadline   time.Time
	interval   time.Duration
	task       Task
	kind       timerKind
	handle     *timerHandle
	sequence   uint64
	index      int
}

type timerHeap []*timerEntry

func (h timerHeap) Len() int { return len(h) }

func (h timerHeap) Less(i, j int) bool {
	if h[i].deadline.Equal(h[j].deadline) {
		return h[i].sequence < h[j].sequence
	}
	return h[i].deadline.Before(h[j].deadline)
}

func (h timerHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *timerHeap) Push(value any) {
	entry := value.(*timerEntry)
	entry.index = len(*h)
	*h = append(*h, entry)
}

func (h *timerHeap) Pop() any {
	old := *h
	n := len(old)
	entry := old[n-1]
	old[n-1] = nil
	entry.index = -1
	*h = old[:n-1]
	return entry
}

func (h *timerHeap) discardCancelled() {
	for h.Len() > 0 && h.Peek().handle.isCancelled() {
		heap.Pop(h)
	}
}

func (h timerHeap) Peek() *timerEntry {
	if len(h) == 0 {
		return nil
	}
	return h[0]
}
