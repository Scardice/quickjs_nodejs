package crypto

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	quickjs "github.com/buke/quickjs-go"
)

func encodeBase64URL(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeBase64URL(value string) ([]byte, error) {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("invalid base64url data: %w", err)
	}
	return data, nil
}

func readBufferSource(ctx *quickjs.Context, value *quickjs.Value) ([]byte, error) {
	if ctx == nil || value == nil || value.IsNull() || value.IsUndefined() {
		return nil, errors.New("expected an ArrayBuffer or typed array")
	}
	if value.Context() != ctx {
		return nil, errors.New("value belongs to a different context")
	}
	switch {
	case value.IsUint8Array(), value.IsUint8ClampedArray():
		data, err := value.ToUint8Array()
		return append([]byte(nil), data...), err
	case value.IsInt8Array():
		data, err := value.ToInt8Array()
		if err != nil {
			return nil, err
		}
		out := make([]byte, len(data))
		for i, item := range data {
			out[i] = byte(item)
		}
		return out, nil
	case value.IsInt16Array():
		data, err := value.ToInt16Array()
		if err != nil {
			return nil, err
		}
		out := make([]byte, 2*len(data))
		for i, item := range data {
			binary.LittleEndian.PutUint16(out[2*i:], uint16(item))
		}
		return out, nil
	case value.IsUint16Array():
		data, err := value.ToUint16Array()
		if err != nil {
			return nil, err
		}
		out := make([]byte, 2*len(data))
		for i, item := range data {
			binary.LittleEndian.PutUint16(out[2*i:], item)
		}
		return out, nil
	case value.IsInt32Array():
		data, err := value.ToInt32Array()
		if err != nil {
			return nil, err
		}
		out := make([]byte, 4*len(data))
		for i, item := range data {
			binary.LittleEndian.PutUint32(out[4*i:], uint32(item))
		}
		return out, nil
	case value.IsUint32Array():
		data, err := value.ToUint32Array()
		if err != nil {
			return nil, err
		}
		out := make([]byte, 4*len(data))
		for i, item := range data {
			binary.LittleEndian.PutUint32(out[4*i:], item)
		}
		return out, nil
	case value.IsFloat32Array():
		data, err := value.ToFloat32Array()
		if err != nil {
			return nil, err
		}
		out := make([]byte, 4*len(data))
		for i, item := range data {
			binary.LittleEndian.PutUint32(out[4*i:], math.Float32bits(item))
		}
		return out, nil
	case value.IsFloat64Array():
		data, err := value.ToFloat64Array()
		if err != nil {
			return nil, err
		}
		out := make([]byte, 8*len(data))
		for i, item := range data {
			binary.LittleEndian.PutUint64(out[8*i:], math.Float64bits(item))
		}
		return out, nil
	case value.IsByteArray():
		data, err := value.ToByteArray(uint(value.ByteLen()))
		return append([]byte(nil), data...), err
	}
	// DataView is not reported as a typed array by quickjs-go. Its backing
	// ArrayBuffer and byte offset/length are exposed by the ECMAScript object.
	if value.IsObject() {
		bufferValue := value.Get("buffer")
		if bufferValue != nil {
			defer bufferValue.Free()
			if bufferValue.IsByteArray() {
				all, err := bufferValue.ToByteArray(uint(bufferValue.ByteLen()))
				if err != nil {
					return nil, err
				}
				offset := propertyInt(value, "byteOffset", 0)
				length := propertyInt(value, "byteLength", int64(len(all))-offset)
				if offset < 0 || length < 0 || offset > int64(len(all)) || length > int64(len(all))-offset {
					return nil, errors.New("invalid byte range")
				}
				return append([]byte(nil), all[offset:offset+length]...), nil
			}
		}
	}
	return nil, errors.New("expected an ArrayBuffer or typed array")
}

func propertyInt(value *quickjs.Value, name string, fallback int64) int64 {
	property := value.Get(name)
	if property == nil {
		return fallback
	}
	defer property.Free()
	if property.IsException() || property.IsUndefined() {
		return fallback
	}
	return property.ToInt64()
}

func arrayValues(value *quickjs.Value) ([]*quickjs.Value, error) {
	if value == nil || !value.IsArray() {
		return nil, errors.New("expected an array")
	}
	lengthValue := value.Get("length")
	if lengthValue == nil {
		return nil, errors.New("array has no length")
	}
	length := lengthValue.ToInt64()
	lengthValue.Free()
	if length < 0 || length > 1<<20 {
		return nil, errors.New("array is too large")
	}
	values := make([]*quickjs.Value, 0, length)
	for index := int64(0); index < length; index++ {
		item := value.GetIdx(index)
		if item == nil {
			for _, previous := range values {
				previous.Free()
			}
			return nil, errors.New("failed to read array")
		}
		values = append(values, item)
	}
	return values, nil
}

func readStringProperty(value *quickjs.Value, name string) (string, bool) {
	if value == nil {
		return "", false
	}
	property := value.Get(name)
	if property == nil {
		return "", false
	}
	defer property.Free()
	if property.IsUndefined() || property.IsNull() || property.IsException() {
		return "", false
	}
	return property.ToString(), true
}

func readBoolProperty(value *quickjs.Value, name string, fallback bool) bool {
	if value == nil {
		return fallback
	}
	property := value.Get(name)
	if property == nil {
		return fallback
	}
	defer property.Free()
	if property.IsUndefined() {
		return fallback
	}
	return property.ToBool()
}

func bytesForCrypto(ctx *quickjs.Context, value *quickjs.Value) ([]byte, error) {
	data, err := readBufferSource(ctx, value)
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), data...), nil
}

func readJWKSecret(value *quickjs.Value) ([]byte, error) {
	if value == nil || !value.IsObject() {
		return nil, errors.New("JWK must be an object")
	}
	kty, ok := readStringProperty(value, "kty")
	if !ok || kty != "oct" {
		return nil, errors.New("only octet JWK keys are supported")
	}
	encoded, ok := readStringProperty(value, "k")
	if !ok {
		return nil, errors.New("JWK.k is required")
	}
	secret, err := decodeBase64URL(encoded)
	if err != nil {
		return nil, err
	}
	if len(secret) == 0 {
		return nil, errors.New("JWK.k must not be empty")
	}
	return secret, nil
}

func jwkHMACAlgorithm(hashName string, length int) string {
	var prefix string
	switch normalizeName(hashName) {
	case "SHA-1", "SHA1":
		prefix = "HS1"
	case "SHA-224":
		prefix = "HS224"
	case "SHA-384":
		prefix = "HS384"
	case "SHA-512":
		prefix = "HS512"
	default:
		prefix = "HS256"
	}
	if length <= 0 {
		return prefix
	}
	return prefix
}

func jwkAESAlgorithm(name string, length int) string {
	switch normalizeName(name) {
	case "AES-CBC":
		return fmt.Sprintf("A%dCBC", length)
	case "AES-CTR":
		return fmt.Sprintf("A%dCTR", length)
	case "AES-GCM":
		return fmt.Sprintf("A%dGCM", length)
	case "AES-KW":
		return fmt.Sprintf("A%dKW", length)
	default:
		return ""
	}
}
