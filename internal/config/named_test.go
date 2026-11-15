package config

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func decodeNamedConfig(t *testing.T, input string) (ServerConfig, error) {
	t.Helper()
	var raw rawServerConfig
	if _, err := toml.Decode(input, &raw); err != nil {
		t.Fatalf("TOML decode: %v", err)
	}
	return parseRawConfig(raw)
}

func TestNamedRuleSyntaxes(t *testing.T) {
	for _, options := range []string{
		`[['--workspace', 'sandbox'], ['{--name,-n}', 'scratch-*']]`,
		`[{ names = ['--workspace'], values = ['sandbox'], occurs = '1' }, { names = ['--name', '-n'], values = ['scratch-*'] }]`,
		`[['--workspace', 'sandbox'], { names = ['--name', '-n'], values_ref = 'owned_names' }]`,
	} {
		t.Run(options, func(t *testing.T) {
			cfg, err := decodeNamedConfig(t, `
[value_sets]
owned_names = ['scratch-*']
[command.tool]
allow = ['legacy *', ['status'], { prefix = ['group', 'remove'], options = `+options+` }]
deny = [{ prefix = ['group', 'remove'], options = [['--workspace', 'sandbox'], ['{--name,-n}', 'scratch-protected']] }]
`)
			if err != nil {
				t.Fatal(err)
			}
			cc := cfg.Commands["tool"]
			if len(cc.Allow) != 3 || cc.Allow[2].Named == nil || cc.Deny[0].Named == nil {
				t.Fatal("rule variants not decoded")
			}
			for _, args := range []string{
				"group remove --workspace sandbox -n scratch-one",
				"group remove --name=scratch-one --workspace=sandbox",
			} {
				if !MatchRule(cc.Allow[2], strings.Fields(args)) {
					t.Errorf("rejected %q", args)
				}
			}
			for _, args := range []string{
				"group remove --workspace sandbox -n scratch-one --name=scratch-two",
				"group remove --workspace sandbox",
				"group remove --workspace sandbox -n unrelated",
				"group remove --workspace sandbox -n=scratch-one",
			} {
				if MatchRule(cc.Allow[2], strings.Fields(args)) {
					t.Errorf("accepted %q", args)
				}
			}
			if !MatchRule(cc.Allow[0], []string{"LEGACY", "anything"}) || !MatchRule(cc.Allow[1], []string{"STATUS"}) {
				t.Fatal("legacy rules lost case-insensitive matching")
			}
		})
	}
}

func TestNamedEmptyValueReference(t *testing.T) {
	_, err := decodeNamedConfig(t, `
[value_sets]
empty = []
[[command.tool.allow]]
prefix = ['run']
options = [{ names = ['--name'], values_ref = 'empty' }]
`)
	if err == nil || !strings.Contains(err.Error(), `allow[0]: options[0]: values_ref: value set "empty" must not be empty`) {
		t.Fatalf("empty reference error = %v", err)
	}
}

func TestNamedArrayOfTables(t *testing.T) {
	cfg, err := decodeNamedConfig(t, `
[[command.tool.allow]]
prefix = ['exec']
positionals = ['worker-*']
after_double_dash = ['*:+']
options = [
  ['--config', '/tmp/sandbox/*.conf'],
  { names = ['--interactive', '-i'], values = [], occurs = '?' },
]

[[command.tool.allow]]
prefix = ['status']

[[command.tool.deny]]
prefix = ['exec']
positionals = ['worker-protected']
after_double_dash = ['*:+']
options = [['--config', '*']]
`)
	if err != nil {
		t.Fatal(err)
	}
	cc := cfg.Commands["tool"]
	if len(cc.Allow) != 2 || len(cc.Deny) != 1 || !MatchRule(cc.Allow[0], strings.Fields("exec worker-one --config /tmp/sandbox/a.conf -- id")) || !MatchRule(cc.Allow[1], []string{"status"}) {
		t.Fatal("array-of-tables decoding failed")
	}
}

func TestNamedCompactSwitchAndTuple(t *testing.T) {
	cfg, err := decodeNamedConfig(t, `
[[command.tool.allow]]
prefix = ['run']
options = [['--confirm'], ['--pair', 'left-*', 'right-*']]
`)
	if err != nil {
		t.Fatal(err)
	}
	rule := cfg.Commands["tool"].Allow[0]
	if !MatchRule(rule, strings.Fields("run --pair left-one right-two --confirm")) {
		t.Fatal("rejected compact switch/tuple")
	}
	for _, args := range []string{
		"run --pair left-one right-two",
		"run --confirm --pair right-two left-one",
		"run --confirm --pair left-one --confirm",
		"run --confirm=true --pair left-one right-two",
	} {
		if MatchRule(rule, strings.Fields(args)) {
			t.Errorf("accepted %q", args)
		}
	}
}

func TestNamedConfigErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		rule string
		want string
	}{
		{"missing prefix", `{ options = [] }`, "prefix must not be empty"},
		{"prefix type", `{ prefix = 'run' }`, "prefix:"},
		{"prefix item", `{ prefix = [42] }`, "prefix:"},
		{"prefix quantifier", `{ prefix = ['run:+'] }`, "prefix[0]"},
		{"unknown rule key", `{ prefix = ['run'], option = [] }`, `unknown field "option"`},
		{"options type", `{ prefix = ['run'], options = {} }`, "options:"},
		{"option type", `{ prefix = ['run'], options = [42] }`, "options[0]"},
		{"empty compact", `{ prefix = ['run'], options = [[]] }`, "compact option"},
		{"compact value type", `{ prefix = ['run'], options = [['--name', 42]] }`, "expected string"},
		{"compact wildcard names", `{ prefix = ['run'], options = [['--name*', '*']] }`, "invalid literal option name"},
		{"compact empty alias", `{ prefix = ['run'], options = [['{--name,}', '*']] }`, "invalid literal option name"},
		{"compact nested alias", `{ prefix = ['run'], options = [['{--name,{-n,-N}}', '*']] }`, "invalid literal option name"},
		{"missing names", `{ prefix = ['run'], options = [{ values = ['*'] }] }`, "names:"},
		{"names type", `{ prefix = ['run'], options = [{ names = '--name', values = ['*'] }] }`, "names:"},
		{"empty names", `{ prefix = ['run'], options = [{ names = [], values = ['*'] }] }`, "names must not be empty"},
		{"missing values", `{ prefix = ['run'], options = [{ names = ['--name'] }] }`, "exactly one of values or values_ref"},
		{"values type", `{ prefix = ['run'], options = [{ names = ['--name'], values = '*' }] }`, "values:"},
		{"values item", `{ prefix = ['run'], options = [{ names = ['--name'], values = [42] }] }`, "values:"},
		{"unknown option key", `{ prefix = ['run'], options = [{ names = ['--name'], values = ['*'], occur = '1' }] }`, `unknown field "occur"`},
		{"occurs type", `{ prefix = ['run'], options = [{ names = ['--name'], values = ['*'], occurs = 1 }] }`, "occurs:"},
		{"occurs empty", `{ prefix = ['run'], options = [{ names = ['--name'], values = ['*'], occurs = '' }] }`, "occurs:"},
		{"occurs invalid", `{ prefix = ['run'], options = [{ names = ['--name'], values = ['*'], occurs = 'many' }] }`, "occurs:"},
		{"reference type", `{ prefix = ['run'], options = [{ names = ['--name'], values_ref = [] }] }`, "values_ref:"},
		{"reference unknown", `{ prefix = ['run'], options = [{ names = ['--name'], values_ref = 'missing' }] }`, "unknown value set"},
		{"both values forms", `{ prefix = ['run'], options = [{ names = ['--name'], values = ['*'], values_ref = 'names' }] }`, "exactly one of values or values_ref"},
		{"duplicate aliases", `{ prefix = ['run'], options = [['{--name,-n}', '*'], ['-n', '*']] }`, "duplicate option alias"},
		{"positionals type", `{ prefix = ['run'], positionals = '*' }`, "positionals:"},
		{"tail type", `{ prefix = ['run'], after_double_dash = '*' }`, "after_double_dash:"},
		{"optional value", `{ prefix = ['run'], options = [['--name', '*:?']] }`, "minimum at least one"},
		{"ambiguous arity", `{ prefix = ['run'], positionals = ['*'], options = [['--names', '*:+']] }`, "cannot be combined with positionals"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeNamedConfig(t, "[command.tool]\nallow = ["+tc.rule+"]")
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), `command "tool": allow[0]:`) {
				t.Fatalf("error = %v; want rule path and %q", err, tc.want)
			}
		})
	}
	_, err := decodeNamedConfig(t, "[value_sets]\nbroken = ['[']")
	if err == nil || !strings.Contains(err.Error(), "value_sets.broken") {
		t.Fatalf("invalid unused value set error = %v", err)
	}
}

func TestShimRuleFormatsRemainUnchanged(t *testing.T) {
	var raw rawShimConfig
	_, err := toml.Decode(`
[command.tool]
run_local = ['version *', ['status']]
[[command.tool.upload]]
match = ['send', '*:*']
file_args = ['--file']
`, &raw)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := parseRawShimConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.RunLocalMatch("tool", []string{"STATUS"}) || !cfg.RunLocalMatch("tool", []string{"VERSION", "all"}) || len(cfg.UploadRulesForCommand("tool")) != 1 {
		t.Fatal("existing shim behavior changed")
	}
	if _, err := ParseRules([]any{map[string]any{"prefix": []any{"run"}}}, "run_local"); err == nil {
		t.Fatal("unexpectedly enabled server structured rules in shim")
	}
}
