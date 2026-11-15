package policy

import (
	"fmt"

	"github.com/mmazur/cmdproxy/internal/config"
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

	for _, rule := range cc.Deny {
		if config.MatchRule(rule, args) {
			reason := "args matched deny pattern"
			if rule.LegacyGlob != "" {
				reason = fmt.Sprintf("args matched deny pattern %q", rule.LegacyGlob)
			} else if rule.Named != nil {
				reason = "args matched named-option deny rule"
			}
			return Decision{Deny, reason}
		}
	}

	for _, rule := range cc.Allow {
		if config.MatchRule(rule, args) {
			reason := "args matched positional allow rule"
			if rule.LegacyGlob != "" {
				reason = fmt.Sprintf("args matched allow pattern %q", rule.LegacyGlob)
			} else if rule.Named != nil {
				reason = "args matched named-option allow rule"
			}
			return Decision{Allow, reason}
		}
	}

	return Decision{Deny, "no allow pattern matched"}
}
