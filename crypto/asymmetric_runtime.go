package crypto

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/ed25519"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"math/big"

	quickjs "github.com/buke/quickjs-go"
)

func (s *cryptoState) generateAsymmetricKey(ctx *quickjs.Context, name string, algorithm *quickjs.Value, extractable bool, usages []string) *quickjs.Value {
	publicUsages, privateUsages := splitGeneratedUsages(name, usages)
	switch name {
	case "RSASSA-PKCS1-V1_5", "RSA-PSS", "RSA-OAEP", "RSAES-PKCS1-V1_5":
		modulusLength := intProperty(algorithm, "modulusLength", 0)
		if modulusLength < 512 || modulusLength%8 != 0 {
			return cryptoOperationError(ctx, "RSA modulusLength must be a multiple of 8 and at least 512 bits")
		}
		publicExponent, err := rsaPublicExponent(ctx, algorithm)
		if err != nil {
			return cryptoThrow(ctx, err)
		}
		private, err := generateRSAKeyWithExponent(modulusLength, publicExponent)
		if err != nil {
			return cryptoThrow(ctx, err)
		}
		hashValue := algorithmProperty(algorithm, "hash")
		hashName, err := algorithmHash(hashValue, "SHA-256")
		if hashValue != nil {
			hashValue.Free()
		}
		if err != nil {
			return cryptoThrow(ctx, err)
		}
		publicKey := &cryptoKey{Type: "public", Algorithm: name, Hash: hashName, Extractable: true, Usages: publicUsages, RSAPublic: &private.PublicKey}
		privateKey := &cryptoKey{Type: "private", Algorithm: name, Hash: hashName, Extractable: extractable, Usages: privateUsages, RSAPrivate: private}
		return s.keyPairValue(ctx, publicKey, privateKey)
	case "ECDSA", "ECDH":
		curveName, err := algorithmCurveName(algorithm)
		if err != nil {
			return cryptoThrow(ctx, err)
		}
		curve, normalized, err := namedCurveByName(curveName)
		if err != nil {
			return cryptoThrow(ctx, err)
		}
		if name == "ECDSA" {
			hashName, err := operationHashName(algorithm, "SHA-256")
			if err != nil {
				return cryptoThrow(ctx, err)
			}
			private, err := ecdsa.GenerateKey(curve, cryptorand.Reader)
			if err != nil {
				return cryptoThrow(ctx, err)
			}
			publicKey := &cryptoKey{Type: "public", Algorithm: name, Hash: hashName, NamedCurve: normalized, Extractable: true, Usages: publicUsages, ECDSAPublic: &private.PublicKey}
			privateKey := &cryptoKey{Type: "private", Algorithm: name, Hash: hashName, NamedCurve: normalized, Extractable: extractable, Usages: privateUsages, ECDSAPrivate: private}
			return s.keyPairValue(ctx, publicKey, privateKey)
		}
		ecdhCurve, err := ecdhCurveByElliptic(curve)
		if err != nil {
			return cryptoThrow(ctx, err)
		}
		private, err := ecdhCurve.GenerateKey(cryptorand.Reader)
		if err != nil {
			return cryptoThrow(ctx, err)
		}
		publicKey := &cryptoKey{Type: "public", Algorithm: name, NamedCurve: normalized, Extractable: true, Usages: publicUsages, ECDHPublic: private.PublicKey()}
		privateKey := &cryptoKey{Type: "private", Algorithm: name, NamedCurve: normalized, Extractable: extractable, Usages: privateUsages, ECDHPrivate: private}
		return s.keyPairValue(ctx, publicKey, privateKey)
	case "ED25519":
		public, private, err := ed25519.GenerateKey(cryptorand.Reader)
		if err != nil {
			return cryptoThrow(ctx, err)
		}
		publicKey := &cryptoKey{Type: "public", Algorithm: name, Extractable: true, Usages: publicUsages, EdPublic: append(ed25519.PublicKey(nil), public...)}
		privateKey := &cryptoKey{Type: "private", Algorithm: name, Extractable: extractable, Usages: privateUsages, EdPrivate: append(ed25519.PrivateKey(nil), private...)}
		return s.keyPairValue(ctx, publicKey, privateKey)
	case "X25519":
		private, err := ecdh.X25519().GenerateKey(cryptorand.Reader)
		if err != nil {
			return cryptoThrow(ctx, err)
		}
		publicKey := &cryptoKey{Type: "public", Algorithm: name, Extractable: true, Usages: publicUsages, XPublic: private.PublicKey()}
		privateKey := &cryptoKey{Type: "private", Algorithm: name, Extractable: extractable, Usages: privateUsages, XPrivate: private}
		return s.keyPairValue(ctx, publicKey, privateKey)
	default:
		return cryptoThrow(ctx, fmt.Errorf("unsupported generateKey algorithm: %s", name))
	}
}

func splitGeneratedUsages(name string, usages []string) ([]string, []string) {
	publicUsages := make([]string, 0, len(usages))
	privateUsages := make([]string, 0, len(usages))
	for _, usage := range usages {
		switch name {
		case "RSASSA-PKCS1-V1_5", "RSA-PSS", "ECDSA", "ED25519":
			if usage == "verify" {
				publicUsages = append(publicUsages, usage)
			}
			if usage == "sign" {
				privateUsages = append(privateUsages, usage)
			}
		case "RSA-OAEP", "RSAES-PKCS1-V1_5":
			if usage == "encrypt" || usage == "wrapKey" {
				publicUsages = append(publicUsages, usage)
			}
			if usage == "decrypt" || usage == "unwrapKey" {
				privateUsages = append(privateUsages, usage)
			}
		case "ECDH", "X25519":
			if usage == "deriveBits" || usage == "deriveKey" {
				privateUsages = append(privateUsages, usage)
			}
		}
	}
	return publicUsages, privateUsages
}

func rsaPublicExponent(ctx *quickjs.Context, algorithm *quickjs.Value) (int, error) {
	value := algorithmProperty(algorithm, "publicExponent")
	if value == nil || value.IsUndefined() || value.IsNull() {
		if value != nil {
			value.Free()
		}
		return 65537, nil
	}
	data, err := readBufferSource(ctx, value)
	value.Free()
	if err != nil {
		return 0, err
	}
	if len(data) == 0 || len(data) > 8 {
		return 0, errors.New("RSA publicExponent must be a non-empty integer")
	}
	var exponent uint64
	for _, byteValue := range data {
		exponent = (exponent << 8) | uint64(byteValue)
	}
	if exponent > uint64(^uint(0)>>1) {
		return 0, errors.New("RSA publicExponent is too large")
	}
	result := int(exponent)
	if result < 3 || result%2 == 0 {
		return 0, errors.New("RSA publicExponent must be an odd integer greater than one")
	}
	return result, nil
}

func generateRSAKeyWithExponent(modulusLength, exponent int) (*rsa.PrivateKey, error) {
	if exponent == 65537 {
		return rsa.GenerateKey(cryptorand.Reader, modulusLength)
	}
	one := big.NewInt(1)
	exponentValue := big.NewInt(int64(exponent))
	pBits := modulusLength / 2
	qBits := modulusLength - pBits
	for attempt := 0; attempt < 256; attempt++ {
		p, err := cryptorand.Prime(cryptorand.Reader, pBits)
		if err != nil {
			return nil, err
		}
		q, err := cryptorand.Prime(cryptorand.Reader, qBits)
		if err != nil {
			return nil, err
		}
		if p.Cmp(q) == 0 {
			continue
		}
		n := new(big.Int).Mul(p, q)
		if n.BitLen() != modulusLength {
			continue
		}
		pMinusOne := new(big.Int).Sub(p, one)
		qMinusOne := new(big.Int).Sub(q, one)
		if new(big.Int).GCD(nil, nil, exponentValue, pMinusOne).Cmp(one) != 0 ||
			new(big.Int).GCD(nil, nil, exponentValue, qMinusOne).Cmp(one) != 0 {
			continue
		}
		phi := new(big.Int).Mul(pMinusOne, qMinusOne)
		privateExponent := new(big.Int).ModInverse(exponentValue, phi)
		if privateExponent == nil {
			continue
		}
		private := &rsa.PrivateKey{
			PublicKey: rsa.PublicKey{N: n, E: exponent},
			D:         privateExponent,
			Primes:    []*big.Int{p, q},
		}
		if err := private.Validate(); err != nil {
			continue
		}
		private.Precompute()
		return private, nil
	}
	return nil, errors.New("failed to generate RSA key with requested publicExponent")
}

func (s *cryptoState) keyPairValue(ctx *quickjs.Context, publicKey, privateKey *cryptoKey) *quickjs.Value {
	publicValue := s.addKey(ctx, publicKey)
	if publicValue == nil {
		return ctx.ThrowInternalError("create public CryptoKey")
	}
	privateValue := s.addKey(ctx, privateKey)
	if privateValue == nil {
		publicValue.Free()
		return ctx.ThrowInternalError("create private CryptoKey")
	}
	pair := ctx.NewObject()
	if pair == nil {
		publicValue.Free()
		privateValue.Free()
		return ctx.ThrowInternalError("create CryptoKeyPair")
	}
	pair.Set("publicKey", publicValue)
	pair.Set("privateKey", privateValue)
	return pair
}
