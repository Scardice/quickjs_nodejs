package buffer

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/Scardice/quickjs_nodejs/errors"
	quickjs "github.com/buke/quickjs-go"
)

func decodeString(value, encoding string) []byte {
	bytes, _ := decodeStringChecked(value, encoding)
	return bytes
}

func decodeStringChecked(value, encoding string) ([]byte, error) {
	switch strings.ToLower(encoding) {
	case "", "utf8", "utf-8":
		if utf8.ValidString(value) {
			return []byte(value), nil
		}
		return []byte(strings.ToValidUTF8(value, "\uFFFD")), nil
	case "hex":
		return decodeHex(value), nil
	case "base64":
		return decodeBase64(value, false), nil
	case "base64url", "base64-url":
		return decodeBase64(value, true), nil
	default:
		return nil, fmt.Errorf("Unknown encoding: %s", encoding)
	}
}

func decodeHex(value string) []byte {
	result := make([]byte, 0, len(value)/2)
	for i := 0; i+1 < len(value); i += 2 {
		var decoded [1]byte
		n, err := hex.Decode(decoded[:], []byte(value[i:i+2]))
		if err != nil || n != 1 {
			break
		}
		result = append(result, decoded[0])
	}
	return result
}

func decodeBase64(value string, _ bool) []byte {
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range value {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '=':
			builder.WriteRune(r)
		case r == '+' || r == '-':
			builder.WriteByte('+')
		case r == '/' || r == '_':
			builder.WriteByte('/')
		}
	}
	clean := strings.TrimRight(builder.String(), "=")
	if clean == "" {
		return nil
	}
	for len(clean)%4 != 0 {
		clean += "="
	}
	decoded, err := base64.StdEncoding.DecodeString(clean)
	if err != nil {
		// Raw decoding accepts the unpadded form and is useful for partially
		// valid input after Node-style invalid-character filtering.
		decoded, _ = base64.RawStdEncoding.DecodeString(strings.TrimRight(clean, "="))
	}
	return decoded
}

func encodeString(value []byte, encoding string) (string, error) {
	switch strings.ToLower(encoding) {
	case "", "utf8", "utf-8":
		return strings.ToValidUTF8(string(value), "\uFFFD"), nil
	case "hex":
		return hex.EncodeToString(value), nil
	case "base64":
		return base64.StdEncoding.EncodeToString(value), nil
	case "base64url", "base64-url":
		return base64.RawURLEncoding.EncodeToString(value), nil
	default:
		return "", fmt.Errorf("Unknown encoding: %s", encoding)
	}
}

func (s *bufferState) alloc(ctx *quickjs.Context, args []*quickjs.Value) *quickjs.Value {
	if len(args) == 0 {
		return throwBufferRangeError(ctx, "The \"size\" argument must be of type number")
	}
	size, ok := finiteInteger(args[0])
	if !ok {
		return throwBufferRangeError(ctx, "The \"size\" argument must be a finite non-negative integer")
	}
	data := make([]byte, int(size))
	if len(args) <= 1 || args[1] == nil || args[1].IsUndefined() || args[1].IsNull() {
		return s.fromBytes(ctx, data)
	}
	fill := args[1]
	if fill.IsString() {
		pattern, err := decodeStringChecked(fill.ToString(), encodingName(args, 2))
		if err != nil {
			return errors.ThrowTypeError(ctx, errors.ErrCodeUnknownEncoding, "%s", err)
		}
		fillPattern(data, pattern)
		return s.fromBytes(ctx, data)
	}
	value := fill.ToFloat64()
	if !math.IsNaN(value) && !math.IsInf(value, 0) {
		byteValue := byte(int64(value))
		for i := range data {
			data[i] = byteValue
		}
	}
	return s.fromBytes(ctx, data)
}

func fillPattern(dst, pattern []byte) {
	if len(pattern) == 0 {
		return
	}
	for i := range dst {
		dst[i] = pattern[i%len(pattern)]
	}
}

func (s *bufferState) toString(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	data, ok := bytesFromValue(this)
	if !ok {
		return throwBufferTypeError(ctx, "Method Buffer.prototype.toString called on incompatible receiver")
	}
	encoding := encodingName(args, 0)
	start := integerArgument(args, 1, 0)
	if start < 0 {
		start = 0
	}
	if start >= int64(len(data)) {
		return ctx.NewString("")
	}
	end := integerArgument(args, 2, int64(len(data)))
	if end < 0 || start >= end {
		return ctx.NewString("")
	}
	if end > int64(len(data)) {
		end = int64(len(data))
	}
	encoded, err := encodeString(data[start:end], encoding)
	if err != nil {
		return errors.ThrowTypeError(ctx, errors.ErrCodeUnknownEncoding, "%s", err)
	}
	return ctx.NewString(encoded)
}

func (s *bufferState) equals(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	left, ok := bytesFromValue(this)
	if !ok || len(args) == 0 {
		return throwBufferTypeError(ctx, "Method Buffer.prototype.equals called on incompatible receiver")
	}
	right, ok := bytesFromValue(args[0])
	if !ok {
		return throwBufferTypeError(ctx, "The \"otherBuffer\" argument must be an instance of Buffer or Uint8Array")
	}
	if len(left) != len(right) {
		return ctx.NewBool(false)
	}
	for i := range left {
		if left[i] != right[i] {
			return ctx.NewBool(false)
		}
	}
	return ctx.NewBool(true)
}

func (s *bufferState) write(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	if len(args) == 0 || args[0] == nil || !args[0].IsString() {
		return throwBufferTypeError(ctx, "The \"string\" argument must be of type string")
	}
	length := int64(lenBytes(this))
	offset := integerArgument(args, 1, 0)
	if offset < 0 || offset > length {
		return throwBufferRangeError(ctx, "The \"offset\" argument is out of range")
	}
	maxLength := length - offset
	writeLength := integerArgument(args, 2, maxLength)
	if writeLength < 0 {
		return throwBufferRangeError(ctx, "The \"length\" argument is out of range")
	}
	if writeLength > maxLength {
		writeLength = maxLength
	}
	data, err := decodeStringChecked(args[0].ToString(), encodingName(args, 3))
	if err != nil {
		return errors.ThrowTypeError(ctx, errors.ErrCodeUnknownEncoding, "%s", err)
	}
	if int64(len(data)) < writeLength {
		writeLength = int64(len(data))
	}
	if !writeBytes(ctx, this, offset, data[:writeLength]) {
		return throwBufferTypeError(ctx, "failed to write Buffer")
	}
	return ctx.NewInt64(writeLength)
}

func bytesFromValue(value *quickjs.Value) ([]byte, bool) {
	if value == nil || (!value.IsUint8Array() && !value.IsUint8ClampedArray()) {
		return nil, false
	}
	data, err := value.ToUint8Array()
	if err != nil {
		return nil, false
	}
	return data, true
}

func lenBytes(value *quickjs.Value) int {
	data, ok := bytesFromValue(value)
	if !ok {
		return 0
	}
	return len(data)
}

func writeBytes(ctx *quickjs.Context, target *quickjs.Value, offset int64, data []byte) bool {
	if target == nil {
		return false
	}
	for i, value := range data {
		// SetIdx consumes the created numeric value; it must not be freed again.
		target.SetIdx(offset+int64(i), ctx.NewInt32(int32(value)))
	}
	return true
}
