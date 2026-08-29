// Package structuredclone provides a Web structuredClone implementation for QuickJS.
package structuredclone

import (
	"errors"
	"fmt"

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

  function cloneError(message) {
    const error = new Error(message || "The object could not be cloned.");
    error.name = "DataCloneError";
    return error;
  }

  function cloneValue(value, seen) {
    if (value === null || (typeof value !== "object" && typeof value !== "function")) {
      if (typeof value === "symbol" || typeof value === "function") throw cloneError();
      return value;
    }
    if (typeof value === "function") throw cloneError();
    if (seen.has(value)) return seen.get(value);

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
      copy.lastIndex = value.lastIndex;
      seen.set(value, copy);
      return copy;
    }
    if (typeof ArrayBuffer === "function" && value instanceof ArrayBuffer) {
      const copy = value.slice(0);
      seen.set(value, copy);
      return copy;
    }
    if (typeof ArrayBuffer === "function" && typeof ArrayBuffer.isView === "function" && ArrayBuffer.isView(value)) {
      const buffer = cloneValue(value.buffer, seen);
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
      for (const entry of value) copy.set(cloneValue(entry[0], seen), cloneValue(entry[1], seen));
      return copy;
    }
    if (value instanceof Set) {
      const copy = new Set();
      seen.set(value, copy);
      for (const entry of value) copy.add(cloneValue(entry, seen));
      return copy;
    }
    if (Array.isArray(value)) {
      const copy = new Array(value.length);
      seen.set(value, copy);
      for (const key of Reflect.ownKeys(value)) {
        if (key === "length") continue;
        const descriptor = Object.getOwnPropertyDescriptor(value, key);
        if (descriptor && descriptor.enumerable) copy[key] = cloneValue(value[key], seen);
      }
      return copy;
    }

    const copy = {};
    seen.set(value, copy);
    for (const key of Reflect.ownKeys(value)) {
      const descriptor = Object.getOwnPropertyDescriptor(value, key);
      if (descriptor && descriptor.enumerable) copy[key] = cloneValue(value[key], seen);
    }
    return copy;
  }

  function structuredClone(value, options) {
    if (options && options.transfer && options.transfer.length) throw cloneError("Transfer is not supported");
    return cloneValue(value, new Map());
  }

  const exports = { structuredClone };
  Object.defineProperty(globalThis, key, { value: exports, configurable: false, enumerable: false, writable: false });
  return exports;
})()`

func ensureExports(ctx *quickjs.Context) (*quickjs.Value, error) {
	if ctx == nil {
		return nil, errors.New("structuredclone: nil context")
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
		Aliases: []string{"node:" + ModuleName, "@seal/structuredclone"},
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
