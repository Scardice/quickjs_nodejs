package websocket

import (
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"

	"github.com/Scardice/quickjs_nodejs/eventloop"
	"github.com/Scardice/quickjs_nodejs/module"
	quickjs "github.com/buke/quickjs-go"
)

const ModuleName = "websocket"

var apiSequence atomic.Uint64

type apiBuilder struct {
	config Config
	apiKey string
}

func Module(options ...Option) module.Definition {
	builder := &apiBuilder{
		config: applyOptions(options),
		apiKey: nextAPIKey(),
	}
	return module.Definition{
		Name:    ModuleName,
		Aliases: []string{"node:websocket"},
		Exports: []module.Export{
			{Name: "WebSocket", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return builder.exportValue(ctx, "WebSocket")
			}}},
			{Name: "CONNECTING", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return ctx.NewInt32(connectingState), nil
			}}},
			{Name: "OPEN", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return ctx.NewInt32(openState), nil
			}}},
			{Name: "CLOSING", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return ctx.NewInt32(closingState), nil
			}}},
			{Name: "CLOSED", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return ctx.NewInt32(closedState), nil
			}}},
			{Name: "default", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return builder.exportValue(ctx, "default")
			}}},
		},
	}
}

func InstallGlobal(ctx *quickjs.Context, options ...Option) error {
	if ctx == nil {
		return errors.New("websocket: nil context")
	}
	builder := &apiBuilder{config: applyOptions(options), apiKey: nextAPIKey()}
	api, err := builder.build(ctx)
	if err != nil {
		return err
	}
	defer api.Free()
	for _, name := range []string{"WebSocket", "CONNECTING", "OPEN", "CLOSING", "CLOSED"} {
		value := api.Get(name)
		if value == nil {
			return fmt.Errorf("websocket: missing global %s", name)
		}
		ctx.Globals().Set(name, value)
	}
	return nil
}

func nextAPIKey() string {
	return fmt.Sprintf("__quickjs_nodejs_websocket_api_%d", apiSequence.Add(1))
}

func (builder *apiBuilder) exportValue(ctx *quickjs.Context, name string) (*quickjs.Value, error) {
	api, err := builder.build(ctx)
	if err != nil {
		return nil, err
	}
	value := api.Get(name)
	api.Free()
	if value == nil {
		return nil, fmt.Errorf("websocket: export %q is unavailable", name)
	}
	return value, nil
}

func (builder *apiBuilder) build(ctx *quickjs.Context) (*quickjs.Value, error) {
	global := ctx.Globals()
	cached := global.Get(builder.apiKey)
	if cached != nil && cached.IsObject() {
		return cached, nil
	}
	if cached != nil {
		cached.Free()
	}

	runtime := newRuntime(ctx, builder.config, builder.apiKey)
	eventloop.RegisterContextResource(ctx, runtime)
	native := ctx.NewObject()
	if native == nil {
		return nil, errors.New("websocket: create native object")
	}
	native.Set("open", ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if len(args) < 1 || args[0] == nil {
			return ctx.ThrowTypeError("websocket URL is required")
		}
		protocols, err := readProtocols(args[1:])
		if err != nil {
			return ctx.ThrowTypeError("%s", err)
		}
		return ctx.NewString(runtime.open(args[0].ToString(), protocols))
	}))
	native.Set("send", ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if len(args) < 2 || args[0] == nil {
			return ctx.ThrowTypeError("websocket connection is required")
		}
		data, err := websocketBody(ctx, args[1])
		if err != nil {
			return ctx.ThrowTypeError("%s", err)
		}
		messageType := textMessage
		if args[1] != nil && !args[1].IsString() {
			messageType = binaryMessage
		}
		if err := runtime.send(args[0].ToString(), data, messageType); err != nil {
			return ctx.ThrowError(err)
		}
		return ctx.NewUndefined()
	}))
	native.Set("close", ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if len(args) < 1 || args[0] == nil {
			return ctx.NewUndefined()
		}
		code := 1000
		if len(args) > 1 && args[1] != nil && !args[1].IsUndefined() {
			code = int(args[1].ToInt32())
			if (code < 1000 || code > 4999) && code != 1000 {
				return ctx.ThrowRangeError("invalid websocket close code")
			}
		}
		reason := ""
		if len(args) > 2 && args[2] != nil && !args[2].IsUndefined() {
			reason = args[2].ToString()
		}
		runtime.close(args[0].ToString(), code, reason)
		return ctx.NewUndefined()
	}))
	global.Set(builder.apiKey, native)

	code := fmt.Sprintf("(function(native) {%s})(globalThis[%s])", websocketImplementation, strconv.Quote(builder.apiKey))
	result := ctx.Eval(code)
	if result == nil {
		global.Delete(builder.apiKey)
		return nil, errors.New("websocket: initialization returned nil")
	}
	if result.IsException() {
		err := ctx.Exception()
		result.Free()
		global.Delete(builder.apiKey)
		if err == nil {
			err = errors.New("websocket: initialization failed")
		}
		return nil, err
	}
	global.Set(builder.apiKey, result)
	return global.Get(builder.apiKey), nil
}

func readProtocols(values []*quickjs.Value) ([]string, error) {
	if len(values) == 0 || values[0] == nil || values[0].IsUndefined() || values[0].IsNull() {
		return nil, nil
	}
	value := values[0]
	if value.IsString() {
		return []string{value.ToString()}, nil
	}
	if !value.IsArray() {
		return nil, errors.New("websocket protocols must be a string or array")
	}
	protocols := make([]string, value.Len())
	for index := range protocols {
		item := value.GetIdx(int64(index))
		if item == nil {
			return nil, errors.New("websocket protocol is unavailable")
		}
		if !item.IsString() {
			item.Free()
			return nil, errors.New("websocket protocol must be a string")
		}
		protocols[index] = item.ToString()
		item.Free()
	}
	return protocols, nil
}

const websocketImplementation = `
  const sockets = new Map();
  const listeners = new WeakMap();
  function emit(socket, type, event) {
    const handler = socket["on" + type];
    if (typeof handler === "function") {
      try { handler.call(socket, event); } catch (_) {}
    }
    const registered = listeners.get(socket);
    if (!registered || !registered[type]) return;
    for (const listener of registered[type].slice()) {
      try { listener.call(socket, event); } catch (_) {}
    }
  }
  function dispatch(id, type, payload, code, reason) {
    const socket = sockets.get(id);
    if (!socket) return;
    if (type === "open") {
      socket.readyState = 1;
      socket.protocol = String(payload || "");
      emit(socket, "open", {type: "open", target: socket});
      return;
    }
    if (type === "message") {
      let data = payload;
      if (typeof payload !== "string") {
        const bytes = payload instanceof Uint8Array ? payload : new Uint8Array(payload);
        data = socket.binaryType === "arraybuffer"
          ? bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength)
          : bytes;
      }
      emit(socket, "message", {type: "message", data, target: socket});
      return;
    }
    if (type === "error") {
      const message = String(payload || "websocket error");
      emit(socket, "error", {type: "error", message, target: socket});
      return;
    }
    if (type === "close") {
      socket.readyState = 3;
      sockets.delete(id);
      emit(socket, "close", {
        type: "close",
        code: code || 1006,
        reason: String(reason || ""),
        wasClean: code === 1000 || code === 1001,
        target: socket
      });
    }
  }
  Object.defineProperty(native, "__dispatch", {value: dispatch});
  class WebSocket {
    constructor(url, protocols) {
      this.url = String(url);
      this.protocol = "";
      this.readyState = 0;
      this.bufferedAmount = 0;
      this.binaryType = "arraybuffer";
      this.onopen = null;
      this.onmessage = null;
      this.onerror = null;
      this.onclose = null;
      this._id = native.open(this.url, protocols);
      sockets.set(this._id, this);
      listeners.set(this, Object.create(null));
    }
    send(data) {
      if (this.readyState !== 1) throw new Error("WebSocket is not open");
      native.send(this._id, data);
    }
    close(code, reason) {
      if (this.readyState >= 2) return;
      this.readyState = 2;
      native.close(this._id, code, reason);
    }
    addEventListener(type, listener) {
      if (typeof listener !== "function") return;
      const registered = listeners.get(this);
      if (!registered[type]) registered[type] = [];
      if (!registered[type].includes(listener)) registered[type].push(listener);
    }
    removeEventListener(type, listener) {
      const registered = listeners.get(this);
      if (!registered || !registered[type]) return;
      registered[type] = registered[type].filter(item => item !== listener);
    }
  }
  const api = {WebSocket, CONNECTING: 0, OPEN: 1, CLOSING: 2, CLOSED: 3};
  Object.defineProperty(api, "__native", {value: native});
  api.default = {WebSocket, CONNECTING: 0, OPEN: 1, CLOSING: 2, CLOSED: 3};
  return api;
`
