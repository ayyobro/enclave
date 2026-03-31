package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"golang.org/x/crypto/nacl/box"

	"enclave/internal/config"
)

// GenerateKeypair creates a new X25519 keypair for NaCl box encryption.
func GenerateKeypair() (pub, priv *[32]byte, err error) {
	return box.GenerateKey(rand.Reader)
}

// SaveKeys writes the keypair to ~/.enclave/identity.key and identity.pub.
func SaveKeys(pub, priv *[32]byte) error {
	if err := config.EnsureDataDir(); err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}

	if err := os.WriteFile(config.IdentityKeyPath(), priv[:], 0600); err != nil {
		return fmt.Errorf("writing private key: %w", err)
	}

	if err := os.WriteFile(config.IdentityPubPath(), pub[:], 0644); err != nil {
		return fmt.Errorf("writing public key: %w", err)
	}

	return nil
}

// LoadKeys reads the keypair from disk.
func LoadKeys() (pub, priv *[32]byte, err error) {
	privBytes, err := os.ReadFile(config.IdentityKeyPath())
	if err != nil {
		return nil, nil, fmt.Errorf("reading private key: %w", err)
	}

	// Check permissions on private key
	info, err := os.Stat(config.IdentityKeyPath())
	if err != nil {
		return nil, nil, err
	}
	if info.Mode().Perm()&0077 != 0 {
		return nil, nil, fmt.Errorf("private key %s has too open permissions %v, should be 0600", config.IdentityKeyPath(), info.Mode().Perm())
	}

	pubBytes, err := os.ReadFile(config.IdentityPubPath())
	if err != nil {
		return nil, nil, fmt.Errorf("reading public key: %w", err)
	}

	if len(privBytes) != 32 || len(pubBytes) != 32 {
		return nil, nil, errors.New("invalid key size: expected 32 bytes")
	}

	priv = new([32]byte)
	pub = new([32]byte)
	copy(priv[:], privBytes)
	copy(pub[:], pubBytes)

	return pub, priv, nil
}

// KeysExist returns true if identity keys are already on disk.
func KeysExist() bool {
	_, err := os.Stat(config.IdentityKeyPath())
	return err == nil
}

// LoadContactKey reads a contact's public key from ~/.enclave/contacts/{name}.pub.
func LoadContactKey(name string) (*[32]byte, error) {
	path := config.ContactsPath() + "/" + name + ".pub"
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) != 32 {
		return nil, fmt.Errorf("invalid contact key size for %s: expected 32 bytes", name)
	}
	key := new([32]byte)
	copy(key[:], data)
	return key, nil
}

// SaveContactKey saves a contact's public key to disk.
func SaveContactKey(name string, pub *[32]byte) error {
	if err := config.EnsureDataDir(); err != nil {
		return err
	}
	path := config.ContactsPath() + "/" + name + ".pub"
	return os.WriteFile(path, pub[:], 0644)
}

// ListContacts returns the names of all saved contacts.
func ListContacts() ([]string, error) {
	entries, err := os.ReadDir(config.ContactsPath())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var names []string
	for _, e := range entries {
		name := e.Name()
		if len(name) > 4 && name[len(name)-4:] == ".pub" {
			names = append(names, name[:len(name)-4])
		}
	}
	return names, nil
}

// PubKeyToHex returns a hex-encoded public key string.
func PubKeyToHex(pub *[32]byte) string {
	return hex.EncodeToString(pub[:])
}

// PubKeyToBase64 returns a base64-encoded public key string.
func PubKeyToBase64(pub *[32]byte) string {
	return base64.StdEncoding.EncodeToString(pub[:])
}

// PubKeyFromBase64 decodes a base64-encoded public key.
func PubKeyFromBase64(s string) (*[32]byte, error) {
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid base64: %w", err)
	}
	if len(data) != 32 {
		return nil, fmt.Errorf("invalid key size: got %d, expected 32", len(data))
	}
	key := new([32]byte)
	copy(key[:], data)
	return key, nil
}
