package crypto

import (
	"crypto/hmac"
	"crypto/md5"  //nolint:gosec // MD5 is retained for Node compatibility.
	"crypto/sha1" //nolint:gosec // SHA-1 is retained for Node compatibility.
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"hash"
	"strings"
)

func hashFactory(name string) (func() hash.Hash, int, error) {
	switch normalizeName(name) {
	case "MD5":
		return md5.New, md5.Size, nil
	case "SHA-1", "SHA1":
		return sha1.New, sha1.Size, nil
	case "SHA-224":
		return sha256.New224, sha256.Size224, nil
	case "SHA-256":
		return sha256.New, sha256.Size, nil
	case "SHA-384":
		return sha512.New384, sha512.Size384, nil
	case "SHA-512":
		return sha512.New, sha512.Size, nil
	default:
		return nil, 0, errors.New("unsupported hash algorithm: " + name)
	}
}

func digestBytes(name string, data []byte) ([]byte, error) {
	factory, _, err := hashFactory(name)
	if err != nil {
		return nil, err
	}
	h := factory()
	_, _ = h.Write(data)
	return h.Sum(nil), nil
}

func hmacBytes(name string, key, data []byte) ([]byte, error) {
	factory, _, err := hashFactory(name)
	if err != nil {
		return nil, err
	}
	mac := hmac.New(factory, key)
	_, _ = mac.Write(data)
	return mac.Sum(nil), nil
}

func pbkdf2Bytes(hashName string, password, salt []byte, iterations, length int) ([]byte, error) {
	if iterations <= 0 {
		return nil, errors.New("iterations must be positive")
	}
	if length < 0 {
		return nil, errors.New("length must not be negative")
	}
	factory, hashSize, err := hashFactory(hashName)
	if err != nil {
		return nil, err
	}
	if length == 0 {
		return []byte{}, nil
	}
	blocks := (length + hashSize - 1) / hashSize
	if blocks > int(^uint32(0)) {
		return nil, errors.New("derived key is too large")
	}
	out := make([]byte, 0, blocks*hashSize)
	for block := 1; block <= blocks; block++ {
		mac := hmac.New(factory, password)
		_, _ = mac.Write(salt)
		var index [4]byte
		binary.BigEndian.PutUint32(index[:], uint32(block))
		_, _ = mac.Write(index[:])
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iterations; i++ {
			mac = hmac.New(factory, password)
			_, _ = mac.Write(u)
			u = mac.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:length], nil
}

func hkdfBytes(hashName string, secret, salt, info []byte, length int) ([]byte, error) {
	if length < 0 {
		return nil, errors.New("length must not be negative")
	}
	factory, hashSize, err := hashFactory(hashName)
	if err != nil {
		return nil, err
	}
	if length == 0 {
		return []byte{}, nil
	}
	if length > 255*hashSize {
		return nil, errors.New("derived key is too large")
	}
	if salt == nil {
		salt = make([]byte, hashSize)
	}
	extractor := hmac.New(factory, salt)
	_, _ = extractor.Write(secret)
	prk := extractor.Sum(nil)
	out := make([]byte, 0, length)
	var previous []byte
	for counter := byte(1); len(out) < length; counter++ {
		mac := hmac.New(factory, prk)
		_, _ = mac.Write(previous)
		_, _ = mac.Write(info)
		_, _ = mac.Write([]byte{counter})
		previous = mac.Sum(nil)
		out = append(out, previous...)
	}
	return out[:length], nil
}

func normalizeName(name string) string {
	raw := strings.TrimSpace(name)
	normalized := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(raw, "_", "-"), " ", ""))
	switch normalized {
	case "SHA1":
		return "SHA-1"
	case "SHA224":
		return "SHA-224"
	case "SHA256":
		return "SHA-256"
	case "SHA384":
		return "SHA-384"
	case "SHA512":
		return "SHA-512"
	case "AESCBC":
		return "AES-CBC"
	case "AESCTR":
		return "AES-CTR"
	case "AESGCM":
		return "AES-GCM"
	case "AESKW":
		return "AES-KW"
	case "RSAPSS":
		return "RSA-PSS"
	case "RSAOAEP":
		return "RSA-OAEP"
	case "RSASSA-PKCS1-V1.5", "RSASSA-PKCS1-V1-5", "RSASSAPKCS1V15", "RSASSA-PKCS1V1-5":
		return "RSASSA-PKCS1-V1_5"
	case "RSAES-PKCS1-V1.5", "RSAES-PKCS1-V1-5", "RSAESPKCS1V15":
		return "RSAES-PKCS1-V1_5"
	default:
		return normalized
	}
}
