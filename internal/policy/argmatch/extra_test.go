package argmatch

import (
	"slices"
	"strings"
	"testing"
)

func TestNamedExtraArgs(t *testing.T) {
	spec := NamedSpec{Prefix: []string{"group", "remove"}, Options: []OptionSpec{
		{Names: []string{"--workspace", "-w"}, Values: []string{"sandbox"}},
		{Names: []string{"--name", "-n"}, Values: []string{"scratch-*"}},
		{Names: []string{"--endpoint"}, Values: []string{"*"}, Occurs: "0"},
	}}
	strict := compileNamedTest(t, spec)
	spec.AllowExtraArgs = true
	permissive := compileNamedTest(t, spec)
	for _, args := range []string{
		"group remove --workspace sandbox -n scratch-one --confirm",
		"group remove --region test --workspace sandbox --quiet -n scratch-one --format=json",
		"group remove --name=scratch-one extra --workspace=sandbox -q",
		// Undeclared option arities are deliberately not interpreted. This
		// matches even if the actual program treats --name as description's value.
		"group remove --description --name scratch-one --workspace sandbox",
	} {
		argv := strings.Fields(args)
		if strict.Match(argv) || !permissive.Match(argv) {
			t.Errorf("%q: want strict rejection and permissive acceptance", args)
		}
	}
	for _, args := range []string{
		"group remove --confirm --workspace sandbox", // missing name
		"group remove --workspace other -n scratch-one --confirm",
		"group remove --workspace sandbox -n unrelated --confirm",
		"group remove --workspace sandbox -n scratch-one --name=scratch-one --confirm",
		"group remove --workspace sandbox -n scratch-one -w sandbox --confirm",
		"group remove --workspace sandbox -n scratch-one --endpoint=other --confirm",
		"group remove --workspace sandbox --name --confirm",
		"group remove --workspace sandbox --name= --confirm",
		"group remove --workspace sandbox --name=--other --confirm",
		"group remove --workspace sandbox -n=scratch-one --confirm",
		"group remove --workspace sandbox -n scratch-one -nscratch-two",
		"group remove --workspace sandbox -n scratch-one -it",
		"group remove --workspace sandbox -n scratch-one -q=true",
		"group remove --workspace sandbox -n scratch-one -",
		"group remove --workspace sandbox -n scratch-one -- --confirm",
		"--confirm group remove --workspace sandbox -n scratch-one",
		"other group remove --workspace sandbox -n scratch-one",
	} {
		if permissive.Match(strings.Fields(args)) {
			t.Errorf("permissive mode bypassed a constraint: %q", args)
		}
	}
	if permissive.Match([]string{"group", "remove", "--workspace", "sandbox", "--description=--name scratch-one"}) {
		t.Fatal("an undeclared value was split into selector tokens")
	}
}

func TestNamedExtraArgsPositionals(t *testing.T) {
	for _, positions := range [][]string{nil, {}, {"worker-*"}} {
		rule := compileNamedTest(t, NamedSpec{
			Prefix: []string{"exec"}, AllowExtraArgs: true, Positionals: positions,
			Options: []OptionSpec{{Names: []string{"--config"}, Values: []string{"safe.conf"}}},
		})
		for _, tc := range []struct {
			args string
			want bool
		}{
			{"exec --config safe.conf --quiet", len(positions) == 0},
			{"exec worker-one --quiet --config safe.conf", positions == nil || len(positions) == 1},
			// The value of an unknown option is a leftover bare argument.
			{"exec worker-one --region test --config safe.conf", positions == nil},
		} {
			if got := rule.Match(strings.Fields(tc.args)); got != tc.want {
				t.Errorf("positionals=%#v, %q: got %v, want %v", positions, tc.args, got, tc.want)
			}
		}
	}
}

func TestNamedExtraArgsSeparatorAndValues(t *testing.T) {
	rule := compileNamedTest(t, NamedSpec{
		Prefix: []string{"exec"}, AllowExtraArgs: true,
		Options:         []OptionSpec{{Names: []string{"--config"}, Values: []string{"safe.conf"}}},
		AfterDoubleDash: []string{"echo", "hello"},
	})
	for _, tc := range []struct {
		args string
		want bool
	}{
		{"exec worker-one --quiet --config=safe.conf -- echo hello", true},
		{"exec worker-one --quiet -- echo hello --config=safe.conf", false},
		{"exec worker-one --config safe.conf -- echo hello extra", false},
		{"exec worker-one --config safe.conf --", false},
		{"exec worker-one --config safe.conf", false},
		{"exec worker-one --config -- echo hello", false},
	} {
		if got := rule.Match(strings.Fields(tc.args)); got != tc.want {
			t.Errorf("%q: got %v, want %v", tc.args, got, tc.want)
		}
	}
	batch := compileNamedTest(t, NamedSpec{
		Prefix: []string{"batch"}, AllowExtraArgs: true,
		Options: []OptionSpec{{Names: []string{"--items"}, Values: []string{"scratch-*:+"}}},
	})
	if !batch.Match(strings.Fields("batch --quiet --items scratch-one scratch-two --confirm")) {
		t.Fatal("extra options prevented a valid batch")
	}
	if batch.Match(strings.Fields("batch --items scratch-one unrelated --confirm")) {
		t.Fatal("invalid batch value escaped as an extra argument")
	}
	repeated := compileNamedTest(t, NamedSpec{
		Prefix: []string{"run"}, AllowExtraArgs: true,
		Options: []OptionSpec{{Names: []string{"--label"}, Values: []string{"scratch-*"}, Occurs: "+"}},
	})
	if repeated.Match(strings.Fields("run --label unrelated --quiet --label scratch-one")) {
		t.Fatal("a later occurrence hid an invalid value")
	}
}

func FuzzNamedExtraArgs(f *testing.F) {
	for _, seed := range []string{
		"exec\x00--quiet\x00--config=safe.conf\x00worker-one\x00--\x00id",
		"exec\x00--other\x00--config\x00safe.conf\x00--\x00id",
		"exec\x00--\x00id\x00--config\x00safe.conf",
	} {
		f.Add(seed)
	}
	rule, err := CompileNamed(NamedSpec{
		Prefix: []string{"exec"}, AllowExtraArgs: true,
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
		if args[0] != "exec" || separator < 1 || separator == len(args)-1 {
			t.Fatalf("accepted malformed prefix/tail: %q", args)
		}
		count := 0
		for i := 1; i < separator; i++ {
			switch args[i] {
			case "--config", "-c":
				count++
				i++
				if i >= separator || args[i] != "safe.conf" {
					t.Fatalf("accepted invalid config: %q", args)
				}
			case "--config=safe.conf":
				count++
			}
		}
		if count != 1 {
			t.Fatalf("accepted missing or duplicate config: %q", args)
		}
		if !rule.Match(slices.Concat(args[:separator+1], []string{"--config", "wrong"})) {
			t.Fatal("container argv affected outer matching")
		}
	})
}
