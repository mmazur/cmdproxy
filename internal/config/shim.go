package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type ShimCommandConfig struct {
	Target  string   `toml:"target"`
	SSHArgs []string `toml:"ssh_args"`
	Stdin   bool     `toml:"stdin"`
}

type ShimConfig struct {
	Target   string                       `toml:"target"`
	SSHArgs  []string                     `toml:"ssh_args"`
	Commands map[string]ShimCommandConfig `toml:"command"`
}

func LoadShimConfig() (ShimConfig, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ShimConfig{}, fmt.Errorf("user config dir: %w", err)
	}

	path := filepath.Join(configDir, "cmdproxy", "shim.toml")
	var cfg ShimConfig
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return ShimConfig{}, fmt.Errorf("load shim config %s: %w", path, err)
	}
	return cfg, nil
}

func (c *ShimConfig) TargetForCommand(cmd string) string {
	if cc, ok := c.Commands[cmd]; ok && cc.Target != "" {
		return cc.Target
	}
	return c.Target
}

func (c *ShimConfig) SSHArgsForCommand(cmd string) []string {
	if cc, ok := c.Commands[cmd]; ok && len(cc.SSHArgs) > 0 {
		return cc.SSHArgs
	}
	return c.SSHArgs
}

func (c *ShimConfig) StdinEnabled(cmd string) bool {
	if cc, ok := c.Commands[cmd]; ok {
		return cc.Stdin
	}
	return false
}
