package argmatch

import (
	"slices"
	"strings"
	"testing"
)

func compileNamedTest(t *testing.T, spec NamedSpec) *NamedRule {
	t.Helper()
	rule, err := CompileNamed(spec)
	if err != nil {
		t.Fatal(err)
	}
	return rule
}

func TestNamedOptions(t *testing.T) {
	rule := compileNamedTest(t, NamedSpec{
		Prefix: []string{"group", "{create,remove}"},
		Options: []OptionSpec{
			{Names: []string{"--workspace", "-w"}, Values: []string{"sandbox"}},
			{Names: []string{"--name", "-n"}, Values: []string{"scratch-*"}},
			{Names: []string{"--yes", "-y"}, Occurs: "?"},
			{Names: []string{"--format"}, Values: []string{"{json,text}"}, Occurs: "?"},
			{Names: []string{"--endpoint"}, Values: []string{"*"}, Occurs: "0"},
		},
	})
	tests := []struct {
		name string
		args string
		want bool
	}{
		{"normal", "group remove --workspace sandbox -n scratch-one", true},
		{"reordered", "group remove --name scratch-one -w sandbox", true},
		{"assignments", "group create --name=scratch-one --workspace=sandbox --yes --format=json", true},
		{"short assignment", "group create -n=scratch-one --workspace=sandbox", false},
		{"missing option", "group remove -n scratch-one", false},
		{"wrong workspace", "group remove -w elsewhere -n scratch-one", false},
		{"wrong name", "group remove -w sandbox -n unrelated", false},
		{"same alias repeated", "group remove -w sandbox -n scratch-one -n scratch-one", false},
		{"different alias repeated", "group remove -w sandbox --workspace=sandbox -n scratch-one", false},
		{"conflicting alias repeated", "group remove -w sandbox --workspace=elsewhere -n scratch-one", false},
		{"optional repeated", "group remove -w sandbox -n scratch-one --yes -y", false},
		{"forbidden", "group remove -w sandbox -n scratch-one --endpoint local", false},
		{"unknown", "group remove -w sandbox -n scratch-one --other scratch-two", false},
		{"unknown assignment", "group remove -w sandbox -n scratch-one --other=x", false},
		{"name abbreviation", "group remove --work sandbox -n scratch-one", false},
		{"joined short value", "group remove -wsandbox -n scratch-one", false},
		{"short cluster", "group remove -w sandbox -n scratch-one -yy", false},
		{"switch assignment", "group remove -w sandbox -n scratch-one --yes=true", false},
		{"missing value", "group remove -w sandbox --name", false},
		{"option as value", "group remove -w --name scratch-one", false},
		{"dash assignment", "group remove -w sandbox --name=--other", false},
		{"empty assignment", "group remove -w sandbox --name=", false},
		{"stray positional", "group remove -w sandbox -n scratch-one extra", false},
		{"wrong prefix", "remove group -w sandbox -n scratch-one", false},
		{"leading option", "--yes group remove -w sandbox -n scratch-one", false},
		{"option inside prefix", "group --yes remove -w sandbox -n scratch-one", false},
		{"prefix case", "Group remove -w sandbox -n scratch-one", false},
		{"name case", "group remove -W sandbox -n scratch-one", false},
		{"value case", "group remove -w Sandbox -n scratch-one", false},
		{"unexpected separator", "group remove -w sandbox -n scratch-one --", false},
		{"empty argv", "", false},
		{"short argv", "group", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rule.Match(strings.Fields(tt.args)); got != tt.want {
				t.Fatalf("Match(%q) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
	if rule.Match([]string{"group", "remove", "-w", "sandbox", "-n", ""}) {
		t.Fatal("accepted an empty separated value")
	}
}

func TestNamedPermutations(t *testing.T) {
	rule := compileNamedTest(t, NamedSpec{
		Prefix: []string{"run"},
		Options: []OptionSpec{
			{Names: []string{"--workspace", "-w"}, Values: []string{"sandbox"}},
			{Names: []string{"--name", "-n"}, Values: []string{"scratch-*"}},
			{Names: []string{"--yes"}},
		},
	})
	groups := [][]string{{"-w", "sandbox"}, {"--name=scratch-one"}, {"--yes"}}
	for i := range groups {
		for j := range groups {
			for k := range groups {
				if i == j || i == k || j == k {
					continue
				}
				args := slices.Concat([]string{"run"}, groups[i], groups[j], groups[k])
				if !rule.Match(args) {
					t.Errorf("rejected permutation %q", args)
				}
			}
		}
	}
}

func TestNamedSeparator(t *testing.T) {
	rule := compileNamedTest(t, NamedSpec{
		Prefix: []string{"exec"}, Positionals: []string{"worker-*"},
		Options: []OptionSpec{
			{Names: []string{"--config"}, Values: []string{"/tmp/sandbox/*.conf"}},
			{Names: []string{"--pool", "-p"}, Values: []string{"test"}, Occurs: "?"},
		},
		AfterDoubleDash: []string{"*:+"},
	})
	for _, args := range []string{
		"exec worker-one --config /tmp/sandbox/a.conf -- id",
		"exec --config=/tmp/sandbox/a.conf -p test worker-one -- sh --config other -- id",
		"exec -p test worker-one --config /tmp/sandbox/a.conf -- --config other",
	} {
		if !rule.Match(strings.Fields(args)) {
			t.Errorf("rejected %q", args)
		}
	}
	for _, args := range []string{
		"exec worker-one -- sh --config /tmp/sandbox/a.conf",
		"exec worker-one --config -- id",
		"exec worker-one --config /tmp/sandbox/a.conf",
		"exec worker-one --config /tmp/sandbox/a.conf --",
		"exec worker-one --config /tmp/sandbox/a.conf --config=/tmp/sandbox/a.conf -- id",
		"exec worker-one --config /tmp/sandbox/a.conf --endpoint other -- id",
		"exec worker-one --config /tmp/sandbox/A.CONF -- id",
		"exec --pool worker-one --config /tmp/sandbox/a.conf -- id",
	} {
		if rule.Match(strings.Fields(args)) {
			t.Errorf("accepted %q", args)
		}
	}
	emptyTail := compileNamedTest(t, NamedSpec{Prefix: []string{"exec"}, AfterDoubleDash: []string{}})
	if !emptyTail.Match([]string{"exec", "--"}) || emptyTail.Match([]string{"exec"}) || emptyTail.Match([]string{"exec", "--", "id"}) {
		t.Fatal("empty tail must require a separator and no arguments after it")
	}
	restricted := compileNamedTest(t, NamedSpec{Prefix: []string{"exec"}, AfterDoubleDash: []string{"echo", "hello"}})
	if !restricted.Match([]string{"exec", "--", "echo", "hello"}) || restricted.Match([]string{"exec", "--", "echo", "hello", "extra"}) {
		t.Fatal("tail must match completely")
	}
}

func TestNamedValuesAndOccurrences(t *testing.T) {
	for _, tc := range []struct {
		values []string
		good   []string
		bad    []string
	}{
		{[]string{"item-*"}, []string{"item-a"}, []string{"item-a", "item-b"}},
		{[]string{"item-*:+"}, []string{"item-a", "item-b", "item-c"}, []string{"item-a", "other", "item-c"}},
		{[]string{"item-*:2"}, []string{"item-a", "item-b"}, []string{"other", "item-b"}},
		{[]string{"item-*:2-3"}, []string{"item-a", "item-b"}, []string{"other", "item-b"}},
		{[]string{"item-*", "version-*"}, []string{"item-a", "version-one"}, []string{"other", "version-one"}},
	} {
		t.Run(strings.Join(tc.values, ","), func(t *testing.T) {
			r := compileNamedTest(t, NamedSpec{Prefix: []string{"run"}, Options: []OptionSpec{{Names: []string{"--items"}, Values: tc.values}}})
			if !r.Match(append([]string{"run", "--items"}, tc.good...)) {
				t.Fatal("rejected good values")
			}
			if r.Match(append([]string{"run", "--items"}, tc.bad...)) || r.Match([]string{"run", "--items"}) {
				t.Fatal("accepted bad or missing values")
			}
		})
	}
	for _, tc := range []struct {
		occurs string
		good   []int
		bad    []int
	}{
		{"1", []int{1}, []int{0, 2}},
		{"?", []int{0, 1}, []int{2}},
		{"0", []int{0}, []int{1}},
		{"2", []int{2}, []int{0, 1, 3}},
		{"1-3", []int{1, 2, 3}, []int{0, 4}},
		{"*", []int{0, 1, 4}, nil},
		{"+", []int{1, 4}, []int{0}},
		{"2+", []int{2, 4}, []int{0, 1}},
	} {
		t.Run(tc.occurs, func(t *testing.T) {
			r := compileNamedTest(t, NamedSpec{Prefix: []string{"run"}, Options: []OptionSpec{{Names: []string{"--label", "-l"}, Values: []string{"good-*"}, Occurs: tc.occurs}}})
			for _, n := range append(slices.Clone(tc.good), tc.bad...) {
				args := []string{"run"}
				for i := 0; i < n; i++ {
					if i%2 == 0 {
						args = append(args, "--label=good-a")
					} else {
						args = append(args, "-l", "good-b")
					}
				}
				if got, want := r.Match(args), slices.Contains(tc.good, n); got != want {
					t.Errorf("count %d: got %v, want %v", n, got, want)
				}
			}
			if r.Match([]string{"run", "--label=bad", "-l", "good-b"}) {
				t.Fatal("last occurrence hid a bad value")
			}
		})
	}
}

func TestNamedArityBoundaries(t *testing.T) {
	r := compileNamedTest(t, NamedSpec{
		Prefix: []string{"run"}, Positionals: []string{"destination"},
		Options: []OptionSpec{{Names: []string{"--pair"}, Values: []string{"item-*:2"}}},
	})
	if !r.Match(strings.Fields("run --pair item-a item-b destination")) || r.Match(strings.Fields("run --pair= item-a item-b destination")) {
		t.Fatal("fixed count must leave suffix positionals and reject assignments")
	}
	r = compileNamedTest(t, NamedSpec{Prefix: []string{"run"}, Options: []OptionSpec{
		{Names: []string{"--items"}, Values: []string{"item-*:1-2"}},
		{Names: []string{"--yes"}},
	}})
	if !r.Match(strings.Fields("run --items item-a item-b --yes")) {
		t.Fatal("variable values must stop at the next option")
	}
	for _, args := range []string{"run --items=item-a --yes", "run --items item-a item-b item-c --yes", "run --items --yes"} {
		if r.Match(strings.Fields(args)) {
			t.Errorf("accepted %q", args)
		}
	}
	assignment := compileNamedTest(t, NamedSpec{Prefix: []string{"run"}, Options: []OptionSpec{{Names: []string{"--expression"}, Values: []string{"a=b"}}}})
	if !assignment.Match([]string{"run", "--expression=a=b"}) {
		t.Fatal("assignment must split at the first equals sign only")
	}
	// Wildcards in positionals cannot hide unknown options or consume option values.
	positions := compileNamedTest(t, NamedSpec{Prefix: []string{"run"}, Positionals: []string{"*:*"}})
	if positions.Match(strings.Fields("run --unknown x")) || positions.Match(strings.Fields("run --")) {
		t.Fatal("positional wildcard swallowed an option or separator")
	}
}

func TestNamedDoesNotSearchInsideValues(t *testing.T) {
	rule := compileNamedTest(t, NamedSpec{Prefix: []string{"run"}, Options: []OptionSpec{
		{Names: []string{"--name"}, Values: []string{"scratch-*"}},
		{Names: []string{"--description"}, Values: []string{"*"}, Occurs: "?"},
	}})
	if rule.Match([]string{"run", "--description", "text --name scratch-one"}) {
		t.Fatal("option text inside a value satisfied the required name")
	}
	if !rule.Match([]string{"run", "--description", "text --name other", "--name=scratch-one"}) {
		t.Fatal("a value containing spaces was split into options")
	}
}

func TestNamedRejectsShortAssignments(t *testing.T) {
	rule := compileNamedTest(t, NamedSpec{Prefix: []string{"write"}, Options: []OptionSpec{
		{Names: []string{"--output", "-o"}, Values: []string{"/tmp/sandbox/*"}},
	}})
	for _, args := range [][]string{
		{"write", "-o=/tmp/sandbox/file"},
		{"write", "-o", "=/tmp/sandbox/file"},
	} {
		if rule.Match(args) {
			t.Errorf("accepted ambiguous or mismatched output path %q", args)
		}
	}
	for _, args := range [][]string{
		{"write", "-o", "/tmp/sandbox/file"},
		{"write", "--output=/tmp/sandbox/file"},
	} {
		if !rule.Match(args) {
			t.Errorf("rejected unambiguous output path %q", args)
		}
	}
}

func TestNamedSyntaxResults(t *testing.T) {
	spec := NamedSpec{
		Prefix: []string{"run"},
		Options: []OptionSpec{
			{Names: []string{"--name", "-n"}, Values: []string{"*"}, Occurs: "?"},
			{Names: []string{"--confirm", "-i"}, Occurs: "?"},
			{Names: []string{"--items"}, Values: []string{"*", "*"}, Occurs: "?"},
			{Names: []string{"--labels"}, Values: []string{"*:+"}, Occurs: "?"},
		},
	}
	strict := compileNamedTest(t, spec)
	spec.AllowExtraArgs = true
	permissive := compileNamedTest(t, spec)
	tail := compileNamedTest(t, NamedSpec{Prefix: spec.Prefix, AfterDoubleDash: []string{"-"}})

	for _, tc := range []struct {
		rule *NamedRule
		args []string
		want MatchResult
	}{
		{strict, []string{"other", "-n=value"}, Unrelated},
		{strict, []string{"run", "-n=value"}, ShortAssignment},
		{strict, []string{"run", "-x=value"}, NoMatch},
		{permissive, []string{"run", "-x=value"}, ShortAssignment},
		{strict, []string{"run", "-nvalue"}, JoinedShortOption},
		{strict, []string{"run", "-it"}, JoinedShortOption},
		{strict, []string{"run", "-xt"}, NoMatch},
		{permissive, []string{"run", "-xt"}, JoinedShortOption},
		{permissive, []string{"run", "-10"}, NoMatch},
		{strict, []string{"run", "--confirm=false"}, SwitchAssignment},
		{strict, []string{"run", "-i=false"}, SwitchAssignment},
		{strict, []string{"run", "--items=first", "second"}, MultiValueAssignment},
		{strict, []string{"run", "--labels=value"}, MultiValueAssignment},
		{strict, []string{"run", "--name"}, MissingOptionValue},
		{strict, []string{"run", "--labels"}, MissingOptionValue},
		{strict, []string{"run", "--labels", "--confirm"}, MissingOptionValue},
		{strict, []string{"run", "--name="}, InvalidOptionValue},
		{strict, []string{"run", "--name=-value"}, InvalidOptionValue},
		{strict, []string{"run", "--name", "-value"}, InvalidOptionValue},
		{strict, []string{"run", "--name", ""}, InvalidOptionValue},
		{permissive, []string{"run", "--labels", ""}, InvalidOptionValue},
		{permissive, []string{"run", "--labels", "label", ""}, Matched},
		{strict, []string{"run", "-"}, StandaloneDash},
		{strict, []string{"run", "--"}, UnexpectedSeparator},
		{tail, []string{"run"}, MissingSeparator},
		{tail, []string{"run", "--", "-it"}, NoMatch},
		{tail, []string{"run", "--", "-"}, Matched},
	} {
		if got := tc.rule.Check(tc.args); got != tc.want {
			t.Errorf("Check(%q) = %v, want %v", tc.args, got, tc.want)
		}
	}
}

func TestCompileNamedErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		spec NamedSpec
	}{
		{"empty prefix", NamedSpec{}},
		{"prefix glob", NamedSpec{Prefix: []string{"["}}},
		{"prefix quantifier", NamedSpec{Prefix: []string{"run:+"}}},
		{"prefix option", NamedSpec{Prefix: []string{"--run"}}},
		{"positional glob", NamedSpec{Prefix: []string{"run"}, Positionals: []string{"["}}},
		{"tail glob", NamedSpec{Prefix: []string{"run"}, AfterDoubleDash: []string{"["}}},
		{"missing names", NamedSpec{Prefix: []string{"run"}, Options: []OptionSpec{{}}}},
		{"bad name", NamedSpec{Prefix: []string{"run"}, Options: []OptionSpec{{Names: []string{"--*"}}}}},
		{"bare name", NamedSpec{Prefix: []string{"run"}, Options: []OptionSpec{{Names: []string{"name"}}}}},
		{"reserved separator", NamedSpec{Prefix: []string{"run"}, Options: []OptionSpec{{Names: []string{"--"}}}}},
		{"cluster name", NamedSpec{Prefix: []string{"run"}, Options: []OptionSpec{{Names: []string{"-abc"}}}}},
		{"duplicate names", NamedSpec{Prefix: []string{"run"}, Options: []OptionSpec{{Names: []string{"-n", "-n"}}}}},
		{"overlapping names", NamedSpec{Prefix: []string{"run"}, Options: []OptionSpec{{Names: []string{"-n"}}, {Names: []string{"-n"}}}}},
		{"bad occurrence", NamedSpec{Prefix: []string{"run"}, Options: []OptionSpec{{Names: []string{"-n"}, Occurs: "3-1"}}}},
		{"bad values", NamedSpec{Prefix: []string{"run"}, Options: []OptionSpec{{Names: []string{"-n"}, Values: []string{"["}}}}},
		{"optional values", NamedSpec{Prefix: []string{"run"}, Options: []OptionSpec{{Names: []string{"-n"}, Values: []string{"*:?"}}}}},
		{"mixed quantified values", NamedSpec{Prefix: []string{"run"}, Options: []OptionSpec{{Names: []string{"-n"}, Values: []string{"*:+", "*"}}}}},
		{"variable with positionals", NamedSpec{Prefix: []string{"run"}, Positionals: []string{"*"}, Options: []OptionSpec{{Names: []string{"-n"}, Values: []string{"*:+"}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := CompileNamed(tc.spec); err == nil {
				t.Fatal("expected configuration error")
			}
		})
	}
}

func TestPositionalMinimumChecksEveryValue(t *testing.T) {
	for _, pattern := range []string{"good:2", "good:2+", "good:2-3"} {
		segments, err := ParseSegments([]string{pattern})
		if err != nil {
			t.Fatal(err)
		}
		if Match(segments, []string{"bad", "good"}) {
			t.Errorf("%s skipped its first value", pattern)
		}
		if !Match(segments, []string{"good", "good"}) {
			t.Errorf("%s rejected matching values", pattern)
		}
	}
}

func FuzzNamedMatch(f *testing.F) {
	for _, seed := range []string{
		"exec\x00worker-one\x00--config\x00safe.conf\x00--\x00id",
		"exec\x00worker-one\x00--\x00sh\x00--config\x00safe.conf",
		"exec\x00--config=safe.conf\x00worker-one\x00--\x00--",
		"exec\x00--config\x00--\x00safe.conf",
	} {
		f.Add(seed)
	}
	rule, err := CompileNamed(NamedSpec{
		Prefix: []string{"exec"}, Positionals: []string{"worker-*"},
		Options:         []OptionSpec{{Names: []string{"--config", "-c"}, Values: []string{"safe.conf"}}},
		AfterDoubleDash: []string{"*:+"},
	})
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, input string) {
		args := strings.Split(input, "\x00")
		if !rule.Match(args) {
			return
		}
		separator := slices.Index(args, "--")
		if len(args) < 5 || args[0] != "exec" || separator < 1 || separator == len(args)-1 {
			t.Fatalf("accepted malformed argv %q", args)
		}
		// Check the accepted option/positional partition independently of Match.
		var positions []string
		count := 0
		for i := 1; i < separator; i++ {
			switch args[i] {
			case "--config", "-c":
				count++
				i++
				if i >= separator || args[i] != "safe.conf" {
					t.Fatalf("accepted missing or bad value %q", args)
				}
			case "--config=safe.conf":
				count++
			default:
				positions = append(positions, args[i])
			}
		}
		if count != 1 || len(positions) != 1 || !strings.HasPrefix(positions[0], "worker-") {
			t.Fatalf("accepted ambiguous argv %q", args)
		}
		// Once the separator is found, container options cannot change the result.
		mutated := slices.Concat(args[:separator+1], []string{"--config", "wrong", "--"})
		if !rule.Match(mutated) {
			t.Fatal("container argv changed the outer match")
		}
	})
}
