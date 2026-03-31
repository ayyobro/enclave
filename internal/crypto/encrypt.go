package crypto

import (
	"crypto/rand"
	"errors"
	"fmt"

	"golang.org/x/crypto/nacl/box"
)

// SealMessage encrypts plaintext using NaCl box (X25519 + XSalsa20-Poly1305).
// Returns a random nonce and the authenticated ciphertext.
func SealMessage(plaintext []byte, peerPub, myPriv *[32]byte) (nonce [24]byte, ciphertext []byte, err error) {
	if _, err = rand.Read(nonce[:]); err != nil {
		return nonce, nil, fmt.Errorf("generating nonce: %w", err)
	}
	ciphertext = box.Seal(nil, plaintext, &nonce, peerPub, myPriv)
	return nonce, ciphertext, nil
}

// OpenMessage decrypts and authenticates a NaCl box ciphertext.
func OpenMessage(ciphertext []byte, nonce *[24]byte, peerPub, myPriv *[32]byte) ([]byte, error) {
	plaintext, ok := box.Open(nil, ciphertext, nonce, peerPub, myPriv)
	if !ok {
		return nil, errors.New("decryption failed: message authentication error")
	}
	return plaintext, nil
}

// Precompute returns a shared key for a specific peer. Use this for
// repeated encryption/decryption with the same contact to avoid
// repeated Curve25519 scalar multiplications.
func Precompute(peerPub, myPriv *[32]byte) *[32]byte {
	var shared [32]byte
	box.Precompute(&shared, peerPub, myPriv)
	return &shared
}

// SealPrecomputed encrypts using a precomputed shared key.
func SealPrecomputed(plaintext []byte, shared *[32]byte) (nonce [24]byte, ciphertext []byte, err error) {
	if _, err = rand.Read(nonce[:]); err != nil {
		return nonce, nil, fmt.Errorf("generating nonce: %w", err)
	}
	ciphertext = box.SealAfterPrecomputation(nil, plaintext, &nonce, shared)
	return nonce, ciphertext, nil
}

// OpenPrecomputed decrypts using a precomputed shared key.
func OpenPrecomputed(ciphertext []byte, nonce *[24]byte, shared *[32]byte) ([]byte, error) {
	plaintext, ok := box.OpenAfterPrecomputation(nil, ciphertext, nonce, shared)
	if !ok {
		return nil, errors.New("decryption failed: message authentication error")
	}
	return plaintext, nil
}
