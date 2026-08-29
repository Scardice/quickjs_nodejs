package console

import (
	"sync"
	"testing"

	"github.com/Scardice/quickjs_nodejs/internal/testutil"
	"github.com/Scardice/quickjs_nodejs/module"
	quickjs "github.com/buke/quickjs-go"
)

type recordingPrinter struct {
	mu     sync.Mutex
	logs   []string
	warns  []string
	errors []string
}

func (p *recordingPrinter) Log(message string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.logs = append(p.logs, message)
}

func (p *recordingPrinter) Warn(message string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.warns = append(p.warns, message)
}

func (p *recordingPrinter) Error(message string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.errors = append(p.errors, message)
}

func TestConsolePrinterAndESMExports(t *testing.T) {
	printer := &recordingPrinter{}
	registry := module.NewRegistry()
	if err := registry.Add(ModuleWithPrinter(printer)); err != nil {
		t.Fatal(err)
	}
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := registry.Register(ctx); err != nil {
			t.Fatal(err)
		}
		result := ctx.Eval(`(async () => {
			const m = await import("node:console");
			m.log("a", 1);
			m.info("b");
			m.debug("c");
			m.warn("d", {x: 1});
			m.error("e");
			m.default.log("f");
			return typeof m.console.log + ":" + typeof m.default.error;
		})()`, quickjs.EvalAwait(true))
		if result == nil {
			t.Fatal("console evaluation returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("console evaluation failed: %v", ctx.Exception())
		}
		if got := result.ToString(); got != "function:function" {
			t.Fatalf("unexpected console exports %q", got)
		}
	})

	printer.mu.Lock()
	defer printer.mu.Unlock()
	if got, want := printer.logs, []string{"a 1", "b", "c", "f"}; !equalStrings(got, want) {
		t.Fatalf("unexpected log calls %#v, want %#v", got, want)
	}
	if got, want := printer.warns, []string{"d [object Object]"}; !equalStrings(got, want) {
		t.Fatalf("unexpected warn calls %#v, want %#v", got, want)
	}
	if got, want := printer.errors, []string{"e"}; !equalStrings(got, want) {
		t.Fatalf("unexpected error calls %#v, want %#v", got, want)
	}
}

func TestConsoleGlobalInstallIsExplicit(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		before := ctx.Eval(`typeof console`)
		if got := before.ToString(); got != "undefined" {
			before.Free()
			t.Fatalf("console was installed implicitly as %q", got)
		}
		before.Free()

		printer := &recordingPrinter{}
		if err := InstallGlobalWithPrinter(ctx, printer); err != nil {
			t.Fatal(err)
		}
		result := ctx.Eval(`typeof console.log + ":" + typeof console.error`)
		defer result.Free()
		if got := result.ToString(); got != "function:function" {
			t.Fatalf("unexpected global console type %q", got)
		}
	})
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
