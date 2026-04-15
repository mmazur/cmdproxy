package argmatch

import (
	"testing"
)

// helper to parse segments or fail
func mustParse(t *testing.T, patterns []string) []Segment {
	t.Helper()
	segs, err := ParseSegments(patterns)
	if err != nil {
		t.Fatalf("ParseSegments(%v) error: %v", patterns, err)
	}
	return segs
}

func TestMatch(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		args     []string
		want     bool
	}{
		// === Exact positional matching ===
		{
			name:     "exact match two args",
			patterns: []string{"account", "show"},
			args:     []string{"account", "show"},
			want:     true,
		},
		{
			name:     "exact match no args",
			patterns: nil,
			args:     nil,
			want:     true,
		},
		{
			name:     "exact match empty patterns empty args",
			patterns: []string{},
			args:     []string{},
			want:     true,
		},
		{
			name:     "too few args",
			patterns: []string{"account", "show"},
			args:     []string{"account"},
			want:     false,
		},
		{
			name:     "too many args",
			patterns: []string{"account"},
			args:     []string{"account", "show"},
			want:     false,
		},
		{
			name:     "wrong arg",
			patterns: []string{"account", "show"},
			args:     []string{"account", "list"},
			want:     false,
		},
		{
			name:     "no patterns, has args",
			patterns: nil,
			args:     []string{"something"},
			want:     false,
		},
		{
			name:     "has patterns, no args",
			patterns: []string{"account"},
			args:     nil,
			want:     false,
		},

		// === Single-arg glob matching ===
		{
			name:     "star matches any single arg",
			patterns: []string{"account", "*"},
			args:     []string{"account", "show"},
			want:     true,
		},
		{
			name:     "star requires an arg",
			patterns: []string{"account", "*"},
			args:     []string{"account"},
			want:     false,
		},
		{
			name:     "prefix glob",
			patterns: []string{"list*"},
			args:     []string{"list-all"},
			want:     true,
		},
		{
			name:     "prefix glob no match",
			patterns: []string{"list*"},
			args:     []string{"get-all"},
			want:     false,
		},
		{
			name:     "char class match",
			patterns: []string{"[a-z]*"},
			args:     []string{"hello"},
			want:     true,
		},
		{
			name:     "char class rejects dash prefix",
			patterns: []string{"[a-z]*"},
			args:     []string{"--help"},
			want:     false,
		},

		// === Zero or more (:*) ===
		{
			name:     "star quant matches zero args",
			patterns: []string{"cmd", "*:*"},
			args:     []string{"cmd"},
			want:     true,
		},
		{
			name:     "star quant matches multiple args",
			patterns: []string{"cmd", "*:*"},
			args:     []string{"cmd", "a", "b", "c"},
			want:     true,
		},
		{
			name:     "star quant alone matches empty",
			patterns: []string{"*:*"},
			args:     nil,
			want:     true,
		},
		{
			name:     "star quant alone matches many",
			patterns: []string{"*:*"},
			args:     []string{"a", "b", "c"},
			want:     true,
		},

		// === One or more (:+) ===
		{
			name:     "plus quant requires at least one",
			patterns: []string{"*:+"},
			args:     nil,
			want:     false,
		},
		{
			name:     "plus quant matches one",
			patterns: []string{"*:+"},
			args:     []string{"a"},
			want:     true,
		},
		{
			name:     "plus quant matches many",
			patterns: []string{"*:+"},
			args:     []string{"a", "b", "c"},
			want:     true,
		},
		{
			name:     "plus with glob constraint",
			patterns: []string{"[a-z]*:+"},
			args:     []string{"foo", "bar"},
			want:     true,
		},
		{
			name:     "plus with glob rejects non-matching",
			patterns: []string{"[a-z]*:+"},
			args:     []string{"foo", "--bar"},
			want:     false,
		},

		// === Optional (:?) ===
		{
			name:     "optional present",
			patterns: []string{"get", "*:?"},
			args:     []string{"get", "pods"},
			want:     true,
		},
		{
			name:     "optional absent",
			patterns: []string{"get", "*:?"},
			args:     []string{"get"},
			want:     true,
		},
		{
			name:     "optional rejects two",
			patterns: []string{"get", "*:?"},
			args:     []string{"get", "pods", "extra"},
			want:     false,
		},

		// === Exact numeric (:N) ===
		{
			name:     "exact 2",
			patterns: []string{"a:2", "b"},
			args:     []string{"a", "a", "b"},
			want:     true,
		},
		{
			name:     "exact 2 too few",
			patterns: []string{"a:2", "b"},
			args:     []string{"a", "b"},
			want:     false,
		},
		{
			name:     "exact 2 too many",
			patterns: []string{"a:2", "b"},
			args:     []string{"a", "a", "a", "b"},
			want:     false,
		},
		{
			name:     "exact 0 skips",
			patterns: []string{"a:0", "b"},
			args:     []string{"b"},
			want:     true,
		},

		// === N or more (:N+) ===
		{
			name:     "2+ matches exactly 2",
			patterns: []string{"*:2+", "end"},
			args:     []string{"a", "b", "end"},
			want:     true,
		},
		{
			name:     "2+ matches more than 2",
			patterns: []string{"*:2+", "end"},
			args:     []string{"a", "b", "c", "end"},
			want:     true,
		},
		{
			name:     "2+ rejects fewer than 2",
			patterns: []string{"*:2+", "end"},
			args:     []string{"a", "end"},
			want:     false,
		},

		// === Range (:N-M) ===
		{
			name:     "range 1-3 matches 1",
			patterns: []string{"a:1-3", "end"},
			args:     []string{"a", "end"},
			want:     true,
		},
		{
			name:     "range 1-3 matches 2",
			patterns: []string{"a:1-3", "end"},
			args:     []string{"a", "a", "end"},
			want:     true,
		},
		{
			name:     "range 1-3 matches 3",
			patterns: []string{"a:1-3", "end"},
			args:     []string{"a", "a", "a", "end"},
			want:     true,
		},
		{
			name:     "range 1-3 rejects 0",
			patterns: []string{"a:1-3", "end"},
			args:     []string{"end"},
			want:     false,
		},
		{
			name:     "range 1-3 rejects 4",
			patterns: []string{"a:1-3", "end"},
			args:     []string{"a", "a", "a", "a", "end"},
			want:     false,
		},

		// === Backtracking ===
		{
			name:     "star quant must not consume the last arg",
			patterns: []string{"*:*", "end"},
			args:     []string{"a", "b", "end"},
			want:     true,
		},
		{
			name:     "star quant finds end among similar args",
			patterns: []string{"*:*", "end"},
			args:     []string{"end"},
			want:     true,
		},
		{
			name:     "two variable segments backtrack",
			patterns: []string{"[a-z]*:+", "[0-9]*:+"},
			args:     []string{"foo", "bar", "123"},
			want:     true,
		},
		{
			name:     "two variable segments minimal",
			patterns: []string{"[a-z]*:+", "[0-9]*:+"},
			args:     []string{"x", "1"},
			want:     true,
		},
		{
			name:     "two variable segments fail no digits",
			patterns: []string{"[a-z]*:+", "[0-9]*:+"},
			args:     []string{"foo", "bar"},
			want:     false,
		},
		{
			name:     "two variable segments fail no letters",
			patterns: []string{"[a-z]*:+", "[0-9]*:+"},
			args:     []string{"123", "456"},
			want:     false,
		},
		{
			name:     "backtrack with middle literal",
			patterns: []string{"*:*", "mid", "*:*"},
			args:     []string{"a", "b", "mid", "c"},
			want:     true,
		},
		{
			name:     "backtrack with middle literal at start",
			patterns: []string{"*:*", "mid", "*:*"},
			args:     []string{"mid", "c"},
			want:     true,
		},
		{
			name:     "backtrack with middle literal at end",
			patterns: []string{"*:*", "mid", "*:*"},
			args:     []string{"a", "mid"},
			want:     true,
		},
		{
			name:     "backtrack with middle literal alone",
			patterns: []string{"*:*", "mid", "*:*"},
			args:     []string{"mid"},
			want:     true,
		},
		{
			name:     "backtrack no middle literal",
			patterns: []string{"*:*", "mid", "*:*"},
			args:     []string{"a", "b", "c"},
			want:     false,
		},

		// === Real-world-ish patterns ===
		{
			name:     "az subcommands then --help",
			patterns: []string{"[a-z]*:+", "--help"},
			args:     []string{"acr", "list", "--help"},
			want:     true,
		},
		{
			name:     "az single subcmd --help",
			patterns: []string{"[a-z]*:+", "--help"},
			args:     []string{"account", "--help"},
			want:     true,
		},
		{
			name:     "az rejects flag before subcmd --help",
			patterns: []string{"[a-z]*:+", "--help"},
			args:     []string{"-q", "account", "--help"},
			want:     false,
		},
		{
			name:     "az subcommands then list then trailing",
			patterns: []string{"[a-z]*:*", "list*", "*:*"},
			args:     []string{"list"},
			want:     true,
		},
		{
			name:     "az subcommands then list-show trailing",
			patterns: []string{"[a-z]*:*", "list*", "*:*"},
			args:     []string{"acr", "list-all", "--output", "json"},
			want:     true,
		},
		{
			name:     "az deep subcommands then list",
			patterns: []string{"[a-z]*:*", "list*", "*:*"},
			args:     []string{"cmd1", "cmd2", "list"},
			want:     true,
		},
		{
			name:     "az rejects no list",
			patterns: []string{"[a-z]*:*", "list*", "*:*"},
			args:     []string{"cmd1", "cmd2"},
			want:     false,
		},
		{
			name:     "az rejects flag in subcmds",
			patterns: []string{"[a-z]*:*", "list*", "*:*"},
			args:     []string{"-q", "list"},
			want:     false,
		},
		{
			name:     "acr show with exact args",
			patterns: []string{"acr", "show", "*"},
			args:     []string{"acr", "show", "myregistry"},
			want:     true,
		},
		{
			name:     "acr show rejects no arg",
			patterns: []string{"acr", "show", "*"},
			args:     []string{"acr", "show"},
			want:     false,
		},
		{
			name:     "acr show rejects extra args",
			patterns: []string{"acr", "show", "*"},
			args:     []string{"acr", "show", "a", "b"},
			want:     false,
		},

		// === Case insensitivity ===
		{
			name:     "case insensitive literal",
			patterns: []string{"account", "show"},
			args:     []string{"Account", "SHOW"},
			want:     true,
		},
		{
			name:     "case insensitive glob",
			patterns: []string{"list*"},
			args:     []string{"List-All"},
			want:     true,
		},
		{
			name:     "case insensitive char class",
			patterns: []string{"[a-z]*:+", "--help"},
			args:     []string{"ACR", "List", "--HELP"},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			segs := mustParse(t, tt.patterns)
			got := Match(segs, tt.args)
			if got != tt.want {
				t.Errorf("Match(%v, %v) = %v, want %v", tt.patterns, tt.args, got, tt.want)
			}
		})
	}
}
