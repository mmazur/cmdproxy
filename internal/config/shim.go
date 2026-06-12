package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/mmazur/cmdproxy/internal/policy/argmatch"
)

type UploadRule struct {
	Match    []argmatch.Segment
	FileArgs []string
}

type ShimCommandConfig struct {
	Target   string   `toml:"target"`
	SSHArgs  []string `toml:"ssh_args"`
	RunLocal []Rule   `toml:"-"`
	LocalBin string   `toml:"local_bin"`
	Upload   []UploadRule
}

type ShimConfig struct {
	Target   string                       `toml:"target"`
	SSHArgs  []string                     `toml:"ssh_args"`
	Commands map[string]ShimCommandConfig `toml:"command"`
}

type rawShimCommandConfig struct {
	Target   string   `toml:"target"`
	SSHArgs  []string `toml:"ssh_args"`
	RunLocal []any    `toml:"run_local"`
	LocalBin string   `toml:"local_bin"`
	Upload   []struct {
		Match    []string `toml:"match"`
		FileArgs []string `toml:"file_args"`
	} `toml:"upload"`
}

type rawShimConfig struct {
	Target   string                          `toml:"target"`
	SSHArgs  []string                        `toml:"ssh_args"`
	Commands map[string]rawShimCommandConfig `toml:"command"`
}

func LoadShimConfig() (ShimConfig, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ShimConfig{}, fmt.Errorf("user config dir: %w", err)
	}

	path := filepath.Join(configDir, "cmdproxy", "shim.toml")
	var raw rawShimConfig
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return ShimConfig{}, fmt.Errorf("load shim config %s: %w", path, err)
	}
	return parseRawShimConfig(raw)
}

func parseRawShimConfig(raw rawShimConfig) (ShimConfig, error) {
	cfg := ShimConfig{
		Target:   raw.Target,
		SSHArgs:  raw.SSHArgs,
		Commands: make(map[string]ShimCommandConfig, len(raw.Commands)),
	}

	for cmd, rawCC := range raw.Commands {
		runLocal, err := ParseRules(rawCC.RunLocal, fmt.Sprintf("command.%s.run_local", cmd))
		if err != nil {
			return ShimConfig{}, err
		}
		upload := make([]UploadRule, 0, len(rawCC.Upload))
		for i, rawUpload := range rawCC.Upload {
			if len(rawUpload.Match) == 0 {
				return ShimConfig{}, fmt.Errorf("command.%s.upload[%d]: match is required", cmd, i)
			}
			if len(rawUpload.FileArgs) == 0 {
				return ShimConfig{}, fmt.Errorf("command.%s.upload[%d]: file_args is required", cmd, i)
			}
			match, err := argmatch.ParseSegments(rawUpload.Match)
			if err != nil {
				return ShimConfig{}, fmt.Errorf("command.%s.upload[%d].match: %w", cmd, i, err)
			}
			upload = append(upload, UploadRule{
				Match:    match,
				FileArgs: rawUpload.FileArgs,
			})
		}
		cfg.Commands[cmd] = ShimCommandConfig{
			Target:   rawCC.Target,
			SSHArgs:  rawCC.SSHArgs,
			RunLocal: runLocal,
			LocalBin: rawCC.LocalBin,
			Upload:   upload,
		}
	}

	return cfg, nil
}

func (c *ShimConfig) RunLocalMatch(cmd string, args []string) bool {
	cc, ok := c.Commands[cmd]
	if !ok {
		return false
	}
	for _, rule := range cc.RunLocal {
		if MatchRule(rule, args) {
			return true
		}
	}
	return false
}

func (c *ShimConfig) LocalBinPath(cmd string) (string, error) {
	if cc, ok := c.Commands[cmd]; ok && cc.LocalBin != "" {
		return cc.LocalBin, nil
	}

	shimExe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot determine shim executable: %w", err)
	}
	shimResolved, err := filepath.EvalSymlinks(shimExe)
	if err != nil {
		shimResolved = shimExe
	}

	pathEnv := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(pathEnv) {
		candidate := filepath.Join(dir, cmd)
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			resolved = candidate
		}
		if resolved == shimResolved {
			continue
		}
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("no local binary found for %q in PATH (skipping shim)", cmd)
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

func (c *ShimConfig) UploadRulesForCommand(cmd string) []UploadRule {
	if cc, ok := c.Commands[cmd]; ok {
		return cc.Upload
	}
	return nil
}

type TargetMode int

const (
	TargetSSH TargetMode = iota
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
