package quickjs_nodejs

import (
	"runtime"
	"strings"
	"testing"

	abortmodule "github.com/Scardice/quickjs_nodejs/abort"
	blobmodule "github.com/Scardice/quickjs_nodejs/blob"
	cryptomodule "github.com/Scardice/quickjs_nodejs/crypto"
	fetchmodule "github.com/Scardice/quickjs_nodejs/fetch"
	messagechannelmodule "github.com/Scardice/quickjs_nodejs/messagechannel"
	"github.com/Scardice/quickjs_nodejs/module"
	structuredclonemodule "github.com/Scardice/quickjs_nodejs/structuredclone"
	utilmodule "github.com/Scardice/quickjs_nodejs/util"

	quickjs "github.com/buke/quickjs-go"
)

func TestQuickJSSmoke(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	rt := quickjs.NewRuntime(quickjs.WithStrictOSThread(true))
	if rt == nil {
		t.Fatal("NewRuntime returned nil")
	}
	defer rt.Close()

	ctx := rt.NewContextWithOptions(quickjs.NoBootstrap())
	if ctx == nil {
		t.Fatal("NewContextWithOptions returned nil")
	}
	defer ctx.Close()

	value := ctx.Eval("1 + 2")
	if value == nil {
		t.Fatal("Eval returned nil")
	}
	defer value.Free()
	if value.ToInt32() != 3 {
		t.Fatalf("unexpected eval result: %v", value.ToInt32())
	}
}

func TestBuiltinRegistryOmitsSealSpecifiers(t *testing.T) {
	registry := module.NewRegistry()
	for _, definition := range []module.Definition{
		abortmodule.Module(),
		blobmodule.Module(),
		cryptomodule.Module(),
		fetchmodule.Module(),
		messagechannelmodule.Module(),
		structuredclonemodule.Module(),
		utilmodule.Module(),
	} {
		if err := registry.Add(definition); err != nil {
			t.Fatal(err)
		}
	}

	names := make(map[string]struct{})
	for _, name := range registry.Names() {
		if strings.HasPrefix(name, "@seal/") {
			t.Fatalf("registered obsolete Seal specifier %q", name)
		}
		names[name] = struct{}{}
	}
	for _, name := range []string{
		"abort", "node:abort",
		"blob", "node:blob",
		"crypto", "node:crypto",
		"fetch", "node:fetch",
		"messagechannel", "node:messagechannel",
		"structuredclone", "node:structuredclone",
		"util", "node:util",
	} {
		if _, ok := names[name]; !ok {
			t.Errorf("missing module specifier %q", name)
		}
	}
}
