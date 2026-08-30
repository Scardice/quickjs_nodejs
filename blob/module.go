// Package blob provides Blob and File Web API bindings for QuickJS.
package blob

import (
	"errors"
	"fmt"

	"github.com/Scardice/quickjs_nodejs/module"
	quickjs "github.com/buke/quickjs-go"
)

const (
	ModuleName = "blob"
	hiddenKey  = "__quickjs_nodejs_blob_exports"
)

const implementation = `(function () {
  const key = "__quickjs_nodejs_blob_exports";
  if (globalThis[key]) return globalThis[key];

  const data = new WeakMap();

  function toUSVString(value) {
    const input = String(value);
    let output = "";
    for (let index = 0; index < input.length; index++) {
      const code = input.charCodeAt(index);
      if (code >= 0xD800 && code <= 0xDBFF) {
        const next = input.charCodeAt(index + 1);
        if (next >= 0xDC00 && next <= 0xDFFF) {
          output += input[index] + input[index + 1];
          index++;
        } else {
          output += "\uFFFD";
        }
      } else if (code >= 0xDC00 && code <= 0xDFFF) {
        output += "\uFFFD";
      } else {
        output += input[index];
      }
    }
    return output;
  }

  function encodeUTF8(value) {
    const input = toUSVString(value);
    const output = [];
    for (let index = 0; index < input.length; index++) {
      let codePoint = input.charCodeAt(index);
      if (codePoint >= 0xD800 && codePoint <= 0xDBFF) {
        codePoint = 0x10000 + (codePoint - 0xD800) * 0x400 + (input.charCodeAt(++index) - 0xDC00);
      }
      if (codePoint <= 0x7F) {
        output.push(codePoint);
      } else if (codePoint <= 0x7FF) {
        output.push(0xC0 | (codePoint >> 6), 0x80 | (codePoint & 0x3F));
      } else if (codePoint <= 0xFFFF) {
        output.push(0xE0 | (codePoint >> 12), 0x80 | ((codePoint >> 6) & 0x3F), 0x80 | (codePoint & 0x3F));
      } else {
        output.push(0xF0 | (codePoint >> 18), 0x80 | ((codePoint >> 12) & 0x3F), 0x80 | ((codePoint >> 6) & 0x3F), 0x80 | (codePoint & 0x3F));
      }
    }
    return new Uint8Array(output);
  }

  function decodeUTF8(input) {
    let output = "";
    for (let index = 0; index < input.length;) {
      const first = input[index++];
      if (first <= 0x7F) {
        output += String.fromCharCode(first);
        continue;
      }
      let size = 0;
      let codePoint = 0;
      let minimum = 0;
      if (first >= 0xC2 && first <= 0xDF) {
        size = 1;
        codePoint = first & 0x1F;
        minimum = 0x80;
      } else if (first >= 0xE0 && first <= 0xEF) {
        size = 2;
        codePoint = first & 0x0F;
        minimum = 0x800;
      } else if (first >= 0xF0 && first <= 0xF4) {
        size = 3;
        codePoint = first & 0x07;
        minimum = 0x10000;
      } else {
        output += "\uFFFD";
        continue;
      }
      if (index + size > input.length) {
        output += "\uFFFD";
        continue;
      }
      let valid = true;
      for (let offset = 0; offset < size; offset++) {
        const next = input[index + offset];
        if ((next & 0xC0) !== 0x80) {
          valid = false;
          break;
        }
        codePoint = (codePoint << 6) | (next & 0x3F);
      }
      if (!valid || codePoint < minimum || codePoint > 0x10FFFF || (codePoint >= 0xD800 && codePoint <= 0xDFFF)) {
        output += "\uFFFD";
        continue;
      }
      index += size;
      if (codePoint <= 0xFFFF) {
        output += String.fromCharCode(codePoint);
      } else {
        const adjusted = codePoint - 0x10000;
        output += String.fromCharCode(0xD800 + (adjusted >> 10), 0xDC00 + (adjusted & 0x3FF));
      }
    }
    return output;
  }

  if (typeof globalThis.TextEncoder !== "function") {
    globalThis.TextEncoder = class TextEncoder {
      get encoding() { return "utf-8"; }
      encode(input = "") { return encodeUTF8(input); }
    };
  }
  if (typeof globalThis.TextDecoder !== "function") {
    globalThis.TextDecoder = class TextDecoder {
      get encoding() { return "utf-8"; }
      decode(input = undefined) {
        const bytes = input === undefined ? new Uint8Array(0) : input instanceof Uint8Array ? input : new Uint8Array(input);
        return decodeUTF8(bytes);
      }
    };
  }

  function copyBytes(bytes) {
    return new Uint8Array(bytes);
  }

  function normalizeType(value) {
    const type = String(value);
    for (let index = 0; index < type.length; index++) {
      const code = type.charCodeAt(index);
      if (code < 0x20 || code > 0x7E) return "";
    }
    return type.toLowerCase();
  }

  function bytesForPart(part) {
    if (part instanceof Blob) return copyBytes(data.get(part));
    if (typeof ArrayBuffer === "function" && part instanceof ArrayBuffer) return new Uint8Array(part.slice(0));
    if (typeof ArrayBuffer === "function" && typeof ArrayBuffer.isView === "function" && ArrayBuffer.isView(part)) {
      return copyBytes(new Uint8Array(part.buffer, part.byteOffset, part.byteLength));
    }
    return encodeUTF8(part);
  }

  function joinParts(parts) {
    const chunks = [];
    let size = 0;
    for (const part of parts) {
      const bytes = bytesForPart(part);
      chunks.push(bytes);
      size += bytes.length;
    }
    const output = new Uint8Array(size);
    let offset = 0;
    for (const chunk of chunks) {
      output.set(chunk, offset);
      offset += chunk.length;
    }
    return output;
  }

  function toIntegerOrInfinity(value) {
    const number = Number(value);
    if (Number.isNaN(number)) return 0;
    if (!Number.isFinite(number)) return number;
    const sign = number < 0 ? -1 : 1;
    const absolute = Math.abs(number);
    let integer = Math.floor(absolute);
    const fraction = absolute - integer;
    if (fraction > 0.5 || (fraction === 0.5 && integer % 2 !== 0)) integer++;
    return sign * integer;
  }

  class Blob {
    constructor(...args) {
      let parts = args[0];
      const options = args[1];
      if (parts === undefined) parts = [];
      if (parts === null || (typeof parts !== "object" && typeof parts !== "function")) {
        throw new TypeError("Blob constructor requires an iterable blobParts argument");
      }
      const bytes = joinParts(parts);
      if (options != null && typeof options !== "object" && typeof options !== "function") {
        throw new TypeError("Blob options must be an object");
      }
      const endingsValue = options == null ? undefined : options.endings;
      const endings = endingsValue === undefined ? "transparent" : String(endingsValue);
      if (endings !== "transparent" && endings !== "native") {
        throw new TypeError("Blob endings must be transparent or native");
      }
      const type = options == null ? undefined : options.type;
      data.set(this, bytes);
      this._type = type === undefined ? "" : normalizeType(type);

    }
    get size() { return data.get(this).length; }
    get type() { return this._type; }
    get [Symbol.toStringTag]() { return "Blob"; }

    arrayBuffer() {
      const bytes = copyBytes(data.get(this));
      return Promise.resolve(bytes.buffer);
    }

    bytes() {
      return Promise.resolve(copyBytes(data.get(this)));
    }

    text() {
      return Promise.resolve(decodeUTF8(data.get(this)));
    }

    slice(start = 0, end = undefined, contentType = "") {
      const size = this.size;
      const relativeStart = toIntegerOrInfinity(start);
      const first = relativeStart === -Infinity ? 0 : relativeStart < 0 ? Math.max(size + relativeStart, 0) : Math.min(relativeStart, size);
      const relativeEnd = end === undefined ? size : toIntegerOrInfinity(end);
      const last = relativeEnd === -Infinity ? 0 : relativeEnd < 0 ? Math.max(size + relativeEnd, 0) : Math.min(relativeEnd, size);
      return new Blob([data.get(this).subarray(first, Math.max(first, last))], { type: contentType });
    }
  }

  class File extends Blob {
    constructor(parts, name, options = {}) {
      super(parts, options);
      this._name = toUSVString(name);
      const lastModified = options == null || options.lastModified === undefined ? Date.now() : Number(options.lastModified);
      this._lastModified = Number.isFinite(lastModified) ? Math.trunc(lastModified) : 0;
    }

    get name() { return this._name; }
    get lastModified() { return this._lastModified; }
    get [Symbol.toStringTag]() { return "File"; }
  }

  function bytes(value) {
    if (!(value instanceof Blob)) throw new TypeError("value is not a Blob");
    return copyBytes(data.get(value));
  }

  const exports = { Blob, File, bytes };
  Object.defineProperty(globalThis, key, { value: exports, configurable: false, enumerable: false, writable: false });
  return exports;
})()`

func ensureExports(ctx *quickjs.Context) (*quickjs.Value, error) {
	if ctx == nil {
		return nil, errors.New("blob: nil context")
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
		return nil, errors.New("blob: initialization returned nil")
	}
	if value.IsException() {
		err := ctx.Exception()
		value.Free()
		if err == nil {
			err = errors.New("blob: initialization failed")
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
		return nil, fmt.Errorf("blob: export %q is unavailable", name)
	}
	return value, nil
}

// Module returns the ESM definition for Blob and File.
func Module() module.Definition {
	return module.Definition{
		Name:    ModuleName,
		Aliases: []string{"node:" + ModuleName},
		Exports: []module.Export{
			{Name: "Blob", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return exportValue(ctx, "Blob")
			}}},
			{Name: "File", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return exportValue(ctx, "File")
			}}},
			{Name: "default", Spec: quickjs.FactorySpec{Factory: ensureExports}},
		},
	}
}

// InstallGlobal installs Blob and File on globalThis.
func InstallGlobal(ctx *quickjs.Context) error {
	exports, err := ensureExports(ctx)
	if err != nil {
		return err
	}
	defer exports.Free()
	for _, name := range []string{"Blob", "File"} {
		value := exports.Get(name)
		if value == nil {
			return fmt.Errorf("blob: export %q is unavailable", name)
		}
		if !ctx.Globals().DefinePropertyValue(name, value, quickjs.PropConfigurable|quickjs.PropWritable) {
			value.Free()
			return fmt.Errorf("blob: install global %q", name)
		}
		value.Free()
	}
	return nil
}

// Bytes returns a copied byte representation of a Blob or File.
func Bytes(ctx *quickjs.Context, value *quickjs.Value) ([]byte, error) {
	if ctx == nil || value == nil {
		return nil, errors.New("blob: nil value")
	}
	exports, err := ensureExports(ctx)
	if err != nil {
		return nil, err
	}
	defer exports.Free()
	bytesFn := exports.Get("bytes")
	if bytesFn == nil {
		return nil, errors.New("blob: bytes export is unavailable")
	}
	defer bytesFn.Free()
	result := bytesFn.Execute(value)
	if result == nil {
		return nil, errors.New("blob: bytes returned nil")
	}
	defer result.Free()
	if result.IsException() {
		return nil, ctx.Exception()
	}
	if !result.IsUint8Array() && !result.IsUint8ClampedArray() {
		return nil, errors.New("blob: bytes returned a non-byte value")
	}
	bytes, err := result.ToUint8Array()
	if err != nil {
		return nil, fmt.Errorf("blob: read bytes: %w", err)
	}
	return append([]byte(nil), bytes...), nil
}
