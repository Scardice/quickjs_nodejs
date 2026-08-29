// Package process provides the explicit process.env ESM module.
package process

import (
	"fmt"

	"github.com/Scardice/quickjs_nodejs/module"
	quickjs "github.com/buke/quickjs-go"
)

const ModuleName = "process"

type EnvProvider func() map[string]string

type Config struct {
	Env EnvProvider
}

type Option func(*Config)

func WithEnvProvider(provider EnvProvider) Option {
	return func(config *Config) { config.Env = provider }
}

func WithEnvSnapshot(values map[string]string) Option {
	snapshot := cloneEnvironment(values)
	return WithEnvProvider(func() map[string]string { return cloneEnvironment(snapshot) })
}

func applyOptions(options []Option) Config {
	config := Config{}
	for _, option := range options {
		if option != nil {
			option(&config)
		}
	}
	return config
}

func cloneEnvironment(values map[string]string) map[string]string {
	copyValues := make(map[string]string, len(values))
	for key, value := range values {
		copyValues[key] = value
	}
	return copyValues
}

func envValue(ctx *quickjs.Context, provider EnvProvider) (*quickjs.Value, error) {
	values := map[string]string{}
	if provider != nil {
		values = cloneEnvironment(provider())
	}
	value, err := ctx.Marshal(values)
	if err != nil {
		return nil, err
	}
	return value, nil
}

func processValue(ctx *quickjs.Context, provider EnvProvider) (*quickjs.Value, error) {
	env, err := envValue(ctx, provider)
	if err != nil {
		return nil, err
	}
	object := ctx.NewObject()
	if object == nil {
		env.Free()
		return nil, fmt.Errorf("create process object")
	}
	object.Set("env", env)
	return object, nil
}

func Module(options ...Option) module.Definition {
	config := applyOptions(options)
	return module.Definition{
		Name:    ModuleName,
		Aliases: []string{"node:" + ModuleName},
		Exports: []module.Export{
			{Name: "default", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return processValue(ctx, config.Env)
			}}},
			{Name: "env", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return envValue(ctx, config.Env)
			}}},
		},
	}
}

func InstallGlobal(ctx *quickjs.Context, options ...Option) error {
	if ctx == nil {
		return fmt.Errorf("process: nil context")
	}
	process, err := processValue(ctx, applyOptions(options).Env)
	if err != nil {
		return err
	}
	ctx.Globals().Set("process", process)
	return nil
}
