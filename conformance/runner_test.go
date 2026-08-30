//go:build conformance

package conformance

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

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
