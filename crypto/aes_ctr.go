package crypto

import (
	"crypto/cipher"
	"errors"
	"math/big"
)

func aesCTRBytes(block cipher.Block, counter []byte, counterBits int, data []byte) ([]byte, error) {
	if len(counter) != block.BlockSize() {
		return nil, errors.New("AES-CTR counter must be 16 bytes")
	}
	if counterBits < 1 || counterBits > len(counter)*8 {
		return nil, errors.New("AES-CTR length must be between 1 and 128")
	}
	blocks := (len(data) + block.BlockSize() - 1) / block.BlockSize()
	if !aesCTRHasCapacity(counter, counterBits, blocks) {
		return nil, errors.New("AES-CTR counter would repeat")
	}

	result := make([]byte, len(data))
	current := append([]byte(nil), counter...)
	stream := make([]byte, block.BlockSize())
	for offset := 0; offset < len(data); offset += block.BlockSize() {
		block.Encrypt(stream, current)
		end := min(offset+block.BlockSize(), len(data))
		for index := offset; index < end; index++ {
			result[index] = data[index] ^ stream[index-offset]
		}
		incrementAESCTRCounter(current, counterBits)
	}
	return result, nil
}

func aesCTRHasCapacity(counter []byte, counterBits, blocks int) bool {
	if blocks == 0 {
		return true
	}
	limit := new(big.Int).Lsh(big.NewInt(1), uint(counterBits))
	limit.Sub(limit, big.NewInt(1))
	current := new(big.Int).SetBytes(counter)
	current.And(current, limit)
	current.Add(current, big.NewInt(int64(blocks-1)))
	return current.Cmp(limit) <= 0
}

func incrementAESCTRCounter(counter []byte, counterBits int) {
	for bit := 0; bit < counterBits; bit++ {
		index := len(counter) - 1 - bit/8
		mask := byte(1 << uint(bit%8))
		if counter[index]&mask == 0 {
			counter[index] |= mask
			return
		}
		counter[index] &^= mask
	}
}
