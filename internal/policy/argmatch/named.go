package argmatch

import (
	"fmt"
	"strings"
)

// OptionSpec declares literal aliases, value segments, and an occurrence limit.
// An empty Occurs defaults to exactly one occurrence, across all aliases.
type OptionSpec struct {
	Names  []string
	Values []string
	Occurs string
}

// NamedSpec describes an ordered prefix followed by interspersed options and
// positionals. A nil AfterDoubleDash forbids --; a non-nil list requires it.
// AllowExtraArgs permits undeclared options and, when Positionals is nil,
// unmatched bare arguments. It does not infer undeclared options' value arities.
type NamedSpec struct {
	Prefix          []string
	Options         []OptionSpec
	Positionals     []string
	AfterDoubleDash []string
	AllowExtraArgs  bool
}

type namedOption struct {
	values []Segment
	occurs Quantifier
	// -1 means a variable-length run; otherwise the exact value count.
	arity int
}

// NamedRule is immutable after compilation and safe for concurrent matching.
type NamedRule struct {
	prefix           []Segment
	options          []namedOption
	names            map[string]int
	positionals      []Segment
	tail             []Segment
	hasTail          bool
	allowExtraArgs   bool
	checkPositionals bool
}

// CompileNamed validates the option declarations and compiles their patterns.
// Unlike legacy positional rules, all structured-rule matching is case-sensitive.
func CompileNamed(spec NamedSpec) (*NamedRule, error) {
	if len(spec.Prefix) == 0 {
		return nil, fmt.Errorf("prefix must not be empty")
	}
	r := &NamedRule{
		names:            make(map[string]int),
		hasTail:          spec.AfterDoubleDash != nil,
		allowExtraArgs:   spec.AllowExtraArgs,
		checkPositionals: !spec.AllowExtraArgs || spec.Positionals != nil,
	}
	var err error
	r.prefix, err = ParseCaseSensitiveSegments(spec.Prefix)
	if err != nil {
		return nil, fmt.Errorf("prefix: %w", err)
	}
	for i, pattern := range spec.Prefix {
		_, _, quantified, _ := splitSegment(pattern)
		if quantified || strings.HasPrefix(pattern, "-") {
			return nil, fmt.Errorf("prefix[%d]: expected an unquantified non-option glob", i)
		}
	}
	r.positionals, err = ParseCaseSensitiveSegments(spec.Positionals)
	if err != nil {
		return nil, fmt.Errorf("positionals: %w", err)
	}
	r.tail, err = ParseCaseSensitiveSegments(spec.AfterDoubleDash)
	if err != nil {
		return nil, fmt.Errorf("after_double_dash: %w", err)
	}
	for i, opt := range spec.Options {
		if err := r.addOption(i, opt); err != nil {
			return nil, fmt.Errorf("options[%d]: %w", i, err)
		}
	}
	return r, nil
}

func validOptionName(name string) bool {
	if strings.HasPrefix(name, "--") {
		name = name[2:]
	} else if len(name) == 2 && name[0] == '-' {
		name = name[1:]
	} else {
		return false
	}
	if name == "" || name[0] == '-' || name[0] == '_' {
		return false
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func (r *NamedRule) addOption(index int, spec OptionSpec) error {
	if len(spec.Names) == 0 {
		return fmt.Errorf("names must not be empty")
	}
	for _, name := range spec.Names {
		if !validOptionName(name) {
			return fmt.Errorf("invalid literal option name %q", name)
		}
		if _, exists := r.names[name]; exists {
			return fmt.Errorf("duplicate option alias %q", name)
		}
		r.names[name] = index
	}
	if spec.Occurs == "" {
		spec.Occurs = "1"
	}
	q, err := parseQuantifier(spec.Occurs)
	if err != nil {
		return fmt.Errorf("occurs: %w", err)
	}
	values, err := ParseCaseSensitiveSegments(spec.Values)
	if err != nil {
		return fmt.Errorf("values: %w", err)
	}
	opt := namedOption{values: values, occurs: q, arity: len(values)}
	for _, value := range values {
		count := value.Quantifier
		if count.Min < 1 || len(values) > 1 && (count.Min != 1 || count.Max != 1) {
			return fmt.Errorf("values must be exact-one segments or one repeated segment with minimum at least one")
		}
		if len(values) == 1 {
			opt.arity = count.Min
			if count.Min != count.Max {
				opt.arity = -1
				if len(r.positionals) != 0 {
					return fmt.Errorf("variable value count cannot be combined with positionals")
				}
			}
		}
	}
	r.options = append(r.options, opt)
	return nil
}

func optionValue(value string) bool {
	return value != "" && !strings.HasPrefix(value, "-")
}

// MatchResult describes a match, a syntax rejection, or another mismatch.
type MatchResult int

const (
	// NoMatch is a mismatch after the prefix matched, without a syntax diagnosis.
	NoMatch MatchResult = iota
	// Unrelated indicates that the rule did not recognize the command prefix.
	Unrelated
	Matched
	ShortAssignment
	JoinedShortOption
	SwitchAssignment
	MultiValueAssignment
	MissingOptionValue
	InvalidOptionValue
	StandaloneDash
	UnexpectedSeparator
	MissingSeparator
)

// Match reports whether args satisfy the named rule.
func (r *NamedRule) Match(args []string) bool {
	return r.Check(args) == Matched
}

// Check never searches inside declared option values or the -- tail for a
// required option, nor backtracks over option boundaries. In permissive mode,
// undeclared tokens pass through without interpreting their value arities.
func (r *NamedRule) Check(args []string) MatchResult {
	if len(args) < len(r.prefix) {
		return Unrelated
	}
	for i := range r.prefix {
		if strings.HasPrefix(args[i], "-") || !r.prefix[i].Match(args[i]) {
			return Unrelated
		}
	}
	args = args[len(r.prefix):]
	counts := make([]int, len(r.options))
	var positionals []string
	separator := false
	for i := 0; i < len(args); {
		token := args[i]
		if token == "--" {
			if !r.hasTail {
				return UnexpectedSeparator
			}
			if !Match(r.tail, args[i+1:]) {
				return NoMatch
			}
			separator = true
			break
		}
		if !strings.HasPrefix(token, "-") {
			if r.checkPositionals {
				positionals = append(positionals, token)
			}
			i++
			continue
		}
		name, assigned, hasAssignment := strings.Cut(token, "=")
		if token == "-" {
			return StandaloneDash
		}
		if len(name) > 2 && !strings.HasPrefix(name, "--") && validOptionName(name[:2]) {
			_, declared := r.names[name[:2]]
			if (declared || r.allowExtraArgs) && strings.Trim(name[1:], "0123456789") != "" {
				return JoinedShortOption
			}
			return NoMatch
		}
		// Short assignments are ambiguous: getopt-style parsers treat -n=X
		// as a joined value of "=X", rather than the long-option value "X".
		if hasAssignment && !strings.HasPrefix(name, "--") {
			if index, declared := r.names[name]; declared {
				if r.options[index].arity == 0 {
					return SwitchAssignment
				}
				return ShortAssignment
			}
			if r.allowExtraArgs && validOptionName(name) {
				return ShortAssignment
			}
			return NoMatch
		}
		index, exists := r.names[name]
		if !exists {
			if r.allowExtraArgs && validOptionName(name) {
				i++
				continue
			}
			return NoMatch
		}
		opt := &r.options[index]
		counts[index]++
		if opt.occurs.Max != -1 && counts[index] > opt.occurs.Max {
			return NoMatch
		}
		i++
		if hasAssignment {
			switch opt.arity {
			case 0:
				return SwitchAssignment
			case 1:
				if !optionValue(assigned) {
					return InvalidOptionValue
				}
			default:
				return MultiValueAssignment
			}
			if !Match(opt.values, []string{assigned}) {
				return NoMatch
			}
			continue
		}
		start := i
		if opt.arity == -1 {
			for i < len(args) && optionValue(args[i]) {
				i++
			}
			if i-start < opt.values[0].Quantifier.Min {
				if i < len(args) && args[i] == "" {
					return InvalidOptionValue
				}
				return MissingOptionValue
			}
		} else {
			if opt.arity > len(args)-i {
				return MissingOptionValue
			}
			for end := i + opt.arity; i < end; i++ {
				if !optionValue(args[i]) {
					return InvalidOptionValue
				}
			}
		}
		if !Match(opt.values, args[start:i]) {
			return NoMatch
		}
	}
	if r.checkPositionals && !Match(r.positionals, positionals) {
		return NoMatch
	}
	for i, opt := range r.options {
		if counts[i] < opt.occurs.Min {
			return NoMatch
		}
	}
	if r.hasTail && !separator {
		return MissingSeparator
	}
	return Matched
}
