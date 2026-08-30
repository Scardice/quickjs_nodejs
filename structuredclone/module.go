// Package structuredclone provides a Web structuredClone implementation for QuickJS.
package structuredclone

import (
	"errors"
	"fmt"

	"github.com/Scardice/quickjs_nodejs/blob"
	"github.com/Scardice/quickjs_nodejs/module"
	quickjs "github.com/buke/quickjs-go"
)

const (
	ModuleName = "structuredclone"
	hiddenKey  = "__quickjs_nodejs_structuredclone_exports"
)

const implementation = `(function () {
  const key = "__quickjs_nodejs_structuredclone_exports";
  if (globalThis[key]) return globalThis[key];

  const Blob = globalThis.Blob;
  const File = globalThis.File;
  function cloneError(message) {
    if (typeof DOMException === "function") {
      return new DOMException(message || "The object could not be cloned.", "DataCloneError");
    }
    const error = new Error(message || "The object could not be cloned.");
    error.name = "DataCloneError";
    error.code = 25;
    return error;
  }

  function messagePortBridge() {
    const bridge = globalThis.__quickjs_nodejs_messagechannel_exports;
    if (!bridge || typeof bridge.isMessagePort !== "function") return null;
    return bridge;
  }

  function transferSet(options) {
    if (!options || options.transfer === undefined) return new Set();
    const transfer = options.transfer;
    if (transfer === null || typeof transfer[Symbol.iterator] !== "function") {
      throw new TypeError("transfer must be iterable");
    }
    const bridge = messagePortBridge();
    const values = new Set();
    for (const value of transfer) {
      if (values.has(value) || !bridge || !bridge.isMessagePort(value) || !bridge.canTransferMessagePort(value)) {
        throw cloneError();
      }
      values.add(value);
    }
    return values;
  }

  function cloneValue(value, seen, transfers) {
    if (value === null || (typeof value !== "object" && typeof value !== "function")) {
      if (typeof value === "symbol" || typeof value === "function") throw cloneError();
      return value;
    }
    if (typeof value === "function") throw cloneError();
    if (seen.has(value)) return seen.get(value);

    const bridge = messagePortBridge();
    if (bridge && bridge.isMessagePort(value)) {
      if (!transfers.has(value)) throw cloneError();
      const copy = bridge.transferMessagePort(value);
      if (copy === null) throw cloneError();
      seen.set(value, copy);
      return copy;
    }

    if (value instanceof Boolean) {
      const copy = new Boolean(value.valueOf());
      seen.set(value, copy);
      return copy;
    }
    if (value instanceof String) {
      const copy = new String(value.valueOf());
      seen.set(value, copy);
      return copy;
    }
    if (value instanceof Number) {
      const copy = new Number(value.valueOf());
      seen.set(value, copy);
      return copy;
    }
    if (typeof BigInt === "function" && value instanceof BigInt) {
      const copy = Object(value.valueOf());
      seen.set(value, copy);
      return copy;
    }

    if (typeof File === "function" && value instanceof File) {
      const copy = new File([value], value.name, { type: value.type, lastModified: value.lastModified });
      seen.set(value, copy);
      return copy;
    }
    if (typeof Blob === "function" && value instanceof Blob) {
      const copy = new Blob([value], { type: value.type });
      seen.set(value, copy);
      return copy;
    }
    if (typeof Response === "function" && value instanceof Response) {
      throw cloneError();
    }

    if (value instanceof WeakMap || value instanceof WeakSet || value instanceof Promise) {
      throw cloneError();
    }
    if (value instanceof Date) {
      const copy = new Date(value.getTime());
      seen.set(value, copy);
      return copy;
    }
    if (value instanceof RegExp) {
      const copy = new RegExp(value.source, value.flags);
      seen.set(value, copy);
      return copy;
    }
    if (value instanceof Error) {
      const hasMessage = Object.prototype.hasOwnProperty.call(value, "message");
      const copy = hasMessage ? new value.constructor(value.message) : new value.constructor();
      seen.set(value, copy);
      if (Object.prototype.hasOwnProperty.call(value, "cause")) {
        copy.cause = cloneValue(value.cause, seen, transfers);
      }
      return copy;
    }
    if (typeof ArrayBuffer === "function" && value instanceof ArrayBuffer) {
      const copy = value.slice(0);
      seen.set(value, copy);
      return copy;
    }
    if (typeof ArrayBuffer === "function" && typeof ArrayBuffer.isView === "function" && ArrayBuffer.isView(value)) {
      const buffer = cloneValue(value.buffer, seen, transfers);
      let copy;
      if (value instanceof DataView) {
        copy = new DataView(buffer, value.byteOffset, value.byteLength);
      } else {
        copy = new value.constructor(buffer, value.byteOffset, value.length);
      }
      seen.set(value, copy);
      return copy;
    }
    if (value instanceof Map) {
      const copy = new Map();
      seen.set(value, copy);
      for (const entry of value) copy.set(cloneValue(entry[0], seen, transfers), cloneValue(entry[1], seen, transfers));
      return copy;
    }
    if (value instanceof Set) {
      const copy = new Set();
      seen.set(value, copy);
      for (const entry of value) copy.add(cloneValue(entry, seen, transfers));
      return copy;
    }
    if (Array.isArray(value)) {
      const copy = new Array(value.length);
      seen.set(value, copy);
      for (const key of Reflect.ownKeys(value)) {
        if (key === "length") continue;
        const descriptor = Object.getOwnPropertyDescriptor(value, key);
        if (descriptor && descriptor.enumerable) copy[key] = cloneValue(value[key], seen, transfers);
      }
      return copy;
    }

    const copy = {};
    seen.set(value, copy);
    for (const key of Reflect.ownKeys(value)) {
      const descriptor = Object.getOwnPropertyDescriptor(value, key);
      if (descriptor && descriptor.enumerable) copy[key] = cloneValue(value[key], seen, transfers);
    }
    return copy;
  }

  function cloneWithTransfers(value, options) {
    const transfers = transferSet(options);
    const seen = new Map();
    const copy = cloneValue(value, seen, transfers);
    const bridge = messagePortBridge();
    const ports = [];
    for (const transfer of transfers) {
      let transferred = seen.get(transfer);
      if (!transferred) {
        transferred = bridge.transferMessagePort(transfer);
        if (transferred === null) throw cloneError();
        seen.set(transfer, transferred);
      }
      ports.push(transferred);
    }
    return {value: copy, ports};
  }

  function structuredClone(value, options) {
    return cloneWithTransfers(value, options).value;
  }

  function cloneForMessaging(value, transfer) {
    const clone = cloneWithTransfers(value, {transfer});
    return {data: clone.value, ports: clone.ports};
  }

  const exports = { structuredClone, cloneForMessaging };
  Object.defineProperty(globalThis, key, { value: exports, configurable: false, enumerable: false, writable: false });
  return exports;
})()`

func ensureExports(ctx *quickjs.Context) (*quickjs.Value, error) {
	if ctx == nil {
		return nil, errors.New("structuredclone: nil context")
	}
	if err := blob.InstallGlobal(ctx); err != nil {
		return nil, fmt.Errorf("structuredclone: install Blob globals: %w", err)
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
		return nil, errors.New("structuredclone: initialization returned nil")
	}
	if value.IsException() {
		err := ctx.Exception()
		value.Free()
		if err == nil {
			err = errors.New("structuredclone: initialization failed")
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
		return nil, fmt.Errorf("structuredclone: export %q is unavailable", name)
	}
	return value, nil
}

// Module returns the ESM definition for structuredClone.
func Module() module.Definition {
	return module.Definition{
		Name:    ModuleName,
		Aliases: []string{"node:" + ModuleName},
		Exports: []module.Export{
			{Name: "structuredClone", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return exportValue(ctx, "structuredClone")
			}}},
			{Name: "default", Spec: quickjs.FactorySpec{Factory: ensureExports}},
		},
	}
}

// InstallGlobal installs structuredClone on globalThis.
func InstallGlobal(ctx *quickjs.Context) error {
	value, err := exportValue(ctx, "structuredClone")
	if err != nil {
		return err
	}
	if !ctx.Globals().DefinePropertyValue("structuredClone", value, quickjs.PropConfigurable|quickjs.PropWritable) {
		value.Free()
		return fmt.Errorf("structuredclone: install global")
	}
	value.Free()
	return nil
}
