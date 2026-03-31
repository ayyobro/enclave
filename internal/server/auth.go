package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/nacl/box"
)

// GenerateInviteToken creates a random invite token and returns the raw token
// (to give to the user) and its SHA-256 hash (to store in the database).
func GenerateInviteToken() (token string, tokenHash []byte, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("generating random bytes: %w", err)
	}
	token = base64.URLEncoding.EncodeToString(raw)
	hash := sha256.Sum256(raw)
	return token, hash[:], nil
}

// HashInviteToken computes the SHA-256 hash of a base64url-encoded invite token.
func HashInviteToken(token string) ([]byte, error) {
	raw, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("decoding invite token: %w", err)
	}
	hash := sha256.Sum256(raw)
	return hash[:], nil
}

// GenerateChallenge creates a random 32-byte challenge nonce for auth.
func GenerateChallenge() ([32]byte, error) {
	var challenge [32]byte
	if _, err := rand.Read(challenge[:]); err != nil {
		return challenge, err
	}
	return challenge, nil
}

// VerifyChallengeResponse verifies that the client encrypted the challenge
// correctly using their private key and the server's public key.
func VerifyChallengeResponse(
	response []byte,
	challenge [32]byte,
	clientPub *[32]byte,
	serverPriv *[32]byte,
) (bool, error) {
	if len(response) < 24 {
		return false, fmt.Errorf("response too short")
	}

	// The response is: nonce (24 bytes) + box.Seal output
	var nonce [24]byte
	copy(nonce[:], response[:24])
	ciphertext := response[24:]

	plaintext, ok := box.Open(nil, ciphertext, &nonce, clientPub, serverPriv)
	if !ok {
		return false, nil
	}

	if len(plaintext) != 32 {
		return false, nil
	}

	var got [32]byte
	copy(got[:], plaintext)
	return got == challenge, nil
}
