package fetch

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/Scardice/quickjs_nodejs/eventloop"
	quickjs "github.com/buke/quickjs-go"
)

type fetchRuntime struct {
	config Config
	closed atomic.Bool
	nextID atomic.Uint64

	mu       sync.Mutex
	requests map[string]fetchRequest
}

type fetchRequest struct {
	cancel  context.CancelFunc
	release func()
}

func newFetchRuntime(_ *quickjs.Context, config Config) *fetchRuntime {
	return &fetchRuntime{
		config:   config,
		requests: make(map[string]fetchRequest),
	}
}

func (r *fetchRuntime) register(cancel context.CancelFunc, release func()) (string, error) {
	if r == nil || cancel == nil {
		return "", errors.New("fetch: runtime is closed")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed.Load() {
		return "", errors.New("fetch: runtime is closed")
	}
	id := "fetch-" + formatID(r.nextID.Add(1))
	r.requests[id] = fetchRequest{cancel: cancel, release: release}
	return id, nil
}

func (r *fetchRuntime) cancel(id string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	request := r.requests[id]
	r.mu.Unlock()
	if request.cancel != nil {
		request.cancel()
	}
}

func (r *fetchRuntime) complete(id string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	request, ok := r.requests[id]
	if ok {
		delete(r.requests, id)
	}
	r.mu.Unlock()
	if ok && request.release != nil {
		request.release()
	}
}

func (r *fetchRuntime) Close() error {
	if r == nil || r.closed.Swap(true) {
		return nil
	}
	r.mu.Lock()
	requests := make([]fetchRequest, 0, len(r.requests))
	for id, request := range r.requests {
		delete(r.requests, id)
		requests = append(requests, request)
	}
	r.mu.Unlock()
	for _, request := range requests {
		if request.cancel != nil {
			request.cancel()
		}
		if request.release != nil {
			request.release()
		}
	}
	return nil
}

func formatID(id uint64) string {
	const digits = "0123456789abcdefghijklmnopqrstuvwxyz"
	if id == 0 {
		return "0"
	}
	var buffer [13]byte
	index := len(buffer)
	for id > 0 {
		index--
		buffer[index] = digits[id%36]
		id /= 36
	}
	return string(buffer[index:])
}

var _ eventloop.Resource = (*fetchRuntime)(nil)
