package quickjs_nodejs

import (
	"fmt"
	"sync"
	"testing"

	"github.com/Scardice/quickjs_nodejs/buffer"
	"github.com/Scardice/quickjs_nodejs/console"
	"github.com/Scardice/quickjs_nodejs/eventloop"
	"github.com/Scardice/quickjs_nodejs/module"
	"github.com/Scardice/quickjs_nodejs/process"
	"github.com/Scardice/quickjs_nodejs/url"
	"github.com/Scardice/quickjs_nodejs/util"
	quickjs "github.com/buke/quickjs-go"
)

type integrationPrinter struct {
	mu   sync.Mutex
	logs []string
}

func (p *integrationPrinter) Log(message string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.logs = append(p.logs, message)
}

func (*integrationPrinter) Warn(string)  {}
func (*integrationPrinter) Error(string) {}

func TestIntegration(t *testing.T) {
	const envKey = "QUICKJS_NODEJS_INTEGRATION"
	envSnapshot := map[string]string{envKey: "host"}
	printer := &integrationPrinter{}
	registry := module.NewRegistry()
	for _, definition := range []module.Definition{
		buffer.Module(),
		console.ModuleWithPrinter(printer),
		process.Module(process.WithEnvSnapshot(envSnapshot)),
		url.Module(),
		util.Module(),
	} {
		if err := registry.Add(definition); err != nil {
			t.Fatal(err)
		}
	}

	withoutGlobals, err := eventloop.New(eventloop.WithRegistry(registry))
	if err != nil {
		t.Fatal(err)
	}
	result := ""
	if err := withoutGlobals.Run(func(ctx *quickjs.Context) error {
		value := ctx.Eval(`typeof Buffer + ":" + typeof console + ":" + typeof process + ":" + typeof URL`)
		if value == nil {
			return fmt.Errorf("global smoke returned nil")
		}
		defer value.Free()
		if value.IsException() {
			return fmt.Errorf("global smoke failed: %v", ctx.Exception())
		}
		result = value.ToString()
		return nil
	}); err != nil {
		withoutGlobals.Close()
		t.Fatal(err)
	}
	if err := withoutGlobals.Close(); err != nil {
		t.Fatal(err)
	}
	if result != "undefined:undefined:undefined:undefined" {
		t.Fatalf("unexpected implicit globals %q", result)
	}

	withGlobals, err := eventloop.New(
		eventloop.WithRegistry(registry),
		eventloop.WithGlobals(
			buffer.InstallGlobal,
			func(ctx *quickjs.Context) error { return console.InstallGlobalWithPrinter(ctx, printer) },
			func(ctx *quickjs.Context) error {
				return process.InstallGlobal(ctx, process.WithEnvSnapshot(envSnapshot))
			},
			url.InstallGlobal,
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer withGlobals.Close()

	result = ""
	if err := withGlobals.Run(func(ctx *quickjs.Context) error {
		value := ctx.Eval(`(async () => {
			const b = await import("buffer");
			const bn = await import("node:buffer");
			const c = await import("console");
			const cn = await import("node:console");
			const p = await import("process");
			const pn = await import("node:process");
			const u = await import("url");
			const un = await import("node:url");
			const ut = await import("util");
			const utn = await import("node:util");
			const params = new u.URLSearchParams("a=1");
			params.append("b", "two words");
			c.log("integration", 1);
			return [
				b.Buffer.from("ok").toString("hex"),
				bn.Buffer.from("alias").toString(),
			typeof un.URL,
				new u.URL("https://example.com/x").hostname,
				params.toString(),
				ut.format("%s:%d", "ok", 7),
				utn.format("%s", "alias"),
				p.env.QUICKJS_NODEJS_INTEGRATION,
				pn.env.QUICKJS_NODEJS_INTEGRATION,
				typeof Buffer,
				typeof console.log,
				typeof process.env,
				typeof URLSearchParams,
				cn.error === c.error
			].join("|");
		})()`, quickjs.EvalAwait(true))
		if value == nil {
			return fmt.Errorf("module smoke returned nil")
		}
		defer value.Free()
		if value.IsException() {
			return fmt.Errorf("module smoke failed: %v", ctx.Exception())
		}
		result = value.ToString()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got, want := result, "6f6b|alias|function|example.com|a=1&b=two+words|ok:7|alias|host|host|function|function|object|function|true"; got != want {
		t.Fatalf("unexpected integration result %q, want %q", got, want)
	}

	printer.mu.Lock()
	defer printer.mu.Unlock()
	if len(printer.logs) != 1 || printer.logs[0] != "integration 1" {
		t.Fatalf("unexpected integration printer calls %#v", printer.logs)
	}
}
