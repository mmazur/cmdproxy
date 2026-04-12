package policy

import (
	"fmt"
	"strings"

	"github.com/gobwas/glob"

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

	argsStr := strings.Join(args, " ")

	for _, pattern := range cc.Deny {
		g, err := glob.Compile(pattern)
		if err != nil {
			return Decision{Deny, fmt.Sprintf("bad deny glob %q: %v", pattern, err)}
		}
		if g.Match(argsStr) {
			return Decision{Deny, fmt.Sprintf("args matched deny pattern %q", pattern)}
		}
	}

	for _, pattern := range cc.Allow {
		g, err := glob.Compile(pattern)
		if err != nil {
			return Decision{Deny, fmt.Sprintf("bad allow glob %q: %v", pattern, err)}
		}
		if g.Match(argsStr) {
			return Decision{Allow, fmt.Sprintf("args matched allow pattern %q", pattern)}
		}
	}

	return Decision{Deny, "no allow pattern matched"}
}
