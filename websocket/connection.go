package websocket

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Scardice/quickjs_nodejs/buffer"
	"github.com/Scardice/quickjs_nodejs/limits"
	quickjs "github.com/buke/quickjs-go"
	gorilla "github.com/gorilla/websocket"
)

const (
	connectingState = 0
	openState       = 1
	closingState    = 2
	closedState     = 3

	textMessage   = gorilla.TextMessage
	binaryMessage = gorilla.BinaryMessage
)

type Conn interface {
	ReadMessage() (messageType int, data []byte, err error)
	WriteMessage(messageType int, data []byte) error
	Close() error
}

type readLimitConn interface {
	SetReadLimit(int64)
}

type Dialer interface {
	DialContext(ctx context.Context, urlStr string, requestHeader http.Header) (Conn, *http.Response, error)
}

type DialerFunc func(context.Context, string, http.Header) (Conn, *http.Response, error)

func (dialer DialerFunc) DialContext(ctx context.Context, urlStr string, requestHeader http.Header) (Conn, *http.Response, error) {
	return dialer(ctx, urlStr, requestHeader)
}

type Policy func(*url.URL) error

type Config struct {
	Dialer         Dialer
	Headers        http.Header
	Policy         Policy
	ResourceLimits *limits.Runtime
}

type Option func(*Config)

func WithDialer(dialer Dialer) Option {
	return func(config *Config) { config.Dialer = dialer }
}

func WithHeaders(headers http.Header) Option {
	return func(config *Config) { config.Headers = cloneHeader(headers) }
}

func WithPolicy(policy Policy) Option {
	return func(config *Config) { config.Policy = policy }
}

// WithResourceLimits shares one runtime-local connection policy between
// WebSocket module exports and WebSocket globals.
func WithResourceLimits(resourceLimits *limits.Runtime) Option {
	return func(config *Config) { config.ResourceLimits = resourceLimits }
}

func applyOptions(options []Option) Config {
	config := Config{}
	for _, option := range options {
		if option != nil {
			option(&config)
		}
	}
	config.Headers = cloneHeader(config.Headers)
	return config
}

type runtime struct {
	ctx    *quickjs.Context
	config Config
	apiKey string
	nextID atomic.Uint64
	closed atomic.Bool

	mu          sync.Mutex
	connections map[string]*connection
}

type connection struct {
	runtime *runtime
	id      string
	ctx     context.Context
	cancel  context.CancelFunc
	release func()

	mu          sync.Mutex
	conn        Conn
	state       int
	closeCode   int
	closeReason string
	closeOnce   sync.Once
	writeMu     sync.Mutex
}

func newRuntime(ctx *quickjs.Context, config Config, apiKey string) *runtime {
	return &runtime{
		ctx:         ctx,
		config:      config,
		apiKey:      apiKey,
		connections: make(map[string]*connection),
	}
}

func (r *runtime) open(rawURL string, protocols []string) (string, error) {
	if r == nil {
		return "", errors.New("websocket: runtime is closed")
	}
	release, err := r.config.ResourceLimits.AcquireWebSocket()
	if err != nil {
		return "", err
	}
	id := fmt.Sprintf("ws-%d", r.nextID.Add(1))
	connectCtx, cancel := context.WithCancel(context.Background())
	connection := &connection{
		runtime: r,
		id:      id,
		ctx:     connectCtx,
		cancel:  cancel,
		release: release,
		state:   connectingState,
	}
	r.mu.Lock()
	if r.closed.Load() {
		r.mu.Unlock()
		cancel()
		release()
		return "", errors.New("websocket: runtime is closed")
	}
	r.connections[id] = connection
	r.mu.Unlock()
	go r.connect(connection, rawURL, protocols)
	return id, nil
}

func (r *runtime) connect(connection *connection, rawURL string, protocols []string) {
	if r.closed.Load() {
		r.finish(connection, 1006, "")
		return
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "ws" && parsed.Scheme != "wss") {
		r.fail(connection, fmt.Errorf("websocket: invalid URL %q", rawURL))
		return
	}
	if r.config.Policy != nil {
		if err := r.config.Policy(parsed); err != nil {
			r.fail(connection, err)
			return
		}
	}
	if r.config.Dialer == nil {
		r.fail(connection, errors.New("websocket: no dialer configured"))
		return
	}

	header := cloneHeader(r.config.Headers)
	if len(protocols) > 0 {
		header.Set("Sec-WebSocket-Protocol", strings.Join(protocols, ", "))
	}
	conn, response, err := r.config.Dialer.DialContext(connection.ctx, rawURL, header)
	if err != nil {
		if connection.isClosing() {
			code, reason := connection.closeDetails()
			r.finish(connection, code, reason)
		} else {
			r.fail(connection, err)
		}
		return
	}
	if conn == nil {
		r.fail(connection, errors.New("websocket: dialer returned nil connection"))
		return
	}
	if !connection.attach(conn) {
		_ = conn.Close()
		return
	}
	protocol := ""
	if response != nil {
		protocol = response.Header.Get("Sec-WebSocket-Protocol")
	}
	r.scheduleEvent(connection.id, "open", []byte(protocol), textMessage, 0, "")

	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			code, reason := connection.closeDetails()
			if closeErr, ok := err.(*gorilla.CloseError); ok {
				code, reason = closeErr.Code, closeErr.Text
			}
			r.finish(connection, code, reason)
			return
		}
		switch messageType {
		case textMessage:
			r.scheduleEvent(connection.id, "message", data, textMessage, 0, "")
		case binaryMessage:
			r.scheduleEvent(connection.id, "message", data, binaryMessage, 0, "")
		}
	}
}
func (r *runtime) send(id string, data []byte, messageType int) error {
	connection := r.lookup(id)
	if connection == nil {
		return errors.New("websocket: unknown connection")
	}
	connection.mu.Lock()
	if connection.state != openState || connection.conn == nil {
		connection.mu.Unlock()
		return errors.New("websocket: connection is not open")
	}
	conn := connection.conn
	connection.mu.Unlock()
	go func() {
		connection.writeMu.Lock()
		err := conn.WriteMessage(messageType, data)
		connection.writeMu.Unlock()
		if err != nil {
			r.fail(connection, err)
		}
	}()
	return nil
}

func (r *runtime) close(id string, code int, reason string) {
	connection := r.lookup(id)
	if connection == nil {
		return
	}
	connection.mu.Lock()
	if connection.state == closedState {
		connection.mu.Unlock()
		return
	}
	connection.state = closingState
	connection.closeCode = code
	connection.closeReason = reason
	conn := connection.conn
	connection.mu.Unlock()
	connection.cancel()
	if conn != nil {
		_ = conn.Close()
	}
	r.finish(connection, code, reason)
}

func (r *runtime) fail(connection *connection, err error) {
	if connection.isClosing() {
		code, reason := connection.closeDetails()
		r.finish(connection, code, reason)
		return
	}
	r.scheduleEvent(connection.id, "error", []byte(err.Error()), textMessage, 0, "")
	r.finish(connection, 1006, "")
}

func (r *runtime) finish(connection *connection, code int, reason string) {
	connection.closeOnce.Do(func() {
		connection.mu.Lock()
		connection.state = closedState
		connection.closeCode = code
		connection.closeReason = reason
		conn := connection.conn
		release := connection.release
		connection.mu.Unlock()
		connection.cancel()
		if conn != nil {
			_ = conn.Close()
		}
		r.mu.Lock()
		delete(r.connections, connection.id)
		r.mu.Unlock()
		if release != nil {
			release()
		}
		r.scheduleEvent(connection.id, "close", nil, textMessage, code, reason)
	})
}

func (r *runtime) lookup(id string) *connection {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.connections[id]
}

func (r *runtime) closeAll() {
	if r.closed.Swap(true) {
		return
	}
	r.mu.Lock()
	connections := make([]*connection, 0, len(r.connections))
	for _, connection := range r.connections {
		connections = append(connections, connection)
	}
	r.mu.Unlock()
	for _, connection := range connections {
		r.close(connection.id, 1001, "runtime closed")
	}
}

func (r *runtime) Close() error {
	r.closeAll()
	return nil
}

func (r *runtime) scheduleEvent(id, event string, data []byte, messageType, code int, reason string) {
	if r.closed.Load() {
		return
	}
	payload := append([]byte(nil), data...)
	if !r.ctx.Schedule(func(inner *quickjs.Context) {
		global := inner.Globals()
		api := global.Get(r.apiKey)
		if api == nil {
			return
		}
		native := api.Get("__native")
		if native == nil {
			api.Free()
			return
		}
		dispatch := native.Get("__dispatch")
		if dispatch == nil {
			native.Free()
			api.Free()
			return
		}
		idValue := inner.NewString(id)
		eventValue := inner.NewString(event)
		var payloadValue *quickjs.Value
		if messageType == textMessage {
			payloadValue = inner.NewString(string(payload))
		} else {
			payloadValue = inner.NewUint8Array(payload)
		}
		codeValue := inner.NewInt32(int32(code))
		reasonValue := inner.NewString(reason)
		result := dispatch.Execute(inner.Globals(), idValue, eventValue, payloadValue, codeValue, reasonValue)
		if result != nil {
			result.Free()
		}
		dispatch.Free()
		native.Free()
		api.Free()
	}) {
		return
	}
}

func (connection *connection) attach(conn Conn) bool {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	if connection.state == closedState || connection.state == closingState {
		return false
	}
	if limiter, ok := conn.(readLimitConn); ok {
		if maxBytes := connection.runtime.config.ResourceLimits.Config().MaxWebSocketMessageBytes; maxBytes > 0 {
			limiter.SetReadLimit(maxBytes)
		}
	}
	connection.conn = conn
	connection.state = openState
	return true
}

func (connection *connection) isClosing() bool {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	return connection.state == closingState || connection.state == closedState
}

func (connection *connection) closeDetails() (int, string) {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	code := connection.closeCode
	if code == 0 {
		code = 1006
	}
	return code, connection.closeReason
}

func cloneHeader(source http.Header) http.Header {
	if source == nil {
		return make(http.Header)
	}
	return source.Clone()
}

func websocketBody(ctx *quickjs.Context, value *quickjs.Value) ([]byte, error) {
	if value == nil || value.IsNull() || value.IsUndefined() {
		return nil, nil
	}
	if value.IsString() {
		return []byte(value.ToString()), nil
	}
	return buffer.Bytes(ctx, value)
}
