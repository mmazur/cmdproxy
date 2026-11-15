package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mmazur/cmdproxy/internal/config"
)

func loadNamedTestConfig(t *testing.T, contents string) config.ServerConfig {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	dir := filepath.Join(root, "cmdproxy", "profiles")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "test.toml"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadServerConfig("test")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestNamedPolicyEvaluation(t *testing.T) {
	for _, options := range []string{
		`[['--workspace', 'sandbox'], ['{--name,-n}', 'scratch-*']]`,
		`[{ names = ['--workspace'], values = ['sandbox'] }, { names = ['--name', '-n'], values = ['scratch-*'], occurs = '1' }]`,
	} {
		t.Run(options, func(t *testing.T) {
			cfg := loadNamedTestConfig(t, `
stdin = false
[command.tool]
allow = ['legacy *', ['status'], { prefix = ['group', 'remove'], options = `+options+` }]
deny = [
  { prefix = ['group', 'remove'], options = [['--workspace', '*'], ['{--name,-n}', 'scratch-protected']] },
  '*scratch-blocked*',
  ['group', 'remove', '*:*', 'scratch-reserved', '*:*'],
]

[command.fallback]
allow = ['*', { prefix = ['run'], options = [['--name', 'scratch-*']] }]
`)
			for _, tc := range []struct {
				command string
				args    string
				verdict Verdict
			}{
				{"tool", "group remove --workspace sandbox -n scratch-one", Allow},
				{"tool", "group remove --name=scratch-one --workspace=sandbox", Allow},
				{"tool", "group remove --workspace sandbox -n scratch-protected", Deny},
				{"tool", "group remove --workspace sandbox -n scratch-blocked", Deny},
				{"tool", "group remove --workspace sandbox -n scratch-reserved", Deny},
				{"tool", "group remove --workspace sandbox -n scratch-one --name=scratch-two", Deny},
				{"tool", "group remove --workspace sandbox -n unrelated", Deny},
				{"tool", "status", Allow},
				{"tool", "legacy anything", Allow},
				{"missing", "status", Deny},
				// A failed rule is not a global veto; any other allow still wins.
				{"fallback", "run --name other --name scratch-one", Allow},
			} {
				if got := Evaluate(cfg, tc.command, strings.Fields(tc.args)); got.Verdict != tc.verdict {
					t.Errorf("%s %s: %v, want %v", tc.command, tc.args, got, tc.verdict)
				}
			}
			decision := Evaluate(cfg, "tool", strings.Fields("group remove --workspace sandbox -n scratch-one"))
			if decision.Reason != "args matched named-option allow rule" {
				t.Errorf("incorrect reason: %s", decision.Reason)
			}
			decision = Evaluate(cfg, "tool", strings.Fields("group remove --workspace sandbox -n scratch-protected"))
			if decision.Reason != "args matched named-option deny rule" {
				t.Errorf("incorrect deny reason: %s", decision.Reason)
			}
			if cfg.Stdin || cfg.Commands["tool"].Stdin || cfg.Commands["tool"].AllowFileTransfer {
				t.Fatal("structured rules changed execution controls")
			}
		})
	}
}

func TestNamedDenyMatchesOnlyDeclaredShape(t *testing.T) {
	cfg := loadNamedTestConfig(t, `
[command.tool]
allow = ['item *']
deny = [{ prefix = ['item', 'show'], options = [['--name', 'scratch-protected']] }]
`)
	for _, tc := range []struct {
		args string
		want Verdict
	}{
		{"item show --name scratch-protected", Deny},
		{"item show --name scratch-protected --verbose", Allow},
		{"item show --name scratch-protected extra", Allow},
		{"item show --name scratch-protected --", Allow},
		{"item show --nam scratch-protected", Allow},
	} {
		if got := Evaluate(cfg, "tool", strings.Fields(tc.args)); got.Verdict != tc.want {
			t.Errorf("%s: %v, want %v", tc.args, got, tc.want)
		}
	}
}

func TestNamedSelectorAlternatives(t *testing.T) {
	cfg := loadNamedTestConfig(t, `
[value_sets]
owned_ids = ['workspace/sandbox/items/scratch-*']
[[command.tool.allow]]
prefix = ['item', 'remove']
options = [['--workspace', 'sandbox'], ['{--name,-n}', 'scratch-*']]

[[command.tool.allow]]
prefix = ['item', 'remove']
options = [
  { names = ['--ids'], values_ref = 'owned_ids' },
  { names = ['--workspace'], values = ['sandbox'], occurs = '?' },
]

[[command.tool.allow]]
prefix = ['batch', 'remove']
options = [['--ids', 'workspace/sandbox/items/scratch-*:+']]

# The meaning of an option belongs to this particular command rule.
[[command.tool.allow]]
prefix = ['document', 'create']
options = [['--workspace', 'sandbox'], ['--scope', '/']]
`)
	for _, tc := range []struct {
		args string
		want Verdict
	}{
		{"item remove --workspace sandbox -n scratch-one", Allow},
		{"item remove --ids workspace/sandbox/items/scratch-one", Allow},
		{"item remove --workspace sandbox --ids workspace/sandbox/items/scratch-one", Allow},
		{"item remove --ids workspace/sandbox/items/scratch-one --workspace=sandbox", Allow},
		{"item remove --ids workspace/other/items/scratch-one", Deny},
		{"item remove --ids workspace/sandbox/items/scratch-one --workspace=other", Deny},
		{"item remove --ids workspace/sandbox/items/scratch-one -n scratch-one --workspace sandbox", Deny},
		{"item remove --workspace sandbox -n scratch-one --ids workspace/sandbox/items/scratch-one", Deny},
		{"item remove --ids workspace/sandbox/items/scratch-one workspace/sandbox/items/scratch-two", Deny},
		{"batch remove --ids workspace/sandbox/items/scratch-one workspace/sandbox/items/scratch-two", Allow},
		{"batch remove --ids workspace/sandbox/items/scratch-one workspace/other/items/scratch-two", Deny},
		{"batch remove --ids workspace/other/items/scratch-one workspace/sandbox/items/scratch-two", Deny},
		{"batch remove --ids workspace/sandbox/items/scratch-one --ids workspace/sandbox/items/scratch-two", Deny},
		{"document create --scope / --workspace sandbox", Allow},
		{"document create --scope /other --workspace sandbox", Deny},
	} {
		if got := Evaluate(cfg, "tool", strings.Fields(tc.args)); got.Verdict != tc.want {
			t.Errorf("%s: %v, want %v", tc.args, got, tc.want)
		}
	}
}
