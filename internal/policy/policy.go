package policy

import (
	"fmt"
	"strings"

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
}

func Evaluate(cfg config.ServerConfig, cmd string, args []string) Decision {
	cc, ok := cfg.Commands[cmd]
	if !ok {
		return Decision{Deny, fmt.Sprintf("command %q not in allowlist", cmd)}
	}

	argsStr := strings.ToLower(strings.Join(args, " "))

	for _, rule := range cc.Deny {
		if matchRule(rule, argsStr, args) {
			reason := "args matched deny pattern"
			if rule.LegacyGlob != "" {
				reason = fmt.Sprintf("args matched deny pattern %q", rule.LegacyGlob)
			}
			return Decision{Deny, reason}
		}
	}

	for _, rule := range cc.Allow {
		if matchRule(rule, argsStr, args) {
			reason := "args matched positional allow rule"
			if rule.LegacyGlob != "" {
				reason = fmt.Sprintf("args matched allow pattern %q", rule.LegacyGlob)
			}
			return Decision{Allow, reason}
		}
	}

	return Decision{Deny, "no allow pattern matched"}
}

func matchRule(rule config.Rule, argsStr string, args []string) bool {
	if rule.LegacyGlob != "" {
		return rule.Compiled.Match(argsStr)
	}
	return argmatch.Match(rule.Segments, args)
}
