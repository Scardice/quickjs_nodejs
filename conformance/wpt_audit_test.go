//go:build conformance_audit

package conformance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	abortmodule "github.com/Scardice/quickjs_nodejs/abort"
	cryptomodule "github.com/Scardice/quickjs_nodejs/crypto"
	"github.com/Scardice/quickjs_nodejs/eventloop"
	fetchmodule "github.com/Scardice/quickjs_nodejs/fetch"
	"github.com/Scardice/quickjs_nodejs/internal/testutil"
	messagechannelmodule "github.com/Scardice/quickjs_nodejs/messagechannel"
	structuredclonemodule "github.com/Scardice/quickjs_nodejs/structuredclone"
	urlmodule "github.com/Scardice/quickjs_nodejs/url"
	quickjs "github.com/buke/quickjs-go"
)

func TestWPTImplementedAPIs(t *testing.T) {
	root, err := suiteRoot("wpt")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("URL", func(t *testing.T) {
		passed, failed, failures := runWPTURLData(t, root)
		passed, failed, failures = runWPTFiles(t, root, []string{
			"url/url-searchparams.any.js",
			"url/url-setters-stripping.any.js",
			"url/url-statics-canparse.any.js",
			"url/url-statics-parse.any.js",
			"url/url-tojson.any.js",
			"url/urlsearchparams-append.any.js",
			"url/urlsearchparams-constructor.any.js",
			"url/urlsearchparams-delete.any.js",
			"url/urlsearchparams-foreach.any.js",
			"url/urlsearchparams-get.any.js",
			"url/urlsearchparams-getall.any.js",
			"url/urlsearchparams-has.any.js",
			"url/urlsearchparams-set.any.js",
			"url/urlsearchparams-size.any.js",
			"url/urlsearchparams-sort.any.js",
			"url/urlsearchparams-stringifier.any.js",
		}, []eventloop.GlobalInstaller{func(ctx *quickjs.Context) error {
			return fetchmodule.InstallGlobal(ctx)
		}, urlmodule.InstallGlobal}, passed, failed, failures)
		reportWPTResults(t, passed, failed, failures)
	})

	t.Run("Fetch", func(t *testing.T) {
		passed, failed, failures := runWPTFiles(t, root, []string{
			"fetch/api/headers/header-setcookie.any.js",
			"fetch/api/headers/header-values-normalize.any.js",
			"fetch/api/headers/header-values.any.js",
			"fetch/api/headers/headers-basic.any.js",
			"fetch/api/headers/headers-casing.any.js",
			"fetch/api/headers/headers-combine.any.js",
			"fetch/api/headers/headers-errors.any.js",
			"fetch/api/headers/headers-forbidden-override.any.js",
			"fetch/api/headers/headers-no-cors.any.js",
			"fetch/api/headers/headers-normalize.any.js",
			"fetch/api/headers/headers-record.any.js",
			"fetch/api/headers/headers-structure.any.js",
			"fetch/api/request/forbidden-method.any.js",
			"fetch/api/request/request-consume-empty.any.js",
			"fetch/api/request/request-error.any.js",
			"fetch/api/request/request-headers.any.js",
			"fetch/api/request/request-init-002.any.js",
			"fetch/api/request/request-init-contenttype.any.js",
			"fetch/api/request/request-init-priority.any.js",
			"fetch/api/request/request-keepalive.any.js",
			"fetch/api/request/request-structure.any.js",
			"fetch/api/response/json.any.js",
			"fetch/api/response/response-consume-empty.any.js",
			"fetch/api/response/response-error.any.js",
			"fetch/api/response/response-headers-guard.any.js",
			"fetch/api/response/response-init-001.any.js",
			"fetch/api/response/response-init-002.any.js",
			"fetch/api/response/response-init-contenttype.any.js",
			"fetch/api/response/response-static-error.any.js",
			"fetch/api/response/response-static-json.any.js",
			"fetch/api/response/response-static-redirect.any.js",
			"url/urlencoded-parser.any.js",
		}, []eventloop.GlobalInstaller{cryptomodule.InstallGlobal, urlmodule.InstallGlobal, func(ctx *quickjs.Context) error {
			return fetchmodule.InstallGlobal(ctx)
		}}, 0, 0, nil)
		reportWPTResults(t, passed, failed, failures)
	})

	t.Run("Abort", func(t *testing.T) {
		passed, failed, failures := runWPTFiles(t, root, []string{
			"dom/abort/event.any.js",
		}, []eventloop.GlobalInstaller{cryptomodule.InstallGlobal, abortmodule.InstallGlobal}, 0, 0, nil)
		reportWPTResults(t, passed, failed, failures)
	})

	t.Run("structuredClone", func(t *testing.T) {
		passed, failed, failures := runWPTFiles(t, root, []string{
			"html/webappapis/structured-clone/structured-clone.any.js",
		}, []eventloop.GlobalInstaller{cryptomodule.InstallGlobal, structuredclonemodule.InstallGlobal, messagechannelmodule.InstallGlobal, func(ctx *quickjs.Context) error {
			return fetchmodule.InstallGlobal(ctx)
		}}, 0, 0, nil)
		reportWPTResults(t, passed, failed, failures)
	})

	t.Run("WebCrypto", func(t *testing.T) {
		t.Log("excluded WebCrypto CryptoKey/subtle cases: generating a key in the WPT harness triggers QuickJS JS_FreeRuntime's gc_obj_list assertion")
		passed, failed, failures := runWPTFiles(t, root, wptWebCryptoFiles(), []eventloop.GlobalInstaller{cryptomodule.InstallGlobal}, 0, 0, nil)
		reportWPTResults(t, passed, failed, failures)
	})
}

func runWPTURLData(t *testing.T, root string) (int, int, []string) {
	t.Helper()
	passed, failed := 0, 0
	failures := make([]string, 0)
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := urlmodule.InstallGlobal(ctx); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"urltestdata.json", "urltestdata-javascript-only.json"} {
			data, err := os.ReadFile(filepath.Join(root, "url", "resources", name))
			if err != nil {
				t.Fatal(err)
			}
			var entries []json.RawMessage
			if err := json.Unmarshal(data, &entries); err != nil {
				t.Fatal(err)
			}
			for index, entry := range entries {
				entry = json.RawMessage(strings.TrimSpace(string(entry)))
				if len(entry) == 0 || entry[0] == '"' {
					continue
				}
				result, err := evalWPTURLVector(ctx, entry)
				if err == nil && result == "" {
					passed++
					continue
				}
				failed++
				message := result
				if err != nil {
					message = err.Error()
				}
				appendWPTFailure(&failures, fmt.Sprintf("url/resources/%s[%d]: %s", name, index, message))
			}
		}
	})
	return passed, failed, failures
}

func evalWPTURLVector(ctx *quickjs.Context, entry json.RawMessage) (string, error) {
	value := ctx.Eval(fmt.Sprintf(`(() => {
		const expected = %s;
		const base = expected.base === null ? undefined : expected.base;
		try {
			const url = new URL(expected.input, base);
			if (expected.failure) return "expected TypeError";
			for (const property of ["href", "origin", "protocol", "username", "password", "host", "hostname", "port", "pathname", "search", "hash"]) {
				if (property in expected && url[property] !== expected[property]) return property + " = " + JSON.stringify(url[property]) + ", want " + JSON.stringify(expected[property]);
			}
			if ("searchParams" in expected && url.searchParams.toString() !== expected.searchParams) return "searchParams = " + JSON.stringify(url.searchParams.toString()) + ", want " + JSON.stringify(expected.searchParams);
			return "";
		} catch (error) {
			if (expected.failure && error instanceof TypeError) return "";
			return (error && error.name ? error.name + ": " : "") + String(error);
		}
	})()`, entry))
	if value == nil {
		return "", fmt.Errorf("WPT URL vector returned nil")
	}
	defer value.Free()
	if value.IsException() {
		return "", ctx.Exception()
	}
	return value.ToString(), nil
}

func runWPTFiles(t *testing.T, root string, files []string, installers []eventloop.GlobalInstaller, passed, failed int, failures []string) (int, int, []string) {
	t.Helper()
	for _, path := range files {
		result, err := runWPTHarness(root, path, installers...)
		if err != nil {
			failed++
			appendWPTFailure(&failures, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		for _, test := range result.Tests {
			if test.Status == 0 {
				passed++
				continue
			}
			failed++
			appendWPTFailure(&failures, fmt.Sprintf("%s: %s: %s", path, test.Name, test.Message))
		}
	}
	return passed, failed, failures
}

func wptWebCryptoFiles() []string {
	return []string{
		"WebCryptoAPI/getRandomValues.any.js",
		"WebCryptoAPI/randomUUID.https.any.js",
	}
}

func reportWPTResults(t *testing.T, passed, failed int, failures []string) {
	t.Helper()
	t.Logf("WPT: %d passed, %d failed", passed, failed)
	if failed == 0 {
		return
	}
	for _, failure := range failures {
		t.Log(failure)
	}
	t.Errorf("WPT conformance failures: %d", failed)
}

func appendWPTFailure(failures *[]string, failure string) {
	if len(*failures) < 25 {
		*failures = append(*failures, failure)
	}
}
