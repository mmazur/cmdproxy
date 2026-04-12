package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type ServerCommandConfig struct {
	Allow []string `toml:"allow"`
	Deny  []string `toml:"deny"`
}

type ServerConfig struct {
	Commands map[string]ServerCommandConfig `toml:"command"`
}

func LoadServerConfig(profileName string) (ServerConfig, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ServerConfig{}, fmt.Errorf("user config dir: %w", err)
	}

	path := filepath.Join(configDir, "cmdproxy", "profiles", profileName+".toml")
	var cfg ServerConfig
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return ServerConfig{}, fmt.Errorf("load server config %s: %w", path, err)
	}
	return cfg, nil
}
