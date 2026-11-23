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
// Exactly one of LegacyGlob, Segments, or Named is set.
type Rule struct {
	LegacyGlob string              // non-empty for plain string entries
	Compiled   glob.Glob           // pre-compiled glob (only for LegacyGlob)
	Segments   []argmatch.Segment  // non-nil for list entries
	Named      *argmatch.NamedRule // non-nil for structured server rules
}

type ServerCommandConfig struct {
	Allow             []Rule `toml:"-"`
	Deny              []Rule `toml:"-"`
	Stdin             bool   `toml:"-"`
	AllowFileTransfer bool   `toml:"allow_file_transfer"`
}

type ServerConfig struct {
	Stdin    bool                           `toml:"-"`
	Commands map[string]ServerCommandConfig `toml:"command"`
}

// raw types for TOML decoding before validation
type rawServerCommandConfig struct {
	Allow             []any `toml:"allow"`
	Deny              []any `toml:"deny"`
	Stdin             *bool `toml:"stdin"`
	AllowFileTransfer bool  `toml:"allow_file_transfer"`
}

type rawServerConfig struct {
	Stdin     *bool                             `toml:"stdin"`
	Commands  map[string]rawServerCommandConfig `toml:"command"`
	ValueSets map[string][]string               `toml:"value_sets"`
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
	for name, patterns := range raw.ValueSets {
		if _, err := argmatch.ParseCaseSensitiveSegments(patterns); err != nil {
			return ServerConfig{}, fmt.Errorf("value_sets.%s: %w", name, err)
		}
	}
	stdinDefault := true
	if raw.Stdin != nil {
		stdinDefault = *raw.Stdin
	}

	cfg := ServerConfig{
		Stdin:    stdinDefault,
		Commands: make(map[string]ServerCommandConfig, len(raw.Commands)),
	}

	for cmd, rawCC := range raw.Commands {
		cc, err := parseCommandConfig(rawCC, stdinDefault, raw.ValueSets)
		if err != nil {
			return ServerConfig{}, fmt.Errorf("command %q: %w", cmd, err)
		}
		cfg.Commands[cmd] = cc
	}

	return cfg, nil
}

func parseCommandConfig(raw rawServerCommandConfig, stdinDefault bool, valueSets map[string][]string) (ServerCommandConfig, error) {
	allow, err := parseRules(raw.Allow, "allow", true, valueSets)
	if err != nil {
		return ServerCommandConfig{}, err
	}
	deny, err := parseRules(raw.Deny, "deny", true, valueSets)
	if err != nil {
		return ServerCommandConfig{}, err
	}
	stdin := stdinDefault
	if raw.Stdin != nil {
		stdin = *raw.Stdin
	}
	return ServerCommandConfig{
		Allow:             allow,
		Deny:              deny,
		Stdin:             stdin,
		AllowFileTransfer: raw.AllowFileTransfer,
	}, nil
}

func MatchRule(rule Rule, args []string) bool {
	return MatchRuleResult(rule, args) == argmatch.Matched
}

// MatchRuleResult reports a match or, where the rule recognizes the command
// prefix, why it failed to match.
func MatchRuleResult(rule Rule, args []string) argmatch.MatchResult {
	if rule.Named != nil {
		return rule.Named.Check(args)
	}
	var matched bool
	if rule.LegacyGlob != "" {
		matched = rule.Compiled.Match(strings.ToLower(strings.Join(args, " ")))
	} else {
		matched = argmatch.Match(rule.Segments, args)
	}
	if matched {
		return argmatch.Matched
	}
	return argmatch.Unrelated
}

func ParseRules(entries []any, label string) ([]Rule, error) {
	return parseRules(entries, label, false, nil)
}

func parseRules(entries []any, label string, structured bool, valueSets map[string][]string) ([]Rule, error) {
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

		case map[string]any:
			if !structured {
				return nil, fmt.Errorf("%s[%d]: structured rules are only supported in server allow/deny", label, i)
			}
			named, err := parseNamedRule(v, valueSets)
			if err != nil {
				return nil, fmt.Errorf("%s[%d]: %w", label, i, err)
			}
			rules = append(rules, Rule{Named: named})

		default:
			return nil, fmt.Errorf("%s[%d]: expected string, array, or server rule table, got %T", label, i, entry)
		}
	}
	return rules, nil
}
