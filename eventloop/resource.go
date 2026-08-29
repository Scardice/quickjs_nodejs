package eventloop

import (
	"sync"

	quickjs "github.com/buke/quickjs-go"
)

// Resource is closed on the EventLoop owner before its QuickJS context closes.
type Resource interface {
	Close() error
}

var contextOwners sync.Map

func bindContextOwner(ctx *quickjs.Context, loop *EventLoop) {
	if ctx != nil && loop != nil {
		contextOwners.Store(ctx, loop)
	}
}

func unbindContextOwner(ctx *quickjs.Context) {
	if ctx != nil {
		contextOwners.Delete(ctx)
	}
}

// RegisterContextResource associates a host resource with the EventLoop that
// owns ctx. It is useful to module installers that only receive *quickjs.Context.
func RegisterContextResource(ctx *quickjs.Context, resource Resource) bool {
	if ctx == nil || resource == nil {
		return false
	}
	owner, ok := contextOwners.Load(ctx)
	if !ok {
		return false
	}
	return owner.(*EventLoop).RegisterResource(resource)
}

// RegisterResource arranges for a resource to close before the runtime closes.
func (l *EventLoop) RegisterResource(resource Resource) bool {
	if l == nil || resource == nil || l.closed.Load() {
		return false
	}
	if l.isOwner() {
		if l.state == stateClosed {
			return false
		}
		l.resources = append(l.resources, resource)
		return true
	}
	return l.call(command{kind: commandRegisterResource, resource: resource}) == nil
}

func (l *EventLoop) closeResources() {
	for index := len(l.resources) - 1; index >= 0; index-- {
		if err := l.resources[index].Close(); err != nil {
			l.logger.Errorf("event loop resource close failed: %v", err)
		}
	}
	l.resources = nil
}
