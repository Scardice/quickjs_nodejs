package crypto

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Scardice/quickjs_nodejs/limits"
	quickjs "github.com/buke/quickjs-go"
)

type cryptoKey struct {
	Type        string
	Algorithm   string
	Hash        string
	NamedCurve  string
	Length      int
	Extractable bool
	Usages      []string
	Secret      []byte

	RSAPublic    *rsa.PublicKey
	RSAPrivate   *rsa.PrivateKey
	ECDSAPublic  *ecdsa.PublicKey
	ECDSAPrivate *ecdsa.PrivateKey
	ECDHPublic   *ecdh.PublicKey
	ECDHPrivate  *ecdh.PrivateKey
	EdPublic     ed25519.PublicKey
	EdPrivate    ed25519.PrivateKey
	XPublic      *ecdh.PublicKey
	XPrivate     *ecdh.PrivateKey
}

type cryptoState struct {
	keys           map[string]*cryptoKey
	keyStore       *quickjs.Value
	next           uint64
	resourceLimits *limits.Runtime
}

func newCryptoState(ctx *quickjs.Context, resourceLimits *limits.Runtime) (*cryptoState, error) {
	if ctx == nil {
		return nil, errors.New("crypto: nil context")
	}
	keyStore := ctx.Eval("new WeakMap()")
	if keyStore == nil || keyStore.IsException() {
		if keyStore != nil {
			keyStore.Free()
		}
		return nil, errors.New("crypto: create key store")
	}
	return &cryptoState{
		keys:           make(map[string]*cryptoKey),
		keyStore:       keyStore,
		resourceLimits: resourceLimits,
	}, nil
}

func (s *cryptoState) Close() error {
	if s == nil {
		return nil
	}
	if s.keyStore != nil {
		s.keyStore.Free()
		s.keyStore = nil
	}
	clear(s.keys)
	return nil
}

func (s *cryptoState) addKey(ctx *quickjs.Context, key *cryptoKey) *quickjs.Value {

	if s == nil || key == nil || ctx == nil || s.keyStore == nil {
		return nil
	}
	s.next++
	id := strconv.FormatUint(s.next, 10)
	s.keys[id] = key

	object := ctx.NewObject()
	if object == nil {
		delete(s.keys, id)
		return nil
	}
	if !defineReadOnlyProperty(object, "type", ctx.NewString(key.Type)) ||
		!defineReadOnlyProperty(object, "extractable", ctx.NewBool(key.Extractable)) {
		object.Free()
		delete(s.keys, id)
		return nil
	}
	algorithm := keyAlgorithmValue(ctx, key)
	if algorithm == nil || !defineReadOnlyProperty(object, "algorithm", algorithm) {
		object.Free()
		delete(s.keys, id)
		return nil
	}
	usages := stringArray(ctx, key.Usages)
	if usages == nil || !freezeValue(ctx, usages) {
		if usages != nil {
			usages.Free()
		}
		object.Free()
		delete(s.keys, id)
		return nil
	}
	if !defineReadOnlyProperty(object, "usages", usages) {
		object.Free()
		delete(s.keys, id)
		return nil
	}

	handle := ctx.NewString(id)
	if handle == nil {
		object.Free()
		delete(s.keys, id)
		return nil
	}
	stored := s.keyStore.Call("set", object, handle)
	handle.Free()
	if stored == nil || stored.IsException() {
		if stored != nil {
			stored.Free()
		}
		object.Free()
		delete(s.keys, id)
		return nil
	}
	stored.Free()
	return object
}

func defineReadOnlyProperty(object *quickjs.Value, name string, value *quickjs.Value) bool {
	if value == nil {
		return false
	}
	defer value.Free()
	return object.DefineProperty(name, quickjs.PropertyDescriptor{
		Value: value,
		Flags: quickjs.PropHasValue |
			quickjs.PropHasWritable |
			quickjs.PropHasConfigurable |
			quickjs.PropHasEnumerable |
			quickjs.PropEnumerable,
	})
}

func freezeValue(ctx *quickjs.Context, value *quickjs.Value) bool {
	object := ctx.Globals().Get("Object")
	if object == nil {
		return false
	}
	defer object.Free()
	freeze := object.Get("freeze")
	if freeze == nil {
		return false
	}
	defer freeze.Free()
	result := freeze.Execute(object, value)
	if result == nil {
		return false
	}
	defer result.Free()
	return !result.IsException()
}

func keyAlgorithmValue(ctx *quickjs.Context, key *cryptoKey) *quickjs.Value {
	algorithm := ctx.NewObject()
	if algorithm == nil {
		return nil
	}
	displayName := key.Algorithm
	if displayName == "ED25519" {
		displayName = "Ed25519"
	}
	algorithm.Set("name", ctx.NewString(displayName))
	if key.Length > 0 {
		algorithm.Set("length", ctx.NewInt32(int32(key.Length)))
	}
	if key.Hash != "" && key.Algorithm != "ECDSA" {
		hash := ctx.NewObject()
		hash.Set("name", ctx.NewString(key.Hash))
		algorithm.Set("hash", hash)
	}
	if key.NamedCurve != "" {
		algorithm.Set("namedCurve", ctx.NewString(key.NamedCurve))
	}
	return algorithm
}

func stringArray(ctx *quickjs.Context, values []string) *quickjs.Value {
	array := ctx.Eval("[]")
	if array == nil || array.IsException() {
		return array
	}
	for index, value := range values {
		array.Set(strconv.Itoa(index), ctx.NewString(value))
	}
	array.Set("length", ctx.NewInt32(int32(len(values))))
	return array
}

func (s *cryptoState) keyFromValue(ctx *quickjs.Context, value *quickjs.Value) (*cryptoKey, error) {
	if !s.isKey(ctx, value) {
		return nil, errors.New("invalid CryptoKey")
	}
	handle := s.keyStore.Call("get", value)
	if handle == nil {
		return nil, errors.New("invalid CryptoKey")
	}
	defer handle.Free()
	if handle.IsException() || handle.IsUndefined() || handle.IsNull() || !handle.IsString() {
		return nil, errors.New("invalid CryptoKey")
	}
	key := s.keys[handle.ToString()]
	if key == nil {
		return nil, errors.New("invalid or expired CryptoKey")
	}
	return key, nil
}

func (s *cryptoState) isKey(ctx *quickjs.Context, value *quickjs.Value) bool {
	if s == nil || ctx == nil || value == nil || !value.IsObject() || value.Context() != ctx || s.keyStore == nil {
		return false
	}
	registered := s.keyStore.Call("has", value)
	if registered == nil {
		return false
	}
	defer registered.Free()
	return !registered.IsException() && registered.ToBool()
}

func algorithmName(value *quickjs.Value) (string, *quickjs.Value, error) {
	if value == nil || value.IsUndefined() || value.IsNull() {
		return "", nil, errors.New("algorithm is required")
	}
	if value.IsString() {
		return normalizeName(value.ToString()), nil, nil
	}
	if !value.IsObject() {
		return "", nil, errors.New("algorithm must be a string or object")
	}
	name, ok := readStringProperty(value, "name")
	if !ok || strings.TrimSpace(name) == "" {
		return "", nil, errors.New("algorithm.name is required")
	}
	return normalizeName(name), value, nil
}

func algorithmHash(value *quickjs.Value, fallback string) (string, error) {
	if value == nil || value.IsUndefined() || value.IsNull() {
		return fallback, nil
	}
	if value.IsString() {
		return normalizeName(value.ToString()), nil
	}
	name, ok := readStringProperty(value, "name")
	if !ok {
		return "", errors.New("algorithm.hash.name is required")
	}
	return normalizeName(name), nil
}

func usageList(value *quickjs.Value) ([]string, error) {
	if value == nil || value.IsUndefined() || value.IsNull() {
		return nil, nil
	}
	items, err := arrayValues(value)
	if err != nil {
		return nil, err
	}
	usages := make([]string, 0, len(items))
	for _, item := range items {
		if item == nil || !item.IsString() {
			for _, value := range items {
				if value != nil {
					value.Free()
				}
			}
			return nil, errors.New("key usage must be a string")
		}
		usages = append(usages, item.ToString())
	}
	for _, item := range items {
		item.Free()
	}
	return usages, nil
}

func validateKeyUsages(algorithm, keyType string, usages []string) error {
	seen := make(map[string]struct{}, len(usages))
	for _, usage := range usages {
		if _, duplicate := seen[usage]; duplicate {
			return fmt.Errorf("duplicate key usage: %s", usage)
		}
		seen[usage] = struct{}{}
		if !keyUsageAllowed(algorithm, keyType, usage) {
			return fmt.Errorf("%s is not a valid usage for %s %s key", usage, keyType, algorithm)
		}
	}
	return nil
}

func keyUsageAllowed(algorithm, keyType, usage string) bool {
	switch normalizeName(algorithm) {
	case "AES-CBC", "AES-CTR", "AES-GCM":
		return usage == "encrypt" || usage == "decrypt" || usage == "wrapKey" || usage == "unwrapKey"
	case "AES-KW":
		return usage == "wrapKey" || usage == "unwrapKey"
	case "HMAC":
		return usage == "sign" || usage == "verify"
	case "PBKDF2", "HKDF":
		return usage == "deriveKey" || usage == "deriveBits"
	case "RSASSA-PKCS1-V1_5", "RSA-PSS", "ECDSA", "ED25519":
		if keyType == "public" {
			return usage == "verify"
		}
		if keyType == "private" {
			return usage == "sign"
		}
		return usage == "sign" || usage == "verify"
	case "RSA-OAEP", "RSAES-PKCS1-V1_5":
		if keyType == "public" {
			return usage == "encrypt" || usage == "wrapKey"
		}
		if keyType == "private" {
			return usage == "decrypt" || usage == "unwrapKey"
		}
		return usage == "encrypt" || usage == "decrypt" || usage == "wrapKey" || usage == "unwrapKey"
	case "ECDH", "X25519":
		return keyType != "public" && (usage == "deriveKey" || usage == "deriveBits")
	default:
		return false
	}
}

func hasUsage(key *cryptoKey, usage string) bool {
	if key == nil {
		return false
	}
	for _, candidate := range key.Usages {
		if candidate == usage {
			return true
		}
	}
	return false
}

func ensureExtractable(key *cryptoKey) error {
	if key == nil {
		return errors.New("invalid CryptoKey")
	}
	if !key.Extractable {
		return errors.New("key is not extractable")
	}
	return nil
}

func keyTypeForAlgorithm(name string) string {
	switch normalizeName(name) {
	case "HMAC", "AES-CBC", "AES-CTR", "AES-GCM", "AES-KW", "PBKDF2", "HKDF":
		return "secret"
	default:
		return "secret"
	}
}

func keyError(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}
