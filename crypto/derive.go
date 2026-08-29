package crypto

import (
	"errors"

	quickjs "github.com/buke/quickjs-go"
)

func (s *cryptoState) deriveECDH(ctx *quickjs.Context, algorithm *quickjs.Value, key *cryptoKey, length int) ([]byte, error) {
	if key == nil {
		return nil, errors.New("base key is required")
	}
	publicValue := algorithmProperty(algorithm, "public")
	if publicValue == nil {
		return nil, errors.New("ECDH public key is required")
	}
	public, err := s.keyFromValue(ctx, publicValue)
	publicValue.Free()
	if err != nil {
		return nil, err
	}
	var shared []byte
	switch {
	case key.Algorithm == "ECDH" && key.ECDHPrivate != nil && public.Algorithm == "ECDH" && public.ECDHPublic != nil:
		shared, err = key.ECDHPrivate.ECDH(public.ECDHPublic)
	case key.Algorithm == "X25519" && key.XPrivate != nil && public.Algorithm == "X25519" && public.XPublic != nil:
		shared, err = key.XPrivate.ECDH(public.XPublic)
	default:
		return nil, errors.New("ECDH keys are incompatible")
	}
	if err != nil {
		return nil, err
	}
	if length < 0 || length > len(shared) {
		return nil, errors.New("deriveBits length exceeds the shared secret size")
	}
	return append([]byte(nil), shared[:length]...), nil
}
