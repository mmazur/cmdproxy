package argmatch

import (
	"testing"
)

func TestParseSegment_ValidPatterns(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantGlob string
		wantMin int
		wantMax int
	}{
		// No quantifier (implicit :1)
		{name: "literal", raw: "foo", wantGlob: "foo", wantMin: 1, wantMax: 1},
		{name: "glob star", raw: "*", wantGlob: "*", wantMin: 1, wantMax: 1},
		{name: "glob prefix", raw: "list*", wantGlob: "list*", wantMin: 1, wantMax: 1},
		{name: "glob char class", raw: "[a-z]*", wantGlob: "[a-z]*", wantMin: 1, wantMax: 1},
		{name: "glob question mark", raw: "fo?", wantGlob: "fo?", wantMin: 1, wantMax: 1},
		{name: "double dash flag", raw: "--help", wantGlob: "--help", wantMin: 1, wantMax: 1},

		// Shorthand quantifiers
		{name: "star quantifier", raw: "*:*", wantGlob: "*", wantMin: 0, wantMax: -1},
		{name: "plus quantifier", raw: "*:+", wantGlob: "*", wantMin: 1, wantMax: -1},
		{name: "question quantifier", raw: "*:?", wantGlob: "*", wantMin: 0, wantMax: 1},
		{name: "glob with star quant", raw: "[a-z]*:*", wantGlob: "[a-z]*", wantMin: 0, wantMax: -1},
		{name: "glob with plus quant", raw: "[a-z]*:+", wantGlob: "[a-z]*", wantMin: 1, wantMax: -1},
		{name: "glob with question quant", raw: "--verbose:?", wantGlob: "--verbose", wantMin: 0, wantMax: 1},

		// Numeric quantifiers
		{name: "exact 1", raw: "foo:1", wantGlob: "foo", wantMin: 1, wantMax: 1},
		{name: "exact 3", raw: "foo:3", wantGlob: "foo", wantMin: 3, wantMax: 3},
		{name: "exact 0", raw: "foo:0", wantGlob: "foo", wantMin: 0, wantMax: 0},

		// N+ quantifiers
		{name: "0+", raw: "foo:0+", wantGlob: "foo", wantMin: 0, wantMax: -1},
		{name: "1+", raw: "foo:1+", wantGlob: "foo", wantMin: 1, wantMax: -1},
		{name: "2+", raw: "*:2+", wantGlob: "*", wantMin: 2, wantMax: -1},
		{name: "10+", raw: "x:10+", wantGlob: "x", wantMin: 10, wantMax: -1},

		// N-M quantifiers
		{name: "0-1", raw: "foo:0-1", wantGlob: "foo", wantMin: 0, wantMax: 1},
		{name: "2-5", raw: "foo:2-5", wantGlob: "foo", wantMin: 2, wantMax: 5},
		{name: "1-1", raw: "foo:1-1", wantGlob: "foo", wantMin: 1, wantMax: 1},
		{name: "0-0", raw: "foo:0-0", wantGlob: "foo", wantMin: 0, wantMax: 0},
		{name: "3-10", raw: "*:3-10", wantGlob: "*", wantMin: 3, wantMax: 10},

		// Escaped colons in glob
		{name: "escaped colon no quant", raw: `http\://host`, wantGlob: "http://host", wantMin: 1, wantMax: 1},
		{name: "escaped colon with quant", raw: `http\://host:+`, wantGlob: "http://host", wantMin: 1, wantMax: -1},
		{name: "multiple escaped colons", raw: `a\:b\:c`, wantGlob: "a:b:c", wantMin: 1, wantMax: 1},
		{name: "escaped colon at end no quant", raw: `foo\:`, wantGlob: "foo:", wantMin: 1, wantMax: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seg, err := ParseSegment(tt.raw)
			if err != nil {
				t.Fatalf("ParseSegment(%q) error: %v", tt.raw, err)
			}
			if seg.GlobPattern != tt.wantGlob {
				t.Errorf("GlobPattern = %q, want %q", seg.GlobPattern, tt.wantGlob)
			}
			if seg.Quantifier.Min != tt.wantMin {
				t.Errorf("Quantifier.Min = %d, want %d", seg.Quantifier.Min, tt.wantMin)
			}
			if seg.Quantifier.Max != tt.wantMax {
				t.Errorf("Quantifier.Max = %d, want %d", seg.Quantifier.Max, tt.wantMax)
			}
		})
	}
}

func TestParseSegment_InvalidPatterns(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "empty glob", raw: ":*"},
		{name: "empty glob plus", raw: ":+"},
		{name: "empty glob number", raw: ":3"},
		{name: "invalid quantifier word", raw: "foo:bar"},
		{name: "invalid quantifier symbol", raw: "foo:!"},
		{name: "invalid quantifier empty after colon", raw: "foo:"},
		{name: "multiple unescaped colons", raw: "a:b:c"},
		{name: "three colons", raw: "a:1:2"},
		{name: "negative number", raw: "foo:-1"},
		{name: "range min > max", raw: "foo:5-3"},
		{name: "range negative min", raw: "foo:-1-3"},
		{name: "range with word", raw: "foo:a-b"},
		{name: "N+ negative", raw: "foo:-1+"},
		{name: "N+ with word", raw: "foo:abc+"},
		{name: "bad glob pattern", raw: "[invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseSegment(tt.raw)
			if err == nil {
				t.Errorf("ParseSegment(%q) expected error, got nil", tt.raw)
			}
		})
	}
}

func TestParseSegment_GlobMatching(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		arg      string
		wantMatch bool
	}{
		{name: "literal match", raw: "foo", arg: "foo", wantMatch: true},
		{name: "literal no match", raw: "foo", arg: "bar", wantMatch: false},
		{name: "star matches anything", raw: "*", arg: "anything", wantMatch: true},
		{name: "star matches empty", raw: "*", arg: "", wantMatch: true},
		{name: "prefix glob match", raw: "list*", arg: "list-all", wantMatch: true},
		{name: "prefix glob exact", raw: "list*", arg: "list", wantMatch: true},
		{name: "prefix glob no match", raw: "list*", arg: "get", wantMatch: false},
		{name: "char class match", raw: "[a-z]*", arg: "hello", wantMatch: true},
		{name: "char class no match dash", raw: "[a-z]*", arg: "--help", wantMatch: false},
		{name: "char class no match digit", raw: "[a-z]*", arg: "123", wantMatch: false},
		{name: "question mark match", raw: "fo?", arg: "foo", wantMatch: true},
		{name: "question mark no match", raw: "fo?", arg: "fooo", wantMatch: false},
		{name: "flag exact", raw: "--help", arg: "--help", wantMatch: true},
		{name: "flag no match", raw: "--help", arg: "--version", wantMatch: false},
		{name: "escaped colon in glob", raw: `host\:8080`, arg: "host:8080", wantMatch: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seg, err := ParseSegment(tt.raw)
			if err != nil {
				t.Fatalf("ParseSegment(%q) error: %v", tt.raw, err)
			}
			got := seg.Match(tt.arg)
			if got != tt.wantMatch {
				t.Errorf("Segment(%q).Match(%q) = %v, want %v", tt.raw, tt.arg, got, tt.wantMatch)
			}
		})
	}
}

func TestParseSegments(t *testing.T) {
	t.Run("valid list", func(t *testing.T) {
		patterns := []string{"[a-z]*:+", "list*", "*:*"}
		segs, err := ParseSegments(patterns)
		if err != nil {
			t.Fatalf("ParseSegments error: %v", err)
		}
		if len(segs) != 3 {
			t.Fatalf("got %d segments, want 3", len(segs))
		}
		// First: [a-z]* with 1+
		if segs[0].GlobPattern != "[a-z]*" || segs[0].Quantifier.Min != 1 || segs[0].Quantifier.Max != -1 {
			t.Errorf("segment 0: got %+v", segs[0])
		}
		// Second: list* with :1
		if segs[1].GlobPattern != "list*" || segs[1].Quantifier.Min != 1 || segs[1].Quantifier.Max != 1 {
			t.Errorf("segment 1: got %+v", segs[1])
		}
		// Third: * with 0+
		if segs[2].GlobPattern != "*" || segs[2].Quantifier.Min != 0 || segs[2].Quantifier.Max != -1 {
			t.Errorf("segment 2: got %+v", segs[2])
		}
	})

	t.Run("error in list", func(t *testing.T) {
		patterns := []string{"foo", ":bad", "bar"}
		_, err := ParseSegments(patterns)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("empty list", func(t *testing.T) {
		segs, err := ParseSegments(nil)
		if err != nil {
			t.Fatalf("ParseSegments(nil) error: %v", err)
		}
		if len(segs) != 0 {
			t.Fatalf("got %d segments, want 0", len(segs))
		}
	})
}

func TestSplitSegment(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantGlob  string
		wantQuant string
		wantColon bool
		wantErr   bool
	}{
		{name: "no colon", raw: "foo", wantGlob: "foo", wantQuant: "", wantColon: false},
		{name: "one colon", raw: "foo:+", wantGlob: "foo", wantQuant: "+", wantColon: true},
		{name: "escaped colon", raw: `foo\:bar`, wantGlob: `foo\:bar`, wantQuant: "", wantColon: false},
		{name: "escaped then real", raw: `foo\:bar:+`, wantGlob: `foo\:bar`, wantQuant: "+", wantColon: true},
		{name: "two real colons", raw: "a:b:c", wantErr: true},
		{name: "escaped and two real", raw: `a\:b:c:d`, wantErr: true},
		{name: "colon at start", raw: ":foo", wantGlob: "", wantQuant: "foo", wantColon: true},
		{name: "colon at end", raw: "foo:", wantGlob: "foo", wantQuant: "", wantColon: true},
		{name: "only escaped colons", raw: `\:\:`, wantGlob: `\:\:`, wantQuant: "", wantColon: false},
		{name: "backslash not before colon", raw: `fo\o:+`, wantGlob: `fo\o`, wantQuant: "+", wantColon: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, q, hasColon, err := splitSegment(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got glob=%q quant=%q", g, q)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if g != tt.wantGlob {
				t.Errorf("glob = %q, want %q", g, tt.wantGlob)
			}
			if q != tt.wantQuant {
				t.Errorf("quant = %q, want %q", q, tt.wantQuant)
			}
			if hasColon != tt.wantColon {
				t.Errorf("hasColon = %v, want %v", hasColon, tt.wantColon)
			}
		})
	}
}

func TestParseQuantifier(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		wantMin int
		wantMax int
		wantErr bool
	}{
		// Default
		{name: "empty", s: "", wantMin: 1, wantMax: 1},

		// Shorthands
		{name: "star", s: "*", wantMin: 0, wantMax: -1},
		{name: "plus", s: "+", wantMin: 1, wantMax: -1},
		{name: "question", s: "?", wantMin: 0, wantMax: 1},

		// Exact N
		{name: "0", s: "0", wantMin: 0, wantMax: 0},
		{name: "1", s: "1", wantMin: 1, wantMax: 1},
		{name: "5", s: "5", wantMin: 5, wantMax: 5},
		{name: "99", s: "99", wantMin: 99, wantMax: 99},

		// N+
		{name: "0+", s: "0+", wantMin: 0, wantMax: -1},
		{name: "1+", s: "1+", wantMin: 1, wantMax: -1},
		{name: "3+", s: "3+", wantMin: 3, wantMax: -1},

		// N-M
		{name: "0-1", s: "0-1", wantMin: 0, wantMax: 1},
		{name: "2-5", s: "2-5", wantMin: 2, wantMax: 5},
		{name: "0-0", s: "0-0", wantMin: 0, wantMax: 0},
		{name: "10-20", s: "10-20", wantMin: 10, wantMax: 20},

		// Invalid
		{name: "word", s: "abc", wantErr: true},
		{name: "negative", s: "-1", wantErr: true},
		{name: "min > max", s: "5-3", wantErr: true},
		{name: "bad N+", s: "abc+", wantErr: true},
		{name: "bad range left", s: "a-5", wantErr: true},
		{name: "bad range right", s: "5-b", wantErr: true},
		{name: "symbol", s: "!", wantErr: true},
		{name: "double plus", s: "++", wantErr: true},
		{name: "double star", s: "**", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := parseQuantifier(tt.s)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got %+v", q)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if q.Min != tt.wantMin {
				t.Errorf("Min = %d, want %d", q.Min, tt.wantMin)
			}
			if q.Max != tt.wantMax {
				t.Errorf("Max = %d, want %d", q.Max, tt.wantMax)
			}
		})
	}
}
