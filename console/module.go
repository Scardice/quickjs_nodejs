// Package console provides Node-style console logging for QuickJS.
package console

import (
	"log"
	"os"

	"github.com/Scardice/quickjs_nodejs/module"
	"github.com/Scardice/quickjs_nodejs/util"
	quickjs "github.com/buke/quickjs-go"
)

const ModuleName = "console"

// Printer receives already-formatted console messages.
type Printer interface {
	Log(string)
	Warn(string)
	Error(string)
}

// StdPrinter adapts output functions to Printer.
type StdPrinter struct {
	StdoutPrint func(string)
	StderrPrint func(string)
}

func (p StdPrinter) Log(message string) {
	if p.StdoutPrint != nil {
		p.StdoutPrint(message)
	}
}

func (p StdPrinter) Warn(message string) {
	if p.StderrPrint != nil {
		p.StderrPrint(message)
	}
}

func (p StdPrinter) Error(message string) {
	if p.StderrPrint != nil {
		p.StderrPrint(message)
	}
}

var (
	stdoutLogger = log.New(os.Stdout, "", log.LstdFlags)
	warnLogger   = log.New(os.Stderr, "WARN ", log.LstdFlags)
	errorLogger  = log.New(os.Stderr, "ERROR ", log.LstdFlags)
)

func selectedPrinter(printer Printer) Printer {
	if printer != nil {
		return printer
	}
	return defaultStdPrinter{}
}

type defaultStdPrinter struct{}

func (defaultStdPrinter) Log(message string) {
	stdoutLogger.Print(message)
}

func (defaultStdPrinter) Warn(message string) {
	warnLogger.Print(message)
}

func (defaultStdPrinter) Error(message string) {
	errorLogger.Print(message)
}

func newConsole(ctx *quickjs.Context, printer Printer) *quickjs.Value {
	object := ctx.NewObject()
	object.Set("log", newLogger(ctx, printer, printer.Log))
	object.Set("info", newLogger(ctx, printer, printer.Log))
	object.Set("debug", newLogger(ctx, printer, printer.Log))
	object.Set("warn", newLogger(ctx, printer, printer.Warn))
	object.Set("error", newLogger(ctx, printer, printer.Error))
	return object
}

func newLogger(ctx *quickjs.Context, _ Printer, output func(string)) *quickjs.Value {
	return ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) (result *quickjs.Value) {
		message := util.Format(ctx, args)
		defer func() {
			if recover() != nil {
				result = ctx.ThrowInternalError("console printer panicked")
			}
		}()
		output(message)
		return ctx.NewUndefined()
	})
}

// Module returns the console and node:console ESM definition using the default printer.
func Module() module.Definition {
	return ModuleWithPrinter(nil)
}

// ModuleWithPrinter returns a console ESM definition using printer.
func ModuleWithPrinter(printer Printer) module.Definition {
	printer = selectedPrinter(printer)
	return module.Definition{
		Name:    ModuleName,
		Aliases: []string{"node:" + ModuleName},
		Exports: []module.Export{
			{Name: "log", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return newLogger(ctx, printer, printer.Log), nil
			}}},
			{Name: "info", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return newLogger(ctx, printer, printer.Log), nil
			}}},
			{Name: "debug", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return newLogger(ctx, printer, printer.Log), nil
			}}},
			{Name: "warn", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return newLogger(ctx, printer, printer.Warn), nil
			}}},
			{Name: "error", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return newLogger(ctx, printer, printer.Error), nil
			}}},
			{Name: "console", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return newConsole(ctx, printer), nil
			}}},
			{Name: "default", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return newConsole(ctx, printer), nil
			}}},
		},
	}
}

// InstallGlobal installs the default console on globalThis.console.
func InstallGlobal(ctx *quickjs.Context) error {
	return InstallGlobalWithPrinter(ctx, nil)
}

// InstallGlobalWithPrinter installs a console using printer on globalThis.console.
func InstallGlobalWithPrinter(ctx *quickjs.Context, printer Printer) error {
	if ctx == nil {
		return os.ErrInvalid
	}
	ctx.Globals().Set("console", newConsole(ctx, selectedPrinter(printer)))
	return nil
}
