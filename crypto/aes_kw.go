package crypto

import (
	"crypto/cipher"
	"encoding/binary"
	"errors"
)

var aesKWDefaultIV = [8]byte{0xa6, 0xa6, 0xa6, 0xa6, 0xa6, 0xa6, 0xa6, 0xa6}

func aesKeyWrap(block cipher.Block, plaintext []byte) ([]byte, error) {
	if len(plaintext) == 0 || len(plaintext)%8 != 0 || len(plaintext) < 16 {
		return nil, errors.New("AES-KW plaintext must contain at least two 64-bit blocks")
	}
	n := len(plaintext) / 8
	r := make([]byte, len(plaintext))
	copy(r, plaintext)
	a := append([]byte(nil), aesKWDefaultIV[:]...)
	var input, output [16]byte
	for round := 0; round < 6; round++ {
		for index := 0; index < n; index++ {
			copy(input[:8], a)
			copy(input[8:], r[index*8:(index+1)*8])
			block.Encrypt(output[:], input[:])
			t := uint64(n*round + index + 1)
			binary.BigEndian.PutUint64(input[:8], binary.BigEndian.Uint64(output[:8])^t)
			copy(a, input[:8])
			copy(r[index*8:(index+1)*8], output[8:])
		}
	}
	return append(a, r...), nil
}

func aesKeyUnwrap(block cipher.Block, wrapped []byte) ([]byte, error) {
	if len(wrapped) < 24 || len(wrapped)%8 != 0 {
		return nil, errors.New("AES-KW ciphertext has invalid length")
	}
	n := len(wrapped)/8 - 1
	a := append([]byte(nil), wrapped[:8]...)
	r := append([]byte(nil), wrapped[8:]...)
	var input, output [16]byte
	for round := 5; round >= 0; round-- {
		for index := n - 1; index >= 0; index-- {
			t := uint64(n*round + index + 1)
			binary.BigEndian.PutUint64(input[:8], binary.BigEndian.Uint64(a)^t)
			copy(input[8:], r[index*8:(index+1)*8])
			block.Decrypt(output[:], input[:])
			copy(a, output[:8])
			copy(r[index*8:(index+1)*8], output[8:])
		}
	}
	if string(a) != string(aesKWDefaultIV[:]) {
		return nil, errors.New("AES-KW integrity check failed")
	}
	return r, nil
}
