// Package url provides URL and URLSearchParams bindings for QuickJS.
package url

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	nodeerrors "github.com/Scardice/quickjs_nodejs/errors"
	"github.com/Scardice/quickjs_nodejs/module"
	quickjs "github.com/buke/quickjs-go"
	"golang.org/x/net/idna"
)

const ModuleName = "url"

const (
	urlStateProperty                = "__quickjs_nodejs_url_state__"
	urlSearchParamsProperty         = "__quickjs_nodejs_url_search_params__"
	paramsStateProperty             = "__quickjs_nodejs_urlsearchparams_state__"
	paramsOwnerProperty             = "__quickjs_nodejs_urlsearchparams_owner__"
	paramsIteratorStateProperty     = "__quickjs_nodejs_urlsearchparams_iterator_state__"
	urlModuleProperty               = "__quickjs_nodejs_url_module__"
	paramsIteratorPrototypeProperty = "__quickjs_nodejs_urlsearchparams_iterator_prototype__"
)

type urlModuleState struct {
	urlPrototype    *quickjs.Value
	paramsPrototype *quickjs.Value
}

// Module returns the memory-backed ESM definition for url and node:url.
func Module() module.Definition {
	factory := func(name string) quickjs.ValueSpec {
		return quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
			return moduleValue(ctx, name), nil
		}}
	}
	return module.Definition{
		Name:    ModuleName,
		Aliases: []string{"node:url"},
		Exports: []module.Export{
			{Name: "URL", Spec: factory("URL")},
			{Name: "URLSearchParams", Spec: factory("URLSearchParams")},
			{Name: "domainToASCII", Spec: factory("domainToASCII")},
			{Name: "domainToUnicode", Spec: factory("domainToUnicode")},
			{Name: "default", Spec: factory("default")},
		},
	}
}

// InstallGlobal installs URL and URLSearchParams on globalThis.
func InstallGlobal(ctx *quickjs.Context) error {
	if ctx == nil {
		return fmt.Errorf("url context is nil")
	}
	bundle := cachedModule(ctx)
	if bundle == nil {
		return fmt.Errorf("create url module failed")
	}
	urlCtor := bundle.Get("URL")
	paramsCtor := bundle.Get("URLSearchParams")
	bundle.Free()
	if urlCtor == nil || paramsCtor == nil {
		if urlCtor != nil {
			urlCtor.Free()
		}
		if paramsCtor != nil {
			paramsCtor.Free()
		}
		return fmt.Errorf("create URL constructors failed")
	}
	ctx.Globals().Set("URL", urlCtor)
	ctx.Globals().Set("URLSearchParams", paramsCtor)
	return nil
}

func moduleValue(ctx *quickjs.Context, name string) *quickjs.Value {
	bundle := cachedModule(ctx)
	if bundle == nil {
		return nil
	}
	if name == "default" {
		return bundle
	}
	value := bundle.Get(name)
	bundle.Free()
	return value
}

func cachedModule(ctx *quickjs.Context) *quickjs.Value {
	globals := ctx.Globals()
	cached := globals.Get(urlModuleProperty)
	if cached != nil && cached.IsObject() {
		return cached
	}
	if cached != nil {
		cached.Free()
	}
	state := &urlModuleState{}
	paramsCtor := newURLSearchParamsConstructor(ctx, state)
	urlCtor := newURLConstructor(ctx, state)
	if paramsCtor == nil || urlCtor == nil || state.paramsPrototype == nil || state.urlPrototype == nil {
		if paramsCtor != nil {
			paramsCtor.Free()
		}
		if urlCtor != nil {
			urlCtor.Free()
		}
		return nil
	}
	bundle := ctx.NewObject()
	bundle.Set("URL", urlCtor)
	bundle.Set("URLSearchParams", paramsCtor)
	bundle.Set("domainToASCII", ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if len(args) == 0 {
			return ctx.NewString("")
		}
		result, err := idna.ToASCII(args[0].ToString())
		if err != nil {
			return ctx.NewString("")
		}
		return ctx.NewString(result)
	}))
	bundle.Set("domainToUnicode", ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if len(args) == 0 {
			return ctx.NewString("")
		}
		result, err := idna.ToUnicode(args[0].ToString())
		if err != nil {
			return ctx.NewString("")
		}
		return ctx.NewString(result)
	}))
	if !globals.DefinePropertyValue(urlModuleProperty, bundle, quickjs.PropConfigurable|quickjs.PropWritable) {
		bundle.Free()
		return nil
	}
	bundle.Free()
	return globals.Get(urlModuleProperty)
}

func newURLConstructor(ctx *quickjs.Context, state *urlModuleState) *quickjs.Value {
	proto := ctx.NewObject()
	if proto == nil {
		return nil
	}
	installURLPrototype(ctx, proto, state)
	implementation := ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		input := ""
		if len(args) > 0 && args[0] != nil {
			input = valueString(args[0])
		}
		var base *parsedURL
		if len(args) > 1 && args[1] != nil && !args[1].IsUndefined() {
			baseHref := ""
			if baseState := getURLState(args[1]); baseState != nil {
				baseHref, _ = stateString(baseState, "href")
				baseState.Free()
			} else {
				baseHref = valueString(args[1])
			}
			var err error
			base, err = parseURL(baseHref, nil, true)
			if err != nil {
				return invalidURL(ctx, baseHref, "Invalid base URL")
			}
		}
		u, err := parseURL(input, base, base == nil)
		if err != nil {
			return invalidURL(ctx, input, "Invalid URL")
		}
		instance := ctx.NewObject()
		if instance == nil {
			return nil
		}
		attachURLState(ctx, instance, u.String())
		if !instance.SetPrototype(state.urlPrototype) {
			instance.Free()
			return nil
		}
		return instance
	})
	if implementation == nil {
		proto.Free()
		return nil
	}
	ctor := makeConstructible(ctx, "__quickjs_url_impl__", "URL", implementation, proto)
	if ctor == nil {
		proto.Free()
		return nil
	}
	state.urlPrototype = proto
	defineMethod(ctx, proto, "toString", func(ctx *quickjs.Context, this *quickjs.Value, _ []*quickjs.Value) *quickjs.Value {
		return urlString(ctx, this)
	})
	defineMethod(ctx, proto, "toJSON", func(ctx *quickjs.Context, this *quickjs.Value, _ []*quickjs.Value) *quickjs.Value {
		return urlString(ctx, this)
	})
	proto.DefinePropertyValue("constructor", ctor, quickjs.PropConfigurable)
	defineMethod(ctx, ctor, "canParse", func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		value := this.CallConstructor(args...)
		if value == nil {
			return ctx.NewBool(false)
		}
		defer value.Free()
		if value.IsException() {
			_ = ctx.Exception()
			return ctx.NewBool(false)
		}
		return ctx.NewBool(true)
	})
	defineMethod(ctx, ctor, "parse", func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		value := this.CallConstructor(args...)
		if value == nil {
			return ctx.NewNull()
		}
		if value.IsException() {
			value.Free()
			_ = ctx.Exception()
			return ctx.NewNull()
		}
		return value
	})
	return ctor
}

func makeConstructible(ctx *quickjs.Context, temporaryName, constructorName string, implementation, prototype *quickjs.Value) *quickjs.Value {
	if implementation == nil || prototype == nil {
		return nil
	}
	globals := ctx.Globals()
	globals.Set(temporaryName, implementation)
	wrapper := ctx.Eval(`(function () {
		const implementation = globalThis["` + temporaryName + `"];
		return function ` + constructorName + `(...args) {
			return implementation(...args);
		};
	})()`)
	globals.Delete(temporaryName)
	if wrapper == nil || wrapper.IsException() {
		if wrapper != nil {
			wrapper.Free()
		}
		return nil
	}
	wrapper.Set("prototype", prototype)
	return wrapper
}

func installURLPrototype(ctx *quickjs.Context, proto *quickjs.Value, state *urlModuleState) {
	properties := []struct {
		name string
		get  func(*quickjs.Context, *quickjs.Value) *quickjs.Value
		set  func(*quickjs.Context, *quickjs.Value, *quickjs.Value) *quickjs.Value
	}{
		{"hash", urlHash, setURLHash},
		{"host", urlHost, setURLHost},
		{"hostname", urlHostname, setURLHostname},
		{"href", urlHref, setURLHref},
		{"origin", urlOrigin, nil},
		{"password", urlPassword, setURLPassword},
		{"pathname", urlPathname, setURLPathname},
		{"port", urlPort, setURLPort},
		{"protocol", urlProtocol, setURLProtocol},
		{"search", urlSearch, setURLSearch},
		{"searchParams", urlSearchParams, nil},
		{"username", urlUsername, setURLUsername},
	}
	for _, property := range properties {
		property := property
		getter := ctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, _ []*quickjs.Value) *quickjs.Value {
			return property.get(ctx, this)
		})
		var setter *quickjs.Value
		if property.set != nil {
			setter = ctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
				var value *quickjs.Value
				if len(args) > 0 {
					value = args[0]
				}
				return property.set(ctx, this, value)
			})
		}
		proto.DefinePropertyGetSet(property.name, getter, setter, quickjs.PropConfigurable)
		getter.Free()
		if setter != nil {
			setter.Free()
		}
	}
	_ = state
}

func newURLSearchParamsConstructor(ctx *quickjs.Context, state *urlModuleState) *quickjs.Value {
	proto := ctx.NewObject()
	if proto == nil {
		return nil
	}
	iteratorPrototype := newParamsIteratorPrototype(ctx)
	if iteratorPrototype == nil {
		proto.Free()
		return nil
	}
	installParamsPrototype(ctx, proto, iteratorPrototype)
	iteratorPrototype.Free()
	implementation := ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		query := ""
		if len(args) > 0 && args[0] != nil && !args[0].IsUndefined() {
			var failure *quickjs.Value
			query, failure = paramsInput(ctx, args[0])
			if failure != nil {
				return failure
			}
		}
		return newParamsObject(ctx, proto, nil, query)
	})
	if implementation == nil {
		proto.Free()
		return nil
	}
	ctor := makeConstructible(ctx, "__quickjs_urlsearchparams_impl__", "URLSearchParams", implementation, proto)
	if ctor == nil {
		proto.Free()
		return nil
	}
	state.paramsPrototype = proto
	proto.DefinePropertyValue("constructor", ctor, quickjs.PropConfigurable)
	return ctor
}

func installParamsPrototype(ctx *quickjs.Context, proto, iteratorPrototype *quickjs.Value) {
	defineMethod(ctx, proto, "append", paramsAppend)
	defineMethod(ctx, proto, "delete", paramsDelete)
	defineMethod(ctx, proto, "get", paramsGet)
	defineMethod(ctx, proto, "getAll", paramsGetAll)
	defineMethod(ctx, proto, "has", paramsHas)
	defineMethod(ctx, proto, "set", paramsSet)
	defineMethod(ctx, proto, "sort", paramsSort)
	defineMethod(ctx, proto, "toString", paramsToString)
	defineMethod(ctx, proto, "forEach", paramsForEach)
	defineMethod(ctx, proto, "keys", func(ctx *quickjs.Context, this *quickjs.Value, _ []*quickjs.Value) *quickjs.Value {
		return newParamsIterator(ctx, this, paramsIteratorKeys, proto)
	})
	defineMethod(ctx, proto, "values", func(ctx *quickjs.Context, this *quickjs.Value, _ []*quickjs.Value) *quickjs.Value {
		return newParamsIterator(ctx, this, paramsIteratorValues, proto)
	})
	entries := ctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, _ []*quickjs.Value) *quickjs.Value {
		return newParamsIterator(ctx, this, paramsIteratorEntries, proto)
	})
	proto.DefinePropertyValue("entries", entries, quickjs.PropConfigurable|quickjs.PropWritable)
	// Install Symbol.iterator using the same function value as entries.
	symbol := ctx.Globals().Get("Symbol")
	if symbol == nil {
		entries.Free()
		return
	}
	iterator := symbol.Get("iterator")
	symbol.Free()
	if iterator == nil {
		entries.Free()
		return
	}
	atom := ctx.AtomFromValue(iterator)
	iterator.Free()
	if atom == nil {
		entries.Free()
		return
	}
	proto.SetAtom(atom, entries)
	entries.Free()
	atom.Free()
	proto.DefinePropertyValue(paramsIteratorPrototypeProperty, iteratorPrototype, quickjs.PropConfigurable|quickjs.PropWritable)
	getter := ctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, _ []*quickjs.Value) *quickjs.Value {
		state := getParamsState(this)
		if state == nil {
			return throwURLInvalidThis(ctx, "Value of this must be of type URLSearchParams")
		}
		defer state.Free()
		return ctx.NewInt64(int64(len(paramsPairs(state))))
	})
	proto.DefinePropertyGetSet("size", getter, nil, quickjs.PropConfigurable)
	getter.Free()
}

func defineMethod(ctx *quickjs.Context, object *quickjs.Value, name string, fn func(*quickjs.Context, *quickjs.Value, []*quickjs.Value) *quickjs.Value) {
	value := ctx.NewFunction(fn)
	object.DefinePropertyValue(name, value, quickjs.PropConfigurable|quickjs.PropWritable)
	value.Free()
}

func attachURLState(ctx *quickjs.Context, object *quickjs.Value, href string) {
	state := ctx.NewObject()
	state.Set("href", ctx.NewString(href))
	object.DefinePropertyValue(urlStateProperty, state, quickjs.PropConfigurable|quickjs.PropWritable)
	state.Free()
}

func getURLState(object *quickjs.Value) *quickjs.Value {
	if object == nil || !object.IsObject() {
		return nil
	}
	state := object.Get(urlStateProperty)
	if state == nil || !state.IsObject() {
		if state != nil {
			state.Free()
		}
		return nil
	}
	return state
}

func urlString(ctx *quickjs.Context, object *quickjs.Value) *quickjs.Value {
	state := getURLState(object)
	if state == nil {
		return throwURLInvalidThis(ctx, "Value of this must be of type URL")
	}
	defer state.Free()
	href, _ := stateString(state, "href")
	return ctx.NewString(href)
}

func stateString(state *quickjs.Value, name string) (string, bool) {
	value := state.Get(name)
	if value == nil {
		return "", false
	}
	defer value.Free()
	if value.IsUndefined() {
		return "", false
	}
	return valueString(value), true
}

func updateURL(ctx *quickjs.Context, object *quickjs.Value, update func(*parsedURL)) *quickjs.Value {
	state := getURLState(object)
	if state == nil {
		return throwURLInvalidThis(ctx, "Value of this must be of type URL")
	}
	defer state.Free()
	href, ok := stateString(state, "href")
	if !ok {
		return throwURLInvalidThis(ctx, "Value of this must be of type URL")
	}
	u, err := parseURL(href, nil, false)
	if err != nil {
		return invalidURL(ctx, href, "Invalid URL")
	}
	update(u)
	state.Set("href", ctx.NewString(u.String()))
	return nil
}

func urlHash(ctx *quickjs.Context, object *quickjs.Value) *quickjs.Value {
	return urlComponent(ctx, object, func(u *parsedURL) string { return u.hash() })
}

func urlHost(ctx *quickjs.Context, object *quickjs.Value) *quickjs.Value {
	return urlComponent(ctx, object, func(u *parsedURL) string { return u.host() })
}

func urlHostname(ctx *quickjs.Context, object *quickjs.Value) *quickjs.Value {
	return urlComponent(ctx, object, func(u *parsedURL) string { return u.hostname() })
}

func urlHref(ctx *quickjs.Context, object *quickjs.Value) *quickjs.Value {
	return urlString(ctx, object)
}

func urlOrigin(ctx *quickjs.Context, object *quickjs.Value) *quickjs.Value {
	return urlComponent(ctx, object, func(u *parsedURL) string { return u.origin() })
}

func urlPassword(ctx *quickjs.Context, object *quickjs.Value) *quickjs.Value {
	return urlComponent(ctx, object, func(u *parsedURL) string { return u.password() })
}

func urlSearchParams(ctx *quickjs.Context, object *quickjs.Value) *quickjs.Value {
	state := getURLState(object)
	if state == nil {
		return throwURLInvalidThis(ctx, "Value of this must be of type URL")
	}
	defer state.Free()
	cached := state.Get(urlSearchParamsProperty)
	if cached != nil && cached.IsObject() {
		return cached
	}
	if cached != nil {
		cached.Free()
	}
	href, ok := stateString(state, "href")
	if !ok {
		return throwURLInvalidThis(ctx, "Value of this must be of type URL")
	}
	parsed, err := parseURL(href, nil, false)
	if err != nil {
		return invalidURL(ctx, href, "Invalid URL")
	}
	moduleBundle := cachedModule(ctx)
	if moduleBundle == nil {
		return ctx.ThrowTypeError("URLSearchParams is unavailable")
	}
	paramsCtor := moduleBundle.Get("URLSearchParams")
	moduleBundle.Free()
	if paramsCtor == nil {
		return ctx.ThrowTypeError("URLSearchParams is unavailable")
	}
	paramsProto := paramsCtor.Get("prototype")
	paramsCtor.Free()
	if paramsProto == nil {
		return ctx.ThrowTypeError("URLSearchParams prototype is unavailable")
	}
	result := newParamsObject(ctx, paramsProto, object, parsed.query())
	paramsProto.Free()
	if result == nil {
		return nil
	}
	state.DefinePropertyValue(urlSearchParamsProperty, result, quickjs.PropConfigurable|quickjs.PropWritable)
	return result
}

func urlPathname(ctx *quickjs.Context, object *quickjs.Value) *quickjs.Value {
	return urlComponent(ctx, object, func(u *parsedURL) string { return u.pathname() })
}

func urlPort(ctx *quickjs.Context, object *quickjs.Value) *quickjs.Value {
	return urlComponent(ctx, object, func(u *parsedURL) string { return u.port() })
}

func urlProtocol(ctx *quickjs.Context, object *quickjs.Value) *quickjs.Value {
	return urlComponent(ctx, object, func(u *parsedURL) string { return u.protocol() })
}

func urlSearch(ctx *quickjs.Context, object *quickjs.Value) *quickjs.Value {
	return urlComponent(ctx, object, func(u *parsedURL) string { return u.search() })
}

func urlUsername(ctx *quickjs.Context, object *quickjs.Value) *quickjs.Value {
	return urlComponent(ctx, object, func(u *parsedURL) string { return u.username() })
}

func urlComponent(ctx *quickjs.Context, object *quickjs.Value, component func(*parsedURL) string) *quickjs.Value {
	state := getURLState(object)
	if state == nil {
		return throwURLInvalidThis(ctx, "Value of this must be of type URL")
	}
	defer state.Free()
	href, ok := stateString(state, "href")
	if !ok {
		return throwURLInvalidThis(ctx, "Value of this must be of type URL")
	}
	u, err := parseURL(href, nil, false)
	if err != nil {
		return invalidURL(ctx, href, "Invalid URL")
	}
	return ctx.NewString(component(u))
}

func setURLHash(ctx *quickjs.Context, object, value *quickjs.Value) *quickjs.Value {
	return updateURL(ctx, object, func(u *parsedURL) { u.setHash(valueString(value)) })
}

func setURLHost(ctx *quickjs.Context, object, value *quickjs.Value) *quickjs.Value {
	return updateURL(ctx, object, func(u *parsedURL) { u.setHost(valueString(value)) })
}

func setURLHostname(ctx *quickjs.Context, object, value *quickjs.Value) *quickjs.Value {
	return updateURL(ctx, object, func(u *parsedURL) { u.setHostname(valueString(value)) })
}

func setURLHref(ctx *quickjs.Context, object, value *quickjs.Value) *quickjs.Value {
	text := valueString(value)
	u, err := parseURL(text, nil, true)
	if err != nil {
		return invalidURL(ctx, text, "Invalid URL")
	}
	state := getURLState(object)
	if state == nil {
		return throwURLInvalidThis(ctx, "Value of this must be of type URL")
	}
	defer state.Free()
	state.Set("href", ctx.NewString(u.String()))
	return nil
}

func setURLPassword(ctx *quickjs.Context, object, value *quickjs.Value) *quickjs.Value {
	return updateURL(ctx, object, func(u *parsedURL) { u.setPassword(valueString(value)) })
}

func setURLPathname(ctx *quickjs.Context, object, value *quickjs.Value) *quickjs.Value {
	return updateURL(ctx, object, func(u *parsedURL) { u.setPathname(valueString(value)) })
}

func setURLPort(ctx *quickjs.Context, object, value *quickjs.Value) *quickjs.Value {
	return updateURL(ctx, object, func(u *parsedURL) { u.setPort(valueString(value)) })
}

func setURLProtocol(ctx *quickjs.Context, object, value *quickjs.Value) *quickjs.Value {
	return updateURL(ctx, object, func(u *parsedURL) { u.setProtocol(valueString(value)) })
}

func setURLSearch(ctx *quickjs.Context, object, value *quickjs.Value) *quickjs.Value {
	return updateURL(ctx, object, func(u *parsedURL) { u.setSearch(valueString(value)) })
}

func setURLUsername(ctx *quickjs.Context, object, value *quickjs.Value) *quickjs.Value {
	return updateURL(ctx, object, func(u *parsedURL) { u.setUsername(valueString(value)) })
}

func valueString(value *quickjs.Value) string {
	if value == nil {
		return ""
	}
	if value.IsString() {
		var text string
		if json.Unmarshal([]byte(value.JSONStringify()), &text) == nil {
			return text
		}
	}
	return value.ToString()
}

func isSpecialScheme(scheme string) bool {
	switch strings.ToLower(scheme) {
	case "ftp", "file", "http", "https", "ws", "wss":
		return true
	default:
		return false
	}
}

func isNetworkScheme(scheme string) bool {
	switch strings.ToLower(scheme) {
	case "ftp", "http", "https", "ws", "wss":
		return true
	default:
		return false
	}
}

func defaultPort(scheme string, port int) bool {
	switch strings.ToLower(scheme) {
	case "ftp":
		return port == 21
	case "http", "ws":
		return port == 80
	case "https", "wss":
		return port == 443
	default:
		return false
	}
}

func invalidURL(ctx *quickjs.Context, input, message string) *quickjs.Value {
	value := nodeerrors.NewTypeError(ctx, nodeerrors.ErrCodeInvalidURL, "%s", message)
	if value == nil {
		return nil
	}
	value.Set("input", ctx.NewString(input))
	return ctx.Throw(value)
}

func throwURLTypeError(ctx *quickjs.Context, code, format string, args ...any) *quickjs.Value {
	return nodeerrors.ThrowTypeError(ctx, code, format, args...)
}

func throwURLInvalidThis(ctx *quickjs.Context, format string, args ...any) *quickjs.Value {
	return throwURLTypeError(ctx, nodeerrors.ErrCodeInvalidThis, format, args...)
}

func throwURLMissingArgs(ctx *quickjs.Context, format string, args ...any) *quickjs.Value {
	return throwURLTypeError(ctx, nodeerrors.ErrCodeMissingArgs, format, args...)
}

func paramsInput(ctx *quickjs.Context, value *quickjs.Value) (string, *quickjs.Value) {
	if value.IsString() {
		return strings.TrimPrefix(valueString(value), "?"), nil
	}
	if value.IsObject() {
		if isDOMExceptionPrototype(ctx, value) {
			return "", ctx.ThrowTypeError("Value of this must be of type DOMException")
		}
		if pairs, found, failure := paramsIterable(ctx, value); found {
			if failure != nil {
				return "", failure
			}
			return encodeParams(pairs), nil
		}
		pairs, failure := paramsRecord(ctx, value)
		if failure != nil {
			return "", failure
		}
		return encodeParams(pairs), nil
	}
	return valueString(value), nil
}

func isDOMExceptionPrototype(ctx *quickjs.Context, value *quickjs.Value) bool {
	constructor := ctx.Globals().Get("DOMException")
	if constructor == nil {
		return false
	}
	defer constructor.Free()
	prototype := constructor.Get("prototype")
	if prototype == nil {
		return false
	}
	defer prototype.Free()
	return value.StrictEqual(prototype)
}

func paramsRecord(ctx *quickjs.Context, value *quickjs.Value) ([]paramPair, *quickjs.Value) {
	object := ctx.Globals().Get("Object")
	if object == nil {
		return nil, ctx.ThrowInternalError("Object constructor is unavailable")
	}
	defer object.Free()
	keys := object.Get("keys")
	if keys == nil {
		return nil, ctx.ThrowInternalError("Object.keys is unavailable")
	}
	defer keys.Free()
	names := keys.Execute(object, value)
	if names == nil {
		return nil, ctx.ThrowInternalError("Object.keys failed")
	}
	if names.IsException() {
		return nil, names
	}
	defer names.Free()
	length := names.Get("length")
	if length == nil {
		return nil, ctx.ThrowInternalError("Object.keys returned an invalid value")
	}
	count := length.ToInt64()
	length.Free()
	reflect := ctx.Globals().Get("Reflect")
	if reflect == nil {
		return nil, ctx.ThrowInternalError("Reflect object is unavailable")
	}
	defer reflect.Free()
	get := reflect.Get("get")
	if get == nil {
		return nil, ctx.ThrowInternalError("Reflect.get is unavailable")
	}
	defer get.Free()

	pairs := make([]paramPair, 0, count)
	indexByName := make(map[string]int, count)
	for index := int64(0); index < count; index++ {
		nameValue := names.GetIdx(index)
		if nameValue == nil {
			return nil, ctx.ThrowInternalError("Object.keys returned an invalid property name")
		}
		name := valueString(nameValue)
		field := get.Execute(reflect, value, nameValue)
		nameValue.Free()
		if field == nil {
			return nil, ctx.ThrowInternalError("could not read parameter record property")
		}
		if field.IsException() {
			return nil, field
		}
		fieldValue := valueString(field)
		if pairIndex, found := indexByName[name]; found {
			pairs[pairIndex].value = fieldValue
		} else {
			indexByName[name] = len(pairs)
			pairs = append(pairs, paramPair{name: name, value: fieldValue})
		}
		field.Free()
	}
	return pairs, nil
}

func paramsIterable(ctx *quickjs.Context, value *quickjs.Value) ([]paramPair, bool, *quickjs.Value) {
	symbol := ctx.Globals().Get("Symbol")
	if symbol == nil {
		return nil, false, nil
	}
	defer symbol.Free()
	iteratorKey := symbol.Get("iterator")
	if iteratorKey == nil {
		return nil, false, nil
	}
	defer iteratorKey.Free()
	iteratorAtom := ctx.AtomFromValue(iteratorKey)
	if iteratorAtom == nil {
		return nil, false, nil
	}
	defer iteratorAtom.Free()
	method := value.GetAtom(iteratorAtom)
	if method == nil {
		return nil, false, nil
	}
	if method.IsException() {
		return nil, true, method
	}
	if method.IsUndefined() {
		method.Free()
		return nil, false, nil
	}
	if !method.IsFunction() {
		method.Free()
		return nil, true, nodeerrors.ThrowTypeError(ctx, nodeerrors.ErrCodeInvalidTuple, "parameter input is not iterable")
	}
	defer method.Free()
	iterator := method.Execute(value)
	if iterator == nil {
		return nil, true, nodeerrors.ThrowTypeError(ctx, nodeerrors.ErrCodeInvalidTuple, "parameter input is not iterable")
	}
	if iterator.IsException() {
		return nil, true, iterator
	}
	defer iterator.Free()

	pairs := make([]paramPair, 0)
	for {
		step := iterator.Call("next")
		if step == nil {
			return nil, true, nodeerrors.ThrowTypeError(ctx, nodeerrors.ErrCodeInvalidTuple, "parameter iterator failed")
		}
		if step.IsException() {
			return nil, true, step
		}
		if !step.IsObject() {
			step.Free()
			return nil, true, nodeerrors.ThrowTypeError(ctx, nodeerrors.ErrCodeInvalidTuple, "parameter iterator failed")
		}
		done := step.Get("done")
		if done == nil {
			step.Free()
			return nil, true, nodeerrors.ThrowTypeError(ctx, nodeerrors.ErrCodeInvalidTuple, "parameter iterator failed")
		}
		if done.IsException() {
			step.Free()
			return nil, true, done
		}
		isDone := done.ToBool()
		done.Free()
		if isDone {
			step.Free()
			break
		}
		entry := step.Get("value")
		step.Free()
		if entry == nil {
			return nil, true, nodeerrors.ThrowTypeError(ctx, nodeerrors.ErrCodeInvalidTuple, "parameter input must contain iterable name-value pairs")
		}
		pair, ok, failure := paramsTupleFromIterable(ctx, entry, iteratorAtom)
		entry.Free()
		if failure != nil {
			return nil, true, failure
		}
		if !ok {
			return nil, true, nodeerrors.ThrowTypeError(ctx, nodeerrors.ErrCodeInvalidTuple, "parameter input must contain iterable name-value pairs")
		}
		pairs = append(pairs, pair)
	}
	return pairs, true, nil
}

func paramsTupleFromIterable(ctx *quickjs.Context, entry *quickjs.Value, iteratorAtom *quickjs.Atom) (paramPair, bool, *quickjs.Value) {
	var pair paramPair
	method := entry.GetAtom(iteratorAtom)
	if method == nil {
		return pair, false, nil
	}
	if method.IsException() {
		return pair, true, method
	}
	if method.IsUndefined() {
		method.Free()
		return pair, false, nil
	}
	if !method.IsFunction() {
		method.Free()
		return pair, false, nil
	}
	iterator := method.Execute(entry)
	method.Free()
	if iterator == nil {
		return pair, true, nodeerrors.ThrowTypeError(ctx, nodeerrors.ErrCodeInvalidTuple, "parameter tuple iterator failed")
	}
	if iterator.IsException() {
		return pair, true, iterator
	}
	defer iterator.Free()

	count := 0
	for {
		step := iterator.Call("next")
		if step == nil {
			return pair, true, nodeerrors.ThrowTypeError(ctx, nodeerrors.ErrCodeInvalidTuple, "parameter tuple iterator failed")
		}
		if step.IsException() {
			return pair, true, step
		}
		if !step.IsObject() {
			step.Free()
			return pair, true, nodeerrors.ThrowTypeError(ctx, nodeerrors.ErrCodeInvalidTuple, "parameter tuple iterator failed")
		}
		done := step.Get("done")
		if done == nil {
			step.Free()
			return pair, true, nodeerrors.ThrowTypeError(ctx, nodeerrors.ErrCodeInvalidTuple, "parameter tuple iterator failed")
		}
		if done.IsException() {
			step.Free()
			return pair, true, done
		}
		isDone := done.ToBool()
		done.Free()
		if isDone {
			step.Free()
			break
		}
		if count >= 2 {
			step.Free()
			return pair, true, nodeerrors.ThrowTypeError(ctx, nodeerrors.ErrCodeInvalidTuple, "parameter input must contain name-value pairs")
		}
		item := step.Get("value")
		step.Free()
		if item == nil {
			return pair, true, nodeerrors.ThrowTypeError(ctx, nodeerrors.ErrCodeInvalidTuple, "parameter input must contain name-value pairs")
		}
		if item.IsException() {
			return pair, true, item
		}
		if count == 0 {
			pair.name = valueString(item)
		} else {
			pair.value = valueString(item)
		}
		item.Free()
		count++
	}
	if count != 2 {
		return pair, true, nodeerrors.ThrowTypeError(ctx, nodeerrors.ErrCodeInvalidTuple, "parameter input must contain name-value pairs")
	}
	return pair, true, nil
}

type paramPair struct {
	name  string
	value string
}

func parseParams(query string) []paramPair {
	if query == "" {
		return nil
	}
	parts := strings.Split(query, "&")
	result := make([]paramPair, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		name, value, hasValue := strings.Cut(part, "=")
		if !hasValue {
			value = ""
		}
		decodedName := unescapeSearchParam(name)
		decodedValue := unescapeSearchParam(value)
		result = append(result, paramPair{name: decodedName, value: decodedValue})
	}
	return result
}

func encodeParams(pairs []paramPair) string {
	var builder strings.Builder
	for i, pair := range pairs {
		if i > 0 {
			builder.WriteByte('&')
		}
		builder.WriteString(escapeSearchParam(pair.name))
		builder.WriteByte('=')
		builder.WriteString(escapeSearchParam(pair.value))
	}
	return builder.String()
}

func attachParamsState(ctx *quickjs.Context, object, owner *quickjs.Value, query string) {
	state := ctx.NewObject()
	state.Set("query", ctx.NewString(query))
	if owner != nil {
		state.DefinePropertyValue(paramsOwnerProperty, owner, quickjs.PropConfigurable|quickjs.PropWritable)
	}
	object.DefinePropertyValue(paramsStateProperty, state, quickjs.PropConfigurable|quickjs.PropWritable)
	state.Free()
}

func newParamsObject(ctx *quickjs.Context, proto, owner *quickjs.Value, query string) *quickjs.Value {
	object := ctx.NewObject()
	if object == nil {
		return nil
	}
	attachParamsState(ctx, object, owner, query)
	if !object.SetPrototype(proto) {
		object.Free()
		return nil
	}
	return object
}

func getParamsState(object *quickjs.Value) *quickjs.Value {
	if object == nil || !object.IsObject() {
		return nil
	}
	state := object.Get(paramsStateProperty)
	if state == nil || !state.IsObject() {
		if state != nil {
			state.Free()
		}
		return nil
	}
	return state
}

func paramsQuery(state *quickjs.Value) string {
	owner := state.Get(paramsOwnerProperty)
	if owner != nil && owner.IsObject() {
		urlState := getURLState(owner)
		if urlState != nil {
			href, _ := stateString(urlState, "href")
			urlState.Free()
			owner.Free()
			owner = nil
			if u, err := parseURL(href, nil, false); err == nil {
				return u.query()
			}
		}
	}
	if owner != nil {
		owner.Free()
	}
	query, _ := stateString(state, "query")
	return query
}

func setParamsQuery(ctx *quickjs.Context, state *quickjs.Value, query string) {
	owner := state.Get(paramsOwnerProperty)
	if owner != nil && owner.IsObject() {
		urlState := getURLState(owner)
		if urlState != nil {
			href, _ := stateString(urlState, "href")
			if u, err := parseURL(href, nil, false); err == nil {
				u.setSearch(query)
				urlState.Set("href", ctx.NewString(u.String()))
			}
			urlState.Free()
		}
		owner.Free()
		return
	}
	if owner != nil {
		owner.Free()
	}
	state.Set("query", ctx.NewString(query))
}

func paramsPairs(state *quickjs.Value) []paramPair {
	return parseParams(paramsQuery(state))
}

func paramsAppend(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	if len(args) < 2 {
		return throwURLMissingArgs(ctx, "The \"name\" and \"value\" arguments must be specified")
	}
	state := getParamsState(this)
	if state == nil {
		return throwURLInvalidThis(ctx, "Value of this must be of type URLSearchParams")
	}
	defer state.Free()
	pairs := paramsPairs(state)
	pairs = append(pairs, paramPair{name: valueString(args[0]), value: valueString(args[1])})
	setParamsQuery(ctx, state, encodeParams(pairs))
	return nil
}
func paramsDelete(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	state := getParamsState(this)
	if state == nil {
		return throwURLInvalidThis(ctx, "Value of this must be of type URLSearchParams")
	}
	defer state.Free()
	if len(args) == 0 {
		return throwURLMissingArgs(ctx, "The \"name\" argument must be specified")
	}
	name := valueString(args[0])
	value := ""
	hasValue := len(args) > 1 && args[1] != nil && !args[1].IsUndefined()
	if hasValue {
		value = valueString(args[1])
	}
	pairs := paramsPairs(state)
	filtered := pairs[:0]
	for _, pair := range pairs {
		remove := pair.name == name && (!hasValue || pair.value == value)
		if !remove {
			filtered = append(filtered, pair)
		}
	}
	setParamsQuery(ctx, state, encodeParams(filtered))
	return nil
}

func paramsGet(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	state := getParamsState(this)
	if state == nil {
		return throwURLInvalidThis(ctx, "Value of this must be of type URLSearchParams")
	}
	defer state.Free()
	if len(args) == 0 {
		return throwURLMissingArgs(ctx, "The \"name\" argument must be specified")
	}
	name := valueString(args[0])
	for _, pair := range paramsPairs(state) {
		if pair.name == name {
			return ctx.NewString(pair.value)
		}
	}
	return ctx.NewNull()
}

func paramsGetAll(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	state := getParamsState(this)
	if state == nil {
		return throwURLInvalidThis(ctx, "Value of this must be of type URLSearchParams")
	}
	defer state.Free()
	if len(args) == 0 {
		return throwURLMissingArgs(ctx, "The \"name\" argument must be specified")
	}
	name := valueString(args[0])
	values := make([]string, 0)
	for _, pair := range paramsPairs(state) {
		if pair.name == name {
			values = append(values, pair.value)
		}
	}
	result, err := ctx.Marshal(values)
	if err != nil {
		return ctx.ThrowTypeError("could not create values array: %s", err)
	}
	return result
}

func paramsHas(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	state := getParamsState(this)
	if state == nil {
		return throwURLInvalidThis(ctx, "Value of this must be of type URLSearchParams")
	}
	defer state.Free()
	if len(args) == 0 {
		return throwURLMissingArgs(ctx, "The \"name\" argument must be specified")
	}
	name := valueString(args[0])
	hasValue := len(args) > 1 && args[1] != nil && !args[1].IsUndefined()
	value := ""
	if hasValue {
		value = valueString(args[1])
	}
	for _, pair := range paramsPairs(state) {
		if pair.name == name && (!hasValue || pair.value == value) {
			return ctx.NewBool(true)
		}
	}
	return ctx.NewBool(false)
}

func paramsSet(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	state := getParamsState(this)
	if state == nil {
		return throwURLInvalidThis(ctx, "Value of this must be of type URLSearchParams")
	}
	defer state.Free()
	if len(args) < 2 {
		return throwURLMissingArgs(ctx, "The \"name\" and \"value\" arguments must be specified")
	}
	name, value := valueString(args[0]), valueString(args[1])
	pairs := paramsPairs(state)
	result := make([]paramPair, 0, len(pairs)+1)
	found := false
	for _, pair := range pairs {
		if pair.name == name {
			if !found {
				result = append(result, paramPair{name: name, value: value})
				found = true
			}
			continue
		}
		result = append(result, pair)
	}
	if !found {
		result = append(result, paramPair{name: name, value: value})
	}
	setParamsQuery(ctx, state, encodeParams(result))
	return nil
}

func compareUTF16(left, right string) int {
	leftOffset, rightOffset := 0, 0
	var leftTrailing, rightTrailing uint16
	leftHasTrailing, rightHasTrailing := false, false
	for {
		leftUnit, leftOK := nextUTF16CodeUnit(left, &leftOffset, &leftTrailing, &leftHasTrailing)
		rightUnit, rightOK := nextUTF16CodeUnit(right, &rightOffset, &rightTrailing, &rightHasTrailing)
		if !leftOK || !rightOK {
			switch {
			case leftOK:
				return 1
			case rightOK:
				return -1
			default:
				return 0
			}
		}
		switch {
		case leftUnit < rightUnit:
			return -1
		case leftUnit > rightUnit:
			return 1
		}
	}
}

func nextUTF16CodeUnit(input string, offset *int, trailing *uint16, hasTrailing *bool) (uint16, bool) {
	if *hasTrailing {
		*hasTrailing = false
		return *trailing, true
	}
	if *offset == len(input) {
		return 0, false
	}
	runeValue, size := utf8.DecodeRuneInString(input[*offset:])
	*offset += size
	if runeValue <= 0xFFFF {
		return uint16(runeValue), true
	}
	runeValue -= 0x10000
	*trailing = 0xDC00 + uint16(runeValue&0x3FF)
	*hasTrailing = true
	return 0xD800 + uint16(runeValue>>10), true
}

func paramsSort(ctx *quickjs.Context, this *quickjs.Value, _ []*quickjs.Value) *quickjs.Value {
	state := getParamsState(this)
	if state == nil {
		return throwURLInvalidThis(ctx, "Value of this must be of type URLSearchParams")
	}
	defer state.Free()
	pairs := paramsPairs(state)
	sort.SliceStable(pairs, func(i, j int) bool { return compareUTF16(pairs[i].name, pairs[j].name) < 0 })
	setParamsQuery(ctx, state, encodeParams(pairs))
	return nil
}

func paramsToString(ctx *quickjs.Context, this *quickjs.Value, _ []*quickjs.Value) *quickjs.Value {
	state := getParamsState(this)
	if state == nil {
		return throwURLInvalidThis(ctx, "Value of this must be of type URLSearchParams")
	}
	defer state.Free()
	return ctx.NewString(encodeParams(paramsPairs(state)))
}

func paramsForEach(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	state := getParamsState(this)
	if state == nil {
		return throwURLInvalidThis(ctx, "Value of this must be of type URLSearchParams")
	}
	defer state.Free()
	if len(args) == 0 || args[0] == nil || !args[0].IsFunction() {
		return throwURLTypeError(ctx, nodeerrors.ErrCodeInvalidArgType, "The callback argument must be a function")
	}
	var thisArg *quickjs.Value
	if len(args) > 1 && args[1] != nil {
		thisArg = args[1]
	} else {
		thisArg = ctx.NewUndefined()
		defer thisArg.Free()
	}
	for index := 0; ; index++ {
		pairs := paramsPairs(state)
		if index >= len(pairs) {
			break
		}
		pair := pairs[index]
		name := ctx.NewString(pair.name)
		value := ctx.NewString(pair.value)
		result := args[0].Execute(thisArg, value, name, this)
		name.Free()
		value.Free()
		if result == nil {
			return ctx.ThrowTypeError("URLSearchParams callback failed")
		}
		if result.IsException() {
			return result
		}
		result.Free()
	}
	return nil
}

type paramsIteratorKind int

const (
	paramsIteratorKeys paramsIteratorKind = iota
	paramsIteratorValues
	paramsIteratorEntries
)

func newParamsIteratorPrototype(ctx *quickjs.Context) *quickjs.Value {
	nativePrototype := ctx.Eval(`Object.getPrototypeOf(Object.getPrototypeOf([][Symbol.iterator]()))`)
	if nativePrototype == nil || nativePrototype.IsException() {
		if nativePrototype != nil {
			nativePrototype.Free()
		}
		return nil
	}
	prototype := ctx.NewObject()
	if prototype == nil || !prototype.SetPrototype(nativePrototype) {
		nativePrototype.Free()
		if prototype != nil {
			prototype.Free()
		}
		return nil
	}
	nativePrototype.Free()

	next := ctx.NewFunction(paramsIteratorNext)
	if next == nil {
		prototype.Free()
		return nil
	}
	prototype.DefinePropertyValue("next", next, quickjs.PropConfigurable|quickjs.PropWritable)
	next.Free()

	symbol := ctx.Globals().Get("Symbol")
	if symbol == nil {
		prototype.Free()
		return nil
	}
	tag := symbol.Get("toStringTag")
	symbol.Free()
	if tag == nil {
		prototype.Free()
		return nil
	}
	atom := ctx.AtomFromValue(tag)
	tag.Free()
	if atom == nil {
		prototype.Free()
		return nil
	}
	tagValue := ctx.NewString("URLSearchParams Iterator")
	ok := prototype.DefinePropertyAtom(atom, quickjs.PropertyDescriptor{
		Flags: quickjs.PropHasValue | quickjs.PropHasConfigurable | quickjs.PropConfigurable,
		Value: tagValue,
	})
	tagValue.Free()
	atom.Free()
	if !ok {
		prototype.Free()
		return nil
	}
	return prototype
}
func newParamsIterator(ctx *quickjs.Context, source *quickjs.Value, kind paramsIteratorKind, paramsPrototype *quickjs.Value) *quickjs.Value {
	if source == nil || paramsPrototype == nil {
		return throwURLInvalidThis(ctx, "Value of this must be of type URLSearchParams")
	}
	prototype := paramsPrototype.Get(paramsIteratorPrototypeProperty)
	if prototype == nil {
		return throwURLTypeError(ctx, nodeerrors.ErrCodeNotImplemented, "URLSearchParams iterator is unavailable")
	}
	defer prototype.Free()
	paramsState := getParamsState(source)
	if paramsState == nil {
		return throwURLInvalidThis(ctx, "Value of this must be of type URLSearchParams")
	}
	paramsState.Free()
	iteratorState := ctx.NewObject()
	if iteratorState == nil {
		return nil
	}
	iteratorState.DefinePropertyValue("source", source, quickjs.PropConfigurable|quickjs.PropWritable)
	iteratorState.Set("kind", ctx.NewInt32(int32(kind)))
	iteratorState.Set("index", ctx.NewInt64(0))

	iterator := ctx.NewObject()
	if iterator == nil {
		iteratorState.Free()
		return nil
	}
	iterator.DefinePropertyValue(paramsIteratorStateProperty, iteratorState, quickjs.PropConfigurable|quickjs.PropWritable)
	iteratorState.Free()
	if !iterator.SetPrototype(prototype) {
		iterator.Free()
		return nil
	}
	return iterator
}

func getParamsIteratorState(object *quickjs.Value) *quickjs.Value {
	if object == nil || !object.IsObject() {
		return nil
	}
	state := object.Get(paramsIteratorStateProperty)
	if state == nil || !state.IsObject() {
		if state != nil {
			state.Free()
		}
		return nil
	}
	return state
}

func paramsIteratorNext(ctx *quickjs.Context, this *quickjs.Value, _ []*quickjs.Value) *quickjs.Value {
	iteratorState := getParamsIteratorState(this)
	if iteratorState == nil {
		return throwURLInvalidThis(ctx, "Value of this must be of type URLSearchParams Iterator")
	}
	defer iteratorState.Free()

	source := iteratorState.Get("source")
	if source == nil {
		return throwURLInvalidThis(ctx, "Value of this must be of type URLSearchParams Iterator")
	}
	defer source.Free()
	paramsState := getParamsState(source)
	if paramsState == nil {
		return throwURLInvalidThis(ctx, "Value of this must be of type URLSearchParams Iterator")
	}
	defer paramsState.Free()

	indexValue := iteratorState.Get("index")
	index := int64(0)
	if indexValue != nil {
		index = indexValue.ToInt64()
		indexValue.Free()
	}
	kindValue := iteratorState.Get("kind")
	kind := paramsIteratorEntries
	if kindValue != nil {
		kind = paramsIteratorKind(kindValue.ToInt64())
		kindValue.Free()
	}
	pairs := paramsPairs(paramsState)
	result := ctx.NewObject()
	if result == nil {
		return nil
	}
	if index >= int64(len(pairs)) {
		result.Set("value", ctx.NewUndefined())
		result.Set("done", ctx.NewBool(true))
		return result
	}

	pair := pairs[index]
	switch kind {
	case paramsIteratorKeys:
		result.Set("value", ctx.NewString(pair.name))
	case paramsIteratorValues:
		result.Set("value", ctx.NewString(pair.value))
	default:
		entry, err := ctx.Marshal([]string{pair.name, pair.value})
		if err != nil {
			result.Free()
			return throwURLTypeError(ctx, nodeerrors.ErrCodeInvalidTuple, "could not create iterator entry: %s", err)
		}
		result.Set("value", entry)
	}
	result.Set("done", ctx.NewBool(false))
	iteratorState.Set("index", ctx.NewInt64(index+1))
	return result
}
