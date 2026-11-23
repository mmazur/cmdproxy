package policy

import (
	"fmt"

	"github.com/mmazur/cmdproxy/internal/config"
	"github.com/mmazur/cmdproxy/internal/policy/argmatch"
)

type Verdict int

const (
	Allow Verdict = iota
	Deny
)

func (v Verdict) String() string {
	if v == Allow {
		return "allow"
	}
	return "deny"
}

type Decision struct {
	Verdict Verdict
	Reason  string
	Hint    string
}

func Evaluate(cfg config.ServerConfig, cmd string, args []string) Decision {
	cc, ok := cfg.Commands[cmd]
	if !ok {
		return Decision{Verdict: Deny, Reason: fmt.Sprintf("command %q not in allowlist", cmd)}
	}

	for _, rule := range cc.Deny {
		if config.MatchRule(rule, args) {
			reason := "args matched deny pattern"
			if rule.LegacyGlob != "" {
				reason = fmt.Sprintf("args matched deny pattern %q", rule.LegacyGlob)
			} else if rule.Named != nil {
				reason = "args matched named-option deny rule"
			}
			return Decision{Verdict: Deny, Reason: reason}
		}
	}

	mismatch := argmatch.Unrelated
	conflictingMismatch := false
	for _, rule := range cc.Allow {
		switch result := config.MatchRuleResult(rule, args); result {
		case argmatch.Matched:
			reason := "args matched positional allow rule"
			if rule.Named != nil {
				reason = "args matched named-option allow rule"
			} else if rule.LegacyGlob != "" {
				reason = fmt.Sprintf("args matched allow pattern %q", rule.LegacyGlob)
			}
			return Decision{Verdict: Allow, Reason: reason}
		case argmatch.Unrelated:
			continue
		default:
			if mismatch != argmatch.Unrelated && mismatch != result {
				conflictingMismatch = true
			}
			mismatch = result
		}
	}

	if conflictingMismatch {
		mismatch = argmatch.NoMatch
	}
	return denyMismatch(mismatch)
}

func denyMismatch(mismatch argmatch.MatchResult) Decision {
	reason := "no allow pattern matched"
	var hint string
	switch mismatch {
	case argmatch.ShortAssignment:
		reason = "named-option rules reject short options written with '='"
		hint = "Pass the short option and its values as separate arguments, e.g. -p VALUE."
	case argmatch.JoinedShortOption:
		reason = "named-option rules reject joined short values and clustered short switches"
		hint = "Use separate arguments, e.g. -p VALUE for an option with a value or -i -t for two switches."
	case argmatch.SwitchAssignment:
		reason = "a switch in a named-option rule takes no value"
		hint = "If you meant to enable the switch, pass it by itself, e.g. --confirm or -i."
	case argmatch.MultiValueAssignment:
		reason = "an option that takes multiple values in a named-option rule rejects '=' assignments"
		hint = "Pass the option and each value as separate arguments, e.g. --items FIRST SECOND."
	case argmatch.MissingOptionValue:
		reason = "an option in a named-option rule is missing one or more values"
		hint = "Put the required values immediately after the option."
	case argmatch.InvalidOptionValue:
		reason = "named-option rules require nonempty option values that do not start with '-'"
		hint = "Supply a nonempty value immediately after the option. If the command needs a dash-leading value, ask the policy owner about a rule that supports it."
	case argmatch.StandaloneDash:
		reason = "named-option rules reject a standalone '-' in the options and positionals"
		hint = "If the command requires a literal '-', ask the policy owner whether it can be allowed for this invocation."
	case argmatch.UnexpectedSeparator:
		reason = "the named-option rule does not accept a '--' separator"
		hint = "If the command requires '--', ask the policy owner whether the argument tail can be allowed."
	case argmatch.MissingSeparator:
		reason = "the named-option rule requires a '--' separator"
		hint = "Check the command syntax for the arguments that belong after '--'."
	}
	return Decision{Verdict: Deny, Reason: reason, Hint: hint}
}
