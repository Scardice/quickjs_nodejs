package util

import (
	"errors"
	"fmt"

	"github.com/Scardice/quickjs_nodejs/module"
	quickjs "github.com/buke/quickjs-go"
)

const extendedKey = "__quickjs_nodejs_util_exports"

const extendedImplementation = `(function () {
  const key = "__quickjs_nodejs_util_exports";
  if (globalThis[key]) return globalThis[key];

  const objectToString = Object.prototype.toString;
  const identifier = /^[A-Za-z_$][A-Za-z0-9_$]*$/;

  function inspectPrimitive(value) {
    if (value === undefined) return "undefined";
    if (value === null) return "null";
    if (typeof value === "string") return "'" + value.replace(/\\/g, "\\\\").replace(/'/g, "\\'") + "'";
    if (typeof value === "bigint") return String(value) + "n";
    if (typeof value === "number" && Object.is(value, -0)) return "-0";
    return String(value);
  }

  function inspectValue(value, depth, seen) {
    if (value === null || (typeof value !== "object" && typeof value !== "function")) return inspectPrimitive(value);
    if (typeof value === "function") {
      const name = value.name;
      return name ? "[Function: " + name + "]" : "[Function (anonymous)]";
    }
    if (seen.has(value)) return "[Circular]";
    if (depth < 0) return Array.isArray(value) ? "[Array]" : "[Object]";
    seen.add(value);

    let result;
    if (value instanceof Date) {
      result = Number.isNaN(value.getTime()) ? "Invalid Date" : "" + value.toISOString();
    } else if (value instanceof RegExp) {
      result = value.toString();
    } else if (value instanceof Map) {
      const items = [];
      for (const entry of value) items.push(inspectValue(entry[0], depth - 1, seen) + " => " + inspectValue(entry[1], depth - 1, seen));
      result = items.length ? "Map(" + items.length + ") { " + items.join(", ") + " }" : "Map(0) {}";
    } else if (value instanceof Set) {
      const items = [];
      for (const entry of value) items.push(inspectValue(entry, depth - 1, seen));
      result = items.length ? "Set(" + items.length + ") { " + items.join(", ") + " }" : "Set(0) {}";
    } else if (Array.isArray(value)) {
      const items = [];
      for (let i = 0; i < value.length; i++) items.push(Object.prototype.hasOwnProperty.call(value, i) ? inspectValue(value[i], depth - 1, seen) : "<empty>");
      result = items.length ? "[ " + items.join(", ") + " ]" : "[]";
    } else {
      const items = [];
      for (const property of Object.keys(value)) {
        const label = identifier.test(property) ? property : inspectPrimitive(property);
        items.push(label + ": " + inspectValue(value[property], depth - 1, seen));
      }
      result = items.length ? "{ " + items.join(", ") + " }" : "{}";
    }
    seen.delete(value);
    return result;
  }

  function inspect(value, options) {
    let depth = 2;
    if (options && typeof options === "object" && Object.prototype.hasOwnProperty.call(options, "depth")) {
      if (options.depth === null) depth = Infinity;
      else if (Number(options.depth) === -1) depth = Infinity;
      else if (Number.isFinite(Number(options.depth))) depth = Number(options.depth);
    }
    return inspectValue(value, depth, new Set());
  }

  function typeTag(value) { return objectToString.call(value); }
  function isTag(tag) { return value => value !== null && value !== undefined && typeTag(value) === tag; }
  const types = {
    isAnyArrayBuffer: value => isTag("[object ArrayBuffer]")(value) || isTag("[object SharedArrayBuffer]")(value),
    isArrayBuffer: isTag("[object ArrayBuffer]"),
    isArgumentsObject: isTag("[object Arguments]"),
    isArrayBufferView: value => typeof ArrayBuffer !== "undefined" && ArrayBuffer.isView(value),
    isAsyncFunction: isTag("[object AsyncFunction]"),
    isBigInt64Array: isTag("[object BigInt64Array]"),
    isBigUint64Array: isTag("[object BigUint64Array]"),
    isBooleanObject: isTag("[object Boolean]"),
    isBoxedPrimitive: value => ["[object Boolean]", "[object Number]", "[object String]", "[object BigInt]", "[object Symbol]"].includes(typeTag(value)),
    isCryptoKey: isTag("[object CryptoKey]"),
    isDataView: isTag("[object DataView]"),
    isDate: isTag("[object Date]"),
    isExternal: () => false,
    isFloat32Array: isTag("[object Float32Array]"),
    isFloat64Array: isTag("[object Float64Array]"),
    isGeneratorFunction: isTag("[object GeneratorFunction]"),
    isGeneratorObject: isTag("[object Generator]"),
    isInt16Array: isTag("[object Int16Array]"),
    isInt32Array: isTag("[object Int32Array]"),
    isInt8Array: isTag("[object Int8Array]"),
    isMap: isTag("[object Map]"),
    isMapIterator: isTag("[object Map Iterator]"),
    isModuleNamespaceObject: isTag("[object Module]"),
    isNativeError: value => value instanceof Error,
    isNumberObject: isTag("[object Number]"),
    isPromise: isTag("[object Promise]"),
    isProxy: () => false,
    isRegExp: isTag("[object RegExp]"),
    isSet: isTag("[object Set]"),
    isSetIterator: isTag("[object Set Iterator]"),
    isSharedArrayBuffer: isTag("[object SharedArrayBuffer]"),
    isStringObject: isTag("[object String]"),
    isSymbolObject: isTag("[object Symbol]"),
    isTypedArray: value => typeof ArrayBuffer !== "undefined" && ArrayBuffer.isView(value) && !(value instanceof DataView),
    isUint16Array: isTag("[object Uint16Array]"),
    isUint32Array: isTag("[object Uint32Array]"),
    isUint8Array: isTag("[object Uint8Array]"),
    isUint8ClampedArray: isTag("[object Uint8ClampedArray]"),
    isWeakMap: isTag("[object WeakMap]"),
    isWeakSet: isTag("[object WeakSet]")
  };

  function promisify(original) {
    if (typeof original !== "function") throw new TypeError("The last argument must be of type Function");
    const custom = typeof Symbol === "function" && Symbol.for ? Symbol.for("nodejs.util.promisify.custom") : undefined;
    if (custom && typeof original[custom] === "function") return original[custom];
    return function (...args) {
      return new Promise((resolve, reject) => {
        let settled = false;
        const callback = (error, ...values) => {
          if (settled) return;
          settled = true;
          if (error) { reject(error); return; }
          resolve(values.length <= 1 ? values[0] : values);
        };
        try { original.apply(this, args.concat(callback)); } catch (error) { if (!settled) { settled = true; reject(error); } }
      });
    };
  }

  function callbackify(original) {
    if (typeof original !== "function") throw new TypeError("The first argument must be of type Function");
    return function (...args) {
      const callback = args.pop();
      if (typeof callback !== "function") throw new TypeError("The last argument must be of type Function");
      Promise.resolve().then(() => original.apply(this, args)).then(
        value => callback.call(this, null, value),
        error => callback.call(this, error || new Error("Promise was rejected with a falsy value"))
      );
    };
  }

  const exports = { inspect, types, promisify, callbackify, format: undefined, default: undefined };
  Object.defineProperty(globalThis, key, { value: exports, configurable: false, enumerable: false, writable: false });
  return exports;
})()`

func ensureExtendedExports(ctx *quickjs.Context) (*quickjs.Value, error) {
	if ctx == nil {
		return nil, errors.New("util: nil context")
	}
	global := ctx.Globals()
	cached := global.Get(extendedKey)
	if cached != nil && cached.IsObject() {
		return cached, nil
	}
	if cached != nil {
		cached.Free()
	}
	exports := ctx.Eval(extendedImplementation)
	if exports == nil {
		return nil, errors.New("util: initialization returned nil")
	}
	if exports.IsException() {
		err := ctx.Exception()
		exports.Free()
		if err == nil {
			err = errors.New("util: initialization failed")
		}
		return nil, err
	}
	format := formatValue(ctx)
	if format == nil {
		exports.Free()
		return nil, errors.New("util: create format function")
	}
	if !exports.DefinePropertyValue("format", format, quickjs.PropConfigurable|quickjs.PropWritable|quickjs.PropEnumerable) {
		format.Free()
		exports.Free()
		return nil, errors.New("util: install format")
	}
	format.Free()
	if !exports.DefinePropertyValue("default", exports, quickjs.PropConfigurable|quickjs.PropWritable|quickjs.PropEnumerable) {
		exports.Free()
		return nil, errors.New("util: install default export")
	}
	return exports, nil
}

func extendedExport(ctx *quickjs.Context, name string) (*quickjs.Value, error) {
	exports, err := ensureExtendedExports(ctx)
	if err != nil {
		return nil, err
	}
	value := exports.Get(name)
	exports.Free()
	if value == nil {
		return nil, fmt.Errorf("util: export %q is unavailable", name)
	}
	return value, nil
}

// Module returns the util and node:util ESM module definition.
func ExtendedModule() module.Definition {
	return module.Definition{
		Name:    ModuleName,
		Aliases: []string{"node:" + ModuleName, "@seal/utilinspect"},
		Exports: []module.Export{
			{Name: "format", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return extendedExport(ctx, "format")
			}}},
			{Name: "inspect", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return extendedExport(ctx, "inspect")
			}}},
			{Name: "types", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return extendedExport(ctx, "types")
			}}},
			{Name: "promisify", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return extendedExport(ctx, "promisify")
			}}},
			{Name: "callbackify", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return extendedExport(ctx, "callbackify")
			}}},
			{Name: "default", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return ensureExtendedExports(ctx)
			}}},
		},
	}
}
