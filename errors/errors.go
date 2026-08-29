// Package errors creates Node-style JavaScript Error objects for QuickJS.
package errors

import (
	"fmt"

	quickjs "github.com/buke/quickjs-go"
)

// Code is a Node-compatible error code.
type Code = string

const (
	ErrCodeInvalidArgType  = "ERR_INVALID_ARG_TYPE"
	ErrCodeInvalidArgValue = "ERR_INVALID_ARG_VALUE"
	ErrCodeMissingArgs     = "ERR_MISSING_ARGS"
	ErrCodeOutOfRange      = "ERR_OUT_OF_RANGE"
	ErrCodeInvalidThis     = "ERR_INVALID_THIS"
	ErrCodeUnknownEncoding = "ERR_UNKNOWN_ENCODING"
	ErrCodeInvalidURL      = "ERR_INVALID_URL"
	ErrCodeInvalidTuple    = "ERR_INVALID_TUPLE"
	ErrCodeNotImplemented  = "ERR_NOT_IMPLEMENTED"
)

// NewError creates an Error instance using ctor when non-nil, or Error otherwise.
// The returned value is owned by the caller.
func NewError(ctx *quickjs.Context, ctor *quickjs.Value, code string, format string, args ...any) *quickjs.Value {
	if ctx == nil {
		return nil
	}
	if ctor == nil {
		ctor = ctx.Globals().Get("Error")
		if ctor == nil {
			return nil
		}
		defer ctor.Free()
	}
	message := ctx.NewString(fmt.Sprintf(format, args...))
	if message == nil {
		return nil
	}
	value := ctor.CallConstructor(message)
	message.Free()
	if value == nil || value.IsException() {
		return value
	}
	if code != "" {
		value.Set("code", ctx.NewString(string(code)))
	}
	toString := ctx.NewFunction(errorToString)
	if toString != nil {
		value.DefinePropertyValue("toString", toString, quickjs.PropConfigurable|quickjs.PropWritable)
		toString.Free()
	}
	return value
}

func errorToString(ctx *quickjs.Context, this *quickjs.Value, _ []*quickjs.Value) *quickjs.Value {
	if this == nil || !this.IsObject() {
		return ctx.NewString("Error")
	}
	nameValue := this.Get("name")
	messageValue := this.Get("message")
	codeValue := this.Get("code")
	defer nameValue.Free()
	defer messageValue.Free()
	defer codeValue.Free()

	name := "Error"
	if nameValue != nil && !nameValue.IsUndefined() {
		name = nameValue.ToString()
	}
	message := ""
	if messageValue != nil && !messageValue.IsUndefined() {
		message = messageValue.ToString()
	}
	code := ""
	if codeValue != nil && !codeValue.IsUndefined() {
		code = codeValue.ToString()
	}
	prefix := name
	if code != "" {
		prefix += " [" + code + "]"
	}
	if message != "" {
		prefix += ": " + message
	}
	return ctx.NewString(prefix)
}

// NewTypeError creates a TypeError with an optional Node error code.
func NewTypeError(ctx *quickjs.Context, code string, format string, args ...any) *quickjs.Value {
	ctor := ctx.Globals().Get("TypeError")
	if ctor == nil {
		return nil
	}
	defer ctor.Free()
	return NewError(ctx, ctor, code, format, args...)
}

// NewRangeError creates a RangeError with an optional Node error code.
func NewRangeError(ctx *quickjs.Context, code string, format string, args ...any) *quickjs.Value {
	ctor := ctx.Globals().Get("RangeError")
	if ctor == nil {
		return nil
	}
	defer ctor.Free()
	return NewError(ctx, ctor, code, format, args...)
}

// ThrowError throws an Error object and returns QuickJS's exception sentinel.
func ThrowError(ctx *quickjs.Context, ctor *quickjs.Value, code string, format string, args ...any) *quickjs.Value {
	value := NewError(ctx, ctor, code, format, args...)
	if value == nil {
		return nil
	}
	return ctx.Throw(value)
}

// ThrowTypeError throws a TypeError with an optional Node error code.
func ThrowTypeError(ctx *quickjs.Context, code string, format string, args ...any) *quickjs.Value {
	return throwWithConstructor(ctx, "TypeError", code, format, args...)
}

// ThrowRangeError throws a RangeError with an optional Node error code.
func ThrowRangeError(ctx *quickjs.Context, code string, format string, args ...any) *quickjs.Value {
	return throwWithConstructor(ctx, "RangeError", code, format, args...)
}

// NewNotCorrectTypeError creates ERR_INVALID_ARG_TYPE for a named argument.
func NewNotCorrectTypeError(ctx *quickjs.Context, name, expected string) *quickjs.Value {
	return NewTypeError(ctx, ErrCodeInvalidArgType, "The \"%s\" argument must be of type %s", name, expected)
}

// NewArgumentOutOfRangeError creates ERR_OUT_OF_RANGE for a named argument.
func NewArgumentOutOfRangeError(ctx *quickjs.Context, name string, value any) *quickjs.Value {
	return NewRangeError(ctx, ErrCodeOutOfRange, "The value of \"%s\" is out of range. Received %v", name, value)
}

func throwWithConstructor(ctx *quickjs.Context, name string, code string, format string, args ...any) *quickjs.Value {
	if ctx == nil {
		return nil
	}
	ctor := ctx.Globals().Get(name)
	if ctor == nil {
		return nil
	}
	defer ctor.Free()
	return ThrowError(ctx, ctor, code, format, args...)
}
