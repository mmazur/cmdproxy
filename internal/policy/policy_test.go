package policy

import (
	"testing"

	"github.com/mmazur/cmdproxy/internal/config"
)

func TestEvaluate(t *testing.T) {
	cfg := config.ServerConfig{
		Commands: map[string]config.ServerCommandConfig{
			"az": {
				Allow: []string{
					"account show *",
					"group list *",
				},
				Deny: []string{
					"* --delete*",
				},
			},
			"jq": {
				Allow: []string{"*"},
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

func TestDecisionString(t *testing.T) {
	if Allow.String() != "allow" {
		t.Errorf("Allow.String() = %q, want %q", Allow.String(), "allow")
	}
	if Deny.String() != "deny" {
		t.Errorf("Deny.String() = %q, want %q", Deny.String(), "deny")
	}
}
