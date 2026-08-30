//go:build conformance

package conformance

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	blobmodule "github.com/Scardice/quickjs_nodejs/blob"
	fetchmodule "github.com/Scardice/quickjs_nodejs/fetch"
	"github.com/Scardice/quickjs_nodejs/internal/testutil"
	urlmodule "github.com/Scardice/quickjs_nodejs/url"
	quickjs "github.com/buke/quickjs-go"
)

func TestWPTURLVector(t *testing.T) {
	root, err := suiteRoot("wpt")
	if err != nil {
		t.Fatal(err)
	}
	vector, err := firstWPTURLVector(filepath.Join(root, "url", "resources", "urltestdata.json"))
	if err != nil {
		t.Fatal(err)
	}

	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := urlmodule.InstallGlobal(ctx); err != nil {
			t.Fatal(err)
		}
		input, err := json.Marshal(vector.Input)
		if err != nil {
			t.Fatal(err)
		}
		base, err := json.Marshal(vector.Base)
		if err != nil {
			t.Fatal(err)
		}
		value := ctx.Eval(fmt.Sprintf("new URL(%s, %s).href", input, base))
		if value == nil {
			t.Fatal("WPT URL evaluation returned nil")
		}
		defer value.Free()
		if value.IsException() {
			t.Fatal(ctx.Exception())
		}
		if got := value.ToString(); got != vector.Href {
			t.Fatalf("WPT URL href = %q, want %q", got, vector.Href)
		}
	})
}

func TestTest262BigIntArithmetic(t *testing.T) {
	root, err := suiteRoot("test262")
	if err != nil {
		t.Fatal(err)
	}
	if err := runTest262Script(root, "test/language/expressions/addition/bigint-arithmetic.js"); err != nil {
		t.Fatal(err)
	}
}

func TestRunWPTHarnessCollectsResults(t *testing.T) {
	root, err := suiteRoot("wpt")
	if err != nil {
		t.Fatal(err)
	}

	result, err := runWPTHarness(root, "url/urlsearchparams-append.any.js", urlmodule.InstallGlobal)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete {
		t.Fatal("WPT harness did not complete")
	}
	if len(result.Tests) == 0 {
		t.Fatal("WPT harness reported no tests")
	}
}

func TestRunWPTHarnessProvidesLocation(t *testing.T) {
	root, err := suiteRoot("wpt")
	if err != nil {
		t.Fatal(err)
	}

	result, err := runWPTHarness(root, "url/url-setters.any.js", urlmodule.InstallGlobal)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete {
		t.Fatal("WPT harness did not complete")
	}
}

func TestRunWPTHarnessProvidesWPTGlobals(t *testing.T) {
	root, err := suiteRoot("wpt")
	if err != nil {
		t.Fatal(err)
	}

	result, err := runWPTHarness(root, "fetch/api/headers/header-values-normalize.any.js", func(ctx *quickjs.Context) error {
		return fetchmodule.InstallGlobal(ctx)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete {
		t.Fatal("WPT harness did not complete")
	}
}

func TestWPTBlobCore(t *testing.T) {
	root, err := suiteRoot("wpt")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"FileAPI/blob/Blob-constructor.any.js",
		"FileAPI/blob/Blob-slice.any.js",
		"FileAPI/blob/Blob-text.any.js",
		"FileAPI/blob/Blob-array-buffer.any.js",
		"FileAPI/blob/Blob-bytes.any.js",
	} {
		t.Run(path, func(t *testing.T) {
			result, err := runWPTHarness(root, path, blobmodule.InstallGlobal)
			if err != nil {
				t.Fatal(err)
			}
			for _, test := range result.Tests {
				if test.Status == 0 {
					continue
				}
				if path == "FileAPI/blob/Blob-constructor.any.js" && test.Name == "Passing a FrozenArray as the blobParts array should work (FrozenArray<MessagePort>)." {
					t.Log("skipped Blob constructor MessagePort transfer assertion: MessageChannel is not implemented")
					continue
				}
				t.Fatalf("%s: %s", test.Name, test.Message)
			}
		})
	}
}

func TestWPTHeadersSetCookie(t *testing.T) {
	root, err := suiteRoot("wpt")
	if err != nil {
		t.Fatal(err)
	}
	result, err := runWPTHarness(root, "fetch/api/headers/header-setcookie.any.js", func(ctx *quickjs.Context) error {
		return fetchmodule.InstallGlobal(ctx)
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range result.Tests {
		if test.Status != 0 {
			t.Fatalf("%s: %s", test.Name, test.Message)
		}
	}
}

func TestWPTHeadersCore(t *testing.T) {
	root, err := suiteRoot("wpt")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"fetch/api/headers/headers-basic.any.js",
		"fetch/api/headers/headers-casing.any.js",
		"fetch/api/headers/headers-combine.any.js",
		"fetch/api/headers/headers-errors.any.js",
		"fetch/api/headers/headers-normalize.any.js",
		"fetch/api/headers/headers-record.any.js",
		"fetch/api/headers/headers-structure.any.js",
	} {
		t.Run(path, func(t *testing.T) {
			result, err := runWPTHarness(root, path, func(ctx *quickjs.Context) error {
				return fetchmodule.InstallGlobal(ctx)
			})
			if err != nil {
				t.Fatal(err)
			}
			for _, test := range result.Tests {
				if test.Status != 0 {
					t.Fatalf("%s: %s", test.Name, test.Message)
				}
			}
		})
	}
}
