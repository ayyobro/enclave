package crypto

import (
	"bytes"
	"testing"
)

func TestGenerateKeypair(t *testing.T) {
	pub, priv, err := GenerateKeypair()
	if err != nil {
		t.Fatalf("GenerateKeypair: %v", err)
	}
	if pub == nil || priv == nil {
		t.Fatal("GenerateKeypair returned nil keys")
	}
	// Keys should not be all zeros
	var zero [32]byte
	if *pub == zero {
		t.Error("public key is all zeros")
	}
	if *priv == zero {
		t.Error("private key is all zeros")
	}
}

func TestSealOpenRoundTrip(t *testing.T) {
	alicePub, alicePriv, err := GenerateKeypair()
	if err != nil {
		t.Fatalf("generating alice's keys: %v", err)
	}
	bobPub, bobPriv, err := GenerateKeypair()
	if err != nil {
		t.Fatalf("generating bob's keys: %v", err)
	}

	plaintext := []byte("Hello, Bob! This is a secret message.")

	nonce, ciphertext, err := SealMessage(plaintext, bobPub, alicePriv)
	if err != nil {
		t.Fatalf("SealMessage: %v", err)
	}

	// Ciphertext should be different from plaintext
	if bytes.Equal(ciphertext, plaintext) {
		t.Error("ciphertext equals plaintext")
	}

	// Bob decrypts with alice's public key and his private key
	got, err := OpenMessage(ciphertext, &nonce, alicePub, bobPriv)
	if err != nil {
		t.Fatalf("OpenMessage: %v", err)
	}

	if !bytes.Equal(got, plaintext) {
		t.Errorf("decrypted text = %q, want %q", got, plaintext)
	}
}

func TestOpenMessageWrongKey(t *testing.T) {
	alicePub, alicePriv, _ := GenerateKeypair()
	bobPub, _, _ := GenerateKeypair()
	_, evePriv, _ := GenerateKeypair()

	plaintext := []byte("Secret message")
	nonce, ciphertext, err := SealMessage(plaintext, bobPub, alicePriv)
	if err != nil {
		t.Fatalf("SealMessage: %v", err)
	}

	// Eve tries to decrypt with her private key — should fail
	_, err = OpenMessage(ciphertext, &nonce, alicePub, evePriv)
	if err == nil {
		t.Error("OpenMessage should fail with wrong private key")
	}
}

func TestOpenMessageTamperedCiphertext(t *testing.T) {
	_, alicePriv, _ := GenerateKeypair()
	bobPub, bobPriv, _ := GenerateKeypair()

	// Get alice's public key from the pair for decryption
	alicePub2, _, _ := GenerateKeypair() // wrong pub key

	plaintext := []byte("Secret message")
	nonce, ciphertext, _ := SealMessage(plaintext, bobPub, alicePriv)

	// Tamper with ciphertext
	ciphertext[0] ^= 0xff

	_, err := OpenMessage(ciphertext, &nonce, alicePub2, bobPriv)
	if err == nil {
		t.Error("OpenMessage should fail with tampered ciphertext")
	}
}

func TestPrecomputedRoundTrip(t *testing.T) {
	alicePub, alicePriv, _ := GenerateKeypair()
	bobPub, bobPriv, _ := GenerateKeypair()

	// Precompute shared keys from both sides
	aliceShared := Precompute(bobPub, alicePriv)
	bobShared := Precompute(alicePub, bobPriv)

	// Shared keys should be identical
	if *aliceShared != *bobShared {
		t.Fatal("precomputed shared keys don't match")
	}

	plaintext := []byte("Hello via precomputed key!")

	nonce, ciphertext, err := SealPrecomputed(plaintext, aliceShared)
	if err != nil {
		t.Fatalf("SealPrecomputed: %v", err)
	}

	got, err := OpenPrecomputed(ciphertext, &nonce, bobShared)
	if err != nil {
		t.Fatalf("OpenPrecomputed: %v", err)
	}

	if !bytes.Equal(got, plaintext) {
		t.Errorf("decrypted = %q, want %q", got, plaintext)
	}
}

func TestUniqueNonces(t *testing.T) {
	pub, priv, _ := GenerateKeypair()
	plaintext := []byte("test")

	nonces := make(map[[24]byte]bool)
	for i := 0; i < 1000; i++ {
		nonce, _, err := SealMessage(plaintext, pub, priv)
		if err != nil {
			t.Fatalf("SealMessage iteration %d: %v", i, err)
		}
		if nonces[nonce] {
			t.Fatalf("duplicate nonce at iteration %d", i)
		}
		nonces[nonce] = true
	}
}

func TestFingerprint(t *testing.T) {
	pub, _, _ := GenerateKeypair()
	fp := Fingerprint(pub)

	// Should be 8 groups of 4 hex chars separated by spaces
	if len(fp) != 39 { // 8*4 + 7 spaces
		t.Errorf("fingerprint length = %d, want 39: %q", len(fp), fp)
	}

	// Same key should give same fingerprint
	fp2 := Fingerprint(pub)
	if fp != fp2 {
		t.Error("fingerprint not deterministic")
	}
}

func TestShortFingerprint(t *testing.T) {
	pub, _, _ := GenerateKeypair()
	sfp := ShortFingerprint(pub)
	if len(sfp) == 0 {
		t.Error("short fingerprint is empty")
	}
	// Should contain "..."
	if !bytes.Contains([]byte(sfp), []byte("...")) {
		t.Errorf("short fingerprint missing ellipsis: %q", sfp)
	}
}

func TestPubKeyBase64RoundTrip(t *testing.T) {
	pub, _, _ := GenerateKeypair()
	encoded := PubKeyToBase64(pub)
	decoded, err := PubKeyFromBase64(encoded)
	if err != nil {
		t.Fatalf("PubKeyFromBase64: %v", err)
	}
	if *pub != *decoded {
		t.Error("base64 round-trip failed")
	}
}
