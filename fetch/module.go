package fetch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/Scardice/quickjs_nodejs/blob"
	"github.com/Scardice/quickjs_nodejs/buffer"
	"github.com/Scardice/quickjs_nodejs/eventloop"
	"github.com/Scardice/quickjs_nodejs/module"
	quickjs "github.com/buke/quickjs-go"
)

const ModuleName = "fetch"

type Policy func(*http.Request) error

type Config struct {
	Transport http.RoundTripper
	Policy    Policy
}

type Option func(*Config)

func WithTransport(transport http.RoundTripper) Option {
	return func(config *Config) { config.Transport = transport }
}

func WithPolicy(policy Policy) Option {
	return func(config *Config) { config.Policy = policy }
}

var apiSequence atomic.Uint64

type apiBuilder struct {
	config    Config
	apiKey    string
	nativeKey string
}

func newAPIBuilder(config Config) *apiBuilder {
	apiKey := fmt.Sprintf("__quickjs_nodejs_fetch_api_%d", apiSequence.Add(1))
	return &apiBuilder{
		config:    config,
		apiKey:    apiKey,
		nativeKey: apiKey + "_native",
	}
}

func Module(options ...Option) module.Definition {
	builder := newAPIBuilder(applyOptions(options))
	return module.Definition{
		Name:    ModuleName,
		Aliases: []string{"node:fetch"},
		Exports: []module.Export{
			{Name: "fetch", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return builder.exportValue(ctx, "fetch")
			}}},
			{Name: "Headers", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return builder.exportValue(ctx, "Headers")
			}}},
			{Name: "Request", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return builder.exportValue(ctx, "Request")
			}}},
			{Name: "Response", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return builder.exportValue(ctx, "Response")
			}}},
			{Name: "FormData", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return builder.exportValue(ctx, "FormData")
			}}},
			{Name: "default", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return builder.exportValue(ctx, "default")
			}}},
		},
	}
}

func InstallGlobal(ctx *quickjs.Context, options ...Option) error {
	if ctx == nil {
		return errors.New("fetch: nil context")
	}
	builder := newAPIBuilder(applyOptions(options))
	exports, err := builder.build(ctx)
	if err != nil {
		return err
	}
	defer exports.Free()
	for _, name := range []string{"fetch", "Headers", "Request", "Response", "FormData"} {
		value := exports.Get(name)
		if value == nil {
			return fmt.Errorf("fetch: missing global %s", name)
		}
		ctx.Globals().Set(name, value)
	}
	return nil
}

func applyOptions(options []Option) Config {
	config := Config{}
	for _, option := range options {
		if option != nil {
			option(&config)
		}
	}
	return config
}

func (builder *apiBuilder) exportValue(ctx *quickjs.Context, name string) (*quickjs.Value, error) {
	exports, err := builder.build(ctx)
	if err != nil {
		return nil, err
	}
	value := exports.Get(name)
	exports.Free()
	if value == nil {
		return nil, fmt.Errorf("fetch: export %q is unavailable", name)
	}
	return value, nil
}

func (builder *apiBuilder) build(ctx *quickjs.Context) (*quickjs.Value, error) {
	if err := blob.InstallGlobal(ctx); err != nil {
		return nil, fmt.Errorf("fetch: install Blob globals: %w", err)
	}

	global := ctx.Globals()
	cached := global.Get(builder.apiKey)
	if cached != nil && cached.IsObject() {
		return cached, nil
	}
	if cached != nil {
		cached.Free()
	}
	runtime := newFetchRuntime(ctx, builder.config)
	eventloop.RegisterContextResource(ctx, runtime)
	native := ctx.NewObject()
	if native == nil {
		return nil, errors.New("fetch: create native object")
	}
	native.Set("fetch", ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return performFetch(ctx, runtime, args)
	}))
	native.Set("cancel", ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if len(args) > 0 && args[0] != nil {
			runtime.cancel(args[0].ToString())
		}
		return ctx.NewUndefined()
	}))
	global.Set(builder.nativeKey, native)
	result := ctx.Eval(fetchImplementationFor(builder.nativeKey))
	if result == nil {
		global.Delete(builder.nativeKey)
		return nil, errors.New("fetch: initialization returned nil")
	}
	if result.IsException() {
		err := ctx.Exception()
		result.Free()
		global.Delete(builder.nativeKey)
		if err == nil {
			err = errors.New("fetch: initialization failed")
		}
		return nil, err
	}
	global.Delete(builder.nativeKey)
	global.Set(builder.apiKey, result)
	return global.Get(builder.apiKey), nil
}

func fetchImplementationFor(nativeKey string) string {
	return strings.Replace(fetchImplementation, "__quickjs_nodejs_fetch_native", nativeKey, 1)
}

const fetchImplementation = `(function () {
  const native = globalThis.__quickjs_nodejs_fetch_native;
  const Blob = globalThis.Blob;
  const blobBytes = globalThis.__quickjs_nodejs_blob_exports.bytes;
  if (typeof globalThis.TextDecoder !== "function") {
    globalThis.TextDecoder = class TextDecoder {
      decode(input) {
        const bytes = input instanceof Uint8Array ? input : new Uint8Array(input);
        if (bytes.length === 0) return "";
        let encoded = "";
        for (const byte of bytes) encoded += "%" + byte.toString(16).padStart(2, "0");
        return decodeURIComponent(encoded);
      }
    };
  }
  function toByteString(value, label) {
    if (typeof value === "symbol") throw new TypeError("Invalid HTTP header " + label);
    const text = String(value);
    for (let index = 0; index < text.length; index++) {
      if (text.charCodeAt(index) > 0xFF) throw new TypeError("Invalid HTTP header " + label);
    }
    return text;
  }
  function normalizeHeaderName(name) {
    const normalized = toByteString(name, "name");
    if (!/^[!#$%&'*+\-.^_\x60|~0-9A-Za-z]+$/.test(normalized)) throw new TypeError("Invalid HTTP header name");
    return normalized.toLowerCase();
  }
  function normalizeHeaderValue(value) {
    const trimmed = toByteString(value, "value").replace(/^[\t\n\r ]+|[\t\n\r ]+$/g, "");
    if (/[\0\r\n]/.test(trimmed)) throw new TypeError("Invalid HTTP header value");
    return trimmed;
  }
  function makeHeadersIterator(headers, project) {
    let index = 0;
    const prototype = Object.create(Object.getPrototypeOf(Object.getPrototypeOf([][Symbol.iterator]())));
    const iterator = Object.create(prototype);
    Object.defineProperty(prototype, "next", {
      configurable: true,
      enumerable: true,
      writable: true,
      value() {
        const entries = headers._entries();
        if (index >= entries.length) return {done: true};
        return {value: project(entries[index++]), done: false};
      }
    });
    return iterator;
  }
  class Headers {
    constructor(init, guard = "none") {
      this._list = [];
      this._guard = guard;
      if (init === undefined) return;
      if (init === null) throw new TypeError("Headers init must not be null");
      if (typeof init[Symbol.iterator] === "function") {
        for (const pair of init) {
          if (pair === null || typeof pair !== "object" || typeof pair[Symbol.iterator] !== "function") {
            throw new TypeError("Header pair must be an iterable object");
          }
          const values = Array.from(pair);
          if (values.length !== 2) throw new TypeError("Header pair must contain exactly two entries");
          this.append(values[0], values[1]);
        }
        return;
      }
      if (typeof init === "object") {
        for (const key of Reflect.ownKeys(init)) {
          const descriptor = Object.getOwnPropertyDescriptor(init, key);
          if (!descriptor || !descriptor.enumerable) continue;
          const normalizedName = normalizeHeaderName(key);
          this.append(normalizedName, init[key]);
        }
        return;
      }
      throw new TypeError("Headers init must be a record or sequence");
    }
    append(name, value) {
      const normalizedName = normalizeHeaderName(name);
      if (this._guard === "response" && normalizedName === "set-cookie") return;
      this._list.push([normalizedName, normalizeHeaderValue(value)]);
    }
    set(name, value) {
      const normalizedName = normalizeHeaderName(name);
      if (this._guard === "response" && normalizedName === "set-cookie") return;
      const normalizedValue = normalizeHeaderValue(value);
      this._list = this._list.filter(entry => entry[0] !== normalizedName);
      this._list.push([normalizedName, normalizedValue]);
    }
    get(name) {
      const normalizedName = normalizeHeaderName(name);
      const values = this._list.filter(entry => entry[0] === normalizedName).map(entry => entry[1]);
      return values.length === 0 ? null : values.join(", ");
    }
    getSetCookie() {
      return this._list.filter(entry => entry[0] === "set-cookie").map(entry => entry[1]);
    }
    has(name) {
      const normalizedName = normalizeHeaderName(name);
      return this._list.some(entry => entry[0] === normalizedName);
    }
    delete(name) {
      const normalizedName = normalizeHeaderName(name);
      this._list = this._list.filter(entry => entry[0] !== normalizedName);
    }
    _entries() {
      const names = Array.from(new Set(this._list.map(entry => entry[0]))).sort();
      const entries = [];
      for (const name of names) {
        const values = this._list.filter(entry => entry[0] === name).map(entry => entry[1]);
        if (name === "set-cookie") {
          for (const value of values) entries.push([name, value]);
        } else {
          entries.push([name, values.join(", ")]);
        }
      }
      return entries;
    }
    entries() { return makeHeadersIterator(this, entry => entry); }
    keys() { return makeHeadersIterator(this, entry => entry[0]); }
    values() { return makeHeadersIterator(this, entry => entry[1]); }
    forEach(callback, thisArg) {
      for (const [name, value] of this.entries()) callback.call(thisArg, value, name, this);
    }
    toJSON() { return Object.fromEntries(this.entries()); }
    [Symbol.iterator]() { return this.entries(); }
  }
  class Request {
    constructor(input, init) {
      init = init || {};
      if (input instanceof Request) {
        this.url = input.url;
        this.method = input.method;
        this.headers = new Headers(input.headers);
        this.body = input.body;
        this.signal = input.signal;
        if (init.method !== undefined) this.method = String(init.method).toUpperCase();
        if (init.headers !== undefined) this.headers = new Headers(init.headers);
        if (init.body !== undefined) this.body = init.body;
        if (init.signal !== undefined) this.signal = init.signal;
      } else {
        this.url = String(input);
        this.method = String(init.method || "GET").toUpperCase();
        this.headers = new Headers(init.headers);
        this.body = init.body === undefined ? null : init.body;
        this.signal = init.signal === undefined ? null : init.signal;
      }
    }
    clone() { return new Request(this); }
  }
  class Response {
    constructor(body, init) {
      init = init || {};
      this.status = init.status === undefined ? 200 : Number(init.status);
      this.statusText = init.statusText === undefined ? "" : String(init.statusText);
      this.headers = new Headers(init.headers, "response");
      this.url = init.url === undefined ? "" : String(init.url);
      this.ok = this.status >= 200 && this.status < 300;
      this.redirected = false;
      this.type = "default";
      this._body = body === undefined || body === null ? new Uint8Array(0) : body instanceof Blob ? blobBytes(body) : body;
      this.bodyUsed = false;
    }
    clone() {
      const body = this._body instanceof Uint8Array ? this._body.slice() : this._body;
      return new Response(body, {status:this.status, statusText:this.statusText, headers:this.headers, url:this.url});
    }
    arrayBuffer() { this.bodyUsed = true; return Promise.resolve(this._body instanceof Uint8Array ? this._body.buffer.slice(this._body.byteOffset, this._body.byteOffset + this._body.byteLength) : new TextEncoder().encode(this._body).buffer); }
    text() { this.bodyUsed = true; return Promise.resolve(this._body instanceof Uint8Array ? new TextDecoder().decode(this._body) : String(this._body)); }
    json() { return this.text().then(value => JSON.parse(value)); }
    blob() { this.bodyUsed = true; return Promise.resolve(new Blob([this._body], {type: this.headers.get("content-type") || ""})); }
  }
  class FormData {
    constructor() { this._entries = []; }
    append(name, value, filename) { this._entries.push([String(name), value, filename]); }
    set(name, value, filename) { this.delete(name); this.append(name, value, filename); }
    get(name) { const item = this._entries.find(entry => entry[0] === String(name)); return item ? item[1] : null; }
    getAll(name) { return this._entries.filter(entry => entry[0] === String(name)).map(entry => entry[1]); }
    has(name) { return this._entries.some(entry => entry[0] === String(name)); }
    delete(name) { this._entries = this._entries.filter(entry => entry[0] !== String(name)); }
    entries() { return this._entries.map(entry => [entry[0], entry[1]]); }
    [Symbol.iterator]() { return this.entries()[Symbol.iterator](); }
  }
  function formDataText(value) {
    if (value instanceof Uint8Array) return new TextDecoder().decode(value);
    return String(value);
  }
  function encodeFormData(form) {
    const boundary = "----quickjs-nodejs-" + Math.random().toString(16).slice(2);
    const parts = [];
    for (const entry of form._entries || []) {
      const name = String(entry[0]).replace(/[\r\n"]/g, "_");
      const filename = entry[2] === undefined ? undefined : String(entry[2]).replace(/[\r\n"]/g, "_");
      let disposition = 'Content-Disposition: form-data; name="' + name + '"';
      if (filename !== undefined) disposition += '; filename="' + filename + '"';
      parts.push("--" + boundary + "\r\n" + disposition + "\r\n\r\n" + formDataText(entry[1]) + "\r\n");
    }
    return {
      body: parts.join("") + "--" + boundary + "--\r\n",
      contentType: "multipart/form-data; boundary=" + boundary
    };
  }
  function normalizeInput(input, init) {
    const request = new Request(input, init);
    let body = request.body;
    if (body instanceof FormData) {
      const encoded = encodeFormData(body);
      body = encoded.body;
      if (!request.headers.has("content-type")) request.headers.set("content-type", encoded.contentType);
    } else if (body instanceof Blob) {
      body = blobBytes(body);
      if (!request.headers.has("content-type") && request.body.type) request.headers.set("content-type", request.body.type);
    } else if (typeof URLSearchParams === "function" && body instanceof URLSearchParams) {
      body = body.toString();
      if (!request.headers.has("content-type")) request.headers.set("content-type", "application/x-www-form-urlencoded;charset=UTF-8");
    }
    return {url: request.url, method: request.method, headers: Array.from(request.headers), body, signal: request.signal};
  }
  function fetch(input, init) {
    const request = normalizeInput(input, init);
    const pending = native.fetch(request);
    const signal = request.signal;
    let removeAbort = () => {};
    if (signal && typeof signal.addEventListener === "function" && typeof signal.removeEventListener === "function") {
      const cancel = () => native.cancel(pending.id);
      signal.addEventListener("abort", cancel, {once: true});
      removeAbort = () => signal.removeEventListener("abort", cancel);
    }
    return pending.promise.then(
      record => {
        removeAbort();
        return new Response(record.body, {status:record.status, statusText:record.statusText, headers:record.headers, url:record.url});
      },
      error => {
        removeAbort();
        throw error;
      }
    );
  }
  return {fetch, Headers, Request, Response, FormData, default: {fetch, Headers, Request, Response, FormData}};
})()`

func performFetch(ctx *quickjs.Context, runtime *fetchRuntime, args []*quickjs.Value) *quickjs.Value {
	if runtime == nil {
		return rejectedFetchHandle(ctx, errors.New("fetch: runtime is unavailable"))
	}
	config := runtime.config
	if len(args) < 1 || args[0] == nil || !args[0].IsObject() {
		return rejectedFetchHandle(ctx, errors.New("fetch: request is required"))
	}
	requestValue := args[0]
	method := "GET"
	if value := requestValue.Get("method"); value != nil {
		if !value.IsUndefined() {
			method = strings.ToUpper(value.ToString())
		}
		value.Free()
	}
	urlValue := requestValue.Get("url")
	if urlValue == nil {
		return rejectedFetchHandle(ctx, errors.New("fetch: request URL is required"))
	}
	rawURL := urlValue.ToString()
	urlValue.Free()
	if _, err := url.Parse(rawURL); err != nil {
		return rejectedFetchHandle(ctx, fmt.Errorf("fetch: invalid URL: %w", err))
	}
	bodyValue := requestValue.Get("body")
	body, err := fetchBody(ctx, bodyValue)
	if bodyValue != nil {
		bodyValue.Free()
	}
	if err != nil {
		return rejectedFetchHandle(ctx, err)
	}
	headers, err := fetchHeaders(requestValue.Get("headers"))
	if err != nil {
		return rejectedFetchHandle(ctx, err)
	}
	request, err := http.NewRequest(method, rawURL, bytes.NewReader(body))
	if err != nil {
		return rejectedFetchHandle(ctx, err)
	}
	request.Header = headers
	if config.Policy != nil {
		if err := config.Policy(request); err != nil {
			return rejectedFetchHandle(ctx, err)
		}
	}
	if config.Transport == nil {
		return rejectedFetchHandle(ctx, errors.New("fetch: no transport configured"))
	}

	signal := requestValue.Get("signal")
	aborted, err := fetchSignalAborted(signal)
	if err != nil {
		return rejectedFetchHandle(ctx, err)
	}
	if aborted {
		return rejectedFetchHandle(ctx, errors.New("The operation was aborted"))
	}

	requestContext, cancel := context.WithCancel(context.Background())
	request = request.WithContext(requestContext)
	requestID, err := runtime.register(cancel)
	if err != nil {
		cancel()
		return rejectedFetchHandle(ctx, err)
	}

	promise := ctx.NewPromise(func(resolve, reject func(*quickjs.Value)) {
		go func() {
			response, err := config.Transport.RoundTrip(request)
			if err != nil {
				fetchErr := err
				scheduleFetch(ctx, runtime, requestID, func(inner *quickjs.Context) {
					rejectError := inner.NewError(normalizeFetchError(fetchErr))
					reject(rejectError)
					rejectError.Free()
				})
				return
			}
			if response == nil {
				scheduleFetch(ctx, runtime, requestID, func(inner *quickjs.Context) {
					rejectError := inner.NewError(errors.New("fetch: transport returned nil response"))
					reject(rejectError)
					rejectError.Free()
				})
				return
			}
			data := []byte(nil)
			if response.Body != nil {
				data, err = io.ReadAll(response.Body)
				response.Body.Close()
			}
			if err != nil {
				fetchErr := err
				scheduleFetch(ctx, runtime, requestID, func(inner *quickjs.Context) {
					rejectError := inner.NewError(normalizeFetchError(fetchErr))
					reject(rejectError)
					rejectError.Free()
				})
				return
			}
			statusText := http.StatusText(response.StatusCode)
			if statusText == "" {
				statusText = response.Status
			}
			responseURL := rawURL
			if response.Request != nil && response.Request.URL != nil {
				responseURL = response.Request.URL.String()
			}
			headerPairs := make([][2]string, 0, len(response.Header))
			for name, values := range response.Header {
				for _, value := range values {
					headerPairs = append(headerPairs, [2]string{name, value})
				}
			}
			scheduleFetch(ctx, runtime, requestID, func(inner *quickjs.Context) {
				record := inner.NewObject()
				record.Set("status", inner.NewInt32(int32(response.StatusCode)))
				record.Set("statusText", inner.NewString(statusText))
				record.Set("url", inner.NewString(responseURL))
				record.Set("headers", headerPairArray(inner, headerPairs))
				record.Set("body", inner.NewUint8Array(data))
				resolve(record)
				record.Free()
			})
		}()
	})
	handle := fetchHandle(ctx, promise, requestID)
	if handle == nil {
		runtime.complete(requestID)
	}
	return handle
}

func scheduleFetch(ctx *quickjs.Context, runtime *fetchRuntime, requestID string, task func(*quickjs.Context)) {
	if ctx == nil || runtime == nil || task == nil || !ctx.Schedule(func(inner *quickjs.Context) {
		runtime.complete(requestID)
		task(inner)
	}) {
		if runtime != nil {
			runtime.complete(requestID)
		}
	}
}

func normalizeFetchError(err error) error {
	if errors.Is(err, context.Canceled) {
		return errors.New("The operation was aborted")
	}
	return err
}

func fetchHandle(ctx *quickjs.Context, promise *quickjs.Value, id string) *quickjs.Value {
	if promise == nil {
		return nil
	}
	handle := ctx.NewObject()
	if handle == nil {
		promise.Free()
		return nil
	}
	handle.Set("id", ctx.NewString(id))
	handle.Set("promise", promise)
	return handle
}

func rejectedFetchPromise(ctx *quickjs.Context, err error) *quickjs.Value {
	if err == nil {
		err = errors.New("fetch failed")
	}
	return ctx.NewPromise(func(_ func(*quickjs.Value), reject func(*quickjs.Value)) {
		rejectError := ctx.NewError(err)
		reject(rejectError)
		rejectError.Free()
	})
}

func rejectedFetchHandle(ctx *quickjs.Context, err error) *quickjs.Value {
	return fetchHandle(ctx, rejectedFetchPromise(ctx, err), "")
}

func fetchSignalAborted(value *quickjs.Value) (bool, error) {
	if value == nil {
		return false, nil
	}
	if value.IsUndefined() || value.IsNull() {
		value.Free()
		return false, nil
	}
	if !value.IsObject() {
		value.Free()
		return false, errors.New("fetch: signal must be an AbortSignal")
	}
	aborted := value.Get("aborted")
	value.Free()
	if aborted == nil {
		return false, errors.New("fetch: signal.aborted is unavailable")
	}
	defer aborted.Free()
	return aborted.ToBool(), nil
}

func fetchBody(ctx *quickjs.Context, value *quickjs.Value) ([]byte, error) {
	if value == nil || value.IsNull() || value.IsUndefined() {
		return nil, nil
	}
	if value.IsString() {
		return []byte(value.ToString()), nil
	}
	return buffer.Bytes(ctx, value)
}

func fetchHeaders(value *quickjs.Value) (http.Header, error) {
	headers := make(http.Header)
	if value == nil {
		return headers, nil
	}
	defer value.Free()
	if value.IsUndefined() || value.IsNull() {
		return headers, nil
	}
	pairs, err := arrayValues(value)
	if err != nil {
		return nil, err
	}
	for _, pair := range pairs {
		if len(pair) < 2 {
			for _, item := range pair {
				item.Free()
			}
			continue
		}
		name := pair[0].ToString()
		value := pair[1].ToString()
		for _, item := range pair {
			item.Free()
		}
		headers.Add(name, value)
	}
	return headers, nil
}

func arrayValues(value *quickjs.Value) ([][]*quickjs.Value, error) {
	if value == nil || !value.IsArray() {
		return nil, errors.New("fetch: headers must be an array")
	}
	length := value.Get("length")
	if length == nil {
		return nil, errors.New("fetch: headers length unavailable")
	}
	count := length.ToInt64()
	length.Free()
	if count < 0 || count > 1<<20 {
		return nil, errors.New("fetch: too many headers")
	}
	pairs := make([][]*quickjs.Value, 0, count)
	for index := int64(0); index < count; index++ {
		pair := value.GetIdx(index)
		if pair == nil || !pair.IsArray() {
			if pair != nil {
				pair.Free()
			}
			return nil, errors.New("fetch: invalid header pair")
		}
		pairLength := pair.Get("length")
		if pairLength == nil {
			pair.Free()
			return nil, errors.New("fetch: invalid header pair length")
		}
		pairCount := pairLength.ToInt64()
		pairLength.Free()
		items := make([]*quickjs.Value, 0, pairCount)
		for itemIndex := int64(0); itemIndex < pairCount; itemIndex++ {
			items = append(items, pair.GetIdx(itemIndex))
		}
		pair.Free()
		pairs = append(pairs, items)
	}
	return pairs, nil
}

func headerPairArray(ctx *quickjs.Context, pairs [][2]string) *quickjs.Value {
	array := ctx.Eval("[]")
	for index, pair := range pairs {
		item := ctx.Eval("[]")
		item.Set("0", ctx.NewString(pair[0]))
		item.Set("1", ctx.NewString(pair[1]))
		array.Set(fmt.Sprintf("%d", index), item)
	}
	array.Set("length", ctx.NewInt32(int32(len(pairs))))
	return array
}
