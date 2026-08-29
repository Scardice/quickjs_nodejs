package url

import (
	"testing"

	"github.com/Scardice/quickjs_nodejs/internal/testutil"
	"github.com/Scardice/quickjs_nodejs/module"
	quickjs "github.com/buke/quickjs-go"
)

func registerURL(t *testing.T, ctx *quickjs.Context) {
	t.Helper()
	registry := module.NewRegistry()
	if err := registry.Add(Module()); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(ctx); err != nil {
		t.Fatal(err)
	}
}

func evalURL(t *testing.T, ctx *quickjs.Context, source string) *quickjs.Value {
	t.Helper()
	value := ctx.Eval(`(async () => {
		const m = await import("node:url");
		`+source+`
	})()`, quickjs.EvalAwait(true))
	if value == nil {
		t.Fatal("url evaluation returned nil")
	}
	if value.IsException() {
		t.Fatalf("url evaluation failed: %v", ctx.Exception())
	}
	return value
}

func TestURLPropertiesAndResolution(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		registerURL(t, ctx)
		result := evalURL(t, ctx, `
			const { URL } = m;
			const u = new URL("https://example.com/a/../b?x=1#frag");
			const relative = new URL("child?q=2", u);
			return [u.href, u.protocol, u.hostname, u.pathname, u.search, u.hash, relative.href, u.toString(), u.toJSON(), u.searchParams === u.searchParams].join("|");
		`)
		defer result.Free()
		if got := result.ToString(); got != "https://example.com/b?x=1#frag|https:|example.com|/b|?x=1|#frag|https://example.com/child?q=2|https://example.com/b?x=1#frag|https://example.com/b?x=1#frag|true" {
			t.Fatalf("unexpected URL result %q", got)
		}
	})
}

func TestURLBaseStringAndIterableSearchParams(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		registerURL(t, ctx)
		result := evalURL(t, ctx, `
			const relative = new m.URL("/child?q=a+b&x=%2A", "https://example.com");
			const params = new m.URLSearchParams(new Map([["a", "1"], ["b", "two words"]]));
			const nested = new m.URLSearchParams(new Set([new Set(["c", "3"])]));
			const stringTuple = new m.URLSearchParams(["xy"]);
			return [relative.href, params.toString(), params.get("b"), nested.toString(), stringTuple.toString()].join("|");
		`)
		defer result.Free()
		if got := result.ToString(); got != "https://example.com/child?q=a+b&x=%2A|a=1&b=two+words|two words|c=3|x=y" {
			t.Fatalf("unexpected URL base/params result %q", got)
		}
	})
}

func TestURLOriginAndHostnameSetter(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		registerURL(t, ctx)
		result := evalURL(t, ctx, `
			const u = new m.URL("https://example.com:8080/path");
			u.hostname = "example.org:9000";
			const unchanged = u.href;
			u.hostname = "münich.example";
			return [unchanged, u.hostname, u.port, u.origin].join("|");
		`)
		defer result.Free()
		if got := result.ToString(); got != "https://example.com:8080/path|xn--mnich-kva.example|8080|https://xn--mnich-kva.example" {
			t.Fatalf("unexpected URL origin/hostname result %q", got)
		}
	})
}

func TestURLReferenceNormalization(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		registerURL(t, ctx)
		result := evalURL(t, ctx, `
			const { URL } = m;
			const same = (actual, expected) => {
				if (actual !== expected) throw new Error(actual + " != " + expected);
			};
			same(new URL("HTTP://test.com").toString(), "http://test.com/");
			same(new URL("HTTPS://á.com").toString(), "https://xn--1ca.com/");
			same(new URL("https://test.com#asdfá").toString(), "https://test.com/#asdf%C3%A1");
			same(new URL("https://test.com/?a=1 /2").toString(), "https://test.com/?a=1%20/2");
			same(new URL("file:///./abc").pathname, "/abc");
			same(new URL("file://").href, "file:///");
			const u = new URL("https://example.org:8888/foo");
			u.port = "443";
			same(u.port, "");
			u.port = "5678abcd";
			same(u.port, "5678");
			u.port = "a5678abcd";
			same(u.port, "");
			u.protocol = "ftp: blah";
			same(u.protocol, "ftp:");
			u.protocol = "foo";
			same(u.protocol, "ftp:");
			u.search = "a=#b";
			same(u.search, "?a=%23b");
			let invalidCode = "";
			try { new URL("http://"); } catch (e) { invalidCode = e.code; }
			let invalidPortCode = "";
			try { new URL("ssh://EEE:ddd"); } catch (e) { invalidPortCode = e.code; }
			same(invalidPortCode, "ERR_INVALID_URL");
			same(invalidCode, "ERR_INVALID_URL");
			same(new URL("foo:Example.com/", "https://example.org/").toString(), "foo:Example.com/");
			same(new URL("foo://Example.com/", "https://example.org/").toString(), "foo://Example.com/");
			same(new URL("fish://host/.").pathname, "/");
			same(new URL("fish://host/a/../b").pathname, "/b");
			same(new URL("otpauth://totp").toString(), "otpauth://totp");
			const otp = new URL("otpauth://totp");
			otp.pathname = "domain.com Domain:user@domain.com";
			same(otp.toString(), "otpauth://totp/domain.com%20Domain:user@domain.com");
			const { domainToASCII, domainToUnicode } = m;
			same(domainToASCII("xn--iñvalid.com"), "");
			same(domainToUnicode("xn--iñvalid.com"), "");
			return true;
		`)
		defer result.Free()
		if !result.ToBool() {
			t.Fatal("normalization script returned false")
		}
	})
}
func TestURLSearchParamsMutationAndIteration(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		registerURL(t, ctx)
		result := evalURL(t, ctx, `
			const { URL, URLSearchParams, domainToASCII, domainToUnicode } = m;
			const u = new URL("https://example.com/?a=1&a=2");
			u.searchParams.append("space", "hello world");
			u.searchParams.set("a", "3");
			const entries = Array.from(u.searchParams.entries()).map(pair => pair.join("=")).join("&");
			const iteratorTag = u.searchParams.entries().toString();
			const standalone = new URLSearchParams({z: "last", a: "first"});
			standalone.sort();
			const deleteUndefined = new URLSearchParams("a=1&a=2");
			deleteUndefined.delete("a", undefined);
			return [u.search, u.searchParams.getAll("a").join(","), entries, standalone.toString(), domainToASCII("münich.example"), domainToUnicode("xn--mnich-kva.example"), iteratorTag, deleteUndefined.toString()].join("|");
		`)
		defer result.Free()
		if got := result.ToString(); got != "?a=3&space=hello+world|3|a=3&space=hello world|a=first&z=last|xn--mnich-kva.example|münich.example|[object URLSearchParams Iterator]|a=1&a=2" {
			t.Fatalf("unexpected URLSearchParams result %q", got)
		}
	})
}

func TestURLSearchParamsTupleValidationAndSync(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		registerURL(t, ctx)
		result := evalURL(t, ctx, `
			const params = new m.URLSearchParams("a=b&cc=d");
			let visited = [];
			params.forEach((value, name, owner) => {
				visited.push(name + ":" + value);
				if (name === "a") owner.set("cc", "d1");
			});
			let tupleCode = "";
			try { new m.URLSearchParams([["only-one"]]); } catch (e) { tupleCode = e.code; }
			const u = new m.URL("https://example.com/");
			const linked = u.searchParams;
			u.search = "x=1";
			let invalidThisCode = "";
			try { params.forEach.call(1, 2); } catch (e) { invalidThisCode = e.code; }
			const invalidMethodCodes = ["delete", "get", "getAll", "has", "set"].map(name => {
				try {
					m.URLSearchParams.prototype[name].call(1);
				} catch (e) {
					return e.code;
				}
				return "";
			}).join(",");
			return [params.entries === params[Symbol.iterator], visited.join(","), params.toString(), tupleCode, linked.size, linked.get("x"), invalidThisCode, invalidMethodCodes].join("|");
		`)
		defer result.Free()
		if got := result.ToString(); got != "true|a:b,cc:d1|a=b&cc=d1|ERR_INVALID_TUPLE|1|1|ERR_INVALID_THIS|ERR_INVALID_THIS,ERR_INVALID_THIS,ERR_INVALID_THIS,ERR_INVALID_THIS,ERR_INVALID_THIS" {
			t.Fatalf("unexpected URLSearchParams tuple/sync result %q", got)
		}
	})
}

func TestURLGlobalInstaller(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := InstallGlobal(ctx); err != nil {
			t.Fatal(err)
		}
		result := ctx.Eval(`new URL("https://example.com").hostname + ":" + (typeof URLSearchParams)`, quickjs.EvalAwait(false))
		if result == nil {
			t.Fatal("global evaluation returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("global evaluation failed: %v", ctx.Exception())
		}
		if got := result.ToString(); got != "example.com:function" {
			t.Fatalf("unexpected global result %q", got)
		}
	})
}
