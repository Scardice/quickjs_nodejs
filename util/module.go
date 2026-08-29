// Package util provides a small Node-compatible utility module for QuickJS.
package util

import (
	"strings"

	"github.com/Scardice/quickjs_nodejs/module"
	quickjs "github.com/buke/quickjs-go"
)

const ModuleName = "util"

// Format applies the format-string rules used by util.format.
func Format(ctx *quickjs.Context, args []*quickjs.Value) string {
	if len(args) == 0 {
		return ""
	}
	format := ""
	if args[0] != nil && !args[0].IsUndefined() {
		format = args[0].ToString()
	}
	if len(args) == 1 {
		return format
	}

	var out strings.Builder
	argIndex := 1
	percent := false
	for _, char := range format {
		if percent {
			if argIndex >= len(args) || args[argIndex] == nil {
				out.WriteByte('%')
				out.WriteRune(char)
				percent = false
				continue
			}
			value := args[argIndex]
			switch char {
			case 's':
				out.WriteString(value.ToString())
				argIndex++
			case 'd':
				out.WriteString(numberString(ctx, value))
				argIndex++
			case 'j':
				out.WriteString(value.JSONStringify())
				argIndex++
			case 'o':
				out.WriteString(objectString(value))
				argIndex++
			case '%':
				out.WriteByte('%')
			default:
				out.WriteByte('%')
				out.WriteRune(char)
			}
			percent = false
			continue
		}
		if char == '%' {
			percent = true
			continue
		}
		out.WriteRune(char)
	}
	if percent {
		out.WriteByte('%')
	}
	for _, value := range args[argIndex:] {
		out.WriteByte(' ')
		if value != nil {
			out.WriteString(value.ToString())
		}
	}
	return out.String()
}

func numberString(ctx *quickjs.Context, value *quickjs.Value) string {
	if ctx == nil || value == nil {
		return "NaN"
	}
	number := ctx.NewFloat64(value.ToFloat64())
	if number == nil {
		return "NaN"
	}
	defer number.Free()
	return number.ToString()
}

func objectString(value *quickjs.Value) string {
	if value == nil {
		return "undefined"
	}
	if value.IsObject() {
		if json := value.JSONStringify(); json != "" && json != "undefined" {
			return json
		}
	}
	return value.ToString()
}

func formatValue(ctx *quickjs.Context) *quickjs.Value {
	return ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return ctx.NewString(Format(ctx, args))
	})
}

// Module returns the util and node:util ESM module definition.
func Module() module.Definition {
	return ExtendedModule()
}
