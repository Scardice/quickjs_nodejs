// Package messagechannel provides MessageChannel and MessagePort bindings for QuickJS.
package messagechannel

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Scardice/quickjs_nodejs/module"
	"github.com/Scardice/quickjs_nodejs/structuredclone"
	quickjs "github.com/buke/quickjs-go"
)

const (
	ModuleName = "messagechannel"
	hiddenKey  = "__quickjs_nodejs_messagechannel_exports"
)

const implementation = `(function () {
  const key = "__quickjs_nodejs_messagechannel_exports";
  if (globalThis[key]) return globalThis[key];

  const native = globalThis.__quickjs_nodejs_messagechannel_native;
  const ports = new Map();
  const constructionToken = {};
  let nextPortID = 0;

  function createEvent(port, entry) {
    return {
      type: "message",
      data: entry.data,
      ports: entry.ports,
      target: port,
      currentTarget: port
    };
  }

  function enqueue(port) {
    if (!port._closed && !port._detached) native.enqueue(port._id);
  }

  function dispatch(id) {
    const port = ports.get(String(id));
    if (!port || port._closed || port._detached || !port._started || port._queue.length === 0) return false;
    const event = createEvent(port, port._queue.shift());
    const listeners = port._listeners.slice();
    if (typeof port._onmessage === "function") listeners.push(port._onmessage);
    for (const listener of listeners) {
      try {
        listener.call(port, event);
      } catch (_) {
      }
    }
    if (port._queue.length > 0) enqueue(port);
    return true;
  }

  function isMessagePort(value) {
    return value instanceof MessagePort;
  }

  function canTransferMessagePort(port) {
    return isMessagePort(port) && !port._closed && !port._detached;
  }

  function transferMessagePort(port) {
    if (!canTransferMessagePort(port)) return null;
    const replacement = new MessagePort(constructionToken);
    replacement._peerID = port._peerID;
    replacement._queue = port._queue;
    replacement._started = port._started;
    const peer = ports.get(port._peerID);
    if (peer) peer._peerID = replacement._id;
    port._queue = [];
    port._detached = true;
    ports.delete(port._id);
    return replacement;
  }

  function postMessageTransfer(options) {
    if (Array.isArray(options)) return options;
    if (options !== null && typeof options === "object" && "transfer" in options) return options.transfer;
    return options;
  }

  class MessagePort {
    constructor(token) {
      if (token !== constructionToken) throw new TypeError("Illegal constructor");
      this._id = String(++nextPortID);
      this._peerID = "";
      this._queue = [];
      this._listeners = [];
      this._onmessage = null;
      this._onmessageerror = null;
      this._started = false;
      this._closed = false;
      this._detached = false;
      ports.set(this._id, this);
    }

    postMessage(value, transfer) {
      if (this._detached) throw new TypeError("MessagePort is detached");
      if (this._closed) return;
      const peer = ports.get(this._peerID);
      if (!peer || peer._closed || peer._detached) return;
      const clone = globalThis.__quickjs_nodejs_structuredclone_exports;
      if (!clone || typeof clone.cloneForMessaging !== "function") throw new TypeError("structuredClone is unavailable");
      peer._queue.push(clone.cloneForMessaging(value, postMessageTransfer(transfer)));
      enqueue(peer);
    }

    start() {
      if (this._detached || this._closed) return;
      this._started = true;
      if (this._queue.length > 0) enqueue(this);
    }

    close() {
      if (this._closed) return;
      this._closed = true;
      this._queue = [];
      ports.delete(this._id);
    }

    addEventListener(type, listener) {
      if (type === "message" && typeof listener === "function" && !this._listeners.includes(listener)) {
        this._listeners.push(listener);
      }
    }

    removeEventListener(type, listener) {
      if (type !== "message") return;
      this._listeners = this._listeners.filter(candidate => candidate !== listener);
    }

    get onmessage() {
      return this._onmessage;
    }

    set onmessage(listener) {
      this._onmessage = typeof listener === "function" ? listener : null;
      if (this._onmessage) this.start();
    }

    get onmessageerror() {
      return this._onmessageerror;
    }

    set onmessageerror(listener) {
      this._onmessageerror = typeof listener === "function" ? listener : null;
    }
  }

  class MessageChannel {
    constructor() {
      const port1 = new MessagePort(constructionToken);
      const port2 = new MessagePort(constructionToken);
      port1._peerID = port2._id;
      port2._peerID = port1._id;
      this.port1 = port1;
      this.port2 = port2;
    }
  }

  const exports = {MessageChannel, MessagePort, dispatch, isMessagePort, canTransferMessagePort, transferMessagePort};
  Object.defineProperty(globalThis, key, {value: exports, configurable: false, enumerable: false, writable: false});
  return exports;
})()`

func ensureExports(ctx *quickjs.Context) (*quickjs.Value, error) {
	if ctx == nil {
		return nil, errors.New("messagechannel: nil context")
	}
	if err := structuredclone.InstallGlobal(ctx); err != nil {
		return nil, fmt.Errorf("messagechannel: install structuredClone: %w", err)
	}
	global := ctx.Globals()
	cached := global.Get(hiddenKey)
	if cached != nil && cached.IsObject() {
		return cached, nil
	}
	if cached != nil {
		cached.Free()
	}

	nativeKey := "__quickjs_nodejs_messagechannel_native"
	native := ctx.NewObject()
	if native == nil {
		return nil, errors.New("messagechannel: create native bridge")
	}
	native.Set("enqueue", ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if len(args) == 0 || args[0] == nil {
			return ctx.NewBool(false)
		}
		id := args[0].ToString()
		scheduled := ctx.Schedule(func(inner *quickjs.Context) {
			exports := inner.Globals().Get(hiddenKey)
			if exports == nil {
				return
			}
			defer exports.Free()
			value := inner.NewString(id)
			if value == nil {
				return
			}
			defer value.Free()
			result := exports.Call("dispatch", value)
			if result != nil {
				result.Free()
			}
		})
		return ctx.NewBool(scheduled)
	}))
	global.Set(nativeKey, native)

	value := ctx.Eval(strings.Replace(implementation, "__quickjs_nodejs_messagechannel_native", nativeKey, 1))
	global.Delete(nativeKey)
	if value == nil {
		return nil, errors.New("messagechannel: initialization returned nil")
	}
	if value.IsException() {
		err := ctx.Exception()
		value.Free()
		if err == nil {
			err = errors.New("messagechannel: initialization failed")
		}
		return nil, err
	}
	return value, nil
}

func exportValue(ctx *quickjs.Context, name string) (*quickjs.Value, error) {
	exports, err := ensureExports(ctx)
	if err != nil {
		return nil, err
	}
	value := exports.Get(name)
	exports.Free()
	if value == nil {
		return nil, fmt.Errorf("messagechannel: export %q is unavailable", name)
	}
	return value, nil
}

// Module returns the ESM definition for MessageChannel and MessagePort.
func Module() module.Definition {
	return module.Definition{
		Name:    ModuleName,
		Aliases: []string{"node:" + ModuleName},
		Exports: []module.Export{
			{Name: "MessageChannel", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return exportValue(ctx, "MessageChannel")
			}}},
			{Name: "MessagePort", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return exportValue(ctx, "MessagePort")
			}}},
			{Name: "default", Spec: quickjs.FactorySpec{Factory: ensureExports}},
		},
	}
}

// InstallGlobal installs MessageChannel and MessagePort on globalThis.
func InstallGlobal(ctx *quickjs.Context) error {
	for _, name := range []string{"MessageChannel", "MessagePort"} {
		value, err := exportValue(ctx, name)
		if err != nil {
			return err
		}
		if !ctx.Globals().DefinePropertyValue(name, value, quickjs.PropConfigurable|quickjs.PropWritable) {
			value.Free()
			return fmt.Errorf("messagechannel: install global %s", name)
		}
		value.Free()
	}
	return nil
}
