//nolint:gosec // WebCrypto compatibility includes SHA-1 and MD5 digest support.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/Scardice/quickjs_nodejs/eventloop"
	"github.com/Scardice/quickjs_nodejs/limits"
	"github.com/Scardice/quickjs_nodejs/module"
	quickjs "github.com/buke/quickjs-go"
)

const (
	ModuleName              = "crypto"
	maxGetRandomValuesBytes = 65536
	cryptoExportsKey        = "__quickjs_nodejs_crypto_exports"
	cryptoNativeKey         = "__quickjs_nodejs_crypto_native"
)

type Option func(*Config)

type Config struct {
	ResourceLimits *limits.Runtime
}

func WithResourceLimits(resourceLimits *limits.Runtime) Option {
	return func(config *Config) { config.ResourceLimits = resourceLimits }
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

// Module returns the crypto and node:crypto WebCrypto module definition.
func Module() module.Definition {
	return module.Definition{
		Name:    ModuleName,
		Aliases: []string{"node:" + ModuleName},
		Exports: []module.Export{
			{Name: "CryptoKey", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return cryptoExport(ctx, "CryptoKey")
			}}},
			{Name: "subtle", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return cryptoExport(ctx, "subtle")
			}}},
			{Name: "getRandomValues", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return cryptoExport(ctx, "getRandomValues")
			}}},
			{Name: "randomUUID", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return cryptoExport(ctx, "randomUUID")
			}}},
			{Name: "webcrypto", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return cryptoExport(ctx, "crypto")
			}}},
			{Name: "default", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return cryptoExport(ctx, "crypto")
			}}},
		},
	}
}

// InstallGlobal installs a fresh WebCrypto-compatible crypto object in ctx.
func InstallGlobal(ctx *quickjs.Context, options ...Option) error {
	if ctx == nil {
		return errors.New("crypto: nil context")
	}
	cryptoObject, err := ensureCrypto(ctx, applyOptions(options))
	if err != nil {
		return err
	}
	ctx.Globals().Set("crypto", cryptoObject)
	return nil
}

func cryptoExport(ctx *quickjs.Context, name string) (*quickjs.Value, error) {
	cryptoObject, err := ensureCrypto(ctx, Config{})
	if err != nil {
		return nil, err
	}
	var value *quickjs.Value
	if name == "crypto" {
		value = cryptoObject
	} else {
		value = cryptoObject.Get(name)
		cryptoObject.Free()
	}
	if value == nil {
		return nil, fmt.Errorf("crypto: export %q is unavailable", name)
	}
	return value, nil
}

func ensureCrypto(ctx *quickjs.Context, config Config) (*quickjs.Value, error) {
	global := ctx.Globals()
	cached := global.Get(cryptoExportsKey)
	if cached != nil && cached.IsObject() {
		cryptoObject := cached.Get("crypto")
		cached.Free()
		if cryptoObject != nil && cryptoObject.IsObject() {
			return cryptoObject, nil
		}
		if cryptoObject != nil {
			cryptoObject.Free()
		}
	} else if cached != nil {
		cached.Free()
	}

	state, err := newCryptoState(ctx, config.ResourceLimits)
	if err != nil {
		return nil, err
	}
	native := ctx.NewObject()
	if native == nil {
		state.Close()
		return nil, errors.New("crypto: create native object")
	}
	installNativeFunctions(ctx, native, state)
	if !global.DefinePropertyValue(cryptoNativeKey, native, quickjs.PropConfigurable) {
		native.Free()
		state.Close()
		return nil, errors.New("crypto: install native object")
	}
	native.Free()

	result := ctx.Eval(cryptoImplementation)
	if result == nil {
		state.Close()
		return nil, errors.New("crypto: initialization returned nil")
	}
	if result.IsException() {
		err := ctx.Exception()
		result.Free()
		if err == nil {
			err = errors.New("crypto: initialization failed")
		}
		state.Close()
		return nil, err
	}
	cryptoObject := result.Get("crypto")
	result.Free()
	if cryptoObject == nil || !cryptoObject.IsObject() {
		if cryptoObject != nil {
			cryptoObject.Free()
		}
		state.Close()
		return nil, errors.New("crypto: initialization returned invalid object")
	}
	eventloop.RegisterContextResource(ctx, state)
	return cryptoObject, nil
}

const cryptoImplementation = `(function () {
  const native = globalThis["__quickjs_nodejs_crypto_native"];
  {
    const legacyCodes = {
      IndexSizeError: 1,
      HierarchyRequestError: 3,
      WrongDocumentError: 4,
      InvalidCharacterError: 5,
      NoModificationAllowedError: 7,
      NotFoundError: 8,
      NotSupportedError: 9,
      InvalidStateError: 11,
      SyntaxError: 12,
      InvalidModificationError: 13,
      NamespaceError: 14,
      InvalidAccessError: 15,
      TypeMismatchError: 17,
      SecurityError: 18,
      NetworkError: 19,
      AbortError: 20,
      URLMismatchError: 21,
      QuotaExceededError: 22,
      TimeoutError: 23,
      InvalidNodeTypeError: 24,
      DataCloneError: 25
    };
    globalThis.DOMException = class DOMException extends Error {
      constructor(message = "", name = "Error") {
        super(message);
        this.name = name;
        this.code = legacyCodes[name] || 0;
        if (name === "QuotaExceededError") {
          this.requested = null;
          this.quota = null;
        }
      }
    };
  }
  globalThis.QuotaExceededError = class QuotaExceededError extends globalThis.DOMException {
    constructor(message = "") {
      super(message, "QuotaExceededError");
      this.requested = null;
      this.quota = null;
    }
  };
  if (typeof globalThis.TextEncoder !== "function") {
    globalThis.TextEncoder = class TextEncoder {
      encode(input) {
        const encoded = unescape(encodeURIComponent(String(input)));
        const bytes = new Uint8Array(encoded.length);
        for (let index = 0; index < encoded.length; index++) bytes[index] = encoded.charCodeAt(index);
        return bytes;
      }
    };
  }
  if (typeof globalThis.TextDecoder !== "function") {
    globalThis.TextDecoder = class TextDecoder {
      decode(input) {
        const bytes = input instanceof Uint8Array ? input : new Uint8Array(input);
        if (bytes.length === 0) return "";
        let encoded = "";
        for (const byte of bytes) encoded += "%" + byte.toString(16).padStart(2, "0");
        return decodeURIComponent(encoded);
      }
    };
  }
  const CryptoKey = function CryptoKey() { throw new TypeError("Illegal constructor"); };
  Object.defineProperty(CryptoKey.prototype, Symbol.toStringTag, { value: "CryptoKey" });
  const subtle = {};
  const wrap = value => {
    if (value && typeof value === "object") {
      if (Array.isArray(value)) return value.map(wrap);
      if (native.isCryptoKey(value)) Object.setPrototypeOf(value, CryptoKey.prototype);
      if (value.publicKey) value.publicKey = wrap(value.publicKey);
      if (value.privateKey) value.privateKey = wrap(value.privateKey);
    }
    return value;
  };
  const asyncCall = (name, args) => Promise.resolve().then(() => wrap(native[name].apply(native, args)));
  for (const name of ["digest", "generateKey", "importKey", "exportKey", "sign", "verify", "encrypt", "decrypt", "deriveBits", "deriveKey", "wrapKey", "unwrapKey"]) {
    subtle[name] = function (...args) { return asyncCall(name, args); };
  }
  const crypto = {
    subtle,
    getRandomValues: function (target) { native.getRandomValues(target); return target; },
    randomUUID: function (...args) { return native.randomUUID.apply(native, args); },
    CryptoKey
  };
  crypto.webcrypto = crypto;
  Object.defineProperty(globalThis, "__quickjs_nodejs_crypto_exports", { value: { crypto, CryptoKey, subtle, getRandomValues: crypto.getRandomValues, randomUUID: crypto.randomUUID }, configurable: false, enumerable: false, writable: false });
  return globalThis.__quickjs_nodejs_crypto_exports;
})()`

func installNativeFunctions(ctx *quickjs.Context, native *quickjs.Value, state *cryptoState) {
	native.Set("isCryptoKey", ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if len(args) != 1 {
			return ctx.NewBool(false)
		}
		return ctx.NewBool(state.isKey(ctx, args[0]))
	}))
	native.Set("getRandomValues", ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return state.getRandomValues(ctx, args)
	}))
	native.Set("randomUUID", ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, _ []*quickjs.Value) *quickjs.Value {
		return state.randomUUID(ctx)
	}))
	native.Set("digest", ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return state.digest(ctx, args)
	}))
	native.Set("generateKey", ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return state.generateKey(ctx, args)
	}))
	native.Set("importKey", ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return state.importKey(ctx, args)
	}))
	native.Set("exportKey", ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return state.exportKey(ctx, args)
	}))
	native.Set("sign", ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return state.sign(ctx, args)
	}))
	native.Set("verify", ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return state.verify(ctx, args)
	}))
	native.Set("encrypt", ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return state.encrypt(ctx, args)
	}))
	native.Set("decrypt", ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return state.decrypt(ctx, args)
	}))
	native.Set("deriveBits", ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return state.deriveBits(ctx, args)
	}))
	native.Set("deriveKey", ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return state.deriveKey(ctx, args)
	}))
	native.Set("wrapKey", ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return state.wrapKey(ctx, args)
	}))
	native.Set("unwrapKey", ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return state.unwrapKey(ctx, args)
	}))
}

func cryptoThrow(ctx *quickjs.Context, err error) *quickjs.Value {
	if err == nil {
		err = errors.New("crypto operation failed")
	}
	return ctx.ThrowError(err)
}

func cryptoDOMException(ctx *quickjs.Context, name, message string) *quickjs.Value {
	constructorName := "DOMException"
	if name == "QuotaExceededError" {
		constructorName = "QuotaExceededError"
	}
	constructor := ctx.Globals().Get(constructorName)
	if constructor == nil {
		return cryptoThrow(ctx, errors.New(message))
	}
	defer constructor.Free()
	messageValue := ctx.NewString(message)
	nameValue := ctx.NewString(name)
	if messageValue == nil || nameValue == nil {
		if messageValue != nil {
			messageValue.Free()
		}
		if nameValue != nil {
			nameValue.Free()
		}
		return cryptoThrow(ctx, errors.New(message))
	}
	value := constructor.CallConstructor(messageValue, nameValue)
	messageValue.Free()
	nameValue.Free()
	if value == nil || value.IsException() {
		return value
	}
	return ctx.Throw(value)
}

func cryptoOperationError(ctx *quickjs.Context, message string) *quickjs.Value {
	return cryptoDOMException(ctx, "OperationError", message)
}

func cryptoTypeMismatchError(ctx *quickjs.Context, message string) *quickjs.Value {
	return cryptoDOMException(ctx, "TypeMismatchError", message)
}

func cryptoQuotaExceededError(ctx *quickjs.Context, message string) *quickjs.Value {
	return cryptoDOMException(ctx, "QuotaExceededError", message)
}

func (s *cryptoState) getRandomValues(ctx *quickjs.Context, args []*quickjs.Value) *quickjs.Value {
	if len(args) < 1 || args[0] == nil {
		return cryptoTypeMismatchError(ctx, "crypto.getRandomValues requires an integer typed array")
	}
	target := args[0]
	if !(target.IsInt8Array() || target.IsUint8Array() || target.IsUint8ClampedArray() || target.IsInt16Array() || target.IsUint16Array() || target.IsInt32Array() || target.IsUint32Array() || target.IsBigInt64Array() || target.IsBigUint64Array()) {
		return cryptoTypeMismatchError(ctx, "crypto.getRandomValues requires an integer typed array")
	}
	length := propertyInt(target, "byteLength", -1)
	if length < 0 {
		return cryptoTypeMismatchError(ctx, "invalid integer typed array")
	}
	if length > maxGetRandomValuesBytes {
		return cryptoQuotaExceededError(ctx, "crypto.getRandomValues: byteLength exceeds 65536")
	}
	data := make([]byte, length)
	if _, err := cryptorand.Read(data); err != nil {
		return cryptoOperationError(ctx, err.Error())
	}
	array := randomTypedArray(ctx, target, data)
	if array == nil {
		return ctx.ThrowInternalError("create random typed array")
	}
	result := target.Call("set", array)
	array.Free()
	if result == nil {
		return ctx.ThrowInternalError("write random bytes")
	}
	if result.IsException() {
		return result
	}
	result.Free()
	return ctx.NewUndefined()
}

func randomTypedArray(ctx *quickjs.Context, target *quickjs.Value, data []byte) *quickjs.Value {
	if target.IsInt8Array() {
		values := make([]int8, len(data))
		for index, value := range data {
			values[index] = int8(value)
		}
		return ctx.NewInt8Array(values)
	}
	if target.IsUint8Array() {
		return ctx.NewUint8Array(data)
	}
	if target.IsUint8ClampedArray() {
		return ctx.NewUint8ClampedArray(data)
	}
	if target.IsInt16Array() {
		values := make([]int16, len(data)/2)
		for index := range values {
			values[index] = int16(binary.LittleEndian.Uint16(data[index*2:]))
		}
		return ctx.NewInt16Array(values)
	}
	if target.IsUint16Array() {
		values := make([]uint16, len(data)/2)
		for index := range values {
			values[index] = binary.LittleEndian.Uint16(data[index*2:])
		}
		return ctx.NewUint16Array(values)
	}
	if target.IsInt32Array() {
		values := make([]int32, len(data)/4)
		for index := range values {
			values[index] = int32(binary.LittleEndian.Uint32(data[index*4:]))
		}
		return ctx.NewInt32Array(values)
	}
	if target.IsUint32Array() {
		values := make([]uint32, len(data)/4)
		for index := range values {
			values[index] = binary.LittleEndian.Uint32(data[index*4:])
		}
		return ctx.NewUint32Array(values)
	}
	if target.IsBigInt64Array() {
		values := make([]int64, len(data)/8)
		for index := range values {
			values[index] = int64(binary.LittleEndian.Uint64(data[index*8:]))
		}
		return ctx.NewBigInt64Array(values)
	}
	if target.IsBigUint64Array() {
		values := make([]uint64, len(data)/8)
		for index := range values {
			values[index] = binary.LittleEndian.Uint64(data[index*8:])
		}
		return ctx.NewBigUint64Array(values)
	}
	return nil
}

func (s *cryptoState) randomUUID(ctx *quickjs.Context) *quickjs.Value {
	var bytes [16]byte
	if _, err := cryptorand.Read(bytes[:]); err != nil {
		return cryptoThrow(ctx, err)
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	return ctx.NewString(fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(bytes[0:4]),
		binary.BigEndian.Uint16(bytes[4:6]),
		binary.BigEndian.Uint16(bytes[6:8]),
		binary.BigEndian.Uint16(bytes[8:10]),
		bytes[10:16]))
}

func (s *cryptoState) digest(ctx *quickjs.Context, args []*quickjs.Value) *quickjs.Value {
	if len(args) < 2 {
		return ctx.ThrowTypeError("crypto.subtle.digest requires algorithm and data")
	}
	name, _, err := algorithmName(args[0])
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	data, err := readBufferSource(ctx, args[1])
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	result, err := digestBytes(name, data)
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	return ctx.NewArrayBuffer(result)
}

func (s *cryptoState) generateKey(ctx *quickjs.Context, args []*quickjs.Value) *quickjs.Value {
	if len(args) < 3 {
		return ctx.ThrowTypeError("crypto.subtle.generateKey requires algorithm, extractable, and keyUsages")
	}
	name, algorithm, err := algorithmName(args[0])
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	extractable := args[1].ToBool()
	usages, err := usageList(args[2])
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	if err := validateKeyUsages(name, "", usages); err != nil {
		return cryptoThrow(ctx, err)
	}
	if name == "RSASSA-PKCS1-V1_5" || name == "RSA-PSS" || name == "RSA-OAEP" || name == "RSAES-PKCS1-V1_5" || name == "ECDSA" || name == "ECDH" || name == "ED25519" || name == "X25519" {
		return s.generateAsymmetricKey(ctx, name, algorithm, extractable, usages)
	}
	switch name {
	case "HMAC":
		hashName, err := algorithmHash(algorithmProperty(algorithm, "hash"), "SHA-256")
		if err != nil {
			return cryptoThrow(ctx, err)
		}
		_, hashSize, err := hashFactory(hashName)
		if err != nil {
			return cryptoThrow(ctx, err)
		}
		length := intProperty(algorithm, "length", int64(hashSize*8))
		if length <= 0 || length%8 != 0 {
			return cryptoOperationError(ctx, "HMAC key length must be a positive multiple of 8")
		}
		secret := make([]byte, length/8)
		if _, err := cryptorand.Read(secret); err != nil {
			return cryptoThrow(ctx, err)
		}
		return s.addKey(ctx, &cryptoKey{Type: "secret", Algorithm: name, Hash: hashName, Length: length, Extractable: extractable, Usages: usages, Secret: secret})
	case "AES-CBC", "AES-CTR", "AES-GCM", "AES-KW":
		length := intProperty(algorithm, "length", 0)
		if length != 128 && length != 192 && length != 256 {
			return cryptoOperationError(ctx, "AES key length must be 128, 192, or 256")
		}
		secret := make([]byte, length/8)
		if _, err := cryptorand.Read(secret); err != nil {
			return cryptoThrow(ctx, err)
		}
		return s.addKey(ctx, &cryptoKey{Type: "secret", Algorithm: name, Length: length, Extractable: extractable, Usages: usages, Secret: secret})
	default:
		return cryptoThrow(ctx, fmt.Errorf("unsupported generateKey algorithm: %s", name))
	}
}

func (s *cryptoState) importKey(ctx *quickjs.Context, args []*quickjs.Value) *quickjs.Value {
	if len(args) < 5 {
		return ctx.ThrowTypeError("crypto.subtle.importKey requires format, keyData, algorithm, extractable, and keyUsages")
	}
	format := strings.ToLower(strings.TrimSpace(args[0].ToString()))
	name, algorithm, err := algorithmName(args[2])
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	extractable := args[3].ToBool()
	usages, err := usageList(args[4])
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	key, err := importKeyMaterial(ctx, format, args[1], name, algorithm, extractable, usages)
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	if err := validateKeyUsages(name, key.Type, usages); err != nil {
		return cryptoThrow(ctx, err)
	}
	return s.addKey(ctx, key)
}

func (s *cryptoState) exportKey(ctx *quickjs.Context, args []*quickjs.Value) *quickjs.Value {
	if len(args) < 2 {
		return ctx.ThrowTypeError("crypto.subtle.exportKey requires format and key")
	}
	key, err := s.keyFromValue(ctx, args[1])
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	if err := ensureExtractable(key); err != nil {
		return cryptoThrow(ctx, err)
	}
	format := strings.ToLower(strings.TrimSpace(args[0].ToString()))
	switch format {
	case "raw":
		data, err := exportRawKeyMaterial(key)
		if err != nil {
			return cryptoThrow(ctx, err)
		}
		return ctx.NewArrayBuffer(data)
	case "jwk":
		object, err := exportJWKObject(ctx, key)
		if err != nil {
			return cryptoThrow(ctx, err)
		}
		return object
	case "pkcs8", "pkcs1", "sec1", "spki":
		data, err := exportDERKeyMaterial(key, format)
		if err != nil {
			return cryptoThrow(ctx, err)
		}
		return ctx.NewArrayBuffer(data)
	default:
		return cryptoThrow(ctx, fmt.Errorf("unsupported exportKey format: %s", format))
	}
}

func (s *cryptoState) sign(ctx *quickjs.Context, args []*quickjs.Value) *quickjs.Value {
	if len(args) < 3 {
		return ctx.ThrowTypeError("crypto.subtle.sign requires algorithm, key, and data")
	}
	name, algorithm, err := algorithmName(args[0])
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	key, err := s.keyFromValue(ctx, args[1])
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	if !hasUsage(key, "sign") {
		return cryptoThrow(ctx, errors.New("key is not permitted for sign"))
	}
	data, err := readBufferSource(ctx, args[2])
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	switch name {
	case "HMAC":
		if key.Algorithm != "HMAC" {
			return cryptoThrow(ctx, errors.New("HMAC key is required"))
		}
		hashName := key.Hash
		signature, err := hmacBytes(hashName, key.Secret, data)
		if err != nil {
			return cryptoThrow(ctx, err)
		}
		return ctx.NewArrayBuffer(signature)
	case "RSASSA-PKCS1-V1_5", "RSA-PSS":
		if key.Algorithm != name || key.RSAPrivate == nil {
			return cryptoThrow(ctx, errors.New("RSA private key is required"))
		}
		hashName := key.Hash
		saltLength := -1
		if name == "RSA-PSS" {
			saltLength = intProperty(algorithm, "saltLength", -1)
		}
		signature, err := rsaSignBytes(name, hashName, saltLength, key.RSAPrivate, data)
		if err != nil {
			return cryptoThrow(ctx, err)
		}
		return ctx.NewArrayBuffer(signature)
	case "ECDSA":
		if key.Algorithm != name || key.ECDSAPrivate == nil {
			return cryptoThrow(ctx, errors.New("ECDSA private key is required"))
		}
		hashName, failure := requiredOperationHash(ctx, algorithm)
		if failure != nil {
			return failure
		}
		signature, err := ecdsaSignBytes(hashName, key.ECDSAPrivate, data)
		if err != nil {
			return cryptoThrow(ctx, err)
		}
		return ctx.NewArrayBuffer(signature)
	case "ED25519":
		if key.Algorithm != name || len(key.EdPrivate) != ed25519.PrivateKeySize {
			return cryptoThrow(ctx, errors.New("Ed25519 private key is required"))
		}
		signature, err := ed25519SignBytes(key.EdPrivate, data)
		if err != nil {
			return cryptoThrow(ctx, err)
		}
		return ctx.NewArrayBuffer(signature)
	default:
		return cryptoThrow(ctx, fmt.Errorf("unsupported sign algorithm: %s", name))
	}
}

func (s *cryptoState) verify(ctx *quickjs.Context, args []*quickjs.Value) *quickjs.Value {
	if len(args) < 4 {
		return ctx.ThrowTypeError("crypto.subtle.verify requires algorithm, key, signature, and data")
	}
	name, algorithm, err := algorithmName(args[0])
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	key, err := s.keyFromValue(ctx, args[1])
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	if !hasUsage(key, "verify") {
		return cryptoThrow(ctx, errors.New("key is not permitted for verify"))
	}
	signature, err := readBufferSource(ctx, args[2])
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	data, err := readBufferSource(ctx, args[3])
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	switch name {
	case "HMAC":
		if key.Algorithm != "HMAC" {
			return cryptoThrow(ctx, errors.New("HMAC key is required"))
		}
		hashName := key.Hash
		expected, err := hmacBytes(hashName, key.Secret, data)
		if err != nil {
			return cryptoThrow(ctx, err)
		}
		return ctx.NewBool(equalBytes(expected, signature))
	case "RSASSA-PKCS1-V1_5", "RSA-PSS":
		if key.Algorithm != name || key.RSAPublic == nil {
			return cryptoThrow(ctx, errors.New("RSA public key is required"))
		}
		hashName := key.Hash
		saltLength := -1
		if name == "RSA-PSS" {
			saltLength = intProperty(algorithm, "saltLength", -1)
		}
		err = rsaVerifyBytes(name, hashName, saltLength, key.RSAPublic, signature, data)
		if err == rsa.ErrVerification {
			return ctx.NewBool(false)
		}
		if err != nil {
			return cryptoThrow(ctx, err)
		}
		return ctx.NewBool(true)
	case "ECDSA":
		if key.Algorithm != name || key.ECDSAPublic == nil {
			return cryptoThrow(ctx, errors.New("ECDSA public key is required"))
		}
		hashName, failure := requiredOperationHash(ctx, algorithm)
		if failure != nil {
			return failure
		}
		valid, err := ecdsaVerifyBytes(hashName, key.ECDSAPublic, signature, data)
		if err != nil {
			return ctx.NewBool(false)
		}
		return ctx.NewBool(valid)
	case "ED25519":
		if key.Algorithm != name || len(key.EdPublic) != ed25519.PublicKeySize {
			return cryptoThrow(ctx, errors.New("Ed25519 public key is required"))
		}
		valid, err := ed25519VerifyBytes(key.EdPublic, signature, data)
		if err != nil {
			return cryptoThrow(ctx, err)
		}
		return ctx.NewBool(valid)
	default:
		return cryptoThrow(ctx, fmt.Errorf("unsupported verify algorithm: %s", name))
	}
}

func (s *cryptoState) encrypt(ctx *quickjs.Context, args []*quickjs.Value) *quickjs.Value {
	return s.crypt(ctx, args, true)
}

func (s *cryptoState) decrypt(ctx *quickjs.Context, args []*quickjs.Value) *quickjs.Value {
	return s.crypt(ctx, args, false)
}

func (s *cryptoState) crypt(ctx *quickjs.Context, args []*quickjs.Value, encrypt bool) *quickjs.Value {
	if len(args) < 3 {
		return ctx.ThrowTypeError("crypto.subtle.encrypt/decrypt requires algorithm, key, and data")
	}
	name, algorithm, err := algorithmName(args[0])
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	key, err := s.keyFromValue(ctx, args[1])
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	usage := "decrypt"
	if encrypt {
		usage = "encrypt"
	}
	if !hasUsage(key, usage) {
		isKeyWrap := name == "AES-KW" || name == "RSA-OAEP" || name == "RSAES-PKCS1-V1_5"
		if !(isKeyWrap && ((encrypt && hasUsage(key, "wrapKey")) || (!encrypt && hasUsage(key, "unwrapKey")))) {
			return cryptoThrow(ctx, fmt.Errorf("key is not permitted for %s", usage))
		}
	}
	data, err := readBufferSource(ctx, args[2])
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	var result []byte
	if name == "RSA-OAEP" || name == "RSAES-PKCS1-V1_5" {
		if key.Algorithm != name {
			return cryptoThrow(ctx, errors.New("RSA key algorithm does not match operation"))
		}
		if encrypt && key.RSAPublic == nil {
			return cryptoThrow(ctx, errors.New("RSA public key is required for encryption"))
		}
		if !encrypt && key.RSAPrivate == nil {
			return cryptoThrow(ctx, errors.New("RSA private key is required for decryption"))
		}
		hashName := key.Hash
		labelValue := algorithmProperty(algorithm, "label")
		label, labelErr := readOptionalBufferSource(ctx, labelValue)
		if labelValue != nil {
			labelValue.Free()
		}
		if labelErr != nil {
			return cryptoThrow(ctx, labelErr)
		}
		if encrypt {
			result, err = rsaEncryptBytes(name, hashName, label, key.RSAPublic, data)
		} else {
			result, err = rsaDecryptBytes(name, hashName, label, key.RSAPrivate, data)
		}
		if err != nil {
			return cryptoThrow(ctx, err)
		}
		return ctx.NewArrayBuffer(result)
	}
	result, err = cryptBytes(name, algorithm, key.Secret, data, encrypt)
	if err != nil {
		return cryptoOperationError(ctx, err.Error())
	}
	return ctx.NewArrayBuffer(result)
}

func (s *cryptoState) deriveBits(ctx *quickjs.Context, args []*quickjs.Value) *quickjs.Value {
	if len(args) < 3 {
		return ctx.ThrowTypeError("crypto.subtle.deriveBits requires algorithm, baseKey, and length")
	}
	algorithm, algorithmObject, err := algorithmName(args[0])
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	key, err := s.keyFromValue(ctx, args[1])
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	if !hasUsage(key, "deriveBits") {
		return cryptoThrow(ctx, errors.New("key is not permitted for deriveBits"))
	}
	lengthBits := args[2].ToInt64()
	if lengthBits < 0 || lengthBits%8 != 0 {
		return cryptoOperationError(ctx, "deriveBits length must be a non-negative multiple of 8")
	}
	if algorithm == "PBKDF2" {
		if maxBytes := s.resourceLimits.Config().MaxPBKDF2OutputBytes; maxBytes > 0 && lengthBits/8 > int64(maxBytes) {
			return cryptoOperationError(ctx, "PBKDF2 output exceeds configured byte limit")
		}
		if maxIterations := s.resourceLimits.Config().MaxPBKDF2Iterations; maxIterations > 0 && intProperty(algorithmObject, "iterations", 0) > maxIterations {
			return cryptoOperationError(ctx, "PBKDF2 iterations exceed configured limit")
		}
	}
	var result []byte
	if algorithm == "ECDH" || algorithm == "X25519" {
		result, err = s.deriveECDH(ctx, algorithmObject, key, int(lengthBits/8))
	} else {
		result, err = deriveMaterial(algorithm, algorithmObject, key.Secret, int(lengthBits/8))
	}
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	return ctx.NewArrayBuffer(result)
}

func (s *cryptoState) deriveKey(ctx *quickjs.Context, args []*quickjs.Value) *quickjs.Value {
	if len(args) < 5 {
		return ctx.ThrowTypeError("crypto.subtle.deriveKey requires algorithm, baseKey, derivedKeyAlgorithm, extractable, and keyUsages")
	}
	algorithm, algorithmObject, err := algorithmName(args[0])
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	base, err := s.keyFromValue(ctx, args[1])
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	if !hasUsage(base, "deriveKey") {
		return cryptoThrow(ctx, errors.New("key is not permitted for deriveKey"))
	}
	derivedName, derivedObject, err := algorithmName(args[2])
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	length := intProperty(derivedObject, "length", 0)
	if length <= 0 || length%8 != 0 {
		return cryptoOperationError(ctx, "derived key length must be a positive multiple of 8")
	}
	var material []byte
	if algorithm == "ECDH" || algorithm == "X25519" {
		material, err = s.deriveECDH(ctx, algorithmObject, base, length/8)
	} else {
		material, err = deriveMaterial(algorithm, algorithmObject, base.Secret, length/8)
	}
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	usages, err := usageList(args[4])
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	if err := validateKeyUsages(derivedName, keyTypeForAlgorithm(derivedName), usages); err != nil {
		return cryptoThrow(ctx, err)
	}
	key := &cryptoKey{Type: keyTypeForAlgorithm(derivedName), Algorithm: derivedName, Length: length, Extractable: args[3].ToBool(), Usages: usages, Secret: material}
	if derivedName == "HMAC" {
		key.Hash, err = algorithmHash(algorithmProperty(derivedObject, "hash"), "SHA-256")
		if err != nil {
			return cryptoThrow(ctx, err)
		}
	}
	return s.addKey(ctx, key)
}

func (s *cryptoState) wrapKey(ctx *quickjs.Context, args []*quickjs.Value) *quickjs.Value {
	if len(args) < 4 {
		return ctx.ThrowTypeError("crypto.subtle.wrapKey requires format, key, wrappingKey, and algorithm")
	}
	key, err := s.keyFromValue(ctx, args[1])
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	if err := ensureExtractable(key); err != nil {
		return cryptoThrow(ctx, err)
	}
	format := strings.ToLower(strings.TrimSpace(args[0].ToString()))
	exported := s.exportKey(ctx, []*quickjs.Value{args[0], args[1]})
	if exported == nil || exported.IsException() {
		return exported
	}
	var data []byte
	if format == "jwk" {
		var objectValue any
		if err := ctx.Unmarshal(exported, &objectValue); err != nil {
			exported.Free()
			return cryptoThrow(ctx, err)
		}
		exported.Free()
		data, err = json.Marshal(objectValue)
	} else {
		data, err = readBufferSource(ctx, exported)
		exported.Free()
	}
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	dataValue := ctx.NewArrayBuffer(data)
	result := s.crypt(ctx, []*quickjs.Value{args[3], args[2], dataValue}, true)
	dataValue.Free()
	return result
}

func (s *cryptoState) unwrapKey(ctx *quickjs.Context, args []*quickjs.Value) *quickjs.Value {
	if len(args) < 7 {
		return ctx.ThrowTypeError("crypto.subtle.unwrapKey requires format, wrappedKey, unwrappingKey, unwrapAlgorithm, unwrappedKeyAlgorithm, extractable, and keyUsages")
	}
	decrypted := s.crypt(ctx, []*quickjs.Value{args[3], args[2], args[1]}, false)
	if decrypted == nil || decrypted.IsException() {
		return decrypted
	}
	defer decrypted.Free()
	format := strings.ToLower(strings.TrimSpace(args[0].ToString()))
	if format != "jwk" {
		return s.importKey(ctx, []*quickjs.Value{args[0], decrypted, args[4], args[5], args[6]})
	}
	data, err := readBufferSource(ctx, decrypted)
	if err != nil {
		return cryptoThrow(ctx, err)
	}
	keyData := ctx.ParseJSON(string(data))
	if keyData == nil {
		return cryptoThrow(ctx, errors.New("parse wrapped JWK"))
	}
	if keyData.IsException() {
		return keyData
	}
	defer keyData.Free()
	return s.importKey(ctx, []*quickjs.Value{args[0], keyData, args[4], args[5], args[6]})
}

func algorithmProperty(algorithm *quickjs.Value, name string) *quickjs.Value {
	if algorithm == nil {
		return nil
	}
	return algorithm.Get(name)
}

func operationHashName(algorithm *quickjs.Value, fallback string) (string, error) {
	hashValue := algorithmProperty(algorithm, "hash")
	if hashValue != nil {
		defer hashValue.Free()
	}
	return algorithmHash(hashValue, fallback)
}

func requiredOperationHash(ctx *quickjs.Context, algorithm *quickjs.Value) (string, *quickjs.Value) {
	hashValue := algorithmProperty(algorithm, "hash")
	if hashValue == nil {
		return "", ctx.ThrowTypeError("algorithm.hash is required")
	}
	defer hashValue.Free()
	if hashValue.IsUndefined() || hashValue.IsNull() {
		return "", ctx.ThrowTypeError("algorithm.hash is required")
	}
	hashName, err := algorithmHash(hashValue, "")
	if err != nil || hashName == "" {
		if err == nil {
			err = errors.New("algorithm.hash.name is required")
		}
		return "", ctx.ThrowTypeError(err.Error())
	}
	return hashName, nil
}

func intProperty(value *quickjs.Value, name string, fallback int64) int {
	if value == nil {
		return int(fallback)
	}
	property := value.Get(name)
	if property == nil {
		return int(fallback)
	}
	defer property.Free()
	if property.IsUndefined() || property.IsNull() {
		return int(fallback)
	}
	return int(property.ToInt64())
}

func requiredIntegerProperty(value *quickjs.Value, name string, minimum, maximum int) (int, error) {
	property := algorithmProperty(value, name)
	if property == nil {
		return 0, fmt.Errorf("%s is required", name)
	}
	defer property.Free()
	if property.IsException() || !property.IsNumber() {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	number := property.ToFloat64()
	if math.IsNaN(number) || math.IsInf(number, 0) || number != math.Trunc(number) || number < float64(minimum) || number > float64(maximum) {
		return 0, fmt.Errorf("%s must be between %d and %d", name, minimum, maximum)
	}
	return int(number), nil
}

func deriveMaterial(name string, algorithm *quickjs.Value, secret []byte, length int) ([]byte, error) {
	switch name {
	case "PBKDF2":
		saltValue := algorithmProperty(algorithm, "salt")
		salt, err := readBufferSource(algorithmContext(algorithm), saltValue)
		if saltValue != nil {
			saltValue.Free()
		}
		if err != nil {
			return nil, err
		}
		iterations := intProperty(algorithm, "iterations", 0)
		hashValue := algorithmProperty(algorithm, "hash")
		hashName, err := algorithmHash(hashValue, "SHA-256")
		if hashValue != nil {
			hashValue.Free()
		}
		if err != nil {
			return nil, err
		}
		return pbkdf2Bytes(hashName, secret, salt, iterations, length)
	case "HKDF":
		saltValue := algorithmProperty(algorithm, "salt")
		infoValue := algorithmProperty(algorithm, "info")
		ctx := algorithmContext(algorithm)
		salt, saltErr := readBufferSource(ctx, saltValue)
		info, infoErr := readBufferSource(ctx, infoValue)
		if saltValue != nil {
			saltValue.Free()
		}
		if saltErr != nil {
			return nil, saltErr
		}
		if infoErr != nil {
			return nil, infoErr
		}
		hashValue := algorithmProperty(algorithm, "hash")
		hashName, err := algorithmHash(hashValue, "SHA-256")
		if hashValue != nil {
			hashValue.Free()
		}
		if err != nil {
			return nil, err
		}
		return hkdfBytes(hashName, secret, salt, info, length)
	default:
		return nil, fmt.Errorf("unsupported derive algorithm: %s", name)
	}
}

func algorithmContext(value *quickjs.Value) *quickjs.Context {
	if value == nil {
		return nil
	}
	return value.Context()
}

func cryptBytes(name string, algorithm *quickjs.Value, key, data []byte, encrypt bool) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	switch name {
	case "AES-GCM":
		ivValue := algorithmProperty(algorithm, "iv")
		iv, err := readBufferSource(algorithmContext(algorithm), ivValue)
		if ivValue != nil {
			ivValue.Free()
		}
		if err != nil {
			return nil, err
		}
		tagLength := intProperty(algorithm, "tagLength", 128)
		switch tagLength {
		case 32, 64, 96, 104, 112, 120, 128:
		default:
			return nil, errors.New("AES-GCM tagLength is invalid")
		}
		var gcm cipher.AEAD
		if len(iv) == 12 {
			gcm, err = cipher.NewGCMWithTagSize(block, tagLength/8)
		} else if tagLength == 128 {
			gcm, err = cipher.NewGCMWithNonceSize(block, len(iv))
		} else {
			return nil, errors.New("AES-GCM non-12-byte iv requires tagLength 128")
		}
		if err != nil {
			return nil, err
		}
		if len(iv) == 0 {
			return nil, errors.New("AES-GCM iv is required")
		}
		aadValue := algorithmProperty(algorithm, "additionalData")
		aad, aadErr := readOptionalBufferSource(algorithmContext(algorithm), aadValue)
		if aadValue != nil {
			aadValue.Free()
		}
		if aadErr != nil {
			return nil, aadErr
		}
		if encrypt {
			return gcm.Seal(nil, iv, data, aad), nil
		}
		return gcm.Open(nil, iv, data, aad)
	case "AES-KW":
		if encrypt {
			return aesKeyWrap(block, data)
		}
		return aesKeyUnwrap(block, data)
	case "AES-CBC":
		ivValue := algorithmProperty(algorithm, "iv")
		iv, err := readBufferSource(algorithmContext(algorithm), ivValue)
		if ivValue != nil {
			ivValue.Free()
		}
		if err != nil {
			return nil, err
		}
		if len(iv) != aes.BlockSize {
			return nil, errors.New("AES-CBC iv must be 16 bytes")
		}
		if encrypt {
			data = pkcs7Pad(data, aes.BlockSize)
		} else if len(data) == 0 || len(data)%aes.BlockSize != 0 {
			return nil, errors.New("AES-CBC ciphertext has invalid length")
		}
		out := make([]byte, len(data))
		if encrypt {
			cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, data)
			return out, nil
		}
		cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, data)
		return pkcs7Unpad(out, aes.BlockSize)
	case "AES-CTR":
		counterValue := algorithmProperty(algorithm, "counter")
		counter, err := readBufferSource(algorithmContext(algorithm), counterValue)
		if counterValue != nil {
			counterValue.Free()
		}
		if err != nil {
			return nil, err
		}
		counterBits, err := requiredIntegerProperty(algorithm, "length", 1, aes.BlockSize*8)
		if err != nil {
			return nil, fmt.Errorf("AES-CTR %w", err)
		}
		return aesCTRBytes(block, counter, counterBits, data)
	default:
		return nil, fmt.Errorf("unsupported encryption algorithm: %s", name)
	}
}

func readOptionalBufferSource(ctx *quickjs.Context, value *quickjs.Value) ([]byte, error) {
	if value == nil || value.IsUndefined() || value.IsNull() {
		return nil, nil
	}
	return readBufferSource(ctx, value)
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	out := make([]byte, len(data)+padding)
	copy(out, data)
	for index := len(data); index < len(out); index++ {
		out[index] = byte(padding)
	}
	return out
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, errors.New("invalid PKCS#7 padding")
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > blockSize || padding > len(data) {
		return nil, errors.New("invalid PKCS#7 padding")
	}
	for _, value := range data[len(data)-padding:] {
		if value != byte(padding) {
			return nil, errors.New("invalid PKCS#7 padding")
		}
	}
	return append([]byte(nil), data[:len(data)-padding]...), nil
}
