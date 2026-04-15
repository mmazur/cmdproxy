package config

import (
	"testing"

	"github.com/BurntSushi/toml"
)

func TestParseRawConfig_MixedAllowTypes(t *testing.T) {
	input := `
[command.az]
allow = [
	["account", "show", "*:*"],
	"acr list *",
	["[a-z]*:+", "--help"],
]
deny = [
	"* --delete*",
	["[a-z]*:*", "delete", "*:*"],
]
`
	var raw rawServerConfig
	if _, err := toml.Decode(input, &raw); err != nil {
		t.Fatalf("toml.Decode error: %v", err)
	}

	cfg, err := parseRawConfig(raw)
	if err != nil {
		t.Fatalf("parseRawConfig error: %v", err)
	}

	az, ok := cfg.Commands["az"]
	if !ok {
		t.Fatal("expected command 'az'")
	}

	if len(az.Allow) != 3 {
		t.Fatalf("allow: got %d rules, want 3", len(az.Allow))
	}

	// First: segment rule
	if az.Allow[0].Segments == nil {
		t.Error("allow[0]: expected segment rule, got glob")
	}
	if len(az.Allow[0].Segments) != 3 {
		t.Errorf("allow[0]: got %d segments, want 3", len(az.Allow[0].Segments))
	}

	// Second: legacy glob
	if az.Allow[1].LegacyGlob != "acr list *" {
		t.Errorf("allow[1]: got glob %q, want %q", az.Allow[1].LegacyGlob, "acr list *")
	}

	// Third: segment rule
	if az.Allow[2].Segments == nil {
		t.Error("allow[2]: expected segment rule, got glob")
	}

	if len(az.Deny) != 2 {
		t.Fatalf("deny: got %d rules, want 2", len(az.Deny))
	}

	// First deny: legacy glob
	if az.Deny[0].LegacyGlob != "* --delete*" {
		t.Errorf("deny[0]: got glob %q, want %q", az.Deny[0].LegacyGlob, "* --delete*")
	}

	// Second deny: segment rule
	if az.Deny[1].Segments == nil {
		t.Error("deny[1]: expected segment rule, got glob")
	}
}

func TestParseRawConfig_InvalidEntries(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name: "bad glob in allow",
			input: `
[command.az]
allow = ["[invalid"]
`,
		},
		{
			name: "bad quantifier in allow segment",
			input: `
[command.az]
allow = [["foo:badquant"]]
`,
		},
		{
			name: "non-string in segment array",
			input: `
[command.az]
allow = [[42]]
`,
		},
		{
			name: "bad glob in deny",
			input: `
[command.az]
deny = ["[invalid"]
`,
		},
		{
			name: "bad quantifier in deny segment",
			input: `
[command.az]
deny = [["foo:badquant"]]
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var raw rawServerConfig
			if _, err := toml.Decode(tt.input, &raw); err != nil {
				t.Fatalf("toml.Decode error: %v", err)
			}
			_, err := parseRawConfig(raw)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}
