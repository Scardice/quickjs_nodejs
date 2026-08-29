package crypto

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"strings"

	quickjs "github.com/buke/quickjs-go"
)

func importKeyMaterial(ctx *quickjs.Context, format string, value *quickjs.Value, algorithm string, algorithmObject *quickjs.Value, extractable bool, usages []string) (*cryptoKey, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case "raw":
		raw, err := readBufferSource(ctx, value)
		if err != nil {
			return nil, err
		}
		return importRawKeyMaterial(raw, algorithm, algorithmObject, extractable, usages)
	case "jwk":
		return importJWKMaterial(value, algorithm, algorithmObject, extractable, usages)
	case "pkcs8":
		raw, err := readBufferSource(ctx, value)
		if err != nil {
			return nil, err
		}
		key, err := parsePrivateKeyMaterial(raw)
		if err != nil {
			return nil, err
		}
		return privateKeyMaterial(key, algorithm, algorithmObject, extractable, usages)
	case "pkcs1":
		raw, err := readBufferSource(ctx, value)
		if err != nil {
			return nil, err
		}
		if key, err := x509.ParsePKCS1PrivateKey(raw); err == nil {
			return privateKeyMaterial(key, algorithm, algorithmObject, extractable, usages)
		}
		key, err := x509.ParsePKCS1PublicKey(raw)
		if err != nil {
			return nil, fmt.Errorf("failed to parse pkcs1 key: %w", err)
		}
		return publicKeyMaterial(key, algorithm, algorithmObject, usages)
	case "sec1":
		raw, err := readBufferSource(ctx, value)
		if err != nil {
			return nil, err
		}
		key, err := x509.ParseECPrivateKey(raw)
		if err != nil {
			return nil, fmt.Errorf("failed to parse sec1 key: %w", err)
		}
		return privateKeyMaterial(key, algorithm, algorithmObject, extractable, usages)
	case "spki":
		raw, err := readBufferSource(ctx, value)
		if err != nil {
			return nil, err
		}
		key, err := x509.ParsePKIXPublicKey(raw)
		if err != nil {
			return nil, fmt.Errorf("failed to parse spki key: %w", err)
		}
		return publicKeyMaterial(key, algorithm, algorithmObject, usages)
	default:
		return nil, fmt.Errorf("unsupported importKey format: %s", format)
	}
}

func importRawKeyMaterial(raw []byte, algorithm string, algorithmObject *quickjs.Value, extractable bool, usages []string) (*cryptoKey, error) {
	copyRaw := append([]byte(nil), raw...)
	switch algorithm {
	case "AES-CBC", "AES-CTR", "AES-GCM", "AES-KW":
		if len(copyRaw) != 16 && len(copyRaw) != 24 && len(copyRaw) != 32 {
			return nil, errors.New("AES raw key length must be 16, 24, or 32 bytes")
		}
		return &cryptoKey{Type: "secret", Algorithm: algorithm, Length: len(copyRaw) * 8, Extractable: extractable, Usages: append([]string(nil), usages...), Secret: copyRaw}, nil
	case "HMAC":
		hashName, err := algorithmHashName(algorithmObject, "SHA-256")
		if err != nil {
			return nil, err
		}
		if _, _, err := hashFactory(hashName); err != nil {
			return nil, err
		}
		length := len(copyRaw) * 8
		if declared := intProperty(algorithmObject, "length", int64(length)); declared > 0 {
			length = declared
		}
		return &cryptoKey{Type: "secret", Algorithm: algorithm, Hash: hashName, Length: length, Extractable: extractable, Usages: append([]string(nil), usages...), Secret: copyRaw}, nil
	case "PBKDF2", "HKDF":
		if len(copyRaw) == 0 {
			return nil, errors.New("raw key must not be empty")
		}
		return &cryptoKey{Type: "secret", Algorithm: algorithm, Length: len(copyRaw) * 8, Extractable: extractable, Usages: append([]string(nil), usages...), Secret: copyRaw}, nil
	case "ECDSA", "ECDH":
		curveName, err := algorithmCurveName(algorithmObject)
		if err != nil {
			return nil, err
		}
		curve, normalized, err := namedCurveByName(curveName)
		if err != nil {
			return nil, err
		}
		x, y := elliptic.Unmarshal(curve, copyRaw)
		if x == nil || y == nil {
			return nil, errors.New("EC raw key must be an uncompressed public point")
		}
		hashName := ""
		if algorithm == "ECDSA" {
			hashName, err = algorithmHashName(algorithmObject, "SHA-256")
			if err != nil {
				return nil, err
			}
		}
		if algorithm == "ECDSA" {
			return &cryptoKey{Type: "public", Algorithm: algorithm, Hash: hashName, NamedCurve: normalized, Extractable: true, Usages: append([]string(nil), usages...), ECDSAPublic: &ecdsa.PublicKey{Curve: curve, X: x, Y: y}}, nil
		}
		ecdhCurve, err := ecdhCurveByElliptic(curve)
		if err != nil {
			return nil, err
		}
		public, err := ecdhCurve.NewPublicKey(copyRaw)
		if err != nil {
			return nil, err
		}
		return &cryptoKey{Type: "public", Algorithm: algorithm, NamedCurve: normalized, Extractable: true, Usages: append([]string(nil), usages...), ECDHPublic: public}, nil
	case "ED25519":
		private := rawKeyHint(algorithmObject) == "private" || (rawKeyHint(algorithmObject) == "" && len(copyRaw) == ed25519.PrivateKeySize) || (rawKeyHint(algorithmObject) == "" && len(copyRaw) == ed25519.SeedSize && usageContains(usages, "sign") && !usageContains(usages, "verify"))
		if private {
			var key ed25519.PrivateKey
			switch len(copyRaw) {
			case ed25519.SeedSize:
				key = ed25519.NewKeyFromSeed(copyRaw)
			case ed25519.PrivateKeySize:
				key = ed25519.PrivateKey(copyRaw)
			default:
				return nil, errors.New("Ed25519 raw private key must be 32-byte seed or 64-byte private key")
			}
			return &cryptoKey{Type: "private", Algorithm: algorithm, Extractable: extractable, Usages: append([]string(nil), usages...), EdPrivate: key}, nil
		}
		if len(copyRaw) != ed25519.PublicKeySize {
			return nil, errors.New("Ed25519 raw public key length must be 32 bytes")
		}
		return &cryptoKey{Type: "public", Algorithm: algorithm, Extractable: true, Usages: append([]string(nil), usages...), EdPublic: ed25519.PublicKey(copyRaw)}, nil
	case "X25519":
		keyType := rawKeyHint(algorithmObject)
		if keyType == "private" {
			private, err := ecdh.X25519().NewPrivateKey(copyRaw)
			if err != nil {
				return nil, err
			}
			return &cryptoKey{Type: "private", Algorithm: algorithm, Extractable: extractable, Usages: append([]string(nil), usages...), XPrivate: private}, nil
		}
		public, err := ecdh.X25519().NewPublicKey(copyRaw)
		if err != nil {
			return nil, err
		}
		return &cryptoKey{Type: "public", Algorithm: algorithm, Extractable: true, Usages: append([]string(nil), usages...), XPublic: public}, nil
	default:
		return nil, fmt.Errorf("unsupported raw key algorithm: %s", algorithm)
	}
}

func importJWKMaterial(value *quickjs.Value, algorithm string, algorithmObject *quickjs.Value, extractable bool, usages []string) (*cryptoKey, error) {
	if value == nil || !value.IsObject() {
		return nil, errors.New("JWK must be an object")
	}
	kty, ok := readStringProperty(value, "kty")
	if !ok {
		return nil, errors.New("JWK.kty is required")
	}
	switch strings.ToUpper(kty) {
	case "OCT":
		encoded, ok := readStringProperty(value, "k")
		if !ok {
			return nil, errors.New("JWK.k is required")
		}
		secret, err := decodeBase64URL(encoded)
		if err != nil || len(secret) == 0 {
			if err != nil {
				return nil, err
			}
			return nil, errors.New("JWK.k must not be empty")
		}
		return importRawKeyMaterial(secret, algorithm, algorithmObject, extractable, usages)
	case "RSA":
		return importRSAJWK(value, algorithm, algorithmObject, extractable, usages)
	case "EC":
		return importECJWK(value, algorithm, algorithmObject, extractable, usages)
	case "OKP":
		return importOKPJWK(value, algorithm, extractable, usages)
	default:
		return nil, fmt.Errorf("unsupported JWK kty: %s", kty)
	}
}

func importRSAJWK(value *quickjs.Value, algorithm string, algorithmObject *quickjs.Value, extractable bool, usages []string) (*cryptoKey, error) {
	n, err := readJWKBigInt(value, "n")
	if err != nil {
		return nil, err
	}
	eValue, err := readJWKBigInt(value, "e")
	if err != nil {
		return nil, err
	}
	if !eValue.IsInt64() || eValue.Sign() <= 0 || !eValue.IsUint64() {
		return nil, errors.New("invalid RSA JWK exponent")
	}
	e64 := eValue.Uint64()
	if e64 > uint64(^uint(0)>>1) {
		return nil, errors.New("RSA JWK exponent is too large")
	}
	e := int(e64)
	if e < 3 || e%2 == 0 {
		return nil, errors.New("invalid RSA JWK exponent")
	}
	hashName, err := algorithmHashName(algorithmObject, "SHA-256")
	if err != nil {
		return nil, err
	}
	if hasValueProperty(value, "d") {
		d, err := readJWKBigInt(value, "d")
		if err != nil {
			return nil, err
		}
		if hasValueProperty(value, "oth") {
			return nil, errors.New("RSA JWK multi-prime oth is unsupported")
		}
		p, q, err := readRSAJWKFactors(value, n, e, d)
		if err != nil {
			return nil, err
		}
		private := &rsa.PrivateKey{PublicKey: rsa.PublicKey{N: n, E: e}, D: d, Primes: []*big.Int{p, q}}
		if err := private.Validate(); err != nil {
			return nil, err
		}
		private.Precompute()
		return &cryptoKey{Type: "private", Algorithm: algorithm, Hash: hashName, Extractable: extractable, Usages: append([]string(nil), usages...), RSAPrivate: private}, nil
	}
	return &cryptoKey{Type: "public", Algorithm: algorithm, Hash: hashName, Extractable: true, Usages: append([]string(nil), usages...), RSAPublic: &rsa.PublicKey{N: n, E: e}}, nil
}

func readRSAJWKFactors(value *quickjs.Value, n *big.Int, e int, d *big.Int) (*big.Int, *big.Int, error) {
	if hasValueProperty(value, "p") != hasValueProperty(value, "q") {
		return nil, nil, errors.New("RSA JWK p/q must both be present")
	}
	if hasValueProperty(value, "p") {
		p, err := readJWKBigInt(value, "p")
		if err != nil {
			return nil, nil, err
		}
		q, err := readJWKBigInt(value, "q")
		if err != nil {
			return nil, nil, err
		}
		return p, q, nil
	}
	return recoverRSAFactorsFromNED(n, e, d)
}

func importECJWK(value *quickjs.Value, algorithm string, algorithmObject *quickjs.Value, extractable bool, usages []string) (*cryptoKey, error) {
	if algorithm != "ECDSA" && algorithm != "ECDH" {
		return nil, errors.New("EC JWK requires ECDSA or ECDH algorithm")
	}
	curveName, ok := readStringProperty(value, "crv")
	if !ok {
		return nil, errors.New("EC JWK crv is required")
	}
	curve, normalized, err := namedCurveByName(curveName)
	if err != nil {
		return nil, err
	}
	x, err := readJWKBigInt(value, "x")
	if err != nil {
		return nil, err
	}
	y, err := readJWKBigInt(value, "y")
	if err != nil {
		return nil, err
	}
	if !curve.IsOnCurve(x, y) {
		return nil, errors.New("invalid EC JWK point")
	}
	if expected, err := algorithmCurveName(algorithmObject); err == nil && expected != "" {
		_, expectedName, curveErr := namedCurveByName(expected)
		if curveErr != nil || expectedName != normalized {
			return nil, errors.New("EC JWK curve does not match algorithm")
		}
	}
	hashName := ""
	if algorithm == "ECDSA" {
		hashName, err = algorithmHashName(algorithmObject, "SHA-256")
		if err != nil {
			return nil, err
		}
	}
	public := &ecdsa.PublicKey{Curve: curve, X: x, Y: y}
	if hasValueProperty(value, "d") {
		d, err := readJWKBigInt(value, "d")
		if err != nil {
			return nil, err
		}
		private := &ecdsa.PrivateKey{PublicKey: *public, D: d}
		if d.Sign() <= 0 || d.Cmp(curve.Params().N) >= 0 || !curve.IsOnCurve(public.X, public.Y) {
			return nil, errors.New("invalid EC JWK private key")
		}
		if algorithm == "ECDSA" {
			return &cryptoKey{Type: "private", Algorithm: algorithm, Hash: hashName, NamedCurve: normalized, Extractable: extractable, Usages: append([]string(nil), usages...), ECDSAPrivate: private}, nil
		}
		ecdhPrivate, err := ecdhPrivateFromECDSA(private)
		if err != nil {
			return nil, err
		}
		return &cryptoKey{Type: "private", Algorithm: algorithm, NamedCurve: normalized, Extractable: extractable, Usages: append([]string(nil), usages...), ECDHPrivate: ecdhPrivate}, nil
	}
	if algorithm == "ECDSA" {
		return &cryptoKey{Type: "public", Algorithm: algorithm, Hash: hashName, NamedCurve: normalized, Extractable: true, Usages: append([]string(nil), usages...), ECDSAPublic: public}, nil
	}
	ecdhCurve, err := ecdhCurveByElliptic(curve)
	if err != nil {
		return nil, err
	}
	ecdhPublic, err := ecdhCurve.NewPublicKey(elliptic.Marshal(curve, x, y))
	if err != nil {
		return nil, err
	}
	return &cryptoKey{Type: "public", Algorithm: algorithm, NamedCurve: normalized, Extractable: true, Usages: append([]string(nil), usages...), ECDHPublic: ecdhPublic}, nil
}

func importOKPJWK(value *quickjs.Value, algorithm string, extractable bool, usages []string) (*cryptoKey, error) {
	curve, ok := readStringProperty(value, "crv")
	if !ok {
		return nil, errors.New("OKP JWK crv is required")
	}
	encoded, ok := readStringProperty(value, "x")
	if !ok {
		return nil, errors.New("OKP JWK x is required")
	}
	publicBytes, err := decodeBase64URL(encoded)
	if err != nil {
		return nil, err
	}
	switch strings.ToUpper(curve) {
	case "ED25519":
		if algorithm != "ED25519" || len(publicBytes) != ed25519.PublicKeySize {
			return nil, errors.New("invalid Ed25519 JWK")
		}
		if privateEncoded, present := readStringProperty(value, "d"); present {
			seed, err := decodeBase64URL(privateEncoded)
			if err != nil || len(seed) != ed25519.SeedSize {
				if err != nil {
					return nil, err
				}
				return nil, errors.New("invalid Ed25519 private seed")
			}
			return &cryptoKey{Type: "private", Algorithm: algorithm, Extractable: extractable, Usages: append([]string(nil), usages...), EdPrivate: ed25519.NewKeyFromSeed(seed)}, nil
		}
		return &cryptoKey{Type: "public", Algorithm: algorithm, Extractable: true, Usages: append([]string(nil), usages...), EdPublic: ed25519.PublicKey(publicBytes)}, nil
	case "X25519":
		if algorithm != "X25519" || len(publicBytes) != 32 {
			return nil, errors.New("invalid X25519 JWK")
		}
		if privateEncoded, present := readStringProperty(value, "d"); present {
			privateBytes, err := decodeBase64URL(privateEncoded)
			if err != nil {
				return nil, err
			}
			private, err := ecdh.X25519().NewPrivateKey(privateBytes)
			if err != nil {
				return nil, err
			}
			return &cryptoKey{Type: "private", Algorithm: algorithm, Extractable: extractable, Usages: append([]string(nil), usages...), XPrivate: private}, nil
		}
		public, err := ecdh.X25519().NewPublicKey(publicBytes)
		if err != nil {
			return nil, err
		}
		return &cryptoKey{Type: "public", Algorithm: algorithm, Extractable: true, Usages: append([]string(nil), usages...), XPublic: public}, nil
	default:
		return nil, fmt.Errorf("unsupported OKP curve: %s", curve)
	}
}

func parsePrivateKeyMaterial(der []byte) (any, error) {
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		return key, nil
	}
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}
	if key, err := x509.ParseECPrivateKey(der); err == nil {
		return key, nil
	}
	return nil, errors.New("failed to parse private key")
}

func privateKeyMaterial(value any, algorithm string, algorithmObject *quickjs.Value, extractable bool, usages []string) (*cryptoKey, error) {
	switch key := value.(type) {
	case *rsa.PrivateKey:
		if !isRSAAlgorithm(algorithm) {
			return nil, errors.New("algorithm is not compatible with RSA private key")
		}
		hashName, err := algorithmHashName(algorithmObject, "SHA-256")
		if err != nil {
			return nil, err
		}
		return &cryptoKey{Type: "private", Algorithm: algorithm, Hash: hashName, Extractable: extractable, Usages: append([]string(nil), usages...), RSAPrivate: key}, nil
	case *ecdsa.PrivateKey:
		if algorithm != "ECDSA" && algorithm != "ECDH" {
			return nil, errors.New("algorithm is not compatible with EC private key")
		}
		namedCurve := namedCurveFromElliptic(key.Curve)
		if namedCurve == "" {
			return nil, errors.New("unsupported EC private key curve")
		}
		if expected, err := algorithmCurveName(algorithmObject); err == nil && expected != "" {
			_, expectedName, curveErr := namedCurveByName(expected)
			if curveErr != nil || expectedName != namedCurve {
				return nil, errors.New("EC key curve does not match algorithm")
			}
		}
		if algorithm == "ECDSA" {
			hashName, err := algorithmHashName(algorithmObject, "SHA-256")
			if err != nil {
				return nil, err
			}
			return &cryptoKey{Type: "private", Algorithm: algorithm, Hash: hashName, NamedCurve: namedCurve, Extractable: extractable, Usages: append([]string(nil), usages...), ECDSAPrivate: key}, nil
		}
		ecdhPrivate, err := ecdhPrivateFromECDSA(key)
		if err != nil {
			return nil, err
		}
		return &cryptoKey{Type: "private", Algorithm: algorithm, NamedCurve: namedCurve, Extractable: extractable, Usages: append([]string(nil), usages...), ECDHPrivate: ecdhPrivate}, nil
	case ed25519.PrivateKey:
		if algorithm != "ED25519" {
			return nil, errors.New("algorithm is not compatible with Ed25519 private key")
		}
		return &cryptoKey{Type: "private", Algorithm: algorithm, Extractable: extractable, Usages: append([]string(nil), usages...), EdPrivate: append(ed25519.PrivateKey(nil), key...)}, nil
	case *ecdh.PrivateKey:
		if algorithm != "X25519" || key.Curve() != ecdh.X25519() {
			return nil, errors.New("algorithm is not compatible with X25519 private key")
		}
		return &cryptoKey{Type: "private", Algorithm: algorithm, Extractable: extractable, Usages: append([]string(nil), usages...), XPrivate: key}, nil
	default:
		return nil, errors.New("unsupported private key type")
	}
}

func publicKeyMaterial(value any, algorithm string, algorithmObject *quickjs.Value, usages []string) (*cryptoKey, error) {
	switch key := value.(type) {
	case *rsa.PublicKey:
		if !isRSAAlgorithm(algorithm) {
			return nil, errors.New("algorithm is not compatible with RSA public key")
		}
		hashName, err := algorithmHashName(algorithmObject, "SHA-256")
		if err != nil {
			return nil, err
		}
		return &cryptoKey{Type: "public", Algorithm: algorithm, Hash: hashName, Extractable: true, Usages: append([]string(nil), usages...), RSAPublic: key}, nil
	case *ecdsa.PublicKey:
		if algorithm != "ECDSA" && algorithm != "ECDH" {
			return nil, errors.New("algorithm is not compatible with EC public key")
		}
		namedCurve := namedCurveFromElliptic(key.Curve)
		if namedCurve == "" {
			return nil, errors.New("unsupported EC public key curve")
		}
		if algorithm == "ECDSA" {
			hashName, err := algorithmHashName(algorithmObject, "SHA-256")
			if err != nil {
				return nil, err
			}
			return &cryptoKey{Type: "public", Algorithm: algorithm, Hash: hashName, NamedCurve: namedCurve, Extractable: true, Usages: append([]string(nil), usages...), ECDSAPublic: key}, nil
		}
		ecdhCurve, err := ecdhCurveByElliptic(key.Curve)
		if err != nil {
			return nil, err
		}
		ecdhPublic, err := ecdhCurve.NewPublicKey(elliptic.Marshal(key.Curve, key.X, key.Y))
		if err != nil {
			return nil, err
		}
		return &cryptoKey{Type: "public", Algorithm: algorithm, NamedCurve: namedCurve, Extractable: true, Usages: append([]string(nil), usages...), ECDHPublic: ecdhPublic}, nil
	case ed25519.PublicKey:
		if algorithm != "ED25519" {
			return nil, errors.New("algorithm is not compatible with Ed25519 public key")
		}
		return &cryptoKey{Type: "public", Algorithm: algorithm, Extractable: true, Usages: append([]string(nil), usages...), EdPublic: append(ed25519.PublicKey(nil), key...)}, nil
	case *ecdh.PublicKey:
		if algorithm != "X25519" || key.Curve() != ecdh.X25519() {
			return nil, errors.New("algorithm is not compatible with X25519 public key")
		}
		return &cryptoKey{Type: "public", Algorithm: algorithm, Extractable: true, Usages: append([]string(nil), usages...), XPublic: key}, nil
	default:
		return nil, errors.New("unsupported public key type")
	}
}

func exportRawKeyMaterial(key *cryptoKey) ([]byte, error) {
	if key == nil {
		return nil, errors.New("invalid CryptoKey")
	}
	if len(key.Secret) > 0 {
		return append([]byte(nil), key.Secret...), nil
	}
	if key.ECDSAPublic != nil {
		return elliptic.Marshal(key.ECDSAPublic.Curve, key.ECDSAPublic.X, key.ECDSAPublic.Y), nil
	}
	if key.ECDSAPrivate != nil {
		public := &key.ECDSAPrivate.PublicKey
		return elliptic.Marshal(public.Curve, public.X, public.Y), nil
	}
	if key.ECDHPublic != nil {
		return append([]byte(nil), key.ECDHPublic.Bytes()...), nil
	}
	if key.ECDHPrivate != nil {
		return append([]byte(nil), key.ECDHPrivate.PublicKey().Bytes()...), nil
	}
	if len(key.EdPublic) > 0 {
		return append([]byte(nil), key.EdPublic...), nil
	}
	if len(key.EdPrivate) > 0 {
		return append([]byte(nil), key.EdPrivate.Public().(ed25519.PublicKey)...), nil
	}
	if key.XPublic != nil {
		return append([]byte(nil), key.XPublic.Bytes()...), nil
	}
	if key.XPrivate != nil {
		return append([]byte(nil), key.XPrivate.PublicKey().Bytes()...), nil
	}
	return nil, errors.New("raw export requires a secret key or supported public key")
}

func exportDERKeyMaterial(key *cryptoKey, format string) ([]byte, error) {
	if key == nil {
		return nil, errors.New("invalid CryptoKey")
	}
	switch strings.ToLower(format) {
	case "pkcs8":
		var private any
		switch {
		case key.RSAPrivate != nil:
			private = key.RSAPrivate
		case key.ECDSAPrivate != nil:
			private = key.ECDSAPrivate
		case key.ECDHPrivate != nil:
			private = ecdsaPrivateFromECDH(key.ECDHPrivate)
		case len(key.EdPrivate) > 0:
			private = key.EdPrivate
		case key.XPrivate != nil:
			private = key.XPrivate
		default:
			return nil, errors.New("pkcs8 export requires a private key")
		}
		return x509.MarshalPKCS8PrivateKey(private)
	case "pkcs1":
		if key.RSAPrivate != nil {
			return x509.MarshalPKCS1PrivateKey(key.RSAPrivate), nil
		}
		if key.RSAPublic != nil {
			return x509.MarshalPKCS1PublicKey(key.RSAPublic), nil
		}
		return nil, errors.New("pkcs1 export requires an RSA key")
	case "sec1":
		if key.ECDSAPrivate != nil {
			return x509.MarshalECPrivateKey(key.ECDSAPrivate)
		}
		if key.ECDHPrivate != nil {
			return x509.MarshalECPrivateKey(ecdsaPrivateFromECDH(key.ECDHPrivate))
		}
		return nil, errors.New("sec1 export requires an EC private key")
	case "spki":
		var public any
		switch {
		case key.RSAPublic != nil:
			public = key.RSAPublic
		case key.RSAPrivate != nil:
			public = &key.RSAPrivate.PublicKey
		case key.ECDSAPublic != nil:
			public = key.ECDSAPublic
		case key.ECDSAPrivate != nil:
			public = &key.ECDSAPrivate.PublicKey
		case key.ECDHPublic != nil:
			public = ecdsaPublicFromECDH(key.ECDHPublic)
		case key.ECDHPrivate != nil:
			public = ecdsaPublicFromECDH(key.ECDHPrivate.PublicKey())
		case len(key.EdPublic) > 0:
			public = key.EdPublic
		case len(key.EdPrivate) > 0:
			public = key.EdPrivate.Public()
		case key.XPublic != nil:
			public = key.XPublic
		case key.XPrivate != nil:
			public = key.XPrivate.PublicKey()
		default:
			return nil, errors.New("spki export requires a public key")
		}
		return x509.MarshalPKIXPublicKey(public)
	default:
		return nil, fmt.Errorf("unsupported exportKey format: %s", format)
	}
}

func exportJWKObject(ctx *quickjs.Context, key *cryptoKey) (*quickjs.Value, error) {
	if key == nil {
		return nil, errors.New("invalid CryptoKey")
	}
	object := ctx.NewObject()
	if object == nil {
		return nil, errors.New("create JWK object")
	}
	set := func(name, value string) {
		object.Set(name, ctx.NewString(value))
	}
	if len(key.Secret) > 0 {
		set("kty", "oct")
		set("k", encodeBase64URL(key.Secret))
	} else if key.RSAPublic != nil || key.RSAPrivate != nil {
		set("kty", "RSA")
		public := key.RSAPublic
		if public == nil {
			public = &key.RSAPrivate.PublicKey
		}
		set("n", encodeBase64URL(public.N.Bytes()))
		set("e", encodeBase64URL(big.NewInt(int64(public.E)).Bytes()))
		if key.RSAPrivate != nil {
			set("d", encodeBase64URL(key.RSAPrivate.D.Bytes()))
			if len(key.RSAPrivate.Primes) >= 2 {
				set("p", encodeBase64URL(key.RSAPrivate.Primes[0].Bytes()))
				set("q", encodeBase64URL(key.RSAPrivate.Primes[1].Bytes()))
				dp := new(big.Int).Mod(key.RSAPrivate.D, new(big.Int).Sub(key.RSAPrivate.Primes[0], big.NewInt(1)))
				dq := new(big.Int).Mod(key.RSAPrivate.D, new(big.Int).Sub(key.RSAPrivate.Primes[1], big.NewInt(1)))
				qi := new(big.Int).ModInverse(key.RSAPrivate.Primes[1], key.RSAPrivate.Primes[0])
				if qi != nil {
					set("dp", encodeBase64URL(dp.Bytes()))
					set("dq", encodeBase64URL(dq.Bytes()))
					set("qi", encodeBase64URL(qi.Bytes()))
				}
			}
		}
	} else if key.ECDSAPublic != nil || key.ECDSAPrivate != nil || key.ECDHPublic != nil || key.ECDHPrivate != nil {
		set("kty", "EC")
		public := key.ECDSAPublic
		if public == nil && key.ECDSAPrivate != nil {
			public = &key.ECDSAPrivate.PublicKey
		}
		if public == nil {
			public = ecdsaPublicFromECDH(key.ECDHPublic)
			if public == nil && key.ECDHPrivate != nil {
				public = ecdsaPublicFromECDH(key.ECDHPrivate.PublicKey())
			}
		}
		if public == nil {
			object.Free()
			return nil, errors.New("EC key has no public point")
		}
		size := (public.Curve.Params().BitSize + 7) / 8
		set("crv", namedCurveFromElliptic(public.Curve))
		set("x", encodeBase64URL(leftPad(public.X.Bytes(), size)))
		set("y", encodeBase64URL(leftPad(public.Y.Bytes(), size)))
		if key.ECDSAPrivate != nil {
			set("d", encodeBase64URL(leftPad(key.ECDSAPrivate.D.Bytes(), size)))
		} else if key.ECDHPrivate != nil {
			private := ecdsaPrivateFromECDH(key.ECDHPrivate)
			set("d", encodeBase64URL(leftPad(private.D.Bytes(), size)))
		}
	} else if len(key.EdPublic) > 0 || len(key.EdPrivate) > 0 {
		set("kty", "OKP")
		set("crv", "Ed25519")
		public := key.EdPublic
		if len(public) == 0 {
			public = key.EdPrivate.Public().(ed25519.PublicKey)
		}
		set("x", encodeBase64URL(public))
		if len(key.EdPrivate) > 0 {
			set("d", encodeBase64URL(key.EdPrivate.Seed()))
		}
	} else if key.XPublic != nil || key.XPrivate != nil {
		set("kty", "OKP")
		set("crv", "X25519")
		public := key.XPublic
		if public == nil {
			public = key.XPrivate.PublicKey()
		}
		set("x", encodeBase64URL(public.Bytes()))
		if key.XPrivate != nil {
			set("d", encodeBase64URL(key.XPrivate.Bytes()))
		}
	} else {
		object.Free()
		return nil, errors.New("unsupported key type for JWK export")
	}
	if key.Algorithm == "HMAC" || key.Algorithm == "AES-CBC" || key.Algorithm == "AES-CTR" || key.Algorithm == "AES-GCM" || key.Algorithm == "AES-KW" || isRSAAlgorithm(key.Algorithm) || key.Algorithm == "ECDSA" || key.Algorithm == "ED25519" || key.Algorithm == "ECDH" || key.Algorithm == "X25519" {
		if algorithm := jwkAlgorithm(key); algorithm != "" {
			set("alg", algorithm)
		}
	}
	object.Set("key_ops", stringArray(ctx, key.Usages))
	object.Set("ext", ctx.NewBool(key.Extractable))
	return object, nil
}

func algorithmHashName(value *quickjs.Value, fallback string) (string, error) {
	if value == nil {
		return fallback, nil
	}
	if value.IsObject() {
		hashValue := value.Get("hash")
		if hashValue != nil {
			defer hashValue.Free()
			if !hashValue.IsUndefined() && !hashValue.IsNull() {
				return algorithmHash(hashValue, fallback)
			}
		}
	}
	return algorithmHash(value, fallback)
}

func algorithmCurveName(value *quickjs.Value) (string, error) {
	if value == nil {
		return "", errors.New("algorithm.namedCurve is required")
	}
	name, ok := readStringProperty(value, "namedCurve")
	if !ok || strings.TrimSpace(name) == "" {
		return "", errors.New("algorithm.namedCurve is required")
	}
	return name, nil
}

func rawKeyHint(value *quickjs.Value) string {
	if value == nil {
		return ""
	}
	name, _ := readStringProperty(value, "keyType")
	return strings.ToLower(strings.TrimSpace(name))
}

func hasValueProperty(value *quickjs.Value, name string) bool {
	if value == nil {
		return false
	}
	property := value.Get(name)
	if property == nil {
		return false
	}
	defer property.Free()
	return !property.IsUndefined()
}

func readJWKBigInt(value *quickjs.Value, name string) (*big.Int, error) {
	encoded, ok := readStringProperty(value, name)
	if !ok {
		return nil, fmt.Errorf("JWK.%s is required", name)
	}
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(data) == 0 {
		if err != nil {
			return nil, fmt.Errorf("invalid JWK.%s: %w", name, err)
		}
		return nil, fmt.Errorf("invalid JWK.%s", name)
	}
	result := new(big.Int).SetBytes(data)
	if result.Sign() <= 0 {
		return nil, fmt.Errorf("invalid JWK.%s", name)
	}
	return result, nil
}

func isRSAAlgorithm(name string) bool {
	switch name {
	case "RSASSA-PKCS1-V1_5", "RSA-PSS", "RSA-OAEP", "RSAES-PKCS1-V1_5":
		return true
	default:
		return false
	}
}

func jwkAlgorithm(key *cryptoKey) string {
	switch key.Algorithm {
	case "AES-GCM":
		return fmt.Sprintf("A%dGCM", key.Length)
	case "AES-KW":
		return fmt.Sprintf("A%dKW", key.Length)
	case "HMAC":
		switch normalizeName(key.Hash) {
		case "SHA-1", "SHA1":
			return "HS1"
		case "SHA-384":
			return "HS384"
		case "SHA-512":
			return "HS512"
		case "MD5":
			return "HMD5"
		default:
			return "HS256"
		}
	case "RSASSA-PKCS1-V1_5":
		return rsaJWKAlgorithm("RS", key.Hash)
	case "RSA-PSS":
		return rsaJWKAlgorithm("PS", key.Hash)
	case "RSA-OAEP":
		switch normalizeName(key.Hash) {
		case "SHA-1", "SHA1":
			return "RSA-OAEP"
		case "SHA-384":
			return "RSA-OAEP-384"
		case "SHA-512":
			return "RSA-OAEP-512"
		default:
			return "RSA-OAEP-256"
		}
	case "RSAES-PKCS1-V1_5":
		return "RSA1_5"
	case "ECDSA":
		switch key.NamedCurve {
		case "P-256":
			return "ES256"
		case "P-384":
			return "ES384"
		case "P-521":
			return "ES512"
		}
	case "ED25519":
		return "EdDSA"
	case "ECDH", "X25519":
		return "ECDH-ES"
	}
	return ""
}

func rsaJWKAlgorithm(prefix, hashName string) string {
	switch normalizeName(hashName) {
	case "SHA-1", "SHA1":
		return prefix + "1"
	case "SHA-384":
		return prefix + "384"
	case "SHA-512":
		return prefix + "512"
	default:
		return prefix + "256"
	}
}

func leftPad(data []byte, size int) []byte {
	if len(data) >= size {
		return append([]byte(nil), data[len(data)-size:]...)
	}
	result := make([]byte, size)
	copy(result[size-len(data):], data)
	return result
}

func ecdhPrivateFromECDSA(key *ecdsa.PrivateKey) (*ecdh.PrivateKey, error) {
	curve, err := ecdhCurveByElliptic(key.Curve)
	if err != nil {
		return nil, err
	}
	return curve.NewPrivateKey(leftPad(key.D.Bytes(), (key.Curve.Params().BitSize+7)/8))
}

func ecdsaPrivateFromECDH(key *ecdh.PrivateKey) *ecdsa.PrivateKey {
	curve := elliptic.P256()
	if key.Curve() == ecdh.P384() {
		curve = elliptic.P384()
	} else if key.Curve() == ecdh.P521() {
		curve = elliptic.P521()
	}
	public := key.PublicKey()
	x, y := elliptic.Unmarshal(curve, public.Bytes())
	return &ecdsa.PrivateKey{PublicKey: ecdsa.PublicKey{Curve: curve, X: x, Y: y}, D: new(big.Int).SetBytes(key.Bytes())}
}

func ecdsaPublicFromECDH(key *ecdh.PublicKey) *ecdsa.PublicKey {
	if key == nil {
		return nil
	}
	var curve elliptic.Curve
	switch key.Curve() {
	case ecdh.P256():
		curve = elliptic.P256()
	case ecdh.P384():
		curve = elliptic.P384()
	case ecdh.P521():
		curve = elliptic.P521()
	default:
		return nil
	}
	x, y := elliptic.Unmarshal(curve, key.Bytes())
	if x == nil || y == nil {
		return nil
	}
	return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}
}

func namedCurveByName(name string) (elliptic.Curve, string, error) {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "P-256", "SECP256R1":
		return elliptic.P256(), "P-256", nil
	case "P-384", "SECP384R1":
		return elliptic.P384(), "P-384", nil
	case "P-521", "SECP521R1":
		return elliptic.P521(), "P-521", nil
	default:
		return nil, "", fmt.Errorf("unsupported namedCurve: %s", name)
	}
}

func namedCurveFromElliptic(curve elliptic.Curve) string {
	switch curve {
	case elliptic.P256():
		return "P-256"
	case elliptic.P384():
		return "P-384"
	case elliptic.P521():
		return "P-521"
	default:
		return ""
	}
}

func ecdhCurveByElliptic(curve elliptic.Curve) (ecdh.Curve, error) {
	switch curve {
	case elliptic.P256():
		return ecdh.P256(), nil
	case elliptic.P384():
		return ecdh.P384(), nil
	case elliptic.P521():
		return ecdh.P521(), nil
	default:
		return nil, errors.New("unsupported ECDH curve")
	}
}

func usageContains(usages []string, wanted string) bool {
	for _, usage := range usages {
		if usage == wanted {
			return true
		}
	}
	return false
}

func recoverRSAFactorsFromNED(n *big.Int, e int, d *big.Int) (*big.Int, *big.Int, error) {
	if n == nil || d == nil || e <= 1 {
		return nil, nil, errors.New("invalid RSA key parameters")
	}
	one := big.NewInt(1)
	k := new(big.Int).Mul(d, big.NewInt(int64(e)))
	k.Sub(k, one)
	if k.Sign() <= 0 || k.Bit(0) != 0 {
		return nil, nil, errors.New("failed to recover RSA factors from n/e/d")
	}
	t := 0
	for k.Bit(0) == 0 {
		k.Rsh(k, 1)
		t++
	}
	nMinusOne := new(big.Int).Sub(new(big.Int).Set(n), one)
	tryBase := func(base *big.Int) (*big.Int, *big.Int, bool) {
		gcd := new(big.Int).GCD(nil, nil, base, n)
		if gcd.Cmp(one) > 0 && gcd.Cmp(n) < 0 {
			p, q := sortedFactors(gcd, new(big.Int).Div(new(big.Int).Set(n), gcd))
			return p, q, true
		}
		y := new(big.Int).Exp(base, k, n)
		if y.Cmp(one) == 0 || y.Cmp(nMinusOne) == 0 {
			return nil, nil, false
		}
		for index := 0; index < t; index++ {
			squared := new(big.Int).Mul(y, y)
			squared.Mod(squared, n)
			if squared.Cmp(one) == 0 {
				factor := new(big.Int).Sub(y, one)
				factor.GCD(nil, nil, factor, n)
				if factor.Cmp(one) <= 0 || factor.Cmp(n) >= 0 {
					return nil, nil, false
				}
				other := new(big.Int).Div(new(big.Int).Set(n), factor)
				if new(big.Int).Mul(new(big.Int).Set(factor), other).Cmp(n) != 0 {
					return nil, nil, false
				}
				p, q := sortedFactors(factor, other)
				return p, q, true
			}
			if squared.Cmp(nMinusOne) == 0 {
				return nil, nil, false
			}
			y = squared
		}
		return nil, nil, false
	}
	for base := int64(2); base < 8192; base++ {
		p, q, ok := tryBase(big.NewInt(base))
		if ok {
			return p, q, nil
		}
	}
	return nil, nil, errors.New("failed to recover RSA factors from n/e/d")
}

func sortedFactors(first, second *big.Int) (*big.Int, *big.Int) {
	if first.Cmp(second) <= 0 {
		return first, second
	}
	return second, first
}
