package config

import (
	"os"
	"path/filepath"
)

const (
	DirName         = ".enclave"
	IdentityKeyFile = "identity.key"
	IdentityPubFile = "identity.pub"
	ConfigFile      = "config.toml"
	ContactsDir     = "contacts"
	MessagesDB      = "messages.db"
)

// DataDir returns the path to ~/.enclave/, respecting ENCLAVE_HOME if set.
func DataDir() string {
	if d := os.Getenv("ENCLAVE_HOME"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", DirName)
	}
	return filepath.Join(home, DirName)
}

func IdentityKeyPath() string { return filepath.Join(DataDir(), IdentityKeyFile) }
func IdentityPubPath() string { return filepath.Join(DataDir(), IdentityPubFile) }
func ConfigPath() string      { return filepath.Join(DataDir(), ConfigFile) }
func ContactsPath() string    { return filepath.Join(DataDir(), ContactsDir) }
func MessagesDBPath() string  { return filepath.Join(DataDir(), MessagesDB) }

// EnsureDataDir creates ~/.enclave/ and its subdirectories if they don't exist.
func EnsureDataDir() error {
	dirs := []string{
		DataDir(),
		ContactsPath(),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0700); err != nil {
			return err
		}
	}
	return nil
}
