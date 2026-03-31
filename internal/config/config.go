package config

import (
	"os"

	"github.com/BurntSushi/toml"
)

// Config holds the client-side configuration.
type Config struct {
	DisplayName string       `toml:"display_name"`
	Registered  bool         `toml:"registered"`
	Server      ServerConfig `toml:"server"`
}

type ServerConfig struct {
	Address string `toml:"address"`
}

// ServerRunConfig holds configuration for running the enclave server.
type ServerRunConfig struct {
	Bind       string `toml:"bind"`
	DBPath     string `toml:"db_path"`
	MaxUsers   int    `toml:"max_users"`
	InviteOnly bool   `toml:"invite_only"`
	LogLevel   string `toml:"log_level"`
}

func DefaultConfig() Config {
	return Config{
		Server: ServerConfig{
			Address: "localhost:9300",
		},
	}
}

func DefaultServerRunConfig() ServerRunConfig {
	return ServerRunConfig{
		Bind:       "0.0.0.0:9200",
		DBPath:     "enclave-server.db",
		MaxUsers:   50,
		InviteOnly: true,
		LogLevel:   "info",
	}
}

// Load reads the config from ~/.enclave/config.toml.
func Load() (Config, error) {
	cfg := DefaultConfig()
	path := ConfigPath()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}

	if err := toml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Save writes the config to ~/.enclave/config.toml.
func Save(cfg Config) error {
	if err := EnsureDataDir(); err != nil {
		return err
	}

	f, err := os.Create(ConfigPath())
	if err != nil {
		return err
	}
	defer f.Close()

	return toml.NewEncoder(f).Encode(cfg)
}
