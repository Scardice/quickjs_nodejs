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
	"sync/atomic"

	quickjs "github.com/buke/quickjs-go"
)

const keyHandleSlot = "__quickjs_nodejs_crypto_key"

var nextStateID uint64

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
	id   string
	keys map[string]*cryptoKey
	next uint64
}

func newCryptoState() *cryptoState {
	return &cryptoState{
		id:   strconv.FormatUint(atomic.AddUint64(&nextStateID, 1), 10),
		keys: make(map[string]*cryptoKey),
	}
}

func (s *cryptoState) addKey(ctx *quickjs.Context, key *cryptoKey) *quickjs.Value {
	if s == nil || key == nil || ctx == nil {
		return nil
	}
	s.next++
	id := s.id + ":" + strconv.FormatUint(s.next, 10)
	s.keys[id] = key

	object := ctx.NewObject()
	if object == nil {
		delete(s.keys, id)
		return nil
	}
	object.Set("type", ctx.NewString(key.Type))
	object.Set("extractable", ctx.NewBool(key.Extractable))
	algorithm := keyAlgorithmValue(ctx, key)
	if algorithm == nil {
		object.Free()
		delete(s.keys, id)
		return nil
	}
	object.Set("algorithm", algorithm)
	object.Set("usages", stringArray(ctx, key.Usages))
	handle := ctx.NewString(id)
	if handle == nil || !object.DefinePropertyValue(keyHandleSlot, handle, quickjs.PropConfigurable) {
		if handle != nil {
			handle.Free()
		}
		object.Free()
		delete(s.keys, id)
		return nil
	}
	handle.Free()
	return object
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
	if key.Hash != "" {
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
	if s == nil || ctx == nil || value == nil || !value.IsObject() {
		return nil, errors.New("expected a CryptoKey")
	}
	if value.Context() != ctx {
		return nil, errors.New("CryptoKey belongs to a different context")
	}
	handle := value.Get(keyHandleSlot)
	if handle == nil {
		return nil, errors.New("invalid CryptoKey")
	}
	defer handle.Free()
	if handle.IsUndefined() || handle.IsNull() {
		return nil, errors.New("invalid CryptoKey")
	}
	id := handle.ToString()
	key := s.keys[id]
	if key == nil {
		return nil, errors.New("invalid or expired CryptoKey")
	}
	return key, nil
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
		if item == nil {
			continue
		}
		if item.IsString() {
			usages = append(usages, item.ToString())
		}
		item.Free()
	}
	return usages, nil
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
