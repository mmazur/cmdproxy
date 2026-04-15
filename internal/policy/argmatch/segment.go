package argmatch

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gobwas/glob"
)

// Quantifier defines how many args a segment can consume.
type Quantifier struct {
	Min int // minimum number of args to match
	Max int // maximum; -1 means unbounded
}

// Segment is a single parsed element from a positional pattern list.
type Segment struct {
	GlobPattern string     // the glob pattern (with escaped colons resolved)
	Quantifier  Quantifier // how many args this segment can consume
	compiled    glob.Glob  // pre-compiled glob
}

// Match tests whether a single argument matches this segment's glob.
// Matching is case-insensitive.
func (s *Segment) Match(arg string) bool {
	return s.compiled.Match(strings.ToLower(arg))
}

// ParseSegment parses a raw segment string like "pattern:quantifier".
//
// Rules:
//   - Literal colons in the glob must be escaped as \:
//   - At most one unescaped colon is allowed (syntax error otherwise)
//   - If no unescaped colon: entire string is a glob, quantifier is {1,1}
//   - If one unescaped colon: left is glob, right is quantifier
//   - Empty glob is a syntax error
//   - Quantifier must be one of: * + ? N N+ N-M
func ParseSegment(raw string) (Segment, error) {
	globPart, quantStr, hasColon, err := splitSegment(raw)
	if err != nil {
		return Segment{}, err
	}

	// Unescape \: → : in the glob part
	globPart = strings.ReplaceAll(globPart, `\:`, `:`)

	if globPart == "" {
		return Segment{}, fmt.Errorf("empty glob pattern in segment %q", raw)
	}

	if hasColon && quantStr == "" {
		return Segment{}, fmt.Errorf("empty quantifier after colon in segment %q", raw)
	}

	q, err := parseQuantifier(quantStr)
	if err != nil {
		return Segment{}, fmt.Errorf("in segment %q: %w", raw, err)
	}

	compiled, err := glob.Compile(strings.ToLower(globPart))
	if err != nil {
		return Segment{}, fmt.Errorf("in segment %q: bad glob %q: %w", raw, globPart, err)
	}

	return Segment{
		GlobPattern: globPart,
		Quantifier:  q,
		compiled:    compiled,
	}, nil
}

// ParseSegments parses a full positional pattern list.
func ParseSegments(patterns []string) ([]Segment, error) {
	segments := make([]Segment, 0, len(patterns))
	for i, p := range patterns {
		seg, err := ParseSegment(p)
		if err != nil {
			return nil, fmt.Errorf("segment [%d]: %w", i, err)
		}
		segments = append(segments, seg)
	}
	return segments, nil
}

// splitSegment finds the unescaped colon in raw and splits into glob and quantifier parts.
// Returns glob, quantifier string, whether a colon was found, and error.
func splitSegment(raw string) (glob string, quant string, hasColon bool, err error) {
	colonIdx := -1
	colonCount := 0
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\\' && i+1 < len(raw) {
			i++ // skip escaped char
			continue
		}
		if raw[i] == ':' {
			colonCount++
			if colonCount > 1 {
				return "", "", false, fmt.Errorf("multiple unescaped colons in segment %q", raw)
			}
			colonIdx = i
		}
	}

	if colonIdx == -1 {
		return raw, "", false, nil
	}
	return raw[:colonIdx], raw[colonIdx+1:], true, nil
}

// parseQuantifier parses a quantifier string.
//
// Accepts:
//
//	""   → {1, 1} (default, no colon present)
//	"*"  → {0, -1}
//	"+"  → {1, -1}
//	"?"  → {0, 1}
//	"N"  → {N, N}
//	"N+" → {N, -1}
//	"N-M"→ {N, M}
func parseQuantifier(s string) (Quantifier, error) {
	if s == "" {
		return Quantifier{Min: 1, Max: 1}, nil
	}

	switch s {
	case "*":
		return Quantifier{Min: 0, Max: -1}, nil
	case "+":
		return Quantifier{Min: 1, Max: -1}, nil
	case "?":
		return Quantifier{Min: 0, Max: 1}, nil
	}

	// N+ form
	if strings.HasSuffix(s, "+") {
		n, err := strconv.Atoi(s[:len(s)-1])
		if err != nil || n < 0 {
			return Quantifier{}, fmt.Errorf("invalid quantifier %q", s)
		}
		return Quantifier{Min: n, Max: -1}, nil
	}

	// N-M form
	if idx := strings.Index(s, "-"); idx > 0 {
		nStr, mStr := s[:idx], s[idx+1:]
		n, err := strconv.Atoi(nStr)
		if err != nil || n < 0 {
			return Quantifier{}, fmt.Errorf("invalid quantifier %q", s)
		}
		m, err := strconv.Atoi(mStr)
		if err != nil || m < 0 {
			return Quantifier{}, fmt.Errorf("invalid quantifier %q", s)
		}
		if n > m {
			return Quantifier{}, fmt.Errorf("invalid quantifier %q: min %d > max %d", s, n, m)
		}
		return Quantifier{Min: n, Max: m}, nil
	}

	// N form
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return Quantifier{}, fmt.Errorf("invalid quantifier %q", s)
	}
	return Quantifier{Min: n, Max: n}, nil
}
