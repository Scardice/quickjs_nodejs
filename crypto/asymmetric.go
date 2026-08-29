package crypto

import (
	stdcrypto "crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec // SHA-1 remains available for WebCrypto compatibility.
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/asn1"
	"errors"
	"fmt"
	"math/big"
)

func cryptoHashByName(name string) (stdcrypto.Hash, error) {
	switch normalizeName(name) {
	case "SHA-1", "SHA1":
		return stdcrypto.SHA1, nil
	case "SHA-224":
		return stdcrypto.SHA224, nil
	case "SHA-256":
		return stdcrypto.SHA256, nil
	case "SHA-384":
		return stdcrypto.SHA384, nil
	case "SHA-512":
		return stdcrypto.SHA512, nil
	default:
		return 0, fmt.Errorf("unsupported cryptographic hash: %s", name)
	}
}

func cryptoDigest(name string, data []byte) (stdcrypto.Hash, []byte, error) {
	hashID, err := cryptoHashByName(name)
	if err != nil {
		return 0, nil, err
	}
	if !hashID.Available() {
		return 0, nil, fmt.Errorf("cryptographic hash is unavailable: %s", name)
	}
	var digest []byte
	switch hashID {
	case stdcrypto.SHA1:
		sum := sha1.Sum(data) //nolint:gosec // SHA-1 is part of the compatibility API.
		digest = sum[:]
	case stdcrypto.SHA224:
		sum := sha256.Sum224(data)
		digest = sum[:]
	case stdcrypto.SHA256:
		sum := sha256.Sum256(data)
		digest = sum[:]
	case stdcrypto.SHA384:
		sum := sha512.Sum384(data)
		digest = sum[:]
	case stdcrypto.SHA512:
		sum := sha512.Sum512(data)
		digest = sum[:]
	default:
		return 0, nil, fmt.Errorf("unsupported cryptographic hash: %s", name)
	}
	return hashID, digest, nil
}

func rsaSignBytes(algorithm, hashName string, saltLength int, key *rsa.PrivateKey, data []byte) ([]byte, error) {
	if key == nil {
		return nil, errors.New("RSA private key is required")
	}
	hashID, digest, err := cryptoDigest(hashName, data)
	if err != nil {
		return nil, err
	}
	switch algorithm {
	case "RSASSA-PKCS1-V1_5":
		return rsa.SignPKCS1v15(cryptorand.Reader, key, hashID, digest)
	case "RSA-PSS":
		if saltLength < 0 {
			saltLength = rsa.PSSSaltLengthEqualsHash
		}
		return rsa.SignPSS(cryptorand.Reader, key, hashID, digest, &rsa.PSSOptions{SaltLength: saltLength, Hash: hashID})
	default:
		return nil, fmt.Errorf("unsupported RSA signing algorithm: %s", algorithm)
	}
}

func rsaVerifyBytes(algorithm, hashName string, saltLength int, key *rsa.PublicKey, signature, data []byte) error {
	if key == nil {
		return errors.New("RSA public key is required")
	}
	hashID, digest, err := cryptoDigest(hashName, data)
	if err != nil {
		return err
	}
	switch algorithm {
	case "RSASSA-PKCS1-V1_5":
		return rsa.VerifyPKCS1v15(key, hashID, digest, signature)
	case "RSA-PSS":
		if saltLength < 0 {
			saltLength = rsa.PSSSaltLengthEqualsHash
		}
		return rsa.VerifyPSS(key, hashID, digest, signature, &rsa.PSSOptions{SaltLength: saltLength, Hash: hashID})
	default:
		return fmt.Errorf("unsupported RSA signing algorithm: %s", algorithm)
	}
}

func rsaEncryptBytes(algorithm, hashName string, label []byte, key *rsa.PublicKey, data []byte) ([]byte, error) {
	if key == nil {
		return nil, errors.New("RSA public key is required")
	}
	switch algorithm {
	case "RSA-OAEP":
		hashID, _, err := cryptoDigest(hashName, nil)
		if err != nil {
			return nil, err
		}
		return rsa.EncryptOAEP(hashID.New(), cryptorand.Reader, key, data, label)
	case "RSAES-PKCS1-V1_5":
		return rsa.EncryptPKCS1v15(cryptorand.Reader, key, data)
	default:
		return nil, fmt.Errorf("unsupported RSA encryption algorithm: %s", algorithm)
	}
}

func rsaDecryptBytes(algorithm, hashName string, label []byte, key *rsa.PrivateKey, data []byte) ([]byte, error) {
	if key == nil {
		return nil, errors.New("RSA private key is required")
	}
	switch algorithm {
	case "RSA-OAEP":
		hashID, _, err := cryptoDigest(hashName, nil)
		if err != nil {
			return nil, err
		}
		return rsa.DecryptOAEP(hashID.New(), cryptorand.Reader, key, data, label)
	case "RSAES-PKCS1-V1_5":
		return rsa.DecryptPKCS1v15(cryptorand.Reader, key, data)
	default:
		return nil, fmt.Errorf("unsupported RSA encryption algorithm: %s", algorithm)
	}
}

func ecdsaSignBytes(hashName string, key *ecdsa.PrivateKey, data []byte) ([]byte, error) {
	if key == nil {
		return nil, errors.New("ECDSA private key is required")
	}
	_, digest, err := cryptoDigest(hashName, data)
	if err != nil {
		return nil, err
	}
	r, s, err := ecdsa.Sign(cryptorand.Reader, key, digest)
	if err != nil {
		return nil, err
	}
	size := (key.Curve.Params().BitSize + 7) / 8
	result := make([]byte, size*2)
	copy(result[size-len(r.Bytes()):size], r.Bytes())
	copy(result[2*size-len(s.Bytes()):], s.Bytes())
	return result, nil
}

func ecdsaVerifyBytes(hashName string, key *ecdsa.PublicKey, signature, data []byte) (bool, error) {
	if key == nil {
		return false, errors.New("ECDSA public key is required")
	}
	_, digest, err := cryptoDigest(hashName, data)
	if err != nil {
		return false, err
	}
	r, s, err := parseECDSASignature(key, signature)
	if err != nil {
		return false, err
	}
	return ecdsa.Verify(key, digest, r, s), nil
}

func parseECDSASignature(key *ecdsa.PublicKey, signature []byte) (*big.Int, *big.Int, error) {
	if key == nil || key.Curve == nil {
		return nil, nil, errors.New("ECDSA key has no curve")
	}
	size := (key.Curve.Params().BitSize + 7) / 8
	if len(signature) == size*2 {
		r := new(big.Int).SetBytes(signature[:size])
		s := new(big.Int).SetBytes(signature[size:])
		if r.Sign() <= 0 || s.Sign() <= 0 {
			return nil, nil, errors.New("invalid ECDSA signature")
		}
		return r, s, nil
	}
	var parsed struct {
		R *big.Int
		S *big.Int
	}
	rest, err := asn1.Unmarshal(signature, &parsed)
	if err != nil || len(rest) != 0 || parsed.R == nil || parsed.S == nil || parsed.R.Sign() <= 0 || parsed.S.Sign() <= 0 {
		return nil, nil, errors.New("invalid ECDSA signature")
	}
	return parsed.R, parsed.S, nil
}

func ed25519SignBytes(key ed25519.PrivateKey, data []byte) ([]byte, error) {
	if len(key) != ed25519.PrivateKeySize {
		return nil, errors.New("Ed25519 private key is required")
	}
	return ed25519.Sign(key, data), nil
}

func ed25519VerifyBytes(key ed25519.PublicKey, signature, data []byte) (bool, error) {
	if len(key) != ed25519.PublicKeySize {
		return false, errors.New("Ed25519 public key is required")
	}
	if len(signature) != ed25519.SignatureSize {
		return false, nil
	}
	return ed25519.Verify(key, data, signature), nil
}

func equalBytes(left, right []byte) bool {
	return len(left) == len(right) && subtle.ConstantTimeCompare(left, right) == 1
}
