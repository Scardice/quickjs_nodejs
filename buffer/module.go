// Package buffer provides a QuickJS-native Buffer implementation.
package buffer

import (
	"fmt"
	"math"

	"github.com/Scardice/quickjs_nodejs/errors"
	"github.com/Scardice/quickjs_nodejs/module"
	quickjs "github.com/buke/quickjs-go"
)

const ModuleName = "buffer"

const bufferMarker = "__quickjs_nodejs_buffer__"

func throwBufferTypeError(ctx *quickjs.Context, format string, args ...any) *quickjs.Value {
	return errors.ThrowTypeError(ctx, errors.ErrCodeInvalidArgType, format, args...)
}

func throwBufferRangeError(ctx *quickjs.Context, format string, args ...any) *quickjs.Value {
	return errors.ThrowRangeError(ctx, errors.ErrCodeOutOfRange, format, args...)
}

type bufferState struct {
	prototype *quickjs.Value
}

// Module returns the memory-backed ESM definition for buffer and node:buffer.
func Module() module.Definition {
	return module.Definition{
		Name:    ModuleName,
		Aliases: []string{"node:buffer"},
		Exports: []module.Export{
			{Name: "Buffer", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return newBufferConstructor(ctx), nil
			}}},
			{Name: "default", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				ctor := newBufferConstructor(ctx)
				obj := ctx.NewObject()
				obj.Set("Buffer", ctor)
				return obj, nil
			}}},
		},
	}
}

// InstallGlobal installs a fresh Buffer constructor on globalThis.
func InstallGlobal(ctx *quickjs.Context) error {
	if ctx == nil {
		return fmt.Errorf("buffer context is nil")
	}
	ctor := newBufferConstructor(ctx)
	if ctor == nil {
		return fmt.Errorf("create Buffer constructor failed")
	}
	ctx.Globals().Set("Buffer", ctor)
	return nil
}

// Bytes returns a copied byte representation of a string or binary value.
func Bytes(ctx *quickjs.Context, value *quickjs.Value) ([]byte, error) {
	if ctx == nil || value == nil {
		return nil, fmt.Errorf("buffer value is nil")
	}
	if value.Context() != ctx {
		return nil, fmt.Errorf("buffer value belongs to a different context")
	}
	if value.IsUint8Array() || value.IsUint8ClampedArray() {
		bytes, err := value.ToUint8Array()
		if err != nil {
			return nil, err
		}
		return append([]byte(nil), bytes...), nil
	}
	if value.IsByteArray() {
		bytes, err := value.ToByteArray(uint(value.ByteLen()))
		if err != nil {
			return nil, err
		}
		return append([]byte(nil), bytes...), nil
	}
	if value.IsString() {
		return []byte(value.ToString()), nil
	}
	return nil, fmt.Errorf("value is not a string, ArrayBuffer, or Uint8Array")
}

// DecodeBytes copies bytes from a string or binary value using encoding.
func DecodeBytes(ctx *quickjs.Context, value, encoding *quickjs.Value) ([]byte, error) {
	if ctx == nil || value == nil {
		return nil, fmt.Errorf("buffer value is nil")
	}
	name := "utf8"
	if encoding != nil && !encoding.IsUndefined() {
		name = encoding.ToString()
	}
	if value.IsString() {
		return decodeStringChecked(value.ToString(), name)
	}
	return Bytes(ctx, value)
}

// EncodeBytes returns an encoded string, or a Buffer when encoding is absent.
func EncodeBytes(ctx *quickjs.Context, data []byte, encoding *quickjs.Value) *quickjs.Value {
	if ctx == nil {
		return nil
	}
	if encoding == nil || encoding.IsUndefined() {
		return WrapBytes(ctx, data)
	}
	encoded, err := encodeString(data, encoding.ToString())
	if err != nil {
		return errors.ThrowTypeError(ctx, errors.ErrCodeUnknownEncoding, "%s", err)
	}
	return ctx.NewString(encoded)
}

// WrapBytes creates a Buffer containing a copy of data.
func WrapBytes(ctx *quickjs.Context, data []byte) *quickjs.Value {
	if ctx == nil {
		return nil
	}
	state := &bufferState{}
	ctor := newBufferConstructorWithState(ctx, state)
	if ctor == nil {
		return nil
	}
	result := state.fromBytes(ctx, data)
	ctor.Free()
	return result
}

func newBufferConstructor(ctx *quickjs.Context) *quickjs.Value {
	return newBufferConstructorWithState(ctx, &bufferState{})
}

func newBufferConstructorWithState(ctx *quickjs.Context, state *bufferState) *quickjs.Value {
	globals := ctx.Globals()
	uint8Ctor := globals.Get("Uint8Array")
	if uint8Ctor == nil || !uint8Ctor.IsFunction() {
		if uint8Ctor != nil {
			uint8Ctor.Free()
		}
		return nil
	}
	uint8Proto := uint8Ctor.Get("prototype")
	uint8Ctor.Free()
	if uint8Proto == nil {
		return nil
	}

	proto := ctx.NewObject()
	if proto == nil || !proto.SetPrototype(uint8Proto) {
		uint8Proto.Free()
		if proto != nil {
			proto.Free()
		}
		return nil
	}
	uint8Proto.Free()
	installPrototypeMethods(ctx, proto, state)
	state.prototype = proto

	implementation := ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return state.fromArgs(ctx, args)
	})
	if implementation == nil {
		proto.Free()
		return nil
	}
	ctor := makeConstructible(ctx, "__quickjs_buffer_impl__", "Buffer", implementation, proto)
	if ctor == nil {
		proto.Free()
		return nil
	}
	proto.DefinePropertyValue("constructor", ctor, quickjs.PropConfigurable)
	installStaticMethods(ctx, ctor, state)
	return ctor
}
func makeConstructible(ctx *quickjs.Context, temporaryName, constructorName string, implementation, prototype *quickjs.Value) *quickjs.Value {
	if implementation == nil || prototype == nil {
		return nil
	}
	globals := ctx.Globals()
	globals.Set(temporaryName, implementation)
	wrapper := ctx.Eval(`(function () {
		const implementation = globalThis["` + temporaryName + `"];
		return function ` + constructorName + `(...args) {
			return implementation(...args);
		};
	})()`)
	globals.Delete(temporaryName)
	if wrapper == nil || wrapper.IsException() {
		if wrapper != nil {
			wrapper.Free()
		}
		return nil
	}
	wrapper.Set("prototype", prototype)
	return wrapper
}

func (s *bufferState) fromArgs(ctx *quickjs.Context, args []*quickjs.Value) *quickjs.Value {
	if len(args) == 0 || args[0] == nil || args[0].IsUndefined() || args[0].IsNull() {
		return throwBufferTypeError(ctx, "The first argument must be a string or an ArrayBuffer, TypedArray, or array-like object")
	}
	value := args[0]
	if value.IsString() {
		data, err := decodeStringChecked(value.ToString(), encodingName(args, 1))
		if err != nil {
			return errors.ThrowTypeError(ctx, errors.ErrCodeUnknownEncoding, "%s", err)
		}
		return s.fromBytes(ctx, data)
	}
	if value.IsNumber() {
		return throwBufferTypeError(ctx, "The first argument must not be a number")
	}
	if value.IsByteArray() {
		bytes, err := value.ToByteArray(uint(value.ByteLen()))
		if err != nil {
			return throwBufferTypeError(ctx, "could not read ArrayBuffer: %s", err)
		}
		return s.fromBytes(ctx, bytes)
	}
	if value.IsUint8Array() || value.IsUint8ClampedArray() {
		bytes, err := value.ToUint8Array()
		if err != nil {
			return throwBufferTypeError(ctx, "could not read typed array: %s", err)
		}
		return s.fromBytes(ctx, bytes)
	}
	if value.IsObject() {
		length := value.Get("length")
		if length != nil && !length.IsUndefined() {
			n := length.ToInt64()
			length.Free()
			if n < 0 {
				n = 0
			}
			if n > maxBufferLength {
				return throwBufferRangeError(ctx, "Invalid array-like length")
			}
			bytes := make([]byte, int(n))
			for i := range bytes {
				item := value.GetInt64(int64(i))
				if item != nil {
					bytes[i] = byte(item.ToInt64())
					item.Free()
				}
			}
			return s.fromBytes(ctx, bytes)
		}
		if length != nil {
			length.Free()
		}
	}
	return throwBufferTypeError(ctx, "The first argument must be a string or an ArrayBuffer, TypedArray, or array-like object")
}

func (s *bufferState) fromBytes(ctx *quickjs.Context, data []byte) *quickjs.Value {
	result := ctx.NewUint8Array(append([]byte(nil), data...))
	if result == nil {
		return nil
	}
	if s.prototype != nil {
		s.markBuffer(ctx, result)
		if !result.SetPrototype(s.prototype) {
			result.Free()
			return nil
		}
	}
	return result
}

func (s *bufferState) markBuffer(ctx *quickjs.Context, value *quickjs.Value) {
	marker := ctx.NewBool(true)
	if marker == nil {
		return
	}
	if !value.DefinePropertyValue(bufferMarker, marker, quickjs.PropConfigurable|quickjs.PropWritable) {
		marker.Free()
		return
	}
	marker.Free()
}

func installStaticMethods(ctx *quickjs.Context, ctor *quickjs.Value, state *bufferState) {
	ctor.Set("from", ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return state.fromArgs(ctx, args)
	}))
	ctor.Set("alloc", ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return state.alloc(ctx, args)
	}))
	ctor.Set("isBuffer", ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if len(args) == 0 {
			return ctx.NewBool(false)
		}
		return ctx.NewBool(isBuffer(args[0]))
	}))
	ctor.Set("byteLength", ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if len(args) == 0 || args[0] == nil {
			return throwBufferTypeError(ctx, "The first argument must be a string or an ArrayBuffer")
		}
		value := args[0]
		if value.IsString() {
			data, err := decodeStringChecked(value.ToString(), encodingName(args, 1))
			if err != nil {
				return errors.ThrowTypeError(ctx, errors.ErrCodeUnknownEncoding, "%s", err)
			}
			return ctx.NewInt64(int64(len(data)))
		}
		if value.IsByteArray() {
			return ctx.NewInt64(value.ByteLen())
		}
		if value.IsUint8Array() || value.IsUint8ClampedArray() {
			return ctx.NewInt64(value.ByteLen())
		}
		return throwBufferTypeError(ctx, "The first argument must be a string or an ArrayBuffer")
	}))
}

func installPrototypeMethods(ctx *quickjs.Context, proto *quickjs.Value, state *bufferState) {
	proto.Set("toString", ctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return state.toString(ctx, this, args)
	}))
	proto.Set("equals", ctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return state.equals(ctx, this, args)
	}))
	proto.Set("write", ctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return state.write(ctx, this, args)
	}))
	for name, fn := range numericMethods {
		method := fn
		proto.Set(name, ctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
			return method(ctx, this, args)
		}))
	}
}

func isBuffer(value *quickjs.Value) bool {
	if value == nil || !value.IsObject() {
		return false
	}
	marker := value.Get(bufferMarker)
	if marker == nil {
		return false
	}
	defer marker.Free()
	return marker.ToBool()
}

func encodingName(args []*quickjs.Value, index int) string {
	if len(args) <= index || args[index] == nil || args[index].IsUndefined() {
		return "utf8"
	}
	return args[index].ToString()
}

func integerArgument(args []*quickjs.Value, index int, fallback int64) int64 {
	if len(args) <= index || args[index] == nil || args[index].IsUndefined() {
		return fallback
	}
	return args[index].ToInt64()
}

func finiteInteger(value *quickjs.Value) (int64, bool) {
	if value == nil || !value.IsNumber() {
		return 0, false
	}
	f := value.ToFloat64()
	if math.IsNaN(f) || math.IsInf(f, 0) || f < 0 || f > float64(maxBufferLength) || f != math.Trunc(f) {
		return 0, false
	}
	return int64(f), true
}

const maxBufferLength = int64(1 << 31)
