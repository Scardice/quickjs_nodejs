// Package abort provides AbortController and AbortSignal for QuickJS.
package abort

import (
	"errors"
	"fmt"

	"github.com/Scardice/quickjs_nodejs/module"
	quickjs "github.com/buke/quickjs-go"
)

const (
	ModuleName = "abort"
	hiddenKey  = "__quickjs_nodejs_abort_exports"
)

const implementation = `(function () {
  const key = "__quickjs_nodejs_abort_exports";
  if (globalThis[key]) return globalThis[key];

  function makeReason(message, name) {
    let error;
    if (typeof DOMException === "function") {
      error = new DOMException(message, name);
    } else {
      error = new Error(message);
      error.name = name;
    }
    return error;
  }

  class AbortSignal {
    constructor() {
      this._aborted = false;
      this._reason = undefined;
      this._listeners = [];
      this.onabort = null;
    }
    get aborted() { return this._aborted; }
    get reason() { return this._reason; }
    addEventListener(type, listener, options) {
      if (type !== "abort" || typeof listener !== "function") return;
      if (this._aborted) {
        listener.call(this, { type: "abort", target: this, currentTarget: this, reason: this._reason });
        return;
      }
      const once = !!(options && typeof options === "object" && options.once);
      this._listeners.push({ listener, once });
    }
    removeEventListener(type, listener) {
      if (type !== "abort") return;
      this._listeners = this._listeners.filter(entry => entry.listener !== listener);
    }
    throwIfAborted() {
      if (this._aborted) throw this._reason;
    }
    _abort(reason) {
      if (this._aborted) return;
      this._aborted = true;
      this._reason = reason === undefined ? makeReason("This operation was aborted", "AbortError") : reason;
      const event = { type: "abort", target: this, currentTarget: this, reason: this._reason };
      const listeners = this._listeners.slice();
      this._listeners = [];
      for (const entry of listeners) {
        if (!entry.once) this._listeners.push(entry);
        entry.listener.call(this, event);
      }
      if (typeof this.onabort === "function") this.onabort.call(this, event);
    }
    static abort(reason) {
      const signal = new AbortSignal();
      signal._abort(reason);
      return signal;
    }
    static timeout(milliseconds) {
      const signal = new AbortSignal();
      const delay = Number(milliseconds);
      setTimeout(() => signal._abort(makeReason("The operation timed out", "TimeoutError")),
        Number.isFinite(delay) && delay > 0 ? delay : 0);
      return signal;
    }
    static any(signals) {
      const combined = new AbortSignal();
      for (const signal of signals) {
        if (!signal || typeof signal.addEventListener !== "function") continue;
        if (signal.aborted) {
          combined._abort(signal.reason);
          break;
        }
        signal.addEventListener("abort", event => combined._abort(event.reason), { once: true });
      }
      return combined;
    }
  }

  class AbortController {
    constructor() { this.signal = new AbortSignal(); }
    abort(reason) { this.signal._abort(reason); }
  }

  const exports = { AbortController, AbortSignal };
  Object.defineProperty(globalThis, key, { value: exports, configurable: false, enumerable: false, writable: false });
  return exports;
})()`

func ensureExports(ctx *quickjs.Context) (*quickjs.Value, error) {
	if ctx == nil {
		return nil, errors.New("abort: nil context")
	}
	global := ctx.Globals()
	cached := global.Get(hiddenKey)
	if cached != nil && cached.IsObject() {
		return cached, nil
	}
	if cached != nil {
		cached.Free()
	}
	value := ctx.Eval(implementation)
	if value == nil {
		return nil, errors.New("abort: initialization returned nil")
	}
	if value.IsException() {
		err := ctx.Exception()
		value.Free()
		if err == nil {
			err = errors.New("abort: initialization failed")
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
		return nil, fmt.Errorf("abort: export %q is unavailable", name)
	}
	return value, nil
}

// Module returns the ESM definition for AbortController and AbortSignal.
func Module() module.Definition {
	return module.Definition{
		Name:    ModuleName,
		Aliases: []string{"node:" + ModuleName, "@seal/abort"},
		Exports: []module.Export{
			{Name: "AbortController", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return exportValue(ctx, "AbortController")
			}}},
			{Name: "AbortSignal", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return exportValue(ctx, "AbortSignal")
			}}},
			{Name: "default", Spec: quickjs.FactorySpec{Factory: ensureExports}},
		},
	}
}

// InstallGlobal installs AbortController and AbortSignal in one Context.
func InstallGlobal(ctx *quickjs.Context) error {
	exports, err := ensureExports(ctx)
	if err != nil {
		return err
	}
	for _, name := range []string{"AbortController", "AbortSignal"} {
		value := exports.Get(name)
		if value == nil {
			exports.Free()
			return fmt.Errorf("abort: export %q is unavailable", name)
		}
		if !ctx.Globals().DefinePropertyValue(name, value, quickjs.PropConfigurable|quickjs.PropWritable) {
			value.Free()
			exports.Free()
			return fmt.Errorf("abort: install global %q", name)
		}
		value.Free()
	}
	exports.Free()
	return nil
}
