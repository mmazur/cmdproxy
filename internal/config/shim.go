package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

type TargetMode int

const (
	TargetSSH    TargetMode = iota
	TargetSocket
)

type Target struct {
	Mode TargetMode
	Addr string // ssh user@host or socket path
}

func ParseTarget(raw string) (Target, error) {
	if strings.HasPrefix(raw, "socket:") {
		path, err := expandSocketPath(strings.TrimPrefix(raw, "socket:"))
		if err != nil {
			return Target{}, err
		}
		return Target{Mode: TargetSocket, Addr: path}, nil
	}
	return Target{Mode: TargetSSH, Addr: raw}, nil
}

func expandSocketPath(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("socket path is empty")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}

	if raw == "~" {
		raw = home
	} else if strings.HasPrefix(raw, "~/") {
		raw = filepath.Join(home, raw[2:])
	}

	raw = strings.ReplaceAll(raw, "$HOME", home)

	if !filepath.IsAbs(raw) {
		return "", fmt.Errorf("socket path must be absolute (start with / or ~), got: %s", raw)
	}

	return filepath.Clean(raw), nil
}
