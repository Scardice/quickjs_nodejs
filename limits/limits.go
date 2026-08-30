// Package limits defines opt-in resource limits shared by one QuickJS runtime.
package limits

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrFetchConcurrencyExceeded     = errors.New("fetch concurrency limit exceeded")
	ErrWebSocketConcurrencyExceeded = errors.New("websocket connection limit exceeded")
)

// Config configures one QuickJS runtime. Zero values leave the corresponding
// resource unlimited.
type Config struct {
	ExecuteTimeout           time.Duration
	MaxFetchConcurrent       int
	MaxFetchResponseBytes    int64
	MaxWebSocketConnections  int
	MaxWebSocketMessageBytes int64
	MaxFilesystemReadBytes   int64
	MaxFilesystemWriteBytes  int64
	MaxPBKDF2Iterations      int
	MaxPBKDF2OutputBytes     int
}

// Validate rejects limits that cannot represent an unlimited-or-positive
// resource policy.
func (c Config) Validate() error {
	if c.ExecuteTimeout < 0 {
		return errors.New("execute timeout must not be negative")
	}
	if c.MaxFetchConcurrent < 0 {
		return errors.New("fetch concurrency limit must not be negative")
	}
	if c.MaxFetchResponseBytes < 0 {
		return errors.New("fetch response byte limit must not be negative")
	}
	if c.MaxWebSocketConnections < 0 {
		return errors.New("websocket connection limit must not be negative")
	}
	if c.MaxWebSocketMessageBytes < 0 {
		return errors.New("websocket message byte limit must not be negative")
	}
	if c.MaxFilesystemReadBytes < 0 {
		return errors.New("filesystem read byte limit must not be negative")
	}
	if c.MaxFilesystemWriteBytes < 0 {
		return errors.New("filesystem write byte limit must not be negative")
	}
	if c.MaxPBKDF2Iterations < 0 {
		return errors.New("PBKDF2 iteration limit must not be negative")
	}
	if c.MaxPBKDF2OutputBytes < 0 {
		return errors.New("PBKDF2 output byte limit must not be negative")
	}
	return nil
}

// Runtime owns counters shared by every API surface in one QuickJS runtime.
type Runtime struct {
	config         Config
	fetchSlots     chan struct{}
	webSocketSlots chan struct{}
}

// NewRuntime validates and copies config before constructing shared counters.
func NewRuntime(config Config) (*Runtime, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	runtime := &Runtime{config: config}
	if config.MaxFetchConcurrent > 0 {
		runtime.fetchSlots = make(chan struct{}, config.MaxFetchConcurrent)
	}
	if config.MaxWebSocketConnections > 0 {
		runtime.webSocketSlots = make(chan struct{}, config.MaxWebSocketConnections)
	}
	return runtime, nil
}

// Config returns the immutable resource policy for this runtime.
func (r *Runtime) Config() Config {
	if r == nil {
		return Config{}
	}
	return r.config
}

// AcquireFetch reserves one fetch slot until its idempotent release is called.
func (r *Runtime) AcquireFetch() (func(), error) {
	if r == nil || r.fetchSlots == nil {
		return func() {}, nil
	}
	select {
	case r.fetchSlots <- struct{}{}:
		return releaseSlot(r.fetchSlots), nil
	default:
		return nil, ErrFetchConcurrencyExceeded
	}
}

// AcquireWebSocket reserves one connection slot until its idempotent release
// is called.
func (r *Runtime) AcquireWebSocket() (func(), error) {
	if r == nil || r.webSocketSlots == nil {
		return func() {}, nil
	}
	select {
	case r.webSocketSlots <- struct{}{}:
		return releaseSlot(r.webSocketSlots), nil
	default:
		return nil, ErrWebSocketConcurrencyExceeded
	}
}

func releaseSlot(slots chan struct{}) func() {
	var once sync.Once
	return func() {
		once.Do(func() { <-slots })
	}
}
