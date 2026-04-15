package policy

import (
	"strings"
	"testing"

	"github.com/gobwas/glob"

	"github.com/mmazur/cmdproxy/internal/config"
	"github.com/mmazur/cmdproxy/internal/policy/argmatch"
)

// mustGlobRule creates a legacy glob Rule or panics.
func mustGlobRule(pattern string) config.Rule {
	g, err := glob.Compile(strings.ToLower(pattern))
	if err != nil {
		panic(err)
	}
	return config.Rule{LegacyGlob: pattern, Compiled: g}
}

// mustSegRule creates a positional segment Rule or panics.
func mustSegRule(patterns ...string) config.Rule {
	segs, err := argmatch.ParseSegments(patterns)
	if err != nil {
		panic(err)
	}
	return config.Rule{Segments: segs}
}

func TestEvaluate(t *testing.T) {
	cfg := config.ServerConfig{
		Commands: map[string]config.ServerCommandConfig{
			"az": {
				Allow: []config.Rule{
					mustGlobRule("account show *"),
					mustGlobRule("group list *"),
				},
				Deny: []config.Rule{
					mustGlobRule("* --delete*"),
				},
			},
			"jq": {
				Allow: []config.Rule{
					mustGlobRule("*"),
				},
			},
		},
	}

	tests := []struct {
		name    string
		cmd     string
		args    []string
		verdict Verdict
	}{
		{
			name:    "command not in config",
			cmd:     "rm",
			args:    []string{"-rf", "/"},
			verdict: Deny,
		},
		{
			name:    "args match deny pattern",
			cmd:     "az",
			args:    []string{"account", "show", "--delete-all"},
			verdict: Deny,
		},
		{
			name:    "args match allow pattern",
			cmd:     "az",
			args:    []string{"group", "list", "--output", "table"},
			verdict: Allow,
		},
		{
			name:    "no allow pattern matches",
			cmd:     "az",
			args:    []string{"vm", "delete", "--name", "prod"},
			verdict: Deny,
		},
		{
			name:    "wildcard allow",
			cmd:     "jq",
			args:    []string{".foo", "bar.json"},
			verdict: Allow,
		},
		{
			name:    "empty args allowed by wildcard",
			cmd:     "jq",
			args:    nil,
			verdict: Allow,
		},
		{
			name:    "deny takes precedence over allow",
			cmd:     "az",
			args:    []string{"account", "show", "--delete"},
			verdict: Deny,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Evaluate(cfg, tt.cmd, tt.args)
			if d.Verdict != tt.verdict {
				t.Errorf("Verdict = %v (reason: %s), want %v", d.Verdict, d.Reason, tt.verdict)
			}
		})
	}
}

func TestEvaluateSegmentRules(t *testing.T) {
	cfg := config.ServerConfig{
		Commands: map[string]config.ServerCommandConfig{
			"az": {
				Allow: []config.Rule{
					// any subcommands starting with a letter, then --help
					mustSegRule("[a-z]*:+", "--help"),
					// any subcommands starting with a letter, then list*, then trailing
					mustSegRule("[a-z]*:*", "list*", "*:*"),
					// exact: account show
					mustSegRule("account", "show"),
					// acr login with one arg
					mustSegRule("acr", "login", "*"),
				},
				Deny: []config.Rule{
					mustGlobRule("* --delete*"),
				},
			},
		},
	}

	tests := []struct {
		name    string
		args    []string
		verdict Verdict
	}{
		// --help pattern
		{name: "subcmd --help", args: []string{"asdf", "--help"}, verdict: Allow},
		{name: "deep subcmd --help", args: []string{"asdf", "kljlsdf", "--help"}, verdict: Allow},
		{name: "flag before subcmd --help rejected", args: []string{"-q", "asdf", "--help"}, verdict: Deny},
		{name: "just --help rejected", args: []string{"--help"}, verdict: Deny},

		// list pattern
		{name: "list alone", args: []string{"list"}, verdict: Allow},
		{name: "list with arg", args: []string{"list", "--arg"}, verdict: Allow},
		{name: "cmd then list", args: []string{"cmd1", "cmd2", "list"}, verdict: Allow},
		{name: "list-allowed with arg", args: []string{"cmd1", "list-allowed", "--all"}, verdict: Allow},
		{name: "flag then list rejected", args: []string{"-q", "list"}, verdict: Deny},
		{name: "no list no help rejected", args: []string{"cmd"}, verdict: Deny},

		// exact account show
		{name: "account show exact", args: []string{"account", "show"}, verdict: Allow},
		{name: "account show extra rejected", args: []string{"account", "show", "extra"}, verdict: Deny},

		// acr login with arg
		{name: "acr login with registry", args: []string{"acr", "login", "myregistry"}, verdict: Allow},
		{name: "acr login no arg rejected", args: []string{"acr", "login"}, verdict: Deny},

		// deny still works with segment allows
		{name: "deny overrides segment allow", args: []string{"list", "--delete-all"}, verdict: Deny},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Evaluate(cfg, "az", tt.args)
			if d.Verdict != tt.verdict {
				t.Errorf("Verdict = %v (reason: %s), want %v", d.Verdict, d.Reason, tt.verdict)
			}
		})
	}
}

func TestEvaluateSegmentDeny(t *testing.T) {
	cfg := config.ServerConfig{
		Commands: map[string]config.ServerCommandConfig{
			"az": {
				Allow: []config.Rule{
					mustSegRule("[a-z]*:+", "*:*"),  // any subcommands with any trailing
				},
				Deny: []config.Rule{
					// deny any command path that ends with "delete" as a positional arg
					mustSegRule("[a-z]*:*", "delete", "*:*"),
					// deny --force anywhere
					mustGlobRule("*--force*"),
				},
			},
		},
	}

	tests := []struct {
		name    string
		args    []string
		verdict Verdict
	}{
		{name: "normal command allowed", args: []string{"vm", "list"}, verdict: Allow},
		{name: "show allowed", args: []string{"vm", "show", "--name", "prod"}, verdict: Allow},
		{name: "delete denied by segment", args: []string{"vm", "delete", "--name", "prod"}, verdict: Deny},
		{name: "deep delete denied", args: []string{"group", "sub", "delete"}, verdict: Deny},
		{name: "force denied by glob", args: []string{"vm", "restart", "--force"}, verdict: Deny},
		{name: "delete-like allowed", args: []string{"vm", "delete-lock"}, verdict: Allow},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Evaluate(cfg, "az", tt.args)
			if d.Verdict != tt.verdict {
				t.Errorf("Verdict = %v (reason: %s), want %v", d.Verdict, d.Reason, tt.verdict)
			}
		})
	}
}

func TestEvaluateMixedRules(t *testing.T) {
	cfg := config.ServerConfig{
		Commands: map[string]config.ServerCommandConfig{
			"az": {
				Allow: []config.Rule{
					mustGlobRule("account show *"),    // legacy glob
					mustSegRule("[a-z]*:+", "--help"), // segment rule
				},
			},
		},
	}

	tests := []struct {
		name    string
		args    []string
		verdict Verdict
	}{
		{name: "legacy glob matches", args: []string{"account", "show", "--output", "json"}, verdict: Allow},
		{name: "segment rule matches", args: []string{"vm", "create", "--help"}, verdict: Allow},
		{name: "neither matches", args: []string{"vm", "delete", "--force"}, verdict: Deny},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Evaluate(cfg, "az", tt.args)
			if d.Verdict != tt.verdict {
				t.Errorf("Verdict = %v (reason: %s), want %v", d.Verdict, d.Reason, tt.verdict)
			}
		})
	}
}

func TestDecisionString(t *testing.T) {
	if Allow.String() != "allow" {
		t.Errorf("Allow.String() = %q, want %q", Allow.String(), "allow")
	}
	if Deny.String() != "deny" {
		t.Errorf("Deny.String() = %q, want %q", Deny.String(), "deny")
	}
}
