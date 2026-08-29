// Package url provides URL and URLSearchParams bindings for QuickJS.
package url

import (
	"fmt"
	"math"
	"net"
	neturl "net/url"
	"path"
	"sort"
	"strconv"
	"strings"

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
			input = args[0].ToString()
		}
		var base *neturl.URL
		if len(args) > 1 && args[1] != nil && !args[1].IsUndefined() {
			baseHref := ""
			if baseState := getURLState(args[1]); baseState != nil {
				baseHref, _ = stateString(baseState, "href")
				baseState.Free()
			} else {
				baseHref = args[1].ToString()
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
	return value.ToString(), true
}

func updateURL(ctx *quickjs.Context, object *quickjs.Value, update func(*neturl.URL)) *quickjs.Value {
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
	normalizeURL(u)
	state.Set("href", ctx.NewString(u.String()))
	return nil
}

func urlHash(ctx *quickjs.Context, object *quickjs.Value) *quickjs.Value {
	return urlComponent(ctx, object, func(u *neturl.URL) string {
		if u.Fragment == "" {
			return ""
		}
		return "#" + u.EscapedFragment()
	})
}

func urlHost(ctx *quickjs.Context, object *quickjs.Value) *quickjs.Value {
	return urlComponent(ctx, object, func(u *neturl.URL) string { return u.Host })
}

func urlHostname(ctx *quickjs.Context, object *quickjs.Value) *quickjs.Value {
	return urlComponent(ctx, object, func(u *neturl.URL) string { return u.Hostname() })
}

func urlHref(ctx *quickjs.Context, object *quickjs.Value) *quickjs.Value {
	return urlString(ctx, object)
}

func urlOrigin(ctx *quickjs.Context, object *quickjs.Value) *quickjs.Value {
	return urlComponent(ctx, object, func(u *neturl.URL) string {
		if u.Scheme == "file" || u.Host == "" {
			return "null"
		}
		return u.Scheme + "://" + u.Hostname()
	})
}

func urlPassword(ctx *quickjs.Context, object *quickjs.Value) *quickjs.Value {
	return urlComponent(ctx, object, func(u *neturl.URL) string {
		if u.User == nil {
			return ""
		}
		password, _ := u.User.Password()
		return password
	})
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
	parsed, err := neturl.Parse(href)
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
	result := newParamsObject(ctx, paramsProto, object, parsed.RawQuery)
	paramsProto.Free()
	if result == nil {
		return nil
	}
	state.DefinePropertyValue(urlSearchParamsProperty, result, quickjs.PropConfigurable|quickjs.PropWritable)
	return result
}

func urlPathname(ctx *quickjs.Context, object *quickjs.Value) *quickjs.Value {
	return urlComponent(ctx, object, func(u *neturl.URL) string {
		result := u.EscapedPath()
		if result == "" && u.Host != "" && isSpecialScheme(u.Scheme) {
			return "/"
		}
		return result
	})
}

func urlPort(ctx *quickjs.Context, object *quickjs.Value) *quickjs.Value {
	return urlComponent(ctx, object, func(u *neturl.URL) string { return u.Port() })
}

func urlProtocol(ctx *quickjs.Context, object *quickjs.Value) *quickjs.Value {
	return urlComponent(ctx, object, func(u *neturl.URL) string { return u.Scheme + ":" })
}

func urlSearch(ctx *quickjs.Context, object *quickjs.Value) *quickjs.Value {
	return urlComponent(ctx, object, func(u *neturl.URL) string {
		if u.RawQuery == "" {
			return ""
		}
		return "?" + u.RawQuery
	})
}

func urlUsername(ctx *quickjs.Context, object *quickjs.Value) *quickjs.Value {
	return urlComponent(ctx, object, func(u *neturl.URL) string {
		if u.User == nil {
			return ""
		}
		return u.User.Username()
	})
}

func urlComponent(ctx *quickjs.Context, object *quickjs.Value, component func(*neturl.URL) string) *quickjs.Value {
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
	text := valueString(value)
	if strings.HasPrefix(text, "#") {
		text = text[1:]
	}
	return updateURL(ctx, object, func(u *neturl.URL) { u.Fragment = text })
}

func setURLHost(ctx *quickjs.Context, object, value *quickjs.Value) *quickjs.Value {
	text := valueString(value)
	return updateURL(ctx, object, func(u *neturl.URL) { u.Host = text })
}

func setURLHostname(ctx *quickjs.Context, object, value *quickjs.Value) *quickjs.Value {
	text := valueString(value)
	if strings.Contains(text, ":") {
		return nil
	}
	return updateURL(ctx, object, func(u *neturl.URL) {
		if _, err := neturl.ParseRequestURI(u.Scheme + "://" + text); err != nil {
			return
		}
		if port := u.Port(); port != "" {
			u.Host = text + ":" + port
		} else {
			u.Host = text
		}
	})
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
	text := valueString(value)
	return updateURL(ctx, object, func(u *neturl.URL) {
		username := ""
		if u.User != nil {
			username = u.User.Username()
		}
		u.User = neturl.UserPassword(username, text)
	})
}

func setURLPathname(ctx *quickjs.Context, object, value *quickjs.Value) *quickjs.Value {
	text := valueString(value)
	return updateURL(ctx, object, func(u *neturl.URL) { u.Path = text })
}

func setURLPort(ctx *quickjs.Context, object, value *quickjs.Value) *quickjs.Value {
	return updateURL(ctx, object, func(u *neturl.URL) {
		if u.Scheme == "file" {
			return
		}
		port, empty := urlPortValue(value)
		if empty {
			u.Host = u.Hostname()
			return
		}
		if port < 0 {
			return
		}
		host := u.Hostname()
		if defaultPort(u.Scheme, port) {
			u.Host = host
			return
		}
		u.Host = net.JoinHostPort(host, strconv.Itoa(port))
	})
}

func urlPortValue(value *quickjs.Value) (port int, empty bool) {
	port = -1
	if value == nil {
		return port, false
	}
	if value.IsNumber() {
		number := value.ToFloat64()
		if number < 0 {
			return 0, true
		}
		if number == math.Trunc(number) && number <= math.MaxUint16 {
			return int(number), false
		}
	}
	text := value.ToString()
	if text == "" {
		return 0, true
	}
	firstDigit := -1
	for index := 0; index < len(text); index++ {
		if text[index] >= '0' && text[index] <= '9' {
			firstDigit = index
			break
		}
	}
	if firstDigit < 0 {
		return -1, false
	}
	if firstDigit > 0 {
		return 0, true
	}
	port = 0
	for index := firstDigit; index < len(text); index++ {
		char := text[index]
		if char < '0' || char > '9' {
			break
		}
		port = port*10 + int(char-'0')
		if port > math.MaxUint16 {
			return -1, false
		}
	}
	return port, false
}

func setURLProtocol(ctx *quickjs.Context, object, value *quickjs.Value) *quickjs.Value {
	text := valueString(value)
	if position := strings.IndexByte(text, ':'); position >= 0 {
		text = text[:position]
	}
	text = strings.ToLower(text)
	return updateURL(ctx, object, func(u *neturl.URL) {
		if isSpecialScheme(text) != isSpecialScheme(u.Scheme) {
			return
		}
		if _, err := neturl.ParseRequestURI(text + "://" + u.Host); err == nil {
			u.Scheme = text
		}
	})
}

func setURLSearch(ctx *quickjs.Context, object, value *quickjs.Value) *quickjs.Value {
	text := valueString(value)
	text = strings.TrimPrefix(text, "?")
	return updateURL(ctx, object, func(u *neturl.URL) { u.RawQuery = text })
}

func setURLUsername(ctx *quickjs.Context, object, value *quickjs.Value) *quickjs.Value {
	text := valueString(value)
	return updateURL(ctx, object, func(u *neturl.URL) {
		password, hasPassword := "", false
		if u.User != nil {
			password, hasPassword = u.User.Password()
		}
		if hasPassword {
			u.User = neturl.UserPassword(text, password)
		} else {
			u.User = neturl.User(text)
		}
	})
}

func valueString(value *quickjs.Value) string {
	if value == nil {
		return ""
	}
	return value.ToString()
}

func parseURL(input string, base *neturl.URL, requireAbsolute bool) (*neturl.URL, error) {
	input = strings.TrimSpace(input)
	parsed, err := neturl.Parse(input)
	if err != nil {
		return nil, err
	}
	if base != nil {
		parsed = base.ResolveReference(parsed)
	}
	if requireAbsolute && !parsed.IsAbs() {
		return nil, fmt.Errorf("URL is not absolute")
	}
	if parsed.Scheme == "" {
		return nil, fmt.Errorf("URL has no scheme")
	}
	normalizeURL(parsed)
	if isNetworkScheme(parsed.Scheme) && parsed.Host == "" {
		return nil, fmt.Errorf("URL has no hostname")
	}
	return parsed, nil
}

func normalizeURL(u *neturl.URL) {
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Path != "" {
		if !strings.HasPrefix(u.Path, "/") && (isSpecialScheme(u.Scheme) || u.Path != "") {
			u.Path = "/" + u.Path
		}
		cleaned := path.Clean(u.Path)
		if strings.HasSuffix(u.Path, "/") && !strings.HasSuffix(cleaned, "/") {
			cleaned += "/"
		}
		u.Path = cleaned
		u.RawPath = ""
	} else if u.Scheme == "file" || (isSpecialScheme(u.Scheme) && u.Host != "") {
		u.Path = "/"
	}
	if isNetworkScheme(u.Scheme) && u.Host != "" {
		host := u.Hostname()
		if ascii, err := idna.ToASCII(strings.ToLower(host)); err == nil {
			host = ascii
		}
		port := u.Port()
		if port != "" {
			p, err := strconv.Atoi(port)
			if err == nil && defaultPort(u.Scheme, p) {
				port = ""
			}
		}
		if strings.Contains(host, ":") {
			u.Host = net.JoinHostPort(host, port)
		} else if port != "" {
			u.Host = host + ":" + port
		} else {
			u.Host = host
		}
	}
	u.RawQuery = escapeURLQuery(u.RawQuery)
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
		return strings.TrimPrefix(value.ToString(), "?"), nil
	}
	if value.IsObject() {
		if pairs, found, failure := paramsIterable(ctx, value); found {
			if failure != nil {
				return "", failure
			}
			return encodeParams(pairs), nil
		}
		names, err := value.PropertyNames()
		if err == nil {
			pairs := make([]paramPair, 0, len(names))
			for _, name := range names {
				field := value.Get(name)
				if field != nil {
					pairs = append(pairs, paramPair{name: name, value: field.ToString()})
					field.Free()
				}
			}
			return encodeParams(pairs), nil
		}
	}
	return value.ToString(), nil
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
			pair.name = item.ToString()
		} else {
			pair.value = item.ToString()
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
	query = strings.TrimPrefix(query, "?")
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
			if u, err := neturl.Parse(href); err == nil {
				return u.RawQuery
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
			if u, err := neturl.Parse(href); err == nil {
				u.RawQuery = query
				normalizeURL(u)
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
	pairs = append(pairs, paramPair{name: args[0].ToString(), value: args[1].ToString()})
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
	name := args[0].ToString()
	value := ""
	hasValue := len(args) > 1 && args[1] != nil && !args[1].IsUndefined()
	if hasValue {
		value = args[1].ToString()
	}
	pairs := paramsPairs(state)
	filtered := pairs[:0]
	for _, pair := range pairs {
		remove := pair.name == name && (len(args) == 1 || hasValue && pair.value == value)
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
	name := args[0].ToString()
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
	name := args[0].ToString()
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
	name := args[0].ToString()
	hasValue := len(args) > 1 && args[1] != nil && !args[1].IsUndefined()
	value := ""
	if hasValue {
		value = args[1].ToString()
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
	name, value := args[0].ToString(), args[1].ToString()
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

func paramsSort(ctx *quickjs.Context, this *quickjs.Value, _ []*quickjs.Value) *quickjs.Value {
	state := getParamsState(this)
	if state == nil {
		return throwURLInvalidThis(ctx, "Value of this must be of type URLSearchParams")
	}
	defer state.Free()
	pairs := paramsPairs(state)
	sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].name < pairs[j].name })
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
