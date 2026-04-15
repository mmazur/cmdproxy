package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/gobwas/glob"

	"github.com/mmazur/cmdproxy/internal/policy/argmatch"
)

// Rule is a single entry in an allow or deny list.
// Either LegacyGlob or Segments is set, never both.
type Rule struct {
	LegacyGlob string             // non-empty for plain string entries
	Compiled   glob.Glob          // pre-compiled glob (only for LegacyGlob)
	Segments   []argmatch.Segment // non-nil for list entries
}

type ServerCommandConfig struct {
	Allow []Rule `toml:"-"`
	Deny  []Rule `toml:"-"`
}

type ServerConfig struct {
	Commands map[string]ServerCommandConfig `toml:"command"`
}

// raw types for TOML decoding before validation
type rawServerCommandConfig struct {
	Allow []any `toml:"allow"`
	Deny  []any `toml:"deny"`
}

type rawServerConfig struct {
	Commands map[string]rawServerCommandConfig `toml:"command"`
}

func LoadServerConfig(profileName string) (ServerConfig, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ServerConfig{}, fmt.Errorf("user config dir: %w", err)
	}

	path := filepath.Join(configDir, "cmdproxy", "profiles", profileName+".toml")
	var raw rawServerConfig
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return ServerConfig{}, fmt.Errorf("load server config %s: %w", path, err)
	}

	return parseRawConfig(raw)
}

func parseRawConfig(raw rawServerConfig) (ServerConfig, error) {
	cfg := ServerConfig{
		Commands: make(map[string]ServerCommandConfig, len(raw.Commands)),
	}

	for cmd, rawCC := range raw.Commands {
		cc, err := parseCommandConfig(rawCC)
		if err != nil {
			return ServerConfig{}, fmt.Errorf("command %q: %w", cmd, err)
		}
		cfg.Commands[cmd] = cc
	}

	return cfg, nil
}

func parseCommandConfig(raw rawServerCommandConfig) (ServerCommandConfig, error) {
	allow, err := parseRules(raw.Allow, "allow")
	if err != nil {
		return ServerCommandConfig{}, err
	}
	deny, err := parseRules(raw.Deny, "deny")
	if err != nil {
		return ServerCommandConfig{}, err
	}
	return ServerCommandConfig{Allow: allow, Deny: deny}, nil
}

func parseRules(entries []any, label string) ([]Rule, error) {
	var rules []Rule
	for i, entry := range entries {
		switch v := entry.(type) {
		case string:
			g, err := glob.Compile(strings.ToLower(v))
			if err != nil {
				return nil, fmt.Errorf("%s[%d]: bad glob %q: %w", label, i, v, err)
			}
			rules = append(rules, Rule{
				LegacyGlob: v,
				Compiled:   g,
			})

		case []any:
			patterns := make([]string, 0, len(v))
			for j, elem := range v {
				s, ok := elem.(string)
				if !ok {
					return nil, fmt.Errorf("%s[%d][%d]: expected string, got %T", label, i, j, elem)
				}
				patterns = append(patterns, s)
			}
			segs, err := argmatch.ParseSegments(patterns)
			if err != nil {
				return nil, fmt.Errorf("%s[%d]: %w", label, i, err)
			}
			rules = append(rules, Rule{Segments: segs})

		default:
			return nil, fmt.Errorf("%s[%d]: expected string or array, got %T", label, i, entry)
		}
	}
	return rules, nil
}
